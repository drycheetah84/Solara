import idaapi, idc, ida_pro, ida_frame, ida_funcs
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\relay3_out.txt", "w", encoding="utf-8")
for ea in [0x5710683, 0x5710697, 0x57106A2, 0x571073D]:
    a = ib + ea
    out.write("%X %s | op1 value=%s\n" % (ea, idc.generate_disasm_line(a, idc.GENDSM_FORCE_CODE), hex(idc.get_operand_value(a, 1))))
    for n in (0, 1):
        try:
            out.write("    op%d sp-rel: %s\n" % (n, hex(idc.get_spd(a))))
        except Exception as e: pass
out.close(); ida_pro.qexit(0)
