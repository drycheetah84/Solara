# 47031 i64. Port the NATIVE ball-sim bits: find the UOfflineBallSimSubsystem
# spawner (checks GetNetMode==2/0 then SpawnActor(ABallSimManager) and sets a
# world-global), so we can re-resolve: (a) the spawn-gate cmp netmode,2 site
# (22284 base+0x53DC768, patch byte +0x53DC76A), (b) the ballsim world-global
# (22284 base+0x9C5C880), (c) the spawner fn (22284 sub_7FF67743C730). Approach:
# find code xrefs to the "BallSimManager"/"OfflineBallSimSubsystem" FName string
# literals, decompile the referencing functions (the spawner is among them).
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\ballsim_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait()
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(ea,cap=9000):
        try:
            f=ida_funcs.get_func(ea); cf=ida_hexrays.decompile(f.start_ea if f else ea)
            return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    wanted=["BallSimManager","OfflineBallSimSubsystem"]
    out={"imagebase":hex(IB),"string_hits":{},"funcs":{}}
    seen=set()
    for s in idautils.Strings():
        try: txt=str(s)
        except: continue
        for w in wanted:
            if txt==w or txt==("/Script/A2."+w):
                out["string_hits"].setdefault(w,[]).append(hex(s.ea-IB))
                for xr in idautils.XrefsTo(s.ea):
                    f=ida_funcs.get_func(xr.frm)
                    if f and f.start_ea not in seen and len(out["funcs"])<24:
                        seen.add(f.start_ea)
                        out["funcs"][hex(f.start_ea-IB)]={"refs":w,"decomp":dec(f.start_ea)}
                        log("fn",hex(f.start_ea-IB),"refs",w)
    json.dump(out,open(A+r"\ballsim-dump.json","w"),indent=1,default=str)
    open(A+r"\ballsim_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\ballsim_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
