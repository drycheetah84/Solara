# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\tsync3_out.txt", "w", encoding="utf-8")
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

dec(ib + 0x56EE6B0, "gate sub_1456EE6B0", 6000)
dec(ib + 0x545B250, "AvgWorst sender", 4000)
f = ida_funcs.get_func(ib + 0x56F7230)
callers = sorted({ida_funcs.get_func(x).start_ea for x in idautils.CodeRefsTo(ib + 0x56F7230, 0) if ida_funcs.get_func(x)})
drefs = [hex(x - ib) for x in idautils.DataRefsTo(ib + 0x56F7230)]
out.write("\n=== 56F7230 code callers %s data refs (vtable slots) %s\n" % ([hex(c - ib) for c in callers], drefs))
for c in callers[:3]:
    dec(c, "caller of tick 56F7230", 8000)
# vtable of A2PawnNetInputComponent via the known Server_NewSubmitInputs impl 0x57105D0 at slot +1232
vrefs = [x for x in idautils.DataRefsTo(ib + 0x57105D0)]
out.write("\n=== data refs to submit impl 57105D0: %s\n" % [hex(x - ib) for x in vrefs])
for n in ["Client_ReportRollbackAvgWorstRemainingTime", "Client_ReportMissedInputs"]:
    for fn in native(n):
        dec(fn, "exec " + n, 3000)
for v in vrefs:
    base = v - 1232
    out.write("vtable base guess %X (slot+1232 = submit impl)\n" % (base - ib))
    for off in range(1180, 1300, 8):
        out.write("  +%d -> %X\n" % (off, ida_bytes.get_qword(base + off) - ib))
    # tick slot? where does 56F7230 appear in this vtable
    for off in range(0, 3000, 8):
        if ida_bytes.get_qword(base + off) == ib + 0x56F7230: out.write("  56F7230 is vtable slot +%d\n" % off)
out.close(); ida_pro.qexit(0)
