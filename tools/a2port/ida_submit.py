# 47031 i64. Find the ball-sim INPUT RPC (Server_SubmitInputs and kin) -> its exec
# + _Implementation -> the ingest/create-sim path. The create-sim is where a player
# first gets a sim entry on the server. Search candidate FName strings, dump xref
# funcs + decompiles; also flag funcs that reference the sim-array offset 0x460.
import json, re, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\submit_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(ea,cap=9000):
        f=ida_funcs.get_func(ea)
        if not f: return None,None
        try: cf=ida_hexrays.decompile(f.start_ea); return hex(f.start_ea-IB), (str(cf)[:cap] if cf else "")
        except Exception as e: return hex(f.start_ea-IB), "ERR:"+str(e)
    wanted=["SubmitInput","SubmitSimInput","SimInput","Server_SubmitInputs","SubmitInputs",
            "SubmitBallSimInput","ServerSubmitInputs","Server_SubmitSimInputs"]
    out={"imagebase":hex(IB),"string_hits":{},"funcs":{}}
    seen=set()
    for s in idautils.Strings():
        try: txt=str(s)
        except: continue
        for w in wanted:
            if w.lower() in txt.lower():
                fns=[]
                for xr in idautils.XrefsTo(s.ea):
                    f=ida_funcs.get_func(xr.frm)
                    if f: fns.append(hex(f.start_ea-IB))
                out["string_hits"].setdefault(w,[]).append({"ea":hex(s.ea-IB),"txt":txt[:70],"xref_funcs":sorted(set(fns))})
                for xr in idautils.XrefsTo(s.ea):
                    f=ida_funcs.get_func(xr.frm)
                    if f and f.start_ea not in seen:
                        seen.add(f.start_ea)
                        st,dc=dec(f.start_ea)
                        out["funcs"][st]={"refs":txt[:50],"decomp":dc}
                        log("fn",st,"refs",txt[:40])
    json.dump(out,open(A+r"\submit-dump.json","w"),indent=1,default=str)
    open(A+r"\submit_ok.txt","w").write("ok"); log("DONE hits",{k:len(v) for k,v in out['string_hits'].items()})
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\submit_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
