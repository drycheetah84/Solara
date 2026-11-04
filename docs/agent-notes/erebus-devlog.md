# EREBUS PROTOCOL — development log

Evidence levels used below: IMPLEMENTED < COMPILED < COOKED < LOADED (local open) < DEDICATED-SERVER TESTED <
MULTI-CLIENT TESTED < VR VERIFIED < PRODUCTION-READY.

## 2026-11-03 16:39 — Production overhaul, session 1: baseline

**Inspected**
- Test installs carry the 16:07 cook (`Erebus-Windows_P`, 2.8 MB): 7-zone map with STALE subtitle text (baked from the
  previous script). Source map + show rebuilt 16:24 with correct text, not cooked.
- DLL `build\50451\x64\Release\HalcyonA2.dll` md5 2356f5a4 (16:06): seven-zone director (`Server\erebus_director.inc`).
- DLL log `%TEMP%\HalcyonA2.log` last written 14:11 -> no run since then had the DLL injected. Client logs show every
  event-map run (23:22, 23:52, 00:20 UTC) was a client opening the map LOCALLY (`Browse: /Game/Halcyon/...`), never a
  dedicated server. => director, gates, Wwise cues, doors, teleport, hand tracking: NEVER EXECUTED.
- pid 7292 was the user's client (server-copy exe, `-log -NewConsole`); closed with permission.

**Backups**: `HalcyonEvent\backups\20261103-1639\` (patch, DLL, builder scripts, director).

## 2026-11-03 16:45-17:10 — First dedicated-server runs (local), DLL director under test

**Fixes found by running it (all in the DLL, built locally, NOT deployed):**
1. The DLL always ran `open .../Station_Prime_P` after flipping the process to server mode, so the event map never
   loaded on a server (user pointed this out). Added `-HxMap=<package>` (absent -> Station Prime, unchanged).
2. Dumper-7 `FName::ToString()` strips everything up to the last '/', which destroyed every tag containing a Wwise
   path (0 cues/0 sfx parsed). Director now uses `GetRawString()` -> 26 cues, 8 sfx.
3. The server keeps its own local player pawn; it counted as a player. Excluded (no controller / IsLocalController).
4. Flatscreen clients have no HMD: head/hand components report exactly (0,0,0). Director now falls back to the body
   position for zeroed points (user confirmed VR clients feed real values - still to verify with a headset).

**Evidence (DLL log, local server + flatscreen client, `erebus_test_server.sh -ErebusTest` + `erebus_walkthrough.sh`):**
- Map loads via -HxMap; director finds show actor, 7 gates, 26 cues, 8 sfx, 168 objective actors, 55 beams.
- Gates: show pauses exactly at the gate time (36.0 s, 94.0 s); timeout forces the objective and resumes; gates
  are skipped when the objective finishes before the show reaches them.
- Objectives verified on the server: relays (4/4), grid (beam touch detected, end reached), valves (3 s holds),
  cores (5/5), resonators (wrong order resets, then C-A-D-B completes).

**Broken / unverified:**
- PLAYER RELOCATION DOES NOT WORK for a flatscreen client: `K2_TeleportTo` succeeds server-side but the client's next
  `Server_SetFrequentData` snaps the pawn back; `Client_HandleRestrictedMovementOfPlayer` (King's Court kick-out RPC)
  sent with clientRPC=1 but the client camera never moved (bridge status). => beam "send back" is currently a no-op.
  Next: test respawn (destroy pawn + `RestartPlayerAtTransform`) as the server-authoritative relocation.
- Test teleports only register because the director evaluates in the same tick (and "hold" mode re-pins each tick).
- Not yet verified: door sliding as seen by a client, Wwise audibility, replicated sequence on the client,
  hand tracking with a real headset, multiple clients, late join.

## 2026-11-03 17:10-17:40 — Client-side verification (capture bridge screenshots from the real client)

**Verified in the real client (flatscreen, connected to the local dedicated server):**
- The cooked map renders with A2's real materials; atrium, concourse, docking bay all load and look far better than
  the editor previews (shots: `Renders\ingame_*.png`, `fix2_grid.png`, `ctl_before.png`).
- **The replicated show sequence reaches the client**: at show t=268 s the client shows the red alarm on the
  balconies and the core swapped to the spike crystals (`Renders\ingame_alarm.png`). Sequence replication = WORKS.
- Conduit test failure was a test-tool bug (K2_TeleportTo refuses spots inside collision) -> tester now placed with
  K2_SetActorLocation(teleport, no sweep).

**Broken (root-caused as far as possible):**
- Objective actors loaded with `RemoteRole=ROLE_None` (editor saved bReplicates but not the role). Director now
  re-registers them via SetReplicates(false->true) -> RemoteRole=1 server-side, door moves server-side
  (z -27800 -> -26750, Movable, repMove=1) BUT THE CLIENT STILL SHOWS THE DOOR CLOSED. Direct actor replication of
  map actors does not reach clients (cause not found: likely A2's replication setup / net-startup handling).
- Laser beams (engine Cylinder + MI_VFX_SCRTargetBeam01a) are INVISIBLE in game (before/after control shots).
- Relay crystal OFF (CrystalEdge02a) and ON (MOVER01) both render dark green in game -> states unreadable.
- Wall builder left 2 m holes to space beside every 12 m doorway (fixed in build_erebus.py, map not yet rebuilt).
- Player relocation (beam zap, fall recovery) still has no working mechanism; respawn path coded, untested.

**Decision pending:** drive all objective visual state through small replicated LevelSequences (one per state
change, played by the DLL) since sequence replication is the one channel proven to reach clients; this also gives
late joiners the right state (replicated playback position) and lets doors/crystals animate.

## 2026-11-03 ~18:00-19:30 — Replication fixed at the source (user: "mark them for replication in the project and cook")

Root causes, each proven with client-side evidence (bridge `actors <name>` query = live client positions):
1. Stock StaticMeshActor/TextRenderActor instances CANNOT change RemoteRole / bAlwaysRelevant / NetDormancy /
   bReplicateMovement (editor: "read-only" / "cannot be edited on instances") -> they cooked with RemoteRole=None.
   FIX: class-level replication. `make_rep_blueprints.py` -> BP_ErebusRepMesh / BP_ErebusRepText (bReplicates,
   ReplicateMovement, AlwaysRelevant, DORM_Awake, NetLoadOnClient, Movable, bStaticMeshReplicateMovement). The map
   builder places every tagged objective actor as one of these (`rep_swap()` in build_erebus.py).
   RESULT (client): hidden/shown state now replicates -> counters tick (RELAYS ONLINE 0/4 -> 1/4 on the client),
   **laser-grid beams now appear** (they were never invisible; their un-hide never replicated before).
2. Replicated *movement* of map actors still never reached the client (server door z -27800 -> -26750, client
   -27800) even with all flags. Not chased further.
   FIX: state sequences (`build_erebus_stateseq.py`, run after the show builder): LS_DOORS_<objective> (doors slide
   10.5 m up in 3 s) and LS_BRIDGE (5 garden-bridge segments rise 18 m out of the chasm, staggered), replicated,
   pause-at-end, tagged EREBUS_SEQ_<NAME>. Director plays them (PlayState), rewinds them on reset, ShowActor() skips them.
   RESULT (client): relays door z -27800 -> -26750 on the client. VERIFIED.
3. Late join: a client joining after a sequence paused never evaluated it (door shut for the newcomer).
   FIX: director detects new pawns and 4 s later re-jumps every PAUSED sequence (show at a gate + door/bridge states)
   to its own position (multicast). RESULT: late joiner sees the open door (z -26750). VERIFIED.

Build order is now: dialogue -> voices -> map -> show -> **stateseq** -> cook -> check_refs -> test.
Testing trap found: stale clients survive server restarts and the capture bridge may talk to them -> always kill
every A2 process except the server before launching a test client.
Open: bay floors/lower walls look much darker in recent client shots than in the first session (cause unknown, check
after the walkthrough); crystal ON/OFF still look alike; player relocation (zap/fall) still unsolved.

## 2026-11-03 18:01 — Full walkthrough #4 (dedicated server + 1 flatscreen client): ALL 7 OBJECTIVES DONE
relays, grid (beam touch + end), valves, cores, resonators (wrong-first reset, then C-A-D-B), conduits, shards:
every one "DONE" in the server log, zero timeouts; show gates paused/resumed (shards gate paused at 520.1 s, cleared).
Client-verified state sequences: relays door z -27800 -> -26750; garden bridge segments z -28450 -> -26650 (+18 m).
Late join verified for doors. Not yet tested: late join during a show gate pause, multiple simultaneous clients,
VR hands, Wwise audibility on the client, respawn/relocation, active-crystal readability.

## 2026-11-03 18:05-18:15 — Player relocation (beam zap / fall recovery): status
- Respawn (K2_DestroyActor + GameMode->RestartPlayerAtTransform): BROKEN - A2's game mode spawns no pawn, client is
  left body-less at (0,0,0). DISABLED in the director.
- Root cause of "teleport doesn't stick": [FDSYNC] mirrors the client's FrequentData root onto the server pawn; the
  client owns its position. Any relocation must run ON the client.
- Candidate: A2's own /Game/Environments/A2/BP_Teleporter (Box overlap -> TeleportLocation). Editor stand-in at the
  same path (make_teleporter_stub.py, never cooked) lets L_Erebus place instances; at runtime the REAL BP loads
  (verified: BP_Teleporter_C_0 + its ChildActor present on the client). Pawn spawned INSIDE the box did not teleport
  (BeginOverlap needs entering). Needs a walking client (user / VR) to prove. Test teleporter removed from the map,
  recooked clean (18:09, MISSING 0).

## 2026-11-03 18:15-18:45 — Player relocation SOLVED (client-verified)
- AVRPawn::Client_TeleportPawnWithLoading = A2's own server->owning-client RPC. Director Relocate() now calls it
  (+ server copy). Client moves itself and stays; server sees it via FDSYNC.
- Verified on the client (bridge `actors`): test move to (15000,-1800,-27500) stuck;
  chasm drop (8200,6000,-29700) -> garden west edge (6000,6000,-26550);
  out of world z -31500 -> checkpoint (bay spawn while relays current);
  grid beam touch -> grid start (-11600,0,-27700).
- New FallTick: chasm box + global z < -31000 -> Checkpoint() = the doorway of the previous objective (lowered by the
  10.5 m door rise), else the bay spawn. 3 s cooldown per player, zap sound.
- Client bridge (tools/specedit50451, dsound.dll in client50451) gained `pawnto` / `pawnwalk`; they move the pawn but
  the VR pawn snaps back (its own tracking), so they are NOT a walking simulation. Teleporter-prefab route abandoned
  (BP_Teleporter is for props; stand-ins kept in the editor project, never cooked). Test teleporters removed, recooked
  clean (MISSING 0, no teleporter refs in the map).

## 2026-11-03 18:50 — PAUSED by user (back to RE work). State at pause:
- Two-client test (client-side evidence via bridges on 47650/47651, `HalcyonEvent\erebus_multiclient.sh`):
  crystals swap, counters update, 55 beams show/hide on BOTH clients. No replication graph is live (plain UNetDriver).
- OPEN BUG 1: zone-door state sequences fail on BOTH clients once a 2nd client is connected (single client works,
  before or after a gate pause; late join after opening works). Show resync ruled out (test switch). Untested:
  replicated movement via bStaticMeshReplicateMovement (switch C:\Halcyon\erebus_doormove.txt added in the director).
- OPEN BUG 2: client HUD RTT ~3800 ms on L_Erebus; user confirms ping is LOW on Station Prime and on prod, so the
  cause is our map/event code. My websocket theory was WRONG (same AWS ws errors occur on Station Prime) - reverted.
  Server CPU low (~13-25% of a core); "Skipped 16 command frames to catchup" appears with clients connected.
  Next step: make ProfDump run on every map (it is gated behind the ball-sync loop) + log max DLL tick gap, then
  compare L_Erebus vs Station Prime. Suspects: ~3,900 mesh actors / 174 always-relevant awake actors, DLL routines
  that scale with object count (ObjIndexTick, OVLEVT overlap enabling, FDSYNC near=1022).
- Test switches left in the director (harmless unless the files exist): erebus_doormove.txt, erebus_noresync_show.txt.
