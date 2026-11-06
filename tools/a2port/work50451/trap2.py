# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\trap2_out.txt", "w", encoding="utf-8")
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

for a, nm in [(0x55A2500, "BreakTrap impl"), (0x55C6DB0, "ResetTrap impl"), (0x55CCF90, "SetTrapActive impl"), (0x55D2250, "ToggleActorFading impl"), (0x55BCA70, "OnRep_Fading impl")]:
    if not ida_funcs.get_func(ib + a): idc.create_insn(ib + a)
    h = dec(ib + a, nm, 9000)
    if h:
        callees = []
        for ea in idautils.FuncItems(h.start_ea):
            for x in idautils.CodeRefsFrom(ea, 0):
                f = ida_funcs.get_func(x)
                if f and f.start_ea == x and x != h.start_ea and x not in callees: callees.append(x)
        out.write("CALLEES %s: %s\n" % (nm, [hex(c - ib) for c in callees[:20]]))
# who marks bFading dirty (call sites of 55AD5C0 via code refs incl. jumps)
refs = [x for x in idautils.CodeRefsTo(ib + 0x55AD5C0, 1)]
out.write("refs to 55AD5C0: %s\n" % [hex(r - ib) for r in refs[:10]])
out.close(); ida_pro.qexit(0)
