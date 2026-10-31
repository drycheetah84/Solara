# natpairs.py -- cross-build function pairs from UE native registration tables (natives_<tag>.json).
# 1) pair class tables by identical name sequence (unique on both sides), then by name-set Jaccard >= 0.75
#    (mutual best) for tables whose class gained/lost functions; 2) pair exec thunks by UFunction name;
# 3) pair impls: each thunk's specific callees (generic VM helpers called by many thunks removed),
#    paired positionally when both sides have the same count. Writes natpairs_<mode>_50.json.
import json, os, sys, collections
JOBS = [tuple(a.split(":")) for a in sys.argv[1:]] or [("22284all", "22", "50451"), ("47031", "47", "50451")]
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
def J(n): return json.load(open(os.path.join(W, n)))
def generic(arrs):
    c = collections.Counter()
    for a in arrs:
        for _, _, cl in a["e"]: c.update(set(cl))
    return {f for f, n in c.items() if n > 40}
for src, mode, dst in JOBS:
    D = J("natives_%s.json" % dst); gD = generic(D)
    S = J("natives_%s.json" % src); gS = generic(S)
    key = lambda a: tuple(n for n, _, _ in a["e"])
    bS, bD = collections.defaultdict(list), collections.defaultdict(list)
    for i, a in enumerate(S): bS[key(a)].append(i)
    for i, a in enumerate(D): bD[key(a)].append(i)
    apair, usedS, usedD = [], set(), set()
    for k, si in bS.items():
        di = bD.get(k)
        if di and len(si) == len(di):                  # equal multiplicity: address order within the group
            for x, y in zip(sorted(si, key=lambda i: S[i]["at"]), sorted(di, key=lambda i: D[i]["at"])):
                apair.append((x, y, "exact" if len(si) == 1 else "exact-dup")); usedS.add(x); usedD.add(y)
    nS = {i: set(key(S[i])) for i in range(len(S)) if i not in usedS}
    nD = {i: set(key(D[i])) for i in range(len(D)) if i not in usedD}
    inv = collections.defaultdict(set)
    for j, s in nD.items():
        for n in s: inv[n].add(j)
    def best(s, pool_inv, pool):
        sc = collections.Counter()
        for n in s:
            for j in pool_inv.get(n, ()): sc[j] += 1
        r = sorted(((c / len(s | pool[j]), j) for j, c in sc.items()), reverse=True)
        return r[0] if r else (0, None)
    invS = collections.defaultdict(set)
    for i, s in nS.items():
        for n in s: invS[n].add(i)
    for i, s in nS.items():
        jac, j = best(s, inv, nD)
        if j is None or jac < 0.75: continue
        jac2, i2 = best(nD[j], invS, nS)
        if i2 == i: apair.append((i, j, "fuzzy%.2f" % jac))
    pairs, kinds = {}, collections.Counter()
    for i, j, how in apair:
        dmap = {n: (f, cl) for n, f, cl in D[j]["e"]}
        for n, f, cl in S[i]["e"]:
            if n not in dmap: continue
            fd, cld = dmap[n]
            pairs.setdefault(f, (fd, how+"/exec")); kinds["exec"] += 1
            ss = [c for c in cl if c not in gS]; sd = [c for c in cld if c not in gD]
            if ss and len(ss) == len(sd):
                for a, b in zip(ss, sd): pairs.setdefault(a, (b, how+"/impl")); kinds["impl"] += 1
    json.dump(sorted([a, b[0], b[1]] for a, b in pairs.items()), open(os.path.join(W, "natpairs_%s_%s.json" % (mode, dst[:2])), "w"))
    print(src, "tables", len(S), "->", len(D), "| paired tables", collections.Counter(h.split("0.")[0] for _, _, h in apair),
          "| func pairs", len(pairs), dict(kinds))
