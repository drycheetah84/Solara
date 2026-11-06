# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\trap1_out.txt", "w", encoding="utf-8")
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

for n in ["OnRep_Fading", "toggleActorFading", "ToggleActorFading", "BreakTrap", "ResetTrap", "SetTrapActive"]:
    for fn in native(n):
        dec(fn, "exec " + n, 4000)
for a in (0x55AD5E9, 0x5504787):
    f = ida_funcs.get_func(ib + a)
    if not f: idc.create_insn(ib + a)
    h = dec(ib + a, "uses bFading name @%X" % a, 9000)
    if h:
        callers = sorted({ida_funcs.get_func(x).start_ea for x in idautils.CodeRefsTo(h.start_ea, 0) if ida_funcs.get_func(x)})
        out.write("CALLERS %X: %s\n" % (h.start_ea - ib, [hex(c - ib) for c in callers[:8]]))
for s in (0x92A5AE0, 0x92A5B08, 0x92A27A8):
    refs = [x for x in idautils.DataRefsTo(ib + s)]
    out.write("data refs to str %X: %s\n" % (s, [hex(r - ib) for r in refs[:8]]))
out.close(); ida_pro.qexit(0)
