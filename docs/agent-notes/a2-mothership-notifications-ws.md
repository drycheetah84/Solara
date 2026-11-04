# a2-mothership-notifications-ws

> 50451 Mothership websocket = live moderation push (Warning/Mute/Unmute boards + voice mute); frame JSON {Title,Body,RecipientId}; IDA RVAs; Solara plan
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

RE'd 2026-11-03 (IDA scripts ws5..ws9.py in tools/a2port/work50451, run on scratch copy shard9; shard8 untouched).
NOT the cause of lag (user corrected me: prod + Station Prime have the same dead-AWS errors with low ping) - see [compare-against-baseline](compare-against-baseline.md).

**What it is:** OnlineSubsystemMothership "notifications" websocket = moderation push channel. Config
`[OnlineSubsystemMothership] WebSocketUrl` (DefaultEngine.ini in A2-Windows.pak; shipped value = dead AWS
wss://pw7x9mgtwg.execute-api.us-west-2.amazonaws.com/prod-A2-ws-stage -> 401). Connects after Mothership login
(4DA4940 -> connect 4DB72F0, using a string from the login result - token vs player id NOT yet confirmed; capture
the upgrade request on Solara to settle URL/query/headers). Reconnect backoff 1,2,4,8,8.. s (4DC8E30).
libwebsockets (not curl) -> midnight's curl_easy_setopt redirect does NOT cover it.

**Frame:** text JSON, all three keys required strings (deserializer 4DC9CE0, msg vtable 8F21B98):
`{"Title":"Warning|Mute|Unmute","Body":"<category>|<reason1,reason2>|<seconds>","RecipientId":"<id>"}`
Parsed by 4DE4E20: Body split on '|' (const 8DB2BB0) -> category, reasons (split ','), duration string.
Event dispatcher 4DE7290 (0 connect, 1 message, 2 close, 3 error).

**Client behaviour (UA2NotificationComponent, 563C9D0/5633180/5633420, binds in 5621C20):**
- Warning (needs >=1 reason): board "<CATEGORY> WARNING FOR <reasons joined ' AND '>\nNEXT COMES MUTE"
- Mute: board "MUTED FOR <sec/60> MINUTES FOR\n<reasons>" (no/empty duration -> "MUTED FOREVER FOR"), + voice mute (sub_1453DDA50(x,1))
- Unmute: voice unmute (sub_1453DDA50(x,0)), no board
- log "(<RecipientId>) <Title>: <reasons>"; console a2.SendNotification / a2.SendPremadeNotification (test data "voice|some_offense", "fake-recipient-id")

**Solara:** has bans/reports/mutes collections (internal/mothership/moderation.go) + gorilla/websocket (EOS gateway) but
no push. Plan: ws endpoint on :90, map socket->player at upgrade, push frames from dashboard warn/mute/unmute; clients
need WebSocketUrl pointed at Solara (pak config patch, or hook in midnight/version.dll).
