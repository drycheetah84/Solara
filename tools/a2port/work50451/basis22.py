# basis22.py -- inputs for resolving the 22284-carried live RVAs directly 22284 -> 50451.
#  * rva22_targets.txt : the live RVAs whose source line says "TODO-47031 UNRESOLVED (kept 22284 value)"
#  * basis22.json      : 22284 rva -> 50451 rva, composed from verified "[47031 high] was 0x<22284>" pairs
#                        (22284 -> 47031) and the confirmed 47031 -> 50451 map (confirm2 + body-matches).
import json, os, re, glob
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SRV = r"C:\Users\drych\ProjectDrift\Solara\Server"

conf = {k.upper(): v["rva50"] for k, v in json.load(open(os.path.join(W, "confirm2.json")))["confirmed"].items()}
conf["41F5A60"] = "0x41f5a20"
for p in glob.glob(os.path.join(W, "bodymatch_f_*.jsonl")):
    for ln in open(p):
        try: r = json.loads(ln)
        except Exception: continue
        if r.get("status") == "body-match": conf.setdefault(r["rva47"].upper(), r["rva50"])

lit = re.compile(r'0x0*([0-9A-Fa-f]{5,8})')
targets, pairs22_47 = set(), {}
for fn in ("dllmain_47031.cpp", "specedit.h"):
    for l in open(os.path.join(SRV, fn), encoding="utf-8", errors="ignore"):
        code, _, cmt = l.partition("//")
        if "TODO-47031 UNRESOLVED" in cmt:
            for h in lit.findall(code): targets.add(h.upper())
        m = re.search(r'\[47031 high\] was 0x0*([0-9A-Fa-f]{5,8})', cmt)
        if m:
            v47 = lit.findall(code)
            if v47: pairs22_47[m.group(1).upper()] = v47[0].upper()

basis = {r22: conf[r47] for r22, r47 in pairs22_47.items() if r47 in conf}
open(os.path.join(W, "rva22_targets.txt"), "w").write("\n".join(sorted(targets)) + "\n")
json.dump(basis, open(os.path.join(W, "basis22.json"), "w"), indent=0)
open(os.path.join(W, "rva22_basis.txt"), "w").write("\n".join(sorted(basis)) + "\n")
print("22284-carried targets:", len(targets))
print("22284->47031 comment pairs:", len(pairs22_47), " composed to 50451:", len(basis))
