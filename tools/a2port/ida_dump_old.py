# Dump the 22284 IDB into an oracle for porting: for every port site, the
# containing function's name, bounds, pseudocode, referenced string literals,
# immediate constants, and callers/callees (as RVAs). Plus global maps:
# every named function and named data item (name->rva), and a string->rvas
# table for string-xref re-finding in the new binary. Headless, read-only.
import json, re
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes, ida_name, ida_nalt, ida_xref

IB = idaapi.get_imagebase()
OUT = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\old-ida.json"
SITES = json.load(open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\sites.json"))


def rva(ea):
    return ea - IB


def auto_named(ea):
    n = idc.get_name(ea)
    return n and not re.match(r"^(sub|loc|off|dword|qword|byte|word|unk|j_|nullsub|stru|asc|a[A-Z0-9])", n)


def func_strings(f):
    out = []
    for ea in idautils.FuncItems(f.start_ea):
        for xr in idautils.DataRefsFrom(ea):
            s = idc.get_strlit_contents(xr, -1, idc.STRTYPE_C)
            if s:
                try:
                    out.append(s.decode("utf-8", "replace"))
                except Exception:
                    pass
                continue
            s = idc.get_strlit_contents(xr, -1, idc.STRTYPE_C_16)
            if s:
                out.append(s.decode("utf-16-le", "replace"))
    # de-dup, cap
    seen, res = set(), []
    for s in out:
        if s not in seen and len(s) >= 4:
            seen.add(s); res.append(s[:160])
        if len(res) >= 24:
            break
    return res


def pseudo(ea, maxlen=4000):
    try:
        cf = ida_hexrays.decompile(ea)
        if cf:
            return str(cf)[:maxlen]
    except Exception:
        pass
    return None


def callers(f):
    out = set()
    for xr in idautils.CodeRefsTo(f.start_ea, 0):
        cf = ida_funcs.get_func(xr)
        if cf:
            out.add(cf.start_ea)
    return [hex(rva(x)) for x in sorted(out)][:12]


def callees(f):
    out = set()
    for ea in idautils.FuncItems(f.start_ea):
        for xr in idautils.CodeRefsFrom(ea, 0):
            cf = ida_funcs.get_func(xr)
            if cf and cf.start_ea != f.start_ea:
                out.add(cf.start_ea)
    res = []
    for x in sorted(out):
        res.append({"rva": hex(rva(x)), "name": idc.get_func_name(x)})
        if len(res) >= 20:
            break
    return res


def dump_site(s):
    ea = IB + int(s["rva"], 16)
    d = {"name": s["name"], "rva": s["rva"], "kind": s["kind"], "note": s.get("note", "")}
    f = ida_funcs.get_func(ea)
    if not f:
        # data or raw site: record name + who references it
        d["sym"] = idc.get_name(ea)
        xr = []
        for x in idautils.XrefsTo(ea):
            cf = ida_funcs.get_func(x.frm)
            xr.append({"from_rva": hex(rva(x.frm)),
                       "in_func": idc.get_func_name(cf.start_ea) if cf else None,
                       "in_func_rva": hex(rva(cf.start_ea)) if cf else None})
            if len(xr) >= 20:
                break
        d["xrefs_to"] = xr
        d["is_func"] = False
        return d
    d["is_func"] = True
    d["func_name"] = idc.get_func_name(f.start_ea)
    d["func_start_rva"] = hex(rva(f.start_ea))
    d["inner_offset"] = ea - f.start_ea
    d["func_size"] = f.end_ea - f.start_ea
    d["named"] = auto_named(f.start_ea)
    d["strings"] = func_strings(f)
    d["callers"] = callers(f)
    d["callees"] = callees(f)
    d["pseudo"] = pseudo(f.start_ea)
    return d


ida_hexrays.init_hexrays_plugin()
# unique RVA sites only
seen, sites_out = set(), []
for s in SITES["all"]:
    key = int(s["rva"], 16)
    if s["bucket"] == "bound" or key in seen:
        continue
    seen.add(key)
    sites_out.append(dump_site(s))

# global symbol maps
named_funcs = {}
for ea in idautils.Functions():
    n = idc.get_func_name(ea)
    if n and auto_named(ea):
        named_funcs[n] = hex(rva(ea))

# string -> rvas (for re-finding by unique string in the new binary)
strtab = {}
for s in idautils.Strings():
    try:
        txt = str(s)
    except Exception:
        continue
    if len(txt) < 5:
        continue
    refs = [hex(rva(x.frm)) for x in idautils.XrefsTo(s.ea)][:8]
    if refs:
        strtab.setdefault(txt[:160], []).extend(refs)

json.dump({"imagebase": IB, "sites": sites_out,
           "named_func_count": len(named_funcs), "named_funcs": named_funcs,
           "string_count": len(strtab)},
          open(OUT, "w"), indent=1, default=str)
# strings table is large: separate file
json.dump(strtab, open(OUT.replace("old-ida", "old-strings"), "w"), default=str)
print("dumped", len(sites_out), "sites,", len(named_funcs), "named funcs,", len(strtab), "strings")
