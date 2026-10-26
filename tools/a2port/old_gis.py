# Run on OLD 22284 IDB. Characterize the functions that reference GIsClient
# (0x96D8672) and GIsServer (0x96D8673) so they can be re-found in the named
# 47031 IDB: named callees, named callers, strings, size, and the referencing
# instruction's disassembly. Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\oldgis_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n"); LOG.flush()


try:
    ida_hexrays.init_hexrays_plugin()
    IB = idaapi.get_imagebase()
    log("OLD imagebase", hex(IB))

    def rva(ea): return ea - IB

    def named(ea):
        n = idc.get_func_name(ea)
        return n if (n and not n.startswith("sub_")) else None

    def characterize(func_ea, ref_ea):
        f = ida_funcs.get_func(func_ea)
        if not f:
            return None
        callees_named, callers_named = [], []
        for item in idautils.FuncItems(f.start_ea):
            for xr in idautils.CodeRefsFrom(item, 0):
                cf = ida_funcs.get_func(xr)
                if cf and cf.start_ea != f.start_ea:
                    nn = named(cf.start_ea)
                    if nn:
                        callees_named.append(nn)
        for xr in idautils.CodeRefsTo(f.start_ea, 0):
            cf = ida_funcs.get_func(xr)
            if cf:
                nn = named(cf.start_ea)
                if nn:
                    callers_named.append(nn)
        # strings
        strs = []
        for item in idautils.FuncItems(f.start_ea):
            for d in idautils.DataRefsFrom(item):
                s = idc.get_strlit_contents(d, -1, idc.STRTYPE_C) or idc.get_strlit_contents(d, -1, idc.STRTYPE_C_16)
                if s:
                    try:
                        strs.append(s.decode("utf-8", "replace") if isinstance(s, bytes) else s)
                    except Exception:
                        pass
        return {"func_rva": hex(rva(f.start_ea)), "func_name": idc.get_func_name(f.start_ea),
                "size": f.end_ea - f.start_ea,
                "ref_rva": hex(rva(ref_ea)), "ref_disasm": idc.generate_disasm_line(ref_ea, 0),
                "callees_named": list(dict.fromkeys(callees_named))[:16],
                "callers_named": list(dict.fromkeys(callers_named))[:16],
                "strings": list(dict.fromkeys(strs))[:10]}

    out = {}
    for nm, grva in (("GIsClient", 0x96D8672), ("GIsServer", 0x96D8673)):
        gea = IB + grva
        refs = []
        for x in idautils.XrefsTo(gea):
            f = ida_funcs.get_func(x.frm)
            if f:
                c = characterize(f.start_ea, x.frm)
                if c:
                    refs.append(c)
        # de-dup by func
        seen, urefs = set(), []
        for r in refs:
            if r["func_rva"] not in seen:
                seen.add(r["func_rva"]); uref = r; urefs.append(r)
        out[nm] = {"global_rva": hex(grva), "refs": urefs}
        log(nm, "refs", len(urefs), "named callees in first:",
            urefs[0]["callees_named"][:5] if urefs else [])

    json.dump(out, open(A + r"\oldgis-dump.json", "w"), indent=1, default=str)
    open(A + r"\oldgis_ok.txt", "w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\oldgis_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
