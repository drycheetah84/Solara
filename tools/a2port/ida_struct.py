# NAMED 47031 IDB. Resolve the remaining unnamed-internal FUNCTION sites by
# distinctive named-callee structure. Old callee names are normalised to the
# 47031 naming (strip Dumper's _Implementation suffix; A2->Ax class rename),
# looked up in the named IDB, and the new function calling that same callee set
# is the port. Ranked by callee-overlap then size. Read-only.
import json, re, traceback
import idaapi, idautils, idc, ida_funcs

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\struct_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n"); LOG.flush()


# unresolved FUNCTION sites -> their old func start (for size) ; callees come
# from old-ida.json
SITES = ["SimIntegrate_RVA", "StepSim_RVA", "Reconcile_RVA", "BallAdvance_RVA",
         "BallReconcile_RVA", "IngestInput_RVA", "SendResults_RVA", "ReplaySnapshot_RVA",
         "ReplayPostStep_RVA", "SendPhysImpl_RVA", "CosmeticCmp_RVA", "PrintString_RVA",
         "BallSimGetWorld_RVA", "JoinBuild_RVA", "SendJoin_RVA", "GiveCheck_RVA",
         "FetchRoles_RVA", "RolesFetched_RVA", "FNameResolve_RVA", "TextLayoutLeaf_RVA",
         "EvtDispatch_RVA", "WorldGetNetMode_RVA", "GtcTick_RVA"]


def normalize(cn):
    """old IDA/Dumper callee name -> candidate 47031 names."""
    out = set()
    base = cn
    # demangle MSVC StaticClass: ?StaticClass@UFoo@@... -> UFoo::StaticClass
    m = re.match(r"\?(\w+)@(\w+)@@", cn)
    if m:
        out.add("%s::%s" % (m.group(2), m.group(1)))
    # strip Dumper _Implementation suffix
    if base.endswith("_Implementation"):
        base = base[:-len("_Implementation")]
    out.add(base)
    out.add(base.replace("UA2", "UAx").replace("AA2", "AAx"))
    return {x for x in out if x and not x.startswith("sub_")}


try:
    IB = idaapi.get_imagebase()

    def rva(ea): return ea - IB

    def va(r): return IB + r

    old = {s["name"]: s for s in json.load(open(A + r"\old-ida.json"))["sites"]}

    # name -> ea in new
    name2ea = {}
    for ea in idautils.Functions():
        n = idc.get_func_name(ea)
        if n and not n.startswith("sub_"):
            name2ea.setdefault(n, ea)
    log("named funcs", len(name2ea))

    # reverse index built lazily per callee
    caller_cache = {}

    def callers_of(name):
        if name in caller_cache:
            return caller_cache[name]
        ea = name2ea.get(name)
        s = set()
        if ea is not None:
            for x in idautils.CodeRefsTo(ea, 0):
                f = ida_funcs.get_func(x)
                if f:
                    s.add(f.start_ea)
        caller_cache[name] = s
        return s

    def fsize(ea):
        f = ida_funcs.get_func(ea)
        return (f.end_ea - f.start_ea) if f else 0

    out = {}
    for site in SITES:
        o = old.get(site, {})
        oldsz = o.get("func_size", 0)
        raw = [c["name"] for c in o.get("callees", []) if not c["name"].startswith("sub_")]
        # normalise + keep only callees that exist (named) in new
        norm = []
        for cn in raw:
            cands = normalize(cn)
            hit = [c for c in cands if c in name2ea]
            if hit:
                norm.append(hit[0])
        norm = list(dict.fromkeys(norm))
        rec = {"old_size": oldsz, "old_callees_raw": raw[:8], "new_callees_used": norm}
        if not norm:
            rec["result"] = "no-named-callees"
            out[site] = rec
            log(site, "no named callees")
            continue
        sets = [callers_of(n) for n in norm]
        # rank candidate funcs by how many of our callees they call
        from collections import Counter
        cnt = Counter()
        for s in sets:
            for f in s:
                cnt[f] += 1
        ranked = cnt.most_common(8)
        rec["candidates"] = [{"rva": hex(rva(f)), "hits": h, "of": len(norm),
                              "size": fsize(f), "name": idc.get_func_name(f)} for f, h in ranked]
        # pick: max hits, then size closeness
        best = None
        if ranked:
            topn = max(h for _, h in ranked)
            tied = [f for f, h in ranked if h == topn]
            tied.sort(key=lambda f: abs(fsize(f) - oldsz))
            bf = tied[0]
            conf = "high" if (topn == len(norm) and topn >= 2 and len(tied) == 1) else (
                   "med" if topn >= 2 else "low")
            rec["pick"] = {"rva": hex(rva(bf)), "hits": topn, "of": len(norm),
                           "size": fsize(bf), "conf": conf, "tie": len(tied)}
        out[site] = rec
        log(site, "pick", rec.get("pick"))

    json.dump(out, open(A + r"\struct-dump.json", "w"), indent=1, default=str)
    open(A + r"\struct_ok.txt", "w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\struct_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
