// Authorization for the A2StationDb API (:78) — SECURITY HARDENING, a
// deliberate deviation from the original (KNOWN_ISSUES SEC-F2).
//
// The original verified credentials on only a handful of routes, so anyone
// reaching :78 could mint an API key for any user (POST /users/{id}/api_key),
// grant roles, delete users or rewrite station config. The API's own native
// model is key-based: clients log in, receive an API key and send it as
// `x-api-key` (the dedicated server sends the master key, -DashboardApiKey).
// This layer enforces that model:
//
//	a2Admin  — master key (x-api-key), game-server key (x-server-api-key), or a
//	           user key whose user holds "admin" on the target station or
//	           globally (station Admins keep in-game management).
//	a2Writer — any credential this backend issued: master/server key, a user
//	           key from a login, or a deployment key.
//	a2Open   — unchanged: logins, reads, the game-server ingest routes
//	           (register_server, update_player_count, server_heartbeat — the
//	           DLL's credentials on those could not be verified, and refusing
//	           them would stop servers registering), the spec-level routes
//	           (public by design in the original, asserted by its tests), the
//	           drafters feed and board images. Routes that already performed
//	           their own master-key check keep it.
//
// Per-server identity tokens (SEC-SID, docs/SERVER_IDENTITY.md):
//
//   - A valid Kind="server" token in x-server-api-key or x-api-key counts as
//     the game-server credential (admin + writer tiers), BUT bound: on every
//     route (open ones included) a deployment_id the request names (route
//     parameter, query, or top-level JSON body property) must be the token's
//     deployment, and a station_id it names must be the token's station (its
//     own binding or the station its deployment registered under) -> else
//     403 {"error":"forbidden"}.
//   - The static ServerMasterKey / SERVER_API_KEY are accepted in compat mode
//     (deprecation warning per route, once an hour) and rejected when
//     SOLARA_REQUIRE_SERVER_TOKENS=1.
//   - The dashboard admin key (x-dashboard-key) is an admin credential here,
//     so admins keep a way in when strict mode retires the master key.
//   - Ingest routes (register_server, update_player_count, server_heartbeat)
//     stay open in compat mode and require a valid server token in strict mode.
//   - Purge/prune (maintenance) never accept server tokens.
package ares

import (
	"crypto/subtle"
	"net/url"
	"strings"

	"solara/internal/ares/models"
	"solara/internal/hosting"
	"solara/internal/serverid"
	"solara/internal/stj"
)

type a2AuthLevel int

const (
	a2Open a2AuthLevel = iota
	a2Writer
	a2Admin
)

// a2RouteAuth assigns a level to each state-changing route (by C# method name).
var a2RouteAuth = map[string]a2AuthLevel{
	// Solara: challenge records -- any issued key; the handler limits a player key to its own user_id
	"Solara/AddChallengeRecord": a2Writer,
	// Solara: quest rewards -- the handler further requires a trusted (game server) credential
	"Solara/PostQuestReward": a2Writer,
	// account / privilege management
	"CreateUserApiKey":           a2Admin,
	"CreateOrUpdateUser":         a2Admin,
	"DeleteUser":                 a2Admin,
	"CreateDeploymentApiKey":     a2Admin,
	"DeleteDeployment":           a2Admin,
	"DeleteStation":              a2Admin,
	"CreateStationRole":          a2Admin,
	"UpdateStationRole":          a2Admin,
	"DeleteStationRole":          a2Admin,
	"AssignPerms":                a2Admin,
	"AddStationUserRole":         a2Admin,
	"GrantStationUserRoleByName": a2Admin,
	"DeleteStationUserRole":      a2Admin,
	"AddStationUserRoleByName":   a2Admin,
	"BanStationUser":             a2Admin,
	"UnbanStationUser":           a2Admin,
	// other writes
	"CreateDeployment":         a2Writer,
	"UpdateDeployment":         a2Writer,
	"AddServerEvent":           a2Writer,
	"SetDeploymentConfigV1":    a2Writer,
	"DeleteDeploymentConfigV1": a2Writer,
	"CreateStation":            a2Writer,
	"UpdateStation":            a2Writer,
	"SetStationConfig":         a2Writer,
	"DeleteStationConfig":      a2Writer,
	"CreateStationEvent":       a2Writer,
	"UpdateStationEvent":       a2Writer,
	"DeleteStationEvent":       a2Writer,
	"SignupStationEvent":       a2Writer,
	"DeleteStationEventSignup": a2Writer,
	"SetStationEventConfig":    a2Writer,
	"DeleteStationEventConfig": a2Writer,
}

func a2CtEq(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// a2IngestRoutes are the game-server ingest routes: open in compat mode, a
// valid server token required in strict mode (SOLARA_REQUIRE_SERVER_TOKENS=1).
var a2IngestRoutes = map[string]bool{
	"RegisterServer":    true,
	"UpdatePlayerCount": true,
	"ServerHeartbeat":   true,
}

// a2Cred is the classified credential of a request.
type a2Cred struct {
	// trusted: a game-server identity (per-server token, or a static shared key
	// the strict/compat policy still allows) or the dashboard admin key.
	trusted bool
	token   *serverid.ServerTokenDbObject
	user    *models.UserDataResponse // the user behind a user key
	issued  bool                     // any key this backend issued
}

// a2ServerToken returns the valid per-server token (Kind "server") presented
// in x-server-api-key or x-api-key, or nil.
func a2ServerToken(c *hosting.Context) *serverid.ServerTokenDbObject {
	k := serverid.TokenFromHeaders(c.Request.Header)
	if k == "" {
		return nil
	}
	if r, ok := serverid.Validate(k); ok && r.Kind == serverid.KindServer {
		return r
	}
	return nil
}

// a2DashboardAdmin: the dashboard (:8080) admin key in x-dashboard-key is an
// admin credential on :78 too (the strict-mode replacement for admins' use of
// ServerMasterKey). Only consulted when the header is present.
func a2DashboardAdmin(c *hosting.Context) bool {
	if k, ok := c.Request.Header("x-dashboard-key"); !ok || k == "" {
		return false
	}
	return DashboardAuthPreprocessorHasValidAdminKey(c.Request)
}

// a2StaticMasterKey: x-api-key is the static ServerMasterKey and the policy
// still allows it (compat mode, with a deprecation warning naming route).
func a2StaticMasterKey(c *hosting.Context, route string) bool {
	k, ok := c.Request.Header("x-api-key")
	return ok && a2CtEq(k, a2ServerMasterKey()) && serverid.StaticKeyAllowed("ServerMasterKey", route)
}

// a2StaticServerKey: x-server-api-key is the static SERVER_API_KEY, policy permitting.
func a2StaticServerKey(c *hosting.Context, route string) bool {
	k, ok := c.Request.Header("x-server-api-key")
	return ok && a2CtEq(k, a2SpecServerApiKey()) && serverid.StaticKeyAllowed("SERVER_API_KEY", route)
}

// a2MaintenanceAdmin gates the admin-only maintenance handlers (purge,
// prune): the static master key (compat mode only) or the dashboard admin
// key. Per-server tokens deliberately do NOT qualify.
func a2MaintenanceAdmin(c *hosting.Context, route string) bool {
	return a2DashboardAdmin(c) || a2StaticMasterKey(c, route)
}

// a2Credential classifies the request's credential.
func a2Credential(c *hosting.Context, route string, tok *serverid.ServerTokenDbObject) a2Cred {
	if tok != nil {
		return a2Cred{trusted: true, token: tok, issued: true}
	}
	if a2DashboardAdmin(c) || a2StaticMasterKey(c, route) || a2StaticServerKey(c, route) {
		return a2Cred{trusted: true, issued: true}
	}
	k, ok := c.Request.Header("x-api-key")
	if !ok || k == "" || serverid.LooksLikeToken(k) {
		return a2Cred{}
	}
	if keys := a2Keys(); keys != nil {
		// exact (case-sensitive) match: keys are secrets, not names
		if e := keys.FindOne(func(x *models.UserApiKeyDbObject) bool { return x.ApiKey != nil && a2CtEq(*x.ApiKey, k) }); e != nil {
			return a2Cred{user: a2FindUser(e.UserId), issued: true}
		}
	}
	if deps := a2Deployments(); deps != nil {
		if d := deps.FindOne(func(x *models.DeploymentDbObject) bool {
			for _, ak := range x.ApiKeys {
				if a2CtEq(ak, k) {
					return true
				}
			}
			return false
		}); d != nil {
			return a2Cred{issued: true}
		}
	}
	return a2Cred{}
}

// a2TargetStation resolves the station a request acts on ("" if none).
func a2TargetStation(c *hosting.Context) string {
	if c.HasParam("station_id") {
		return c.Param("station_id")
	}
	if c.HasParam("deployment_id") {
		if d := a2FindDeployment(c.Param("deployment_id")); d != nil && d.StationId != nil {
			return *d.StationId
		}
	}
	return ""
}

func a2IsAdminFor(user *models.UserDataResponse, station string) bool {
	if user == nil {
		return false
	}
	for _, p := range a2GetPermissionsForUser(user) {
		if p == nil || p.Permission == nil || p.StationId == nil {
			continue
		}
		perm, st := *p.Permission, *p.StationId
		if perm != "admin" && perm != "global:admin" {
			continue
		}
		if st == "global" || (station != "" && st == station) {
			return true
		}
	}
	return false
}

func a2Unauthorized() hosting.Result { return hosting.Unauthorized(stj.O("error", "unauthorized")) }

func a2Forbidden() hosting.Result { return hosting.Forbidden(stj.O("error", "forbidden")) }

// a2RequestIds collects the values a request names for key: the route
// parameter, the query parameter and a top-level JSON body string property.
func a2RequestIds(c *hosting.Context, key string) []string {
	var out []string
	if c.HasParam(key) {
		out = append(out, a2UnescapeParam(c.Param(key)))
	}
	if v, ok := c.Request.Query(key); ok && v != "" {
		out = append(out, v)
	}
	if body := c.Request.Body(); len(body) > 0 {
		if el, err := stj.Parse(body); err == nil && el.ValueKind() == stj.Object {
			if v, ok := el.TryGetProperty(key); ok && v.ValueKind() == stj.String {
				out = append(out, v.Str())
			}
		}
	}
	return out
}

// a2UnescapeParam decodes a raw (percent-encoded) route parameter.
func a2UnescapeParam(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// a2TokenStation is the station a server token may act on: its own binding,
// else the station its deployment registered under ("" = unknown).
func a2TokenStation(tok *serverid.ServerTokenDbObject) string {
	if st := tok.Station(); st != "" {
		return st
	}
	if d := a2FindDeployment(tok.Deployment()); d != nil && d.StationId != nil {
		return *d.StationId
	}
	return ""
}

// a2CheckTokenBinding enforces a server token's bindings: every deployment id
// the request names must be the token's deployment, and every station id it
// names must be the token's station. Ids compare case-insensitively, like the
// database lookups they feed. nil = allowed.
func a2CheckTokenBinding(c *hosting.Context, tok *serverid.ServerTokenDbObject) hosting.Result {
	for _, d := range a2RequestIds(c, "deployment_id") {
		if !strings.EqualFold(d, tok.Deployment()) {
			return a2Forbidden()
		}
	}
	if sts := a2RequestIds(c, "station_id"); len(sts) > 0 {
		own := a2TokenStation(tok)
		for _, st := range sts {
			if own == "" || !strings.EqualFold(st, own) {
				return a2Forbidden()
			}
		}
	}
	return nil
}

// a2Guard wraps a handler with the route's authorization level and the
// per-server token rules (binding, strict-mode ingest).
func a2Guard(name string, h hosting.Handler) hosting.Handler {
	level := a2RouteAuth[name]
	ingest := a2IngestRoutes[name]
	route := "A2StationDb " + name
	return func(c *hosting.Context) hosting.Result {
		tok := a2ServerToken(c)
		if tok != nil {
			if r := a2CheckTokenBinding(c, tok); r != nil {
				return r
			}
		}
		if ingest && tok == nil && serverid.RequireServerTokens() {
			return a2Unauthorized()
		}
		if level == a2Open {
			return h(c)
		}
		cred := a2Credential(c, route, tok)
		switch level {
		case a2Writer:
			if !cred.issued {
				return a2Unauthorized()
			}
		case a2Admin:
			// A per-server token is narrower than the shared static key it
			// replaces (SEC-SID-3): it may only perform admin actions on its
			// own station; station-less admin routes (minting user API keys,
			// creating/deleting users) never accept a server token.
			if cred.token != nil {
				target := a2TargetStation(c)
				if target == "" || target != a2TokenStation(cred.token) {
					return a2Unauthorized()
				}
				break
			}
			if !cred.trusted && !a2IsAdminFor(cred.user, a2TargetStation(c)) {
				return a2Unauthorized()
			}
		}
		return h(c)
	}
}
