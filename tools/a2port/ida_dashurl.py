# NAMED 47031 IDB. Find how DashboardApiUrl is read (FParse cmdline vs GConfig
# ini) by decompiling the function(s) referencing the "DashboardApiUrl" string
# and the default "https://api.oriondrift.net". Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\dashurl_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    def decomp(ea,cap=3000):
        try: cf=ida_hexrays.decompile(ea); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    targets=["DashboardApiUrl","DashboardApiUrl=","https://api.oriondrift.net"]
    out={"imagebase":hex(IB),"refs":[]}
    for s in idautils.Strings():
        try: t=str(s)
        except Exception: continue
        if t in targets:
            rec={"string":t,"ea":hex(rva(s.ea)),"funcs":[]}
            for x in idautils.XrefsTo(s.ea):
                f=ida_funcs.get_func(x.frm)
                if f:
                    rec["funcs"].append({"func":idc.get_func_name(f.start_ea),
                        "func_rva":hex(rva(f.start_ea)),"decomp":decomp(f.start_ea)})
            # also: no xref? note it (ascii default may be referenced by offset)
            out["refs"].append(rec); log(t, len(rec["funcs"]), "xref funcs")
    json.dump(out,open(A+r"\dashurl-dump.json","w"),indent=1,default=str)
    open(A+r"\dashurl_ok.txt","w").write("ok")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\dashurl_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
