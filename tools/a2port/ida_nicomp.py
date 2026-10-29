# 47031 i64. Decompile UA2PawnNetInputComponent's ball-sim-region vtable overrides
# (from runtime NETINPUT dump) to find its TickComponent (per-frame rollback step)
# and the Server_NewSubmitInputs handler. Look for netmode gates + NewInputBuffer@0xC8.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\nicomp_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    # NETINPUT ball-sim-region slots captured at runtime
    RVAS={"s64":0x56B3490,"s86":0x54268E0,"s151":0x56AC960,"s152":0x56ACC50,"s153":0x56ACAE0,
          "s154":0x56ACA10,"s155":0x56D38E0,"s156":0x56D2860,"s157":0x5425820,
          "s166":0x5426D90,"s168":0x5426EF0,"s170":0x5427040,"s172":0x5427110,
          "s174":0x5427F70,"s176":0x5428220,"s178":0x54286F0}
    out={"imagebase":hex(IB),"funcs":{}}
    for nm,rva in RVAS.items():
        ea=IB+rva; f=ida_funcs.get_func(ea)
        if not f: out["funcs"][nm]={"rva":hex(rva),"err":"nofunc"}; continue
        try: cf=ida_hexrays.decompile(f.start_ea); s=str(cf) if cf else ""
        except Exception as e: s="ERR:"+str(e)
        # fingerprints: netmode gate, NewInputBuffer@0xC8, tick-ish
        fp=[]
        if "1441F5A60" in s: fp.append("netmode")
        if "+ 200)" in s or "0xC8" in s or "+ 0xC8" in s: fp.append("NewInputBuffer?")
        if "float" in s.split("{")[0]: fp.append("float-arg")
        out["funcs"][nm]={"rva":hex(rva),"size":f.end_ea-f.start_ea,"fp":fp,
                          "sig":s.split("{")[0].strip()[:80],"decomp":s[:3500]}
        log(nm,rva,fp)
    json.dump(out,open(A+r"\nicomp-dump.json","w"),indent=1,default=str)
    open(A+r"\nicomp_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\nicomp_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
