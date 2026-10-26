# HalcyonA2 47031 — test runbook

The 47031 DLL is built with a **hook gate** (`g_gameplayHooks=false`): on inject it
installs ONLY the ProcessEvent hook (pass-through) and Terminate/Exit tracers,
and flips the game to server mode via `GIsClient=0 / GIsServer=1`. Every
unverified-offset hook is skipped, so no stale 22284 address is ever written to
or called. This is the safe first step.

Artifacts:
- DLL:  `Server/build/47031/x64/Release/HalcyonA2.dll`
- Injector: `bin/a2inject.exe`  (build: `go build -o bin/a2inject.exe ./cmd/a2inject`)
- DLL log: `%TEMP%\HalcyonA2.log` (also a console window the DLL opens)

## Phase 1 — safe inject + server-flip (no join yet)
1. Launch the game (`A2-Win64-Shipping.exe`) and let it reach the main menu.
2. Open a terminal **with the same elevation as the game** (admin terminal if the
   game is elevated — injection needs equal/greater rights).
3. Inject + watch the log:
   ```
   bin\a2inject.exe
   ```
   (auto-finds the process and the default DLL path, then tails the log. Override
   with `-dll <path>` / `-proc <name>` / `-pid <n>` if needed.)
4. **Expected:** a console window appears; the log prints the `[HalcyonA2]` boot
   banners, `ProcessEvent @ 0x…`, `EXITTRACE armed`, and the GIsServer/GIsClient
   flip. The game should NOT crash and should switch to listen-server behaviour
   (the build auto-listens once GIsServer/GIsClient are set).
5. Paste back the console / `%TEMP%\HalcyonA2.log` output. If it crashes, the
   `[CRASH]` / EXITTRACE lines pinpoint the offending call — that's the next
   offset to resolve.

What "good" looks like: loads, prints banners, stays alive, server flag set.
A crash here means a boot-path call hit a changed 47031 layout — we read the
crash site, resolve that one offset against the live process, rebuild, retry.

## Phase 2 — turn hooks on progressively
Once Phase 1 is stable, flip individual resolved hooks on. The 14 high-confidence
offsets (config getters, UpdateGameState, StartTimerWithCountdown, GolfBallInCup,
GolfOverlap, GetLocalInstance, GetPlayerPawn, JwtBuild) are correct 47031
addresses and safe to enable. Set `g_gameplayHooks=true` for a full-gameplay
attempt only after the remaining internals are resolved, or gate them one at a
time.

## Phase 3 — join against Solara
Run the Solara backend (`go build -o bin/solara.exe ./cmd/solara` then run it; see
docs/BUILD_AND_DEPLOY.md and docs/CONFIGURATION.md for ports :90 mothership / :78
station / :8080 dashboard), point the client's mothership host at it, and connect
a second client. (Endpoint/redirect wiring is the next tool once Phase 1 is green.)
