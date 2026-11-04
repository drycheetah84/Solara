# a2-47031-speceditor-port

> SpecEditor (in-game level editor) ported 22284->47031 — server specedit.h + client dsound.dll, both built
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

SpecEditor = the in-game ImGui level editor for A2. Client mod (`dsound.dll`) tunnels editor commands
(`SE|SPAWN/XFORM/DELETE/QUEST/ENTER`) to the server over a replicated FString RPC
(`Server_AttemptLockObject` / `Server_SetVivoxParticipantID`, `SE|` prefix); the server (`specedit.h`,
included by [a2-47031-desync](a2-47031-desync.md)'s dllmain_47031.cpp) spawns LE_-prefab actors replicated via Iris.
Off by default (`-SpecEdit`), `LE_`-class-fenced, dashboard-grant scoped, SEH-guarded. Original 22284
source: `C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\` + `...\HalcyonA2\HalcyonA2\specedit.h`.

**2026-10-30 PORTED 22284->47031 + BUILT (both halves).** Tracker: `tools/a2port/PORT-SPECEDIT-47031.md`.
- **Server** `Solara/Server/specedit.h` (git-ignored): was a verbatim 22284 copy wired into dllmain but with
  stale offsets. Transport + SPAWN/XFORM/DELETE are SDK-based (portable). Re-resolved 24/33 SandboxEngine
  RVAs (cross-build IDA: unique-string anchors, StringNodeFactory call-site alignment, callee-identity,
  decompile body compare). Key: Malloc 0x54FF5E0, MemFree 0x1177030, StrAssign 0x111B4A0, LeafCtor 0x4959030,
  NodeBuild 0x4A66770, NodeDesc 0x4A87370, AddChild 0x4948C90, Templates 0x49E0870, ContainerCtor 0x4958A00,
  ContainerStat 0x4954550, LeafStat 0x4A61380, StringStat 0x4954370, StringNodeFactory 0x494F200, FindChild
  0x4A7E030, DescFree 0x495A350, NodeMerge 0x4A4AF80, NodeFinish 0x49F8F90, BindProps 0x49D5750, StringNodeVtbl
  0x8D0A1F8, BroadcastEventString 0x4A09560, kPhysSetProfile 0x5595AC0, kLuaTrigger 0x5598B60, kLuaNoColl
  0x556F190, kLuaDefault 0x5596130. SandboxEngine module relocated 22284 0x46-0x47xxxxx -> 47031 0x4A-0x4Bxxxxx
  (NodeBuild delta +0x34F790). 9 TODO (DescInit, DescCopy, NodeRemove, PropString, RawAdd, LeafVtbl,
  compBundleRemoving=A2PlayerQuestComponent::Server_SetQuests_Impl, worldSub, subFs) — direct-call-only, SEH-guarded.
  Built HalcyonA2.dll 17:53.
- **Client** staged at `Solara/tools/specedit47031/` (copied from rigel 22284 mod, SDK includes repointed to
  the 47031 SDK). **Compiles clean against the 47031 Dumper-7 SDK (API stable across builds)** — only raw
  GAME-struct offsets in se_game.cpp drifted. se_reflect.h (UE-core reflection) verified STABLE (no change).
  Ported offsets: RootComponent 0x1A8->0x1C8, Pawn 0x2D8->0x2F8, AcknowledgedPawn 0x340->0x360,
  PlayerCameraManager 0x350->0x370, CameraCachePrivate 0x13A0->0x1420, AModuleSlot::SlotID 0x390->0x3F0,
  StaticMesh 0x560->0x538, shape dim 0x540->0x518, capsule radius 0x544->0x51C, actor-flag byte 0x65->0x69
  (AxHiddenBits int32 inserted @0x60), gravity-volume cluster +0x20, ModuleSlot TicketManager/LoadedGameMode/
  ModuleState 0x438/0x440/0x300 -> 0x488/0x490/0x360, BP_LevelEditorPawn::Cube 0x440->0x488. UNCHANGED
  (verified): USceneComponent transform base 0x1D0 (RelativeLocation 0x130 both builds), FHitResult::Component
  0xD8. TODO (secondary/diagnostic only): tm+0x328, l+0x320, [tcnodes] b+0x260/0x268/0x508/0x524, comp+0x288/
  0x479, r+0x58, p+0x140; client RVAs entitlement 0x5429424 (redundant w/ version.dll), GetClientProgression
  0x46849A0, ATicketManager::OnRep_VerifiedTicketHolders 0x4728BE0. Built dsound.dll 18:23 (2.27MB) BUILD_OK.
  Build script: `tools/specedit47031/build47031.cmd` (cmd /c via powershell; CL env var must NOT be used as a
  var name — reserved by cl.exe; invoke via `cmd /c` not git-bash `cmd //c` which goes interactive).

**Deploy:** server game instance launched with `-SpecEdit -SpecEditLocalTest` (specedit reads GetCommandLineW);
client `dsound.dll` copied next to A2-Win64-Shipping.exe alongside version.dll (different proxy, coexists).

**2026-10-30 LIVE-TESTED + FULL OFFSET RE-PORT.** Editor core WORKS in-game (spawn/palette/place/select confirmed).
Fixes this session: (1) FName-guard VEH ([a2-47031-desync](a2-47031-desync.md)) so the jakeball ball-sim 0x1FFFE AV no longer
crashes the server. (2) Client dsound.dll: join-guard disabled (stale 0x4728BE0 hook corrupted the pump -> empty
editor); QuestAssets icon read UQuestAssets+0x30 fixed TMap<FName,UObject*> -> TMap<FName,int32> (TextureIcons
gone in 47031, now TextureArrayIndexies) which had AV'd PumpImpl every tick before Publish; le_catalogue.txt
REGENERATED from the 47031 prefab dump (Downloads\5c0387f8...\Output\Exports\A2\Content\A2\Prefabs, 251 LE_
prefabs) -> palette fills. (3) **FULL server specedit.h raw-offset re-port**: UPrefabComponent / ULoadedGameMode /
AModuleSlot / ATicketManager / ATeamChangeActor / USandboxEngine / UObjectPrefabDefinitionSettings / UModuleState
all restructured in 47031; built per-class remaps from both SDKs + classified all 164 uses per-line (5 parallel
agents) -> 146 edits applied; PrefabHandle(pc)+GamemodeHandle(l) validated scanners for the two non-reflected
NetVar root handles (pc+0x3A0, l+0x218); SbNodeOf chunk-head 408/432->416/456; NODE/Desc struct confirmed stable
(NodeDesc=node+168 both builds). bActorIsBeingDestroyed 0x65->0x69, AActor repl-flag 0x60->0x64, APawn::Controller
0x2D0->0x2E8. Details + remaps in tools/a2port/PORT-SPECEDIT-47031.md. Server HalcyonA2.dll rebuilt 21:40.
KNOWN SECONDARY GAPS: sandbox-prefab palette (USandboxEngine RawPrefabs sb+0xF8 non-reflected), UObjectPrefab-
Definition Blueprint 0x30->0x38/Settings 0x38->0x40 (not yet applied), LuauBehavior bound-fields +0x3C0/0x3C8,
team-size panel (AModuleSlot::MaximumTeamSizes REMOVED in 47031 -> now on the TicketManager). Game-data (SBDATA)
should now read (handle resolves + offsets correct + node struct stable) -- verify live.
