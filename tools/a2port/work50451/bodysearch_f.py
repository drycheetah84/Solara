# bodysearch_f.py -- sharded, resumable cross-build function locator (source build -> 50451).
# For each source-build target: estimate its 50451 location by interpolating from address-correspondence
# pairs, scan the 50451 functions in a window around the estimate, and confirm by decompiled-body
# similarity (token-level). Results are appended per target (jsonl) so runs resume and nothing is lost.
#   mode 47: targets = 47031-provenance targets   (anchors_47031 / bodies47)
#   mode 22: targets = 22284-provenance literals  (anchors_22284all / bodies_22284all)
# Basis pairs (func_src -> func_50): confirmed map (mode 47) + unique-string pairs strpairs_<mode>_50.json.
# Usage: idat -A -S"bodysearch_f.py <shard> <nshards> <mode>" -L<log> <copy.i64>
import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_pro, json, re, bisect, difflib, time, os
idaapi.auto_wait()
ib = idaapi.get_imagebase()
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SHARD = int(idc.ARGV[1]) if len(idc.ARGV) > 1 else 0
NSH = int(idc.ARGV[2]) if len(idc.ARGV) > 2 else 1
MODE = idc.ARGV[3] if len(idc.ARGV) > 3 else "47"
SFX = {"47": "", "22": "22", "47c": "47c"}[MODE]
OUT = W + r"\bodymatch_f%s_%d.jsonl" % (SFX, SHARD)
LOG = open(W + r"\bodysearch_f%s_%d.progress.log" % (SFX, SHARD), "a", buffering=1)
def log(*a): LOG.write(time.strftime("%H:%M:%S ") + " ".join(str(x) for x in a) + "\n")
def J(n): return json.load(open(os.path.join(W, n)))

prov = J("provenance_final.json")
pairs = set()
if MODE in ("47", "47c"):
    A = J("anchors_47031.json"); B = J("bodies47.json")
    conf = J("confirm2.json")["confirmed"]; conf.setdefault("41F5A60", {"rva50": "0x41f5a20"})
    for rva, c in conf.items():             # a confirmed 47031<->50451 body correspondence is a valid
        a = A.get(rva)                      # address anchor even when the literal itself was stale
        if a and a.get("func") is not None: pairs.add((a["func"], int(c["rva50"], 16) - a["off"]))
    targets = sorted(r for r in A if r not in conf and A[r].get("func") is not None and prov.get(r) != "22284")
    if MODE == "47c":                        # re-verify old confirm2 (it never checked sibling candidates)
        targets = sorted(r for r in conf if r in A and A[r].get("func") is not None and prov.get(r) != "22284")
        pairs = set(p for p in pairs)
else:
    A = J("anchors_22284all.json"); B = J("bodies_22284all.json")
    targets = sorted(l.strip() for l in open(os.path.join(W, "rva22_funcs.txt")) if l.strip())
sp = os.path.join(W, "strpairs_%s_50.json" % MODE[:2])
if os.path.exists(sp):
    for s, d in json.load(open(sp)): pairs.add((s, d))
pairs = sorted(pairs); xs = [p[0] for p in pairs]
log("mode", MODE, "basis pairs", len(pairs))

def interp(f):
    i = bisect.bisect_left(xs, f)
    if i <= 0: return f + (pairs[0][1] - pairs[0][0])
    if i >= len(pairs): return f + (pairs[-1][1] - pairs[-1][0])
    lo, hi = pairs[i - 1], pairs[i]
    if hi[0] == lo[0]: return f + (lo[1] - lo[0])
    t = (f - lo[0]) / (hi[0] - lo[0]); dlo = lo[1] - lo[0]; dhi = hi[1] - hi[0]
    return f + int(dlo + t * (dhi - dlo))

TOK = re.compile(r"[A-Za-z_][A-Za-z_0-9]*|[^\sA-Za-z_0-9]")
def norm(s):
    if not s: return ""
    s = re.sub(r'sub_[0-9A-Fa-f]+', 'F', s)
    s = re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+', 'G', s)
    s = re.sub(r'0x[0-9A-Fa-f]+', 'N', s); s = re.sub(r'\b[0-9]+\b', 'N', s)
    s = re.sub(r'\bv[0-9]+\b', 'v', s); s = re.sub(r'\ba[0-9]+\b', 'a', s)
    return re.sub(r'\s+', '', s)
def toks(n): return TOK.findall(n)[:4000]
_cache = {}
def body50(frva):
    if frva not in _cache:
        try: _cache[frva] = toks(norm(str(ida_hexrays.decompile(ib + frva))))
        except Exception: _cache[frva] = []
    return _cache[frva]

import glob
done_set = set()
for p in glob.glob(W + r"\bodymatch_f%s_*.jsonl" % SFX):   # every shard of this mode
    for ln in open(p):
        try: done_set.add(json.loads(ln)["rva47"])
        except Exception: pass
fo = open(OUT, "a", buffering=1)
WINDOW = 0x20000
MAXC, BUDGET = 16, 90.0
mine = [r for i, r in enumerate(targets) if i % NSH == SHARD and r not in done_set]
log("shard", SHARD, "/", NSH, "targets", len(mine), "(already done", len(done_set), ")")

for k, rva in enumerate(mine):
    t0 = time.time()
    a = A[rva]; off = a["off"]; sz = a["size"]
    bsrc = toks(B.get(rva, {}).get("norm", ""))
    rec = {"rva47": rva, "mode": MODE}
    if not bsrc:
        rec["status"] = "no-body"
    else:
        est = interp(a["func"]); tol = max(0x40, sz // 8)
        cands = {}
        for fea in idautils.Functions(max(ib, ib + est - WINDOW), ib + est + WINDOW):   # cannot loop
            ff = ida_funcs.get_func(fea)
            if not ff: continue
            d = abs((ff.end_ea - ff.start_ea) - sz)
            if d <= tol: cands[fea - ib] = d
        scored = []
        for s in sorted(cands, key=lambda q: (cands[q], abs(q - est)))[:MAXC]:
            if time.time() - t0 > BUDGET: rec["timeout"] = True; break
            b = body50(s)
            if not b: continue
            sm = difflib.SequenceMatcher(None, bsrc, b, autojunk=False)
            if sm.real_quick_ratio() < 0.80 or sm.quick_ratio() < 0.80: continue
            scored.append((sm.ratio(), s))
        scored.sort(reverse=True)
        best, bestf = scored[0] if scored else (0.0, None)
        second = scored[1][0] if len(scored) > 1 else 0.0
        if bestf is not None and best >= 0.90 and best - second >= 0.03:
            rec.update(status="body-match", func50=hex(bestf), off=off, rva50=hex(bestf + off),
                       ratio=round(best, 3), second=round(second, 3))
        elif bestf is not None and best >= 0.97:
            # near-identical twins (template copies / thunks): take the one nearest the estimate, flagged
            twins = [s for r, s in scored if best - r < 0.005]
            pick = min(twins, key=lambda s: abs(s - est))
            rec.update(status="body-twin", func50=hex(pick), off=off, rva50=hex(pick + off), ratio=round(best, 3),
                       twins=[hex(s) for s in twins], est=hex(est))
        else:
            rec.update(status="no-body-match", best=round(best, 3), bestfunc=hex(bestf) if bestf else None,
                       est=hex(est), ncands=len(cands))
    rec["secs"] = round(time.time() - t0, 1)
    fo.write(json.dumps(rec) + "\n")
    log(k + 1, "/", len(mine), rva, rec["status"], rec.get("rva50", ""), rec.get("ratio", rec.get("best", "")), rec["secs"], "s")
log("DONE shard", SHARD)
ida_pro.qexit(0)
