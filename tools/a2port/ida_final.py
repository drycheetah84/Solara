# Focused final pass on the NAMED 47031 IDB. Read-only.
#  1. GIsServer/GIsClient: the BPFL IsServer/IsClient exec -> body(one call past
#     the marshalling helpers) -> the 1-byte global it reads. Report adjacency.
#  2. JwtBuild: xref the unique Vivox-JWT template strings to a function.
#  3. JoinBuild: the function calling BOTH UKismetMathLibrary::UtcNow (impl) and
#     UTargetingSubsystem::StartAsyncTargetingRequest (structural).
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\final_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n"); LOG.flush()


try:
    ida_hexrays.init_hexrays_plugin()
    IB = idaapi.get_imagebase()

    def rva(ea): return ea - IB
    def va(r): return IB + r

    def nonhelper_calls(start, th=1000):
        out = []
        f = ida_funcs.get_func(start)
        if not f:
            return out
        for item in idautils.FuncItems(start):
            for xr in idautils.CodeRefsFrom(item, 0):
                cf = ida_funcs.get_func(xr)
                if not cf or cf.start_ea == start:
                    continue
                c = sum(1 for _ in idautils.CodeRefsTo(cf.start_ea, 0))
                sz = cf.end_ea - cf.start_ea
                nm = idc.get_func_name(cf.start_ea)
                if c <= th and sz >= 12 and "security_check" not in (nm or ""):
                    out.append((rva(cf.start_ea), sz, nm))
        return out

    def byte_globals(start):
        out = []
        for item in idautils.FuncItems(start):
            for d in idautils.DataRefsFrom(item):
                if ida_funcs.get_func(d):
                    continue
                out.append((rva(d), idc.get_item_size(d), idc.get_name(d)))
        seen, res = set(), []
        for o in out:
            if o[0] not in seen:
                seen.add(o[0]); res.append(o)
        return res

    def decomp(start, cap=2500):
        try:
            cf = ida_hexrays.decompile(start)
            if cf:
                return str(cf)[:cap]
        except Exception as e:
            return "ERR:" + str(e)
        return None

    out = {}

    # 1. GIsServer / GIsClient -------------------------------------------------
    preds = {"IsServer": 0x22ad0b0, "IsClient": 0x22aca50, "KSL_IsServer": 0x3c097d0,
             "IsDedicatedServer": 0x22acd10, "GetNetMode": 0x22ac640}
    gg = {}
    for nm, exrva in preds.items():
        rec = {"exec": hex(exrva)}
        nh = nonhelper_calls(va(exrva))
        rec["body_calls"] = [(hex(r), s, n) for r, s, n in nh]
        # follow the first non-helper body call and read its byte globals + decomp
        bodies = {}
        for r, s, n in nh[:3]:
            bg = byte_globals(va(r))
            bodies[hex(r)] = {"size": s, "name": n,
                              "byte_globals": [(hex(x), sz, gn) for x, sz, gn in bg if sz in (1, 4)][:12],
                              "decomp": decomp(va(r), 1200)}
        rec["bodies"] = bodies
        # also: byte globals the exec itself touches
        bg0 = byte_globals(va(exrva))
        rec["exec_byte_globals"] = [(hex(x), sz, gn) for x, sz, gn in bg0 if sz in (1, 4)][:12]
        gg[nm] = rec
        log(nm, "body_calls", rec["body_calls"][:4])
    out["globals"] = gg

    # 2. JwtBuild via unique strings ------------------------------------------
    jwt_markers = ['{ "iss": "', '"vxa": "', '"vxi": ', '"exp": ']
    jwt_hits = {}
    for st in idautils.Strings():
        try:
            t = str(st)
        except Exception:
            continue
        for m in jwt_markers:
            if m in t:
                fs = set()
                for x in idautils.XrefsTo(st.ea):
                    f = ida_funcs.get_func(x.frm)
                    if f:
                        fs.add(rva(f.start_ea))
                jwt_hits.setdefault(m, []).extend(hex(e) for e in sorted(fs))
    out["JwtBuild"] = {"hits": jwt_hits}
    log("jwt", jwt_hits)

    # 3. JoinBuild: caller of UtcNow + StartAsyncTargetingRequest --------------
    def callers_of_name(name):
        ea = idc.get_name_ea_simple(name)
        if ea == idaapi.BADADDR:
            return None, set()
        s = set()
        for x in idautils.CodeRefsTo(ea, 0):
            f = ida_funcs.get_func(x)
            if f:
                s.add(rva(f.start_ea))
        return rva(ea), s
    utc_ea, utc_callers = callers_of_name("UKismetMathLibrary::UtcNow")
    tgt_ea, tgt_callers = callers_of_name("UTargetingSubsystem::StartAsyncTargetingRequest")
    inter = sorted(utc_callers & tgt_callers)
    out["JoinBuild"] = {"utcnow": hex(utc_ea) if utc_ea else None,
                        "targeting": hex(tgt_ea) if tgt_ea else None,
                        "utc_callers": [hex(x) for x in sorted(utc_callers)][:20],
                        "intersection": [hex(x) for x in inter]}
    log("joinbuild inter", out["JoinBuild"]["intersection"])

    json.dump(out, open(A + r"\final-dump.json", "w"), indent=1, default=str)
    open(A + r"\final_ok.txt", "w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\final_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
