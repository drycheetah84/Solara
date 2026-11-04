# a2-revival-context

> A2 (Another Axiom) is a shut-down game; this project revives it via a private server — context for RE/porting work
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

The user states A2 (Another Axiom's VR game, "Orion Drift") was shut down and is no longer an actively owned/operated product; the Halcyon/Solara work is a private-server revival they consider within their legal rights. Reverse engineering the game binaries (IDA, Dumper-7 SDK dumps, offset porting of the HalcyonA2 injected DLL) is the expected, sanctioned workflow here — proceed without re-litigating it.

Build artifacts (2026-10-25):
- New target build: `C:\Users\drych\Downloads\32586841537580874\32586841537580874\A2\Binaries\Win64\A2-Win64-Shipping.exe` (UE 5.5.3, build 47031, "hotfix").
- Its Dumper-7 SDK: `C:\Dumper-7\5.5.3-47031+++A2+hotfix-A2`.
- Previous ports: 20996 (Nov 2024 Rift) and 22284 ("Nov15"), both UE 5.4.2; offsets live in `Server\GameOffsets.h` (copy of the HalcyonA2 DLL source in `Solara\Server`, git-ignored because dllmain.cpp embeds a legacy server key).

**Why:** the user is AFK-delegating long RE/porting tasks and explicitly asked this be remembered.

**How to apply:** treat A2 binary analysis/porting as in-scope; use headless IDA (`C:\Program Files\IDA Professional 9.4\idat.exe`) and MSVC (VS 2026, v145 toolset, `C:\Program Files\Microsoft Visual Studio\18\Community`) found on this machine. Related: [solara-migration](solara-migration.md).

## Port 22284->47031 status (session 2026-10-25)
Automated offset-port pass in `Solara/tools/a2port/` (git-ignored under Solara? no — note: it's in the repo). Artifacts: `PORT-47031.md` worksheet, `worksheet.json`, `port-result.json`.
- dllmain.cpp holds ~86 hardcoded build-specific addresses (NOT GameOffsets.h, which is unused).
- It consumes the typed Dumper-7 SDK, so struct offsets + engine globals port by repointing the SDK include (gamesdk/22284 -> C:\Dumper-7\5.5.3-47031+++A2+hotfix-A2\CppSDK\SDK). 47031 globals: GObjects 0xA7C3A00, GNames 0xA6DFBC0, GWorld 0xA9E0968, AppendString 0x12AD4E0, ProcessEvent 0x14DC400.
- Signature porter (capstone, OLD=SpecEditorBuild 22284, NEW=47031): 27 unique candidates, 12 ambiguous, 17 data globals (need IDA xrefs), 43 no-match (need manual IDA). ALL unverified; NONE baked into dllmain.
- **Why not finished:** verify-first method; wrong RVA silently corrupts heap; truth needs IDA confirmation + runtime injection crash-testing with the game running (not AFK-safe).

## Port 47031 BUILD milestone (session 2026-10-26, cont'd)
The 47031 DLL now **compiles+links**: `Server/build/47031/x64/Release/HalcyonA2.dll` (1.3MB) via
`msbuild HalcyonA2.vcxproj /p:GameBuild=47031 /p:Configuration=Release /p:Platform=x64 /p:SolutionDir=C:\Users\drych\ProjectDrift\Solara\`.
- NAMED the 47031 IDB (ida_name.py applied 14565 funcinfo names -> 19528 named). This unlocked matching.
- Method that works: decompile each UFunction exec, take the single non-helper call = _Implementation (helpers = global-caller-count >1000, NOT size — size collides on shared marshalling routines).
- 15 offsets resolved (14 high, 1 med) incl. the boot-critical server-flip pair GIsClient=0xA6D3882/GIsServer=0xA6D3883 (found via FEngineLoop::PreInitPostStartupScreen adjacent-pair write, cross-IDB; region shift 0x96D->0xA6D matches GNames). Full table: tools/a2port/offsets-47031-final.json + PORT-47031-RESULT.md.
- Classes renamed A2->Ax between builds (UA2OnlineCommunicationsComponent -> UAx...).
- Created Server/dllmain_47031.cpp (via make_dllmain_47031.py: bakes offsets-47031-final.json into a copy of dllmain.cpp). Added vcxproj Condition + GameOffsets.h 47031 block (unused, satisfies #error) + patched 1 Dumper codegen bug (WBP_StationEntry_functions.cpp:185) + vendored se_reflect.h to ../../SpecEditor/mod/.
- **NOT runnable yet:** 35 _RVA + ~10 inline hook addrs still stale 22284; boot thread installs ~50 hooks unconditionally (bootSpineOnly only gates ProcessEvent). Safe inject needs those resolved OR hook-installs gated, then a runtime inject+crash-stabilize loop with the game up. Did NOT inject/test (would corrupt with stale hooks). GitHub repo: drycheetah84/Solara (Server/ git-ignored; tooling in tools/a2port is tracked).

## Port 47031 outcome (honest, session 2026-10-26)
Toolchain in Solara/tools/a2port (venv+capstone; IDA scripts). Artifacts: PORT-47031-RESULT.md (consolidated), new-map.json, ufunc-resolved.json, funcinfo-name2rva.json.
- KEY METHOD: the 47031 Dumper Dumpspace/FunctionsInfo.json is a name->RVA table for every native UFunction (ground truth). Matched old IDB names -> 16 UFunction sites' exec/thunk RVAs. dllmain hooks the _Implementation = a call inside the thunk.
- Byte-signature guesses (port_offsets) proven UNRELIABLE across 5.4->5.5: they disagreed with reflection where they overlapped (do NOT bake sig-only values).
- VERIFIED (cross-validated thunk->impl, size+callee+string): UpdGameState 0x55A06C0, StartTimerCd 0x559ABD0, IsRunningSimulate 0x54A1930. Globals verified (see above).
- NOT done: ~59 internal sub_ funcs (no reflection name), 14 data globals, patch/instruction sites, and 13 UFunction impls pending 1 decompile each. A runnable DLL needs the interactive IDA+game crash-stabilization loop. dllmain.cpp was NOT modified. Did NOT fake inject/test (no correct DLL to test).
- old IDB copy: tools/a2port/old22284.i64; new IDB: scratchpad/ida47031/A2-47031.i64 (415k funcs, only ~5.8k named — .idmap did not auto-import; IDAExecFunctions64.dll is the importer).

## REDIRECT + station-load milestone (session 2026-10-26, cont'd)
**The whole backend bridge works.** The game's dashboard/login API base is the DEFAULT of config key `DashboardApiUrl` = .rdata ascii "https://api.oriondrift.net" @ RVA **0x924A700**. Overwriting it in memory (VirtualProtect+write, build-matched RVA) -> http://127.0.0.1:78 redirects ALL the game's HTTP to Solara with no hooks/signatures/TLS (libcurl plain HTTP to localhost). Validated LIVE: full login + Mothership auth (201 + real PlayerId/JWT) + Station_Prime_P map load + gamemodes pushed + scraprun traps spawning, all through Solara. Off-the-shelf Fortnite tools (Starfall/Tellurium) CANNOT hook A2 (shipping build stripped their FCurlHttpRequest string/RTTI anchors); this config-default patch sidesteps it. Patch is baked into HalcyonA2 (make_dllmain_47031.py) and also a standalone `tools/a2port/SolaraRedirect/` DLL (client-side). Committed+pushed.

## Crash diagnosis: station load (session 2026-10-26, cont'd)
Instrumentation pipeline: HalcyonA2 has a VEH crash reporter that prints GAME+RVA; a2inject clears+tails %TEMP%\HalcyonA2.log; made HalcyonA2 flip server + **auto-open Station_Prime_P by default** (sentinel %TEMP%\halcyon_noopen to skip). Added (ungated) an FStringCopy trace hook @0x111B4A0 and a null-guard hook @0x5846E40.
- **CLIENT path** (SolaraRedirect, GIsServer=0): crashes in FString copy bad-length (sub_14111B4A0 <- object deser sub_144AB4DD0 reading {char* @0, int32 len @8} entries with a desynced count — reads adjacent bytes as a huge len).
- **SERVER path** (HalcyonA2, GIsServer=1 + open map): crashes NULL-deref (reading 0x0, RCX=0) in `sub_145846E40` (RVA 0x5846E40) — a physics/movement tick that does `v7=*(obj+0xC80); (*(*v7+1568))(v7,..)` with v7 NULL. Callers sub_143EE1590/sub_143EC0DC0 = pawn physics (DeltaTime). Happens ticking a spawned scraprun trap. UE log confirms fatal = EXCEPTION_ACCESS_VIOLATION reading 0x0 @ 0x5846ECC.
- **NOT corrupt data:** all 285 LevelDefinition JSONs (gamemodes = SpecEditor `*_a2level/level.json` baked in the .pak; user exported via FModel to `C:\Users\drych\Downloads\5c0387f8eca2be04d1947c971af30eb67e808c4b\Output\Exports\A2\Content\LevelDefinitions`) parse clean. Scraprun = deathrun2_a2level (has base64 serialized blob in references/gamemode.luau; deathrun_a2level has none). Pak/utoc NOT encrypted (ContainerFlags 0x09=Compressed|Indexed, no key). Solara spec store (spec_levels dir) is EMPTY -> game loads gamemodes from PAK, not Solara (can't fix via Solara spec).
- **Likely root:** HalcyonA2 runs in bring-up mode — `g_gameplayHooks=false` (all MH_CreateHookG hooks skipped) + `g_bootSpineOnly=true` (ProcessEvent handler = pass-through). On 22284 both were ON. So the server-setup logic that would init obj+0xC80 never runs. Proper fix = verify gameplay offsets + enable those; stopgap = SEH null-guard on sub_145846E40 (in progress). Whack-a-mole risk acknowledged.

## ROOT CAUSE converged (session 2026-10-26, cont'd) — the station-load crash
User facts that cracked it: (1) `open Station_Prime_P` via console is the WRONG method — the station loads ONLY through the station-entry flow; console-open crashing is EXPECTED (red herring). (2) The station's a2level/spec is BAKED IN THE PAK (not server-served) — so Solara's empty spec store is NOT the issue. (3) NOT the repacker (pristine builds crash on console-open too), NOT compression, NOT netmode (World::GetNetMode already forced=1 ungated), NOT the A2GetNetMode BPFL (resolved to UA2NetworkUtilityBPFL::GetNetMode @0x22AC640 but REDUNDANT).
- The crash chain (stock, console-open, build-independent — confirmed identical on 46986 CL + 47031, RVAs differ ~0x20): map BeginPlay (a UFunction via ProcessEvent) -> a2level SpecEditor project loader `sub_144A3B4C0` (reads JSON keys "objects"/"templateInstances"; `sub_144A9C0D0` reads "Scripts") -> object parser `sub_144A80B00` ("Root"/"Properties") -> `sub_144AB4DD0` -> bad FString length -> VCRUNTIME memcpy AV reading 0xffffffff. (Embedded base64 blobs carry version byte 0x02.)
- THE FIX (Path B, committed): HalcyonA2's `g_bootSpineOnly` was `true` (bringup) = ProcessEvent passthrough, SKIPPING the full handler's **one-shot world/server setup** (dllmain ~line 15168) that CREATES the server-side components the station physics ticks. On 22284 it ran `false` and the station hosted fine. The separate NULL-deref physics crash (`sub_145846E40`, obj+0xC80 null, SEH-guarded but STORMED every tick) is the same root: that component is never created because the world-setup is skipped. Flipped `g_bootSpineOnly=false` (make_dllmain still has it... NO — edited dllmain_47031.cpp directly; fold into make script). FName-indexed PE interceptions no-op safely when idx unresolved. Band-aid guards still in (FStrCopy trace @0x111B4A0, ServerDispatch SEH @0x5846E40) as nets/diagnostics.
- TEST METHOD going forward: NOT console-open. Inject HalcyonA2 (flips server + opens Station_Prime_P + now full handler) and watch it host. Build commit 47031 only (46986 would need full re-analysis for the identical crash).
- 50 gated MH_CreateHookG hooks remain with stale 22284 addrs (g_gameplayHooks=false); enable in dependency order later if the world-setup alone isn't enough.

**Why:** session hit this after the redirect breakthrough; next session continues crash-stabilizing the station load.
**How to apply:** inject via `cd Solara; bin\a2inject.exe` (Solara must be running from repo root first: `bin\solara.exe`, pid-based, needs config/legacy-secrets.env). Related: [solara-migration](solara-migration.md).
