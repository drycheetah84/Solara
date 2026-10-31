# provenance.py -- is each live literal a 47031 address or a stale 22284 one?
# Compares the decompiled body at the literal address in the 22284 IDB vs the 47031 IDB, and the
# comment tag on its source lines. stable = same function in both builds (address valid either way).
import json, os, re, difflib, collections
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SRV = r"C:\Users\drych\ProjectDrift\Solara\Server"
TOK = re.compile(r"[A-Za-z_][A-Za-z_0-9]*|[^\sA-Za-z_0-9]")
b22 = json.load(open(os.path.join(W, "bodies_22284all.json")))
a22 = json.load(open(os.path.join(W, "anchors_22284all.json")))
b47 = json.load(open(os.path.join(W, "bodies47.json")))
A47 = json.load(open(os.path.join(W, "anchors_47031.json")))
xs = [l.strip() for l in open(os.path.join(W, "live_funcs.txt")) if l.strip()]

tags = collections.defaultdict(set)
for fn in ("dllmain_50451.cpp", "specedit.h"):
    for l in open(os.path.join(SRV, fn), encoding="utf-8", errors="ignore"):
        code, _, cmt = l.partition("//")
        for h in re.findall(r'0x0*([0-9A-Fa-f]{5,8})', code):
            h = h.upper()
            if "TODO-47031 UNRESOLVED" in cmt: tags[h].add("TODO22")
            elif "[47031" in cmt or "47031]" in cmt: tags[h].add("47031")
            else: tags[h].add("untagged")

def sim(x, y):
    x, y = TOK.findall(x)[:4000], TOK.findall(y)[:4000]
    if not x or not y: return None
    return round(difflib.SequenceMatcher(None, x, y, autojunk=False).ratio(), 3)

out = {}
for h in xs:
    n22 = b22.get(h, {}).get("norm", ""); n47 = b47.get(h, {}).get("norm", "")
    s = sim(n22, n47)
    f22 = a22.get(h, {}).get("func"); f47 = A47.get(h, {}).get("func")
    verdict = ("stable" if s is not None and s >= 0.85 else
               "moved" if s is not None else "no-body")
    out[h] = {"sim22_47": s, "verdict": verdict, "tags": sorted(tags.get(h, [])),
              "f22": hex(f22) if f22 is not None else None, "f47": hex(f47) if f47 is not None else None,
              "sz22": a22.get(h, {}).get("size"), "sz47": A47.get(h, {}).get("size")}
json.dump(out, open(os.path.join(W, "provenance.json"), "w"), indent=0)
c = collections.Counter((v["verdict"], "+".join(v["tags"])) for v in out.values())
for k, n in sorted(c.items()): print(n, k)
print("\n== moved (address holds a different function in 22284 vs 47031):")
for h, v in sorted(out.items()):
    if v["verdict"] != "stable":
        print(" ", h, v["verdict"], v["sim22_47"], v["tags"], "sz22", v["sz22"], "sz47", v["sz47"])
