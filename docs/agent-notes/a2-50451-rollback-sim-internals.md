---
name: a2-50451-rollback-sim-internals
description: "How the 50451 AA2RollbackManager ball/player sim steps, takes inputs and corrects clients (IDA, 2026-11-05) + why 40-player driftball snapped"
metadata:
  node_type: memory
  type: project
  originSessionId: d98b8c37-5622-4986-8c22-9a025195900d
  modified: 2026-11-05T03:01:04.098Z
---

IDA facts (50451, RVAs from 0x140000000; scripts sim1/sim3.py in tools/a2port/work50451):
- Tick fn 0x55F18B0 -> step driver 0x560C2B0(A2GameState, TG 0) -> Step 0x57339E0(mgr, dt) ONCE per engine tick.
- Step = fixed 90 Hz accumulator: N = floor((dt*timescale@+0x344 + acc@+0x338 double)/0.011112), max 20/tick;
  acc > 0.22224 (20 frames) -> frames dropped (frame@+0x340 ++, "fell behind" log). Authority (+0x328) calls
  0x571A6E0 per sim each tick (dropped, dt ms, hitch = dt > 0.0122). BeginPlay 0x5726240 sets +0x328/+0x329.
- Server_NewSubmitInputs impl 0x57105D0: per input (stride 496) -> insert 0x56F7D30 (rejects |frame - mgr frame| > 100,
  sorted insert, duplicate replaces a placeholder) and records input LEAD = (inputFrame - mgrFrame - 1)*0.011112 into
  comp+272 history (the engine's client time-sync signal).
- 0x5757260 = server CORRECTION check, not an input gather: compares client-reported ball/player state for a frame vs
  the server's history; diff >= threshold (players 1 cm, balls comp+568/+576, ang 0.1) -> correction push
  (0x5747AE0 / vtable +1216) = the snap clients see. Frames older than sim frame - 20 ignored.

Prod evidence (Freerunners 1c8b34fe, ~40 players, 2026-11-05): game loop never < ~520 fps (not CPU bound);
NewInputBuffer median 51 / max 64 (mostly history, not backlog); ~720/s Iris "Failed to resolve clientassigned ref"
warnings for client-side balls (RC/soccer/snowball/golf); UE log at C:\Windows\System32\config\systemprofile\
AppData\Local\A2\Saved\Logs\A2_N.log (one per server).

**Why:** 22284 fixed ball lag by pacing the sim by the slowest player's confirmed input (PumpBallSimStep, -SimDelay=2,
ArmResultsOnStep); 50451 steps on the clock and our FRAME REBASE (inputs relabelled to server frame + 2 on arrival)
hides real lead from the engine's time-sync, so late inputs simulate on different frames than clients predicted ->
corrections -> snapping, worse with more players.
**How to apply:** measure corrections/s (count 0x5747AE0 calls) before/after any change; test on the local rig with
added latency, never on prod. Related: [[a2-rollback-netmode-fix]].
