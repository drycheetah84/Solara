# NAMED 47031 IDB. Pin the deserialize desync: decompile sub_144AB4DD0 (the
# "Properties"/"Root" object reader) and sub_144A80B00, and dump disassembly
# around the crash call site 0x4AB5411 to see what length/field is read. RO.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\desync_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(r,cap=9000):
        try: cf=ida_hexrays.decompile(IB+r); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    def disasm(r0,r1):
        out=[]; ea=IB+r0; end=IB+r1
        while ea<end:
            out.append("%08X  %s" % (ea-IB, idc.generate_disasm_line(ea,0)))
            ea=idc.next_head(ea,end)
        return out
    out={"imagebase":hex(IB)}
    out["sub_144AB4DD0"]=dec(0x4AB4DD0)
    out["sub_144A80B00"]=dec(0x4A80B00)
    out["sub_144AB2880"]=dec(0x4AB2880)
    # disassembly around the crash call site (0x4AB5411) — show the 0x60 before it
    out["callsite_disasm"]=disasm(0x4AB53B0,0x4AB5440)
    json.dump(out,open(A+r"\desync-dump.json","w"),indent=1,default=str)
    open(A+r"\desync_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\desync_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
