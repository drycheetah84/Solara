# HalcyonA2 DLL port: 22284 → 47031 — status

UE 5.4.2 build **22284** → UE 5.5.3 build **47031** ("hotfix"). The injected
server DLL (`Server/dllmain.cpp`, 15k lines) carries ~46 build-specific `_RVA`
constants plus ~10 inline `base + 0x…` hook addresses. This is the port of
those to 47031.

## TL;DR
- **The 47031 DLL compiles and links.** `Server/build/47031/x64/Release/HalcyonA2.dll` (1.3 MB) via
  `msbuild HalcyonA2.vcxproj /p:GameBuild=47031 /p:Configuration=Release /p:Platform=x64 /p:SolutionDir=<repo>\`.
- **15 offsets resolved** to high/med confidence by real proof (14 high, 1 med),
  including the boot-critical `GIsClient`/`GIsServer` server-flip pair.
- **5 engine globals + ProcessEvent port automatically** via the SDK repoint
  (`gamesdk/47031/SDK`), no hand-baking.
- **NOT yet runnable/injectable as-is:** 35 `_RVA` + ~10 inline addresses are
  still the stale 22284 values (flagged `TODO-47031`), and the boot thread
  installs ~50 MinHook hooks unconditionally. Hooking a stale address in 47031
  corrupts if that code runs. `g_bootSpineOnly=true` only neutralises the
  ProcessEvent hook, not the other installs. A safe inject needs those resolved
  or their hook-installs gated.

## Method (so it's reproducible, not guessed)
1. `ida_name.py` — applied 14,565 Dumper `FunctionsInfo.json` names to the 47031
   IDB, so matching has anchors (IDB went 5,791 → 19,528 named funcs).
2. `ida_resolve2.py` / `ida_final.py` — for each UFunction, decompiled the exec
   thunk and took the single **non-helper** call as the `_Implementation`
   (helpers identified by **global caller count** >1000, not by size — size
   heuristics collided on shared marshalling routines).
3. Cross-IDB for data globals: characterised the function referencing
   `GIsClient/GIsServer` in the known-22284 IDB (`old_gis.py`), re-found it in
   47031 by unique strings (`new_gis.py`).
4. SDK for struct member offsets and engine globals (`Basic.hpp`, `*_classes.hpp`).
5. `consolidate.py` → `offsets-47031-final.json`; `make_dllmain_47031.py` bakes
   it into `Server/dllmain_47031.cpp` (resolved value + `// [47031] was 0x…`;
   unresolved kept at 22284 + `TODO-47031`).

Nothing is baked on a size/signature guess alone — those disagreed with the
reflection oracle across the 5.4→5.5 bump and are explicitly rejected.

## Resolved (baked into dllmain_47031.cpp)
| name | 47031 | was (22284) | conf | proof |
|---|---|---|---|---|
| GIsClient_RVA | 0xA6D3882 | 0x96D8672 | high | FEngineLoop::PreInitPostStartupScreen adjacent-pair write (client=server-1) |
| GIsServer_RVA | 0xA6D3883 | 0x96D8673 | high | same, unique 3-string match; region shift 0x96D→0xA6D matches GNames |
| UpdGameState_RVA | 0x55A06C0 | 0x53CB6D0 | high | UGameStateManagerComponent::UpdateGameState impl (tail; 3-method xcheck) |
| StartTimerCd_RVA | 0x559ABD0 | 0x53C4060 | high | UGameTimeComponent::StartTimerWithCountdown impl |
| IsRunningSimulate_RVA | 0x54A1930 | 0x531EF96 | high | UGameplayUtilityStatics::IsRunningSimulate (inlined exec) |
| GetLocalInst_RVA | 0x5810B30 | 0x54EAC60 | high | AVRPawn::GetLocalInstanceWithWorld impl (single non-helper) |
| GetBoolCfg_RVA | 0x4A12EC0 | 0x46DA020 | high | UModuleStateLuaAPI::GetBoolConfigVariable impl |
| GetNumCfg_RVA | 0x4A169D0 | 0x46DDB20 | high | UModuleStateLuaAPI::GetNumberConfigVariable impl |
| GetStrCfg_RVA | 0x4A17C10 | 0x46DE7B0 | high | UModuleStateLuaAPI::GetStringConfigVariable impl |
| GetPlayerPawn_RVA | 0x3EC7C90 | 0x3D24C40 | high | UGameplayStatics::GetPlayerPawn impl (tail==size-best) |
| GolfBallInCup_RVA | 0x55B3900 | 0x53DC320 | high | UGolfCupComponent::BallInCup impl |
| GolfOverlap_RVA | 0x54A9890 | 0x53F03B0 | high | UGolfCupComponent::OnGoalBeginOverlap (name-matched) |
| JwtBuild_RVA | 0x53DB090 | 0x544D9B0 | high | unique Vivox-JWT template strings (iss/vxa/vxi/exp) |
| Spawner_Disc_Off | 0x580 | 0x580 | high | UBallSpawnerComponent::Disc @0x580 (SDK; unchanged) |
| GolfBall_LastHitLocation_Off | 0x860 | 0x658 | med | BP_GolfBallDisplay::LastHitLocation (SDK; class-link vs BP_JakeBall unconfirmed) |

### SDK-auto (via gamesdk/47031 repoint — not baked as constants)
GObjects 0xA7C3A00 · GNames 0xA6DFBC0 · GWorld 0xA9E0968 · AppendString 0x12AD4E0 · ProcessEvent 0x14DC400

## Unresolved (kept at 22284, `TODO-47031`, must resolve or gate before inject)
- **Vtable-dispatched RPCs** (impl reached via `vtable+off`, needs the class vtable):
  `NotifyLeftArena_RVA` (AVRPawn, +2408), `ReqLogin_RVA` (UAxOnlineCommunicationsComponent, +1280).
- **Ball-physics internals** (unnamed C++, invoked only from the PE-hook handler):
  `SimIntegrate StepSim Reconcile BallAdvance BallReconcile IngestInput SendResults
  ReplaySnapshot ReplayPostStep SendPhysImpl`.
- **Join/misc internals**: `JoinBuild SendJoin GiveCheck FetchRoles RolesFetched
  CosmeticCmp FNameResolve TextLayoutLeaf EvtDispatch PrintString BallSimGetWorld
  WorldGetNetMode A2GetNetMode` (A2GetNetMode deliberately not baked: its best
  candidate 0x14DB9C0 is a shared 483-caller resolver, not the specific impl).
- **Data tables / vtables**: `StationStr_Data/Num/Max FNamePoolTbl GtcVft ReplayVtable GtcTick`.
- **Known-bad even on 22284**: `BallNetPersp_RVA` (author already gated OFF).
- **BP member offsets**: `Panel_IsActive_Off Panel_Mesh_Off` (BP_FloorPanelB_LE_C not in SDK headers).
- Plus ~10 inline `base + 0x…` hook addresses in the boot thread (team-join, Q*,
  NetVarReg, Slate, etc.) that are likewise stale.

## Build notes (47031)
- Added `dllmain_47031.cpp` to `HalcyonA2.vcxproj` under `Condition='$(GameBuild)'=='47031'`.
- Added a 47031 block to `GameOffsets.h` (it is `#include`d via pch but **unused**
  by dllmain — the block only satisfies the per-build `#error`).
- Patched one Dumper-7 codegen bug: `WBP_StationEntry_functions.cpp:185`
  `Parms.ReturnValue` → `Parms.ReturnValue_OnMouseButtonDown` (re-apply if the SDK
  is re-dumped).
- `se_reflect.h` (SpecEditor client-mod shared header) must exist at
  `../../SpecEditor/mod/se_reflect.h` relative to `Server/`.

## Next steps (need the game running — interactive)
1. Gate the ~50 boot hook-installs behind a flag so a first inject installs ONLY
   the resolved set + ProcessEvent-passthrough + the GIsServer/GIsClient flip.
2. Inject into a local game instance; confirm it flips to listen-server (the
   build auto-listens once GIsServer/GIsClient are set) and does not crash.
3. Resolve the vtable RPCs (read AVRPawn / UAxOnlineCommunicationsComponent
   vtables) and the physics internals by runtime hook-validation + caller-anchor.
4. Point the client at the Solara backend and run a real join.
