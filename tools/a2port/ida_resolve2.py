# Authoritative resolve pass against the NAMED 47031 IDB. Read-only.
#  A. UFunction exec -> _Implementation: decompile each exec (address from the
#     funcinfo oracle, exact name match), drop shared marshalling helpers
#     (validated by GLOBAL caller count, not size), take the single non-helper
#     call (tail). No non-helper call => impl inlined => exec IS the hook target.
#  B. GIsClient/GIsServer: decompile the net-mode predicates that branch on them
#     and dump the 1-byte data globals they read.
#  C. Capture strings/callees for the unnamed join internals for offline anchoring.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_bytes

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\resolve2_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n"); LOG.flush()


# site -> UFunction exec RVA (exact funcinfo name match)
UFUNC_EXEC = {
    "UpdGameState_RVA": 0x54a2220,      # UGameStateManagerComponent::UpdateGameState
    "StartTimerCd_RVA": 0x54a6810,      # UGameTimeComponent::StartTimerWithCountdown
    "IsRunningSimulate_RVA": 0x54a1930, # UGameplayUtilityStatics::IsRunningSimulate
    "GetBoolCfg_RVA": 0x49c5ba0,        # UModuleStateLuaAPI::GetBoolConfigVariable
    "GetNumCfg_RVA": 0x49c6240,         # UModuleStateLuaAPI::GetNumberConfigVariable
    "GetStrCfg_RVA": 0x49c65c0,         # UModuleStateLuaAPI::GetStringConfigVariable
    "GolfOverlap_RVA": 0x54a9890,       # UGolfCupComponent::OnGoalBeginOverlap
    "GetPlayerPawn_RVA": 0x3ae0310,     # UGameplayStatics::GetPlayerPawn
    "GetLocalInst_RVA": 0x54f2b70,      # AVRPawn::GetLocalInstanceWithWorld
    "A2GetNetMode_RVA": 0x22ac640,      # UA2NetworkUtilityBPFL::GetNetMode
    "NotifyLeftArena_RVA": 0x54f82b0,   # AVRPawn::Server_NotifyPlayerLeftArena
    "ReqLogin_RVA": 0x53dabb0,          # UAxOnlineCommunicationsComponent::RequestVivoxLoginToken
    "GolfBallInCup_RVA": 0x54a9200,     # UGolfCupComponent::BallInCup (old hooked an inner fn)
}
OLD_IMPL_SIZE = {
    "UpdGameState_RVA": 179, "StartTimerCd_RVA": 682, "IsRunningSimulate_RVA": 132,
    "GetBoolCfg_RVA": None, "GetNumCfg_RVA": None, "GetStrCfg_RVA": None,
    "GolfOverlap_RVA": 88, "GetPlayerPawn_RVA": None, "GetLocalInst_RVA": 255,
    "A2GetNetMode_RVA": 188, "NotifyLeftArena_RVA": 56, "ReqLogin_RVA": 61,
    "GolfBallInCup_RVA": 21,
}
# net-mode predicates that read GIsClient/GIsServer (exec RVAs)
NETMODE = {
    "IsServer": 0x22ad0b0, "IsClient": 0x22aca50, "GetNetMode": 0x22ac640,
    "KSL_IsServer": 0x3c097d0, "IsDedicatedServer": 0x22acd10,
}
# unnamed join internals (old-func-start RVAs we want evidence for)
JOIN_INTERNALS = {
    "JwtBuild_RVA": 0x544D9B0, "JoinBuild_RVA": 0x5432A40, "SendJoin_RVA": 0x52EE2E0,
}

try:
    ida_hexrays.init_hexrays_plugin()
    IB = idaapi.get_imagebase()
    log("imagebase", hex(IB))

    def rva(ea): return ea - IB
    def va(r): return IB + r

    _ccache = {}

    def global_callers(ea):
        if ea in _ccache:
            return _ccache[ea]
        c = set()
        for x in idautils.CodeRefsTo(ea, 0):
            f = ida_funcs.get_func(x)
            if f:
                c.add(f.start_ea)
        _ccache[ea] = len(c)
        return len(c)

    def finfo(ea):
        f = ida_funcs.get_func(ea)
        if not f:
            return None
        return {"rva": hex(rva(f.start_ea)), "size": f.end_ea - f.start_ea,
                "name": idc.get_func_name(f.start_ea)}

    def call_targets(start):
        out, seen = [], set()
        f = ida_funcs.get_func(start)
        if not f:
            return out
        for item in idautils.FuncItems(start):
            for xr in idautils.CodeRefsFrom(item, 0):
                cf = ida_funcs.get_func(xr)
                if cf and cf.start_ea != start and cf.start_ea not in seen:
                    seen.add(cf.start_ea)
                    out.append({"rva": hex(rva(cf.start_ea)), "name": idc.get_func_name(cf.start_ea),
                                "size": cf.end_ea - cf.start_ea, "gcallers": global_callers(cf.start_ea)})
        return out

    def data_bytes_refd(start):
        """1-/4-byte data globals read inside func (candidate engine globals)."""
        out = []
        for item in idautils.FuncItems(start):
            for d in idautils.DataRefsFrom(item):
                if ida_funcs.get_func(d):
                    continue
                sz = idc.get_item_size(d)
                out.append({"rva": hex(rva(d)), "size": sz, "name": idc.get_name(d)})
        # de-dup
        seen, res = set(), []
        for o in out:
            if o["rva"] not in seen:
                seen.add(o["rva"]); res.append(o)
        return res[:40]

    def decomp(start, cap=3500):
        try:
            cf = ida_hexrays.decompile(start)
            if cf:
                return str(cf)[:cap]
        except Exception as e:
            return "ERR:" + str(e)
        return None

    def strings_of(start):
        out = []
        for item in idautils.FuncItems(start):
            for d in idautils.DataRefsFrom(item):
                s = idc.get_strlit_contents(d, -1, idc.STRTYPE_C) or idc.get_strlit_contents(d, -1, idc.STRTYPE_C_16)
                if s:
                    try:
                        out.append(s.decode("utf-8", "replace") if isinstance(s, bytes) else s)
                    except Exception:
                        pass
        return list(dict.fromkeys(out))[:16]

    STEP = idaapi  # placeholder

    def pick_impl(exec_rva, old_size):
        start = va(exec_rva)
        f = ida_funcs.get_func(start)
        if not f:
            return {"err": "no func at exec", "exec": hex(exec_rva)}
        calls = call_targets(f.start_ea)
        # helper = shared marshalling routine (called by many funcs) or tiny stub
        nonhelper = [c for c in calls
                     if c["gcallers"] <= 12 and c["size"] >= 12
                     and "security_check" not in (c["name"] or "")
                     and not (c["name"] or "").startswith("j_")]
        rec = {"exec": hex(exec_rva), "exec_name": idc.get_func_name(f.start_ea),
               "exec_size": f.end_ea - f.start_ea, "old_impl_size": old_size,
               "all_calls": calls, "nonhelper": nonhelper,
               "decomp": decomp(f.start_ea, 1800)}
        if not nonhelper:
            rec["impl"] = hex(rva(f.start_ea)); rec["impl_via"] = "inlined-in-exec"
            rec["impl_size"] = f.end_ea - f.start_ea
        elif len(nonhelper) == 1:
            rec["impl"] = nonhelper[0]["rva"]; rec["impl_via"] = "single-nonhelper"
            rec["impl_size"] = nonhelper[0]["size"]
        else:
            # tail call = last in address order inside func; approximate: the one
            # with fewest global callers (most unique = the impl), tiebreak size~old
            cand = sorted(nonhelper, key=lambda c: (c["gcallers"],
                          abs((c["size"] or 0) - (old_size or 0))))
            rec["impl"] = cand[0]["rva"]; rec["impl_via"] = "min-callers/size"
            rec["impl_size"] = cand[0]["size"]; rec["impl_alts"] = [c["rva"] for c in cand[1:4]]
        return rec

    out = {"imagebase": IB, "ufunc": {}, "netmode": {}, "join": {}}

    for site, exrva in UFUNC_EXEC.items():
        try:
            out["ufunc"][site] = pick_impl(exrva, OLD_IMPL_SIZE.get(site))
            r = out["ufunc"][site]
            log(site, "->", r.get("impl"), r.get("impl_via"), "sz", r.get("impl_size"), "(old", OLD_IMPL_SIZE.get(site), ")")
        except Exception as e:
            out["ufunc"][site] = {"err": str(e)}
            log(site, "ERR", e)

    for nm, exrva in NETMODE.items():
        try:
            start = va(exrva)
            f = ida_funcs.get_func(start)
            # the predicate's impl (non-helper call) is what actually reads the global
            pr = pick_impl(exrva, None)
            impl_ea = va(int(pr["impl"], 16)) if pr.get("impl") else start
            out["netmode"][nm] = {"exec": hex(exrva), "impl": pr.get("impl"),
                                  "impl_decomp": decomp(impl_ea, 2200),
                                  "impl_data_bytes": data_bytes_refd(ida_funcs.get_func(impl_ea).start_ea if ida_funcs.get_func(impl_ea) else impl_ea)}
            log("netmode", nm, "impl", pr.get("impl"))
        except Exception as e:
            out["netmode"][nm] = {"err": str(e)}
            log("netmode", nm, "ERR", e)

    for site, oldrva in JOIN_INTERNALS.items():
        # old rva won't be valid in new; capture nothing from exec — instead record
        # that these need caller-anchoring. Provide old strings from old-ida offline.
        out["join"][site] = {"old_rva": hex(oldrva), "note": "unnamed C++; resolve via caller-anchor/string offline"}

    json.dump(out, open(A + r"\resolve2-dump.json", "w"), indent=1, default=str)
    log("DONE")
    open(A + r"\resolve2_ok.txt", "w").write("ok")
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\resolve2_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
