package ares

import (
	"solara/internal/hosting"
	"solara/internal/mothership"
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
