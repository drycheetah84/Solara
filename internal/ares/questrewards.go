// Solara addition: quest completion rewards (StationDb, :78).
//
//	POST /v1/deployments/{deployment_id}/quest_reward   (trusted game server only)
//	  {"event":"on_permanent_quest_cosmetic_grant","params":{"mothership_player_id":"...","transaction_id":"...",
//	   "completed_quest_id":"...","completed_quest_version":"..","completed_permanent_reward_version":".."}}
//
// The game server emits the event through the Mothership events subsystem, which has no backend on Solara; the
// server DLL forwards it here. See internal/mothership/questrewards.go.
package ares

import (
	"encoding/json"
	"strings"

	"solara/internal/hosting"
	"solara/internal/mothership"
	"solara/internal/stj"
)

func (s *A2StationDbServer) PostQuestReward(c *hosting.Context) hosting.Result {
	// Only a game server may grant: a player key could otherwise mint its own rewards.
	cred := a2Credential(c, "A2StationDb Solara/PostQuestReward", a2ServerToken(c))
	if !cred.trusted {
		return a2Forbidden()
	}
	var body struct {
		Event  string            `json:"event"`
		Params map[string]string `json:"params"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		return hosting.BadRequest(stj.O("error", "body must be {event, params}"))
	}
	if body.Event != "on_permanent_quest_cosmetic_grant" {
		// XP grants (on_permanent_quest_xp_grant) have no backend track data to apply; accept and ignore.
		return hosting.Ok(stj.O("applied", false, "reason", "event not handled"))
	}
	p := body.Params
	pid := strings.TrimSpace(p["mothership_player_id"])
	if r := mothership.ResolvePlayerId(pid); r != "" {
		pid = r
	}
	granted, errMsg := mothership.ApplyQuestReward(pid, p["transaction_id"], p["completed_quest_id"], p["completed_permanent_reward_version"])
	if errMsg != "" {
		return hosting.BadRequest(stj.O("error", errMsg))
	}
	return hosting.Ok(stj.O("applied", len(granted) > 0, "player_id", pid, "granted", granted))
}
