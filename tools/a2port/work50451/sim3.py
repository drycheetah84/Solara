# sim3.py -- how AA2RollbackManager consumes player inputs (is a 51-frame NewInputBuffer a backlog?), what the
# per-sim pre-step / authority report do, and who writes the timescale (+0x344). Safe iteration (Functions()).
import idaapi, idautils, ida_funcs, idc, ida_hexrays
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\sim3_out.txt", "w", encoding="utf-8")
done = set()

def dec(ea, why, maxlen=14000):
    f = ida_funcs.get_func(ea)
    if not f or f.start_ea in done or f.start_ea == ib + 0x4DCB930: return None
    done.add(f.start_ea)
    out.write("\n\n######## %X size %X [%s]\n" % (f.start_ea - ib, f.end_ea - f.start_ea, why)); out.flush()
    if f.end_ea - f.start_ea <= 0x4000:
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

# 1) Server_NewSubmitInputs implementation and one level of callees
f = dec(ib + 0x57105D0, "Server_NewSubmitInputs impl")
if f:
    cs = callees(f.start_ea)
    out.write("\nCALLEES of 57105D0: %s\n" % [hex(c - ib) for c in cs])
    for c in cs:
        g = ida_funcs.get_func(c)
        if g and 0x40 <= g.end_ea - g.start_ea <= 0x2000:
            dec(c, "callee of submit", 8000)

# 2) consumers
for rva, why in ((0x5757260, "per-sim input gather (called from Step per sim)"),
                 (0x5791BF0, "per-sim pre-step (frame, dt, acc, N, authority)"),
                 (0x571A6E0, "authority per-sim report (dropped, dtms, hitch)"),
                 (0x57229D0, "input apply"),
                 (0x5893140, "input build A"),
                 (0x5893600, "input build B")):
    dec(ib + rva, why)

# 3) writes to +0x344 (timescale) / +0x338 (accumulator) inside the rollback band, safe iteration
out.write("\n\n=== writes to +0x344/+0x338 in 0x5700000..0x57B0000\n")
for fs in idautils.Functions(ib + 0x5700000, ib + 0x57B0000):
    lst = []
    for h in idautils.FuncItems(fs):
        if idc.get_operand_type(h, 0) == idc.o_displ and idc.get_operand_value(h, 0) in (0x344, 0x338):
            if idc.print_insn_mnem(h).startswith(("mov", "add", "sub", "cvt")):
                lst.append("%X %s" % (h - ib, idc.GetDisasm(h)))
    if lst:
        out.write("func %X:\n  %s\n" % (fs - ib, "\n  ".join(lst[:8])))
out.write("\n=== done\n")
out.close()
idaapi.qexit(0)
