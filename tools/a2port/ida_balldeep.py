# Decompile the native ball-sim cluster + dashboard-login at a list of RVAs,
# normalizing each to its containing function start, dumping decomp + string refs.
# Run on BOTH IDBs (22284 ground-truth, 47031 target) to match functions and read
# the real 47031 struct offsets the game's own seater/step/ingest use.
import json, os, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro, ida_nalt
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
idbp=idc.get_idb_path() or ida_nalt.get_input_file_path() or "unknown"
base=os.path.basename(idbp)
tag="22284" if ("22284" in base or "old" in base.lower()) else "47031"
LOG=open(A+r"\balldeep_%s_log.txt"%tag,"w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    RVAS={"seater":0x545BD80,"build_worker":0x545CCB0,"stepsim":0x54863C0,"ingest":0x54599B0,
          "getworld":0x3509AB0,"getplayerbyid":0x54EB5E0,"dash_login":0x56890C0,
          "dash_apikey_fn":0x567E320,"dash_apikey_fmt":0x5636D00}
    def strrefs(f,cap=50):
        out=[]
        for h in idautils.Heads(f.start_ea,f.end_ea):
            for xr in idautils.XrefsFrom(h,0):
                t=idc.get_str_type(xr.to)
                if t is not None and t>=0:
                    s=idc.get_strlit_contents(xr.to)
                    if s:
                        try: out.append(s.decode('utf-8','replace')[:50])
                        except: pass
            if len(out)>=cap: break
        return out
    out={"idb":base,"tag":tag,"imagebase":hex(IB),"funcs":{}}
    for nm,rva in RVAS.items():
        ea=IB+rva; f=ida_funcs.get_func(ea)
        rec={"rva":hex(rva)}
        if not f: rec["err"]="no func"; out["funcs"][nm]=rec; log(nm,"nofunc"); continue
        rec["func_start_rva"]=hex(f.start_ea-IB); rec["is_start"]=(f.start_ea==ea)
        try:
            cf=ida_hexrays.decompile(f.start_ea); rec["decomp"]=str(cf)[:6500] if cf else None
        except Exception as e: rec["decomp"]="ERR:"+str(e)
        rec["strrefs"]=strrefs(f)
        out["funcs"][nm]=rec; log(nm,rec["func_start_rva"],rec["is_start"])
    json.dump(out,open(A+r"\balldeep-%s.json"%tag,"w"),indent=1,default=str)
    open(A+r"\balldeep_%s_ok.txt"%tag,"w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\balldeep_%s_err.txt"%tag,"w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
