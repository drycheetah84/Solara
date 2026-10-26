# Run inside the NEW (47031) IDB. Resolve every 22284 site from old-ida.json.
# Confidence ladder (never upgrade a weaker method to "high"):
#   name            old func name == a new func name                         high
#   string          a distinctive old string -> exactly one new func         high
#   string-intersect intersection of old strings -> one new func             med
#   sig             port_offsets candidate, confirmed by callee-name overlap  med/low
#   callee-search   new func whose named-callee set best matches old's        low
#   data            data global via a resolved referencing function's xref    med/low
# Instruction/patch sites (inner_offset>0) get the function then copy the
# inner offset, flagged for manual IDA verification (never "high").
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
try:
    OLD = json.load(open(A + r"\old-ida.json"))
    PORT = {r["name"]: r for r in json.load(open(A + r"\port-result.json"))}
    IB = idaapi.get_imagebase()
    ida_hexrays.init_hexrays_plugin()

    def rva(ea):
        return ea - IB

    # --- new-binary indexes -------------------------------------------------
    new_name2ea = {}
    new_callees = {}      # new func start -> frozenset(named callees)
    all_new = []
    for ea in idautils.Functions():
        all_new.append(ea)
        n = idc.get_func_name(ea)
        if n and not n.startswith("sub_"):
            new_name2ea.setdefault(n, ea)

    def named_callees_of(start):
        if start in new_callees:
            return new_callees[start]
        s = set()
        for item in idautils.FuncItems(start):
            for xr in idautils.CodeRefsFrom(item, 0):
                cf = ida_funcs.get_func(xr)
                if cf and cf.start_ea != start:
                    nm = idc.get_func_name(cf.start_ea)
                    if nm and not nm.startswith("sub_"):
                        s.add(nm)
        fs = frozenset(s)
        new_callees[start] = fs
        return fs

    new_str2funcs = {}
    for st in idautils.Strings():
        try:
            txt = str(st)
        except Exception:
            continue
        if len(txt) < 6:
            continue
        fs = set()
        for x in idautils.XrefsTo(st.ea):
            f = ida_funcs.get_func(x.frm)
            if f:
                fs.add(f.start_ea)
        if fs:
            new_str2funcs.setdefault(txt, set()).update(fs)

    results = {}

    def resolve(site):
        r = {"name": site["name"], "old": site["rva"], "kind": site["kind"],
             "is_func": site.get("is_func"), "inner_offset": site.get("inner_offset", 0),
             "old_func": site.get("func_name", ""), "new": None, "new_func_start": None,
             "method": None, "confidence": "none", "note": site.get("note", "")}
        if not site.get("is_func"):
            r["method"] = "data"; r["confidence"] = "todo-data"
            return r
        old_name = site.get("func_name", "")
        cand = None
        if old_name and not old_name.startswith("sub_") and old_name in new_name2ea:
            cand = new_name2ea[old_name]; r["method"] = "name"; r["confidence"] = "high"
        if cand is None:
            for strv in site.get("strings", []):
                fs = new_str2funcs.get(strv)
                if fs and len(fs) == 1:
                    cand = next(iter(fs)); r["method"] = "string"; r["confidence"] = "high"
                    r["via_string"] = strv[:60]; break
            if cand is None:
                inter = None
                for strv in site.get("strings", []):
                    fs = new_str2funcs.get(strv)
                    if fs:
                        inter = fs if inter is None else (inter & fs)
                if inter and len(inter) == 1:
                    cand = next(iter(inter)); r["method"] = "string-intersect"; r["confidence"] = "med"
        if cand is None:
            p = PORT.get(site["name"], {})
            nr = p.get("new_rva")
            if nr and nr != "None":
                try:
                    cea = IB + (int(nr, 16) if isinstance(nr, str) else nr)
                    f = ida_funcs.get_func(cea)
                    start = f.start_ea if f else cea
                    oc = set(c["name"] for c in site.get("callees", []) if not c["name"].startswith("sub_"))
                    ov = len(oc & named_callees_of(start)) if oc else 0
                    r["method"] = "sig"; r["sig_overlap"] = ov
                    r["confidence"] = "med" if ov >= 2 else "low"
                    cand = start
                except Exception:
                    pass
        if cand is None:
            # callee-sequence search: old named-callee set vs every new func
            oc = set(c["name"] for c in site.get("callees", []) if not c["name"].startswith("sub_"))
            if len(oc) >= 3:
                best, bestj = None, 0.0
                osz = site.get("func_size", 0)
                for start in all_new:
                    nc = named_callees_of(start)
                    if not nc:
                        continue
                    inter = len(oc & nc)
                    if inter < 2:
                        continue
                    j = inter / len(oc | nc)
                    if j > bestj:
                        bestj, best = j, start
                if best and bestj >= 0.6:
                    cand = best; r["method"] = "callee-search"; r["confidence"] = "low"; r["jaccard"] = round(bestj, 2)
        if cand is not None:
            f = ida_funcs.get_func(cand)
            start = f.start_ea if f else cand
            r["new_func_start"] = hex(rva(start))
            r["new_func_name"] = idc.get_func_name(start)
            if f:
                r["new_func_size"] = f.end_ea - f.start_ea
            r["old_func_size"] = site.get("func_size")
            if site.get("inner_offset"):
                r["new"] = hex(rva(start) + site["inner_offset"])
                r["inner_note"] = "inner offset copied verbatim; VERIFY instruction in IDA"
                if r["confidence"] == "high":
                    r["confidence"] = "med"
            else:
                r["new"] = hex(rva(start))
        results[site["name"]] = r
        return r

    out = []
    for s in OLD["sites"]:
        try:
            out.append(resolve(s))
        except Exception as e:
            out.append({"name": s["name"], "old": s["rva"], "method": "error",
                        "confidence": "none", "err": str(e)})

    # --- data pass: resolve data globals via a resolved referencing function -
    name2new = {r["name"]: r for r in out}
    for s in OLD["sites"]:
        if s.get("is_func"):
            continue
        r = name2new.get(s["name"])
        if not r:
            continue
        for xr in s.get("xrefs_to", []):
            innm = xr.get("in_func")
            if not innm or innm.startswith("sub_") or innm not in new_name2ea:
                continue
            nstart = new_name2ea[innm]
            # collect data refs (to .data/.rdata/.bss) from the new referencing func
            drefs = []
            for item in idautils.FuncItems(nstart):
                for d in idautils.DataRefsFrom(item):
                    if not ida_funcs.get_func(d):  # not code -> data
                        drefs.append(d)
            drefs = sorted(set(drefs))
            if len(drefs) == 1:
                r["new"] = hex(rva(drefs[0])); r["method"] = "data-xref"; r["confidence"] = "med"
                r["via_func"] = innm; break
            elif drefs:
                r["method"] = "data-xref-multi"; r["confidence"] = "low"
                r["candidates"] = [hex(rva(x)) for x in drefs[:12]]; r["via_func"] = innm; break

    by_conf = {}
    for r in out:
        by_conf[r["confidence"]] = by_conf.get(r["confidence"], 0) + 1
    json.dump({"results": out, "by_confidence": by_conf}, open(A + r"\new-map.json", "w"), indent=1)
    open(A + r"\match_ok.txt", "w").write(json.dumps(by_conf))
except Exception:
    open(A + r"\match_err.txt", "w").write(traceback.format_exc())
