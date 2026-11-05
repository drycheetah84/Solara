package ares

// Solara: backend voice mutes, pushed to every live game server.
//
// A mute stops a player's voice for everyone on whichever game server they are on, until it runs out or is lifted.
// Voice goes client -> Vivox, never through the game server, so the game server DLL enforces it through the game's
// own client-side mute paths (see docs/agent-notes/a2-50451-voice-moderation.md): every listener's replicated
// settings & safety entry for the player gets bMutedForAll + bMothershipMuted, the player gets the muted HUD
// (Client_SetMutedHud) and the game's "MUTED FOR N MINUTES FOR <reason>" board.
//
//   game server -> GET  /v1/deployments/{id}/mutes   text/plain, one active mute per line:
//                                                    "<player id>\t<seconds left, 0 = permanent>\t<reason>"
//   dashboard   -> GET  /api/mutes                   {mutes:[ActiveMute], players:[{id,name,server}]} (who is online)
//   dashboard   -> POST /api/mutes                   {player_id, duration_minutes (0 = permanent), reason}
//   dashboard   -> POST /api/mutes/{player_id}/unmute
//
// Mutes are fleet-wide (a player is muted on every server), stored in the Mothership mutes collection
// (mothership/mutes_export.go). The game servers poll every 5 s, so a change lands within ~6 s.

import (
	"strconv"
	"strings"

	"solara/internal/hosting"
	"solara/internal/mothership"
	"solara/internal/stj"
)

// GetDeploymentMutes is the game server's view: every active mute, one per line.
func (s *A2StationDbServer) GetDeploymentMutes(c *hosting.Context) hosting.Result {
	var b strings.Builder
	for _, m := range mothership.ActiveMutes() {
		reason := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(m.Reason)
		b.WriteString(m.PlayerId)
		b.WriteByte('\t')
		b.WriteString(strconv.FormatInt(m.SecondsLeft, 10))
		b.WriteByte('\t')
		b.WriteString(reason)
		b.WriteByte('\n')
	}
	return hosting.Configurable(200, "text/plain; charset=utf-8", []byte(b.String()))
}

// GetMutes lists the active mutes plus who is online right now (for the player picker).
func (s *AresDashboardServer) GetMutes(c *hosting.Context) hosting.Result {
	notifyMu.Lock()
	players := []NotifyPlayer{}
	for _, d := range notifyFleetLocked() {
		for _, p := range notifyPlayers[d.Id].players {
			p.Server = d.Name
			players = append(players, p)
		}
	}
	notifyMu.Unlock()
	return hosting.Ok(stj.O("mutes", mothership.ActiveMutes(), "players", players))
}

// AddMuteApi mutes a player fleet-wide.
func (s *AresDashboardServer) AddMuteApi(c *hosting.Context) hosting.Result {
	body, err := stj.Parse(c.Request.Body())
	if err != nil {
		return hosting.Ok(dashActionError("invalid JSON body"))
	}
	str := func(k string) string {
		if v, ok := body.TryGetProperty(k); ok && v.ValueKind() == stj.String {
			return strings.TrimSpace(*v.GetString())
		}
		return ""
	}
	minutes := 0
	if v, ok := body.TryGetProperty("duration_minutes"); ok && v.ValueKind() == stj.Number {
		f := v.GetDouble()
		if f != float64(int(f)) {
			return hosting.Ok(dashActionError("duration_minutes must be a whole number"))
		}
		minutes = int(f)
	}
	m, msg := mothership.AddMute(str("player_id"), minutes, str("reason"), "dashboard")
	if m == nil {
		return hosting.Ok(dashActionError(msg))
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(m.MuteId)})
}

// LiftMuteApi unmutes a player fleet-wide.
func (s *AresDashboardServer) LiftMuteApi(c *hosting.Context) hosting.Result {
	playerId := c.Param("player_id")
	if playerId == "" {
		return hosting.Ok(dashActionError("player_id required"))
	}
	if mothership.LiftMutes(playerId) == 0 {
		return hosting.Ok(dashActionError("this player is not muted"))
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(playerId)})
}
