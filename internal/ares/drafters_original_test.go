package ares

// Port of the original test project backend/tests/Drafters/Program.cs
// ("PASS: 24 Drafter authorization checks" against the .NET build). The C#
// harness invoked the handler methods directly, bypassing the dashboard auth
// preprocessor; here the same handlers are reached through their real routes
// with preprocessor enforcement disabled, so each check exercises the
// handler's own authorization exactly as the original did.

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

func TestOriginalDraftersSuite(t *testing.T) {
	t.Setenv("HALCYON_SERVER_API_KEY", "test-server-key")
	a2SpecKeyOnce = sync.Once{}
	t.Cleanup(func() { a2SpecKeyOnce = sync.Once{} })
	off := "0"
	resetDashAuth(t, &off) // admin key = testDashAdminKey; enforcement off
	withTempDB(t)

	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	for _, id := range []string{"alpha", "beta"} {
		st := models.NewStationDbObject()
		st.StationId, st.StationName = ptr(id), ptr(id)
		stations.Insert(st)
	}
	deps := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)
	for _, p := range [][2]string{{"dep-alpha", "alpha"}, {"dep-beta", "beta"}} {
		d := models.NewDeploymentDbObject()
		d.DeploymentId, d.StationId = ptr(p[0]), ptr(p[1])
		deps.Insert(d)
	}
	u := models.NewUserDataResponse()
	u.UserId, u.Username = ptr("user1"), ptr("Drafter")
	docdb.GetCollection(Database, true, models.NewUserDataResponse).Insert(u)

	dash := newAresDashboardServer()
	server := newA2StationDbServer()
	admin := map[string]string{"x-dashboard-key": testDashAdminKey}
	game := map[string]string{"x-server-api-key": "test-server-key"}
	anon := map[string]string{}

	count := 0
	check := func(ok bool, label string) {
		t.Helper()
		if !ok {
			t.Fatalf("check failed: %s", label)
		}
		count++
	}
	callD := func(method, path string, h map[string]string) (int, string) {
		w := do(t, dash, method, path, "", h)
		return w.Code, w.Body.String()
	}
	callS := func(method, path string, h map[string]string) (int, string) {
		w := do(t, &AresDashboardServer{HTTP: server.HTTP}, method, path, "", h)
		return w.Code, w.Body.String()
	}
	grant := func(scope, user string, h map[string]string) int {
		c, _ := callD("POST", "/api/drafters/"+scope+"/"+user, h)
		return c
	}
	revoke := func(scope, user string, h map[string]string) int {
		c, _ := callD("DELETE", "/api/drafters/"+scope+"/"+user, h)
		return c
	}
	feed := func(dep string, h map[string]string) (int, string) { return callS("GET", "/v1/spec/drafters/"+dep, h) }

	check(grant("alpha", "user1", anon) == http.StatusForbidden, "anonymous grant denied")
	check(grant("alpha", "user1", game) == http.StatusForbidden, "server cannot grant")
	c, _ := callD("GET", "/api/drafters/alpha", anon)
	check(c == http.StatusForbidden, "anonymous list denied")
	c, _ = feed("dep-alpha", anon)
	check(c == http.StatusOK, "public server feed allowed")
	check(grant("missing", "user1", admin) == http.StatusNotFound, "unknown station denied")
	check(grant("alpha", "missing", admin) == http.StatusBadRequest, "unknown user denied")
	check(grant("alpha", "user1", admin) == http.StatusOK, "station grant")
	grant("alpha", "user1", admin)
	check(len(DrafterPermissionsList()) == 1, "idempotent grant")
	_, body := feed("dep-alpha", game)
	check(strings.Contains(body, "|user1\n"), "station access")
	_, body = feed("dep-alpha", anon)
	check(strings.Contains(body, "|user1\n"), "public feed contains effective grants")
	_, body = feed("dep-beta", game)
	check(!strings.Contains(body, "user1"), "station isolation")
	c, _ = feed("missing", game)
	check(c == http.StatusNotFound, "unregistered deployment denied")
	grant("global", "user1", admin)
	_, body = feed("dep-beta", game)
	check(strings.Contains(body, "user1"), "global access")
	revoke("alpha", "user1", admin)
	_, body = feed("dep-alpha", game)
	check(strings.Contains(body, "user1"), "global survives station revoke")
	_, body = callD("GET", "/api/drafters/alpha", admin)
	check(strings.Contains(body, "global"), "inherited grant listed")
	revoke("global", "user1", admin)
	_, body = feed("dep-alpha", game)
	check(!strings.Contains(body, "user1"), "revocation")
	check(!DrafterPermissionsValidId("user\ninjected"), "line injection denied")
	check(!DrafterPermissionsApplies("global", ""), "missing station denied")
	check(revoke("global", "user1", anon) == http.StatusForbidden, "anonymous revoke denied")
	c, _ = callS("GET", "/v1/spec/levels", anon)
	check(c == http.StatusOK, "public level list")
	c, _ = callS("GET", "/v1/spec/levels", map[string]string{"x-server-api-key": "obsolete"})
	check(c == http.StatusOK, "obsolete credential does not block public list")
	c, _ = callS("PUT", "/v1/spec/levels/!", anon)
	check(c == http.StatusBadRequest, "public upload reaches name validation")
	c, _ = callS("POST", "/v1/spec/levels/!/loaded", anon)
	check(c == http.StatusBadRequest, "public load reaches name validation")
	c, _ = callS("GET", "/v1/spec/levels/!", anon)
	check(c == http.StatusBadRequest, "public download reaches name validation")

	if count != 24 {
		t.Fatalf("ran %d checks, original suite has 24", count)
	}
	t.Logf("PASS: %d Drafter authorization checks", count)
}

func ptr(s string) *string { return &s }
