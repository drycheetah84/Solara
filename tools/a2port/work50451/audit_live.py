# audit_live.py -- every live (non-comment) hex literal used as an image RVA in the 50451 sources,
# classified against the 47031 target set and the current resolution state. Host python, no IDA.
import json, os, re, glob, collections
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SRC = r"C:\Users\drych\ProjectDrift\Solara\Server"
A = json.load(open(os.path.join(W, "anchors_47031.json")))
conf = json.load(open(os.path.join(W, "confirm2.json")))["confirmed"]
bm = {}
for p in glob.glob(os.path.join(W, "bodymatch_f_*.jsonl")):
    for ln in open(p):
        try: r = json.loads(ln); bm[r["rva47"]] = r
        except Exception: pass
dref = json.load(open(os.path.join(W, "dataref_47031.json"))) if os.path.exists(os.path.join(W, "dataref_47031.json")) else {}

# image-size / range bounds: not function or data RVAs, never remapped
BOUNDS = {"C000000", "20000000", "A5BB000"}
PAT = re.compile(r'(?:GetBase\(\)|\bbase|\bb0|\bb)\s*\+\s*0x0*([0-9A-Fa-f]{5,8})|_RVA\s*=\s*0x0*([0-9A-Fa-f]{5,8})|WriteByte\(\s*(?:GetBase\(\)|base)\s*\+\s*0x0*([0-9A-Fa-f]{5,8})')

sites = []
for fn in ["dllmain_50451.cpp", "specedit.h"]:
    for i, l in enumerate(open(os.path.join(SRC, fn), encoding="utf-8", errors="ignore")):
        code = l.split("//", 1)[0]
        for m in PAT.finditer(code):
            h = next(g for g in m.groups() if g).upper().lstrip("0")
            sites.append((fn, i + 1, h, l.strip()[:110]))

def state(h):
    if h in BOUNDS: return "bound"
    if h in conf or h == "41F5A60": return "confirmed"
    r = bm.get(h)
    if r and r.get("status") == "body-match": return "bodymatch"
    if h not in A: return "not-in-targets"
    if A[h].get("func") is None: return "data-global"
    if r: return "searched-nomatch"
    return "pending-search"

cnt = collections.Counter(); uniq = {}
for fn, ln, h, txt in sites:
    s = state(h); cnt[s] += 1; uniq.setdefault(h, (s, fn, ln, txt))
print("live sites:", len(sites), " unique RVAs:", len(uniq))
print("by site:", dict(cnt))
print("by unique:", dict(collections.Counter(v[0] for v in uniq.values())))
for want in ("not-in-targets", "data-global"):
    print("\n==", want)
    for h, (s, fn, ln, txt) in sorted(uniq.items()):
        if s == want: print(" ", h, fn, ln, "|", txt)
json.dump({h: {"state": v[0], "file": v[1], "line": v[2]} for h, v in uniq.items()},
          open(os.path.join(W, "live_audit.json"), "w"), indent=0)
