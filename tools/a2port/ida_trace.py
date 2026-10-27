# NAMED 47031 IDB. Trace the map-load/station deserialize crash chain: decompile
# each frame's containing function + its named callees, to find WHAT is being
# deserialized and where the bad length originates. Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\trace_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
# crash #3 (map-open FArchive serialize) stack RVAs, bottom(crash)->top(caller)
FRAMES=[0x111B5A6,0x110CFFD,0x11CC976,0x4AB5411,0x4AB2936,0x11C0002,0x4A88FCD,
        0x4955D6B,0x494D41B,0x12A90B0,0x4A80BB5,0x4985A16,0x49FA9AB,0x4960C07]
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    def va(r): return IB+r
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
        return list(dict.fromkeys(out))[:16]
    def strs(start):
        out=[]; f=ida_funcs.get_func(start)
        if not f: return out
        for item in idautils.FuncItems(f.start_ea):
            for d in idautils.DataRefsFrom(item):
                s=idc.get_strlit_contents(d,-1,idc.STRTYPE_C) or idc.get_strlit_contents(d,-1,idc.STRTYPE_C_16)
                if s:
                    try: out.append(s.decode("utf-8","replace") if isinstance(s,bytes) else s)
                    except Exception: pass
        return list(dict.fromkeys(out))[:12]
    def dec(start,cap=3200):
        try: cf=ida_hexrays.decompile(start); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    out={}
    seen=set()
    for r in FRAMES:
        f=ida_funcs.get_func(va(r))
        if not f: out[hex(r)]={"err":"no func"}; continue
        key=rva(f.start_ea)
        if key in seen: continue
        seen.add(key)
        out[hex(r)]={"func":idc.get_func_name(f.start_ea),"func_rva":hex(key),
                     "size":f.end_ea-f.start_ea,"callees":named_callees(f.start_ea),
                     "strings":strs(f.start_ea),"decomp":dec(f.start_ea)}
        log(hex(r),"->",out[hex(r)]["func"],"callees",out[hex(r)]["callees"][:4])
    json.dump(out,open(A+r"\trace-dump.json","w"),indent=1,default=str)
    open(A+r"\trace_ok.txt","w").write("ok")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\trace_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
