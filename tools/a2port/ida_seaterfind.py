# 47031 i64. Find the moved ball-sim seater/step/reconcile cluster. Anchor:
# xrefs to the ball-sim world-global qword_14AC0A1A0 (RVA 0xAC0A1A0, set by the
# spawner sub_5567C30; the sim paths key off it). Dump each referring function's
# signature + strrefs + size + whether it looks like the seater (double ret, big,
# references a StaticClass / vtable call). Also xref ADiscEntity StaticClass if found.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_auto, ida_pro, ida_name
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\seaterfind_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait(); ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def info(f):
        d={"start":hex(f.start_ea-IB),"size":f.end_ea-f.start_ea}
        try:
            cf=ida_hexrays.decompile(f.start_ea)
            if cf:
                s=str(cf); d["sig"]=s.split("{")[0].strip()[:90]
                d["has_double"]=("double" in d["sig"])
                # count indirect calls (vtable) and loops as seater fingerprint
                d["nlines"]=s.count(chr(10))
        except Exception as e: d["sig"]="ERR:"+str(e)
        return d
    out={"imagebase":hex(IB),"worldglobal_xrefs":[],"notes":[]}
    gea=IB+0xAC0A1A0
    seen=set()
    for xr in idautils.XrefsTo(gea):
        f=ida_funcs.get_func(xr.frm)
        where="code@"+hex(xr.frm-IB)
        if f and f.start_ea not in seen:
            seen.add(f.start_ea)
            rec=info(f); rec["xref_at"]=hex(xr.frm-IB); rec["xref_type"]=idc.XrefTypeName(xr.type) if hasattr(idc,'XrefTypeName') else xr.type
            out["worldglobal_xrefs"].append(rec)
            log("xref fn",rec["start"],rec.get("sig"))
        elif not f:
            out["notes"].append("xref from non-func "+where)
    # Also: the spawner sub_5567C30 - list its callees that take the manager (candidate tick/reconcile)
    sp=ida_funcs.get_func(IB+0x5567C30)
    out["spawner_callees"]=[]
    if sp:
        for h in idautils.Heads(sp.start_ea,sp.end_ea):
            for xr in idautils.XrefsFrom(h,0):
                if xr.type in (16,17):  # call
                    tf=ida_funcs.get_func(xr.to)
                    if tf: out["spawner_callees"].append(hex(tf.start_ea-IB))
    json.dump(out,open(A+r"\seaterfind-dump.json","w"),indent=1,default=str)
    open(A+r"\seaterfind_ok.txt","w").write("ok"); log("DONE xrefs=",len(out["worldglobal_xrefs"]))
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\seaterfind_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
