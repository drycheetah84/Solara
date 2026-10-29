# 47031 i64. Find the server PreLogin auth gate that rejects joins with
# "Error initializing server" (private-server bypass target). Locate that string
# + the "LOGIN: Processed login, result:" logger + "PreLogin connect request",
# find the functions that reference them, decompile so we can neuter the gate.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\prelogin_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait()
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(ea,cap=12000):
        try:
            f=ida_funcs.get_func(ea); cf=ida_hexrays.decompile(f.start_ea if f else ea)
            return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    wanted=["Error initializing server","Processed login","PreLogin connect request","initializing server"]
    hits={w:[] for w in wanted}
    for s in idautils.Strings():
        try: txt=str(s)
        except: continue
        for w in wanted:
            if w in txt:
                hits[w].append((s.ea, txt[:80]))
    out={"imagebase":hex(IB),"string_hits":{},"funcs":{}}
    seenfn=set()
    for w,lst in hits.items():
        out["string_hits"][w]=[{"ea":hex(e-IB),"txt":t} for e,t in lst]
        for ea,_ in lst:
            for xr in idautils.XrefsTo(ea):
                f=ida_funcs.get_func(xr.frm)
                if f and f.start_ea not in seenfn:
                    seenfn.add(f.start_ea)
                    key=hex(f.start_ea-IB)
                    out["funcs"][key]={"refs_string":w,"decomp":dec(f.start_ea)}
                    log("fn",key,"refs",w)
    json.dump(out,open(A+r"\prelogin-dump.json","w"),indent=1,default=str)
    open(A+r"\prelogin_ok.txt","w").write("ok"); log("DONE hits:",{k:len(v) for k,v in hits.items()})
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\prelogin_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
