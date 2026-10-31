# datax.py -- describe data globals / in-function code sites in a SOURCE build so they can be found in
# 50451. Per address: IDA name, segment, string contents, the containing function (+ site bytes and
# instruction), every code reference (referencing function, site, mnemonic, ordinal of this ref among
# that function's data refs, the function's unique strings / size / normalized body), and if it looks
# like a vtable, its first vfunc pointers with their functions' strings/size.
# Usage: idat -A -S"datax.py <tag> <rvas.txt>" -L<log> <db.i64>      -> datax_<tag>.json
import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_bytes, ida_pro, json, re, os
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
TAG, LST = idc.ARGV[1], idc.ARGV[2]

def norm(s):
    if not s: return ""
    s = re.sub(r'sub_[0-9A-Fa-f]+', 'F', s)
    s = re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+', 'G', s)
    s = re.sub(r'0x[0-9A-Fa-f]+', 'N', s); s = re.sub(r'\b[0-9]+\b', 'N', s)
    s = re.sub(r'\bv[0-9]+\b', 'v', s); s = re.sub(r'\ba[0-9]+\b', 'a', s)
    return re.sub(r'\s+', '', s)

_fi = {}
def funcinfo(fstart):
    if fstart in _fi: return _fi[fstart]
    f = ida_funcs.get_func(fstart)
    if not f: return None
    strs, drefs, e = set(), [], f.start_ea
    while e < f.end_ea:
        for dr in idautils.DataRefsFrom(e):
            drefs.append((e - ib, dr - ib))
            t = idc.get_strlit_contents(dr, -1, idc.STRTYPE_C)
            if t and 4 <= len(t) <= 160: strs.add(t.decode("utf-8", "ignore"))
            else:
                t = idc.get_strlit_contents(dr, -1, idc.STRTYPE_C_16)
                if t and 4 <= len(t) <= 160: strs.add(t.decode("utf-16le", "ignore"))
        e = idc.next_head(e, f.end_ea)
    try: body = norm(str(ida_hexrays.decompile(f.start_ea)))
    except Exception: body = ""
    r = {"func": f.start_ea - ib, "end": f.end_ea - ib, "size": f.end_ea - f.start_ea,
         "name": idc.get_func_name(f.start_ea), "strings": sorted(strs), "drefs": drefs, "body": body}
    _fi[fstart] = r; return r

out = {}
for ln in open(LST):
    h = ln.strip()
    if not h: continue
    rva = int(h, 16); ea = ib + rva
    rec = {"seg": idc.get_segm_name(ea), "name": idc.get_name(ea),
           "demangled": idc.demangle_name(idc.get_name(ea), idc.get_inf_attr(idc.INF_SHORT_DN)) or ""}
    for st in (idc.STRTYPE_C, idc.STRTYPE_C_16):
        t = idc.get_strlit_contents(ea, -1, st)
        if t and len(t) >= 4:
            rec["string"] = t.decode("utf-16le" if st == idc.STRTYPE_C_16 else "utf-8", "ignore"); break
    f = ida_funcs.get_func(ea)
    if f:
        fi = funcinfo(f.start_ea)
        rec["in_func"] = {"func": fi["func"], "off": ea - f.start_ea, "size": fi["size"], "end": fi["end"],
                          "name": fi["name"], "strings": fi["strings"], "body": fi["body"],
                          "site_bytes": list(ida_bytes.get_bytes(ea, 16) or b""), "insn": idc.GetDisasm(ea)}
    refs = []
    for x in idautils.XrefsTo(ea, 0):
        if not ida_bytes.is_code(ida_bytes.get_full_flags(x.frm)): continue
        rf = ida_funcs.get_func(x.frm)
        if not rf: continue
        fi = funcinfo(rf.start_ea)
        ords = [i for i, (s, d) in enumerate(fi["drefs"]) if s == x.frm - ib and d == rva]
        refs.append({"func": fi["func"], "site": x.frm - ib, "mnem": idc.print_insn_mnem(x.frm),
                     "insn": idc.GetDisasm(x.frm), "ordinal": ords[0] if ords else None,
                     "ndrefs": len(fi["drefs"]), "fname": fi["name"], "fsize": fi["size"],
                     "strings": fi["strings"], "body": fi["body"]})
        if len(refs) >= 8: break
    rec["refs"] = refs
    if rec["seg"] in (".rdata", ".data"):           # vtable-ish: consecutive pointers into .text
        vf = []
        for i in range(6):
            p = ida_bytes.get_qword(ea + 8 * i)
            pf = ida_funcs.get_func(p) if p else None
            if not pf: break
            fi = funcinfo(pf.start_ea)
            vf.append({"func": fi["func"], "size": fi["size"], "name": fi["name"], "strings": fi["strings"][:8]})
        if vf: rec["vfuncs"] = vf
    out[h.upper()] = rec
json.dump(out, open(os.path.join(W, "datax_%s.json" % TAG), "w"))
print("datax", TAG, len(out))
ida_pro.qexit(0)
