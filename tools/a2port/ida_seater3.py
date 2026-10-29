# 47031 i64. Find the ball-sim seater/tick: it CALLS the physics core sub_5656C10
# and/or the hash lookups, walks world players (GObjects/GetByIndex), and returns
# double. Dump callers of sub_5656C10 / sub_564A8A0 / sub_5789730 with fingerprints.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\seater3_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def finger(f):
        d={"start":hex(f.start_ea-IB),"size":f.end_ea-f.start_ea}
        try:
            cf=ida_hexrays.decompile(f.start_ea); s=str(cf) if cf else ""
            d["sig"]=s.split("{")[0].strip()[:90]
            d["double"]=s.split("{")[0].strip().startswith("double")
            d["calls_physics"]="sub_145656C10" in s
            d["walks_obj"]=("GetByIndex" in s) or ("GObjects" in s) or ("NumElements" in s)
            d["nlines"]=s.count(chr(10))
            d["head"]=s[:500]
        except Exception as e: d["sig"]="ERR:"+str(e)
        return d
    out={"imagebase":hex(IB),"callers":{}}
    for tgt in [0x5656C10,0x564A8A0,0x5789730,0x5657120]:
        ea=IB+tgt; callers=[]; seen=set()
        for xr in idautils.XrefsTo(ea):
            if xr.type in (16,17):  # call far/near
                f=ida_funcs.get_func(xr.frm)
                if f and f.start_ea not in seen:
                    seen.add(f.start_ea); callers.append(finger(f))
        out["callers"]["sub_%X"%tgt]=callers
        log(hex(tgt),"callers",len(callers))
    json.dump(out,open(A+r"\seater3-dump.json","w"),indent=1,default=str)
    open(A+r"\seater3_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\seater3_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
