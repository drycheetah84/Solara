# nvalign2.py -- resolve the NetVar setters/IterRelease 22284 -> 50451 by instruction-stream alignment.
# FindChild is byte-verified (22284 0x463FE20 -> 50451 0x49558F0: the only twin carrying `mov byte [rdi+48h],1`).
# For each 22284 function that calls a target, pick the most similar 50451 function among FindChild's callers
# (normalised mnemonic+operand stream, difflib), align the streams, and read the 50451 call opposite the target
# call. Report per target the vote tally; accept only a clear majority with >= 3 votes.
import sys, os, re, difflib, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from pecore import PE, EXE, call_index

FIND22, FIND50 = 0x463FE20, 0x49558F0
T = {"NumSet": 0x4643F30, "BoolSet": 0x46443B0, "StrSet": 0x4644120, "IterRelease": 0x4675A60}
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

cand50 = [(c, stream(P50, c)) for c in funcs(P50, ci50, FIND50) if c[1] - c[0] < 0x3000]
votes = {k: collections.Counter() for k in T}
for name, t in T.items():
    for c22 in funcs(P22, ci22, t):
        if FIND22 not in [x[1] for x in stream(P22, c22)] and name != "IterRelease": continue
        if c22[1] - c22[0] >= 0x3000: continue
        s22 = stream(P22, c22); k22 = [x[0] for x in s22]
        best = None
        for c50, s50 in cand50:
            if abs(len(s50) - len(s22)) > max(8, len(s22) // 4): continue
            r = difflib.SequenceMatcher(None, k22, [x[0] for x in s50], autojunk=False).ratio()
            if best is None or r > best[0]: best = (r, c50, s50)
        if not best or best[0] < 0.80: continue
        r, c50, s50 = best
        sm = difflib.SequenceMatcher(None, k22, [x[0] for x in s50], autojunk=False)
        for tag, i1, i2, j1, j2 in sm.get_opcodes():
            if tag != "equal": continue
            for k in range(i2 - i1):
                if s22[i1 + k][1] == t and s50[j1 + k][1] is not None:
                    votes[name][s50[j1 + k][1]] += 1
for name, c in votes.items():
    print(name, hex(T[name]), "->", [(hex(a), n) for a, n in c.most_common(4)])
