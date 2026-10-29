# 47031 i64. Decompile the live ball-sim vtable overrides captured at runtime, and
# list each one's called sub_ functions (the relocated reconcile/step/seater cluster).
import json, re, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\ballvt_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    RVAS={"MGR64":0x56EF200,"MGR86":0x5483700,"MGR116":0x56E7C30,"MGR160":0x57114C0,
          "SUB97":0x5712550,"SUB102":0x54B6368,"SUB106":0x54B64E0,
          "MGR0_dtor":0x54826F0,"SUB0_dtor":0x54B6A20}
    out={"imagebase":hex(IB),"funcs":{}}
    for nm,rva in RVAS.items():
        ea=IB+rva; f=ida_funcs.get_func(ea)
        rec={"rva":hex(rva)}
        if not f: rec["err"]="nofunc"; out["funcs"][nm]=rec; continue
        rec["start"]=hex(f.start_ea-IB); rec["size"]=f.end_ea-f.start_ea
        try:
            cf=ida_hexrays.decompile(f.start_ea); s=str(cf) if cf else ""
        except Exception as e: s="ERR:"+str(e)
        rec["decomp"]=s[:5000]
        # list called sub_14xxxxxxx with RVA
        calls=sorted(set("0x%X"%(int(m,16)-IB) for m in re.findall(r'sub_14([0-9A-Fa-f]{6,7})', s) for m in [ '14'+m ]))
        rec["calls"]=calls[:40]
        rec["sig"]=s.split("{")[0].strip()[:90]
        out["funcs"][nm]=rec; log(nm,rec.get("start"),rec.get("size"))
    json.dump(out,open(A+r"\ballvt-dump.json","w"),indent=1,default=str)
    open(A+r"\ballvt_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\ballvt_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
