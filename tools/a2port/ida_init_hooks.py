# NAMED 47031 IDB. Resolve the server-init / load crash-fix hooks needed for the
# map-open to survive as a server. Read-only.
#   WorldGetNetMode: the func calling BOTH UReplaySubsystem::IsPlaying and
#                    ::IsRecording (distinctive pair) -> intersection.
#   ArrCopyDrv (FArrayProperty copy driver) + CosmeticCmp: characterize the
#     22284 funcs' shape here is hard; instead dump candidates for offline review.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\inith_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    def va(r): return IB+r
    name2ea={}
    for ea in idautils.Functions():
        n=idc.get_func_name(ea)
        if n and not n.startswith("sub_"): name2ea.setdefault(n,ea)
    def callers(name):
        ea=name2ea.get(name); s=set()
        if ea is not None:
            for x in idautils.CodeRefsTo(ea,0):
                f=ida_funcs.get_func(x)
                if f: s.add(f.start_ea)
        return s
    def info(ea):
        f=ida_funcs.get_func(ea)
        if not f: return None
        return {"rva":hex(rva(f.start_ea)),"size":f.end_ea-f.start_ea,"name":idc.get_func_name(f.start_ea)}
    def dec(ea,cap=1800):
        try: cf=ida_hexrays.decompile(va(ea) if ea<IB else ea); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    out={}
    # WorldGetNetMode: IsPlaying ∩ IsRecording callers
    ip=callers("UReplaySubsystem::IsPlaying"); ir=callers("UReplaySubsystem::IsRecording")
    inter=sorted(ip & ir)
    out["WorldGetNetMode"]={"IsPlaying_callers":len(ip),"IsRecording_callers":len(ir),
        "intersection":[{"info":info(e),"decomp":dec(e)} for e in inter[:6]]}
    log("WorldGetNetMode inter", [hex(rva(e)) for e in inter])
    json.dump(out,open(A+r"\inith-dump.json","w"),indent=1,default=str)
    open(A+r"\inith_ok.txt","w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\inith_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
