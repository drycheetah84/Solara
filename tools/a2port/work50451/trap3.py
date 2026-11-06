# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\trap3_out.txt", "w", encoding="utf-8")
done = set()
SKIP = {ib + 0x4DCB930}   # known decompiler crasher

def dec(ea, why, maxlen=9000):
    f = ida_funcs.get_func(ea)
    if not f or f.start_ea in done or f.start_ea in SKIP: return None
    done.add(f.start_ea)
    out.write("\n\n######## %X size %X [%s]\n" % (f.start_ea - ib, f.end_ea - f.start_ea, why)); out.flush()
    if f.end_ea - f.start_ea <= 0x5000:
        try:
            c = ida_hexrays.decompile(f.start_ea)
            out.write((str(c) if c else "(none)")[:maxlen] + "\n")
        except Exception as e:
            out.write("(decompile failed: %s)\n" % e)
    out.flush()
    return f

def callees(fs):
    r = []
    for h in idautils.FuncItems(fs):
        if idc.print_insn_mnem(h) == "call":
            g = ida_funcs.get_func(idc.get_operand_value(h, 0))
            if g and g.start_ea not in r: r.append(g.start_ea)
    return r

def native(name):
    pat = " ".join("%02X" % b for b in name.encode() + b"\x00")
    ea = ib; res = []
    while True:
        ea = ida_bytes.find_bytes(pat, ea)
        if ea in (None, idaapi.BADADDR): break
        if ida_bytes.get_byte(ea - 1) == 0:
            for x in idautils.DataRefsTo(ea):
                fn = ida_bytes.get_qword(x + 8)
                if ida_funcs.get_func(fn): res.append(fn)
        ea += 1
    return res

import ida_ua
for a, nm in [(0x55CC580, "restore (unfade)"), (0x5504460, "after-fade")]:
    if not ida_funcs.get_func(ib + a): idc.create_insn(ib + a)
    dec(ib + a, nm, 9000)
# writers of byte [reg+2C0h] (704) inside functions that also reference the ScraprunActor vtable/OnRep or 712: scan functions near the class code
hits = []
for seg_ea in [ib + 0x5480000]:
    pass
start, end = ib + 0x5490000, ib + 0x5600000
ea = start
while ea < end:
    if idc.print_insn_mnem(ea) == "mov":
        op = idc.print_operand(ea, 0)
        if "2C0h]" in op and "byte" in op:
            f = ida_funcs.get_func(ea)
            hits.append((ea, f.start_ea if f else 0, idc.generate_disasm_line(ea, 0)))
    ea = idc.next_head(ea, end)
    if ea == idc.BADADDR: break
out.write("\n=== byte writes to +2C0 in 0x5490000..0x5600000: %d\n" % len(hits))
for ea, fs, l in hits[:40]:
    out.write("%X (func %X): %s\n" % (ea - ib, fs - ib if fs else 0, l))
out.close(); ida_pro.qexit(0)
