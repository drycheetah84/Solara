# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\ban2_out.txt", "w", encoding="utf-8")
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

STRS = {0x8f20fb0: "malformed no ClientMessage (error parser)", 0x8f20f10: "no message or statusCode", 0x8f222e0: "Authentication Failed: %s. Errror Code",
        0x90de128: "Complete Meta Auth Failed", 0x90dd1e8: "Mothership auth failed %s", 0x9347510: "HandleMothershipLoginFlowComplete login failed",
        0x8f24c30: "Mothership login failed for user", 0x9310990: "Platform Auth Failed entitled"}
for rva, s in STRS.items():
    refs = list(idautils.DataRefsTo(ib + rva))
    out.write("\n==== xrefs to %s @%X: %s\n" % (s, rva, [hex(r - ib) for r in refs]))
    for r in refs[:4]:
        f = dec(r, "refs '%s'" % s, 12000)
        if f:
            callers = sorted({ida_funcs.get_func(x).start_ea for x in idautils.CodeRefsTo(f.start_ea, 0) if ida_funcs.get_func(x)})
            out.write("CALLERS of %X: %s\n" % (f.start_ea - ib, [hex(c - ib) for c in callers]))
            for c in callers[:2]:
                dec(c, "caller of fn using '%s'" % s, 8000)
out.close(); ida_pro.qexit(0)
