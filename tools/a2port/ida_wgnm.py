# NAMED 47031 IDB. Resolve WorldGetNetMode = the ~410-byte func that calls BOTH
# UReplaySubsystem::IsPlaying and ::IsRecording _Implementations. The funcinfo
# names point at the exec thunks; the real callers call the impl (the non-helper
# tail call inside each exec). So: exec -> impl -> callers, intersect.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\wgnm_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    def va(r): return IB+r
    def gcallers(ea):
        c=set()
        for x in idautils.CodeRefsTo(ea,0):
            f=ida_funcs.get_func(x)
            if f: c.add(f.start_ea)
        return c
    def impl_of(exec_rva):
        # non-helper call (global callers<=1000, size>=12) inside the exec = impl
        start=va(exec_rva); f=ida_funcs.get_func(start)
        if not f: return None
        best=None
        for item in idautils.FuncItems(f.start_ea):
            for xr in idautils.CodeRefsFrom(item,0):
                cf=ida_funcs.get_func(xr)
                if not cf or cf.start_ea==f.start_ea: continue
                nc=len(gcallers(cf.start_ea)); sz=cf.end_ea-cf.start_ea
                nm=idc.get_func_name(cf.start_ea)
                if nc<=1000 and sz>=12 and "security" not in (nm or ""):
                    best=cf.start_ea   # last non-helper = tail
        return best
    def info(ea):
        f=ida_funcs.get_func(ea)
        return {"rva":hex(rva(f.start_ea)),"size":f.end_ea-f.start_ea} if f else None
    ipl=impl_of(0x3fad000); irec=impl_of(0x3fad050)
    log("IsPlaying impl", hex(rva(ipl)) if ipl else None, "IsRecording impl", hex(rva(irec)) if irec else None)
    out={"IsPlaying_impl":info(ipl) if ipl else None,"IsRecording_impl":info(irec) if irec else None}
    if ipl and irec:
        cp=gcallers(ipl); cr=gcallers(irec); inter=sorted(cp & cr)
        out["candidates"]=[info(e) for e in inter]
        log("WorldGetNetMode candidates", [hex(rva(e)) for e in inter])
    json.dump(out,open(A+r"\wgnm-dump.json","w"),indent=1,default=str)
    open(A+r"\wgnm_ok.txt","w").write("ok")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\wgnm_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
