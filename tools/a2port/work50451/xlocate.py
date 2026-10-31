# xlocate.py -- general cross-build function locator, run inside the DESTINATION IDB.
# Per source target (graph_<srctag>.json: func/off/size/callers/callees/normalized body):
#   1) native   : UE registration-table pair by UFunction name (authoritative)
#   2) graph    : candidates = dest callees of mapped callers U dest callers of mapped callees,
#                 scored by body similarity; a caller with an unchanged call count votes for the exact slot
#   3) window   : body search around an interpolated estimate (dense unique-string basis), margin-checked
# Found functions are added to the neighbour map and the whole set is re-run until a fixpoint.
# Usage: idat -A -S"xlocate.py <srctag> <mode 22|47> <dst 47|50> <outtag>" -L<log> <dest.i64>
#   maps: strpairs_<mode>_<dst>.json, natpairs_<mode>_<dst>.json, optional xmap_<mode>_<dst>.json
#   out : xloc_<outtag>.jsonl
import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_pro, json, re, difflib, bisect, os, time
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SRC, MODE, DST, OUTT = idc.ARGV[1], idc.ARGV[2], idc.ARGV[3], idc.ARGV[4]
def J(n): return json.load(open(os.path.join(W, n)))
LOG = open(os.path.join(W, "xloc_%s.progress.log" % OUTT), "w", buffering=1)
def log(*a): LOG.write(time.strftime("%H:%M:%S ") + " ".join(str(x) for x in a) + "\n")
G = J("graph_%s.json" % SRC)

MAP = {}
for s, d in J("strpairs_%s_%s.json" % (MODE, DST)): MAP[s] = d
xp = os.path.join(W, "xmap_%s_%s.json" % (MODE, DST))
if os.path.exists(xp):
    for s, d in J(os.path.basename(xp)): MAP[s] = d
NAT = {}
for s, d, how in J("natpairs_%s_%s.json" % (MODE, DST)):
    if not how.startswith("exact-dup/exec"): MAP[s] = d; NAT[s] = d
log("src", SRC, "->", DST, "targets", len(G), "neighbour map", len(MAP), "native", len(NAT))

TOK = re.compile(r"[A-Za-z_][A-Za-z_0-9]*|[^\sA-Za-z_0-9]")
def norm(s):
    if not s: return ""
    s = re.sub(r'sub_[0-9A-Fa-f]+', 'F', s)
    s = re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+', 'G', s)
    s = re.sub(r'0x[0-9A-Fa-f]+', 'N', s); s = re.sub(r'\b[0-9]+\b', 'N', s)
    s = re.sub(r'\bv[0-9]+\b', 'v', s); s = re.sub(r'\ba[0-9]+\b', 'a', s)
    return re.sub(r'\s+', '', s)
_bc, _cc = {}, {}
def body(fs):
    if fs not in _bc:
        try: _bc[fs] = TOK.findall(norm(str(ida_hexrays.decompile(ib + fs))))[:4000]
        except Exception: _bc[fs] = []
    return _bc[fs]
def calls_of(fs):
    if fs in _cc: return _cc[fs]
    f = ida_funcs.get_func(ib + fs); out = []
    if f:
        e = f.start_ea
        while e < f.end_ea:
            if idc.print_insn_mnem(e) in ("call", "jmp"):
                t = idc.get_operand_value(e, 0); tf = ida_funcs.get_func(t) if t else None
                if tf and tf.start_ea == t: out.append(t - ib)
            e = idc.next_head(e, f.end_ea)
    _cc[fs] = out; return out
def callers_of(fs, cap=200):
    out = set()
    for x in idautils.CodeRefsTo(ib + fs, 0):
        cf = ida_funcs.get_func(x)
        if cf: out.add(cf.start_ea - ib)
        if len(out) > cap: return None
    return out
def fsize(fs):
    f = ida_funcs.get_func(ib + fs)
    return (f.end_ea - f.start_ea) if f and f.start_ea == ib + fs else None
def ratio(a, b):
    sm = difflib.SequenceMatcher(None, a, b, autojunk=False)
    if sm.real_quick_ratio() < 0.5 or sm.quick_ratio() < 0.5: return 0.0
    return sm.ratio()

def graph_try(g, bsrc):
    votes, strong = {}, {}
    for c in g["callers"]:
        c50 = MAP.get(c["func"])
        if c50 is None: continue
        cl = calls_of(c50)
        if len(cl) == c["ncalls"]:
            for o in c["ord"]:
                if o < len(cl): strong.setdefault(cl[o], set()).add(c["func"])
        for t in set(cl): votes.setdefault(t, set()).add(("c", c["func"]))
    for e in g["callees"]:
        e50 = MAP.get(e)
        if e50 is None: continue
        cs = callers_of(e50)
        if cs is None: continue
        for t in cs: votes.setdefault(t, set()).add(("e", e))
    sc = []
    for cand in set(strong) | set(votes):
        csz = fsize(cand)
        if csz is None or abs(csz - g["size"]) > max(0x80, g["size"] // 2): continue
        b = body(cand)
        if not b: continue
        r = ratio(bsrc, b)
        if r > 0: sc.append((r, len(votes.get(cand, ())), len(strong.get(cand, ())), cand))
    sc.sort(key=lambda x: x[0] + 0.02 * min(x[1], 5) + 0.05 * min(x[2], 3), reverse=True)
    if not sc: return None
    r, nv, ns, cand = sc[0]; r2 = sc[1][0] if len(sc) > 1 else 0.0
    ok = (r >= 0.85 and r - r2 >= 0.03) or (nv >= 2 and r >= 0.60 and (len(sc) == 1 or r - r2 >= 0.05)) \
         or (ns >= 1 and r >= 0.50 and (len(sc) == 1 or r - r2 >= 0.05))
    return {"cand": cand, "ratio": round(r, 3), "second": round(r2, 3), "votes": nv, "strong": ns, "ok": ok}

def window_try(g, bsrc, xs, pairs):
    if not pairs: return None
    f = g["func"]; i = bisect.bisect_left(xs, f)
    if i <= 0: est = f + (pairs[0][1] - pairs[0][0])
    elif i >= len(pairs): est = f + (pairs[-1][1] - pairs[-1][0])
    else:
        lo, hi = pairs[i - 1], pairs[i]
        t = (f - lo[0]) / (hi[0] - lo[0]) if hi[0] != lo[0] else 0
        est = f + int((lo[1] - lo[0]) + t * ((hi[1] - hi[0]) - (lo[1] - lo[0])))
    sz = g["size"]; tol = max(0x40, sz // 8); cands = []
    for fea in idautils.Functions(max(ib, ib + est - 0x20000), ib + est + 0x20000):
        csz = fsize(fea - ib)
        if csz is not None and abs(csz - sz) <= tol: cands.append(fea - ib)
    cands.sort(key=lambda q: abs(q - est))
    sc = []
    for cnd in cands[:16]:
        b = body(cnd)
        if b:
            r = ratio(bsrc, b)
            if r > 0: sc.append((r, cnd))
    sc.sort(reverse=True)
    if not sc: return {"est": est, "cand": None}
    r, c = sc[0]; r2 = sc[1][0] if len(sc) > 1 else 0.0
    twins = [x for y, x in sc if r - y < 0.005]
    return {"est": est, "cand": c, "ratio": round(r, 3), "second": round(r2, 3),
            "ok": r >= 0.90 and r - r2 >= 0.03, "twins": twins if r >= 0.97 and len(twins) > 1 else None}

final = {}
for rnd in range(1, 7):
    pairs = sorted(MAP.items()); xs = [p[0] for p in pairs]
    new = 0
    for h, g in sorted(G.items()):
        if final.get(h, {}).get("status") in ("native", "graph", "window", "both"): continue
        rec = {"src": h, "round": rnd}
        if g.get("func") is None or not g.get("body"):
            rec["status"] = "no-src"; final[h] = rec; continue
        if g["func"] in NAT:
            d = NAT[g["func"]]
            rec.update(status="native", func=hex(d), rva50=hex(d + g["off"]))
        else:
            bsrc = TOK.findall(g["body"])[:4000]
            gr = graph_try(g, bsrc); wi = window_try(g, bsrc, xs, pairs)
            rec["graph"] = gr; rec["window"] = wi
            gok = gr and gr["ok"]; wok = wi and wi.get("ok")
            if gok and wok and gr["cand"] == wi["cand"]: rec["status"] = "both"; d = gr["cand"]
            elif gok and wok: rec["status"] = "CONFLICT"; d = None
            elif gok: rec["status"] = "graph"; d = gr["cand"]
            elif wok: rec["status"] = "window"; d = wi["cand"]
            elif wi and wi.get("twins") and gr and gr["cand"] in wi["twins"]:
                rec["status"] = "graph"; d = gr["cand"]                      # graph breaks a twin tie
            else: rec["status"] = "none"; d = None
            if d is not None: rec.update(func=hex(d), rva50=hex(d + g["off"]))
        if rec["status"] in ("native", "graph", "window", "both"):
            new += 1; MAP[g["func"]] = int(rec["func"], 16)
        final[h] = rec
    log("round", rnd, "new", new, "resolved", sum(1 for r in final.values() if r["status"] in ("native", "graph", "window", "both")), "/", len(G))
    if not new: break
with open(os.path.join(W, "xloc_%s.jsonl" % OUTT), "w") as fo:
    for h, rec in sorted(final.items()): fo.write(json.dumps(rec) + "\n")
log("DONE")
ida_pro.qexit(0)
