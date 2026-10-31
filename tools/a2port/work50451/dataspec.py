# dataspec.py -- build the 50451 resolution spec for data globals and in-function patch sites.
# For each address: its source build, the source functions that reference it (or contain it), and those
# functions' 50451 counterparts from the cross-build maps (native names > resolved > unique strings).
import json, os, glob
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
def J(n): return json.load(open(os.path.join(W, n)))
X = {"22": J("datax_22284.json"), "47": J("datax_47031all.json")}

def gmap(mode):
    M = {}
    for s, d in J("strpairs_%s_50.json" % mode): M[s] = d
    tag, af = ("47031", "anchors_47031.json") if mode == "47" else ("22284all", "anchors_22284all.json")
    A = J(af)
    for ln in open(os.path.join(W, "graphmatch_%s.jsonl" % tag)):
        r = json.loads(ln)
        if r["status"] in ("native", "graph-match") and A.get(r["rva"], {}).get("func") is not None:
            M[A[r["rva"]]["func"]] = int(r["func50"], 16)
    for s, d, h in J("natpairs_%s_50.json" % mode):
        if not h.startswith("exact-dup/exec"): M[s] = d
    return M
MAPS = {"22": gmap("22"), "47": gmap("47")}

# source build of each address, from the 22284-vs-47031 inspection (instruction/comment/reference evidence)
PROV = {"12B9074": "47", "54277AD": "22", "54B13D0": "22", "54B1421": "22", "804BDA0": "22", "8D0A1F8": "47",
        "A480348": "47", "A4810D4": "47", "A6D3882": "47", "A6D3883": "47", "AC0A1A0": "47", "FCC220": "22",
        "84A2B38": "22", "84E6D20": "22", "95DF980": "22", "95DF988": "22", "95DF98C": "22", "9710B80": "22",
        "1254F50": "22", "1326B07": "22", "53B3BC3": "22", "53B3BCD": "22", "53B3BE2": "22", "53DC76A": "22",
        "5429424": "22", "5429427": "22", "12F0CB0": "22", "54E17E0": "22", "54E4E10": "22",
        "924A700": "?", "9C5D0A0": "?"}
spec = {}
for h, p in sorted(PROV.items()):
    for b in (("22", "47") if p == "?" else (p,)):
        r = X[b].get(h)
        if not r: continue
        M = MAPS[b]
        e = {"src": b, "seg": r.get("seg"), "name": r.get("name"), "string": r.get("string"), "refs": []}
        f = r.get("in_func")
        if f:
            e["in_func"] = {"func": f["func"], "off": f["off"], "size": f["size"], "end": f["end"],
                            "bytes": f["site_bytes"], "insn": f["insn"], "func50": M.get(f["func"])}
        for x in r.get("refs", []):
            e["refs"].append({"func": x["func"], "site_off": x["site"] - x["func"], "mnem": x["mnem"],
                              "ord": x["ordinal"], "nd": x["ndrefs"], "func50": M.get(x["func"])})
        spec[h + ("" if p != "?" else "@" + b)] = e
json.dump(spec, open(os.path.join(W, "dataspec.json"), "w"), indent=1)
for k, e in spec.items():
    f = e.get("in_func")
    print("%-12s src=%s seg=%-6s refs=%d mapped=%d in_func=%s" % (
        k, e["src"], e["seg"], len(e["refs"]), sum(1 for x in e["refs"] if x["func50"] is not None),
        ("mapped" if f and f["func50"] is not None else ("UNMAPPED" if f else "-"))))
