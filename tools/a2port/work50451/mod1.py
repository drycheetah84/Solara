# mod1.py -- 50451 voice moderation: who mutes whom, where the mute lists come from, how the mute reaches Vivox.
# Run headless on the shard9 COPY (shard8 is the clean source). Safe iteration only (no tail-chunk walks).
import idaapi, idautils, ida_bytes, ida_funcs, ida_pro, ida_hexrays, idc
idaapi.auto_wait()
ib = idaapi.get_imagebase()
out = open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\mod1_out.txt", "w", encoding="utf-8")
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

NATIVES = ["MutePlayerForAll", "MuteAllPlayersForAll", "Server_MutePlayerForAll", "Server_MuteAllPlayersForAll",
           "Client_SetMutedHud", "Server_RegisterPawnForSettingsAndSafety", "UpdateServerOnlySettingsAndSafetyFromClient",
           "Server_ReadServerOnlySettingsAndSafetyPersistentData", "Server_WriteServerOnlySettingsAndSafetyPersistentData",
           "KickPlayerServer", "Server_KickPlayer", "Server_ReportPlayer", "ReportPlayer", "Server_WarnPlayer", "WarnPlayer",
           "BanPlayerFromFleet", "OnRep_ClientVersionOfServerOnlySettingsAndSafetyData"]
for n in NATIVES:
    for fn in native(n):
        f = dec(fn, "exec " + n, 4000)
        if not f: continue
        cs = callees(f.start_ea)
        out.write("CALLEES: %s\n" % [hex(c - ib) for c in cs])
        for c in cs:
            g = ida_funcs.get_func(c)
            if g and 0x80 <= g.end_ea - g.start_ea <= 0x3000:
                h = dec(c, "callee of " + n, 9000)
                if h:
                    for c2 in callees(h.start_ea):
                        g2 = ida_funcs.get_func(c2)
                        if g2 and 0x200 <= g2.end_ea - g2.start_ea <= 0x2000:
                            dec(c2, "callee2 of " + n, 6000)

# string anchors: functions that reference them
STRS = {0x8f1e068: "x-automation-key", 0x8f1e578: "automation", 0xa6481b8: "/v1/moderation",
        0x929072c: "muted", 0x9318b70: "Mute##MuteForAll", 0x91629b0: "ServerOnlyMuteForAllMothershipIDs",
        0x91629d8: "ServerOnlyMothershipMutedMothershipIDs", 0x8f205f8: "/v1/moderation/client/report",
        0x8f20890: "/v1/moderation/server/report"}
for rva, s in STRS.items():
    refs = list(idautils.DataRefsTo(ib + rva))
    out.write("\n==== xrefs to %s @%X: %s\n" % (s, rva, [hex(r - ib) for r in refs]))
    for r in refs[:6]:
        dec(r, "refs '%s'" % s, 7000)

# the voice-mute primitive used by the notifications Mute/Unmute handler, and who else calls it
VM = ib + 0x53DDA50
dec(VM, "voice mute primitive (notifications)")
out.write("\n==== callers of 53DDA50: %s\n" % [hex(ida_funcs.get_func(x).start_ea - ib) for x in idautils.CodeRefsTo(VM, 0) if ida_funcs.get_func(x)])
for x in list(idautils.CodeRefsTo(VM, 0))[:10]:
    dec(x, "calls voice mute 53DDA50", 7000)
out.close(); ida_pro.qexit(0)
