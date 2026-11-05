# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\lerp2_out.txt", "w", encoding="utf-8")
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

import ida_ua, ida_bytes
ea = ib + 0x578E8C0
out.write("=== disasm around the Rollback Replay checkboxes\n")
for i in range(40):
    idc.create_insn(ea)
    out.write("%X  %s\n" % (ea - ib, idc.generate_disasm_line(ea, 0)))
    ea = idc.next_head(ea)
# globals loaded in that window (o_mem operands): value + who else references them
mems = set()
ea = ib + 0x578E8C0
for i in range(40):
    insn = ida_ua.insn_t()
    if ida_ua.decode_insn(insn, ea):
        for op in insn.ops:
            if op.type == ida_ua.o_void: break
            if op.type == ida_ua.o_mem and not (ib + 0x8F00000 <= op.addr <= ib + 0x9500000): mems.add(op.addr)
    ea = idc.next_head(ea)
for g in sorted(mems):
    out.write("\n=== global %X byte=%d dword=%d\n" % (g - ib, ida_bytes.get_byte(g), ida_bytes.get_dword(g)))
    refs = [x for x in idautils.DataRefsTo(g)]
    out.write("refs: %s\n" % [hex(x - ib) for x in refs[:20]])
out.close(); ida_pro.qexit(0)
