# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\feat1_out.txt", "w", encoding="utf-8")
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
for n in ["Server_GrantFeaturedCosmetic", "ServerRewardFeaturedCosmetic", "Multicast_RewardShower", "TestRewardShower"]:
    for fn in native(n):
        f = dec(fn, "exec " + n, 5000)
        if not f: continue
        for c in callees(f.start_ea):
            g = ida_funcs.get_func(c)
            if g and 0x100 <= g.end_ea - g.start_ea <= 0x4000:
                h = dec(c, "callee of " + n, 14000)
                if h:
                    for c2 in callees(h.start_ea):
                        g2 = ida_funcs.get_func(c2)
                        if g2 and 0x200 <= g2.end_ea - g2.start_ea <= 0x3000:
                            dec(c2, "callee2 of " + n, 9000)
# code that loads these strings (A2 module range)
T = {0x9314a50: "ServerApplyFeaturedTransaction: Not running on the server", 0x9201f20: "W:FeaturedTransactionDetails",
     0x9202058: "FeaturedTransactionId", 0x8f1ff78: "transaction_id"}
want = {ib + k: v for k, v in T.items()}
hits = {}
for lo, hi in [(0x4D00000, 0x4F00000), (0x5300000, 0x5A00000)]:
    for fs in idautils.Functions(ib + lo, ib + hi):
        for h in idautils.FuncItems(fs):
            insn = ida_ua.insn_t()
            if not ida_ua.decode_insn(insn, h): continue
            for op in insn.ops:
                if op.type == ida_ua.o_void: break
                v = op.addr if op.type in (ida_ua.o_mem, ida_ua.o_displ) else (op.value if op.type == ida_ua.o_imm else 0)
                if v in want: hits.setdefault(want[v], set()).add(fs)
for s, fns in hits.items():
    out.write("\n==== %s: %s\n" % (s, [hex(f - ib) for f in sorted(fns)]))
    for f in sorted(fns)[:3]:
        dec(f, "uses '%s'" % s, 14000)
out.close(); ida_pro.qexit(0)
