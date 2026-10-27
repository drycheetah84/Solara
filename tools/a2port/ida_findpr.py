# NAMED 47031 IDB. Resolve FCurlHttpRequest::ProcessRequest by the xref to its
# log format string, plus the containing func's prologue + the vtable entry that
# points at it (so Tellurium can be fed hardcoded A2 addresses). Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_bytes
A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\findpr_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    targets=["%p: request (easy handle:%p) has been added to threaded queue for processing",
             "STAT_FCurlHttpRequest_ProcessRequest"]
    out={"imagebase":hex(IB),"hits":[]}
    for st in idautils.Strings():
        try: t=str(st)
        except Exception: continue
        if t in targets:
            rec={"string":t,"str_ea":hex(rva(st.ea)),"xref_funcs":[]}
            for x in idautils.XrefsTo(st.ea):
                f=ida_funcs.get_func(x.frm)
                if f:
                    # bytes at function start (for prologue confirmation)
                    b=ida_bytes.get_bytes(f.start_ea,8) or b""
                    rec["xref_funcs"].append({"func_rva":hex(rva(f.start_ea)),
                        "name":idc.get_func_name(f.start_ea),
                        "size":f.end_ea-f.start_ea,
                        "prologue":b.hex()})
            out["hits"].append(rec)
            log(t, rec["xref_funcs"])
    # For each candidate ProcessRequest func, find a vtable entry (.rdata qword == func)
    for rec in out["hits"]:
        for xf in rec["xref_funcs"]:
            fea=IB+int(xf["func_rva"],16)
            vfts=[]
            for xr in idautils.XrefsTo(fea):
                # data xref from .rdata = vtable slot
                seg=idaapi.getseg(xr.frm)
                segname=idaapi.get_segm_name(seg) if seg else ""
                if segname in (".rdata",".data.rel.ro") or (seg and not ida_funcs.get_func(xr.frm)):
                    vfts.append(hex(rva(xr.frm)))
            xf["vtable_slots"]=vfts[:6]
    json.dump(out,open(A+r"\findpr-dump.json","w"),indent=1,default=str)
    open(A+r"\findpr_ok.txt","w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\findpr_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
