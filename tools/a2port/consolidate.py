#!/usr/bin/env python3
"""Consolidated, hand-reviewed 47031 offset table -> offsets-47031-final.json.
Each value carries its proof. Engine globals + ProcessEvent are SDK-auto (not
baked). GIsServer/GIsClient are pulled from newgis-dump.json if resolved there.
"""
import json, os

A = os.path.dirname(os.path.abspath(__file__))

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
    "GolfBall_LastHitLocation_Off": 0x658, "Panel_IsActive_Off": 0x2C0,
    "Panel_Mesh_Off": 0x2E0, "Spawner_Disc_Off": 0x580,
}

# hand-reviewed resolutions (name/string/structure-proven)
R = {
    # UFunction exec -> _Implementation (decompiled; helpers=gcallers>1000)
    "UpdGameState_RVA": (0x55A06C0, "high", "UGameStateManagerComponent::UpdateGameState impl (tail; sz174~179; 3-method xcheck)"),
    "StartTimerCd_RVA": (0x559ABD0, "high", "UGameTimeComponent::StartTimerWithCountdown impl (tail; sz700~682)"),
    "IsRunningSimulate_RVA": (0x54A1930, "high", "UGameplayUtilityStatics::IsRunningSimulate (inlined exec; sz132==132)"),
    "GetLocalInst_RVA": (0x5810B30, "high", "AVRPawn::GetLocalInstanceWithWorld impl (single non-helper; sz272~255)"),
    "GetBoolCfg_RVA": (0x4A12EC0, "high", "UModuleStateLuaAPI::GetBoolConfigVariable impl (single non-helper; sz96)"),
    "GetNumCfg_RVA": (0x4A169D0, "high", "UModuleStateLuaAPI::GetNumberConfigVariable impl (single non-helper; sz106)"),
    "GetStrCfg_RVA": (0x4A17C10, "high", "UModuleStateLuaAPI::GetStringConfigVariable impl (single non-helper; sz112)"),
    "GetPlayerPawn_RVA": (0x3EC7C90, "high", "UGameplayStatics::GetPlayerPawn impl (tail==sizebest; sz20==20)"),
    "GolfBallInCup_RVA": (0x55B3900, "high", "UGolfCupComponent::BallInCup impl (single non-helper; sz24~21)"),
    "GolfOverlap_RVA": (0x54A9890, "high", "UGolfCupComponent::OnGoalBeginOverlap (name-matched handler)"),
    "JwtBuild_RVA": (0x53DB090, "high", "unique Vivox-JWT template strings xref (iss/vxa/vxi/exp)"),
    "WorldGetNetMode_RVA": (0x41F5A60, "high", "unique caller of IsPlaying+IsRecording impls, sz410==410 (force DedicatedServer before open)"),
    # SDK struct members (exact class+field in 47031 SDK)
    "Spawner_Disc_Off": (0x580, "high", "UBallSpawnerComponent::Disc @0x580 (SDK; == 22284)"),
    # med: best candidate, flagged for runtime confirm
    "GolfBall_LastHitLocation_Off": (0x860, "med", "BP_GolfBallDisplay::LastHitLocation @0x860 (SDK; class-link vs BP_JakeBall unconfirmed)"),
    # A2GetNetMode intentionally NOT baked: 0x14DB9C0 is a shared 483-caller
    # net-mode resolver, not the specific GetNetMode_Implementation. Leave at
    # 22284 value + TODO so a wrong shared-helper hook can't fire.
}

# GIsClient/GIsServer: adjacent byte pair written by the 47031
# FEngineLoop::PreInitPostStartupScreen (sub_1442D4280, matched by its 3 unique
# strings): 'mov cs:byte_14A6D3882,al; mov cs:byte_14A6D3883,al'. Mirrors the
# 22284 layout (client=server-1); region shift 0x96D->0xA6D matches GNames.
R["GIsClient_RVA"] = (0xA6D3882, "high", "FEngineLoop::PreInitPostStartupScreen adjacent-pair write (client=server-1)")
R["GIsServer_RVA"] = (0xA6D3883, "high", "FEngineLoop::PreInitPostStartupScreen adjacent-pair write (unique 3-string match)")
gis_note = "resolved 0xA6D3882/0xA6D3883"

SDK_AUTO = {"GObjects": 0xA7C3A00, "GNames": 0xA6DFBC0, "GWorld": 0xA9E0968,
            "AppendString": 0x12AD4E0, "ProcessEvent": 0x14DC400}

resolved = {k: {"val": v[0], "conf": v[1], "why": v[2]} for k, v in R.items()}
unres = [n for n in OLD22284 if n not in resolved]

print("GIsServer/GIsClient:", gis_note, "\n")
order = {"high": 0, "med": 1}
by = {}
for name, r in sorted(resolved.items(), key=lambda kv: (order.get(kv[1]["conf"], 9), kv[0])):
    by[r["conf"]] = by.get(r["conf"], 0) + 1
    old = OLD22284.get(name)
    tag = "" if old is None else (" SAME" if old == r["val"] else " (was 0x%X)" % old)
    print("  [%-4s] %-28s 0x%-9X%s  %s" % (r["conf"], name, r["val"], tag, r["why"][:52]))
print("\nUNRESOLVED (kept 22284, TODO-47031, gated): %d" % len(unres))
print("   " + ", ".join(sorted(unres)))
print("\nSDK-auto (via repoint):", {k: hex(v) for k, v in SDK_AUTO.items()})
print("by confidence:", by)

json.dump({"resolved": {k: {"val": "0x%X" % v["val"], "conf": v["conf"], "why": v["why"]}
                        for k, v in resolved.items()},
           "sdk_auto": {k: "0x%X" % v for k, v in SDK_AUTO.items()},
           "unresolved": {n: "0x%X" % OLD22284[n] for n in unres}},
          open(os.path.join(A, "offsets-47031-final.json"), "w"), indent=1)
print("\nwrote offsets-47031-final.json")
