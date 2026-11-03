package ares

import (
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

func TestGaia(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	gaiaMu.Lock()
	gaiaEvents, gaiaQueue, gaiaLoaded = map[string][]string{}, map[string][]string{}, false
	gaiaMu.Unlock()
	st := newA2StationDbServer()
	dash := newAresDashboardServer()

	d := models.NewDeploymentDbObject()
	d.DeploymentId, d.StationId = dashStrPtr("dep-g"), dashStrPtr("st-g")
	now := dotnet.UtcNow()
	d.LastEvent = &now
	docdb.GetCollection(Database, true, models.NewDeploymentDbObject).Insert(d)

	if b := do(t, dash, "POST", "/api/stations/st-g/gaia/play", `{"event":"Play_X"}`, adminHdr).Body.String(); !strings.Contains(b, "no live game server") {
		t.Fatal(b)
	}
	if b := a2Do(t, st, "POST", "/v1/deployments/dep-g/gaia/events", "Play_Music_A\nPlay_GAIA_Welcome\n\nPlay_Music_A\n", nil).Body.String(); !strings.Contains(b, `"stored":2`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "GET", "/api/stations/st-g/gaia", "", adminHdr).Body.String(); !strings.Contains(b, `"deployment_id":"dep-g"`) || !strings.Contains(b, `"events":["Play_GAIA_Welcome","Play_Music_A"]`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/stations/st-g/gaia/play", `{"event":"Nope"}`, adminHdr).Body.String(); !strings.Contains(b, "unknown event") {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/stations/st-g/gaia/play", `{"event":"Play_Music_A"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-g/gaia/next", "", nil).Body.String(); b != "Play_Music_A" {
		t.Fatalf("next = %q", b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-g/gaia/next", "", nil).Body.String(); b != "" {
		t.Fatalf("queue should be empty, got %q", b)
	}
	if w := do(t, dash, "POST", "/api/stations/st-g/gaia/play", `{"event":"Play_Music_A"}`, nil); w.Code == 200 && strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatal("play must need the dashboard key")
	}
}

// The reported list survives a backend restart, a newer deployment replaces the station's old list, and "!report"
// only goes to a poller that asks for it (an old DLL would play it as an event and spin).
func TestGaiaPersistsAcrossRestart(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	reset := func() {
		gaiaMu.Lock()
		gaiaEvents, gaiaQueue, gaiaLoaded = map[string][]string{}, map[string][]string{}, false
		gaiaMu.Unlock()
	}
	reset()
	st := newA2StationDbServer()
	dash := newAresDashboardServer()
	now := dotnet.UtcNow()
	for _, id := range []string{"dep-old", "dep-new"} {
		d := models.NewDeploymentDbObject()
		d.DeploymentId, d.StationId, d.LastEvent = dashStrPtr(id), dashStrPtr("st-p"), &now
		docdb.GetCollection(Database, true, models.NewDeploymentDbObject).Insert(d)
	}
	a2Do(t, st, "POST", "/v1/deployments/dep-old/gaia/events", "Play_Old\n", nil)
	a2Do(t, st, "POST", "/v1/deployments/dep-new/gaia/events", "Play_GAIA_Welcome\nPlay_Music_A\n", nil)
	reset() // backend restart: memory gone
	if b := do(t, dash, "GET", "/api/stations/st-p/gaia", "", adminHdr).Body.String(); !strings.Contains(b, `"deployment_id":"dep-new"`) ||
		!strings.Contains(b, `"events":["Play_GAIA_Welcome","Play_Music_A"]`) {
		t.Fatalf("after restart: %s", b)
	}
	if n := len(gaiaStore().FindAll()); n != 1 {
		t.Fatalf("stored lists: %d (the old deployment's should be pruned)", n)
	}
	// A deployment with no list: a plain poll gets nothing; a poll with ?report=1 is asked to report.
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-unknown/gaia/next", "", nil).Body.String(); b != "" {
		t.Fatalf("plain poll got %q", b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-unknown/gaia/next?report=1", "", nil).Body.String(); b != "!report" {
		t.Fatalf("report poll got %q", b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-new/gaia/next?report=1", "", nil).Body.String(); b != "" {
		t.Fatalf("known deployment asked to re-report: %q", b)
	}
}
