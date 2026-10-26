#!/usr/bin/env python3
"""Join extract_sites + porter result + new-SDK globals into one worksheet."""
import json

sites = json.load(open("sites.json"))
port = {r["name"]: r for r in json.load(open("port-result.json"))}

# Ground-truth globals from the 47031 Dumper-7 SDK (CppSDK/SDK/Basic.hpp Offsets::).
SDK_GLOBALS = {
    "GObjects": 0x0A7C3A00, "GNames": 0x0A6DFBC0, "GWorld": 0x0A9E0968,
    "AppendString": 0x012AD4E0, "ProcessEvent": 0x014DC400,
}

rows = []
for s in sites["buckets"]["rva"]:
    name = s["name"]
    r = port.get(name, {})
    new = r.get("new_rva")
    hits = r.get("hits", "")
    if r.get("kind") == "data" or s["kind"] == "data":
        status, cand = "IDA-DATA", ""
    elif new and new != "None":
        status, cand = "CANDIDATE", new
    elif isinstance(hits, int) and hits > 1:
        status, cand = "AMBIGUOUS", ",".join(r.get("candidates", [])[:4])
    else:
        status, cand = "IDA-FIND", ""
    rows.append(dict(name=name, old=s["rva"], new=cand, status=status,
                     kind=s["kind"], line=s["line"], note=s.get("note", "")[:70]))

order = {"CANDIDATE": 0, "AMBIGUOUS": 1, "IDA-DATA": 2, "IDA-FIND": 3}
rows.sort(key=lambda x: (order[x["status"]], x["name"]))
summary = {}
for x in rows:
    summary[x["status"]] = summary.get(x["status"], 0) + 1
json.dump(dict(rows=rows, summary=summary, sdk_globals={k: hex(v) for k, v in SDK_GLOBALS.items()}),
          open("worksheet.json", "w"), indent=1)
print("summary:", summary)
