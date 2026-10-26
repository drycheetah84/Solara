#!/usr/bin/env python3
"""Turn new-map.json (real 47031 IDB match) into PORT-47031-RESULT.md + a
dllmain patch preview. Honest: groups by confidence, lists the manual short-list."""
import json

m = json.load(open("new-map.json"))
old = {s["name"]: s for s in json.load(open("old-ida.json"))["sites"]}
rows = m["results"]
order = {"high": 0, "med": 1, "low": 2, "todo-data": 3, "none": 4}
rows.sort(key=lambda r: (order.get(r["confidence"], 9), r["name"]))

w = [].append
L = []


def P(s):
    L.append(s)


P("# A2 22284 -> 47031 offset resolution (real IDB match)\n")
P(f"by confidence: {m['by_confidence']}\n")
P("Confidence: **high** = exact function-name or unique-string match (trust); "
  "**med** = string-intersection / signature+callee-overlap / single data-xref "
  "(verify in IDA); **low** = callee-search / multi-candidate (treat as a lead); "
  "**none** = unresolved, needs manual IDA.\n")
P("A clean boot does not prove a med/low value correct -- a wrong code RVA can "
  "corrupt silently. Bake only high + runtime-confirmed values.\n")
for conf in ("high", "med", "low", "todo-data", "none"):
    group = [r for r in rows if r["confidence"] == conf]
    if not group:
        continue
    P(f"## {conf}  ({len(group)})\n")
    P("| name | old | new | method | old name / lead |\n|---|---|---|---|---|")
    for r in group:
        lead = r.get("old_func", "")
        if conf in ("none",):
            o = old.get(r["name"], {})
            strs = o.get("strings", [])
            cl = [c["name"] for c in o.get("callees", []) if not c["name"].startswith("sub_")]
            lead = (("strings=" + ";".join(strs[:2])) if strs else "") + \
                   ((" callees=" + ",".join(cl[:3])) if cl else "") or lead or "(no leads)"
        P(f"| {r['name']} | {r['old']} | {r.get('new') or '-'} | {r.get('method') or '-'} | {lead[:70]} |")
    P("")
open("PORT-47031-RESULT.md", "w", encoding="utf-8").write("\n".join(L))
print("wrote PORT-47031-RESULT.md;", m["by_confidence"])
