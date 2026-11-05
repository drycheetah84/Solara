import idaapi, idc, ida_pro
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\relay2_out.txt", "w", encoding="utf-8")
ea = ib + 0x57105D0
end = ib + 0x57105D0 + 0x5F1
while ea < end:
    out.write("%X  %s\n" % (ea - ib, idc.generate_disasm_line(ea, 0)))
    ea = idc.next_head(ea)
out.close(); ida_pro.qexit(0)
