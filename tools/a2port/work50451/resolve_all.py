# resolve_all.py -- final 50451 value for EVERY image-range hex literal in dllmain_50451.cpp + specedit.h.
# Host python (no IDA). Reads the three shipping PEs directly:
#   * function maps (src func -> 50451 func) from all prior evidence, strongest first, conflicts dropped
#   * provenance by .pdata: a function-start literal belongs to the build where it is a RUNTIME_FUNCTION begin
#   * mid-function sites (patch sites, gate compares, range bounds): containing source function -> mapped
#     50451 function -> unique occurrence of the source bytes (rel32/RIP displacements masked) inside it
#   * function starts: map value + masked-prologue comparison as corroboration
# Output: resolved_all.json  {lit: {prov, kind, rva50|null, how, note}}
import json, os, re, glob, struct, bisect, collections, sys
import capstone
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SRC = r"C:\Users\drych\ProjectDrift\Solara\Server"
EXE = {"22": r"C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditorBuild\A2\Binaries\Win64\A2-Win64-Shipping.exe",
       "47": r"C:\Users\drych\Downloads\32586841537580874\32586841537580874\A2\Binaries\Win64\A2-Win64-Shipping.exe",
       "50": os.path.join(W, "A2-50451.exe")}
def J(n): return json.load(open(os.path.join(W, n)))

class PE:
    def __init__(s, path):
        s.d = open(path, "rb").read()
        e = struct.unpack_from("<I", s.d, 0x3C)[0]
        nsec = struct.unpack_from("<H", s.d, e + 6)[0]; osz = struct.unpack_from("<H", s.d, e + 20)[0]
        opt = e + 24
        s.size = struct.unpack_from("<I", s.d, opt + 56)[0]
        exc_rva, exc_sz = struct.unpack_from("<II", s.d, opt + 112 + 3 * 8)
        s.secs = []
        for i in range(nsec):
            o = opt + osz + i * 40
            name = s.d[o:o + 8].rstrip(b"\0").decode(errors="ignore")
            vsz, va, rsz, raw = struct.unpack_from("<IIII", s.d, o + 8)
            s.secs.append((name, va, max(vsz, rsz), raw, rsz))
        s.rf = []
        for i in range(exc_sz // 12):
            b, en, _ = struct.unpack_from("<III", s.d, s.off(exc_rva) + i * 12)
            s.rf.append((b, en))
        s.rf.sort(); s.rfb = [x[0] for x in s.rf]; s.begins = set(s.rfb)
    def off(s, rva):
        for n, va, sz, raw, rsz in s.secs:
            if va <= rva < va + sz and rva - va < rsz: return raw + rva - va
        return None
    def sec(s, rva):
        for n, va, sz, raw, rsz in s.secs:
            if va <= rva < va + sz: return n
        return None
    def read(s, rva, n):
        o = s.off(rva); return s.d[o:o + n] if o is not None else b""
    def chunk(s, rva):
        i = bisect.bisect_right(s.rfb, rva) - 1
        if i >= 0 and s.rf[i][0] <= rva < s.rf[i][1]: return s.rf[i]
        return None
P = {k: PE(v) for k, v in EXE.items()}
cs = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64); cs.detail = True

def masked(pe, start, end):
    """bytes of [start,end) as a list with rel32 branch targets / RIP displacements masked (None)."""
    code = pe.read(start, end - start); out = []
    for ins in cs.disasm(code, start):
        b = list(ins.bytes)
        mask = set()
        dsz = ins.disp_size if hasattr(ins, "disp_size") else 0
        if ins.disp_offset and any(op.type == capstone.x86.X86_OP_MEM and op.mem.base == capstone.x86.X86_REG_RIP for op in ins.operands):
            mask |= set(range(ins.disp_offset, ins.disp_offset + ins.disp_size))
        if ins.imm_offset and ins.group(capstone.CS_GRP_JUMP) or ins.imm_offset and ins.group(capstone.CS_GRP_CALL):
            if ins.imm_size == 4: mask |= set(range(ins.imm_offset, ins.imm_offset + 4))
        out += [None if i in mask else v for i, v in enumerate(b)]
    return out[:end - start]

def find_masked(pe, lo, hi, pat):
    hay = pe.read(lo, hi - lo); n = len(pat); hits = []
    anchor = next((i for i, v in enumerate(pat) if v is not None), None)
    if anchor is None: return hits
    a = bytes([pat[anchor]])
    i = hay.find(a, anchor)
    while i != -1:
        st = i - anchor
        if st + n <= len(hay) and all(v is None or hay[st + k] == v for k, v in enumerate(pat)): hits.append(lo + st)
        i = hay.find(a, i + 1)
    return hits

# ---------------- function maps ----------------
def build_map(mode):
    strong = collections.defaultdict(set); weak = {}
    for s, d, how in J("natpairs_%s_50.json" % mode):
        if not how.startswith("exact-dup/exec"): strong[s].add(("native", d))
    for s, d in J("strpairs_%s_50.json" % mode): weak[s] = d
    tag, A = ("47031", J("anchors_47031.json")) if mode == "47" else ("22284all", J("anchors_22284all.json"))
    for ln in open(os.path.join(W, "graphmatch_%s.jsonl" % tag)):
        r = json.loads(ln)
        a = A.get(r["rva"])
        if r["status"] in ("native", "graph-match") and a and a.get("func") is not None:
            strong[a["func"]].add(("graph", int(r["func50"], 16)))
    pat = "bodymatch_f_*.jsonl" if mode == "47" else "bodymatch_f22_*.jsonl"
    for p in glob.glob(os.path.join(W, pat)):
        for ln in open(p):
            r = json.loads(ln); a = A.get(r.get("rva47") or r.get("rva"))
            if r["status"] == "body-match" and a and a.get("func") is not None:
                strong[a["func"]].add(("body", int(r["func50"], 16)))
    xl = {"47": [("graph_47031x.json", "xloc_d47x.jsonl"), ("graph_47031h.json", "xloc_d47h.jsonl")],
          "22": [("graph_22284x.json", "xloc_d22.jsonl")]}[mode]
    for gf, xf in xl:
        if not os.path.exists(os.path.join(W, xf)): continue
        G = J(gf)
        for ln in open(os.path.join(W, xf)):
            r = json.loads(ln)
            g = G.get(r["src"])
            if r["status"] in ("native", "graph", "window", "both") and g and g.get("func") is not None:
                strong[g["func"]].add(("xloc", int(r["func"], 16)))
    M = {}
    for s, ev in strong.items():
        nat = {d for h, d in ev if h == "native"}
        if nat:
            if len(nat) == 1: M[s] = (next(iter(nat)), "native")
            continue
        ds = {d for h, d in ev}
        if len(ds) == 1: M[s] = (next(iter(ds)), "+".join(sorted({h for h, d in ev})))
    for s, d in weak.items():
        if s not in M: M[s] = (d, "str")
        elif M[s][0] != d and M[s][1] != "native": M[s] = (M[s][0], M[s][1] + "!strdisagree")
    return M
MAP = {"22": build_map("22"), "47": build_map("47")}
if "22" in MAP:   # two-hop 22->47->50 for anything missing
    pass

FL = J("final_live.json"); M22 = J("merged22.json"); PROVF = J("provenance_final.json")

# ---------------- collect literals ----------------
hx = re.compile(r'0x0*([0-9A-Fa-f]{6,8})\b')
lits = collections.OrderedDict()
for fn in ("dllmain_50451.cpp", "specedit.h"):
    for i, l in enumerate(open(os.path.join(SRC, fn), encoding="utf-8", errors="ignore")):
        code = l.split("//", 1)[0]
        code = re.sub(r'"(?:\\.|[^"\\])*"', '""', code)      # ignore literals inside log strings
        for m in hx.finditer(code):
            v = int(m.group(1), 16)
            if 0x1000000 <= v < 0xC000000: lits.setdefault(v, []).append((fn, i + 1))
NOT_RVA = {0x1000000, 0x5000000, 0x5A00000, 0xC000000, 0xA5BB000}   # masks / coarse region bounds, handled by hand

def containing(pe, rva, mode):
    """source function start for a mid-function rva: prefer IDA func from anchors/datax, else pdata chunk."""
    h = "%X" % rva
    for af in (("anchors_47031.json", "datax_47031all.json") if mode == "47" else ("anchors_22284all.json", "datax_22284.json")):
        a = J(af).get(h)
        if a:
            f = a.get("func") if "func" in a else (a.get("in_func") or {}).get("func")
            if f is not None: return f, "ida"
    c = pe.chunk(rva)
    return (c[0], "pdata") if c else (None, None)

def provenance(v):
    h = "%X" % v
    p = PROVF.get(h)
    if p in ("22284", "MANUAL"): return "22"
    if p == "47031": return "47"
    b22, b47 = v in P["22"].begins, v in P["47"].begins
    if b47 and not b22: return "47"
    if b22 and not b47: return "22"
    return "?"

import difflib
def fbody(pe, f, cap=0x600):
    c = pe.chunk(f)
    n = (c[1] - f) if c and c[0] == f else 0x80
    return masked(pe, f, f + min(n, cap)), n
def fsim(pa, fa, pb, fb):
    a, na = fbody(pa, fa); b, nb = fbody(pb, fb)
    if not a or not b: return 0.0, na, nb
    sm = difflib.SequenceMatcher(None, ["?" if x is None else x for x in a], ["?" if x is None else x for x in b], autojunk=False)
    return round(sm.ratio(), 3), na, nb

def func_info(v, mode):
    h = "%X" % v
    for af in (("anchors_47031.json", "datax_47031all.json") if mode == "47" else ("anchors_22284all.json", "datax_22284.json")):
        a = J(af).get(h)
        if a:
            if "func" in a and a["func"] is not None: return a["func"], v - a["func"], "ida"
            f = (a.get("in_func") or {}).get("func")
            if f is not None: return f, v - f, "ida"
    c = P[mode].chunk(v)
    return (c[0], v - c[0], "pdata") if c else (None, None, None)

def try_mode(v, mode):
    h = "%X" % v; pe = P[mode]
    f, off, src = func_info(v, mode)
    if f is None: return None
    cands = []
    if off == 0:
        fl = FL.get(h)
        if fl and fl.get("decision") in ("native", "corroborated", "single") and fl.get("rva50") and fl.get("prov") == {"22": "22284", "47": "47031"}[mode]:
            cands.append((int(fl["rva50"], 16), "final_live:" + fl["decision"]))
        if mode == "22" and M22.get(h, {}).get("status") in ("agree", "direct", "hop"):
            cands.append((int(M22[h]["rva50"], 16), "merged22:" + M22[h]["status"]))
        m = MAP[mode].get(f)
        if m: cands.append((m[0], "map:" + m[1]))
        res = []
        for d, how in cands:
            sim, na, nb = fsim(pe, f, P["50"], d)
            res.append({"kind": "func", "rva50": d, "how": how, "sim": sim, "len": "%d/%d" % (na, nb)})
        if not res: return {"kind": "func", "rva50": None, "how": "unmapped-func", "sim": 0}
        res.sort(key=lambda r: -r["sim"])
        return res[0]
    m = MAP[mode].get(f)
    if not m: return {"kind": "site", "rva50": None, "how": "site-func-unmapped", "func": f, "sim": 0}
    f50 = m[0]
    c0 = pe.chunk(f); size = (c0[1] - c0[0]) if c0 else 0x400
    lo, hi = f50, f50 + max(2 * max(size, off + 64), 0x800)
    for k in (24, 16, 12, 8, 6, 4):
        st = max(f, v - k)
        pat = masked(pe, st, v + 16)
        if len([x for x in pat if x is not None]) < 10: continue
        hits = find_masked(P["50"], lo, hi, pat)
        if len(hits) == 1:
            return {"kind": "site", "rva50": hits[0] + (v - st), "how": "site-bytes(ctx%d,%s)" % (k, m[1]), "func": f, "func50": f50, "sim": 1.0}
    return {"kind": "site", "rva50": None, "how": "site-bytes-nomatch(%s)" % m[1], "func": f, "func50": f50, "sim": 0}

# ---- provenance pins (source build of each literal) ----
PROVX = {}
for h, p in PROVF.items(): PROVX[h] = "47" if p == "47031" else "22"
for h, p in (("12F0CB0", "22"), ("54E17E0", "22"), ("54E4E10", "22"), ("54B13D0", "22"), ("54B1421", "22"), ("54277AD", "22")): PROVX[h] = p
def lastbyte_cmp2(pe, rva, ln):
    b = pe.read(rva, ln)
    if len(b) != ln or b[-1] != 2: return False
    ins = list(cs.disasm(b, rva))
    return len(ins) == 1 and ins[0].mnemonic == "cmp" and ins[0].size == ln
for line in open(os.path.join(SRC, "dllmain_50451.cpp"), encoding="utf-8", errors="ignore"):
    for a, ln in re.findall(r'\{0x([0-9A-Fa-f]{7}),([37])\}', line.split("//")[0]):
        v = int(a, 16); ok = [m for m in ("47", "22") if lastbyte_cmp2(P[m], v, int(ln))]
        PROVX.setdefault(a.upper(), ok[0] if len(ok) == 1 else ("47" if "47" in ok else "?"))
for line in open(os.path.join(SRC, "dllmain_50451.cpp"), encoding="utf-8", errors="ignore"):
    m = re.search(r'\{ 0x([0-9A-Fa-f]{7}), \d, \d, ?\d+,\s*\{([^}]*)\}', line)
    if m:
        v = int(m.group(1), 16); want = [int(x, 16) for x in re.findall(r'0x([0-9A-Fa-f]{2})', m.group(2))]
        tot = int(re.search(r'\d, \d, ?(\d+)', line).group(1)); want = bytes(want[:tot])
        ok = [k for k in ("47", "22") if P[k].read(v, tot) == want]
        PROVX[m.group(1).upper()] = ok[0] if len(ok) == 1 else ("47" if "47" in ok else "?")
for fn in ("specedit.h", "dllmain_50451.cpp"):
    for line in open(os.path.join(SRC, fn), encoding="utf-8", errors="ignore"):
        for a in re.findall(r'=\s*0x([0-9A-Fa-f]{7})', line):
            if a.upper() in PROVX: continue
            if "TODO47031 unresolved" in line or "TODO-47031 UNRESOLVED" in line: PROVX[a.upper()] = "22"
            elif "[47031" in line: PROVX[a.upper()] = "47"
for v in lits:   # generic: a function start in exactly one build
    h = "%X" % v
    if h in PROVX: continue
    b = [m for m in ("47", "22") if func_info(v, m)[1] == 0]
    if len(b) == 1: PROVX[h] = b[0]
json.dump(PROVX, open(os.path.join(W, "provx.json"), "w"), indent=0)

out = {}
for v, sites in lits.items():
    h = "%X" % v
    if v in NOT_RVA: out[h] = {"kind": "bound", "rva50": None, "how": "manual-bound", "sites": sites}; continue
    pv = PROVX.get(h)
    r = {m: (try_mode(v, m) if pv in (None, m) else None) for m in ("47", "22")}
    ok = {m: x for m, x in r.items() if x and x["rva50"] is not None and (x["kind"] == "site" or x["sim"] >= 0.85)}
    rec = {"sites": sites, "alt": {m: (hex(x["rva50"]) if x and x["rva50"] is not None else None, x and x["how"], x and x["sim"]) for m, x in r.items()}}
    if len(ok) == 2 and ok["47"]["rva50"] != ok["22"]["rva50"]:
        rec.update(prov="?", kind=ok["47"]["kind"], rva50=None, how="CONFLICT")
    elif ok:
        m = max(ok, key=lambda k: ok[k]["sim"]); rec.update(prov=m if len(ok) == 1 else "both", **ok[m])
    else:
        best = max((x for x in r.values() if x), key=lambda x: x["sim"], default=None)
        rec.update(prov="?", kind=(best or {}).get("kind"), rva50=None, how=(best or {}).get("how", "no-func"),
                   sim=(best or {}).get("sim"), cand=hex(best["rva50"]) if best and best["rva50"] is not None else None)
    out[h] = rec
# same-build duplicate targets are suspicious
dup = collections.defaultdict(list)
for h, x in out.items():
    if x.get("rva50") is not None and x.get("kind") == "func": dup[(x["prov"], x["rva50"])].append(h)
for (p, d), hs in dup.items():
    if len(hs) > 1:
        for h in hs: out[h]["dup"] = hs
ser = {h: {k: (hex(x) if k in ("rva50", "func", "func50") and isinstance(x, int) else x) for k, x in r.items()} for h, r in out.items()}
json.dump(ser, open(os.path.join(W, "resolved_all.json"), "w"), indent=1)
c = collections.Counter((r.get("kind"), r.get("rva50") is not None) for r in out.values())
print("literals:", len(out), dict(c))
for h, r in sorted(out.items(), key=lambda x: (x[1].get("rva50") is not None, x[0])):
    print("%-8s %-4s %-5s %-10s %-34s sim=%s %s %s" % (h, r.get("prov", ""), r.get("kind"), ser[h].get("rva50"), r.get("how"), r.get("sim"), r.get("len", ""),
          ("DUP" + str(r["dup"])) if r.get("dup") else "", ) + ("" if r.get("rva50") is not None else "  alt=" + str(r.get("alt"))))
