# nvalign3.py -- resolve 22284 functions -> 50451 by instruction-stream alignment of their CALLERS, using any
# already-verified function pair as an anchor to find each caller's 50451 counterpart.
# For every 22284 caller C22 of a target: the anchors C22 also calls give a candidate set (50451 callers of the
# mapped anchors); the most similar candidate (normalised mnemonic stream, difflib >= 0.80) is aligned with C22 and
# the 50451 call opposite the target call gets a vote.  Usage: nvalign3.py Name=22rva [Name=22rva ...]
import sys, os, re, difflib, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from pecore import PE, EXE, call_index

ANCH = {  # 22284 -> 50451, each verified (insn-identical or decompile-checked)
    0x463FE20: 0x49558F0,  # FindChild
    0x465BE70: 0x4974800,  # NodeRemove
    0x46BC8C0: 0x4A01C20,  # NodeFinish
    0x463C090: 0x49519A0,  # AddChild
    0x47236C0: 0x4A902F0,  # NodeDesc
    0x464B970: 0x4961710,  # ContainerCtor
    0x46D50F0: 0x49E9590,  # Templates
    0x464C150: 0x4961D40,  # LeafCtor
    0x46BC830: 0x4A6A300,  # LeafStat
    0x5361530: 0x55371A0,  # Malloc (decompiled setters allocate through it)
    0x4675A60: 0x498E770,  # IterRelease
}
T = {a.split("=")[0]: int(a.split("=")[1], 16) for a in sys.argv[1:]}
P22, P50 = PE(EXE["22"]), PE(EXE["50"])
ci22, ci50 = call_index(P22), call_index(P50)

def stream(pe, c):
    out = []
    for a, n, b, m, ins in pe.insns(c[0], c[1]):
        if a >= c[1]: break
        if ins.mnemonic in ("call", "jmp") and n == 5 and b[0] in (0xE8, 0xE9):
            out.append(("CALL", int(ins.op_str, 16)))
        else:
            out.append((ins.mnemonic + " " + re.sub(r'0x[0-9a-f]{4,}', 'K', ins.op_str), None))
    return out
def funcs(pe, ci, t):
    return {pe.chunk(s) for s in ci.get(t, []) if pe.chunk(s)}

cache50 = {}
def s50(c):
    if c not in cache50: cache50[c] = stream(P50, c)
    return cache50[c]

votes = {k: collections.Counter() for k in T}
for name, t in T.items():
    for c22 in funcs(P22, ci22, t):
        if c22[1] - c22[0] >= 0x4000: continue
        s22 = stream(P22, c22); k22 = [x[0] for x in s22]
        cands = set()
        for x in s22:
            if x[1] in ANCH: cands |= funcs(P50, ci50, ANCH[x[1]])
        best = None
        for c50 in cands:
            if c50[1] - c50[0] >= 0x4000: continue
            st = s50(c50)
            if abs(len(st) - len(s22)) > max(10, len(s22) // 3): continue
            r = difflib.SequenceMatcher(None, k22, [x[0] for x in st], autojunk=False).ratio()
            if best is None or r > best[0]: best = (r, c50, st)
        if not best or best[0] < 0.80: continue
        r, c50, st = best
        sm = difflib.SequenceMatcher(None, k22, [x[0] for x in st], autojunk=False)
        for tag, i1, i2, j1, j2 in sm.get_opcodes():
            if tag != "equal": continue
            for k in range(i2 - i1):
                if s22[i1 + k][1] == t and st[j1 + k][1] is not None:
                    votes[name][st[j1 + k][1]] += 1
for name, c in votes.items():
    print(name, hex(T[name]), "->", [(hex(a), n) for a, n in c.most_common(4)])
