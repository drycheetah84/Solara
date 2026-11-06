# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\trap6_out.txt", "w", encoding="utf-8")
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

for g in (0xACF4B10, 0xACF4B18):
    for r in idautils.DataRefsTo(ib + g):
        f = ida_funcs.get_func(r)
        # look back a few instructions for a lea of a string
        ea = r
        for i in range(6):
            ea = idc.prev_head(ea)
            l = idc.generate_disasm_line(ea, 0)
            if "lea" in l and ";" in l:
                out.write("FName %X <- %s\n" % (g, l))
callers = []
for x in idautils.CodeRefsTo(ib + 0x55CC580, 0):
    f = ida_funcs.get_func(x)
    callers.append((x, f.start_ea if f else 0))
out.write("callers of show/hide 55CC580: %s\n" % [(hex(a - ib), hex(b - ib)) for a, b in callers])
for a, fs in callers:
    if fs and fs != ib + 0x55BCA70:
        h = dec(fs, "caller of show/hide", 5000)
        if h:
            out.write("  data refs (UFunction exec?) to %X: %s\n" % (fs - ib, [hex(x - ib) for x in idautils.DataRefsTo(fs)][:4]))
out.close(); ida_pro.qexit(0)
