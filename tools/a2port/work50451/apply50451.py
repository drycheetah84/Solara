# apply50451.py -- write resolved2.json into the 50451 sources.
#   verified literal   -> its 50451 value
#   unverified literal -> HX_UNRES(0xOLD)  (== 0; the DLL's hook/patch helpers refuse it, see dllmain_50451.cpp)
#   bounds/masks        -> untouched
# dllmain_50451.cpp is 50451-only and is edited in place. specedit.h is shared with the 22284/47031 builds,
# so every changed line is emitted as  #if HALCYON_GAME_BUILD == 50451 <new> #else <old> #endif.
# Only code is rewritten: // comments and "string literals" keep their original text.
import json, os, re, collections
W = os.path.dirname(os.path.abspath(__file__))
SRC = r"C:\Users\drych\ProjectDrift\Solara\Server"
R = json.load(open(os.path.join(W, "resolved2.json")))
MANUAL = {"12B9074": "0x12be784",   # end of FName::ToString guard fn 12B8EF0->12BE600 (identical 0x184-byte body)
          "4A89100": "0x4a92080"}   # end of sub_4A88D90 range -> 4A91D10+0x370 (byte-matched, resolve_all)
VAL = {}
for h, r in R.items():
    if r["kind"] == "bound": continue
    VAL[int(h, 16)] = MANUAL.get(h) or r["rva50"]
hx = re.compile(r'0x0*([0-9A-Fa-f]{6,8})\b')
def rewrite_code(code, stats):
    out, pos = [], 0
    for m in re.finditer(r'"(?:\\.|[^"\\])*"', code):
        out.append(sub_seg(code[pos:m.start()], stats)); out.append(m.group(0)); pos = m.end()
    out.append(sub_seg(code[pos:], stats))
    return "".join(out)
def sub_seg(seg, stats):
    def rep(m):
        v = int(m.group(1), 16)
        if v not in VAL: return m.group(0)
        nv = VAL[v]
        if nv is None: stats["gated"] += 1; return "HX_UNRES(0x%X)" % v
        stats["mapped"] += 1; return "0x%X" % int(nv, 16)
    return hx.sub(rep, seg)
def split_comment(line):
    # first // outside a string literal
    i, n, ins = 0, len(line), False
    while i < n:
        c = line[i]
        if c == '"' and (i == 0 or line[i - 1] != '\\'): ins = not ins
        elif not ins and line.startswith("//", i): return line[:i], line[i:]
        i += 1
    return line, ""
report = {}
for fn in ("dllmain_50451.cpp", "specedit.h"):
    p = os.path.join(SRC, fn)
    lines = open(p, encoding="utf-8", newline="").read().split("\n")
    stats = collections.Counter(); out = []
    for l in lines:
        code, cm = split_comment(l)
        nc = rewrite_code(code, stats)
        if nc == code: out.append(l); continue
        stats["lines"] += 1
        if fn == "specedit.h":
            out += ["#if HALCYON_GAME_BUILD == 50451", nc + cm, "#else", l, "#endif"]
        else:
            out.append(nc + cm)
    open(p, "w", encoding="utf-8", newline="").write("\n".join(out))
    report[fn] = dict(stats)
print(report)
