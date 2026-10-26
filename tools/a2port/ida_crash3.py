import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\c3_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def va(r): return IB+r
    def rva(ea): return ea-IB
    def dec(r,cap=2600):
        f=ida_funcs.get_func(va(r))
        if not f: return (None,None,None)
        try: cf=ida_hexrays.decompile(f.start_ea); s=str(cf)[:cap] if cf else None
        except Exception as e: s="ERR:"+str(e)
        return (idc.get_func_name(f.start_ea), hex(rva(f.start_ea)), s)
    out={}
    for r in (0x111B5A6,0x110CFFD,0x11CC976,0x4AB5411,0x4AB2936,0x494D41B,0x4955D6B):
        nm,fr,s=dec(r)
        out[hex(r)]={"func":nm,"func_rva":fr,"decomp":s}
        log(hex(r),"->",nm,fr)
    json.dump(out,open(A+r"\c3-dump.json","w"),indent=1,default=str)
    open(A+r"\c3_ok.txt","w").write("ok")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\c3_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
