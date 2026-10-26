# OLD 22284 IDB. Characterize the server-init / load crash-fix hook targets so
# they can be re-found in 47031: name, size, named callees, strings, pseudo head.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\oldinith_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    def named_callees(start):
        out=[]
        f=ida_funcs.get_func(start)
        if not f: return out
        for item in idautils.FuncItems(f.start_ea):
            for xr in idautils.CodeRefsFrom(item,0):
                cf=ida_funcs.get_func(xr)
                if cf and cf.start_ea!=f.start_ea:
                    n=idc.get_func_name(cf.start_ea)
                    if n and not n.startswith("sub_"): out.append(n)
        return list(dict.fromkeys(out))[:14]
    def strs(start):
        out=[]; f=ida_funcs.get_func(start)
        if not f: return out
        for item in idautils.FuncItems(f.start_ea):
            for d in idautils.DataRefsFrom(item):
                s=idc.get_strlit_contents(d,-1,idc.STRTYPE_C) or idc.get_strlit_contents(d,-1,idc.STRTYPE_C_16)
                if s:
                    try: out.append(s.decode("utf-8","replace") if isinstance(s,bytes) else s)
                    except Exception: pass
        return list(dict.fromkeys(out))[:10]
    def pc(start,cap=1600):
        try: cf=ida_hexrays.decompile(start); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    T={"WorldGetNetMode":0x4040990,"ArrCopyDrv":0x12F0CB0,"CosmeticCmp":0x53DA880}
    out={}
    for nm,r in T.items():
        f=ida_funcs.get_func(IB+r)
        if not f: out[nm]={"err":"no func"}; continue
        out[nm]={"rva":hex(r),"func":idc.get_func_name(f.start_ea),"size":f.end_ea-f.start_ea,
                 "named_callees":named_callees(f.start_ea),"strings":strs(f.start_ea),"pseudo":pc(f.start_ea)}
        log(nm,out[nm]["func"],"callees",out[nm]["named_callees"][:4])
    json.dump(out,open(A+r"\oldinith-dump.json","w"),indent=1,default=str)
    open(A+r"\oldinith_ok.txt","w").write("ok")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\oldinith_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
