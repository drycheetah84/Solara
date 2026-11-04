package ares

import (
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

func TestNotify(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	notifyMu.Lock()
	notifyPlayers, notifyQueue = map[string]notifyReport{}, map[string][]string{}
	notifyMu.Unlock()
	st := newA2StationDbServer()
	dash := newAresDashboardServer()

	d := models.NewDeploymentDbObject()
	d.DeploymentId, d.StationId = dashStrPtr("dep-n"), dashStrPtr("st-n")
	now := dotnet.UtcNow()
	d.LastEvent = &now
	docdb.GetCollection(Database, true, models.NewDeploymentDbObject).Insert(d)

	if b := do(t, dash, "POST", "/api/stations/st-n/notify", `{"to":"all","message":"hi"}`, adminHdr).Body.String(); !strings.Contains(b, "no live game server") {
		t.Fatal(b)
	}
	// duplicate + blank lines are dropped; names are optional
	if b := a2Do(t, st, "POST", "/v1/deployments/dep-n/notify/players", "p1\tAlice\r\np2\tBob\n\np1\tDup\np3\n", nil).Body.String(); !strings.Contains(b, `"stored":3`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "GET", "/api/stations/st-n/notify", "", adminHdr).Body.String(); !strings.Contains(b, `"deployment_id":"dep-n"`) ||
		!strings.Contains(b, `{"id":"p1","name":"Alice"}`) || !strings.Contains(b, `{"id":"p3","name":""}`) {
		t.Fatal(b)
	}
	for body, want := range map[string]string{
		`{"to":"all","message":"   "}`:                              "message is empty",
		`{"to":[],"message":"hi"}`:                                  "pick at least one player",
		`{"to":["ghost"],"message":"hi"}`:                           "none of the chosen players",
		`{"to":"all","message":"` + strings.Repeat("x", 401) + `"}`: "too long",
	} {
		if b := do(t, dash, "POST", "/api/stations/st-n/notify", body, adminHdr).Body.String(); !strings.Contains(b, want) {
			t.Fatalf("%s -> %s (want %q)", body, b, want)
		}
	}
	// players that left are dropped from the target list; multi-line messages keep their newlines
	if b := do(t, dash, "POST", "/api/stations/st-n/notify", `{"to":["p2","ghost","p1"],"message":"Event in 5 min!\nHead to the atrium"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/stations/st-n/notify", `{"to":"all","message":"Server restart soon"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-n/notify/next", "", nil).Body.String(); b != "p2,p1\nEvent in 5 min!\nHead to the atrium" {
		t.Fatalf("next = %q", b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-n/notify/next", "", nil).Body.String(); b != "*\nServer restart soon" {
		t.Fatalf("next = %q", b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-n/notify/next", "", nil).Body.String(); b != "" {
		t.Fatalf("queue should be empty, got %q", b)
	}
	if w := do(t, dash, "POST", "/api/stations/st-n/notify", `{"to":"all","message":"x"}`, nil); w.Code == 200 && strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatal("sending must need the dashboard key")
	}
}
