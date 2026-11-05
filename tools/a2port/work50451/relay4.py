import idaapi, ida_bytes, ida_pro, struct
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\relay4_out.txt", "w", encoding="utf-8")
for rva in (0xA55D800, 0xA55D810, 0x8045450):
    b = ida_bytes.get_bytes(ib + rva, 16)
    out.write("%X: %s\n" % (rva, struct.unpack('<2d', b)))
out.close(); ida_pro.qexit(0)
