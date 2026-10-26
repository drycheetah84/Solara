// Dashboard API for per-server identity tokens (Solara addition, SEC-SID;
// docs/SERVER_IDENTITY.md). Both routes sit under /api, so the dashboard auth
// preprocessor already requires the admin key or an admin session.
//
//	GET    /api/server-tokens        -> {"tokens":[...]} (never the secret)
//	DELETE /api/server-tokens/{id}   -> {"success":true,"token":{...}} | 404
//
// {id} is the record id (hex) or the token's display prefix ("sst1_xxxxxx").
// Issuance stays on the CLI (`solara server-token issue` / `agent-token issue`)
// so a token is only ever shown on the operator's own terminal.
package ares

import (
	"errors"
	"net/url"

	"solara/internal/hosting"
	"solara/internal/serverid"
	"solara/internal/stj"
)

// serverTokenView is the listing shape: everything but the hash.
func serverTokenView(r *serverid.ServerTokenDbObject) stj.Obj {
	opt := func(p *string) any {
		if p == nil {
			return nil
		}
		return *p
	}
	ts := func(ms int64) any {
		if ms == 0 {
			return nil
		}
		return serverid.FormatMs(ms)
	}
	return stj.O(
		"id", r.IdHex(),
		"prefix", r.Prefix,
		"kind", r.Kind,
		"deployment_id", opt(r.DeploymentId),
		"station_id", opt(r.StationId),
		"box", opt(r.Box),
		"issued_at", ts(r.IssuedAt),
		"last_seen", ts(r.LastSeen),
		"expires_at", ts(r.ExpiresAt),
		"absolute_expiry", ts(r.AbsoluteExpiry),
		"revoked_at", ts(r.RevokedAt),
		"revoke_reason", opt(r.RevokeReason),
	)
}

// ListServerTokens is GET /api/server-tokens.
func (s *AresDashboardServer) ListServerTokens(c *hosting.Context) hosting.Result {
	list, err := serverid.List()
	if err != nil {
		return hosting.Configurable(503, "application/json", stj.MarshalString(stj.O("error", err.Error())))
	}
	out := make([]any, 0, len(list))
	for _, r := range list {
		out = append(out, serverTokenView(r))
	}
	return hosting.Ok(stj.O("tokens", out))
}

// RevokeServerToken is DELETE /api/server-tokens/{id}.
func (s *AresDashboardServer) RevokeServerToken(c *hosting.Context) hosting.Result {
	id, err := url.PathUnescape(c.Param("id"))
	if err != nil {
		id = c.Param("id")
	}
	r, err := serverid.Revoke(id, "revoked from the dashboard")
	switch {
	case errors.Is(err, serverid.ErrNotFound):
		return hosting.NotFound(stj.O("success", false, "error", "token not found"))
	case errors.Is(err, serverid.ErrAmbiguous):
		return hosting.BadRequest(stj.O("success", false, "error", "prefix matches more than one token; use the id"))
	case err != nil:
		return hosting.Configurable(503, "application/json", stj.MarshalString(stj.O("success", false, "error", err.Error())))
	}
	return hosting.Ok(stj.O("success", true, "token", serverTokenView(r)))
}
