# NEW (47031) IDB. For each UFunction candidate (FunctionsInfo RVA), capture
# what's needed to decide thunk-vs-impl and confirm identity:
#   - the function's size, referenced strings, named callees
#   - its call targets (rva, size, strings, named callees) so a thunk can be
#     followed to the _Implementation it calls
# Decision is made in Python against the old oracle. Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
try:
    C = json.load(open(A + r"\ufunc-cands.json"))
    IB = idaapi.get_imagebase()
    ida_hexrays.init_hexrays_plugin()

    def rva(ea):
        return ea - IB

    def strings_of(start):
        out = []
        for item in idautils.FuncItems(start):
            for d in idautils.DataRefsFrom(item):
                s = idc.get_strlit_contents(d, -1, idc.STRTYPE_C) or idc.get_strlit_contents(d, -1, idc.STRTYPE_C_16)
                if s:
                    try:
                        out.append(s.decode("utf-8", "replace") if isinstance(s, bytes) else s)
                    except Exception:
                        pass
        return list(dict.fromkeys(out))[:10]

    def callees(start):
        named, subs = [], []
        for item in idautils.FuncItems(start):
            for xr in idautils.CodeRefsFrom(item, 0):
                cf = ida_funcs.get_func(xr)
                if cf and cf.start_ea != start:
                    nm = idc.get_func_name(cf.start_ea)
                    (named if nm and not nm.startswith("sub_") else subs).append((nm, cf.start_ea))
        return named, subs

    def info(ea):
        f = ida_funcs.get_func(ea)
        if not f:
            return None
        start = f.start_ea
        nm, subs = callees(start)
        return {"rva": hex(rva(start)), "name": idc.get_func_name(start),
                "size": f.end_ea - f.start_ea, "strings": strings_of(start),
                "named_callees": [n for n, _ in nm][:12],
                "sub_callees": [hex(rva(e)) for _, e in subs][:12],
                "sub_callee_sizes": [(hex(rva(e)), (ida_funcs.get_func(e).end_ea - e) if ida_funcs.get_func(e) else 0) for _, e in subs][:12]}

    out = {}
    for site, c in C.items():
        try:
            fea = IB + int(c["funcinfo"], 16)
            top = info(fea)
            rec = {"funcinfo": c["funcinfo"], "old_impl": c["old_impl"], "old_func": c["old_func"],
                   "old_strings": c["old_strings"], "old_callees": c["old_callees"], "old_size": c["old_size"],
                   "funcinfo_fn": top}
            # expand sub_ call targets (possible _Implementation) with their detail
            subdet = []
            for subrva in (top or {}).get("sub_callees", []):
                sea = IB + int(subrva, 16)
                subdet.append(info(sea))
            rec["sub_targets"] = subdet
            out[site] = rec
        except Exception as e:
            out[site] = {"err": str(e), "funcinfo": c.get("funcinfo")}
    json.dump(out, open(A + r"\ufunc-verify.json", "w"), indent=1)
    open(A + r"\ufunc_ok.txt", "w").write(str(len(out)))
except Exception:
    open(A + r"\ufunc_err.txt", "w").write(traceback.format_exc())
