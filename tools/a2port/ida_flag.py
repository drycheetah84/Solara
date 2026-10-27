# NAMED 47031 IDB. Investigate byte_14ABF9AE2 (the server/authority gate in the
# crash fn) + what a1+0xC80 vs a1+760 are. Find writers (who SETs the flag) and
# decompile them to learn what it means / whether HalcyonA2 should set it. RO.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\flag_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    FLAG=IB+0xABF9AE2
    def dec(ea,cap=2600):
        try:
            f=ida_funcs.get_func(ea); cf=ida_hexrays.decompile(f.start_ea if f else ea)
            return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    out={"imagebase":hex(IB),"flag_rva":hex(0xABF9AE2),"writers":[],"readers":[]}
    for xr in idautils.XrefsTo(FLAG):
        ea=xr.frm; mnem=idc.print_insn_mnem(ea); fn=ida_funcs.get_func(ea)
        rec={"at":hex(ea-IB),"insn":idc.generate_disasm_line(ea,0),
             "fn_rva":hex(fn.start_ea-IB) if fn else "?","fn_name":idc.get_func_name(fn.start_ea) if fn else "?"}
        # is it a write? (mov [flag], x)  vs read (cmp/movzx)
        is_write = (idc.get_operand_type(ea,0)==idc.o_mem) and mnem in ("mov","and","or","xor","inc","dec")
        if is_write:
            rec["decomp"]=dec(ea); out["writers"].append(rec)
        else:
            out["readers"].append(rec)
    log("writers",len(out["writers"]),"readers",len(out["readers"]))
    json.dump(out,open(A+r"\flag-dump.json","w"),indent=1,default=str)
    open(A+r"\flag_ok.txt","w").write("ok"); log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\flag_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
