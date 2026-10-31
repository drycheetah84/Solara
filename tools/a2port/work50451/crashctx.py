# crashctx.py -- decompile the functions containing the given RVAs (crash rip + stack frames). Headless.
# Usage: idat -A -S"crashctx.py <out.txt> rva1 rva2 ..." -L<log> <idb>
import idaapi, idc, ida_funcs, ida_hexrays, ida_pro
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(idc.ARGV[1], "w", encoding="utf-8")
for a in idc.ARGV[2:]:
    ea = ib + int(a, 16)
    f = ida_funcs.get_func(ea)
    out.write("===== RVA %s  func=%s  name=%s\n" % (a, hex(f.start_ea - ib) if f else None, idc.get_func_name(ea)))
    out.write("  insn: %s\n" % idc.generate_disasm_line(ea, 0))
    if f:
        try:
            txt = str(ida_hexrays.decompile(f.start_ea))
            out.write(txt[:6000] + "\n")
        except Exception as e:
            out.write("  decompile failed: %s\n" % e)
out.close()
ida_pro.qexit(0)
