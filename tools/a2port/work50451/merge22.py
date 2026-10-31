# merge22.py -- combine 22284->50451 direct (xloc_d22) with the 22284->47031->50451 two-hop (xloc_h22 +
# the 47031->50451 maps). Agreement = strong; single route = accepted; disagreement = CONFLICT.
# Also lists hop-1 47031 functions that still need a 47031->50451 lookup (t47h.txt).
import json, os, glob, collections
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
def J(n): return json.load(open(os.path.join(W, n)))
def jl(n):
    p = os.path.join(W, n)
    return {json.loads(l)["src"]: json.loads(l) for l in open(p)} if os.path.exists(p) else {}
OK = ("native", "graph", "window", "both")
M47 = {}
for s, d in J("strpairs_47_50.json"): M47[s] = d
A47 = J("anchors_47031.json")
for ln in open(os.path.join(W, "graphmatch_47031.jsonl")):
    r = json.loads(ln)
    if r["status"] in ("native", "graph-match") and A47.get(r["rva"], {}).get("func") is not None:
        M47[A47[r["rva"]]["func"]] = int(r["func50"], 16)
G47x = J("graph_47031x.json")
for h, r in jl("xloc_d47x.jsonl").items():
    if r["status"] in OK and G47x.get(h, {}).get("func") is not None: M47[G47x[h]["func"]] = int(r["func"], 16)
if os.path.exists(os.path.join(W, "graph_47031h.json")):
    G47h = J("graph_47031h.json")
    for h, r in jl("xloc_d47h.jsonl").items():
        if r["status"] in OK and G47h.get(h, {}).get("func") is not None: M47[G47h[h]["func"]] = int(r["func"], 16)
for s, d, how in J("natpairs_47_50.json"):
    if not how.startswith("exact-dup/exec"): M47[s] = d

G = J("graph_22284x.json"); D = jl("xloc_d22.jsonl"); H = jl("xloc_h22.jsonl")
out, need, c = {}, set(), collections.Counter()
for h, g in sorted(G.items()):
    if g.get("func") is None: continue
    d = D.get(h, {}); hh = H.get(h, {})
    f_direct = int(d["func"], 16) if d.get("status") in OK else None
    f47 = int(hh["func"], 16) if hh.get("status") in OK else None
    f_hop = M47.get(f47) if f47 is not None else None
    if f47 is not None and f_hop is None: need.add("%X" % f47)
    if f_direct is not None and f_hop is not None:
        st = "agree" if f_direct == f_hop else "CONFLICT"
    elif f_direct is not None: st = "direct"
    elif f_hop is not None: st = "hop"
    else: st = "none"
    f50 = f_direct if st in ("agree", "direct") else f_hop if st == "hop" else None
    out[h] = {"status": st, "func50": hex(f50) if f50 is not None else None,
              "rva50": hex(f50 + g["off"]) if f50 is not None else None,
              "direct": d.get("status"), "f47": hex(f47) if f47 is not None else None,
              "hop": hex(f_hop) if f_hop is not None else None, "direct_f": hex(f_direct) if f_direct is not None else None}
    c[st] += 1
json.dump(out, open(os.path.join(W, "merged22.json"), "w"), indent=1)
open(os.path.join(W, "t47h.txt"), "w").write("\n".join(sorted(need)) + "\n")
print(dict(c), "| hop-1 47031 funcs needing 47->50:", len(need))
for h, r in out.items():
    if r["status"] == "CONFLICT": print("  CONFLICT", h, r)
