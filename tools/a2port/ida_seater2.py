# 47031 i64. Decompile the 6 ball-sim world-global candidates fully, and resolve
# ABallSimManager::StaticClass (sub_54808C0) -> CDO -> vtable -> dump Tick + a few
# virtuals, to locate the reconcile/seater/step among them.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\seater2_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(rva,cap=7000):
        ea=IB+rva; f=ida_funcs.get_func(ea)
        if not f: return {"rva":hex(rva),"err":"nofunc"}
        try: cf=ida_hexrays.decompile(f.start_ea); s=str(cf) if cf else ""
        except Exception as e: s="ERR:"+str(e)
        return {"rva":hex(rva),"start":hex(f.start_ea-IB),"decomp":s[:cap]}
    out={"imagebase":hex(IB),"candidates":{}}
    for rva in [0x564A8A0,0x5656C10,0x5657120,0x5784D20,0x5789730]:
        out["candidates"][hex(rva)]=dec(rva)
        log("decompiled",hex(rva))
    json.dump(out,open(A+r"\seater2-dump.json","w"),indent=1,default=str)
    open(A+r"\seater2_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\seater2_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
