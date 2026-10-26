"""Adds original -> Solara migration links to graphify-out/graph.json.

Run with the graphify interpreter (path in graphify-out/.graphify_python):
    tools/scripts/graph_migration_links.py

For every class/method node extracted from the original C# (reference/halcyon/...),
the Go node with the same name in the corresponding Go package is linked with a
`ported_to` edge (C# type, property and method names are preserved by
docs/PORTING_GUIDE.md). Original nodes get `migration_status` / `migration_evidence`
attributes; unmatched symbols are reported (and listed in
graphify-out/migration_unmatched.txt) for the coverage audit. Edges are stamped
`_origin: semantic` so `graphify update` (AST re-extraction) keeps them.
"""
import json
import re
from collections import defaultdict
from pathlib import Path

GRAPH = Path("graphify-out/graph.json")
MODULES = {  # original source-dir prefix -> Go dir prefixes
    "AUnrealFeatures.AAMothership/": ["internal/mothership/"],
    "AUnrealFeatures.Ares/": ["internal/ares/", "internal/host/", "cmd/solara/"],
    "AUnrealFeatures.EOSSDK/": ["internal/eos/"],
    "AUnrealFeatures.HalcyonSocket/": ["internal/halcyonsocket/"],
    "AUnrealFeatures.Hosting/": ["internal/hosting/", "internal/docdb/", "internal/logging/", "internal/host/", "internal/dotnet/", "internal/stj/"],
    "AUnrealFeatures.DependencyInjection/": ["internal/di/"],
}
STATUS = {  # evidence per module (see VALIDATION_REPORT.md)
    "internal/mothership/": ("compatibility-tested", "route table + 114 endpoints differential (anon+authed+flow) + LiteDB round-trip oracle"),
    "internal/ares/": ("compatibility-tested", "route tables + 139 endpoints differential + Drafters suite 24/24 + LiteDB round-trip oracle"),
    "internal/eos/": ("compatibility-tested", "route tables + 24 endpoints differential + .NET oracle fixtures + WS/STOMP tests"),
    "internal/halcyonsocket/": ("compatibility-tested", "route table + 5 endpoints differential + agent protocol replay vs oracle"),
    "internal/hosting/": ("unit-tested", "pipeline/result/binding tests + differential sweeps"),
    "internal/docdb/": ("unit-tested", "CRUD/collation/LiteDB fixture import tests"),
    "internal/di/": ("integration-tested", "exercised by host startup (e2e run)"),
}


def norm(label: str) -> str:
    return re.sub(r"[^a-z0-9]", "", label.lower())


def main():
    g = json.loads(GRAPH.read_text(encoding="utf-8"))
    nodes = g["nodes"]
    links = [e for e in g["links"] if e.get("relation") != "ported_to"]
    go_by = defaultdict(list)
    for n in nodes:
        sf = n.get("source_file", "")
        if sf.endswith(".go") and not sf.endswith("_test.go"):
            go_by[norm(n.get("label", ""))].append(n)
    matched, unmatched = 0, []
    for n in nodes:
        sf = n.get("source_file", "")
        if not (sf.startswith("reference/halcyon/backend/src/") and sf.endswith(".cs")):
            continue
        if not n.get("_callable"):
            continue
        rel = sf[len("reference/halcyon/backend/src/"):]
        targets = next((v for k, v in MODULES.items() if rel.startswith(k)), None)
        if not targets:
            continue
        cands = [m for m in go_by.get(norm(n.get("label", "")), []) if any(m["source_file"].startswith(t) for t in targets)]
        if not cands and len(norm(n.get("label", ""))) >= 6:
            key = norm(n.get("label", ""))
            cands = [m for k2, ms in go_by.items() if k2.endswith(key) for m in ms
                     if any(m["source_file"].startswith(t) for t in targets)]
        if not cands:
            unmatched.append(f"{rel}\t{n.get('label')}\t{n.get('source_location', '')}")
            n["migration_status"] = "unmatched-by-name"
            continue
        matched += 1
        tgt = cands[0]
        st = next((v for k, v in STATUS.items() if tgt["source_file"].startswith(k)), ("implemented", ""))
        n["migration_status"], n["migration_evidence"] = st
        n["ported_to"] = tgt["source_file"] + ":" + tgt.get("source_location", "")
        links.append({"source": n["id"], "target": tgt["id"], "relation": "ported_to", "_origin": "semantic",
                      "confidence": "INFERRED", "confidence_score": 0.9, "context": "migration",
                      "source_file": sf, "metadata": {"status": st[0]}})
    g["links"] = links
    GRAPH.write_text(json.dumps(g, ensure_ascii=False), encoding="utf-8")
    Path("graphify-out/migration_unmatched.txt").write_text("\n".join(sorted(unmatched)) + "\n", encoding="utf-8")
    print(f"linked {matched} original symbols; {len(unmatched)} unmatched by name (see graphify-out/migration_unmatched.txt)")


if __name__ == "__main__":
    main()
