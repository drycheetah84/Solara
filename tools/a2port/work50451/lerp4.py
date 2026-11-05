import idaapi, idc, ida_pro
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\lerp4_out.txt", "w", encoding="utf-8")
ea = ib + 0x57A1FF0
for i in range(70):
    idc.create_insn(ea)
    out.write("%X  %s\n" % (ea - ib, idc.generate_disasm_line(ea, 0)))
    if idc.print_insn_mnem(ea) in ("retn", "ret") and i > 3: break
    ea = idc.next_head(ea)
out.close(); ida_pro.qexit(0)
