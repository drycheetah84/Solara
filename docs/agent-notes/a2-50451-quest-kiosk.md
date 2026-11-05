# a2-50451-quest-kiosk

> Quest kiosk Unlock fix (50451): stuck quest GROUP flag blocked every progress push; DLL flush+save pass; local test rig; a2inject trap
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

**Root cause (verified live 2026-11-01):** the kiosk Unlock sends `Server_SetProgress(questLine@widget+732, 255)`; the
server ACCEPTS it (stores in UServerProgression PlayerInstances @sp+976, 528-B entries, marks dirty) but the push to the
component (`sub_1449B8E90` -> comp.QuestProgression @0x130, push-model rep idx 3) only runs while the entry's GROUP flag
(entry+224, set only by Server_StartGroup_Impl 0x49B3AF0) is 0 -- and on our server it is left 1. Fresh join + press:
nothing for 20 s; `Server_StopGroup` -> pushed instantly.

**Fix DEPLOYED 2026-11-02 04:10 (prod confirmed saving). Follow-up `QuestOwnerValid` skip (stops 'invalid pawn' log spam from leftover controllers) built + locally verified, NOT deployed yet.**
`Server/dllmain_50451.cpp` `QuestFlushAndSavePass` (runs with InitPlayerQuests ~2 s): calls Server_StopGroup_Impl
0x49B3C10 on each registered comp, then saves comp progression to backend `player_quests` (byte-1 + base64 JSON) on
change; never saves for an org whose fetch failed (`g_qFetchFailed`). Backup `.prequestflush.bak`. DLL at
`build\50451\x64\Release\HalcyonA2.dll`. Local results: unlock replicated in <5 s, `[QSAVE] ... http=200`, and after a
full server restart `fetched 1 real quests` -> client had it.

**Facts:** 37 "could not find quest" errors are from jakeball/KingsCourt Luau `setQuestCompletedForAllPlayers` (cloud
quest defs not loaded on the server), NOT the kiosk. Kiosk tasks aren't activation quests (SetProgress on them errors).
50451 client never writes player_quests_v2 (load = MISSING -> defaults). Client bridge ops: qkiosk/qset/qclear/qrpc/qprog.

**Local test rig (no prod impact):** `C:\Users\drych\solara-local` (DB snapshot + Windows solara.exe; run with
`DASHBOARD_BIND_ADDRESS=127.0.0.2` because Steam holds 127.0.0.1:8080), server from the ORIGINAL install
`33526083366990015\33526083366990015` with `-abslog` and backend 127.0.0.1, client `-SpecEditConnect=127.0.0.1:7777`.
Re-snapshot the DB after the client logs in to prod (its new dashboard key must exist locally, else org stays empty).

**TRAP:** `bin\a2inject.exe` IGNORES positional args -- use `-pid N -dll PATH -watch=false`, otherwise it injects the
first A2 process (it injected the server DLL into a live client and crashed it). See [a2-50451-christmas-port](a2-50451-christmas-port.md).

**Turn-in + admin buttons (2026-11-02):** same root cause. Turn-in = Server_TurnInQuests -> 0x49B9060 -> evaluator + the
same group-gated push; locally it completed 9 parent lines. Admin kiosk buttons: ForceCompleteQuest -> Server_SetProgress
(widget+752, 255); ForceCompleteTask -> Server_OnCompleteQuest (accepted: "privileged, skipping validation"). Progress made
before the deploy was never persisted, so players must redo tasks once. Featured-transaction "Not running on the server" is
a stub in this build (0x56CCE80), unrelated. Kiosk "rewards table" miss = display-only lookup at kiosk+944.

**2026-11-02 big deploy (06:15, md5 ad051cb4):** + ball overlap pump re-enabled (`g_ballOverlapPump`, own flag;
UpdateOverlaps 0x3950F60 verified) + QuestOwnerValid spam fix + HEARTCORE fill (dropped Server_SetHeartballItemId ->
heartBallSpawner(pawn+0x1B20)+0x654 from entity EquippedItems @+0x550+0x88 if it's an AlternateDiscClasses key @0x4A0)
+ diagnostics [GSTATE] (GameStateManagerComponent currentState @0x4CC per slot), [PHYSDISC] (disc overlaps on sandbox
triggers + slot spawner Disc @0x580 / state), [GADGET] (functions run on grenade/launcher classes). Z Drift starts fully
locally (1->2->4->5) via BALLPUT into the cylinder; prod still unconfirmed. AVRPawn vtable RVA 0x92E6018.
Defender once quarantined a HalcyonA2.dll build (false positive) -- never add exclusions without asking.

**BOUNCE synthesis (deployed 2026-11-02 06:44, md5 a4913769):** thrown balls are client-simulated (server only gets
Server_SendPhysicsPropData positions), so UPropMovementComponent::OnBounce (MovementComponent @disc+0x658, OnBounce
@+0x518, parms FHitResult+FVector) never fired server-side -> heart cores "thrown at the ground" and impulse grenades did
nothing. BounceSynthTick (30 Hz): only balls with a client physics stream in the last 0.3 s (g_clientDrivenAt, set in the
SendPhysicsPropData branch); fast (>300) travel then stop/sharp turn -> SphereTraceSingle along old dir -> non-pawn hit
-> broadcast OnBounce bindings (param-size checked); skip if the game raised a bounce in 0.5 s. -NoBounceSynth disables.
Local: 0 idle false bounces; carried drop -> 1 bounce, BP handler ran. specedit BALLPUT marks carried balls client-driven
under -SpecEditLocalTest (prod runs with that flag too; only editor clients can send BALLPUT).

**ScrapRun gatling guns FIXED (deployed 2026-11-02 17:32, md5 61fdfeff, user confirmed "it works"):** LE_BP_AnchoredGatlingGun_C
: AA2GatlingBallGun. Trigger RPC AVRPawn::Server_SendTriggerStateDataWithStartLocAndDir arrived fine; magazine empty
(PooledAmmoActors num @0x300 = 0) because the gun BP's ammo setup never runs server-side. GunAmmoPass (2 s): spawn
BulletClass(@0x338) rounds, append to ActiveAmmoActors (@0x308, grow via 0x1112670) then ProcessEvent
PutAmmoActorBackInMagazine (only accepts rounds already in Active); recycle Active rounds after 3 s idle.
Also live: -NoBounceSynth in server.config.psd1 Args (turned off when user reported arena balls broken; bounce never
fired on arena balls -- 5 bounces all heart cores/grenades -- so probably not the cause; re-enable once arenas confirmed).
[GOLFHIT]/[GUNTRACE] probes live. Golf: only the on-ball StrokeCount (BP_GolfBallDisplay @0x868, replicated, set in
BP NewHitEvent on server) doesn't count -- awaiting [GOLFHIT] data.

**2026-11-02 evening (all user-confirmed unless noted):**
- POOLFIX: UGenericPoolingManager builder (0x58176D0) only builds bClientOnly pools on netmode 3; every shipped
  pool (grenades, snowballs, golf ball display, stickers...) is clientOnly=false -> NOP the two jz at 0x5817775 /
  0x581777D (bytes verified) and run the builder once. Fixed grenade buckets + golf ball hit counter.
- GRENADERETURN: nothing returns a bucket grenade to the pool -> bucket stays full. DLL returns it via the pool's
  PutObjectBackInPool ({obj, 0x1504060(obj, 0x5465B50())}) after throw+DelayBeforePooling or 30 s after grab. Works.
- Heart-ball pods (AA2ProgressionPodActor, box @0x3E0, OnHeartballTriggerBeginOverlap): DLL fires the overlap itself.
- Server Server_AddItemToInventoryById impl is an EMPTY STUB (vtable+2536 -> 0x10F9940): the server's ItemCatalog
  is always empty; the CLIENT's copy is authoritative. Never try to set balances server-side.
- Quest rewards: ProcessReward (0x5706BB0) never runs on our server (delegate bound by a server-side Mothership binder
  that never exists). Solara grants from saved player_quests (completedVersion>0) via a2_50451_quest_rewards.tsv.
- TRAP (2026-11-05): any hook at `HX_UNRES(0x...)` is DEAD on 50451 -- the macro is `0`, so MinHook hooks the image
  base, prints MH_OK, never fires, and its `_Orig` trampoline runs the PE header. QSendSet_Hook (Client_SetQuests
  sender) was one: the Spec Editor never saw a quest bundle and could not send its own. Fixed by observing
  Client_SetQuests in ProcessEvent and sending via the RPC (`[QSEND]`, SendQuestBundleRpc). Editor quests also need:
  a template (50451 has no "Subquest_PKR_Climber"; now Sub_FTUE_Tour_Mainframe) and Name/Location (+0x188/+0x198,
  new in 50451's FAAQuestEntry) set, or the RPC serialises dangling strings and silently never arrives.
  Verified on the local rig: button -> Server_OnQuestStarted -> 4x Server_OnCoinCollected -> Server_OnCompleteQuest.
- TRAP: production ALSO runs -SpecEditLocalTest -> g_seLocalTest is true live. Gate test-only code on IsLocalRig()
  (-ServerName=HalcyonLocalTest), never on g_seLocalTest.
- TRAP: interrupting an idat run corrupts the IDB (shard5/6 died) -> copy shard3 to a new shardN; current = shard7.
- Server config edit trap: removing an Args entry by regex left '' -> agent spinup "ArgumentList null" -> lost the
  watchdog restart; recover with POST :9095/spinup {"box":"win-gamebox-1"} (x-dashboard-key).

**PERF (2026-11-03): ObjIndexTick pacing bug = 500ms ping in full arenas.** Prod [PROF] dll=90% of wall, SafeObjIndexTick
830ms/s at ~17,000 calls/s (code assumed ~800/s, slice=num/700 per call -> walks/s=26). Fixed: slice paced by QPC elapsed
time (one 220k sweep/s). Local: ObjIndexTick 27ms/s, dll 9%. Built, NOT yet deployed at time of writing.
ScrapRun traps: Breaking bool @0x458 on BP_Scraprun_TrapDescriptor_C (push model, rep index 10); TRAPPROBE logs changes;
TRAPRESYNC (re-push) is OFF by default (-TrapResync), unproven. TRAP: interrupted MSBuild leaves cl.exe holding
dllmain_50451.cpp (Edit -> EPERM rename) -> kill cl/MSBuild first.

**TRAP (2026-11-03): manual `POST :9095/spinup {"box":...}` registers the server under a NEW station** (new station id,
empty config: no boards/roles/drafters/whitelist). Watchdog relaunches (kill the process) keep the station. Restart the
game server by killing it, never by bare /spinup; if a spinup is unavoidable, the duplicate deployments must be retired
(POST :9095/watchdog/retire {deployment_id}) or they relaunch. Station config can be recovered from
/opt/solara/snapshots/*_startup/production.solara.db with tools/snapdump (password = PRODUCTION_DB_PASSWORD in
/opt/solara/config/legacy-secrets.env; work on a copy). Board image files live in /opt/solara/boards (served at
https://station.halcy0n.net/board/<file>). Backend restarts drop the agent hub briefly: verify the game process after.
