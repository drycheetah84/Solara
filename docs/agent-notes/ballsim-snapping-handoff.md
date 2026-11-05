# Ball sim snapping (50451): handoff

> Status 2026-11-05: **diagnosed, not fixed.** Written for whoever picks this up next (human or Claude). Read
> [a2-50451-rollback-sim-internals](a2-50451-rollback-sim-internals.md) and
> [a2-rollback-netmode-fix](a2-rollback-netmode-fix.md) first. Never test on the live servers; use the local rig.

## The problem

With ~40 players on one server (Freerunners, deployment `1c8b34fe`, box 1) playing driftball, **players in the sim and
the ball snapped constantly.** With a handful of players (Nov 3) it looked acceptable. It gets worse with player count.

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
  Servers run **uncapped** (~550+ fps); fine for the accumulator math, but frame-time spread feeds the sim.
- **Input buffer** (`[RBINPUT] NewInputBuffer.Num`): median 51, p90 55, max 64. Probably mostly history (nothing prunes
  it to near zero), **not proven** to be a backlog.
- **Iris spam:** ~720/s `LogIrisReferences ... Failed to resolve clientassigned ref ... BP_RemoteControlBall /
  BP_SoccerBall / BP_ChillshotSnowball / BP_DribbleGolfBall` (2.0 M lines in ~2h20m, 535 MB log). Scales with players.
  Costs game-thread time and likely means client ball updates are being dropped.
- 1,343 `LogIrisRpc ... Rejected RPC ... missing object` and 1,673 `ReplicationDisabled from client` errors.
- UE logs per server: `C:\Windows\System32\config\systemprofile\AppData\Local\A2\Saved\Logs\A2_N.log` on the game box;
  the DLL log is `C:\Windows\Temp\HalcyonA2.log` (shared by all servers on the box).

## Leading hypotheses

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

## Instrumentation already added (in the working tree, built, NOT deployed)

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
4. Optional, separate: cap the server frame rate (it runs ~550 fps uncapped) and look into the Iris client-assigned ball
   reference failures.
5. Only after a local fix holds: deploy the `[RBSTAT]` build to prod for one busy session to confirm with real players.
