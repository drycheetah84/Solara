# a2-50451-voice-moderation

> How voice moderation works in A2 50451 (client build we run as the server), what actually works on our servers,
> and what is compiled out. RE'd 2026-11-05 with IDA (`tools/a2port/work50451/mod1.py`, `mod2.py`, run on the shard9
> copy) plus the Dumper-7 SDK. Related: [a2-mothership-notifications-ws](a2-mothership-notifications-ws.md),
> [a2-voip-vivox](a2-voip-vivox.md).

## Short version

There is **no automated voice moderation** (no ToxMod/Modulate/GGWP, no speech-to-text use; the only "transcription"
string is the Vivox SDK's `ChannelSession::BeginSetChannelTranscription`). Voice moderation is all manual:

1. **Personal mute / ghost** (Settings & Safety kiosk): a player mutes someone **for themselves**. Client-side.
2. **Moderator actions** (in-game ImGui moderation panel + `UA2PlayerModerationComponent` RPCs): Mute for all,
   Mute all, Kick, Ban from fleet, Report, Warn. **The server halves are compiled out of this build** except Warn.
3. **Mothership mute** (backend push over the notifications websocket): `Mute` / `Unmute` / `Warning` frames.
   A muted player's **own client mutes its own microphone** and shows the board. Needs a websocket endpoint
   Solara does not have yet.

Voice audio goes client -> Vivox, never through the game server, so the server cannot drop a muted player's
audio itself. Every mute is enforced by a client: the muted player's (mic off) or each listener's (local mute).

## The voice-mute primitive

`sub_1453DDA50(voipUser, bool mute)` mutes **this client's microphone**: `*(a1+624)` -> vtbl+40 (voice-chat user) ->
vtbl+88(mute). Callers:
- Notifications Mute (`5633180`, mute=1) / Unmute (`563C9D0`, mute=0).
- Mic-setting logic `5845E90` / `5846000` (ApplyMicSettings): `AVRPawn::SettingsAndSafetyData.MicSetting`
  (pawn `+0x1F88`): 1/2 = mic muted (off / push-to-talk idle), 0/3 = open. A pawn byte at `+0x1A64` forces mute=1
  regardless of MicSetting (likely the server/Mothership-muted HUD state; `Client_SetMutedHud` is controller vtbl+4032).
- Push-to-talk input `5688070`.

## Personal mute / ghost (works: client-side)

`AVRPawn::ClientVersionOfServerOnlySettingsAndSafetyData` @`0x1ED0` (replicated, RepNotify) is a
`TArray<FA2SettingsAndSafetyPlayerEntry>` (16 B: flag byte + `AVRPawn*`). Flag bits: 0 `bGhosted`, 1 `bMuted`,
2 `bMutedForAll`, 3 `bMuteDirty`, 4 `bMothershipMuted`. The OnRep (`58615C0`) merges server flags into the local
list, then applies each entry to that player's voice component (`+244` muted). Server copy:
`FA2ServerOnlySettingsAndSafetyPlayerEntry` (MothershipId + same bits) via
`Server_Read/WriteServerOnlySettingsAndSafetyPersistentData` (pawn vtbl+2272/+2264, not traced) and
`UpdateServerOnlySettingsAndSafetyFromClient` (`588CA70`, real: copies a client's entry into the list).
`bMuteAllEnabled` @`0x1FAC` / `SetMuteAllFromClient` = "mute everyone for me".

## Moderator actions (compiled out except Warn)

`UA2ModerationSubsystem` (GameInstanceSubsystem): `LastKickMessage` @0x30, `ServerOnlyMuteForAllMothershipIDs`
@0x40, `ServerOnlyMothershipMutedMothershipIDs` @0x50. Static functions and their bodies in 50451:

| Function | Impl | Body in this build |
|---|---|---|
| `MutePlayerForAll` | `56C3020` | **stub**: if caller pawn's role (+376) != 3 log "not authority"; else nothing |
| `MuteAllPlayersForAll` | `56C27E0` | **stub** (same) |
| `KickPlayerServer` | `56BEF50` | **stub** (also called from `5895570`/`5896AF0`, dashboard kick paths) |
| `ReportPlayer` | `56C9BC0` | **stub** |
| `BanPlayerFromFleet` | `56AC410` | **stub** |
| `WarnPlayer` | `56D92C0` | **real**: permission check (`5895010`, perm 39) then shows the warning on the target's notification component (`+3712`) |

The component RPCs (`Server_MutePlayerForAll` vtbl+1232, `Server_MuteAllPlayersForAll` +1224, `Server_KickPlayer`
+1256, `Server_WarnPlayer` +1248, `Server_ReportPlayer`, `Server_BanPlayerFromFleet`) dispatch into those. So on our
servers the in-game panel's Mute for all / Kick / Ban / Report buttons **do nothing** (no list update, no
`bMutedForAll` replicated, no Mothership call). Same pattern as VOIP, where the server channel-URL builders were
stubs and the DLL had to rebuild them.

Mothership endpoints the game knows: `/v1/moderation/client/report` (client-side report, real handler
`ClientCreateReportCompleteDelegateHandler`), `/v1/moderation/server/report`, `/v1/moderation/server/bans/bulk`
(server side, callers compiled out), plus `x-automation-key`. Solara serves all of these plus
`/v1/moderation/automation` (stores mutes) but **pushes nothing to games**.

## Admin kick fixed 2026-11-05: [KICKSELF]

`Client_KickNicely` makes the kicked client call `Server_KickMyself(Reason)` and wait ("LogA2Moderation: Player asked
to kick themselves"). That server half is compiled out too, so every kick left the player connected and stuck; when
they quit and rejoined, the old connection stayed until its 60 s timeout and the rejoins failed (prod box 1: player
1b726e7f kicked, 4 failed rejoins, then success). `ModAdmin::OnKickSelf` now answers it with
`ClientReturnToMainMenuWithTextReason(reason)`: the client leaves at once (`UNetConnection::Close` on the server) and
rejoins normally (rig-verified; the owner rejoined the rig server right after). Local rig trigger:
`%TEMP%\rbtest_kick.txt` = player id. The Quest crash itself was never captured (no headset log/dump).

## Implemented 2026-11-05: [MUTE] (server DLL) + backend mutes (Solara), no client patch, no websocket

How a listener's client mutes someone (traced): the replicated array's OnRep (`58615C0`, param = old array) marks an
entry dirty (bit 3) when its `0x16` bits changed, then `5887BE0` arms a timer whose callback `588B890` calls
`sub_1456C2870(voip, pawn, (flags & 6) != 0)` (the local Vivox mute) for every dirty entry, every ~2 s.
`Client_SetMutedHud(bool)` on the controller lands in `5891170`: sets pawn `+0x1A64` and the muted HUD (`5871B90`).

`ModAdmin` in `Server/dllmain_50451.cpp` (search `[MUTE 2026-11-05]`):
- `Server_MutePlayerForAll` / `Server_MuteAllPlayersForAll` (moderator panel, needs `UserMute` = 38 or Admin): kept
  on that server only (like the original `ServerOnlyMuteForAllMothershipIDs`); mute-all spares players who may mute.
- Backend mutes: a thread polls `GET /v1/deployments/{id}/mutes` every 5 s (Solara `internal/ares/mutes.go`; store
  `internal/mothership/mutes_export.go`, the ported mutes collection + a Solara-only note collection for the reason).
- Once a second it sets bit 2 (panel) or bits 2+4 (backend) on the muted player's entry in EVERY pawn's list and
  clears them on unmute; the muted player gets `Client_SetMutedHud(true/false)`, and for backend mutes the game's own
  board ("MUTED FOR N MINUTES FOR\n<reason>", "MUTED FOREVER FOR" when permanent). Direct writes to the array DO
  replicate (no push-model mark needed).
- Dashboard: Users page "Voice mutes" (`app/users/GameMutes.tsx`): mute by player id (online picker), 10 min to
  permanent, reason; unmute. API `GET/POST /api/mutes`, `POST /api/mutes/{player_id}/unmute`, scoped perm `users.mute`.
- LOCAL RIG ONLY: `%TEMP%\rbtest_mute.txt` = a player id or `all`. Client test tool (`tools/specedit50451`, launch
  with `-MuteProbe`): bridge op `mutestate` dumps our pawn's entries + HUD flag; `[muteprobe]` lines in
  `%TEMP%\spec_editor.log` log every local Vivox mute call.

Verified on the local rig (2 PC clients + the server's own player): server lists populated (3 lists / 6 entries);
mute -> both clients' entries `0x0C`, `[muteprobe] ... -> 1` for exactly the muted pawn, `mutedHud=1` on the muted
client, board rendered (screenshot); unmute -> `-> 0`, HUD 0. Dashboard API -> game server within one poll.
Not verified: the moderator panel RPC path end to end (needs a client with UserMute), and actually hearing silence
(the rig has no voice; the call into the game's own Vivox mute is what was checked).
Caveat: listener-side, so a modded client could unmute locally; the rig clients share one account, and
`0ada636d...` style ids that never show a HUD are the server's own built-in player.
