#!/usr/bin/env python3
"""Merge every resolution source into one 47031 offset table with honest
confidence, and emit a sed-ready patch map for dllmain_47031.cpp.
Sources:
  - resolve2-dump.json  : UFunction exec->impl + net-mode globals
  - SDK Basic.hpp       : engine globals + ProcessEvent (auto via SDK repoint)
  - SDK *_classes.hpp   : struct member offsets
  - prior cross-validated values
Nothing here is 'guessed': a value is HIGH only with a name/string/structural
proof; otherwise it stays the 22284 value flagged TODO-47031 (and must be gated
off if it is on the boot path).
"""
import json, os

A = os.path.dirname(os.path.abspath(__file__))
r2 = json.load(open(os.path.join(A, "resolve2-dump.json")))

# 22284 current values (to diff against / keep when unresolved)
OLD22284 = {
    "GIsClient_RVA": 0x96D8672, "GIsServer_RVA": 0x96D8673,
    "StationStr_Data_RVA": 0x95DF980, "StationStr_Num_RVA": 0x95DF988, "StationStr_Max_RVA": 0x95DF98C,
    "CosmeticCmp_RVA": 0x53DA880, "FNamePoolTbl_RVA": 0x9710B80,
    "GetBoolCfg_RVA": 0x46DA020, "GetNumCfg_RVA": 0x46DDB20, "GetStrCfg_RVA": 0x46DE7B0,
    "UpdGameState_RVA": 0x53CB6D0, "StartTimerCd_RVA": 0x53C4060, "GtcTick_RVA": 0x53C8AF0,
    "GtcVft_RVA": 0x84E6D20, "JwtBuild_RVA": 0x544D9B0, "ReqLogin_RVA": 0x543C7B0,
    "JoinBuild_RVA": 0x5432A40, "SendJoin_RVA": 0x52EE2E0, "ReplayPostStep_RVA": 0x5487560,
    "ReplayVtable_RVA": 0x84A2B38, "ReplaySnapshot_RVA": 0x5482110, "Reconcile_RVA": 0x545BD80,
    "SendPhysImpl_RVA": 0x5503050, "SimIntegrate_RVA": 0x547F7A0, "IngestInput_RVA": 0x54599B0,
    "SendResults_RVA": 0x534A7F0, "StepSim_RVA": 0x54863C0, "BallAdvance_RVA": 0x5464630,
    "BallReconcile_RVA": 0x548FC40, "NotifyLeftArena_RVA": 0x5502B00, "GolfBallInCup_RVA": 0x53DC320,
    "GolfOverlap_RVA": 0x53F03B0, "FNameResolve_RVA": 0x114D690, "TextLayoutLeaf_RVA": 0x1EC07A0,
    "EvtDispatch_RVA": 0x466A240, "GiveCheck_RVA": 0x4723AA0, "FetchRoles_RVA": 0x54B04E0,
    "RolesFetched_RVA": 0x5510C40, "IsRunningSimulate_RVA": 0x531EF96, "A2GetNetMode_RVA": 0x54AF360,
    "WorldGetNetMode_RVA": 0x4040990, "BallSimGetWorld_RVA": 0x3509AB0, "GetLocalInst_RVA": 0x54EAC60,
    "GetPlayerPawn_RVA": 0x3D24C40, "BallNetPersp_RVA": 0x60A0990, "PrintString_RVA": 0x3A29B20,
    # member offsets
    "GolfBall_LastHitLocation_Off": 0x658, "Panel_IsActive_Off": 0x2C0,
    "Panel_Mesh_Off": 0x2E0, "Spawner_Disc_Off": 0x580,
}

resolved = {}   # name -> {"val":int, "conf":str, "why":str}


def put(name, val, conf, why):
    resolved[name] = {"val": val, "conf": conf, "why": why}


# --- UFunction impls from resolve2 ---
for site, r in r2.get("ufunc", {}).items():
    if r.get("err") or not r.get("impl"):
        continue
    impl = int(r["impl"], 16)
    via = r.get("impl_via", "")
    sz, old = r.get("impl_size"), r.get("old_impl_size")
    close = (old is None) or (sz is not None and abs(sz - old) <= max(60, (old or 0) * 0.6))
    # inlined-in-exec and single-nonhelper are structurally certain
    if via in ("inlined-in-exec", "single-nonhelper"):
        conf = "high"
    else:
        conf = "med" if close else "low"
    put(site, impl, conf, "exec->impl %s sz=%s(old %s)" % (via, sz, old))

# --- SDK struct member offsets (verified by exact class+field in 47031 SDK) ---
put("Spawner_Disc_Off", 0x580, "high", "UBallSpawnerComponent::Disc @0x580 (SDK)")
put("GolfBall_LastHitLocation_Off", 0x860, "med", "BP_GolfBallDisplay::LastHitLocation @0x860 (SDK; class-link unconfirmed vs BP_JakeBall)")
# Panel_* left unresolved: BP_FloorPanelB_LE_C not in SDK headers (runtime-verify)

# --- engine globals: provided by the SDK repoint, NOT baked as _RVA ---
SDK_AUTO = {
    "GObjects": 0xA7C3A00, "GNames": 0xA6DFBC0, "GWorld": 0xA9E0968,
    "AppendString": 0x12AD4E0, "ProcessEvent": 0x14DC400,
}

# --- report ---
print("== 47031 OFFSET RESOLUTION (consolidated) ==\n")
print("SDK-auto (via gamesdk/47031 repoint, no bake needed):")
for k, v in SDK_AUTO.items():
    print("  %-14s 0x%X" % (k, v))
print()
order = {"high": 0, "med": 1, "low": 2}
rows = sorted(resolved.items(), key=lambda kv: (order.get(kv[1]["conf"], 9), kv[0]))
by = {}
for name, r in rows:
    by[r["conf"]] = by.get(r["conf"], 0) + 1
    old = OLD22284.get(name)
    chg = "" if old is None else ("  (22284 0x%X%s)" % (old, " SAME" if old == r["val"] else ""))
    print("  [%-4s] %-28s 0x%-9X %s%s" % (r["conf"], name, r["val"], r["why"][:46], chg))

# unresolved (kept at 22284, must be gated if boot-path)
unres = [n for n in OLD22284 if n not in resolved]
print("\nUNRESOLVED (kept at 22284 value, flag TODO-47031): %d" % len(unres))
for n in sorted(unres):
    print("   ", n, "0x%X" % OLD22284[n])

print("\nby confidence:", by)
json.dump({"resolved": {k: {"val": "0x%X" % v["val"], "conf": v["conf"], "why": v["why"]}
                        for k, v in resolved.items()},
           "sdk_auto": {k: "0x%X" % v for k, v in SDK_AUTO.items()},
           "unresolved": {n: "0x%X" % OLD22284[n] for n in unres}},
          open(os.path.join(A, "offsets-47031-final.json"), "w"), indent=1)
print("\nwrote offsets-47031-final.json")
