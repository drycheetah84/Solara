# 47031 i64. Find Server_ExitSpectator (on SpectatorCameraManagerPawn): its exec +
# _Implementation, decompile to see the un-spectate transition + any net-ownership /
# netmode gate (explains why a server-side call didn't transition the client).
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\exitspec_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(ea,cap=8000):
        f=ida_funcs.get_func(ea)
        if not f: return None,None
        try: cf=ida_hexrays.decompile(f.start_ea); return hex(f.start_ea-IB),(str(cf)[:cap] if cf else "")
        except Exception as e: return hex(f.start_ea-IB),"ERR:"+str(e)
    out={"imagebase":hex(IB),"hits":[],"funcs":{}}
    seen=set()
    for s in idautils.Strings():
        try: txt=str(s)
        except: continue
        if "ExitSpectator" in txt:
            out["hits"].append({"ea":hex(s.ea-IB),"txt":txt[:60]})
            for xr in idautils.XrefsTo(s.ea):
                f=ida_funcs.get_func(xr.frm)
                if f and f.start_ea not in seen:
                    seen.add(f.start_ea); st,dc=dec(f.start_ea)
                    out["funcs"][st]={"refs":txt[:40],"decomp":dc}
                    log("fn",st)
    # Also: scan for functions whose name contains ExitSpectator (exec_/_Implementation)
    for ea,name in idautils.Names():
        if "ExitSpectator" in name:
            f=ida_funcs.get_func(ea)
            if f and f.start_ea not in seen:
                seen.add(f.start_ea); st,dc=dec(f.start_ea)
                out["funcs"][st]={"name":name,"decomp":dc}
                log("named",name,st)
    json.dump(out,open(A+r"\exitspec-dump.json","w"),indent=1,default=str)
    open(A+r"\exitspec_ok.txt","w").write("ok"); log("DONE hits",len(out["hits"]))
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\exitspec_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
