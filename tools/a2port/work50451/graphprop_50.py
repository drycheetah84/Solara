# graphprop_50.py -- locate source-build functions in 50451 by call-graph propagation (no address math).
# Neighbours (callers/callees) of each target are mapped to 50451 via unique-string function pairs and
# already-resolved results; the target must then be a 50451 callee of a mapped caller or a 50451 caller
# of a mapped callee. Candidates are scored by decompiled-body similarity; a caller whose call count is
# unchanged contributes a strong vote for the exact call slot (ordinal).
# Usage: idat -A -S"graphprop_50.py <srctag> <mode 22|47>" -L<log> <50451 copy.i64>  -> graphmatch_<srctag>.jsonl
import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_pro, json, re, difflib, glob, os, time
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
TAG, MODE = idc.ARGV[1], idc.ARGV[2]
def J(n): return json.load(open(os.path.join(W, n)))
G = J("graph_%s.json" % TAG)
LOG = open(os.path.join(W, "graphprop_%s.progress.log" % TAG), "a", buffering=1)
def log(*a): LOG.write(time.strftime("%H:%M:%S ") + " ".join(str(x) for x in a) + "\n")

MAP = {}                                                     # source func start -> 50451 func start
for s, d in J("strpairs_%s_50.json" % MODE): MAP[s] = d
if MODE == "47":
    A = J("anchors_47031.json")
    for rva, c in J("confirm2.json")["confirmed"].items():
        a = A.get(rva)
        if a and a.get("func") is not None: MAP[a["func"]] = int(c["rva50"], 16) - a["off"]
    pat = "bodymatch_f_*.jsonl"
else:
    A = J("anchors_22284all.json"); pat = "bodymatch_f22_*.jsonl"
for p in glob.glob(os.path.join(W, pat)):
    for ln in open(p):
        r = json.loads(ln)
        if r.get("status") == "body-match" and A.get(r["rva47"], {}).get("func") is not None:
            MAP[A[r["rva47"]]["func"]] = int(r["func50"], 16)
# UE native registration-table pairs are keyed by UFunction NAME: authoritative, override everything else
for s, d, how in J("natpairs_%s_50.json" % MODE):
    if not how.startswith("exact-dup/exec"): MAP[s] = d
log("mode", MODE, "neighbour map", len(MAP))

def calls_of(fs):
    f = ida_funcs.get_func(ib + fs)
    if not f: return []
    out, e = [], f.start_ea
    while e < f.end_ea:
        if idc.print_insn_mnem(e) in ("call", "jmp"):
            t = idc.get_operand_value(e, 0); tf = ida_funcs.get_func(t) if t else None
            if tf and tf.start_ea == t: out.append(t - ib)
        e = idc.next_head(e, f.end_ea)
    return out
def callers_of(fs, cap=200):
    out = set()
    for x in idautils.CodeRefsTo(ib + fs, 0):
        cf = ida_funcs.get_func(x)
        if cf: out.add(cf.start_ea - ib)
        if len(out) > cap: return None                      # too generic to be informative
    return out
TOK = re.compile(r"[A-Za-z_][A-Za-z_0-9]*|[^\sA-Za-z_0-9]")
def norm(s):
    if not s: return ""
    s = re.sub(r'sub_[0-9A-Fa-f]+', 'F', s)
    s = re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+', 'G', s)
    s = re.sub(r'0x[0-9A-Fa-f]+', 'N', s); s = re.sub(r'\b[0-9]+\b', 'N', s)
    s = re.sub(r'\bv[0-9]+\b', 'v', s); s = re.sub(r'\ba[0-9]+\b', 'a', s)
    return re.sub(r'\s+', '', s)
_bc = {}
def body50(fs):
    if fs not in _bc:
        try: _bc[fs] = TOK.findall(norm(str(ida_hexrays.decompile(ib + fs))))[:4000]
        except Exception: _bc[fs] = []
    return _bc[fs]

NATIVE = {s: d for s, d, how in J("natpairs_%s_50.json" % MODE) if not how.startswith("exact-dup/exec")}
final = {}
def resolve(h, g):
    t0 = time.time(); rec = {"rva": h, "mode": MODE}
    if g.get("func") is None or not g.get("body"):
        rec["status"] = "no-src"; return rec
    if g["func"] in NATIVE:                                  # the target itself is a named native: done
        d = NATIVE[g["func"]]
        rec.update(status="native", func50=hex(d), rva50=hex(d + g["off"])); return rec
    bsrc = TOK.findall(g["body"])[:4000]; sz = g["size"]
    votes, strong = {}, {}
    for c in g["callers"]:
        c50 = MAP.get(c["func"])
        if c50 is None: continue
        cl = calls_of(c50)
        if len(cl) == c["ncalls"]:
            for o in c["ord"]:
                if o < len(cl): strong.setdefault(cl[o], set()).add(("c", c["func"]))
        for t in set(cl): votes.setdefault(t, set()).add(("c", c["func"]))
    for e in g["callees"]:
        e50 = MAP.get(e)
        if e50 is None: continue
        cs = callers_of(e50)
        if cs is None: continue
        for t in cs: votes.setdefault(t, set()).add(("e", e))
    pool = set(strong) | set(votes)
    scored = []
    for cand in pool:
        f = ida_funcs.get_func(ib + cand)
        if not f: continue
        csz = f.end_ea - f.start_ea
        if abs(csz - sz) > max(0x80, sz // 2): continue
        b = body50(cand)
        if not b: continue
        sm = difflib.SequenceMatcher(None, bsrc, b, autojunk=False)
        if sm.real_quick_ratio() < 0.5: continue
        r = sm.ratio()
        scored.append((r, len(votes.get(cand, ())), len(strong.get(cand, ())), cand))
    scored.sort(key=lambda x: (x[0] + 0.02 * min(x[1], 5) + 0.05 * min(x[2], 3)), reverse=True)
    rec.update(pool=len(pool), nmapped_callers=sum(1 for c in g["callers"] if c["func"] in MAP),
               nmapped_callees=sum(1 for e in g["callees"] if e in MAP))
    if scored:
        r, nv, ns, cand = scored[0]
        r2 = scored[1][0] if len(scored) > 1 else 0.0
        ok = (r >= 0.85 and r - r2 >= 0.03) or (nv >= 2 and r >= 0.60 and (len(scored) == 1 or r - r2 >= 0.05)) \
             or (ns >= 1 and r >= 0.50 and (len(scored) == 1 or r - r2 >= 0.05))
        rec.update(status="graph-match" if ok else "graph-weak", func50=hex(cand), rva50=hex(cand + g["off"]),
                   ratio=round(r, 3), second=round(r2, 3), votes=nv, strong=ns)
    else:
        rec["status"] = "graph-none"
    rec["secs"] = round(time.time() - t0, 1)
    return rec

for rnd in range(1, 6):                                      # fixpoint: each find becomes a neighbour anchor
    new = 0
    for h, g in sorted(G.items()):
        if final.get(h, {}).get("status") in ("native", "graph-match"): continue
        rec = resolve(h, g); rec["round"] = rnd
        if rec.get("status") in ("native", "graph-match"):
            new += 1; MAP[g["func"]] = int(rec["func50"], 16)
        final[h] = rec
    log("round", rnd, "new", new, "resolved", sum(1 for r in final.values() if r.get("status") in ("native", "graph-match")), "/", len(G))
    if not new: break
fo = open(os.path.join(W, "graphmatch_%s.jsonl" % TAG), "w", buffering=1)
for h, rec in sorted(final.items()): fo.write(json.dumps(rec) + "\n")
log("DONE")
ida_pro.qexit(0)
