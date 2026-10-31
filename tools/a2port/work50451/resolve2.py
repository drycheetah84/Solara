# resolve2.py -- final 50451 value for every image-range hex literal in dllmain_50451.cpp + specedit.h.
# Every value is located BY BYTES in the 50451 PE and verified, never taken on trust:
#   function start : unique masked-prologue match that is a 50451 .pdata begin, full-body similarity >= 0.85
#                    (candidates: prior function-level decisions, function maps, and a window search)
#   code site      : unique instruction-aligned masked context match (in the mapped function, else in a
#                    +-256KB window around the interpolated estimate)
#   data global    : each source RIP-relative xref is located as a code site; the 50451 instruction's target
#                    (+ the literal's offset from the source target) must agree across xrefs
# 22284 literals are resolved twice (direct 22->50 and 22->47->50); the routes must agree when both exist.
# Provenance comes from provx.json (written by resolve_all.py: proof-tested pins).
import json, os, re, sys, bisect, collections, difflib
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from pecore import PE, EXE, find_masked, call_index
W = os.path.dirname(os.path.abspath(__file__))
SRC = r"C:\Users\drych\ProjectDrift\Solara\Server"
def J(n): return json.load(open(os.path.join(W, n)))
P = {k: PE(v) for k, v in EXE.items()}

# ---------------- function maps + interpolation bases ----------------
def fmap(src, dst):
    strong = collections.defaultdict(set); weak = {}
    for s, d, how in J("natpairs_%s_%s.json" % (src, dst)):
        if not how.startswith("exact-dup/exec"): strong[s].add(("native", d))
    for s, d in J("strpairs_%s_%s.json" % (src, dst)): weak[s] = d
    if dst == "50":
        tag, A = ("47031", J("anchors_47031.json")) if src == "47" else ("22284all", J("anchors_22284all.json"))
        for ln in open(os.path.join(W, "graphmatch_%s.jsonl" % tag)):
            r = json.loads(ln); a = A.get(r["rva"])
            if r["status"] in ("native", "graph-match") and a and a.get("func") is not None:
                strong[a["func"]].add(("graph", int(r["func50"], 16)))
        xl = {"47": [("graph_47031x.json", "xloc_d47x.jsonl"), ("graph_47031h.json", "xloc_d47h.jsonl")],
              "22": [("graph_22284x.json", "xloc_d22.jsonl")]}[src]
    else:
        xl = [("graph_22284x.json", "xloc_h22.jsonl")]
    for gf, xf in xl:
        if not os.path.exists(os.path.join(W, xf)): continue
        G = J(gf)
        for ln in open(os.path.join(W, xf)):
            r = json.loads(ln); g = G.get(r["src"])
            if r["status"] in ("native", "graph", "window", "both") and g and g.get("func") is not None:
                strong[g["func"]].add(("xloc", int(r["func"], 16)))
    M = {}
    for s, ev in strong.items():
        ds = {d for h, d in ev if h == "native"} or {d for h, d in ev}
        if len(ds) == 1: M[s] = next(iter(ds))
    for s, d in weak.items(): M.setdefault(s, d)
    basis = sorted(J("strpairs_%s_%s.json" % (src, dst)))
    return M, basis
ROUTES = {("47", "50"): fmap("47", "50"), ("22", "50"): fmap("22", "50"), ("22", "47"): fmap("22", "47")}

def estimate(basis, v):
    xs = [p[0] for p in basis]; i = bisect.bisect_left(xs, v)
    near = basis[max(0, i - 6):i + 6]
    ds = sorted(b - a for a, b in near)
    return v + ds[len(ds) // 2]

def fbody(pe, f, cap=0x800, hint=None):
    c = pe.chunk(f); n = (c[1] - f) if c and c[0] == f else (hint or 0x80)
    ins = pe.insns(f, f + min(n, cap)); out = []
    for a, k, b, m, _ in ins:
        if a >= f + min(n, cap): break
        out += ["?" if i in m else b[i] for i in range(k)]
    return out, n
def fsim(pa, fa, pb, fb):
    a, na = fbody(pa, fa); b, nb = fbody(pb, fb, hint=na)
    if not a or not b: return 0.0
    if min(na, nb) / max(na, nb) < 0.6: return 0.0
    return round(difflib.SequenceMatcher(None, a, b, autojunk=False).ratio(), 3)

# ---------------- generic locators ----------------
VALID = {}
def loc_site(sp, dp, v, route):
    M, basis = route
    c = sp.chunk(v)
    regions = []
    if c and c[0] in M:
        f50 = M[c[0]]; size = c[1] - c[0]
        regions.append(("func", f50 - 0x40, f50 + 2 * size + 0x400))
    est = estimate(basis, v)
    regions.append(("window", est - 0x40000, est + 0x40000))
    val = VALID.get(v)
    for name, lo, hi in regions:
        ctxs = ((24, 24), (16, 16), (32, 8), (8, 24), (48, 16))
        if name == "func": ctxs += ((12, 8), (8, 8), (16, 4), (4, 12))
        for before, after in ctxs:
            st, pat = sp.window(v, before, after)
            if pat is None or sum(1 for x in pat if x is not None) < (10 if name == "func" else 16): continue
            hits = find_masked(dp, lo, hi, pat)
            if len(hits) == 1:
                d = hits[0] + (v - st)
                if dp is P["50"] and val and not val(d): continue
                return d, "%s(%d/%d)" % (name, before, after)
    return None, None

def loc_func(sp, dp, v, route, extra=()):
    M, basis = route
    cands = list(extra)
    if v in M: cands.append((M[v], "map"))
    est = estimate(basis, v)
    for n in (64, 48, 32):
        st, pat = sp.window(v, 0, n)
        if not pat or sum(1 for x in pat if x is not None) < 16: continue
        hits = find_masked(dp, est - 0x40000, est + 0x40000, pat, limit=8)
        if len(hits) == 1: cands.append((hits[0], "prologue-window%d" % n)); break
        if len(hits) > 1: break
    if not cands or True:
        cv, ch = by_callers(sp, dp, v, route)
        if cv is not None: cands.append((cv, ch))
    best = None
    for d, how in cands:
        s = fsim(sp, v, dp, d)
        if best is None or s > best[2]: best = (d, how, s)
    if best and (best[2] >= 0.85 or (best[1].startswith("callers") and best[2] >= 0.6)): return best[0], "%s sim=%.3f" % (best[1], best[2])
    return None, ("best %s sim=%.3f" % (best[1], best[2])) if best else "no-candidate"

def call_target(pe, at):
    ins = pe.ins_at(at)
    if ins is None or ins.mnemonic not in ("call", "jmp") or ins.size != 5: return None
    return ins.operands[0].imm if ins.operands and ins.operands[0].type == 2 else None
def by_callers(sp, dp, v, route, cap=40):
    """vote: locate each direct call site of v in dp and read its call target."""
    sites = [a for a in call_index(sp).get(v, []) if sp.ins_at(a) is not None and sp.ins_at(a).size == 5][:cap]
    votes = collections.Counter()
    for a in sites:
        d, _ = loc_site(sp, dp, a, route)
        if d is None: continue
        t = call_target(dp, d)
        if t is not None: votes[t] += 1
    if not votes: return None, "callers:%d-unlocated" % len(sites)
    (t, n), = votes.most_common(1)
    tot = sum(votes.values())
    if (n >= 2 and n / tot >= 0.75) or (n == 1 and tot == 1 and len(sites) == 1):
        s = fsim(sp, v, dp, t)
        if s >= 0.6: return t, "callers %d/%d (of %d) sim=%.2f" % (n, tot, len(sites), s)
        return None, "callers %d/%d -> %X but sim=%.2f" % (n, tot, t, s)
    return None, "callers-split %s" % {hex(k): c for k, c in votes.most_common(4)}

def resolve_code(v, prov, extra=()):
    """returns (rva50, how) for a code literal of build prov ('47' or '22')."""
    sp = P[prov]
    is_func = v in sp.begins or any(func_off(v, prov) == 0 for _ in [0])
    if prov == "47":
        return (loc_func if is_func else loc_site)(sp, P["50"], v, ROUTES[("47", "50")], *([extra] if is_func else []))
    d, dh = (loc_func if is_func else loc_site)(sp, P["50"], v, ROUTES[("22", "50")], *([extra] if is_func else []))
    m47, h47 = (loc_func if is_func else loc_site)(sp, P["47"], v, ROUTES[("22", "47")])
    hop, hh = (None, None)
    if m47 is not None:
        hop, hh = (loc_func if is_func else loc_site)(P["47"], P["50"], m47, ROUTES[("47", "50")])
    if d is not None and hop is not None:
        return (d, "agree[%s | via47 %X %s]" % (dh, m47, hh)) if d == hop else (None, "CONFLICT direct %X vs hop %X" % (d, hop))
    if d is not None: return d, "direct[%s]" % dh
    if hop is not None: return hop, "hop[47 %X %s; %s]" % (m47, h47, hh)
    return None, "none[direct:%s | 47:%s]" % (dh, h47)

A47, A22 = J("anchors_47031.json"), J("anchors_22284all.json")
def func_off(v, prov):
    a = (A47 if prov == "47" else A22).get("%X" % v)
    return (v - a["func"]) if a and a.get("func") is not None else None

# ---------------- data globals ----------------
DX = {"47": J("datax_47031all.json"), "22": J("datax_22284.json")}
def rip_target(pe, at):
    ins = pe.ins_at(at)
    if ins is None: return None
    for op in ins.operands:
        if op.type == 3 and op.mem.base == 41:   # X86_OP_MEM, X86_REG_RIP
            return at + ins.size + op.mem.disp
    return None
def resolve_data(v, prov):
    r = DX[prov].get("%X" % v)
    if not r or not r.get("refs"): return None, "no-xrefs"
    votes = collections.Counter(); seen = 0
    for x in r["refs"]:
        site = x["site"]; t = rip_target(P[prov], site)
        if t is None: continue
        d, how = resolve_code(site, prov) if True else (None, None)
        if d is None: continue
        t50 = rip_target(P["50"], d)
        if t50 is None: continue
        seen += 1; votes[t50 + (v - t)] += 1
    if not votes: return None, "xrefs-unlocated(%d)" % len(r["refs"])
    (best, n), = votes.most_common(1)
    if len(votes) == 1 and (n >= 2 or len(r["refs"]) == 1): return best, "xrefs %d/%d agree" % (n, len(r["refs"]))
    return None, "xref-votes %s" % {hex(k): c for k, c in votes.items()}

for line in open(os.path.join(SRC, "dllmain_50451.cpp"), encoding="utf-8", errors="ignore"):
    code = line.split("//")[0]
    for a, ln in re.findall(r'\{0x([0-9A-Fa-f]{7}),([37])\}', code):
        L = int(ln)
        VALID[int(a, 16)] = (lambda L: lambda d: P["50"].read(d + L - 1, 1) == b"" and (P["50"].ins_at(d) is not None and P["50"].ins_at(d).mnemonic == "cmp" and P["50"].ins_at(d).size == L))(L)
    m = re.search(r'\{ 0x([0-9A-Fa-f]{7}), \d, \d, ?(\d+),\s*\{([^}]*)\}', line)
    if m:
        want = bytes(int(x, 16) for x in re.findall(r'0x([0-9A-Fa-f]{2})', m.group(3)))[:int(m.group(2))]
        VALID[int(m.group(1), 16)] = (lambda w: lambda d: P["50"].read(d, len(w)) == w)(want)

# ---------------- literals ----------------
PROVX = J("provx.json")
hx = re.compile(r'0x0*([0-9A-Fa-f]{6,8})\b')
lits = collections.OrderedDict()
for fn in ("dllmain_50451.cpp", "specedit.h"):
    for i, l in enumerate(open(os.path.join(SRC, fn), encoding="utf-8", errors="ignore")):
        code = re.sub(r'"(?:\\.|[^"\\])*"', '""', l.split("//", 1)[0])
        for m in hx.finditer(code):
            v = int(m.group(1), 16)
            if 0x1000000 <= v < 0xC000000: lits.setdefault(v, []).append((fn, i + 1))
NOT_RVA = {0x1000000, 0x5000000, 0x5A00000, 0xC000000, 0xA5BB000}
FL, M22 = J("final_live.json"), J("merged22.json")

out = {}
for v, sites in lits.items():
    h = "%X" % v
    rec = {"sites": sites}
    if v in NOT_RVA: rec.update(kind="bound", rva50=None, how="not-an-rva/coarse-bound"); out[h] = rec; continue
    prov = PROVX.get(h)
    if prov not in ("22", "47"):
        prov = "47" if any(func_off(v, "47") == 0 for _ in [0]) else ("22" if func_off(v, "22") == 0 else None)
    if prov is None:
        # data literal with no pin: try 47 then 22 by section
        for p in ("47", "22"):
            if P[p].sec(v) in (".data", ".rdata") and "%X" % v in DX[p]: prov = p; break
    if prov is None: rec.update(kind="?", rva50=None, how="no-provenance"); out[h] = rec; continue
    rec["prov"] = prov
    sec = P[prov].sec(v)
    if sec not in (".text",):
        d, how = resolve_data(v, prov); rec.update(kind="data", rva50=d, how=how)
    else:
        extra = []
        fl = FL.get(h)
        if fl and fl.get("rva50") and fl.get("prov") == {"22": "22284", "47": "47031"}[prov]: extra.append((int(fl["rva50"], 16), "final_live"))
        if prov == "22" and M22.get(h, {}).get("rva50"): extra.append((int(M22[h]["rva50"], 16), "merged22"))
        d, how = resolve_code(v, prov, extra)
        rec.update(kind="func" if (v in P[prov].begins or func_off(v, prov) == 0) else "site", rva50=d, how=how)
    out[h] = rec
    print("%-8s %s %-5s %-10s %s" % (h, prov, rec["kind"], hex(d) if rec["rva50"] is not None else "-", rec["how"]), flush=True)

json.dump({h: dict(r, rva50=hex(r["rva50"]) if r.get("rva50") is not None else None) for h, r in out.items()},
          open(os.path.join(W, "resolved2.json"), "w"), indent=1)
c = collections.Counter((r["kind"], r.get("rva50") is not None) for r in out.values())
print("TOTAL", len(out), dict(c))
