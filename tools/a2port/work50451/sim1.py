# sim1.py -- rollback ball sim timing: step driver 0x560C2B0 (A2GameState), Step 0x57339E0, BeginPlay 0x5726240,
# the driver's callers/callees, and every float/double constant that looks like a sim timestep (1/90, 1/72, 90, 72...).
import idaapi, idautils, ida_funcs, ida_bytes, idc, ida_hexrays, struct
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\sim1_out.txt", "w", encoding="utf-8")
done = set()
SKIP = {ib + 0x4DCB930}

def strs(fs):
    r = []
    for h in idautils.FuncItems(fs):
        for i in range(3):
            v = idc.get_operand_value(h, i)
            if not v or v < ib: continue
            for t in (idc.STRTYPE_C, idc.STRTYPE_C_16):
                s = idc.get_strlit_contents(v, -1, t)
                if s and len(s) >= 2 and all(32 <= c < 127 for c in s[:40]):
                    x = s.decode('ascii', 'replace')[:80]
                    if x not in r: r.append(x)
                    break
    return r

def consts(fs):
    # float/double literals loaded from .rdata by this function
    r = []
    for h in idautils.FuncItems(fs):
        m = idc.print_insn_mnem(h)
        if not m.startswith(("movss", "movsd", "mulss", "mulsd", "divss", "divsd", "addss", "addsd", "subss", "subsd", "comiss", "comisd", "ucomiss", "ucomisd", "minss", "maxss", "cvtss", "movaps", "movups")):
            continue
        for i in range(2):
            if idc.get_operand_type(h, i) != idc.o_mem: continue
            v = idc.get_operand_value(h, i)
            f4 = struct.unpack("<f", ida_bytes.get_bytes(v, 4) or b"\0\0\0\0")[0]
            f8 = struct.unpack("<d", ida_bytes.get_bytes(v, 8) or b"\0" * 8)[0]
            r.append("%X:%s f=%g d=%g" % (h - ib, m, f4, f8))
    return r

def dec(ea, why, maxlen=16000):
    f = ida_funcs.get_func(ea)
    if not f or f.start_ea in done or f.start_ea in SKIP: return None
    done.add(f.start_ea)
    out.write("\n\n######## %X size %X [%s]\nSTRS %s\nCONSTS %s\n" % (f.start_ea - ib, f.end_ea - f.start_ea, why, strs(f.start_ea), consts(f.start_ea)[:40]))
    out.flush()
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
            t = idc.get_operand_value(h, 0)
            g = ida_funcs.get_func(t)
            if g and g.start_ea not in r: r.append(g.start_ea)
    return r

def callers(ea):
    r = []
    for x in idautils.CodeRefsTo(ea, 0):
        g = ida_funcs.get_func(x)
        if g and g.start_ea not in r: r.append(g.start_ea)
    return r

for rva, why in ((0x560C2B0, "step driver (A2GameState)"), (0x57339E0, "AA2RollbackManager Step"), (0x5726240, "AA2RollbackManager BeginPlay")):
    f = dec(ib + rva, why)
    out.write("\nCALLERS of %X: %s\n" % (rva, [hex(c - ib) for c in callers(ib + rva)]))
    if f:
        cs = callees(f.start_ea)
        out.write("CALLEES of %X: %s\n" % (rva, [hex(c - ib) for c in cs]))

# one level down from the step driver and from Step (skip tiny helpers and huge ones)
for rva in (0x560C2B0, 0x57339E0):
    f = ida_funcs.get_func(ib + rva)
    if not f: continue
    for c in callees(f.start_ea):
        g = ida_funcs.get_func(c)
        if g and 0x40 <= g.end_ea - g.start_ea <= 0x2500:
            dec(c, "callee of %X" % rva, 9000)

# callers of the step driver, one level up (who ticks it, with what delta)
for c in callers(ib + 0x560C2B0):
    dec(c, "caller of step driver", 12000)

# timestep-like constants anywhere in .rdata near the rollback code: list xrefs to them inside 0x5700000-0x5760000
want = {}
for seg in idautils.Segments():
    if idc.get_segm_name(seg) not in (".rdata",): continue
    s, e = idc.get_segm_start(seg), idc.get_segm_end(seg)
    for v in (1.0/90, 1.0/72, 1.0/60, 1.0/120, 90.0, 72.0, 60.0, 120.0):
        for ea in (s,):
            pass
out.write("\n\n=== done\n")
out.close()
idaapi.qexit(0)
