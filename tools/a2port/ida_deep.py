# Headless deep-resolve against the NEW (47031) IDB. Read-only.
# Produces deep-dump.json: for every pending site, the raw evidence needed to
# decide the port deterministically (thunk call-targets, string xrefs,
# distinctive-callee caller-sets, data refs of referencing funcs). Decisions
# are made offline in Python so we never reload the 2.3GB IDB to iterate.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\deep_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n")
    LOG.flush()


try:
    ida_hexrays.init_hexrays_plugin()
    IB = idaapi.get_imagebase()
    log("imagebase", hex(IB))

    def rva(ea):
        return ea - IB

    def va(r):
        return IB + r

    pend = json.load(open(A + r"\pending-work.json"))
    old = {s["name"]: s for s in json.load(open(A + r"\old-ida.json"))["sites"]}
    ucand = json.load(open(A + r"\ufunc-cands.json"))

    # ---- NEW indexes -------------------------------------------------------
    log("building name2ea...")
    name2ea = {}
    all_funcs = []
    for ea in idautils.Functions():
        all_funcs.append(ea)
        n = idc.get_func_name(ea)
        if n and not n.startswith("sub_"):
            name2ea.setdefault(n, ea)
    log("named funcs:", len(name2ea), "total:", len(all_funcs))

    def finfo(ea):
        f = ida_funcs.get_func(ea)
        if not f:
            return None
        return {"start": hex(rva(f.start_ea)), "size": f.end_ea - f.start_ea,
                "name": idc.get_func_name(f.start_ea)}

    def call_targets(start):
        out, seen = [], set()
        f = ida_funcs.get_func(start)
        if not f:
            return out
        for item in idautils.FuncItems(start):
            for xr in idautils.CodeRefsFrom(item, 0):
                cf = ida_funcs.get_func(xr)
                if cf and cf.start_ea != start and cf.start_ea not in seen:
                    seen.add(cf.start_ea)
                    out.append({"rva": hex(rva(cf.start_ea)),
                                "name": idc.get_func_name(cf.start_ea),
                                "size": cf.end_ea - cf.start_ea})
        return out

    def data_refs(start):
        out = []
        for item in idautils.FuncItems(start):
            for d in idautils.DataRefsFrom(item):
                if not ida_funcs.get_func(d):
                    out.append(rva(d))
        return sorted(set(out))

    def decomp(start, cap=6000):
        try:
            cf = ida_hexrays.decompile(start)
            if cf:
                return str(cf)[:cap]
        except Exception as e:
            return "DECOMP_ERR:" + str(e)
        return None

    # string -> [func rva]
    log("building string index...")
    want_strings = set()
    for w in pend:
        for s in w.get("strings", []):
            if len(s) >= 6:
                want_strings.add(s)
    for k, c in ucand.items():
        for s in c.get("old_strings", []):
            if len(s) >= 6:
                want_strings.add(s)
    str2funcs = {}
    if want_strings:
        for st in idautils.Strings():
            try:
                txt = str(st)
            except Exception:
                continue
            if txt in want_strings:
                fs = set()
                for x in idautils.XrefsTo(st.ea):
                    f = ida_funcs.get_func(x.frm)
                    if f:
                        fs.add(rva(f.start_ea))
                if fs:
                    str2funcs.setdefault(txt, []).extend(sorted(fs))
    for k in list(str2funcs):
        str2funcs[k] = sorted(set(str2funcs[k]))
    log("resolved strings:", len(str2funcs))

    # reverse caller index for distinctive callees
    log("collecting distinctive callees...")
    want_callees = set()
    for w in pend:
        for c in w.get("callees", []):
            want_callees.add(c)
    callee_callers = {}
    for cn in want_callees:
        cea = name2ea.get(cn)
        if cea is None:
            continue
        callers = set()
        for x in idautils.CodeRefsTo(cea, 0):
            cf = ida_funcs.get_func(x)
            if cf:
                callers.add(rva(cf.start_ea))
        callee_callers[cn] = sorted(callers)
    log("callee reverse-index entries:", len(callee_callers))

    out = {"imagebase": IB, "sites": {}, "ufunc": {}, "str2funcs": str2funcs,
           "callee_callers": callee_callers}

    # ---- UFunction thunks --------------------------------------------------
    for k, c in ucand.items():
        fi = c.get("funcinfo")
        rec = {"old": c, "thunk_rva": fi}
        if fi:
            try:
                tf = ida_funcs.get_func(va(int(fi, 16)))
                if tf:
                    rec["thunk"] = finfo(tf.start_ea)
                    rec["thunk_calls"] = call_targets(tf.start_ea)
                    rec["thunk_decomp"] = decomp(tf.start_ea, 2500)
                else:
                    rec["err"] = "no func at thunk rva"
            except Exception as e:
                rec["err"] = str(e)
        out["ufunc"][k] = rec
    log("ufunc thunks dumped:", len(out["ufunc"]))

    # ---- per pending site --------------------------------------------------
    for w in pend:
      try:
        nm = w["name"]
        o = old.get(nm, {})
        rec = {"conf_old": w["conf"], "is_func": w.get("is_func"),
               "inner": w.get("inner", 0), "old_rva": w["old"],
               "strings": w.get("strings", []), "callees": w.get("callees", []),
               "xref_funcs": w.get("xref_funcs", [])}
        rec["string_hits"] = {s: str2funcs.get(s, []) for s in w.get("strings", []) if s in str2funcs}
        dcall = [c for c in w.get("callees", []) if c in callee_callers]
        if dcall:
            sets = [set(callee_callers[c]) for c in dcall]
            inter = set.intersection(*sets) if sets else set()
            rank = {}
            for s in sets:
                for r in s:
                    rank[r] = rank.get(r, 0) + 1
            ranked = sorted(rank.items(), key=lambda kv: -kv[1])[:8]
            rec["struct_callees_used"] = dcall
            rec["struct_intersection"] = [hex(x) for x in sorted(inter)]
            rec["struct_ranked"] = [{"rva": hex(r), "hits": h, "info": finfo(va(r))}
                                    for r, h in ranked]
        if not w.get("is_func"):
            dref = {}
            for fn in w.get("xref_funcs", []):
                fea = name2ea.get(fn)
                if fea is not None:
                    dref[fn] = {"data_refs": [hex(x) for x in data_refs(fea)][:40],
                                "decomp": decomp(fea, 3500)}
            rec["data_via"] = dref
        rec["old_size"] = o.get("func_size")
        out["sites"][nm] = rec
      except Exception as e:
        out["sites"][w.get("name", "?")] = {"site_err": str(e)}
        log("site err", w.get("name"), e)

    json.dump(out, open(A + r"\deep-dump.json", "w"), indent=1, default=str)
    log("DONE sites:", len(out["sites"]))
    open(A + r"\deep_ok.txt", "w").write("ok %d" % len(out["sites"]))
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\deep_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
