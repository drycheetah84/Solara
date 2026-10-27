import json, traceback, idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(ea,cap=1500):
        try: cf=ida_hexrays.decompile(ea); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    hits=[]
    for ea in idautils.Functions():
        n=idc.get_func_name(ea)
        if n and "NetMode" in n:
            hits.append((hex(ea-IB), n))
    out={"imagebase":hex(IB),"names":hits}
    # decompile the ones that look like GetNetMode impl/exec
    out["detail"]={}
    for rva,n in hits:
        if "GetNetMode" in n:
            out["detail"][n]={"rva":rva,"decomp":dec(IB+int(rva,16))}
    json.dump(out,open(A+r"\nm2-dump.json","w"),indent=1,default=str)
    open(A+r"\nm2_ok.txt","w").write("ok")
except Exception:
    open(A+r"\nm2_err.txt","w").write(traceback.format_exc())
