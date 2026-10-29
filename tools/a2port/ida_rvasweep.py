# 47031 i64. Staleness sweep: for each hardcoded RVA in dllmain_47031.cpp, is
# base+RVA actually a FUNCTION START (code) / valid data global on 47031? If it
# lands mid-function or wrong segment -> the RVA is stale (22284) and must be
# re-resolved. Also anchor the dashboard-login + quest-register functions by
# their unique strings so we can find the real 47031 RVAs.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes, ida_segment, ida_auto, ida_pro, ida_name
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\rvasweep_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_auto.auto_wait()
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def strrefs(ea,cap=40):
        out=[]
        f=ida_funcs.get_func(ea)
        if not f: return out
        for h in idautils.Heads(f.start_ea,f.end_ea):
            for xr in idautils.XrefsFrom(h,0):
                t=idc.get_str_type(xr.to)
                if t is not None and t>=0:
                    s=idc.get_strlit_contents(xr.to)
                    if s:
                        try: out.append(s.decode('utf-8','replace')[:60])
                        except: pass
            if len(out)>=cap: break
        return out
    CODE={
      "ballsim_reconcile":0x545BD80,"ballsim_getworld":0x3509AB0,"ballsim_getplayerbyid":0x54EB5E0,
      "ballsim_stepsim":0x54863C0,"ballsim_ingest":0x54599B0,"ballsim_buildworker":0x545CCB0,
      "overlaps":0x37B02C0,"goal_scorer":0x53AF7C0,"allocator":0x5361530,"quest_register":0x468BE60,
      "comp_fn_14147":0x46849A0,"dash_sessionlogin":0x54B5660,"dash_init_reader":0x5413CC0,
      # anchors known-good (sanity)
      "GOOD_fstrcopy":0x111B4A0,"GOOD_srvdispatch":0x5846E40,"GOOD_prelogin":0x558C600,
      "GOOD_getnetmode":0x41F5A60,"GOOD_mgrspawner":0x5567C30,"GOOD_deserstruct":0x4AB4DD0,
    }
    DATA={"ballsim_worldglobal":0xAC0A1A0,"dash_apikey_global":0x9BD4460,"rawdata_vtable":0x804BDA0}
    out={"imagebase":hex(IB),"code":{},"data":{},"anchors":{}}
    for nm,rva in CODE.items():
        ea=IB+rva; f=ida_funcs.get_func(ea)
        seg=ida_segment.getseg(ea); segn=ida_segment.get_segm_name(seg) if seg else "?"
        rec={"rva":hex(rva),"seg":segn,"is_code":idc.is_code(ida_bytes.get_flags(ea)),
             "func_start":bool(f and f.start_ea==ea),
             "in_func_at":hex(f.start_ea-IB) if f else None,
             "name":ida_name.get_name(ea) or (ida_name.get_name(f.start_ea) if f else "")}
        if f and f.start_ea==ea: rec["strrefs"]=strrefs(ea)
        out["code"][nm]=rec; log(nm,rec["func_start"],rec["seg"],rec.get("in_func_at"))
    for nm,rva in DATA.items():
        ea=IB+rva; seg=ida_segment.getseg(ea); segn=ida_segment.get_segm_name(seg) if seg else "?"
        out["data"][nm]={"rva":hex(rva),"seg":segn,"is_code":idc.is_code(ida_bytes.get_flags(ea)),
                          "name":ida_name.get_name(ea)}
        log("data",nm,segn)
    # anchor dashboard + quest by strings
    anchors=["log_in_with_key","x-api-key","No dashboard api key","DashboardApiKey",
             "Server_SetQuestProgressionAndInitializeQuests","log_in_with_key"]
    for s in idautils.Strings():
        try: txt=str(s)
        except: continue
        for aw in anchors:
            if aw.lower() in txt.lower():
                fns=[]
                for xr in idautils.XrefsTo(s.ea):
                    f=ida_funcs.get_func(xr.frm)
                    if f: fns.append(hex(f.start_ea-IB))
                out["anchors"].setdefault(aw,[]).append({"ea":hex(s.ea-IB),"txt":txt[:70],"xref_funcs":sorted(set(fns))})
    json.dump(out,open(A+r"\rvasweep-dump.json","w"),indent=1,default=str)
    open(A+r"\rvasweep_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\rvasweep_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
    try: ida_pro.qexit(0)
    except Exception: pass
