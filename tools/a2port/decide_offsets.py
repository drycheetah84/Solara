#!/usr/bin/env python3
"""Offline decision pass over deep-dump.json (no IDB reload).
Turns raw evidence into resolved 47031 RVAs with an honest confidence tag.
Method per category:
  UFunction thunk  -> the single non-helper call inside the exec-thunk IS the
                      _Implementation. 1 candidate => high; size-matched => high;
                      else med/ambiguous.
  string-anchored  -> distinctive string -> exactly 1 new func => high.
  structural       -> new func calling ALL the site's distinctive named callees;
                      unique intersection => med/high by #callees + size.
  data global      -> referencing named func's data refs (eyeball-assisted).
Prints a worksheet and writes resolved-offsets.json.
"""
import json, re, sys

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
d = json.load(open(A + r"\deep-dump.json"))

HELPER = re.compile(
    r"security_check_cookie|CheckForDebuggerJustMyCode|_alloca|__GSHandlerCheck|"
    r"guard_dispatch|guard_check|_RTC_|__chkstk|IsProcessorFeature|"
    r"^j_|ProcessEvent$|StaticFindObject|GetPrivateStaticClass", re.I)
MEMFN = re.compile(r"^(memset|memcpy|memmove|memcmp)$", re.I)


def hx(x):
    return x if isinstance(x, str) else hex(x)


# Build the set of shared param-step helpers from the whole thunk corpus:
# FFrame::Step dispatchers and per-param steppers are called by many thunks.
def build_helper_set(ufunc):
    from collections import Counter
    freq = Counter()
    for rec in ufunc.values():
        for c in (rec.get("thunk_calls") or []):
            freq[c["rva"]] += 1
    # a call reused across >=3 distinct exec-thunks is a shared marshalling helper
    return {rva for rva, n in freq.items() if n >= 3}


HELPER_RVAS = set()  # populated in main


def pick_impl(rec):
    """In a UFunction exec-thunk, the _Implementation is the single non-helper
    call (preferring the tail call). No such call => impl is inlined => the
    thunk itself IS the implementation hook target."""
    calls = rec.get("thunk_calls") or []
    old = rec.get("old", {})
    old_size = old.get("old_size", 0)
    thunk_rva = rec.get("thunk_rva")
    thunk_info = rec.get("thunk") or {}
    cands = []
    for c in calls:
        nm = c.get("name") or ""
        if c["rva"] in HELPER_RVAS:
            continue
        if HELPER.search(nm):
            continue
        if MEMFN.match(nm) and old_size > 120:
            continue
        if c["size"] < 12:            # decompiler-noise micro-stubs (e.g. Reserve)
            continue
        cands.append(c)
    if not cands:
        # impl inlined into the exec-thunk -> hook the thunk itself
        ts = thunk_info.get("size")
        close = old_size and ts and abs(ts - old_size) <= max(40, old_size * 0.6)
        return {"rva": thunk_rva, "size": ts, "name": thunk_info.get("name")}, \
            "inlined-in-thunk", ("high" if close else "med"), calls
    impl_named = [c for c in cands if c.get("name", "").endswith("_Implementation")]
    pool = impl_named or cands
    if len(pool) == 1:
        c = pool[0]
        close = old_size and abs(c["size"] - old_size) <= max(60, old_size * 0.6)
        return c, ("single-impl-named" if impl_named else "single-nonhelper-call"), \
            ("high" if close else "med"), calls
    # multiple non-helper calls: the _Implementation is the tail (last) call;
    # corroborate with size closeness
    pool.sort(key=lambda c: abs(c["size"] - old_size))
    best = pool[0]
    close = old_size and abs(best["size"] - old_size) <= max(60, old_size * 0.6)
    return best, "tail/size(%d cands)" % len(pool), ("high" if close else "med"), calls


def decide_struct(rec):
    inter = rec.get("struct_intersection") or []
    ranked = rec.get("struct_ranked") or []
    ncall = len(rec.get("struct_callees_used") or [])
    old_size = rec.get("old_size") or 0
    if len(inter) == 1:
        r = inter[0]
        info = next((x["info"] for x in ranked if x["rva"] == r), None)
        close = info and old_size and abs(info["size"] - old_size) <= max(60, old_size * 0.6)
        conf = "high" if (ncall >= 2 and close) else ("med" if ncall >= 2 else "low")
        return r, "struct-intersection(%d callees)" % ncall, conf, info
    if ranked:
        top = ranked[0]
        if len(ranked) == 1 or (len(ranked) > 1 and top["hits"] > ranked[1]["hits"]):
            info = top.get("info")
            close = info and old_size and abs(info["size"] - old_size) <= max(60, old_size * 0.6)
            return top["rva"], "struct-top(%d/%d hits)" % (top["hits"], ncall), \
                ("med" if close else "low"), info
    return None, "struct-ambiguous", "none", None


def decide_string(hits):
    singles = {s: v for s, v in hits.items() if len(v) == 1}
    if singles:
        s, v = sorted(singles.items(), key=lambda kv: -len(kv[0]))[0]
        return hx(v[0]), "string:%s" % s[:28], "high"
    sets = [set(v) for v in hits.values() if v]
    if sets:
        inter = set.intersection(*sets)
        if len(inter) == 1:
            return hx(next(iter(inter))), "string-intersect", "med"
    return None, "string-ambiguous", "none"


resolved = {}
HELPER_RVAS = build_helper_set(d.get("ufunc", {}))
print("shared step-helpers (called by >=3 thunks):", sorted(HELPER_RVAS))
print("=" * 78)
print("UFUNCTION THUNKS  (exec-thunk -> _Implementation)")
print("=" * 78)
for k, rec in d.get("ufunc", {}).items():
    if rec.get("err"):
        print(f"  {k:28} ERR {rec['err']}")
        resolved[k] = {"conf": "none", "why": rec["err"]}
        continue
    c, method, conf, calls = pick_impl(rec)
    old = rec.get("old", {})
    if c:
        resolved[k] = {"new": c["rva"], "impl_size": c["size"], "old_impl_size": old.get("old_size"),
                       "method": method, "conf": conf, "thunk": rec.get("thunk_rva"),
                       "old_func": old.get("old_func")}
        print(f"  [{conf:4}] {k:28} -> {c['rva']:>10} sz={c['size']:<5} "
              f"(old {old.get('old_size')}) via {method}  [{c.get('name') or 'sub_'}]")
    else:
        resolved[k] = {"conf": "none", "why": method, "thunk": rec.get("thunk_rva")}
        print(f"  [none] {k:28} {method}; thunk_calls={[ (x.get('name'),x['rva']) for x in (calls or []) ][:5]}")

print()
print("=" * 78)
print("PENDING SITES  (string / structural / data)")
print("=" * 78)
for nm, rec in d.get("sites", {}).items():
    if rec.get("site_err"):
        print(f"  {nm:30} SITE_ERR {rec['site_err']}")
        continue
    is_func = rec.get("is_func")
    inner = rec.get("inner", 0)
    old_size = rec.get("old_size")
    chosen = None
    # 1) string
    if rec.get("string_hits"):
        r, method, conf = decide_string(rec["string_hits"])
        if r:
            chosen = (r, method, conf)
    # 2) structural
    if not chosen and rec.get("struct_ranked") is not None:
        r, method, conf, info = decide_struct(rec)
        if r:
            chosen = (r, method, conf)
    # 3) data
    if not chosen and not is_func and rec.get("data_via"):
        # leave for manual eyeball but surface single-dataref funcs
        for fn, dv in rec["data_via"].items():
            dr = dv.get("data_refs") or []
            if len(dr) == 1:
                chosen = (dr[0], "data-xref:%s" % fn, "med")
                break
    if chosen:
        r, method, conf = chosen
        entry = {"new_func": r, "method": method, "conf": conf, "inner": inner,
                 "old_rva": rec.get("old_rva"), "old_size": old_size, "is_func": is_func}
        if inner:
            entry["note"] = "inner offset %d NOT auto-applied; verify instruction in func" % inner
            if conf == "high":
                conf = entry["conf"] = "med"
        resolved[nm] = entry
        print(f"  [{conf:4}] {nm:30} -> {r:>10} inner={inner:<4} via {method}")
    else:
        resolved[nm] = {"conf": "none", "inner": inner, "old_rva": rec.get("old_rva")}
        extra = ""
        if rec.get("data_via"):
            extra = " data_via=" + ",".join(rec["data_via"].keys())
        print(f"  [none] {nm:30} (inner={inner}){extra}")

by = {}
for v in resolved.values():
    by[v["conf"]] = by.get(v["conf"], 0) + 1
json.dump(resolved, open(A + r"\resolved-offsets.json", "w"), indent=1)
print()
print("by confidence:", by)
print("wrote resolved-offsets.json")
