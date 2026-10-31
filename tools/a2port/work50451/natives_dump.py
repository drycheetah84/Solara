# natives_dump.py -- UE native-function registration tables: .rdata arrays of {const char* Name, FNativeFuncPtr}
# (FNameNativePtrPair, one array per UClass, consumed by FNativeFunctionRegistrar::RegisterFunctions).
# Each entry pairs a UFunction name with its exec thunk; the array's name sequence identifies the class
# across builds. Usage: idat -A -S"natives_dump.py <tag>" -L<log> <db.i64>  -> natives_<tag>.json
import idaapi, idc, ida_funcs, ida_bytes, ida_segment, ida_pro, json, os, struct, re
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
TAG = idc.ARGV[1]
segs = {}
for n in (".text", ".rdata", ".data"):
    s = ida_segment.get_segm_by_name(n)
    if s: segs[n] = (s.start_ea, s.end_ea)
tlo, thi = segs[".text"]; rlo, rhi = segs[".rdata"]
IDENT = re.compile(rb"^[A-Za-z_][A-Za-z0-9_]{0,120}$")
_sc = {}
def cstr(p):
    if p in _sc: return _sc[p]
    b = ida_bytes.get_bytes(p, 128) or b""
    z = b.find(b"\0"); s = b[:z] if z > 0 else b""
    r = s.decode() if s and IDENT.match(s) else None
    _sc[p] = r; return r
data = ida_bytes.get_bytes(rlo, rhi - rlo)
arrays, cur, prev = [], [], None
for off in range(0, len(data) - 16, 8):
    p0, p1 = struct.unpack_from("<QQ", data, off)
    ok = False
    if rlo <= p0 < rhi and tlo <= p1 < thi:
        f = ida_funcs.get_func(p1)
        if f and f.start_ea == p1:
            nm = cstr(p0)
            if nm:
                ok = True
                if prev is not None and off - prev == 16: cur.append([nm, p1 - ib])
                else:
                    if len(cur) >= 2: arrays.append({"at": rlo + prev - ib - 16 * (len(cur) - 1), "e": cur})
                    cur = [[nm, p1 - ib]]
                prev = off
    if not ok and prev is not None and off - prev > 16:
        if len(cur) >= 2: arrays.append({"at": rlo + prev - ib - 16 * (len(cur) - 1), "e": cur})
        cur, prev = [], None
if len(cur) >= 2: arrays.append({"at": rlo + prev - ib - 16 * (len(cur) - 1), "e": cur})
def calls_of(fs):
    f = ida_funcs.get_func(ib + fs); out = []
    if not f: return out
    e = f.start_ea
    while e < f.end_ea:
        if idc.print_insn_mnem(e) in ("call", "jmp"):
            t = idc.get_operand_value(e, 0); tf = ida_funcs.get_func(t) if t else None
            if tf and tf.start_ea == t: out.append(t - ib)
        e = idc.next_head(e, f.end_ea)
    return out
for a in arrays:
    for ent in a["e"]: ent.append(calls_of(ent[1]))
json.dump(arrays, open(os.path.join(W, "natives_%s.json" % TAG), "w"))
print("natives", TAG, "arrays", len(arrays), "entries", sum(len(a["e"]) for a in arrays))
ida_pro.qexit(0)
