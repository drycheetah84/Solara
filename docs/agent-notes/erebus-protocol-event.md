# erebus-protocol-event

> Halcyon's first custom live event \"EREBUS PROTOCOL\" (user-chosen name) — plot, lore sources, asset pipeline, project paths
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

Event name **EREBUS PROTOCOL** (user chose it 2026-11-03 over "SIGNAL // CYPHER"). ~12-minute synced show at the plaza.
Plot (canon-based, lore PDF `C:\Users\drych\Downloads\DEFRAG_FILE_DIRECTORY.pdf`, narrated by CODEX; digest text at
scratchpad `pdf/paged.txt` if still present): teasers = CODEX recovering a file + boards flashing binary
"WHERE DID CYPHER GO" + Echo Bot scavenger hunt -> event: GAIA welcome, Commodore Vector: crystal output spiking past
2.718%/cycle -> EREBUS (security protocol from the Frag, p.11/97) takes over boards -> emergency quantum-tunnel jump
(game's own skybox-transfer sequence SQ_VFX_SkyboxTransfer01a, nebula sky Prime_Vista_Nebula2) -> trapped H0w-RD
C0sine broadcast -> Cypher (missing Rust Runners legend, Unit 17) returns on the Driftball court and waves -> EREBUS
was a homing protocol (keep partly mysterious) -> fireworks, Spark + Crankshaft commentary, exhibition match.
Reward: attendee-only "Cypher's Finish" cosmetic (canon: Spark's Supplies sells vintage finishes). Open decisions:
voice lines (actor/synthetic vs text), Cypher permanent vs tease, date.

Pipeline: game = UE 5.5.3 (`Unreal Engine 5.5.3` string; IoStore v8; paks unencrypted). Project
`C:\Users\drych\HalcyonEvent\A2\A2.uproject` (UE 5.5.4 at `C:\Program Files\Epic Games\UE_5.5`, Python works headless:
`UnrealEditor-Cmd.exe <uproject> -run=pythonscript -script=<py> -unattended -nop4 -nosplash -NullRHI`; Android
platform NOT yet installed). Exports via local UE4Decompiler clone `C:\Users\drych\tools_ext\UE4Decompiler` (patched:
Oodle init from oodle-data-shared.dll, export --mapping, NAudio.Core/FixedMathSharp refs) with usmap
`C:\Dumper-7\5.5.3-50451+++A2+pod-cosmetics-A2\Mappings\...usmap` -> `C:\Users\drych\HalcyonEvent\Exports` (glb/png/json),
imported 208/208 to matching /Game paths (`Scripts/import_exports.py`). Asset list: repo `tools/event/SIGNAL_CYPHER_EXPORTS.md`.
Cypher body = /Game/A2/Characters/A2RobotC (team logos + numbers atlases); Orion ship = /Game/A2/Enviro/Vista/Orion/SM_OrionShip_02.
Animation method: [a2-robot-animation](a2-robot-animation.md).

**CUSTOM MAPS LOAD IN A2 (2026-11-03, smoke map L_Erebus_Smoke confirmed: "LoadMap complete" in the client).** Recipe =
`C:\Users\drych\HalcyonEvent\cook_patch.ps1 -Maps <map>`: UAT BuildCookRun ... -iostore **-VersionCookedContent**
(MUST: unversioned cooks crash A2 — "ObjectSerializationError ... Bad export index", A2's engine fork has different class
layouts; and do NOT set CanUseUnversionedPropertySerialization=False globally, the editor asserts), then UnrealPak IoStore
with a response file filtered to `\Content\Halcyon\` only (never ship engine content or our shader archive, both named
like the game's) -> `Erebus-Windows_P.utoc/.ucas` + a tiny companion `.pak` (mount point ../../../A2/Content/Halcyon/).
The script holds aside the user's UE4.21 `%APPDATA%\Unreal Engine\UnrealBuildTool\BuildConfiguration.xml`
(Compiler=VisualStudio2017 breaks UAT 5.5) and restores it. UnrealPak command files: use forward slashes. Test installs:
hardlinked server copy `C:\Users\drych\HalcyonEvent\ServerCopy` (cp -al; add files only) + client50451. The client
ignores a map on its command line (boots EntryLevel) — the server loads the map, the client follows on connect.
Full-quality map porting is impractical (geometry = Simplygon stand-ins/premerged across ~80 sublevels); build event maps
from A2 kit pieces (exported: 1,641 meshes). Own extractor for map-embedded meshes: `C:\Users\drych\HalcyonEvent\MapMeshExtract`.

**Event map + show (2026-11-03, v2):** `A2\Scripts\build_erebus.py` builds `/Game/Halcyon/Events/Erebus/Maps/L_Erebus`
(hangar 48x48 -> 64 m corridor -> 120x120 open atrium, ~1630 A2 kit meshes referenced by real game paths; crystal core =
SM_PKR_Crystal_ShardLarge01a/Med with A2 MIs CrystalEdge02a (calm) / MOVER01 (spike); doors Door_<Hangar|Atrium>_<L|R>);
`build_erebus_show.py` keys LS_Erebus_Show (doors 1:00/2:00, awaken 3:00, red alarm+gold crystals 5:00, jump flash 7:00,
Orion fly-past, calm 8:00, finale 10:00, end 12:00; env HX_SHOW_SCALE time-compress, HX_SHOW_AUTOPLAY) + replicated
LevelSequenceActor. Preview: `render_preview.sh <name>` (HX_SHOW_TIMES/HX_SHOTS env for beat stills). Ref check:
`check_refs.py scan.json <cooked pkgs>` (must be MISSING_TOTAL 0). Gotchas: headless -> no set_folder_path, no
spawn_actor_from_object; save_dirty_packages after save_current_level; UE glTF import keeps only UV0/UV1 (A2 swatch UV is
glTF UV2 -> glb_swatch_uv.py for preview stand-ins); A2 lights ~15 cd, fixed exposure; visibility-track key = VISIBLE.
v3 (same day): VFX = A2's own assets only (A2 has NO Niagara; effects are mesh+material BPs): fireworks =
SM_VFX_FirePlane01a + MI_FlipbookSpriteAlpha01a (additive unlit 2-sided quad, scale ~12-18), jump portal = SM_PortalEdge
(MI_PortalEdge_01) + /Game/StarterContent/Shapes/Shape_Plane (MI_PortalCard_01), Orion + SM_OrionShip_04_Thruster2.
SFX/VO = our own SoundWaves (UE audio mixer runs alongside Wwise in A2; OGG/OPUS/BINKA decoders loaded) generated by
`HalcyonEvent\gen_audio.py` (SAPI Zira=CODEX, David=EREBUS/CYPHER + ffmpeg FX; cues.json drives subtitles+audio tracks).
Cypher = SkeletalMeshActor SK_A2RobotD (project skeleton path == game's), scale 1.4; ROBOT PIVOT IS AT HEAD HEIGHT (mesh
hangs ~0.95 m below) - land at floor+0.95*scale; Cypher's Finish via MovieSceneComponentMaterialTrack (5.5 API:
add_color_parameter_key(MaterialParameterInfo(name,GLOBAL_PARAMETER,-1), frame, value, "", "")). Show start in prod:
DLL [SHOW] (built 2026-11-03, NOT deployed) adds "!show:start|stop|restart" to the GAIA event list when a
LevelSequenceActor is loaded (dashboard GAIA panel buttons) + `-ErebusShowAt=<unix>`; build the prod show with
HX_SHOW_AUTOPLAY=0 HX_SHOW_SCALE=1.

v4 (user feedback "isn't close to a good event": wanted Fish Audio VO, real Cypher animation, objectives, real SFX):
- VO = Fish Audio (`HalcyonEvent\gen_voice_fish.py`; key in `HalcyonEvent\fish_audio.key` - never print; voices in
  `fish_voices.json`: CODEX=female 9335..., EREBUS=male bb0f..., CYPHER=adcb...). Script/gates = `gen_audio_lines.py`.
- SFX = A2's own Wwise events, fired by the DLL ([EREBUSDIR], built not deployed) from EREBUS_CUE tags on the show
  actor (LoadAsset_Blocking by path + A2AnnouncementManager broadcast). Synth SFX disabled (USE_SYNTH_SFX=False).
- Objectives (DLL, server-side): relays (touch 4 kiosks) -> security laser grid (beam touch = K2_TeleportTo corridor
  start; done when half the players reach x>=108 m) -> 6 conduits (hold; charge 0-100%) -> 30 shards (hand pickup).
  Map actors tagged EREBUS_* + replicated (SetActorHiddenInGame replicates); show pauses at EREBUS_GATE|id|t|timeout.
  windows.h renames GetCurrentTime -> call the UFunction via ProcessEvent.
- Cypher clips: `HalcyonEvent\anim\cypher_moves.py` (Blender; reset bone SCALE every frame or the rig explodes;
  FBX export must use the default bake_anim_use_all_actions - False exports NO animation; delete actions between clips)
  -> `A2\Scripts\import_cypher_moves.py` (Interchange, one task per call) -> AS_Cypher_<Clip>.
- v5 (user: "map too small... want 12-25 min gameplay, big map"): 7 zones in `A2\Scripts\zones_1_5.py` (exec'd by
  build_erebus.py): DOCKING BAY x-216..-136 (spawn, relays) -> airlock -> CONCOURSE x-120..0 (laser grid) -> REACTOR
  SHAFT x0..40, floor FZ-24 (3 valves, hold 3 s) -> ARCHIVE x-4..44 y32..80 @FZ+12 (5 data cores) -> CRYSTAL GARDEN
  x52..112 y30..90 @FZ+12 (resonators in order 2,0,3,1; bridge EREBUS_BRIDGE_n revealed) -> atrium W balcony -> ATRIUM
  (conduits, spike, jump, Cypher) -> OBSERVATION DECK x232..296 (shards). ~3900 meshes, ~220 point lights (perf risk,
  esp. Quest). Director moved to `Server\erebus_director.inc` (git-ignored like dllmain): chain + doors
  EREBUS_DOOR_<obj>_<n> slide up via K2_SetActorLocation (Movable + replicated movement). Script v3 = 26 Fish lines;
  show clock: atrium beats authored in v2 time, F() remaps >=180 by -40, INV() for new-clock values.
- Unverified in game: fireworks/portal looks, Wwise broadcast audibility, teleport on VR pawns, hand positions server-side.

**Working UE loop (2026-11-03, first good render `Renders\Erebus_UE_Test1.mp4`):** `C:\Users\drych\HalcyonEvent\render.sh <name>`
rebuilds `Scripts\build_test_sequence.py` (level L_Erebus_Test + LS_Erebus_Test) headless, renders via `-game
-MovieSceneCaptureType=/Script/MovieSceneCapture.AutomatedLevelSequenceCapture -MovieFormat=PNG`, grids frames
(`sheet.ps1`); ffmpeg via WinGet Gyan.FFmpeg. Gotchas: (1) Git Bash rewrites `/Game/...` args -> prefix with
MSYS_NO_PATHCONV=1; (2) `unreal.Rotator(a,b,c)` positional = (roll, pitch, yaw) — always use keywords; (3) glTF exports
are in METRES (ship ×0.05 ≈ 100 m); (4) the player robot in the project is the Creator Kit FBX
(`replace_robot_with_creatorkit.py`) — glTF/glb skeletal exports (RobotD, RobotC) have mismatched bind poses and fly
apart when animated; (5) Python cannot create skeleton sockets in 5.5 (SocketName read-only); (6) game robot paint =
MM_Character params (ColorA/B, Mask Index, Badge Index, Player ID atlas) -> drive the game's MI_A2RobotD with a runtime
MID, never ship a stub parent material. Cypher's Finish v1 = PatternRebarRunner (mask 23) + rust/gunmetal/teal.

## Replication + relocation facts (2026-11-03, client-verified)
- Objective actors must be BP_ErebusRepMesh/BP_ErebusRepText (class-level replication; stock instances cook RemoteRole=None). Hidden state replicates.
- Actor MOVEMENT never replicates -> doors/bridge move via replicated pause-at-end state sequences (build_erebus_stateseq.py, run AFTER the show builder; tags EREBUS_SEQ_*; ShowActor skips them). Director re-jumps paused sequences 4 s after a new player joins (late join verified).
- Build order: dialogue -> voices -> map -> show -> stateseq -> cook -> check_refs -> test.
- Client owns player position ([FDSYNC] copies client root to server pawn). WORKING relocation: AVRPawn::Client_TeleportPawnWithLoading (server->client RPC) - used by Relocate(), beam zap and FallTick (chasm + z<-31000 -> checkpoint). Respawn DESTROYS the pawn (disabled). Bridge pawnto/pawnwalk snap back (not a walk sim).
- Client-side truth: bridge `op "actors <name>"` gives live client actor positions. Kill ALL A2 processes before a test (launcher does it now).
- Full walkthrough #4: all 7 objectives DONE on the dedicated server, zero timeouts.
- PAUSED 2026-11-03 18:50 for RE work. Open: doors fail with 2 clients; RTT ~3.8 s on L_Erebus only (NOT the websocket - Station Prime/prod fine). See DEVLOG.md 18:50 entry.
