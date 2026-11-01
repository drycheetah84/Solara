// Solara extension: the 50451 client's progression reads.
//
// GET /v1/progression/client   -> {"Results": [track progress]}  item parser 0x4DD32D0:
//
//	{CurrentLevelName, CurrentLevelId, PlayerId, LastUpdated, Progress, ...}
//
// GET /v1/progression-tree/client -> {"Results": [tree state]}  item parser 0x4DD4620:
//
//	{PlayerId, Track, Tree, NodeDefinitions, ...}
//
// Neither route existed (404 -> "Tracks/Trees Fetch failed ... not a valid JSON object"). The original service's
// track/tree definitions are not available, so a player has no tracked progression yet: an empty list, which
// both list parsers accept.
package mothership

import (
	"solara/internal/hosting"
	"solara/internal/stj"
)

func (s *MothershipServer) ClientGetProgression(c *hosting.Context) hosting.Result {
	if !isClient(c.Request) && !isAdmin(c.Request) {
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}
	return jsonAnon(stj.O("Results", []any{}), 200)
}

func (s *MothershipServer) ClientGetProgressionTrees(c *hosting.Context) hosting.Result {
	if !isClient(c.Request) && !isAdmin(c.Request) {
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}
	return jsonAnon(stj.O("Results", []any{}), 200)
}
