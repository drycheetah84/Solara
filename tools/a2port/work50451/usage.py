# usage.py -- for each given literal: every source line using it, the enclosing C++ function, and the
# nearest enclosing `if (...)` gate, so unresolved items can be understood semantically.
import re, sys, os
SRV = r"C:\Users\drych\ProjectDrift\Solara\Server"
lits = [a.upper() for a in sys.argv[1:]]
lines = open(os.path.join(SRV, "dllmain_50451.cpp"), encoding="utf-8", errors="ignore").read().split("\n")
fnre = re.compile(r'^(?:static\s+|inline\s+)*[\w:<>\*&\s]+?\b(\w+)\s*\([^;]*\)\s*(?:noexcept\s*)?\{?\s*$')
def enclosing(i):
    for j in range(i, max(0, i - 400), -1):
        l = lines[j]
        if l and not l.startswith((" ", "\t", "/", "#", "}")) and "(" in l and not l.rstrip().endswith(";"):
            m = fnre.match(l.strip())
            if m: return m.group(1), j + 1
    return "?", 0
def gate(i):
    depth = 0
    for j in range(i, max(0, i - 120), -1):
        depth += lines[j].count("}") - lines[j].count("{")
        if depth < 0:
            m = re.search(r'\bif\s*\((.{0,90})', lines[j])
            if m: return m.group(1).strip()
            depth = 0
    return ""
names = {}
for i, l in enumerate(lines):
    code = l.split("//")[0]
    m = re.search(r'(\w+_RVA)\s*=\s*0x0*([0-9A-Fa-f]{5,8})', code)
    if m: names[m.group(2).upper()] = m.group(1)
for h in lits:
    ids = [h] + ([names[h]] if h in names else [])
    print("=====", h, ("(" + names[h] + ")") if h in names else "")
    for i, l in enumerate(lines):
        code = l.split("//")[0]
        if any(re.search(r'0x0*' + h + r'\b', code, re.I) if x == h else re.search(r'\b' + x + r'\b', code) for x in ids):
            fn, at = enclosing(i)
            print("  L%-6d in %-34s gate[%s]  %s" % (i + 1, fn, gate(i)[:60], l.strip()[:120]))
