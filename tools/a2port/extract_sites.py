#!/usr/bin/env python3
r"""extract_sites.py - pull every build-specific address out of dllmain.cpp.

Two kinds are emitted into a port-sites JSON the porter understands:

  * named constants   static constexpr uintptr_t NAME_RVA = 0x...;   (code or data)
  * inline call sites  reinterpret_cast<...>(base + 0x....)(...)  / base + 0x.... reads

Data-vs-func is guessed from the name/comment (Str/Ptr/Log/GIs*/Pool/Tbl/Data/Num/Max
-> data, resolved via IDA xrefs; everything else -> func). The guess only decides how
the porter treats it; every result is verified in IDA regardless.

    python extract_sites.py <dllmain.cpp> -o sites.json
"""
import argparse
import json
import re

DATA_HINT = re.compile(r"(GIs|Str_|_Str|Ptr|Log[A-Z]|Pool|Tbl|_Data|_Num|_Max|StationStr|Client)", re.I)

CONST_RE = re.compile(
    r"constexpr\s+uintptr_t\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(0x[0-9A-Fa-f]+)\s*;([^\n]*)"
)
INLINE_RE = re.compile(r"base\s*\+\s*(0x[0-9A-Fa-f]+)")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("src")
    ap.add_argument("-o", "--out", required=True)
    a = ap.parse_args()
    lines = open(a.src, encoding="utf-8", errors="replace").read().splitlines()
    sites, seen = [], {}

    for i, line in enumerate(lines, 1):
        for m in CONST_RE.finditer(line):
            name, hexv, trail = m.group(1), m.group(2), m.group(3).strip()
            kind = "data" if DATA_HINT.search(name) else "func"
            sites.append(dict(name=name, rva=hexv, kind=kind, line=i,
                              note=trail.lstrip("/ ").strip(), source="const"))
            seen[int(hexv, 16)] = name

    inline_idx = 0
    for i, line in enumerate(lines, 1):
        if "base +" not in line and "base+" not in line:
            continue
        for m in INLINE_RE.finditer(line):
            rva = int(m.group(1), 16)
            inline_idx += 1
            # classify: a few known data reads vs call/thunk targets
            ctx = line.strip()
            kind = "data" if DATA_HINT.search(ctx) else "func"
            sites.append(dict(name=f"inline_{i}_{rva:08x}", rva=hex(rva), kind=kind,
                              line=i, note=ctx[:120], source="inline",
                              dup_of=seen.get(rva)))

    # Re-bucket by value magnitude and name:
    #   struct : value < 0x100000 or name endswith _Off  -> SDK-derived member offset
    #   bound  : value is a round image-size/range constant (ends in many zeros, >= 0x0A000000)
    #            used as `base + BOUND` for range checks -> derived from new image size, not ported
    #   rva    : everything else -> code/data address to signature-port + IDA-verify
    for s in sites:
        v = int(s["rva"], 16)
        if v < 0x100000 or s["name"].endswith("_Off"):
            s["bucket"] = "struct"
        elif v >= 0x0A000000 and (v & 0xFFFFF) == 0:
            s["bucket"] = "bound"
        else:
            s["bucket"] = "rva"

    buckets = {b: [s for s in sites if s["bucket"] == b] for b in ("rva", "struct", "bound")}
    # porter input: unique RVA-bucket addresses only
    uniq = {}
    for s in buckets["rva"]:
        uniq.setdefault(int(s["rva"], 16), s)
    porter = [{"name": s["name"], "rva": s["rva"], "kind": s["kind"], "note": s.get("note", "")}
              for s in uniq.values()]

    c = dict(total=len(sites), rva_unique=len(uniq),
             rva=len(buckets["rva"]), struct=len(buckets["struct"]), bound=len(buckets["bound"]),
             data_in_rva=sum(1 for s in buckets["rva"] if s["kind"] == "data"))
    json.dump(dict(all=sites, porter=porter, buckets=buckets, counts=c),
              open(a.out, "w"), indent=1)
    print(c)
    json.dump(porter, open(a.out.replace(".json", ".porter.json"), "w"), indent=1)


if __name__ == "__main__":
    main()
