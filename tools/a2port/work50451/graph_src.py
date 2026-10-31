# graph_src.py -- call-graph neighbourhood of each target in a SOURCE build (for graph propagation).
# Per target: its callers (function starts + which call ordinal in that caller targets us) and its
# callees (ordered), plus size and normalized body. Usage:
#   idat -A -S"graph_src.py <tag> <targets.txt>" -L<log> <db.i64>   -> graph_<tag>.json
import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_bytes, ida_pro, json, re, os
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
TAG, LST = idc.ARGV[1], idc.ARGV[2]

def calls_of(f):
    out, e = [], f.start_ea
    while e < f.end_ea:
        if idc.print_insn_mnem(e) in ("call", "jmp"):
            t = idc.get_operand_value(e, 0)
            tf = ida_funcs.get_func(t) if t else None
            if tf and tf.start_ea == t: out.append(t - ib)          # direct call/tail-jump to a function start
        e = idc.next_head(e, f.end_ea)
    return out
def norm(s):
    if not s: return ""
    s = re.sub(r'sub_[0-9A-Fa-f]+', 'F', s)
    s = re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+', 'G', s)
    s = re.sub(r'0x[0-9A-Fa-f]+', 'N', s); s = re.sub(r'\b[0-9]+\b', 'N', s)
    s = re.sub(r'\bv[0-9]+\b', 'v', s); s = re.sub(r'\ba[0-9]+\b', 'a', s)
    return re.sub(r'\s+', '', s)

out = {}
for ln in open(LST):
    h = ln.strip()
    if not h: continue
    ea = ib + int(h, 16); f = ida_funcs.get_func(ea)
    if not f: out[h] = {"func": None}; continue
    callers = []
    for x in idautils.CodeRefsTo(f.start_ea, 0):
        cf = ida_funcs.get_func(x)
        if not cf: continue
        cl = calls_of(cf)
        callers.append({"func": cf.start_ea - ib, "ord": [i for i, c in enumerate(cl) if c == f.start_ea - ib], "ncalls": len(cl)})
        if len(callers) >= 24: break
    try: body = norm(str(ida_hexrays.decompile(f.start_ea)))
    except Exception: body = ""
    out[h] = {"func": f.start_ea - ib, "off": ea - f.start_ea, "size": f.end_ea - f.start_ea,
              "callers": callers, "callees": calls_of(f)[:60], "body": body}
json.dump(out, open(os.path.join(W, "graph_%s.json" % TAG), "w"))
print("graph", TAG, len(out))
ida_pro.qexit(0)
