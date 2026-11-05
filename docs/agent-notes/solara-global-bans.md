# solara-global-bans

> 2026-11-05. How bans work in A2 50451 and Solara's global ban (`internal/mothership/globalban.go`).

## What the game has

- **Fleet bans:** moderation panel `Server_BanPlayerFromFleet` ("You have been banned from this fleet for ...") ->
  Mothership `POST /v1/moderation/server/ban`. Enforcement at station login is compiled out of this build; the DLL
  (`ModAdmin::OnLogin`) checks `GET /v1/moderation/server/active_ban` and kicks. Game wording for that:
  "You are banned from this fleet. Contact this community's admins for more information."
- **Meta device bans:** Quest attestation at Mothership login (`CompleteQuestAuth`: `device_ban.is_banned`, tamper ->
  `banDevice`) -> 401, no session.
- The Mothership login client reads NO ban field. A refused login is just a failed login, but its error text IS
  parsed and logged/shown. **Verified shape (prod, 2026-11-05):**
  `{"statusCode":403,"message":"<JSON STRING: {\"ClientMessage\":\"...\",\"TraceId\":\"...\",\"MothershipErrorCode\":<int>}>"}`.
  The parser's three checks, in order: "no message or statusCode found" (message as a nested object fails here),
  "message is not a valid JSON object", "no ClientMessage, TraceId or MothershipErrorCode found" (a STRING error code
  fails here; the client's fallback code is 99999). Result on the client: "Authentication Failed: <ClientMessage>.
  Errror Code: 40301. Trace ID: <ban id>", and the player cannot log in.

## Solara global ban

Any active `MothershipBanDbObject` (panel, game server, dashboard) is global:
- every token-minting client login (Quest complete, Rift, Steam) refuses with 403 in the shape above (error code
  40301), ClientMessage =
  "You are banned from Halcyon for N more minutes/hours/days | permanently. Reason: ...";
- `getUserIdFromToken` rejects a banned player's existing tokens, so a live session ends at once.
- Our own game servers (`isTrustedServer`) are exempt. No game-server polling (owner's call).
- Dashboard Users page "In-game bans": Ban (player id = Mothership id or Meta id; resolved to every linked player
  AND the id as typed), 1 h .. permanent (stored as 100 years so unban, ExpiresAt = now, still works), reason; Unban.
  API `POST /api/game_bans`, `POST /api/game_bans/{player_id}/unban`, scoped perm `users.ban`.
- Trap: the local rig's PC client logs in to the PROD Mothership, so a ban in solara-local never applies to it.
