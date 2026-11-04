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
		!strings.Contains(b, `{"id":"p1","name":"Alice","server":""}`) || !strings.Contains(b, `{"id":"p3","name":"","server":""}`) {
		t.Fatal(b)
	}
	for body, want := range map[string]string{
		`{"to":"all","message":"   "}`:                              "message is empty",
		`{"to":[],"message":"hi"}`:                                  "pick at least one player",
		`{"to":["ghost"],"message":"hi"}`:                           "none of the chosen players are on a server",
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

// Fleet-wide: the list covers every live server (tagged with its name), "everyone" reaches every live server, and a
// chosen player is sent to whichever server they are on -- even another station's.
func TestNotifyFleetWide(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	notifyMu.Lock()
	notifyPlayers, notifyQueue = map[string]notifyReport{}, map[string][]string{}
	notifyMu.Unlock()
	st := newA2StationDbServer()
	dash := newAresDashboardServer()
	now := dotnet.UtcNow()
	gone := now.AddMinutes(-10)
	for _, r := range []struct {
		id, name, station string
		at                *dotnet.DateTime
	}{{"dep-h1", "Halcyon", "st-h", &now}, {"dep-h2", "Halcyon 2", "st-h", &now}, {"dep-club", "Clubhouse", "st-c", &now}, {"dep-dead", "Old", "st-c", &gone}} {
		d := models.NewDeploymentDbObject()
		d.DeploymentId, d.StationId, d.LastEvent, d.DeploymentName = dashStrPtr(r.id), dashStrPtr(r.station), r.at, dashStrPtr(r.name)
		docdb.GetCollection(Database, true, models.NewDeploymentDbObject).Insert(d)
	}
	a2Do(t, st, "POST", "/v1/deployments/dep-h1/notify/players", "a\tAlice\n", nil)
	a2Do(t, st, "POST", "/v1/deployments/dep-h2/notify/players", "b\tBob\n", nil)
	a2Do(t, st, "POST", "/v1/deployments/dep-club/notify/players", "c\tCara\n", nil)
	a2Do(t, st, "POST", "/v1/deployments/dep-dead/notify/players", "z\tZed\n", nil)
	b := do(t, dash, "GET", "/api/stations/st-h/notify", "", adminHdr).Body.String()
	for _, w := range []string{`{"id":"a","name":"Alice","server":"Halcyon"}`, `{"id":"b","name":"Bob","server":"Halcyon 2"}`, `{"id":"c","name":"Cara","server":"Clubhouse"}`} {
		if !strings.Contains(b, w) {
			t.Fatalf("missing %s in %s", w, b)
		}
	}
	if strings.Contains(b, "Zed") {
		t.Fatalf("dead server listed: %s", b)
	}
	if b := do(t, dash, "POST", "/api/stations/st-h/notify", `{"to":["b","c"],"message":"hey"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/stations/st-h/notify", `{"to":"all","message":"restart"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	want := map[string]string{
		"dep-h1":   "*\nrestart|",
		"dep-h2":   "b\nhey|*\nrestart|",
		"dep-club": "c\nhey|*\nrestart|",
		"dep-dead": "",
	}
	for dep, w := range want {
		got := ""
		for {
			n := a2Do(t, st, "GET", "/v1/deployments/"+dep+"/notify/next", "", nil).Body.String()
			if n == "" {
				break
			}
			got += n + "|"
		}
		if got != w {
			t.Fatalf("%s got %q, want %q", dep, got, w)
		}
	}
}
