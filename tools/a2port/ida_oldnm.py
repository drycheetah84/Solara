import json, traceback, idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(rva,cap=2500):
        try:
            f=ida_funcs.get_func(IB+rva); cf=ida_hexrays.decompile(f.start_ea if f else IB+rva)
            return (hex((f.start_ea-IB) if f else rva), str(cf)[:cap] if cf else None)
        except Exception as e: return (hex(rva),"ERR:"+str(e))
    out={"imagebase":hex(IB)}
    fr,dc=dec(0x54AF360); out["fn_start"]=fr; out["decomp"]=dc
    # also its callers (who uses netmode to gate) - names
    callers=[]
    f=ida_funcs.get_func(IB+0x54AF360)
    if f:
        for xr in idautils.CodeRefsTo(f.start_ea,0):
            cf=ida_funcs.get_func(xr)
            if cf: callers.append((hex(cf.start_ea-IB), idc.get_func_name(cf.start_ea)))
    out["callers"]=callers[:20]
    json.dump(out,open(A+r"\oldnm-dump.json","w"),indent=1,default=str)
    open(A+r"\oldnm_ok.txt","w").write("ok")
except Exception:
    open(A+r"\oldnm_err.txt","w").write(traceback.format_exc())
