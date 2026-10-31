# applyfields.py -- verify agent field classifications against the SDK dumps, then rewrite stale offsets.
# Input: fieldclaims.json (array of {line, literal, class, field, ...}) from the per-range classifiers.
# A claim is APPLIED only if:  the parsed SDK really has class.field at `literal` in 22284 or 47031 (provenance),
# the 50451 SDK has the same class.field, the literal occurs on that source line, and confidence != low.
# Usage: py -3 -I applyfields.py <claims.json> [--write]
import json, re, sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "fieldaudit.py")).read().split("src = sys.argv[1]")[0])
SRC = r"C:\Users\drych\ProjectDrift\Solara\Server\dllmain_50451.cpp"
for _a in sys.argv[2:]:
    if _a.startswith("--src="): SRC = _a[6:]   # e.g. specedit.h (shared by builds: see the #if guard below)
claims = json.load(open(sys.argv[1]))
lines = open(SRC, encoding="utf-8", newline="").read().split("\n")
applied, already, rejected = [], [], []
edits = {}
for c in claims:
    if c.get("class") in (None, "?") or c.get("field") in (None, "?"):
        rejected.append((c, "unclassified")); continue
    lit = int(c["literal"], 16) if str(c["literal"]).lower().startswith("0x") else int(c["literal"])
    if c.get("field") == "sizeof":   # struct size (stride / buffer), from the dump's "// 0xSIZE (..)" header
        v = {b: next((SIZE[b][k] for k in strip(c["class"]) if k in SIZE[b]), None) for b in S}
    elif c.get("delta_of"):   # e.g. TArray Num = field + 8
        v = {b: (lambda x: None if x is None else x + c["delta"])(get(b, c["class"], c["delta_of"])) for b in S}
    else:
        v = {b: path(b, c["class"], c["field"]) for b in S}
        if c.get("old_field"):   # renamed field: provenance from the old name in the old builds
            for b in ("22284", "47031"):
                if v[b] is None: v[b] = path(b, c["class"], c["old_field"])
    n = v["50451"]
    if n is None: rejected.append((c, "no 50451 field")); continue
    if lit == n: already.append(c); continue
    if lit not in (v["22284"], v["47031"]): rejected.append((c, f"literal not the field offset in 22284/47031 ({v})")); continue
    if c.get("confidence") == "low": rejected.append((c, "low confidence")); continue
    i = c["line"] - 1
    pat = re.compile(r'(?<![0-9A-Za-z_])(0x0*%X)(?![0-9A-Fa-f])' % lit, re.I)
    code = lines[i].split("//", 1)[0]
    if not pat.search(code): rejected.append((c, "literal not in code part of line")); continue
    edits.setdefault(i, []).append((pat, lit, n, c))
for i, es in edits.items():
    code, sep, cmt = lines[i].partition("//")
    seen = {}
    for k, (pat, lit, n, c) in enumerate(es):   # placeholders first: 0x2B0->0x310 and 0x310->0x370 on one line must not chain
        if lit in seen: continue
        seen[lit] = n
        code = pat.sub("@@%d@@" % k, code)
        applied.append((c["line"], c["class"], c["field"], hex(lit), hex(n)))
    for k, (pat, lit, n, c) in enumerate(es):
        code = code.replace("@@%d@@" % k, "0x%X" % n)
    tag = " [50451 offs]" if "[50451 offs]" not in cmt else ""
    new = code + sep + cmt + (tag if sep else ("  //" + tag if es else ""))
    if SRC.lower().endswith("specedit.h"):   # shared by 22284/47031/50451: keep the original line for the others
        eol = "\r" if lines[i].endswith("\r") else ""
        new = new.rstrip("\r")
        lines[i] = "#if HALCYON_GAME_BUILD == 50451" + eol + "\n" + new + eol + "\n#else" + eol + "\n" + lines[i].rstrip("\r") + eol + "\n#endif" + eol
    else:
        lines[i] = new
print("applied", len(applied), "already-ok", len(already), "rejected", len(rejected))
for a in applied: print("  APPLY", *a)
for c, why in rejected: print("  REJECT", c.get("line"), c.get("class"), c.get("field"), c.get("literal"), "--", why)
if "--write" in sys.argv:
    open(SRC, "w", encoding="utf-8", newline="").write("\n".join(lines))
    print("written")
