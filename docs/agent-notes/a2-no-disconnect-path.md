# a2-no-disconnect-path

> A2 clients cannot survive a server restart — no disconnect path; every new server needs a freshly launched client
>
> Type: feedback. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

A2 has **no disconnect path**: stopping/restarting the game server crashes every connected client
(AV reading 0x8 during world teardown). This is expected game behaviour, not a bug in our DLLs.

**Why:** the user corrected me (2026-10-31) after I chased two client "crashes" that were just the server restarts;
"u stopping the srvr crashes the client because this game has no disconnect path per new server needs new client".

**How to apply:** after every server restart, kill any old client and LAUNCH A NEW ONE (don't `op join` an old one,
don't debug the resulting crash dump). Order: server up + injected + listening :7777 -> then start the client.
See [a2-client-mcp-capture](a2-client-mcp-capture.md), [a2-50451-christmas-port](a2-50451-christmas-port.md).
