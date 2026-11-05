import ida_segment
# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\tsync5_out.txt", "w", encoding="utf-8")
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

import struct
pat = struct.pack("<Q", ib + 0x57105D0)
seg = ida_segment.get_segm_by_name(".rdata")
ea = seg.start_ea; hits = []
while True:
    ea = ida_bytes.find_bytes(pat, ea + 1, seg.end_ea)
    if ea == idc.BADADDR or ea is None or ea >= seg.end_ea: break
    hits.append(ea)
    if len(hits) > 6: break
out.write("submit-impl pointer hits: %s\n" % [hex(h - ib) for h in hits])
for h in hits:
    base = h - 1232
    t = ida_bytes.get_qword(base + 1216)
    out.write("=== vtable %X slot+1216 -> %X\n" % (base - ib, t - ib))
    if not ida_funcs.get_func(t): idc.create_insn(t)
    dec(t, "AvgWorst client impl", 9000)
    break
out.close(); ida_pro.qexit(0)
