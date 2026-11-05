package ares

import (
	"strings"

	"solara/internal/hosting"
	"solara/internal/mothership"
	"solara/internal/stj"
)

// GetGameBans (Solara) lists the still-active Mothership bans -- the ones the in-game moderation panel and
// game servers issue, which the station ban list (user.Bans) does not hold. GET /api/game_bans
func (s *AresDashboardServer) GetGameBans(c *hosting.Context) hosting.Result {
	return hosting.Ok(mothership.ActiveBans())
}

// LiftGameBans (Solara) ends a Mothership player's active bans now. POST /api/game_bans/{player_id}/unban
func (s *AresDashboardServer) LiftGameBans(c *hosting.Context) hosting.Result {
	playerId := c.Param("player_id")
	if playerId == "" {
		return hosting.Ok(dashActionError("player_id required"))
	}
	if n := mothership.LiftBans(playerId); n == 0 {
		return hosting.Ok(dashActionError("no active ban for this player"))
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(playerId)})
}

// AddGameBan (Solara) bans a player globally: no login, and their current session stops working.
// POST /api/game_bans {player_id (Mothership or Meta id), duration_minutes (0 = permanent), reason}
func (s *AresDashboardServer) AddGameBan(c *hosting.Context) hosting.Result {
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
		if f != float64(int(f)) || f < 0 {
			return hosting.Ok(dashActionError("duration_minutes must be a whole number of minutes (0 = permanent)"))
		}
		minutes = int(f)
	}
	ids, msg := mothership.AddBan(str("player_id"), minutes, str("reason"), "dashboard")
	if ids == nil {
		return hosting.Ok(dashActionError(msg))
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(strings.Join(ids, ","))})
}
