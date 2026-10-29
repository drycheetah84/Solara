# 47031 i64. Full decompile of the ABallSimManager::Tick (0x57114C0) + its worker
# family (0x543DA00/0x543DBA0/0x543DE00/0x543E1D0) and subsystem tick body 0x5703CD0,
# to extract the server-authoritative seat/reconcile/step path + exact offsets.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\tick_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    RVAS={"Tick_57114C0":0x57114C0,"w_543DA00":0x543DA00,"w_543DBA0":0x543DBA0,
          "w_543DE00":0x543DE00,"w_543E1D0":0x543E1D0,"subtick_5703CD0":0x5703CD0}
    out={"imagebase":hex(IB),"funcs":{}}
    for nm,rva in RVAS.items():
        ea=IB+rva; f=ida_funcs.get_func(ea)
        if not f: out["funcs"][nm]={"rva":hex(rva),"err":"nofunc"}; continue
        try: cf=ida_hexrays.decompile(f.start_ea); s=str(cf) if cf else ""
        except Exception as e: s="ERR:"+str(e)
        out["funcs"][nm]={"rva":hex(rva),"size":f.end_ea-f.start_ea,"decomp":s[:22000]}
        log(nm,"len",len(s))
    json.dump(out,open(A+r"\tick-dump.json","w"),indent=1,default=str)
    open(A+r"\tick_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\tick_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
