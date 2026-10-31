# consolidate.py -- one decision per live literal from every evidence source, with conflicts flagged.
# Evidence (strongest first): native (UE registration table, keyed by UFunction name) > graph-match
# (call-graph propagation, validated 37/37) and body-match (margin-checked) > body-twin (only if another
# source picks the same twin). Old confirm2 entries count only if re-verified (mode 47c) or corroborated.
import json, os, glob, collections
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
def J(n): return json.load(open(os.path.join(W, n)))
def jl(pat):
    out = {}
    for p in glob.glob(os.path.join(W, pat)):
        for ln in open(p):
            r = json.loads(ln); out[r.get("rva47", r.get("rva"))] = r
    return out
L = J("live_audit.json"); P = J("provenance_final.json")
ev = {"47": collections.defaultdict(list), "22": collections.defaultdict(list)}
for h, r in jl("bodymatch_f_*.jsonl").items():
    if r["status"] in ("body-match", "body-twin"): ev["47"][h].append((r["status"], r["rva50"], r.get("twins")))
for h, r in jl("bodymatch_f47c_*.jsonl").items():
    if r["status"] in ("body-match", "body-twin"): ev["47"][h].append(("recheck-" + r["status"], r["rva50"], r.get("twins")))
for h, r in jl("bodymatch_f22_*.jsonl").items():
    if r["status"] in ("body-match", "body-twin"): ev["22"][h].append((r["status"], r["rva50"], r.get("twins")))
for t, m in (("47031", "47"), ("22284all", "22")):
    for ln in open(os.path.join(W, "graphmatch_%s.jsonl" % t)):
        r = json.loads(ln)
        if r["status"] in ("native", "graph-match"): ev[m][r["rva"]].append((r["status"], r["rva50"], None))
for h, c in J("confirm2.json")["confirmed"].items(): ev["47"][h].append(("confirm2", c["rva50"], None))

STRONG = {"native", "graph-match", "body-match", "recheck-body-match"}
def decide(es):
    if not es: return None, "none", []
    nat = [v for s, v, _ in es if s == "native"]
    if nat: return nat[0], "native", es
    strong = collections.Counter(v.lower() for s, v, _ in es if s in STRONG)
    if len(strong) > 1: return None, "CONFLICT", es
    if strong:
        v = next(iter(strong)); n = strong[v]
        return v, ("corroborated" if n >= 2 or any(s == "confirm2" and x.lower() == v for s, x, _ in es) else "single"), es
    twins = [(v, tw) for s, v, tw in es if s.endswith("body-twin")]
    c2 = [v.lower() for s, v, _ in es if s == "confirm2"]
    if twins and c2 and c2[0] in [t.lower() for t in (twins[0][1] or [])]: return c2[0], "twin+confirm2", es
    return None, "unverified", es

out, summary = {}, collections.Counter()
for h, v in sorted(L.items()):
    if v["state"] == "bound": continue
    prov = P.get(h, "47031" if v["state"] != "data-global" else "data")
    if v["state"] == "data-global": prov = "data"
    if prov == "MANUAL": out[h] = {"prov": "manual", "decision": "manual"}; summary["manual (patch site/range)"] += 1; continue
    if prov == "data": out[h] = {"prov": "data", "decision": "data"}; summary["data global"] += 1; continue
    m = "22" if prov == "22284" else "47"
    val, how, es = decide(ev[m].get(h, []))
    out[h] = {"prov": prov, "rva50": val, "decision": how, "evidence": [[s, x] for s, x, _ in es]}
    summary["%s:%s" % (prov, how)] += 1
json.dump(out, open(os.path.join(W, "final_live.json"), "w"), indent=1)
for k, n in sorted(summary.items()): print("%4d  %s" % (n, k))
print("\nCONFLICTS / UNRESOLVED (function literals):")
for h, r in sorted(out.items()):
    if r.get("decision") in ("CONFLICT", "unverified", "none"):
        print(" ", h, r["prov"], r["decision"], r.get("evidence"))
