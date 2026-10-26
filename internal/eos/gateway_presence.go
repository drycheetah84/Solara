// Port of AUnrealFeatures.EOSSDK/EosGatewayServer.Presence.cs.
//
// EOS Presence (Social interface). Real traffic observed:
//
//	POST /epic/presence/internal/v1/_/{account_id}/presence/{connection_id}
//	(sent with X-HTTP-Method-Override: PATCH — actual wire verb is POST)
//	Body: {"status":"online","conn":{"props":{}}}
//
// Just ack it; nothing downstream consumes stored presence state.
package eos

import (
	"solara/internal/hosting"
	"solara/internal/stj"
)

// UpdatePresence mirrors EosGatewayServer.UpdatePresence.
func (s *EosGatewayServer) UpdatePresence(c *hosting.Context) hosting.Result {
	body := utf8GetString(c.Request.Body())
	s.HTTP.Logger.Information("Presence update for account={AccountId} conn={ConnectionId}: {Body}",
		c.Param("account_id"), c.Param("connection_id"), body)
	return hosting.Configurable(200, "application/json", stj.MustMarshal(stj.O()))
}
