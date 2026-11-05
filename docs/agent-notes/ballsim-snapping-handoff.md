# Ball sim snapping (50451): handoff

> Status 2026-11-05 (late): **prod is on DLL `8E4D13BB` (commit 2e62694) on all 5 box 1 servers**, with `[RBSTAT]`,
> [RBDEDUPE] and the relay hotfix. Read **"Prod data after the hotfix"** right below first: it rules out two of the
> theories further down and points at **late inputs**. Written for whoever picks this up next (human or Claude). Read
> [a2-50451-rollback-sim-internals](a2-50451-rollback-sim-internals.md) and
> [a2-rollback-netmode-fix](a2-rollback-netmode-fix.md) first. Never test on the live servers; use the local rig.

## Deployed 2026-11-05 evening (DLL 6e0178e6): all three follow-ups below are live

- `[RBSTAT]` gained `plcorr/s` (player corrections: the correction check's call to `RollbackPlayer` vtbl slot +1216,
  RVA 0x5747980, hooked lazily from the first seated player), `leadUp` / `leadDown`.
- `[RBLEAD]` adaptive lead (search `[RBLEAD 2026-11-05]`): per player per second, >=2 late inputs (or one 2+ frames
  late) -> +1 frame (+2 if 3+ late), max +6; 20 s of min lead >= 4 -> -1. `-NoRbAdaptLead` off. Local rig: forcing +3
  moved lead ~4.7 -> ~8 at once and it held across re-seats (the client does not fight it).
- `[RBSIMMAP]` relay same-sim check now uses the sim index the server passes to the results sender
  (conn -> pawn @0xA8 -> nic @0x1690), falling back to 80 m when unknown; `[RBFWDID]` prints both ids.
  Not yet seen with two matches running at once.
What to read on the next busy night: do `plcorr/s` spikes line up with `late`, and does `leadUp` bring `late` down?

## Prod data after the hotfix (2026-11-05, box 1, read-only)

Source: `C:\Windows\Temp\HalcyonA2.log` on box 1 (shared by all 5 servers, so `[RBSTAT]` lines from idle servers
show `fwd/s=0`; look at lines with `batches/s` > 90). Sample: one 2–3 player driftball match, **not** the 40-player
crowd, so it shows the mechanism, not the worst case.

| Signal | Value | Meaning |
|---|---|---|
| `corr/s` | **0** everywhere | No **ball** corrections (`0x5747AE0`). Player corrections (vtable `+1216`) are **not counted yet** |
| `dup` | **0** | [RBDEDUPE] skips nothing on prod (it ticks ~90/s). Harmless, kept |
| `ticks/s` | 87–91, `dtMax` 11–24 ms | Server tick is fine. Not the cause |
| `fwd/s` / `fwdBlocked/s` | ~175 / **0** in the active match | Relay works again after the hotfix |
| `lead` | normally min 2, avg 3–4 | Healthy most of the time… |
| `lead` / `late` in bursts | avg **0.3–1.0**, min **−11**, `late` **up to 23–27/s** | …but inputs regularly arrive after the server already simulated that frame |

### Finding 1: `@0x1820` is not the sim id (relay same-sim check is effectively off)

All 80 `[RBFWDID]` lines show `sim@0x1820=0xFFFFFFFF` for **both** players while they were seated together in one
match (seat idx 2/6, 3, 5/7). So `AVRPawn + 0x1820` (first dword of `CurrentRollbackSimulationOutline`) does not hold
the seated sim's index on the server, or it's the wrong offset. Commit a8e11d3 relied on it and blocked every relay
(teammates froze); hotfix 2e62694 only blocks when both ids are valid and differ, otherwise falls back to the old 80 m
rule. **Net effect: prod relays exactly like before a8e11d3**, so the cross-match relay theory below is still
**untested**. To-do: find the real "which sim is this player in" field, e.g. the sim the
`Server_NewSubmitInputs` impl (`0x57105D0`) inserts into, or the outline's player list, and key the relay on that.

### Finding 2 (current lead): late inputs from the fixed 2-frame rebase

The frame rebase (`want = serverF + 2 - clientLast`, see "What our DLL does on top") gives every player a **fixed**
2-frame (~22 ms) buffer at seat time and never adapts it (only re-fixes on >300 frames of drift). Quest Wi-Fi
jitter or slow clock drift eats that, and prod shows it: bursts where a quarter of a player's inputs are late. A late
input means the server simulated that frame with a guessed input; when the real one disagrees, the player (and the
ball, if touched) snaps. A busier server/network makes the bursts worse, which matches "only bad at 40 players".

Proposed fixes, smallest first (none built yet; the owner has not approved a prod deploy of any of them):

1. **Bigger fixed lead:** `serverF + 2` → `serverF + 4` (+22 ms input delay). One-line change, easy A/B on prod via
   `late` and `lead min` in `[RBSTAT]`.
2. **Adaptive lead:** per player, track `lead min` over a window; if inputs keep arriving late, bump that player's
   offset by 1–2 frames (and slowly back down when lead stays high). Must also re-adjust the outbound `frame - offset`
   for results and relayed inputs consistently (`g_rbFrameOff`).
3. **22284-style pacing:** cap the accumulator (`+0x338`) so the sim never passes the slowest seated player's newest
   input minus a small delay (see "How 22284 fixed ball lag").

Also add a **player-correction counter** (hook the vtable `+1216` call in `0x5757260`) so `[RBSTAT]` shows whether
late bursts line up with player corrections. That would prove Finding 2.

### Deploying to box 1 (what actually works)

Copying a new DLL over `C:\Halcyon\HalcyonA2.dll` while servers run fails silently (file locked), and the agent caches
the DllPath it read at connect time. Working method: **rename** the loaded `HalcyonA2.dll` (allowed while mapped),
copy the new one to the same path, then restart servers **one at a time**, waiting for `injected pid N` in
`C:\Halcyon\logs\agent.log` (~65 s) before the next (killing a server kills any other still initializing). Verify the
loaded module hash per process. Run the script on the box with `powershell -File` (stdin drops multi-line loops).
Box 2's Halcyon server is still on the older `2DB11F71` build (old 80 m relay, no `[RBFWDID]`).

## The problem

With ~40 players **on the server** (Freerunners, deployment `1c8b34fe`, box 1) and a few of them playing driftball,
**players in the sim and the ball snapped constantly.** With a handful of players on the server (Nov 3) it looked
acceptable. It gets worse with the server's population, not with how many are seated in the sim.

## Fix: [RBDEDUPE] duplicate sim-results sends (2026-11-05, alongside the cross-match relay fix)

Measured on the local rig: the sim-results sender (`0x54FC0F0`, `Client_NewSendServerSimResults`) runs up to once per
**engine tick**, not once per 90 Hz sim frame. With one seated PC client the server sent 233 results/s, of which ~144
were byte-identical to the previous send (same frame, same ball and player states), because the rig ticks ~233/s.
**Prod does not tick that fast:** its step driver runs ~86–89 ticks/s (`[RBSTAT] ticks/s=88`; the earlier "~550 fps"
counted `[PROF]` hook calls, not frames), so on prod results already go out about once per sim frame and the dedupe
probably skips little there. It is cheap and safe to ship, but **probably not the main cause on prod**; `dup` in
`[RBSTAT]` on a busy prod server settles it within minutes (near 0 rules the repeat theory out). Both that RPC and
`Client_NewSendOtherPlayerInputs` (the
`RBFWD` relay) are **unreliable**; a busy server's pose traffic fills every connection, and UE drops unreliable RPCs on a
full connection, so the duplicates crowd out exactly what seated players need (leading explanation for the
population scaling; the drops themselves are not measured).

The DLL now skips a send that is byte-identical to the last one to that connection for that sim, so ~90/s go out (one
per sim frame), like a ~90 Hz dedicated server. A re-simulated frame (different bytes) still goes out. Locally,
dedupe on vs off: 0 corrections, 0 missed-input reports, inputs steady at 180 batches/s, no rollback errors in the
client log. `-NoRbDedupe` turns it off; on the local rig `%TEMP%\rbtest_nodedupe.txt` turns it off live (A/B).
`[RBSTAT]` gained `sends/s` and `dup` (repeats found; skipped while the dedupe is on).

Still to confirm: a busy prod session with this build (corr/s should drop; `fwdBlocked/s` > 0 shows the cross-match
relay was happening). If snapping remains, the next suspect is the per-connection pose budget (`[NETSCALE]` assumes
45 poses/s, but a 50451 PC client sends ~184/s, see `[POSERATE]`).

## Seating a PC client on the local rig (works, 2026-11-05)

Local server WITHOUT `-NoSpecBlock` (so the PC client gets a real pawn), SpecEditor client, then over the bridge:
`join 127.0.0.1:7777`; `slots` lists arena slot ids (driftball: `TKB_Driftplex01`..`06`, `TKB_Prime`, ...);
`tp 16736 6900 -25766` + `arena TKB_Driftplex01` (inside Driftplex01); `tp 14644 6442 -26800` (its red team switcher;
blue is at y 7358) -> server logs `ADMITDIAG ... ADMITTED`; after ~8 s `tp 16200 6700 -25900` onto the field.
`[RBFRAMEFIX]` then shows the player's offset and `[RBSTAT]` prints every second.
Trap: once seated, the sim owns the player's position, so a teleport can be undone and drop the player back on its
own switcher, which clears the team (`Client_TeamClearSuccess`, `LeftArena`). Repeat the sequence if that happens.
A second client never stayed seated this way (no movement input over the bridge), so two-player tests need a person
on a headset or keyboard.

## How the 50451 sim works (IDA, RVAs relative to 0x140000000)

| Piece | RVA | What it does |
|---|---|---|
| Tick function | `0x55F18B0` | Calls the step driver with the GameState and tick group |
| Step driver | `0x560C2B0` | On A2GameState (`+0x348` flag); calls Step **once per engine tick** with dt |
| Step | `0x57339E0` | Fixed **90 Hz** accumulator: `N = floor((dt * timescale@+0x344 + acc@+0x338) / 0.011112)`, max 20 per tick. Above 0.22224 s of backlog it drops frames. Frame counter at `+0x340` (= +832) |
| BeginPlay | `0x5726240` | Sets authority `+0x328` and drive `+0x329` (both patched to 1 by our DLL, see netmode fix note) |
| Authority report | `0x571A6E0` | Per sim per tick: dropped frames, dt in ms, "hitch" (dt > 0.0122 s) |
| Server_NewSubmitInputs impl | `0x57105D0` | Per input (stride 496): insert via `0x56F7D30`; records **input lead** `(inputFrame - serverFrame - 1) * 0.011112` into comp `+272` (the engine's time-sync signal) |
| Input insert | `0x56F7D30` | Rejects inputs more than 100 frames from the server frame; sorted insert; a real input replaces a placeholder for the same frame |
| **Correction check** | `0x5757260` | For a frame, compares what each client reported (balls: pos/vel/ang; players) with the server's history. Difference above threshold (**players 1 cm**, balls comp `+568` / `+576`, ang 0.1) → **pushes a correction** (`0x5747AE0` for balls, vtable `+1216` for players). **Every client-visible snap is one of these.** Frames older than sim frame − 20 are ignored |
| Missed-inputs report | `0x545B1E0` | Server → client "you missed N inputs" (client adjusts its lead) |
| Sim results send | `0x54FC0F0` | Client_NewSendServerSimResults (our hook also pushes ball state to spectator actors) |

IDA scripts: `tools/a2port/work50451/sim1.py` (driver/Step) and `sim3.py` (inputs, corrections). Run headless on a
**copy** of the IDB (shard9; shard8 is the clean source). Never kill a running idat (it corrupts the IDB). Iterate
functions with `idautils.Functions()`: a hand-rolled `get_func`/`end_ea` walk loops forever on function tail chunks.

## What our DLL does on top (Server/dllmain_50451.cpp)

- **Frame rebase** (`[RBFRAMEFIX]`, search `want = serverF + 2 - clientLast`): clients stamp inputs on their own clock,
  so on the first input from each player we fix an offset that makes their newest input = **server frame + 2**. The
  offset only changes if it drifts more than 300 frames. Outbound results / missed reports get `frame - offset`.
- **Input relay** (`[RBFWD]`, `SafeRbForwardInputs`): forwards each player's rebased inputs to the other seated players.
- **Ball push** (`[RBBALLPUSH]`, `RbBallPush`): copies sim ball state onto the outline Discs actors for spectators.
- The old 22284 pumps (`PumpBallSimStep` etc.) are **off** on 50451 (`g_ballPumpsEnabled=false`): they call a stepper
  that does not exist in this build.

## Evidence from the 40-player session (prod logs, read-only)

- **Not CPU bound.** The game loop never dropped below ~520 fps (per-frame hooks in `[PROF]`), even at 40 players.
  Correction: those were `[PROF]` hook-call counts, not frames. `[RBSTAT]` shows the step driver (the engine tick
  that runs the sim and sends results) at ~86–89 ticks/s on prod.
- **Input buffer** (`[RBINPUT] NewInputBuffer.Num`): median 51, p90 55, max 64. Probably mostly history (nothing prunes
  it to near zero), **not proven** to be a backlog.
- **Iris spam:** ~720/s `LogIrisReferences ... Failed to resolve clientassigned ref ... BP_RemoteControlBall /
  BP_SoccerBall / BP_ChillshotSnowball / BP_DribbleGolfBall` (2.0 M lines in ~2h20m, 535 MB log). Scales with players.
  Costs game-thread time and likely means client ball updates are being dropped.
- 1,343 `LogIrisRpc ... Rejected RPC ... missing object` and 1,673 `ReplicationDisabled from client` errors.
- UE logs per server: `C:\Windows\System32\config\systemprofile\AppData\Local\A2\Saved\Logs\A2_N.log` on the game box;
  the DLL log is `C:\Windows\Temp\HalcyonA2.log` (shared by all servers on the box).

## Earlier lead: cross-match input relay (commit a8e11d3; its sim-id field turned out wrong, see Finding 1)

The 40-player night had **several full 4v4 matches running at once**. Our `RBFWD` relay sent each seated player's
inputs to every seated player **within 80 m**, not "in the same sim". Neighbouring matches got each other's inputs,
and the receiving client resolves the sender by `PlayerIndex` (the seat number, 0..7 in every match), so a foreign
seat-N input was applied to its own seat-N player: players and the ball snapping, **only when several matches run**
(invisible with one match, which is why Nov 3 looked fine). Fix: relay only when both pawns'
`CurrentRollbackSimulationOutline` (`@0x1820`, first dword = SimIndex) match. `[RBSTAT]` gained `fwd/s` and
`fwdBlocked/s` (relays the old rule would have sent across sims) to confirm on a busy night.

Early `[RBSTAT]` data (one player in a sim, Nov 5): sim tick steady 90/s, lead ~6–8 frames, 23 late inputs in
159 s, **0 ball corrections**, 0 missed reports. So timing/lateness alone did not produce snaps.

## Other hypotheses

1. **Late inputs → corrections.** 50451 steps on the clock, not on inputs. Our rebase starts every player only
   **2 frames (22 ms)** ahead, so any jitter above that makes an input late; the server simulates the frame without it,
   the client's prediction differs, and `0x5757260` pushes a correction to everyone in the sim. More players = more
   late inputs per second. (The client's own lead adaptation through missed-input reports should still work through the
   fixed offset, so this is likely but **unproven**.)
2. **Corrections not caused by lateness**: our input relay (`RBFWD`) or ball push producing state the server's
   correction check disagrees with.
3. **Dropped client ball updates** (the Iris resolve failures) adding mismatches.

## How 22284 fixed ball lag (for reference)

`dllmain.cpp` (22284), `PumpBallSimStep`: the server sim was **paced by inputs**: each tick it stepped toward
`slowest active player's newest input − SimDelay (2)`, caught up several frames when behind, held when ahead, and only
sent results for confirmed frames (`ArmResultsOnStep`; pinning results every frame caused the old snap-back stutter).
That never simulated a frame before every player's input for it had arrived. The equivalent for 50451 would be to cap
the accumulator (`+0x338`) so the sim frame never passes the slowest seated player's newest input minus a small delay.

## Instrumentation (deployed to prod box 1 since 2026-11-05; fields added since: `sends/s`, `dup`, `fwd/s`, `fwdBlocked/s`)

`[RBSTAT]` once per second from the step driver hook (diagnostic only, no behaviour change):

```
[RBSTAT] corr/s=… missedRep/s=… missedSum=… batches/s=… lead min=… avg=… max=… late=… | ticks/s=… hitch=… dtMax=…ms
```

- `corr/s`: corrections pushed (hook on `0x5747AE0`), i.e. snaps.
- `missedRep/s`, `missedSum`: missed-input reports sent and inputs they reported.
- `lead`: newest input frame minus server frame when a batch arrives (after the rebase); `late` = batches with lead ≤ 0.
- `ticks/s`, `hitch`, `dtMax`: server frame rate and frame-time spikes as the sim sees them.

Verified locally that the hooks install (`create=0 enable=0`); no numbers yet (no player was seated).

Also in the working tree: `CurlRewrite` sends the game's own Mothership / station traffic to
`https://<MothershipHost|BackendHost>` when the port is 443, so a server outside the Azure network can run against the
prod backend through the tunnel (`-BackendHost=station.halcy0n.net -BackendPort=443 -MothershipHost=mothership-prod.halcy0n.net -MothershipPort=443`).

## How to test (local, no headset needed)

Scripts (copies in `tools/ballsim/`, originals in `C:\Users\drych\HalcyonEvent\`):

- `ballsim_rig.sh N`: local Solara backend must be running first
  (`cd C:\Users\drych\solara-local && MOTHERSHIP_INSECURE=1 DASHBOARD_BIND_ADDRESS=127.0.0.2 ./bin/solara.exe serve`).
  Starts the hardlinked server copy (never the original install) **without `-NoSpecBlock`** so PC clients get a real
  pawn, injects the freshly built DLL, launches N windowed PC clients, prints `[RBSTAT]`.
- `ballsim_prod_test.sh`: same server but registered on the **prod** backend with `-RegisterIp=<LAN IP>`, so a headset
  on the home Wi-Fi can join it. Delete its deployment/station from prod afterwards.

To get a PC client into a sim, use the client capture bridge (`dsound.dll`, `HX_MCP_PORT=47650/47651`,
`py -3 -I C:\Users\drych\HalcyonEvent\mcpq.py op "<op>"`): `actors <name>` to find the driftball arena and its slot,
`tp x y z` to walk in, `arena <slotID>` (`Server_NotifyPlayerEnteredArena`) to be seated, `ballpos` to watch balls.
**Not done yet:** finding the driftball arena's slot ID/location in Station Prime and confirming a PC pawn gets seated
(check the server log for the player in `[RBFRAME]` with `lastFrame` = sim frame). To add jitter, use a tool like
clumsy on one client's UDP 7777 traffic and compare `corr/s` / `late`.

## Suggested next steps

1. Get two PC clients seated in one driftball sim; record `[RBSTAT]` baseline.
2. Add 30–80 ms jitter to one client. If `late` and `corr/s` jump together → hypothesis 1. Try a larger rebase lead
   (`serverF + 2` → `+6`) and/or 22284-style pacing (cap `+0x338` by the slowest seated player's newest input − 2).
3. If `corr/s` is high with no late inputs → look at `RBFWD` relay and `RbBallPush` (hypothesis 2).
4. Optional, separate: look into the Iris client-assigned ball reference failures. (Capping the server frame rate is
   moot: prod already ticks ~88/s, see above.)
5. Only after a local fix holds: deploy the `[RBSTAT]` build to prod for one busy session to confirm with real players.
