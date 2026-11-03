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
		"GET /api/scoped-tokens/permissions": true, "GET /api/scoped-tokens": true,
		"POST /api/scoped-tokens": true, "DELETE /api/scoped-tokens/{id}": true,
	}
	inTable := map[string]bool{}
	for _, r := range dashScopedRoutes {
		inTable[r.Method+" "+r.Pattern] = true
		if !dashKnownPerm(r.Perm) {
			t.Fatalf("route %s %s uses unknown permission %s", r.Method, r.Pattern, r.Perm)
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
	check("GET", "/api/drafters/global", 403)      // global drafters need "*"
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
