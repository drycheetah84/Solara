# Driftball (TKB_*) goal scoring and match results

Status 2026-11-08: fix built (`[GOALGATE]`, `[MATCHRESULT]` reset tracking in `Server/dllmain_50451.cpp`), **not
deployed or tested in game yet**.

## Symptoms on prod
- The mercy rule ended games after 1 visible goal in one game and after 4 in the next.
- The ranked bot recorded wrong winners: `GET /api/stations/{id}/matches` showed a short game repeating the previous
  game's exact final score on the same slot (TKB_Driftplex01 seq 318 7-3 in 336 s, then seq 319 7-3 in 59 s; 320/321
  7-5 twice; also 99/100, 101/105, 248/317, TKB_Driftplex02 18/19, 20/21).

## Why
1. **Hidden Luau points.** `OnTeamScored` in the driftball gamemode (reference copy:
   `local/ref/.../SpecEditor/luau/game-scripts/TKB_Prime__gamemode.luau`) adds `goalPoints` to its Luau
   `team0points/team1points` *before* checking the state. The mercy rule (`MercyScoreDifference`) compares those
   counters. The visible scoreboard (ScoreComponent) only moves on `GameScore:incrementScore`, which runs in RUNNING /
   OVERTIME_RUNNING. The counters are reset in `SetupForPlay` (BEGIN_PLAY), *before* the WAITING_TO_START warm-up.
   Stock A2 keeps goals disabled outside play. Our `EnableGoals()` forces `bGoalEnabled@0x5A9` on for every goal,
   all the time, so warm-up goals and goals between points were counted in the hidden counters.
2. **One goal could score twice.** `DetectGoals` (geometric, 10 Hz) and the native overlap from the ball pump (~45 Hz
   since the fast-disc overlaps of 2026-11-05) both reach `GoalComponent::OverlapBegin`. `ScoreGoal` zeroes the
   goal's in-goal flag `+0x480` before calling it, so the native one-shot guard can't block the second one.
3. **Leftover score in the results.** `Gamemode:startGame()` makes `GetIsGameRunning` true a few seconds before
   `GameScore:resetScore()` (GAME_START_COUNTDOWN). The match-result pass kept the "highest score seen", which was
   the previous game's leftover, so a short game was reported with the last game's score.

## Fix
- `GoalGateCheck`: for a `TKB_*` slot whose GameStateManager state is known (recorded in `UpdGameState_Hook` and the
  GAME_BEGIN watchdog), a ball goal counts only in RUNNING(5) / OVERTIME_RUNNING(13), at most once per entry into that
  state. Practice (15-19) is untouched. Enforced in the ProcessEvent hook on `GoalComponent::OverlapBegin` (both paths
  go through it), in `DetectGoals` and in `ScoreGoal`'s raw-scorer fallback. A point is only marked as used when the
  native actually took it (`+0x480` went from not-257 to 257). Blocked goals log `[GOALGATE] ... ignored`.
- `MatchResultSlot`: a score only counts after the component has been seen to reset below its start value. If it never
  resets, the report is the change from the start value (a pure leftover comes out 0-0 = draw, winner -1).

## To verify after deploy
- Score in warm-up: expect `[GOALGATE] ... no point in play (state 1)`, and the first match goal shows 1-0 (no early mercy).
- `[MATCHRESULT] ... score reset 7-3 -> 0-0` at the start of each game, and the reported score matches the board.
- The ranked bot (`stationResults.js`) also ignores a result that repeats the slot's previous score, as a second guard.
