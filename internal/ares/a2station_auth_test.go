package ares

import (
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

// Security hardening of the station API (KNOWN_ISSUES SEC-F2).
func TestA2StationAuthorization(t *testing.T) {
	s, _ := a2TestEnv(t)
	master := map[string]string{"x-api-key": a2TestMasterKey}
	server := map[string]string{"x-server-api-key": "test-server-api-key"}

	// fixtures: two stations, a victim, a plain player, and an Admin of station "st1"
	for _, id := range []string{"st1", "st2"} {
		if w := a2Do(t, s, "POST", "/stations/create", `{"station_name":"S","station_id":"`+id+`"}`, master); w.Code != 200 {
			t.Fatalf("create %s: %d %s", id, w.Code, w.Body.String())
		}
	}
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	for _, id := range []string{"victim", "player", "stadmin"} {
		u := models.NewUserDataResponse()
		u.UserId, u.Username, u.Roles = a2Str(id), a2Str(id), []*string{}
		users.Insert(u)
	}
	if w := a2Do(t, s, "POST", "/stations/st1/users/stadmin/grant/Admin", "", master); w.Code != 200 {
		t.Fatalf("grant: %d %s", w.Code, w.Body.String())
	}
	playerKey := map[string]string{"x-api-key": a2MintUserKey(a2Str("player"))}
	adminKey := map[string]string{"x-api-key": a2MintUserKey(a2Str("stadmin"))}
	bogus := map[string]string{"x-api-key": "not-a-key"}

	cases := []struct {
		name, method, path string
		hdr                map[string]string
		want               int
	}{
		// the reported takeover: mint a key for any user without credentials
		{"anon mint key", "POST", "/users/victim/api_key", nil, 401},
		{"bogus key mint", "POST", "/users/victim/api_key", bogus, 401},
		{"player mint other key", "POST", "/users/victim/api_key", playerKey, 401},
		{"station admin mint key (needs global)", "POST", "/users/victim/api_key", adminKey, 401},
		{"master mint key", "POST", "/users/victim/api_key", master, 200},
		{"server key mint key", "POST", "/users/victim/api_key", server, 200},
		{"anon delete user", "DELETE", "/users/victim", nil, 401},
		{"anon grant role", "POST", "/stations/st1/users/victim/grant/Admin", nil, 401},
		{"player grant role", "POST", "/stations/st1/users/victim/grant/Admin", playerKey, 401},
		{"station admin grants on own station", "POST", "/stations/st1/users/victim/grant/Helper", adminKey, 200},
		{"station admin grants on other station", "POST", "/stations/st2/users/victim/grant/Helper", adminKey, 401},
		{"anon station config write", "POST", "/stations/st1/config", nil, 401},
		{"player station config write", "POST", "/stations/st1/config", playerKey, 200},
		{"anon delete station", "DELETE", "/stations/st2", nil, 401},
		// still open: logins, reads, game-server ingest, public spec list
		{"anon register_server stays open", "POST", "/register_server", nil, 200},
		{"anon read stations", "GET", "/v1/stations", nil, 200},
		{"anon spec level list", "GET", "/v1/spec/levels", nil, 200},
	}
	for _, tc := range cases {
		body := ""
		if tc.method == "POST" {
			body = `{"deployment_id":"d-auth","k":"v"}`
		}
		hdr := map[string]string{"Content-Type": "application/json"}
		for k, v := range tc.hdr {
			hdr[k] = v
		}
		w := a2Do(t, s, tc.method, tc.path, body, hdr)
		if w.Code != tc.want {
			t.Errorf("%s: got %d (%s), want %d", tc.name, w.Code, w.Body.String(), tc.want)
		}
		if tc.want == 401 && w.Body.String() != `{"error":"unauthorized"}` {
			t.Errorf("%s: body %s", tc.name, w.Body.String())
		}
	}
	// a deployment key (issued by CreateDeployment) is a writer credential
	w := a2Do(t, s, "POST", "/stations/st1/deployments", `{}`, map[string]string{"x-api-key": a2TestMasterKey, "Content-Type": "application/json"})
	depKey := a2Json(t, w).GetProperty("api_key").Str()
	if depKey == "" {
		t.Fatalf("no deployment key: %s", w.Body.String())
	}
	if w := a2Do(t, s, "POST", "/stations/st1/config", `{"k":"v"}`, map[string]string{"x-api-key": depKey}); w.Code != 200 {
		t.Errorf("deployment key writer: %d", w.Code)
	}
}
