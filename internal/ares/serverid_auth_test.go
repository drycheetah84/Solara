package ares

import (
	"solara/internal/ares/models"
	"solara/internal/docdb"
	"strings"
	"testing"

	"solara/internal/serverid"
)

// Per-server identity tokens on the station API (:78) -- SEC-SID.
func TestA2ServerTokenBinding(t *testing.T) {
	s, _ := a2TestEnv(t)
	resetDashAuth(t, nil)
	serverid.SetDatabase(Database)
	t.Cleanup(func() { serverid.SetDatabase(nil) })
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "")
	master := map[string]string{"x-api-key": a2TestMasterKey, "Content-Type": "application/json"}

	for _, id := range []string{"stA", "stB"} {
		if w := a2Do(t, s, "POST", "/stations/create", `{"station_name":"S","station_id":"`+id+`"}`, master); w.Code != 200 {
			t.Fatalf("create %s: %d %s", id, w.Code, w.Body.String())
		}
	}
	tokA, _, err := serverid.IssueSpec(serverid.Spec{Kind: serverid.KindServer, DeploymentId: "depA", StationId: "stA", Box: "gb1"})
	if err != nil {
		t.Fatal(err)
	}
	tokB, recB, _ := serverid.Issue(serverid.KindServer, "depB", "gb1", serverid.TTLs{})
	agentTok, _, _ := serverid.Issue(serverid.KindAgent, "", "gb1", serverid.TTLs{})
	asA := map[string]string{"x-server-api-key": tokA, "x-api-key": tokA, "Content-Type": "application/json"}
	asAapi := map[string]string{"x-api-key": tokA, "Content-Type": "application/json"}

	type tc struct {
		name, method, path, body string
		hdr                      map[string]string
		want                     int
	}
	run := func(cases []tc) {
		t.Helper()
		for _, c := range cases {
			w := a2Do(t, s, c.method, c.path, c.body, c.hdr)
			if w.Code != c.want {
				t.Errorf("%s: got %d (%s), want %d", c.name, w.Code, w.Body.String(), c.want)
			}
			if c.want == 403 && w.Body.String() != `{"error":"forbidden"}` {
				t.Errorf("%s: body %s", c.name, w.Body.String())
			}
			if c.want == 401 && w.Body.String() != `{"error":"unauthorized"}` {
				t.Errorf("%s: body %s", c.name, w.Body.String())
			}
		}
	}
	run([]tc{
		{"register own deployment", "POST", "/register_server", `{"ip":"1.2.3.4","port":"7777","deployment_id":"depA","server_name":"A","region":"eu"}`, asA, 200},
		{"register foreign deployment", "POST", "/register_server", `{"ip":"1.2.3.4","port":"7777","deployment_id":"depB"}`, asA, 403},
		{"player count own", "POST", "/update_player_count", `{"deployment_id":"depA","player_count":3}`, asA, 200},
		{"player count foreign", "POST", "/update_player_count", `{"deployment_id":"depB","player_count":3}`, asA, 403},
		{"player count foreign (case variant)", "POST", "/update_player_count", `{"deployment_id":"DEPB","player_count":3}`, asA, 403},
		{"heartbeat foreign", "POST", "/server_heartbeat", `{"deployment_id":"depB","pid":1}`, asAapi, 403},
		{"config fetch own", "GET", "/v1/deployments/depA/config", "", asAapi, 200},
		{"config fetch foreign", "GET", "/v1/deployments/depB/config", "", asAapi, 403},
		{"deployment fetch foreign", "GET", "/v1/deployments/depB?include_station_config=true", "", asAapi, 403},
		{"query deployment_id foreign", "GET", "/v1/stations?deployment_id=depB", "", asAapi, 403},
		{"station write own", "POST", "/stations/stA/config", `{"k":"v"}`, asAapi, 200},
		{"station write foreign", "POST", "/stations/stB/config", `{"k":"v"}`, asAapi, 403},
		{"admin route own station", "POST", "/stations/stA/users/u1/grant/Helper", "", asAapi, 200},
		{"admin route foreign station", "POST", "/stations/stB/users/u1/grant/Helper", "", asAapi, 403},
		{"agent token is no server credential", "POST", "/stations/stA/config", `{"k":"v"}`, map[string]string{"x-api-key": agentTok}, 401},
		{"maintenance refuses server tokens", "POST", "/v1/server/admin/purge_stations", "", asA, 200}, // 200 {"success":false}
		// compat mode: anonymous ingest and the static keys still work
		{"anon ingest (compat)", "POST", "/update_player_count", `{"deployment_id":"depA","player_count":1}`, nil, 200},
		{"static server key (compat)", "POST", "/stations/stB/config", `{"k":"v"}`, map[string]string{"x-server-api-key": "test-server-api-key"}, 200},
	})
	if w := a2Do(t, s, "POST", "/v1/server/admin/purge_stations", "", asA); w.Body.String() != `{"success":false}` {
		t.Fatalf("purge with a server token: %s", w.Body.String())
	}

	// a token whose deployment registered under a station inherits it
	run([]tc{
		{"register B", "POST", "/register_server", `{"ip":"1.2.3.5","port":"7777","deployment_id":"depB"}`, map[string]string{"x-api-key": tokB}, 200},
	})
	if d := a2FindDeployment("depB"); d == nil || d.StationId == nil {
		t.Fatal("depB not registered")
	} else {
		run([]tc{
			{"inherited station own", "POST", "/stations/" + *d.StationId + "/config", `{"k":"v"}`, map[string]string{"x-api-key": tokB}, 200},
			{"inherited station foreign", "POST", "/stations/stA/config", `{"k":"v"}`, map[string]string{"x-api-key": tokB}, 403},
		})
	}

	// LogInWithKey: the master-key response shape with the token's identity
	w := a2Do(t, s, "POST", "/users/log_in_with_key", "", map[string]string{"x-api-key": tokA})
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `"user_id":"halcyon-server:depA"`) || !strings.Contains(body, `"station_id":"global"`) || !strings.Contains(body, tokA) {
		t.Fatalf("token login: %d %s", w.Code, body)
	}
	wm := a2Do(t, s, "POST", "/users/log_in_with_key", "", map[string]string{"x-api-key": a2TestMasterKey})
	if strings.Replace(wm.Body.String(), a2TestMasterKey, tokA, 1) != strings.Replace(body, "halcyon-server:depA", "halcyon-server", 1) {
		t.Fatalf("shape differs from the master-key login:\n%s\n%s", wm.Body.String(), body)
	}
	if _, err := serverid.Revoke(recB.Prefix, "test"); err != nil {
		t.Fatal(err)
	}
	run([]tc{
		{"revoked token is anonymous on writes", "POST", "/stations/stA/config", `{"k":"v"}`, map[string]string{"x-api-key": tokB}, 401},
	})
	if w := a2Do(t, s, "POST", "/users/log_in_with_key", "", map[string]string{"x-api-key": tokB}); w.Body.String() == body || strings.Contains(w.Body.String(), "halcyon-server") {
		t.Fatalf("revoked token logged in: %s", w.Body.String())
	}

	// ── strict mode ──
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "1")
	run([]tc{
		{"strict: anon ingest", "POST", "/update_player_count", `{"deployment_id":"depA","player_count":1}`, nil, 401},
		{"strict: anon register", "POST", "/register_server", `{"deployment_id":"depZ"}`, nil, 401},
		{"strict: static server key", "POST", "/stations/stB/config", `{"k":"v"}`, map[string]string{"x-server-api-key": "test-server-api-key"}, 401},
		{"strict: static master key", "POST", "/users/u1/api_key", "", map[string]string{"x-api-key": a2TestMasterKey}, 401},
		{"strict: token ingest", "POST", "/update_player_count", `{"deployment_id":"depA","player_count":1}`, asA, 200},
		{"strict: dashboard admin key", "POST", "/users/u1/api_key", "", map[string]string{"x-dashboard-key": testDashAdminKey}, 200},
	})
	if w := a2Do(t, s, "POST", "/users/log_in_with_key", "", map[string]string{"x-api-key": a2TestMasterKey}); strings.Contains(w.Body.String(), "halcyon-server") {
		t.Fatalf("strict: master key login: %s", w.Body.String())
	}
	if w := a2Do(t, s, "POST", "/v1/server/admin/prune_sessions", "", map[string]string{"x-api-key": a2TestMasterKey}); w.Body.String() != `{"success":false}` {
		t.Fatalf("strict: prune with master key: %s", w.Body.String())
	}
	if w := a2Do(t, s, "POST", "/v1/server/admin/prune_sessions", "", map[string]string{"x-dashboard-key": testDashAdminKey}); w.Body.String() != `{"success":true}` {
		t.Fatalf("strict: prune with dashboard key: %s", w.Body.String())
	}
}

func TestDashboardServerTokenApi(t *testing.T) {
	withTempDB(t)
	resetDashAuth(t, nil)
	serverid.SetDatabase(Database)
	t.Cleanup(func() { serverid.SetDatabase(nil) })
	s := newAresDashboardServer()
	tok, rec, _ := serverid.Issue(serverid.KindServer, "depA", "gb1", serverid.TTLs{})

	if w := do(t, s, "GET", "/api/server-tokens", "", nil); w.Code != 401 {
		t.Fatalf("anon list: %d", w.Code)
	}
	w := do(t, s, "GET", "/api/server-tokens", "", adminHdr)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"prefix":"`+rec.Prefix+`"`) || strings.Contains(w.Body.String(), tok[11:]) || strings.Contains(w.Body.String(), rec.TokenHash) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "DELETE", "/api/server-tokens/nope", "", adminHdr); w.Code != 404 {
		t.Fatalf("revoke unknown: %d", w.Code)
	}
	if w := do(t, s, "DELETE", "/api/server-tokens/"+rec.IdHex(), "", adminHdr); w.Code != 200 || !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if _, ok := serverid.Validate(tok); ok {
		t.Fatal("token still valid after dashboard revoke")
	}
}

// SEC-SID-3: a per-server token is narrower than the shared key it replaces —
// admin actions only on its own station; station-less admin routes never.
func TestA2ServerTokenAdminScope(t *testing.T) {
	s, _ := a2TestEnv(t)
	resetDashAuth(t, nil)
	serverid.SetDatabase(Database)
	t.Cleanup(func() { serverid.SetDatabase(nil) })
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "")
	master := map[string]string{"x-api-key": a2TestMasterKey, "Content-Type": "application/json"}
	for _, id := range []string{"stA", "stB"} {
		a2Do(t, s, "POST", "/stations/create", `{"station_name":"S","station_id":"`+id+`"}`, master)
	}
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	u := models.NewUserDataResponse()
	u.UserId, u.Username, u.Roles = a2Str("victim"), a2Str("v"), []*string{}
	users.Insert(u)
	tok, _, err := serverid.IssueSpec(serverid.Spec{Kind: serverid.KindServer, DeploymentId: "depA", StationId: "stA", Box: "gb1"})
	if err != nil {
		t.Fatal(err)
	}
	asTok := map[string]string{"x-api-key": tok, "Content-Type": "application/json"}
	cases := []struct {
		name, method, path string
		want               int
	}{
		{"mint user key (station-less)", "POST", "/users/victim/api_key", 401},
		{"delete user (station-less)", "DELETE", "/users/victim", 401},
		{"create/update user (station-less)", "POST", "/users", 401},
		{"grant role on own station", "POST", "/stations/stA/users/victim/grant/Helper", 200},
	}
	for _, c := range cases {
		if w := a2Do(t, s, c.method, c.path, `{}`, asTok); w.Code != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, w.Code, w.Body.String(), c.want)
		}
	}
	// another station: refused (binding 403 or admin 401 — either way not 200)
	if w := a2Do(t, s, "POST", "/stations/stB/users/victim/grant/Helper", `{}`, asTok); w.Code == 200 {
		t.Errorf("token acted on a foreign station: %d", w.Code)
	}
}
