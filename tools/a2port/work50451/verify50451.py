# verify50451.py -- static audit of the applied 50451 sources (run after apply50451.py).
# Every image-range hex literal in 50451 code (dllmain_50451.cpp + the 50451 branches of specedit.h) must be
# a verified 50451 value (resolved2.json / apply MANUAL / signature-found) or a known non-RVA bound.
# Anything else is an un-ported old value -> FAIL. Also re-checks known patch-site bytes in the 50451 PE.
import json, os, re, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from pecore import PE, EXE
W = os.path.dirname(os.path.abspath(__file__)); SRC = r"C:\Users\drych\ProjectDrift\Solara\Server"
R = json.load(open(os.path.join(W, "resolved2.json")))
ok = {int(r["rva50"], 16) for r in R.values() if r.get("rva50")}
ok |= {0x12BE784, 0x4A92080,                                    # apply MANUAL (fn-end / range-end, byte-matched)
       0x53D9DE4, 0x53D9DE7, 0x57942B5, 0x57942BF, 0x57942D4}   # unique signature hits (entitlement, ball-spawner)
BOUNDS = {0x1000000, 0x5000000, 0x5A00000, 0xC000000, 0xA5BB000}
hx = re.compile(r'0x0*([0-9A-Fa-f]{6,8})\b')
bad = []; n = 0
def scan(fn, lines):
    global n
    for i, l in enumerate(lines):
        code = re.sub(r'"(?:\\.|[^"\\])*"', '""', l.split("//", 1)[0])
        code = re.sub(r'HX_UNRES\(0x[0-9A-Fa-f]+\)', '0', code)
        for m in hx.finditer(code):
            v = int(m.group(1), 16)
            if not (0x1000000 <= v < 0xC000000) or v in BOUNDS: continue
            n += 1
            if v not in ok: bad.append((fn, i + 1, hex(v), l.strip()[:100]))
scan("dllmain_50451.cpp", open(os.path.join(SRC, "dllmain_50451.cpp"), encoding="utf-8", errors="ignore").read().split("\n"))
# specedit.h: only the lines compiled for 50451 (outside #else branches of 50451 conditionals)
sl = open(os.path.join(SRC, "specedit.h"), encoding="utf-8", errors="ignore").read().split("\n")
keep, state = [], []
for l in sl:
    t = l.strip()
    if t.startswith("#if HALCYON_GAME_BUILD == 50451"): state.append("in"); keep.append(""); continue
    if t.startswith("#if"): state.append("other")
    elif t.startswith("#else") and state and state[-1] == "in": state[-1] = "else"; keep.append(""); continue
    elif t.startswith("#endif") and state:
        s = state.pop(); keep.append("")
        if s in ("in", "else"): continue
        continue
    keep.append("" if "else" in state else l)
scan("specedit.h", keep)
P = PE(EXE["50"])
sig = {0x53D9DE4: bytes([0x40, 0x84, 0xED, 0x75, 0x59, 0x80, 0x3D]), 0x57942B5: bytes([0x75, 0x1E, 0x80, 0x3D]),
       0x9317DC0: b"https://api.oriondrift.net\0"}
sigbad = [hex(a) for a, b in sig.items() if P.read(a, len(b)) != b]
print("50451 image literals checked:", n, " un-ported:", len(bad), " signature mismatches:", sigbad)
for b in bad: print("  UNPORTED", *b)
sys.exit(1 if bad or sigbad else 0)
