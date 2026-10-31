# fieldaudit.py -- audit raw struct offsets in dllmain_50451.cpp against the 22284/47031/50451 SDK dumps.
# Every line holding a hex offset AND a `Class::Field` mention (code or comment) is checked: if the literal equals
# that field's 22284 or 47031 offset but not its 50451 offset, it is stale.
import re, glob, json, sys, collections
SD = r"C:\Users\drych\ProjectDrift\Solara\Server\gamesdk"
PAR = {}; TYP = {}; SIZE = {}
def load(b):
    d = collections.defaultdict(dict); PAR[b] = {}; TYP[b] = collections.defaultdict(dict); SIZE[b] = {}
    for fn in glob.glob(rf"{SD}\{b}\SDK\*_classes.hpp") + glob.glob(rf"{SD}\{b}\SDK\*_structs.hpp"):
        cur = None; lastsz = None
        for l in open(fn, encoding="utf-8", errors="ignore"):
            msz = re.match(r'// (0x[0-9A-Fa-f]+) \(0x[0-9A-Fa-f]+ - 0x[0-9A-Fa-f]+\)', l)
            if msz: lastsz = int(msz.group(1), 16)
            m = re.match(r'(?:class|struct)\s+(?:(?:SDK_ALIGN|alignas)\(0x\w+\)\s+)?(\w+)\b(?!;)', l)
            if m and not l.rstrip().endswith(";"):
                cur = m.group(1)
                if lastsz is not None: SIZE[b].setdefault(cur, lastsz); lastsz = None
                pm = re.search(r':\s*public\s+(\w+)', l)
                if pm: PAR[b][cur] = pm.group(1)
            m = re.search(r'\s(\w+)(?:\[\w+\])?(?:\s*:\s*\d+)?;\s+// (0x[0-9A-F]+)\(', l)
            if m and cur:
                d[cur].setdefault(m.group(1), int(m.group(2), 16))
                tm = re.match(r'\s*(?:struct\s+|class\s+)?(?:TArray<\s*)?(?:struct\s+|class\s+)?(\w+)', l)
                if tm: TYP[b][cur].setdefault(m.group(1), tm.group(1))
    return d
S = {b: load(b) for b in ("22284", "47031", "50451")}
def strip(c):  # UE prefix-less lookup: AVRPawn / VRPawn / UA2Foo / FBar
    return [c, "A" + c, "U" + c, "F" + c] if c[:1] not in "AUF" or not c[1:2].isupper() else [c, c[1:]]
def get(b, c, f):
    for k in strip(c):
        seen = 0
        while k and seen < 30:
            if k in S[b]:
                if f in S[b][k]: return S[b][k][f]
                for ff, o in S[b][k].items():
                    if ff.lower() == f.lower(): return o
            k = PAR[b].get(k); seen += 1
    return None
def typ(b, c, f):
    for k in strip(c):
        seen = 0
        while k and seen < 30:
            if k in TYP[b]:
                for ff, t in TYP[b][k].items():
                    if ff.lower() == f.lower(): return t
            k = PAR[b].get(k); seen += 1
    return None
def path(b, c, p):
    """Offset of a dotted member path; trailing Num/Data = TArray count/data; X/Y/Z = FVector doubles."""
    off, cur = 0, c
    for seg in p.split("."):
        if seg == "Num": off += 8; continue
        if seg == "Data": continue
        if seg in ("X", "Y", "Z") and cur in ("FVector", "Vector"): off += {"X": 0, "Y": 8, "Z": 16}[seg]; continue
        o = get(b, cur, seg)
        if o is None: return None
        off += o; cur = typ(b, cur, seg)
    return off
src = sys.argv[1]
lines = open(src, encoding="utf-8", errors="ignore").read().split("\n")
stale, ok, unk = [], 0, 0
for i, l in enumerate(lines):
    hx = [int(h, 16) for h in re.findall(r'0x([0-9A-Fa-f]{2,5})\b', l)]
    refs = re.findall(r'\b([AUF]?[A-Z]\w+)::(\w+)\b', l)
    if not hx or not refs: continue
    for c, f in refs:
        v = {b: get(b, c, f) for b in S}
        if v["50451"] is None: continue
        for h in hx:
            if h == v["50451"]: ok += 1
            elif h in (v["22284"], v["47031"]):
                stale.append((i + 1, c, f, hex(h), {b: (hex(x) if x is not None else None) for b, x in v.items()}, l.strip()[:140]))
print("ok", ok, "stale", len(stale))
for s in stale: print(*s)
