# a2-50451-christmas-port

> Christmas build = 5.5.3-50451 pod-cosmetics; autonomous port of the 47031 HalcyonA2 work (seat/editor) to it
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

**LATEST (2026-10-31 10:00): STRUCT OFFSETS PORTED + CLIENT JOIN/HEARTBALL/ARENA-TICKET VERIFIED.** The RVA port
had left ~668 raw 22284 struct offsets in dllmain_50451.cpp (several were WRITES into wrong fields). 260 fixed via
work50451/applyfields.py (SDK-verified claims1-4.json) + hand fixes; see PORT-50451.md "STRUCT-OFFSET PORT".
ComponentToWorld = 0x1E0 on 50451 (0x1D0 on 47031). MMSEAT now OFF by default (-MmSeat): user corrected me —
players WALK into arenas (team-changer doors), nobody is auto-seated; free balls (HeartBall) exist outside arenas.
Still open: ball-sim arena seat gate (pawn+0x328 native, unmapped) and real hand hits (need VR).
Build server: MSBuild HalcyonA2.vcxproj -p:GameBuild=50451 "-p:SolutionDir=C:\Users\drych\ProjectDrift\Solara\\".

**(2026-10-30 23:52 PDT): 50451 STATION HOSTS.** Byte-verified RVA port applied (work50451/resolve2.py ->
apply50451.py -> verify50451.py: 159 literals, 0 stale; unresolved = HX_UNRES gated, list in tools/a2port/PORT-50451.md).
Run: `A2-Win64-Shipping.exe -nullrhi -nosound -log` (50451 install) then `bin\a2inject.exe -dll build\50451\x64\Release\HalcyonA2.dll`
-> listens :7777, Station_Prime_P loaded, 10min 0 faults. MUST be -nullrhi (render thread crashes on GIsClient flip).
UWorld::GetNetMode 50451 = 0x41FB690 (older notes saying 0x41F5A20 / RVAMAP_50451.txt values are WRONG — superseded).
Next: client join test against the 50451 server; port the client dsound.dll MCP bridge to 50451.

**Christmas build = A2 version `1.0.50451`, branch `++A2+pod-cosmetics`** (confirmed via manifest.json
`"version":"1.0.50451"`). Game install: `C:\Users\drych\Downloads\33526083366990015\33526083366990015\`
(launcher A2.exe; shipping exe `A2\Binaries\Win64\A2-Win64-Shipping.exe`, 185,906,176 B, dated 2026-10-27).
User says 50451 does NOT have the buffer-overflow/deserialize-desync crash the 47031/22284 builds had
(the SpecEditor level-tree desync + ball-sim), so the station should host cleanly without the FStrCopy/
ServerDispatch/FName ride-through guards. Task (2026-10-30, user AFK): migrate ALL the 47031 RE work
(offsets, native fns, structs, specedit.h, dllmain, the matchmaker native-seat) to 50451 + build self-test infra.

**SDK available:** full Dumper-7 dump at `C:\Dumper-7\5.5.3-50451+++A2+pod-cosmetics-A2\CppSDK\SDK` (1454 files),
plus `.usmap` (Mappings) and `.idmap` (IDAMappings — binary reflection/type map, NOT a function-RVA table).
Other builds dumped under C:\Dumper-7: 22284 (Release), 47031 (hotfix), 50451, 68365, 74462(A7).
**50451 engine offsets (Basic.hpp):** GObjects=0x0A8B0200, GWorld=0x0AACD168, ProcessEvent=0x014E1B10,
ProcessEventIdx=0x4F, AppendString=0x012B2BF0.
**Drift gauge:** AActor::RootComponent = 0x1C8 in BOTH 47031 and 50451 -> 47031->50451 drift is SMALL vs
22284->47031. So the reflected-offset re-port is a 47031->50451 SDK class diff (smaller delta set).

**My actual 47031 implementation** (to migrate FROM): `C:\Users\drych\ProjectDrift\Solara\Server\`
(dllmain_47031.cpp + specedit.h, built via HalcyonA2.vcxproj -p:GameBuild=47031; gamesdk/47031/SDK).
NOT `C:\Dumper-7\...` (those are just SDK dumps). See [a2-47031-speceditor-port](a2-47031-speceditor-port.md), [a2-47031-desync](a2-47031-desync.md).
Matchmaker native seat (just added for 47031): ADriftballMatchmaker FindArenaWithOpenSlot->ReserveArenaSlot
ForPlayer->MatchmakerFinished (MatchmakerSeat()/SafeMatchmakerSeat in dllmain_47031.cpp, g_mmSeat).

**No 50451 IDB yet** -> building one from the exe (headless idat -B, long) for native-RVA resolution. Native
RVAs needing re-resolution 47031->50451: SeSb SandboxEngine cluster, netmode-2 gates (PatchNetmode2Gates),
GetNetMode (PREPHYSBONES anchor), native manager spawner (22284 sub_5567C30), ball-sim/rollback, FName::ToString
guard range, the crash-handler IDB-base constant. Reflected offsets (specedit.h/dllmain) via 47031->50451 SDK diff.

**STATUS (2026-10-30, autonomous session):**
DONE: (1) 50451 IDB built -> `tools/a2port/work50451/A2-50451.exe.i64` (ib 0x140000000, 417648 funcs). (2) gamesdk/50451
copied + GameBuild=50451 target (dllmain_50451.cpp, vcxproj cond, GameOffsets block) — **DLL COMPILES+LINKS clean**
(build/50451/x64/Release/HalcyonA2.dll). (3) Reflected offsets: ALL sandbox classes (UPrefabComponent/AModuleSlot/
ULoadedGameMode/ATicketManager/ADriftballMatchmaker/USandboxEngine/etc.) are BYTE-IDENTICAL 47031<->50451 — the whole
specedit.h reflected port carries over; only AVRPawn drifted (CurrentRollbackSimulationOutline 0x1608->0x1820, PlayerIndex
0x15F7->0x180F, NetInputComponent 0x1560->0x1690) + UA2PawnNetInputComponent 5 later fields. Applied to dllmain_50451.
(5) MatchmakerSeat is reflection-driven (GetFunction by name) -> build-agnostic once pawn offset fixed. (6) self-test
harness `tools/a2port/selftest50451.ps1` (10 PASS/0 FAIL/1 SKIP-runtime) + inventory `tools/a2port/PORT-50451.md`.
Build fixes: 50451 Dumper-7 dump bugs (1077 DUMPER7_ASSERTS_* disabled, dup members renamed, WBP ReturnValue).
IN PROGRESS / NOT DONE: (4) native-RVA re-resolve — ~100 hardcoded 47031 RVAs in dllmain_50451/specedit.h are STALE.
GetNetMode resolved (47031 0x41F5A60 -> **50451 0x41F5A20**, same switch fn shifted -0x40); method proven. WorldGetNetMode/
A2GetNetMode + the 58 netmode-2 gates + spawner sub_5567C30 + ball-sim + SeSb cluster + VEH ranges + WriteByte sites
UNRESOLVED (multi-hour cross-build RE, IDB ready). **DLL is NOT inject-safe yet** (would patch/hook wrong 50451 addrs).
BLOCKERS: native-RVA volume (~100, hours); runtime "balls" verification needs an interactive VR/flatscreen client on a
live 50451 server (cannot drive headless). Balls NOT yet functional on 50451 (needs netmode-gate flip RVAs + runtime).
NEXT: re-resolve the netmode-2 gates (ida_nm2gates `cmp,2` scan) + WorldGetNetMode + spawner (seat-critical set), then
gate off remaining unresolved + rebuild + inject-validate.

**NATIVE-RVA AUTO-PORTER (built this session) + 87 CONFIRMED.** Cross-build matcher over both IDBs: anchor-extract
(strings/callees) -> string-anchor match -> per-region interp (deltas grow: +0x5D10@0x46x, +0x9DA0@0x53x, -0x40@GetNetMode)
-> decompiled-body-diff confirm (>=0.90). Pipeline in work50451/: anchors_a_47031.py, match_b_50451.py, resolve_c_50451.py,
decomp_47031.py, decomp_50451.py, diff_confirm.py. **Authoritative confirmed map: work50451/RVAMAP_50451.txt (+.json), 87
entries.** Confirmed incl GetNetMode 0x41F5A60->0x41F5A20, WorldGetNetMode 0x4040990->0x40465C0, A2GetNetMode 0x54AF360->
0x54DA270, NodeRemove 0x465BE70->0x4661B80, NodeDesc->0x4A902F0, FindChild->0x4A86FB0, PropString->0x46B9BC0, DescInit->
0x46C4AB0, DescCopy->0x46C48D0. REMAINING ~232/319: 77 interp-rejected + 117 unresolved (need callee-identity) + 39 DATA
globals (world-global 0xAC0A1A0, VEH bounds, byte-patch sites -> need data-xref). NOT applied to source yet (partial apply
= mixed stale/new = crash; apply needs full map or gating). Self-test: tools/a2port/selftest50451.ps1 (10 PASS/1 SKIP-runtime).

WORLD NETVARS + BOARDS (2026-11-01, verified live): 50451 world-tree root = UNetVarSystem+0x1A8 (game getter
sub_143590B00 `lea rax,[rcx+1A8h]`, called on [NetVarsManager+0x58] at 0x570E2CE) -- the old +0x150 faulted every
world write. Board/sign URLs reach the game as world netvars config/stationConfig/<key>: Solara's netvar_overrides
now emits them (the game's own include_station_config fetch is not triggered on 50451). New board uploads apply
within ~1 min, no restart. DLL source edits (dllmain_50451.cpp, specedit.h) are git-ignored; backups *.preworldroot.bak.

SANDBOX SPAWN CORRUPTION (2026-11-01): specedit.h SE_VT_LB_ADDPROPS/FILLDATA were one slot off on 50451 (207/206,
fuzzy-matched). Game's own calls prove ADDPROPS = slot 206 (0x670, 0x4A4925B (t, holder)) and FILLDATA = slot 205
(0x668, 0x49E6E49 (t, node+0x1C8 serverData, node+0x1D0 gameData)). Wrong slots scribbled into live gamemode netvar
tables and faulted (SbDefaultsCore -4 -> spawn -204); joiners then spun forever in the netvar hash-chain lookup
0x49557F0 (PC hang) or SIGSEGV (Quest) right after "Received Base data for gamemode". Server restart clears it.
Hang-diag trick: sample the hung PC client's busiest thread RIP (ctypes GetThreadContext) -> RVA via the
solara_version.log "hooked at" line (base = that - 0x79AFEB0).
FIXED + VERIFIED 2026-11-02 00:14: also needed the "current sandbox engine" global qword_14ACA62A8 (RVA 0xACA62A8,
=USandboxEngine; BindProps reads [g+0x1B0] PrefabTypeManager) set around the template virtuals (game sets it at
0x4A49251, clears at 0x4A4926B). With slots 206/205 + the global: sandbox spawn "defaults from 1 template(s), 3 default
node(s) ... result 6", and a fresh client joined afterwards without hanging.
FINAL 2026-11-02 01:09 (verified: Text + ProgressionButton placed with "defaults from 1 template(s), 1 default node(s)
result 6", fresh join loaded 60 gamemodes, no crash): defaults ARE required on 50451 (else client 0x55CD1C0 reads
default__layoutSettings unchecked and crashes) -- but CLONE, never move: per-type clone table is a TArray at
0xA62BFB0 (data ptr; Num at 0xA62BFB8), cloneFn(&out, src), copy byte +26, RawAdd the clone, then free the holder
with 0x49750C0 (type-dispatched destroy). Moving the original node out of the holder = corrupt replicated data.
GAIA: 3 AnnouncementManagerComponents exist; two belong to CDOs (Default__AxGameState / Default__BP_A2GameState_C) --
broadcast only on the one whose owner is a live BP_A2GameState_C.
