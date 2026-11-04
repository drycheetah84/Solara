# compare-against-baseline

> Before blaming a perf/lag problem on an external cause, compare against the known-good baseline (stock map / prod)
>
> Type: feedback. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

When something regresses (e.g. client RTT in the thousands on the EREBUS map), first compare the same measurement on
the known-good baseline (Station Prime, prod servers) before naming a cause. I blamed the dead AWS Mothership
websocket; the user pointed out ping is low on Station Prime and prod, which have the same websocket errors.

**Why:** user lost trust ("u tried to blame this on the socket") and stopped the investigation.
**How to apply:** for any lag/perf/bug hunt, find what differs between the bad case and a good control first;
only then propose a cause, and say it's a hypothesis until the control confirms it. See [erebus-protocol-event](erebus-protocol-event.md).
