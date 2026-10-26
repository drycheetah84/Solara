package mothership

import (
	"testing"
	"time"

	"solara/internal/serverid"
)

// Per-server tokens on the Mothership trusted-server checks (SEC-SID).
func TestMothershipServerTokens(t *testing.T) {
	s := newTestServer(t)
	serverid.SetDatabase(Database)
	t.Cleanup(func() { serverid.SetDatabase(nil) })
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "")

	good, _, err := serverid.Issue(serverid.KindServer, "depA", "gb1", serverid.TTLs{})
	if err != nil {
		t.Fatal(err)
	}
	revoked, rec, _ := serverid.Issue(serverid.KindServer, "depB", "gb1", serverid.TTLs{})
	if _, err := serverid.Revoke(rec.Prefix, "test"); err != nil {
		t.Fatal(err)
	}
	agent, _, _ := serverid.Issue(serverid.KindAgent, "", "gb1", serverid.TTLs{})
	// an expired token: issued "in the past" with a 1h idle window
	past := serverid.NewStore(Database)
	past.Now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expired, _, _ := past.Issue(serverid.KindServer, "depC", "", serverid.TTLs{Idle: time.Hour, Max: 24 * time.Hour})
	foreign := "sst1_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // well-formed, never issued

	path := "/v1/server/authorized?id=nobody"
	cases := []struct {
		name, key string
		want      int
	}{
		{"valid token", good, 200},
		{"revoked token", revoked, 403},
		{"expired token", expired, 403},
		{"foreign token", foreign, 403},
		{"agent token is not a server credential", agent, 403},
		{"static key (compat)", testServerKey, 200},
		{"no key", "", 403},
	}
	for _, c := range cases {
		var rec = do(t, s, "GET", path, "")
		if c.key != "" {
			rec = do(t, s, "GET", path, "", "x-server-api-key", c.key)
		}
		if rec.Code != c.want {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
	}

	// strict mode: the static key no longer authenticates a game server
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "1")
	if rec := do(t, s, "GET", path, "", "x-server-api-key", testServerKey); rec.Code != 403 {
		t.Errorf("strict static: %d", rec.Code)
	}
	if rec := do(t, s, "GET", path, "", "x-server-api-key", good); rec.Code != 200 {
		t.Errorf("strict token: %d", rec.Code)
	}
	// IsAdmin (server half): token yes, static key no (strict), revoked no
	if rec := do(t, s, "GET", "/v1/permissions/server", "", "x-server-api-key", good); rec.Code != 200 {
		t.Errorf("admin via token: %d", rec.Code)
	}
	if rec := do(t, s, "GET", "/v1/permissions/server", "", "x-server-api-key", testServerKey); rec.Code == 200 {
		t.Errorf("admin via static key in strict mode: %d", rec.Code)
	}
	if rec := do(t, s, "GET", "/v1/permissions/server", "", "x-server-api-key", revoked); rec.Code == 200 {
		t.Errorf("admin via revoked token: %d", rec.Code)
	}
	if rec := do(t, s, "GET", "/v1/permissions/server", "", "x-automation-key", testAutomationKey); rec.Code != 200 {
		t.Errorf("automation key: %d", rec.Code)
	}
}
