// Solara addition: scoped dashboard tokens.
//
// The dashboard admin key (DASHBOARD_ADMIN_KEY, the "master" key) can mint tokens that carry a chosen set of
// permissions, limited to chosen stations / fleets. Only the master key can create, list or revoke them.
//
//	GET    /api/scoped-tokens/permissions  -> {"permissions":[{name, description}]}  (master)
//	GET    /api/scoped-tokens              -> {"tokens":[view]}                      (master)
//	POST   /api/scoped-tokens              -> {"token":"sdt1_...", ...view}          (master; the secret is shown once)
//	       body {name, permissions:[...], stations:[...], fleets:[...], expires_in_days?, expires_in_minutes?}
//	DELETE /api/scoped-tokens/{id}         -> {"success":true}                       (master)
//	GET    /api/scoped-tokens/self         -> {"master":bool, ...token view}           (any valid credential)
//
// A scoped token is presented like the admin key (x-dashboard-key or x-api-key). Every /api route maps to one
// permission (dashScopedRoutes); a route that names a station ({station_id}, or a drafters {scope} that is not
// "global") also needs that station in the token's scope. Station-less routes need the "*" scope, except
// GET /api/stations and GET /api/deployments, which a scoped token may call and which only list its stations.
// Routes not in the table (server tokens, admin grants, scoped-token management) stay master-only. In Solara a fleet IS a station (fleet_id == station_id, see a2station_v2.go),
// so "fleets" and "stations" are one scope list. Tokens are stored as SHA-256 hashes; the secret is never stored.
package ares

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const dashScopedPrefix = "sdt1_"

// DashScopedTokenDbObject is one stored scoped token (hash only).
type DashScopedTokenDbObject struct {
	docdb.AstraDbObject
	TokenId     string   `json:"token_id"`
	Name        string   `json:"name"`
	Prefix      string   `json:"prefix"`
	Hash        string   `json:"hash"`
	Permissions []string `json:"permissions"`
	Stations    []string `json:"stations"`
	CreatedAt   int64    `json:"created_at"`
	ExpiresAt   int64    `json:"expires_at"`
	RevokedAt   int64    `json:"revoked_at"`
	LastUsed    int64    `json:"last_used"`
}

func NewDashScopedTokenDbObject() *DashScopedTokenDbObject { return &DashScopedTokenDbObject{} }

func dashScopedTokens() *docdb.Collection[DashScopedTokenDbObject] {
	return docdb.GetCollection(Database, true, NewDashScopedTokenDbObject)
}

// dashScopedPerms is the permission catalogue (name -> what it allows).
var dashScopedPerms = []struct{ Name, Desc string }{
	{"fleets.view", "view the fleet list only: station names and player counts"},
	{"stats.read", "server stats, telemetry, events, agents, netvar dumps, websocket clients"},
	{"users.read", "list users and view inventories"},
	{"users.ban", "ban / unban users and lift game bans"},
	{"users.inventory", "grant and revoke inventory items"},
	{"catalog.read", "list grantable catalogue items"},
	{"stations.read", "view stations, config, whitelist, members, bans, roles, board, deployments, events, gaia"},
	{"stations.config", "edit station config, whitelist and board image"},
	{"stations.manage", "create stations, create deployments, spin up servers"},
	{"stations.delete", "delete stations / fleets"},
	{"stations.events", "create and delete station events"},
	{"stations.roles", "create / delete roles, edit role permissions, assign / remove user roles"},
	{"stations.gaia", "play Gaia sequences"},
	{"spec.read", "view saved Spec Editor levels"},
	{"spec.write", "edit and delete saved Spec Editor levels"},
	{"drafters.read", "list Spec Editor drafters"},
	{"drafters.write", "grant and revoke Spec Editor drafters"},
	{"sessions.read", "list EOS sessions"},
	{"sessions.write", "add and remove EOS sessions"},
}

type dashScopedRoute struct {
	Method, Pattern, Perm string
	ListFiltered          bool // station-less list a scoped token may call (the handler filters it)
}

var dashScopedRoutes = []dashScopedRoute{
	{"GET", "/api/stats", "stats.read", false},
	{"GET", "/api/telemetry", "stats.read", false},
	{"GET", "/api/events", "stats.read", false},
	{"GET", "/api/agents", "stats.read", false},
	{"GET", "/api/netvars/dumps", "stats.read", false},
	{"GET", "/api/netvars/dump", "stats.read", false},
	{"GET", "/api/ws-clients", "stats.read", false},
	{"GET", "/api/ws-clients/{session_id}/messages", "stats.read", false},
	{"GET", "/api/users", "users.read|stations.roles|stations.config", true}, // player picker for roles / arena whitelists
	{"GET", "/api/users/{user_id}/inventory", "users.read", false},
	{"POST", "/api/users/{user_id}/ban", "users.ban", false},
	{"PATCH", "/api/users/{user_id}/unban", "users.ban", false},
	{"GET", "/api/game_bans", "users.ban", false},
	{"POST", "/api/game_bans/{player_id}/unban", "users.ban", false},
	{"POST", "/api/users/{user_id}/inventory", "users.inventory", false},
	{"DELETE", "/api/users/{user_id}/inventory/{item_id}", "users.inventory", false},
	{"GET", "/api/catalog/items", "catalog.read", false},
	{"GET", "/api/board/keys", "stations.read", true}, // the fixed slot list, not station data
	{"GET", "/api/stations", "stations.read|fleets.view", true},
	{"GET", "/api/deployments", "stations.read|fleets.view", true},
	{"GET", "/api/stations/{station_id}/config", "stations.read", false},
	{"GET", "/api/stations/{station_id}/whitelist", "stations.read", false},
	{"GET", "/api/stations/{station_id}/members", "stations.read", false},
	{"GET", "/api/stations/{station_id}/bans", "stations.read", false},
	{"GET", "/api/stations/{station_id}/roles", "stations.read", false},
	{"GET", "/api/stations/{station_id}/board", "stations.read", false},
	{"GET", "/api/stations/{station_id}/deployments", "stations.read", false},
	{"GET", "/api/stations/{station_id}/events", "stations.read", false},
	{"GET", "/api/stations/{station_id}/gaia", "stations.read", false},
	{"PATCH", "/api/stations/{station_id}/config", "stations.config", false},
	{"DELETE", "/api/stations/{station_id}/config", "stations.config", false},
	{"PUT", "/api/stations/{station_id}/whitelist", "stations.config", false},
	{"POST", "/api/stations/{station_id}/board", "stations.config", false},
	{"DELETE", "/api/stations/{station_id}/board", "stations.config", false},
	{"POST", "/api/stations", "stations.manage", false},
	{"DELETE", "/api/stations/{station_id}", "stations.delete", false},
	{"POST", "/api/stations/{station_id}/deployments", "stations.manage", false},
	{"POST", "/api/stations/{station_id}/spinup", "stations.manage", false},
	{"POST", "/api/stations/{station_id}/events", "stations.events", false},
	{"DELETE", "/api/stations/{station_id}/events/{event_id}", "stations.events", false},
	{"POST", "/api/stations/{station_id}/roles", "stations.roles", false},
	{"DELETE", "/api/stations/{station_id}/roles/{role_id}", "stations.roles", false},
	{"PATCH", "/api/stations/{station_id}/roles/{role_id}/permissions", "stations.roles", false},
	{"POST", "/api/stations/{station_id}/users/{user_id}/roles/{role_id}", "stations.roles", false},
	{"DELETE", "/api/stations/{station_id}/users/{user_id}/roles/{role_id}", "stations.roles", false},
	{"POST", "/api/stations/{station_id}/gaia/play", "stations.gaia", false},
	{"GET", "/api/spec/levels", "spec.read", false},
	{"GET", "/api/spec/levels/{name}", "spec.read", false},
	{"PATCH", "/api/spec/levels/{name}", "spec.write", false},
	{"DELETE", "/api/spec/levels/{name}", "spec.write", false},
	{"GET", "/api/drafters/{scope}", "drafters.read", false},
	{"POST", "/api/drafters/{scope}/{user_id}", "drafters.write", false},
	{"DELETE", "/api/drafters/{scope}/{user_id}", "drafters.write", false},
	{"GET", "/api/sessions", "sessions.read", false},
	{"POST", "/api/sessions", "sessions.write", false},
	{"DELETE", "/api/sessions/{session_id}", "sessions.write", false},
}

func dashKnownPerm(p string) bool {
	for _, x := range dashScopedPerms {
		if x.Name == p {
			return true
		}
	}
	return false
}

// dashMatchRoute matches a request path against the table; params are the {name} segments.
func dashMatchRoute(method, path string) (*dashScopedRoute, map[string]string) {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimSuffix(path, "/")
	segs := strings.Split(path, "/")
	for i := range dashScopedRoutes {
		r := &dashScopedRoutes[i]
		if !strings.EqualFold(r.Method, method) {
			continue
		}
		ps := strings.Split(r.Pattern, "/")
		if len(ps) != len(segs) {
			continue
		}
		params, ok := map[string]string{}, true
		for k := range ps {
			if strings.HasPrefix(ps[k], "{") {
				params[strings.Trim(ps[k], "{}")] = segs[k]
			} else if !strings.EqualFold(ps[k], segs[k]) {
				ok = false
				break
			}
		}
		if ok {
			return r, params
		}
	}
	return nil, nil
}

func dashTokenHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// dashPresentedKey is the credential header exactly as DashboardAuthPreprocessorHasValidAdminKey reads it.
func dashPresentedKey(req *hosting.Request) string {
	k, ok := req.Header("x-dashboard-key")
	if !ok {
		k, _ = req.Header("x-api-key")
	}
	return strings.TrimSpace(k)
}

// dashScopedTokenFor returns the live (not revoked / expired) token the request presents, or nil.
func dashScopedTokenFor(req *hosting.Request) *DashScopedTokenDbObject {
	k := dashPresentedKey(req)
	if !strings.HasPrefix(k, dashScopedPrefix) {
		return nil
	}
	col := dashScopedTokens()
	if col == nil {
		return nil
	}
	h := dashTokenHash(k)
	t := col.FindOne(func(t *DashScopedTokenDbObject) bool { return dashboardCtEquals(t.Hash, h) })
	if t == nil || t.RevokedAt != 0 || (t.ExpiresAt != 0 && time.Now().UnixMilli() > t.ExpiresAt) {
		return nil
	}
	return t
}

// allows: the token grants perm for station ("" = a station-less route, which needs the "*" scope). A permission
// entry is either "perm" (valid for the token's Stations) or "perm@<station>" / "perm@*" (valid for that station only),
// so a token built from several Discord roles keeps each role's permissions on that role's stations.
func (t *DashScopedTokenDbObject) allows(perm, station string) bool {
	for _, p := range t.Permissions {
		name, at, scoped := strings.Cut(p, "@")
		if name != perm {
			continue
		}
		if !scoped {
			if station == "" {
				if t.hasGlobal() {
					return true
				}
			} else if t.AllowsStation(station) {
				return true
			}
			continue
		}
		if at == "*" || (station != "" && strings.EqualFold(at, station)) {
			return true
		}
	}
	return false
}

func (t *DashScopedTokenDbObject) hasGlobal() bool {
	for _, s := range t.Stations {
		if s == "*" {
			return true
		}
	}
	return false
}

// hasAny: the token holds perm for at least one station (used by list routes, which filter per station).
func (t *DashScopedTokenDbObject) hasAny(perm string) bool {
	for _, p := range t.Permissions {
		if name, _, _ := strings.Cut(p, "@"); name == perm {
			return true
		}
	}
	return false
}

// AllowsStation: the token's scope includes this station / fleet ("*" = every station), either through its Stations
// list or through a station-scoped permission entry ("perm@<station>" / "perm@*").
func (t *DashScopedTokenDbObject) AllowsStation(id string) bool {
	for _, s := range t.Stations {
		if s == "*" || strings.EqualFold(s, id) {
			return true
		}
	}
	for _, p := range t.Permissions {
		if _, at, scoped := strings.Cut(p, "@"); scoped && (at == "*" || strings.EqualFold(at, id)) {
			return true
		}
	}
	return false
}

// dashScopedDecision authorizes a scoped token for one request: "" = allowed, else the refusal reason.
func dashScopedDecision(t *DashScopedTokenDbObject, method, path string) string {
	if strings.EqualFold(method, "GET") && strings.EqualFold(strings.TrimSuffix(strings.SplitN(path, "?", 2)[0], "/"), "/api/scoped-tokens/self") {
		return "" // any live token may read its own permissions / scope
	}
	r, params := dashMatchRoute(method, path)
	if r == nil {
		return "route is master-key only"
	}
	station := params["station_id"]
	if s, ok := params["scope"]; ok && !strings.EqualFold(s, "global") {
		station = s
	}
	perms := strings.Split(r.Perm, "|") // a route may accept any of several permissions
	if r.ListFiltered && station == "" {
		for _, p := range perms {
			if t.hasAny(p) {
				return "" // the handler lists only the token's stations
			}
		}
		return "missing permission " + r.Perm
	}
	for _, p := range perms {
		if t.allows(p, station) {
			return ""
		}
	}
	held := false
	for _, p := range perms {
		held = held || t.hasAny(p)
	}
	switch {
	case !held:
		return "missing permission " + r.Perm
	case station != "":
		return "station " + station + " is outside this token's scope"
	default:
		return "route is not station-specific: needs the \"*\" scope"
	}
}

// dashRequestScope is the scoped token behind a request that the preprocessor let through, or nil for the master
// key / an admin session. List handlers use it to show only the token's stations.
func dashRequestScope(req *hosting.Request) *DashScopedTokenDbObject {
	if DashboardAuthPreprocessorHasValidAdminKey(req) || dashboardHasValidSession(req) {
		return nil
	}
	return dashScopedTokenFor(req)
}

func dashScopedView(t *DashScopedTokenDbObject) stj.Obj {
	ms := func(v int64) any {
		if v == 0 {
			return nil
		}
		return time.UnixMilli(v).UTC().Format(time.RFC3339)
	}
	return stj.O("id", t.TokenId, "name", t.Name, "prefix", t.Prefix, "permissions", t.Permissions, "stations", t.Stations,
		"created_at", ms(t.CreatedAt), "expires_at", ms(t.ExpiresAt), "revoked_at", ms(t.RevokedAt), "last_used", ms(t.LastUsed))
}

func dashJson(status int, v any) hosting.Result {
	return hosting.Configurable(status, "application/json", stj.MarshalString(v))
}

func dashMasterOnly(c *hosting.Context) hosting.Result {
	if !DashboardAuthPreprocessorHasValidAdminKey(c.Request) {
		return dashJson(403, stj.O("error", "only the master dashboard key can manage scoped tokens"))
	}
	return nil
}

// ScopedTokenSelf is GET /api/scoped-tokens/self: what the caller's credential may do. The master key (or an admin
// session) -> {"master":true}; a scoped token -> {"master":false, ...its view}. The Next.js dashboard uses it to
// validate a token at login and to show the session's access.
func (s *AresDashboardServer) ScopedTokenSelf(c *hosting.Context) hosting.Result {
	if DashboardAuthPreprocessorHasValidAdminKey(c.Request) || dashboardHasValidSession(c.Request) {
		return dashJson(200, stj.O("master", true))
	}
	t := dashScopedTokenFor(c.Request)
	if t == nil {
		return dashJson(401, stj.O("error", "unauthorized"))
	}
	return dashJson(200, append(stj.O("master", false), dashScopedView(t)...))
}

// ListScopedTokenPerms is GET /api/scoped-tokens/permissions.
func (s *AresDashboardServer) ListScopedTokenPerms(c *hosting.Context) hosting.Result {
	if r := dashMasterOnly(c); r != nil {
		return r
	}
	out := []any{}
	for _, p := range dashScopedPerms {
		out = append(out, stj.O("name", p.Name, "description", p.Desc))
	}
	return dashJson(200, stj.O("permissions", out))
}

// ListScopedTokens is GET /api/scoped-tokens.
func (s *AresDashboardServer) ListScopedTokens(c *hosting.Context) hosting.Result {
	if r := dashMasterOnly(c); r != nil {
		return r
	}
	out := []any{}
	if col := dashScopedTokens(); col != nil {
		all := col.FindAll()
		sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt > all[j].CreatedAt })
		for _, t := range all {
			out = append(out, dashScopedView(t))
		}
	}
	return dashJson(200, stj.O("tokens", out))
}

// CreateScopedToken is POST /api/scoped-tokens.
func (s *AresDashboardServer) CreateScopedToken(c *hosting.Context) hosting.Result {
	if r := dashMasterOnly(c); r != nil {
		return r
	}
	doc, err := stj.Parse(c.Request.Body())
	if err != nil {
		return dashJson(400, stj.O("error", "body must be JSON"))
	}
	strs := func(key string) []string {
		out := []string{}
		if v, ok := doc.TryGetProperty(key); ok {
			for _, e := range v.EnumerateArray() {
				if sp := e.GetString(); sp != nil && strings.TrimSpace(*sp) != "" {
					out = append(out, strings.TrimSpace(*sp))
				}
			}
		}
		return out
	}
	name := ""
	if v, ok := doc.TryGetProperty("name"); ok {
		if sp := v.GetString(); sp != nil {
			name = *sp
		}
	}
	perms := strs("permissions")
	if len(perms) == 0 {
		return dashJson(400, stj.O("error", "permissions: at least one is required"))
	}
	seen := map[string]bool{}
	uniq := []string{}
	allScoped := true
	for _, p := range perms {
		name, at, scoped := strings.Cut(p, "@")
		allScoped = allScoped && scoped
		if !dashKnownPerm(name) {
			return dashJson(400, stj.O("error", "unknown permission "+name))
		}
		if scoped && at != "*" && a2FindStation(at) == nil {
			return dashJson(400, stj.O("error", "unknown station / fleet "+at+" in "+p))
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	scope := append(strs("stations"), strs("fleets")...) // a fleet id is a station id in Solara
	if len(scope) == 0 && !allScoped {
		return dashJson(400, stj.O("error", "stations / fleets: name at least one (or \"*\" for every station)"))
	}
	stSeen := map[string]bool{}
	stations := []string{}
	for _, id := range scope {
		if id != "*" && a2FindStation(id) == nil {
			return dashJson(400, stj.O("error", "unknown station / fleet "+id))
		}
		if !stSeen[strings.ToLower(id)] {
			stSeen[strings.ToLower(id)] = true
			stations = append(stations, id)
		}
	}
	sort.SliceStable(stations, func(i, j int) bool { return stations[i] == "*" && stations[j] != "*" })
	var expires int64
	if v, ok := doc.TryGetProperty("expires_in_days"); ok {
		if d, ok := v.TryGetInt64(); ok && d > 0 {
			expires = time.Now().Add(time.Duration(d) * 24 * time.Hour).UnixMilli()
		}
	}
	// expires_in_minutes (Solara, Discord sign-in): short-lived per-session tokens; wins over expires_in_days.
	if v, ok := doc.TryGetProperty("expires_in_minutes"); ok {
		if m, ok := v.TryGetInt64(); ok && m > 0 {
			expires = time.Now().Add(time.Duration(m) * time.Minute).UnixMilli()
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return dashJson(500, stj.O("error", "could not generate a token"))
	}
	secret := dashScopedPrefix + hex.EncodeToString(raw)
	idb := make([]byte, 8)
	_, _ = rand.Read(idb)
	t := &DashScopedTokenDbObject{TokenId: hex.EncodeToString(idb), Name: strings.TrimSpace(name), Prefix: secret[:len(dashScopedPrefix)+8],
		Hash: dashTokenHash(secret), Permissions: uniq, Stations: stations, CreatedAt: time.Now().UnixMilli(), ExpiresAt: expires}
	col := dashScopedTokens()
	if col == nil {
		return dashJson(503, stj.O("error", "database unavailable"))
	}
	col.Insert(t)
	return dashJson(200, append(stj.O("token", secret), dashScopedView(t)...))
}

// RevokeScopedToken is DELETE /api/scoped-tokens/{id} (record id or display prefix).
func (s *AresDashboardServer) RevokeScopedToken(c *hosting.Context) hosting.Result {
	if r := dashMasterOnly(c); r != nil {
		return r
	}
	id := c.Param("id")
	col := dashScopedTokens()
	if col == nil {
		return dashJson(503, stj.O("error", "database unavailable"))
	}
	t := col.FindOne(func(t *DashScopedTokenDbObject) bool { return t.TokenId == id || t.Prefix == id })
	if t == nil {
		return dashJson(404, stj.O("error", "no such token"))
	}
	if t.RevokedAt == 0 {
		t.RevokedAt = time.Now().UnixMilli()
		col.Update(t)
	}
	return dashJson(200, stj.O("success", true, "token", dashScopedView(t)))
}

// dashScopedTouch records last use (at most once a minute per token).
func dashScopedTouch(t *DashScopedTokenDbObject) {
	now := time.Now().UnixMilli()
	if now-t.LastUsed < 60000 {
		return
	}
	t.LastUsed = now
	if col := dashScopedTokens(); col != nil {
		col.Update(t)
	}
}
