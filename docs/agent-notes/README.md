# Agent notes (Claude / AI agents working on Solara + A2)

Knowledge Claude collected while working on this repo with the project owner: reverse-engineering findings, IDA
addresses, protocol contracts, test rigs and traps that cost hours to discover. Read the notes relevant to your
task **before** starting. They are snapshots: verify any status line ("built, not deployed", "open") against the
code and git history before acting on it. "The user" in a note means the project owner.

Conventions used throughout:

- **Game builds:** A2 50451 (UE 5.5.3, the current live build), 47031 and 22284 (older). The game-server DLL is
  `Server/dllmain_50451.cpp` (+ `specedit.h`, `erebus_director.inc`); generated SDKs live in `Server/gamesdk/<build>`.
- **RVAs** are relative to image base `0x140000000` of the matching build's `A2-Win64-Shipping.exe`.
- **IDA:** headless `idat.exe -A -S"script.py" <shard>\A2-50451.exe.i64`; scripts and IDB shards live in
  `tools/a2port/work50451` (IDBs are not in git). Never interrupt an idat run (corrupts the IDB): work on a copy.
- **Nothing gets deployed to production** (backend restart, DLL push, dashboard rebuild) without the owner's approval.

## Status as of 2026-11-03 (owner-confirmed)

Fixed / done, even where an older note still says "open" or "not deployed": the Quest kiosk unlock, the rollback
arena balls, per-client PC (RIFT) identities, the Station_Prime level-tree desync, and VOIP (Vivox).

## Index

### Working rules (read first)
| Note | Why it matters |
|---|---|
| [compare-against-baseline](compare-against-baseline.md) | Before blaming lag/bugs on an external cause, measure the known-good control (Station Prime / prod) first |
| [cpp-escape-via-heredoc](cpp-escape-via-heredoc.md) | Patching C++ through a bash heredoc turns `\n` escapes into real newlines; use the Edit tool or a script file |
| [a2-no-disconnect-path](a2-no-disconnect-path.md) | A server restart kills every client; relaunch a fresh client after each restart |
| [event-build-quality](event-build-quality.md) | Event maps must be event-scale and textured; never show blocky previews |

### Project context
| Note | Contents |
|---|---|
| [a2-revival-context](a2-revival-context.md) | What A2 is, why RE/porting is in scope, toolchain (IDA, MSVC), porting history |
| [solara-migration](solara-migration.md) | Solara = Go port of the .NET Halcyon backend; oracles, open items |
| [solara-cloudflare-tunnel](solara-cloudflare-tunnel.md) | Hostnames -> ports under `*.halcy0n.net`, AWS boxes, UDP caveat |

### Game server DLL / reverse engineering (50451 unless noted)
| Note | Contents |
|---|---|
| [a2-50451-christmas-port](a2-50451-christmas-port.md) | How the 47031 DLL work was ported to 50451; building the 50451 IDB |
| [a2-rollback-netmode-fix](a2-rollback-netmode-fix.md) | Arena balls (rollback sim): the working fix set (netmode, authority flag, frame rebase, ball push) |
| [a2-50451-quest-kiosk](a2-50451-quest-kiosk.md) | Quest progression, the local test rig, `a2inject -pid` trap |
| [a2-50451-mothership-schemas](a2-50451-mothership-schemas.md) | Client Mothership response contracts (inventory, storefront, IAP, tech trees) |
| [a2-mothership-notifications-ws](a2-mothership-notifications-ws.md) | The Mothership websocket = moderation push (Warning/Mute/Unmute); `Client_ShowNotification` for custom boards |
| [a2-voip-vivox](a2-voip-vivox.md) | Voice chat setup per build |
| [ballsim-snapping-handoff](ballsim-snapping-handoff.md) | **Open:** 40-player driftball snapping: prod `[RBSTAT]` data (late inputs = current lead, `@0x1820` is not the sim id), fixes to try, deploy method |
| [a2-50451-rollback-sim-internals](a2-50451-rollback-sim-internals.md) | How the 50451 rollback sim steps, takes inputs and corrects clients (IDA RVAs) |
| [a2-47031-desync](a2-47031-desync.md) | 47031 Station_Prime crash investigation + rollback/seat RE notes (long) |
| [a2-47031-speceditor-port](a2-47031-speceditor-port.md) | In-game level editor port 22284 -> 47031 |

### Clients and testing
| Note | Contents |
|---|---|
| [a2-client-mcp-capture](a2-client-mcp-capture.md) | Drive the Windows client (free-cam, screenshots, `actors` queries) via the dsound.dll bridge |
| [a2-android-obb-patching](a2-android-obb-patching.md) | Quest client: pak/OBB rebuild, where the engine log is |

### EREBUS PROTOCOL (custom live event, paused)
| Note | Contents |
|---|---|
| [erebus-protocol-event](erebus-protocol-event.md) | Plot, UE 5.5.4 project, cook/patch pipeline, replication + relocation facts |
| [erebus-devlog](erebus-devlog.md) | Day-by-day dev log with test evidence and the open bugs at the pause |
| [a2-robot-animation](a2-robot-animation.md) | How to animate the robot (Cypher) so it looks right |
