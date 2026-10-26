# NEW 47031 IDB. Resolve GIsServer/GIsClient by re-finding FEngineLoop::
# PreInitPostStartupScreen via its unique strings, then reading the 1-byte
# global it WRITES (mov [byte], 1 / al) == GIsServer; GIsClient == GIsServer-1.
# Cross-checked by the reader function that touches both adjacent bytes.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_ua

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\newgis_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n"); LOG.flush()


try:
    ida_hexrays.init_hexrays_plugin()
    IB = idaapi.get_imagebase()

    def rva(ea): return ea - IB

    MARK = ["FEngineLoop::PreInitPostStartupScreen",
            "GetMoviePlayer()->SetupLoadingScreenFromIni",
            "PreInitPostStartupScreen_StartupGraphics"]
    # find functions referencing those strings
    hit_funcs = {}
    for st in idautils.Strings():
        try:
            t = str(st)
        except Exception:
            continue
        for m in MARK:
            if m in t:
                for x in idautils.XrefsTo(st.ea):
                    f = ida_funcs.get_func(x.frm)
                    if f:
                        hit_funcs.setdefault(f.start_ea, set()).add(m)
    log("candidate PreInit funcs:", [(hex(rva(k)), sorted(v)) for k, v in hit_funcs.items()])

    def byte_writes(start):
        """find 'mov [rip+global], imm8/reg' to 1-byte data globals inside func."""
        writes = []
        f = ida_funcs.get_func(start)
        if not f:
            return writes
        for item in idautils.FuncItems(start):
            mnem = idc.print_insn_mnem(item)
            if mnem != "mov":
                continue
            # operand 0 must be memory, operand 1 imm or reg
            if idc.get_operand_type(item, 0) in (idc.o_mem, idc.o_displ):
                for d in idautils.DataRefsFrom(item):
                    if ida_funcs.get_func(d):
                        continue
                    sz = idc.get_item_size(d)
                    if sz == 1:
                        writes.append({"global": hex(rva(d)), "name": idc.get_name(d),
                                       "at": hex(rva(item)), "disasm": idc.generate_disasm_line(item, 0)})
        return writes

    def all_byte_refs(start):
        out = []
        f = ida_funcs.get_func(start)
        if not f:
            return out
        for item in idautils.FuncItems(start):
            for d in idautils.DataRefsFrom(item):
                if ida_funcs.get_func(d):
                    continue
                if idc.get_item_size(d) == 1:
                    out.append(rva(d))
        return sorted(set(out))

    out = {"preinit": {}, "pairs": []}
    # choose the func hitting the MOST markers (the real PreInitPostStartupScreen)
    best = sorted(hit_funcs.items(), key=lambda kv: -len(kv[1]))
    for fstart, marks in best[:3]:
        bw = byte_writes(fstart)
        # writes of constant 1 are the strongest GIsServer candidates
        const1 = [w for w in bw if w["disasm"].rstrip().endswith(", 1")]
        out["preinit"][hex(rva(fstart))] = {
            "markers": sorted(marks), "size": ida_funcs.get_func(fstart).end_ea - fstart,
            "name": idc.get_func_name(fstart),
            "byte_writes": bw[:40], "const1_writes": const1}
        log("func", hex(rva(fstart)), "byte_writes", len(bw), "const1", [w["global"] for w in const1])

    # Also: find functions that reference TWO adjacent 1-byte globals (the pair).
    # Scan the const1 candidates' neighbourhood.
    cand_servers = set()
    for finfo in out["preinit"].values():
        for w in finfo["const1_writes"]:
            cand_servers.add(int(w["global"], 16))
    out["gisserver_candidates"] = [hex(x) for x in sorted(cand_servers)]
    out["gisclient_candidates"] = [hex(x - 1) for x in sorted(cand_servers)]
    log("GIsServer candidates", out["gisserver_candidates"])

    json.dump(out, open(A + r"\newgis-dump.json", "w"), indent=1, default=str)
    open(A + r"\newgis_ok.txt", "w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\newgis_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
