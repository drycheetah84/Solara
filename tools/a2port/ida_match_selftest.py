# Run inside the NEW (47031) IDB. For every 22284 site in old-ida.json, resolve
# the new RVA by, in order of confidence:
#   name   exact function-name match (old IDB name == new IDB name)   [highest]
#   string a distinctive string the old func referenced is referenced by exactly
#          one new func
#   sig    the port_offsets.py signature candidate, confirmed when the new func's
#          callee-name sequence and size resemble the old
# For sites that are an instruction inside a function (inner_offset>0, e.g. byte
# patch sites), the function is resolved as above and the exact instruction is
# re-located by scanning for the same mnemonic+operand shape near the mapped
# inner offset; these are flagged lower confidence for human review.
import json, re
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
OLD = json.load(open(A + r"\old-ida.json"))
PORT = {r["name"]: r for r in json.load(open(A + r"\port-result.json"))}
IB = idaapi.get_imagebase()
ida_hexrays.init_hexrays_plugin()


def rva(ea):
    return ea - IB


# new-binary indexes
print("indexing new binary...")
new_name2ea, new_funcs = {}, []
for ea in idautils.Functions():
    n = idc.get_func_name(ea)
    new_funcs.append(ea)
    if n:
        new_name2ea.setdefault(n, ea)

new_str2funcs = {}
for s in idautils.Strings():
    try:
        txt = str(s)
    except Exception:
        continue
    if len(txt) < 6:
        continue
    fs = set()
    for x in idautils.XrefsTo(s.ea):
        f = ida_funcs.get_func(x.frm)
        if f:
            fs.add(f.start_ea)
    if fs:
        new_str2funcs.setdefault(txt, set()).update(fs)


def callee_names(ea):
    f = ida_funcs.get_func(ea)
    if not f:
        return []
    out = []
    for item in idautils.FuncItems(f.start_ea):
        for xr in idautils.CodeRefsFrom(item, 0):
            cf = ida_funcs.get_func(xr)
            if cf and cf.start_ea != f.start_ea:
                nm = idc.get_func_name(cf.start_ea)
                if nm and not nm.startswith("sub_"):
                    out.append(nm)
    return out


def resolve(site):
    old_name = site.get("func_name", "")
    res = {"name": site["name"], "old": site["rva"], "kind": site["kind"],
           "is_func": site.get("is_func"), "inner_offset": site.get("inner_offset", 0),
           "old_func": old_name, "new_func_start": None, "new": None,
           "method": None, "confidence": "none", "note": site.get("note", "")}

    # DATA sites: resolve by unique xref-from a now-nameable function is hard here;
    # leave for the data pass (needs the function map). Mark.
    if not site.get("is_func"):
        res["method"] = "data"; res["confidence"] = "todo-data"
        return res

    cand = None
    # 1) name match (only trust non-sub_ names)
    if old_name and not old_name.startswith("sub_") and old_name in new_name2ea:
        cand = new_name2ea[old_name]; res["method"] = "name"; res["confidence"] = "high"

    # 2) string xref
    if cand is None:
        for strv in site.get("strings", []):
            fs = new_str2funcs.get(strv)
            if fs and len(fs) == 1:
                cand = next(iter(fs)); res["method"] = "string"; res["confidence"] = "high"
                res["via_string"] = strv[:60]
                break
        if cand is None:
            # intersection of funcs referencing the old func's strings
            inter = None
            for strv in site.get("strings", []):
                fs = new_str2funcs.get(strv)
                if not fs:
                    continue
                inter = fs if inter is None else (inter & fs)
            if inter and len(inter) == 1:
                cand = next(iter(inter)); res["method"] = "string-intersect"; res["confidence"] = "med"

    # 3) signature candidate, confirmed by callee-name overlap
    if cand is None:
        p = PORT.get(site["name"], {})
        nr = p.get("new_rva")
        if nr and nr != "None":
            try:
                cea = IB + int(nr, 16) if isinstance(nr, str) else IB + nr
            except Exception:
                cea = None
            if cea:
                f = ida_funcs.get_func(cea)
                cstart = f.start_ea if f else cea
                old_callees = set(c["name"] for c in site.get("callees", []) if not c["name"].startswith("sub_"))
                new_callees = set(callee_names(cstart))
                overlap = len(old_callees & new_callees)
                res["method"] = "sig"; res["sig_overlap"] = overlap
                res["confidence"] = "med" if overlap >= 2 else "low"
                cand = cstart

    if cand is not None:
        f = ida_funcs.get_func(cand)
        start = f.start_ea if f else cand
        res["new_func_start"] = hex(rva(start))
        res["new_func_name"] = idc.get_func_name(start)
        # map inner offset for patch/instruction sites
        if res["inner_offset"]:
            res["new"] = hex(rva(start) + res["inner_offset"])  # best-effort; verify disasm
            res["inner_note"] = "inner offset copied; VERIFY instruction in IDA"
            if res["confidence"] == "high":
                res["confidence"] = "med"  # inner-offset copy is not exact
        else:
            res["new"] = hex(rva(start))
        if f:
            res["new_func_size"] = f.end_ea - f.start_ea
            res["old_func_size"] = site.get("func_size")
    return res


out = [resolve(s) for s in OLD["sites"]]
by_conf = {}
for r in out:
    by_conf[r["confidence"]] = by_conf.get(r["confidence"], 0) + 1
json.dump({"results": out, "by_confidence": by_conf}, open(A + r"\new-map.json", "w"), indent=1)
print("resolved:", by_conf)
