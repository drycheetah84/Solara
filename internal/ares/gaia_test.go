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
	gaiaEvents, gaiaQueue = map[string][]string{}, map[string][]string{}
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
