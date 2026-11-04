package ares

import (
	"encoding/json"
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

// Every dashboard /api route is either in the scoped-token table or deliberately master-only.
func TestScopedTokenRouteCoverage(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	masterOnly := map[string]bool{
		"POST /api/users/{user_id}/grant_admin": true, "DELETE /api/users/{user_id}/grant_admin": true,
		"GET /api/server-tokens": true, "DELETE /api/server-tokens/{id}": true,
		"GET /api/scoped-tokens/permissions": true, "GET /api/scoped-tokens/self": true, "GET /api/scoped-tokens": true,
		"POST /api/scoped-tokens": true, "DELETE /api/scoped-tokens/{id}": true,
		"GET /api/titledata": true, "PUT /api/titledata/{key}": true, "DELETE /api/titledata/{key}": true,
		"GET /api/anonymous-names": true, "PUT /api/anonymous-names": true, "DELETE /api/anonymous-names": true,
		"POST /api/anonymous-names/preview": true,
		"GET /api/users/{user_id}/global_permissions": true, "PUT /api/users/{user_id}/global_permissions": true,
	}
	inTable := map[string]bool{}
	for _, r := range dashScopedRoutes {
		inTable[r.Method+" "+r.Pattern] = true
		for _, p := range strings.Split(r.Perm, "|") {
			if !dashKnownPerm(p) {
				t.Fatalf("route %s %s uses unknown permission %s", r.Method, r.Pattern, p)
			}
		}
	}
	for _, e := range s.HTTP.Endpoints() {
		k := e.Method + " " + e.RouteUri
		if !strings.HasPrefix(e.RouteUri, "/api/") {
			continue
		}
		if !inTable[k] && !masterOnly[k] {
			t.Errorf("dashboard route %s has no scoped-token permission and is not listed as master-only", k)
		}
		delete(inTable, k)
	}
	for k := range inTable {
		t.Errorf("scoped-token table names %s, which is not a dashboard route", k)
	}
}

func TestScopedTokens(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	for _, id := range []string{"stA", "stB"} {
		st := models.NewStationDbObject()
		st.StationId, st.StationName = dashStrPtr(id), dashStrPtr("Station "+id)
		stations.Insert(st)
	}
	create := func(hdr map[string]string, body string) (int, map[string]any) {
		w := do(t, s, "POST", "/api/scoped-tokens", body, hdr)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	// Only the master key may mint.
	if code, _ := create(nil, `{"permissions":["stations.read"],"stations":["stA"]}`); code != 401 {
		t.Fatalf("no credential: %d", code)
	}
	if code, out := create(adminHdr, `{"permissions":["nope"],"stations":["stA"]}`); code != 400 {
		t.Fatalf("unknown perm accepted: %d %v", code, out)
	}
	if code, out := create(adminHdr, `{"permissions":["stations.read"],"stations":["ghost"]}`); code != 400 {
		t.Fatalf("unknown station accepted: %d %v", code, out)
	}
	code, out := create(adminHdr, `{"name":"stA mod","permissions":["stations.read","drafters.read"],"fleets":["stA"]}`)
	tok, _ := out["token"].(string)
	if code, o := create(adminHdr, `{"name":"short","permissions":["stations.read"],"stations":["stA"],"expires_in_minutes":30}`); code != 200 || o["expires_at"] == nil {
		t.Fatalf("expires_in_minutes: %d %v", code, o)
	}
	if code != 200 || !strings.HasPrefix(tok, "sdt1_") {
		t.Fatalf("create: %d %v", code, out)
	}
	th := map[string]string{"x-dashboard-key": tok}
	// A scoped token cannot mint or list tokens.
	if code, _ := create(th, `{"permissions":["stations.read"],"stations":["*"]}`); code != 403 {
		t.Fatalf("scoped token minted a token: %d", code)
	}
	if w := do(t, s, "GET", "/api/scoped-tokens", "", th); w.Code != 403 {
		t.Fatalf("scoped token listed tokens: %d", w.Code)
	}
	check := func(method, path string, want int) {
		t.Helper()
		w := do(t, s, method, path, "", th)
		if (want == 200 && (w.Code == 401 || w.Code == 403)) || (want != 200 && w.Code != want) {
			t.Fatalf("%s %s: %d %s (want %d)", method, path, w.Code, w.Body.String(), want)
		}
	}
	check("GET", "/api/stations/stA/config", 200)  // in scope, has permission
	check("GET", "/api/stations/stB/config", 403)  // other station
	check("POST", "/api/stations/stA/spinup", 403) // missing stations.manage
	check("GET", "/api/stats", 403)                // station-less route needs "*"
	check("GET", "/api/server-tokens", 403)        // master-only
	check("GET", "/api/drafters/stA", 200)         // drafters.read on its own station
	check("GET", "/api/drafters/global", 403)
	if w := do(t, s, "GET", "/api/scoped-tokens/self", "", th); w.Code != 200 || !strings.Contains(w.Body.String(), `"master":false`) ||
		!strings.Contains(w.Body.String(), `"stations":["stA"]`) {
		t.Fatalf("self: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/scoped-tokens/self", "", adminHdr); !strings.Contains(w.Body.String(), `"master":true`) {
		t.Fatalf("self (master): %s", w.Body.String())
	} // global drafters need "*"
	// GET /api/stations lists only the token's stations.
	w := do(t, s, "GET", "/api/stations", "", th)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"stA"`) || strings.Contains(w.Body.String(), `"stB"`) {
		t.Fatalf("station list: %d %s", w.Code, w.Body.String())
	}
	// The master key still sees everything.
	w = do(t, s, "GET", "/api/stations", "", adminHdr)
	if !strings.Contains(w.Body.String(), `"stB"`) {
		t.Fatalf("master station list: %s", w.Body.String())
	}
	// Revoked -> the credential no longer works at all.
	id, _ := out["id"].(string)
	if w := do(t, s, "DELETE", "/api/scoped-tokens/"+id, "", adminHdr); w.Code != 200 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/stations/stA/config", "", th); w.Code != 401 {
		t.Fatalf("revoked token still works: %d", w.Code)
	}
	// The stored record never holds the secret.
	for _, r := range dashScopedTokens().FindAll() {
		if strings.Contains(r.Hash, tok) || r.Prefix == tok {
			t.Fatal("secret stored")
		}
	}
}

// fleets.view sees the fleet list (names, player counts) and nothing else.
func TestScopedTokenFleetsView(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	st := models.NewStationDbObject()
	st.StationId, st.StationName = dashStrPtr("stV"), dashStrPtr("Viewable")
	docdb.GetCollection(Database, true, models.NewStationDbObject).Insert(st)
	w := do(t, s, "POST", "/api/scoped-tokens", `{"name":"viewer","permissions":["fleets.view"],"stations":["*"]}`, adminHdr)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	th := map[string]string{"x-dashboard-key": out["token"].(string)}
	if w := do(t, s, "GET", "/api/stations", "", th); w.Code != 200 || !strings.Contains(w.Body.String(), "Viewable") {
		t.Fatalf("fleet list: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/deployments", "", th); w.Code != 200 {
		t.Fatalf("deployments: %d", w.Code)
	}
	for _, p := range []string{"/api/stations/stV/config", "/api/stations/stV/members", "/api/users", "/api/stats"} {
		if w := do(t, s, "GET", p, "", th); w.Code != 403 {
			t.Fatalf("%s should be refused: %d", p, w.Code)
		}
	}
}

// Per-permission station scoping: "view every fleet" + "configure fleet A" must not configure fleet B.
func TestScopedTokenPerPermissionStations(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	for _, id := range []string{"stA", "stB"} {
		st := models.NewStationDbObject()
		st.StationId, st.StationName = dashStrPtr(id), dashStrPtr("Station "+id)
		st.Config = map[string]*string{}
		docdb.GetCollection(Database, true, models.NewStationDbObject).Insert(st)
	}
	w := do(t, s, "POST", "/api/scoped-tokens", `{"name":"captain+member","permissions":["fleets.view@*","stations.read@stA","stations.config@stA"]}`, adminHdr)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	tok, _ := out["token"].(string)
	if tok == "" {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	th := map[string]string{"x-dashboard-key": tok}
	if w := do(t, s, "GET", "/api/stations", "", th); w.Code != 200 || !strings.Contains(w.Body.String(), "stB") {
		t.Fatalf("fleet list should show every fleet: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "PATCH", "/api/stations/stA/config", `{"k":"v"}`, th); w.Code != 200 {
		t.Fatalf("own fleet config: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "PATCH", "/api/stations/stB/config", `{"k":"v"}`, th); w.Code != 403 {
		t.Fatalf("other fleet config must be refused: %d", w.Code)
	}
	if w := do(t, s, "GET", "/api/stations/stB/config", "", th); w.Code != 403 {
		t.Fatalf("other fleet read must be refused: %d", w.Code)
	}
}

// stations.manage (captains) must not delete a station; only stations.delete (or the master key) can.
func TestScopedTokenManageCannotDelete(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	st := models.NewStationDbObject()
	st.StationId, st.StationName = dashStrPtr("stC"), dashStrPtr("Captained")
	docdb.GetCollection(Database, true, models.NewStationDbObject).Insert(st)
	w := do(t, s, "POST", "/api/scoped-tokens", `{"name":"captain","permissions":["stations.manage@stC","stations.read@stC"]}`, adminHdr)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	th := map[string]string{"x-dashboard-key": out["token"].(string)}
	if w := do(t, s, "DELETE", "/api/stations/stC", "", th); w.Code != 403 {
		t.Fatalf("captain delete must be refused: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/stations/stC/config", "", th); w.Code == 403 {
		t.Fatalf("captain read refused")
	}
}

// A captain (station-scoped perms only) can load the board slots, the player picker and (with spec.read@*) levels.
func TestScopedTokenCaptainTabs(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	st := models.NewStationDbObject()
	st.StationId, st.StationName = dashStrPtr("stC"), dashStrPtr("Captained")
	docdb.GetCollection(Database, true, models.NewStationDbObject).Insert(st)
	w := do(t, s, "POST", "/api/scoped-tokens", `{"name":"captain","permissions":["stations.read@stC","stations.roles@stC","spec.read@*"]}`, adminHdr)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	th := map[string]string{"x-dashboard-key": out["token"].(string)}
	for _, p := range []string{"/api/board/keys", "/api/users", "/api/stations/stC/members", "/api/stations/stC/board", "/api/spec/levels"} {
		if w := do(t, s, "GET", p, "", th); w.Code == 403 {
			t.Fatalf("%s refused: %s", p, w.Body.String())
		}
	}
	if w := do(t, s, "GET", "/api/stats", "", th); w.Code != 403 {
		t.Fatalf("stats should stay refused: %d", w.Code)
	}
}
