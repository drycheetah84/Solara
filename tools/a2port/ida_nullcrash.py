# NAMED 47031 IDB. The real SERVER crash: null-deref (RCX=0) at RVA 0x5846ECC
# during scraprun trap spawn. Decompile the crash fn + the exact faulting insn +
# the caller chain, to find what object is null. Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\nullcrash_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(r,cap=8000):
        try:
            f=ida_funcs.get_func(IB+r)
            cf=ida_hexrays.decompile(f.start_ea if f else IB+r)
            return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    def fnrva(r):
        f=ida_funcs.get_func(IB+r); return hex(f.start_ea-IB) if f else "?"
    def asm_around(r, before=0x30, after=0x10):
        out=[]; ea=IB+r-before; end=IB+r+after
        while ea<end:
            mark="  <<< CRASH" if ea==IB+r else ""
            out.append("%08X  %s%s"%(ea-IB, idc.generate_disasm_line(ea,0), mark))
            nxt=idc.next_head(ea,end+16)
            if nxt<=ea: break
            ea=nxt
        return out
    out={"imagebase":hex(IB)}
    out["crash_fn_rva"]=fnrva(0x5846ECC)
    out["crash_asm"]=asm_around(0x5846ECC)
    out["crash_decomp"]=dec(0x5846ECC)
    for r,label in [(0x3EE5DE8,"caller1"),(0x3EC0F30,"caller_outer"),(0x3EE15F3,"caller_top"),(0x4EBA271,"frame_4EBA271"),(0x36400C3,"frame_36400C3")]:
        out[label]={"rva_in":hex(r),"fn_rva":fnrva(r),"decomp":dec(r,5000)}
    json.dump(out,open(A+r"\nullcrash-dump.json","w"),indent=1,default=str)
    open(A+r"\nullcrash_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\nullcrash_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
