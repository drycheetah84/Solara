# NAMED 47031 IDB. Resolve A2's netmode getters: decompile the exec thunks for
# UA2NetworkUtilityBPFL::GetNetMode / IsDedicatedServer / IsServer, extract the
# single non-helper call = _Implementation (what native server-gating calls).
# Also decompile each impl so we can confirm it reads the real netmode. RO.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\netmode_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
THUNKS={"GetNetMode":0x22AD400,"IsDedicatedServer":0x22ADB10,"IsServer":0x22ADEB0}
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def caller_count(ea):
        return sum(1 for _ in idautils.CodeRefsTo(ea,0))
    def dec(ea,cap=1600):
        try:
            f=ida_funcs.get_func(ea); cf=ida_hexrays.decompile(f.start_ea if f else ea)
            return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    def impl_of(thunk_rva):
        # collect direct call targets inside the thunk function
        f=ida_funcs.get_func(IB+thunk_rva)
        if not f: return None,[]
        cands=[]
        for item in idautils.FuncItems(f.start_ea):
            if idc.print_insn_mnem(item)=="call":
                for xr in idautils.CodeRefsFrom(item,0):
                    cf=ida_funcs.get_func(xr)
                    if cf and cf.start_ea!=f.start_ea:
                        cands.append(cf.start_ea)
        cands=list(dict.fromkeys(cands))
        # impl = the one with FEW global callers (not a shared helper >1000)
        scored=[(c, caller_count(c)) for c in cands]
        impl=[c for c,n in scored if n<1000]
        return (impl[0] if len(impl)==1 else None), [(hex(c-IB),n) for c,n in scored]
    out={"imagebase":hex(IB)}
    for name,rva in THUNKS.items():
        impl,cands=impl_of(rva)
        rec={"thunk_rva":hex(rva),"thunk_decomp":dec(IB+rva,1200),"candidates":cands,
             "impl_rva":hex(impl-IB) if impl else None,
             "impl_decomp":dec(impl,1600) if impl else None}
        out[name]=rec
        log(name,"impl",rec["impl_rva"],"cands",cands)
    json.dump(out,open(A+r"\netmode-dump.json","w"),indent=1,default=str)
    open(A+r"\netmode_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\netmode_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
