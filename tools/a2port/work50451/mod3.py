# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\mod3_out.txt", "w", encoding="utf-8")
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
for rva, why in [(0x58615C0, "OnRep settings&safety FULL"), (0x588B890, "timer cb from 5887BE0 (try-mute loop?)"),
                 (0x5845E90, "ApplyMicSettings-ish FULL"), (0x553E070, "entity -> voice comp")]:
    dec(ib + rva, why, 20000)
# who touches pawn+0x1A64 (forced mic mute) and voice comp +244, in the A2 module code range
hits = {}
for fs in idautils.Functions(ib + 0x5300000, ib + 0x5A00000):
    for h in idautils.FuncItems(fs):
        insn = ida_ua.insn_t()
        if not ida_ua.decode_insn(insn, h): continue
        for op in insn.ops:
            if op.type == ida_ua.o_void: break
            if op.type in (ida_ua.o_displ, ida_ua.o_phrase) and op.addr in (0x1A64,):
                hits.setdefault(fs, []).append("%X %s" % (h - ib, idc.generate_disasm_line(h, 0)))
out.write("\n==== +0x1A64 users: %d\n" % len(hits))
for fs, l in sorted(hits.items()):
    out.write("%X: %s\n" % (fs - ib, " | ".join(l[:4])))
for fs in sorted(hits)[:14]:
    dec(fs, "uses pawn+0x1A64", 5000)
# Client_SetMutedHud impl: exec 5546D80 calls this->vtbl+4032. Find vtables whose slot 504 is a function that
# writes +0x1A64, else just list candidates: data refs to the exec are in the UFunction table, not vtables.
out.close(); ida_pro.qexit(0)
