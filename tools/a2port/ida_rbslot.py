# 47031 i64. Decompile AA2RollbackManager's one ball-sim override (slot116=0x56E7A90)
# and the old mgr's slot116 (0x56E7C30) for comparison, plus look at what the net-input
# component's tick/handlers do, to find where the rollback sim actually STEPS.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\rbslot_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    RVAS={"rbmgr_slot116":0x56E7A90,"oldmgr_slot116":0x56E7C30,
          "netinput_s155":0x56D38E0,"netinput_s156":0x56D2860,"netinput_s64":0x56B3490}
    out={"imagebase":hex(IB),"funcs":{}}
    for nm,rva in RVAS.items():
        ea=IB+rva; f=ida_funcs.get_func(ea)
        if not f: out["funcs"][nm]={"rva":hex(rva),"err":"nofunc"}; continue
        try: cf=ida_hexrays.decompile(f.start_ea); s=str(cf) if cf else ""
        except Exception as e: s="ERR:"+str(e)
        out["funcs"][nm]={"rva":hex(rva),"size":f.end_ea-f.start_ea,"decomp":s[:5500]}
        log(nm,"len",len(s))
    json.dump(out,open(A+r"\rbslot-dump.json","w"),indent=1,default=str)
    open(A+r"\rbslot_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\rbslot_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
