# strpairs.py -- dense cross-build address correspondence from unique string references.
# A string referenced by exactly one function in BOTH builds pairs those two functions. Pairs whose
# source function maps to conflicting targets are dropped, then pairs whose address delta disagrees
# with the local median delta of their neighbours are dropped (outlier rejection).
import json, os, sys, statistics
JOBS = [tuple(a.split(":")) for a in sys.argv[1:]] or [("22284all", "22", "50451"), ("47031", "47", "50451")]
W = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
def J(n): return json.load(open(os.path.join(W, n)))
for src, tag, dst in JOBS:
    S, D = J("strindex_%s.json" % src), J("strindex_%s.json" % dst)
    m = {}
    for s, fs in S.items():
        if len(fs) != 1 or len(s) < 8: continue
        fd = D.get(s)
        if not fd or len(fd) != 1: continue
        m.setdefault(fs[0], set()).add(fd[0])
    raw = sorted((a, next(iter(b))) for a, b in m.items() if len(b) == 1)
    keep = []
    K = 12
    for i, (a, b) in enumerate(raw):
        nb = [y - x for x, y in raw[max(0, i - K):i] + raw[i + 1:i + 1 + K]]
        if nb and abs((b - a) - statistics.median(nb)) <= 0x8000: keep.append([a, b])
    json.dump(keep, open(os.path.join(W, "strpairs_%s_%s.json" % (tag, dst[:2])), "w"))
    print(src, "->", dst, ": unique-string func pairs", len(raw), "after outlier rejection", len(keep))
