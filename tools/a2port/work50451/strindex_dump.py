# strindex_dump.py -- dump an IDB's string->referencing-function index (for dense cross-build address
# correspondence), and optionally anchors + normalized decompiled bodies for a list of target RVAs.
# Usage: idat -A -S"strindex_dump.py <tag> [targets.txt]" -L<log> <db.i64>
#   writes strindex_<tag>.json  {string: [func_rva,...]}
#          anchors_<tag>.json   {rva: {func, off, size, strings, callees}}      (if targets given)
#          bodies_<tag>.json    {rva: {norm, len}}                              (if targets given)
import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_pro, json, re, os
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
TAG = idc.ARGV[1] if len(idc.ARGV) > 1 else "db"
TGT = idc.ARGV[2] if len(idc.ARGV) > 2 else None

sidx = {}
for s in idautils.Strings():
    try: val = str(s)
    except Exception: continue
    if not (5 <= len(val) <= 160): continue
    fs = set()
    for x in idautils.XrefsTo(s.ea, 0):
        f = ida_funcs.get_func(x.frm)
        if f: fs.add(f.start_ea - ib)
    if fs: sidx.setdefault(val, set()).update(fs)
json.dump({k: sorted(v) for k, v in sidx.items()}, open(os.path.join(W, "strindex_%s.json" % TAG), "w"))
print("strindex", TAG, len(sidx))

def norm(s):
    if not s: return ""
    s = re.sub(r'sub_[0-9A-Fa-f]+', 'F', s)
    s = re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+', 'G', s)
    s = re.sub(r'0x[0-9A-Fa-f]+', 'N', s)
    s = re.sub(r'\b[0-9]+\b', 'N', s)
    s = re.sub(r'\bv[0-9]+\b', 'v', s); s = re.sub(r'\ba[0-9]+\b', 'a', s)
    return re.sub(r'\s+', '', s)

if TGT:
    targets = [int(l.strip(), 16) for l in open(TGT) if l.strip()]
    anchors, bodies = {}, {}
    for rva in targets:
        key = "%X" % rva
        f = ida_funcs.get_func(ib + rva)
        if not f:
            anchors[key] = {"func": None, "seg": idc.get_segm_name(ib + rva)}
            continue
        strs, cs, e = set(), [], f.start_ea
        while e < f.end_ea:
            for dr in idautils.DataRefsFrom(e):
                t = idc.get_strlit_contents(dr, -1, idc.STRTYPE_C)
                if t and 4 <= len(t) <= 160: strs.add(t.decode("utf-8", "ignore"))
            if idc.print_insn_mnem(e) == "call":
                tf = ida_funcs.get_func(idc.get_operand_value(e, 0))
                if tf and (tf.start_ea - ib) not in cs: cs.append(tf.start_ea - ib)
            e = idc.next_head(e, f.end_ea)
        anchors[key] = {"func": f.start_ea - ib, "off": ib + rva - f.start_ea, "size": f.end_ea - f.start_ea,
                        "strings": sorted(strs), "callees": cs[:40]}
        try: body = str(ida_hexrays.decompile(f.start_ea))
        except Exception: body = ""
        bodies[key] = {"norm": norm(body), "len": len(body)}
    json.dump(anchors, open(os.path.join(W, "anchors_%s.json" % TAG), "w"))
    json.dump(bodies, open(os.path.join(W, "bodies_%s.json" % TAG), "w"))
    print("anchors", TAG, len(anchors), "funcs", sum(1 for a in anchors.values() if a["func"] is not None))
ida_pro.qexit(0)
