# a2-rollback-netmode-fix

> A2 50451 arena balls (rollback sim) on our client-build server — the full working fix set (verified 2026-10-31)
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

Arena balls (jakeball/driftball/soccer) WORK on 50451 as of 2026-10-31 (user confirmed: hitter + 2nd client both see it).
All in dllmain_50451.cpp (git-ignored), PatchRollbackNetmode1 + hooks:
1. AA2RollbackManager::BeginPlay 0x5726240: `cmp eax,3` @0x57262CB -> 1 (drive flag +0x329) — user's rule:
   patch netmode compares to 1, never spoof netmode 3.
2. BeginPlay `mov byte [rdi+328h],0` @0x5726272 -> 1 (authority flag; server check folded to 0 in client build).
   Gates seat push into RollbackConnectionComponent(+0x7C8)+0xD0 (sub_5734870) and result sending.
3. Step 0x57339E0 `cmp eax,2` @0x5733AA7 -> 1.
4. FRAME REBASE: clients stamp inputs on their own manager clock (+832, from join) vs server's (from start).
   Server_NewSubmitInputs pre-call: shift CommandFrame (stride 496) so newest = server+832 +2; offset per
   PawnNetInputComponent (g_rbFrameOff, re-anchor >300). Outbound: sender 0x54FC0F0 (Client_NewSendServerSimResults)
   and missed-inputs 0x545B1E0 get frame - offset. Without this player lastFrame=-1 and NOTHING is ever sent.
5. Ball push: in the 0x54FC0F0 hook, copy FFastRollbackBallState (0x50: pos/vel/ang) onto outline Discs actors
   (K2_SetActorLocation + PhysicsSync rep @+0xC8) once per sim/frame -> spectators see it.
Spectator assign 0x5723020 skips VR pawns (conn+0xA8 VRPawn set) by design — don't rely on it.
Step driver 0x560C2B0 on A2GameState (+0x348). Sim map mgr+0x348 (176-B elems, value @+8; +16 frame, +80 players).

Same class of bug elsewhere: client-build code gates server logic on GetNetMode (0x41FB690) == 0/2/3. Example:
PhysicalComponent::OverlapBegin player count (ScrapRun plates) -> [PLATENM1] `test eax,eax`->`dec eax` @0x55BF822.
When a "server-side" feature is dead, look for a 0x41FB690 call next to it first.

SERVER PAWNS NEVER MOVE (found 2026-11-01): player motion reaches the server only as
UA2PlayerEntity::Server_SetFrequentData (~20 Hz), which is just relayed to other clients, so the server's
AVRPawn actor sits at spawn and NO server overlap ever sees a real player (ScrapRun plates looked fixed
only because [SVTP] moved the server pawn directly -- never verify server triggers that way again).
[FDSYNC] in dllmain_50451.cpp copies FrequentData.Root (parms+0x08, FVector of 3 doubles) onto the pawn
whose AVRPawn::Entity (@0x1B80) == the entity, via K2_SetActorLocation(no sweep/teleport). Traps found:
UA2PlayerEntity::Pawn @0x480 can name a parked spawn pawn. pawn+0x1820 is an EMBEDDED outline struct:
seated = first DWORD (sim id) != 0xFFFFFFFF (reading it as a pointer made everyone look seated). FDSYNC
MUST skip seated pawns, or the replicated server actor overrides the arena sim and other players see a
seated player frozen while the ball moves (user report 2026-11-01). Verified end-to-end with the PC client (bridge tp ->
client reports -> server pawn follows -> RunnerPressureTrigger=1 -> "Scraprun Practice match started").

Related: [a2-50451-christmas-port](a2-50451-christmas-port.md)
