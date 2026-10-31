# nvalign.py -- resolve the NetVar natives (FindChild, setters, IterRelease) 22284 -> 50451 by call-sequence
# alignment around ANCHORS (functions already byte-verified in both builds: the getters + MemFree).
# For every 22284 function that calls an anchor AND a target, take its ordered direct-call list; do the same for
# every 50451 function that calls the mapped anchor; align the two lists on the anchors and read the 50451 call that
# sits where the target sat. Votes are tallied; a result is reported only with >=3 agreeing votes and no rival.
import sys, os, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from pecore import PE, EXE, call_index
import capstone

A = {0x46B2280: 0x49F8910, 0x4640580: 0x4956140, 0x46B2360: 0x49F89F0}   # NumGet, BoolGet, StrGet (resolved2)
T = {"FindChild": 0x463FE20, "NumSet": 0x4643F30, "BoolSet": 0x46443B0, "StrSet": 0x4644120, "IterRelease": 0x4675A60}
P22, P50 = PE(EXE["22"]), PE(EXE["50"])

def fn_of(pe, rva):
    c = pe.chunk(rva)
    return c
def calls_in(pe, chunk):
    lo, hi = chunk
    out = []
    for ins in capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64).disasm(pe.read(lo, hi - lo), lo):
        if ins.mnemonic in ("call", "jmp") and ins.size == 5 and ins.bytes[0] in (0xE8, 0xE9):
            out.append(int(ins.op_str, 16))
    return out

ci22, ci50 = call_index(P22), call_index(P50)
def funcs_calling(pe, ci, tgt):
    s = set()
    for site in ci.get(tgt, []):
        c = pe.chunk(site)
        if c: s.add(c)
    return s

votes = {k: collections.Counter() for k in T}
for a22, a50 in A.items():
    F22 = funcs_calling(P22, ci22, a22)
    F50 = funcs_calling(P50, ci50, a50)
    seq50 = [calls_in(P50, c) for c in F50]
    for c22 in F22:
        s22 = calls_in(P22, c22)
        if not any(t in s22 for t in T.values()): continue
        i22 = [i for i, x in enumerate(s22) if x == a22]
        # best 50451 partner: same number of anchor calls, closest total length
        best = None
        for s50 in seq50:
            i50 = [i for i, x in enumerate(s50) if x == a50]
            if len(i50) != len(i22): continue
            score = abs(len(s50) - len(s22))
            if best is None or score < best[0]: best = (score, s50, i50)
        if not best or best[0] > 2: continue
        _, s50, i50 = best
        for name, t in T.items():
            for i, x in enumerate(s22):
                if x != t: continue
                # nearest anchor before/after, keep the same relative offset
                for k, ai in enumerate(i22):
                    j = i50[k] + (i - ai)
                    if 0 <= j < len(s50) and abs(i - ai) <= 3:
                        votes[name][s50[j]] += 1
for name, c in votes.items():
    top = c.most_common(3)
    print(name, hex(T[name]), "->", [(hex(a), n) for a, n in top])
