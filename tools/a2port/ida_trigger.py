import json, traceback, idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
# 46986 caller RVAs above sub_144A80B00 (land in same 47031 funcs)
FRAMES=[0x4a9c940,0x4a3b68f,0x47b5104,0x418d0f3,0x3c87bc3,0x3ad2a2f,0x42cd05c,0x42db424]
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def info(r):
        f=ida_funcs.get_func(IB+r)
        if not f: return {"rva":hex(r),"err":"no func"}
        strs=[]
        for item in idautils.FuncItems(f.start_ea):
            for d in idautils.DataRefsFrom(item):
                s=idc.get_strlit_contents(d,-1,idc.STRTYPE_C) or idc.get_strlit_contents(d,-1,idc.STRTYPE_C_16)
                if s:
                    try: strs.append(s.decode('utf-8','replace') if isinstance(s,bytes) else str(s))
                    except: pass
        callees=[]
        for item in idautils.FuncItems(f.start_ea):
            for xr in idautils.CodeRefsFrom(item,0):
                cf=ida_funcs.get_func(xr)
                if cf and cf.start_ea!=f.start_ea:
                    n=idc.get_func_name(cf.start_ea)
                    if n and not n.startswith("sub_"): callees.append(n)
        try: dc=str(ida_hexrays.decompile(f.start_ea))[:2200]
        except Exception as e: dc="ERR:"+str(e)
        return {"rva":hex(r),"fn":idc.get_func_name(f.start_ea),"fn_rva":hex(f.start_ea-IB),
                "strings":list(dict.fromkeys(strs))[:15],"named_callees":list(dict.fromkeys(callees))[:15],"decomp":dc}
    out={"imagebase":hex(IB),"frames":[info(r) for r in FRAMES]}
    json.dump(out,open(A+r"\trigger-dump.json","w"),indent=1,default=str)
    open(A+r"\trigger_ok.txt","w").write("ok")
except Exception:
    open(A+r"\trigger_err.txt","w").write(traceback.format_exc())
