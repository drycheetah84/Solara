# gen_catalogue.py <Prefabs export dir> <out le_catalogue.txt> -- palette list from an exported Content/A2/Prefabs tree.
# Every LE_*.uasset becomes "<category>|/Game/A2/Prefabs/<rel>/<Name>.<Name>_C"; category = folder path joined with
# " / " (top level = "General"). Only file NAMES are read, never file contents.
import os, sys
root, out = sys.argv[1], sys.argv[2]
rows = []
for d, _, files in os.walk(root):
    rel = os.path.relpath(d, root).replace("\\", "/")
    for f in files:
        if not (f.startswith("LE_") and f.endswith(".uasset")): continue
        name = f[:-len(".uasset")]
        sub = "" if rel == "." else rel + "/"
        cat = "General" if rel == "." else " / ".join(rel.split("/"))
        rows.append(f"{cat}|/Game/A2/Prefabs/{sub}{name}.{name}_C")
rows.sort()
with open(out, "w", encoding="utf-8", newline="\n") as fh:
    fh.write("# Generated from the 50451 prefab dump (Content/A2/Prefabs). <category>|<class path>.\n")
    fh.write("\n".join(rows) + "\n")
print(len(rows), "prefabs ->", out)
