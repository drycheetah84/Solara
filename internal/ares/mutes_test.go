package ares

import (
	"strings"
	"testing"

	"solara/internal/mothership"
)

func TestMutes(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	old := mothership.Database
	mothership.Database = Database
	t.Cleanup(func() { mothership.Database = old })
	st := newA2StationDbServer()
	dash := newAresDashboardServer()

	if b := a2Do(t, st, "GET", "/v1/deployments/dep-m/mutes", "", nil).Body.String(); b != "" {
		t.Fatalf("no mutes yet: %q", b)
	}
	for body, want := range map[string]string{
		`{"duration_minutes":5}`:                                         "player_id required",
		`{"player_id":"p1","duration_minutes":-1}`:                       "duration must be",
		`{"player_id":"p1","duration_minutes":1.5}`:                      "whole number",
		`{"player_id":"p1","reason":"` + strings.Repeat("x", 121) + `"}`: "too long",
		`{"player_id":"bad\tid"}`:                                        "player_id required",
	} {
		if b := do(t, dash, "POST", "/api/mutes", body, adminHdr).Body.String(); !strings.Contains(b, want) {
			t.Fatalf("%s -> %s (want %q)", body, b, want)
		}
	}
	if b := do(t, dash, "POST", "/api/mutes", `{"player_id":"p1","duration_minutes":10,"reason":"slurs\tin\nvoice"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/mutes", `{"player_id":"p2"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	// re-muting replaces the old mute instead of stacking
	if b := do(t, dash, "POST", "/api/mutes", `{"player_id":"p1","duration_minutes":10,"reason":"slurs in voice"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	b := a2Do(t, st, "GET", "/v1/deployments/dep-m/mutes", "", nil).Body.String()
	lines := strings.Split(strings.TrimRight(b, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines: %q", b)
	}
	got := map[string][]string{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			t.Fatalf("bad line %q", l)
		}
		got[f[0]] = f
	}
	if p1 := got["p1"]; p1 == nil || p1[1] == "0" || p1[2] != "slurs in voice" {
		t.Fatalf("p1 = %q", p1)
	}
	if p2 := got["p2"]; p2 == nil || p2[1] != "0" || p2[2] != "" {
		t.Fatalf("p2 (permanent) = %q", p2)
	}
	if b := do(t, dash, "GET", "/api/mutes", "", adminHdr).Body.String(); !strings.Contains(b, `"player_id":"p1"`) || !strings.Contains(b, `"expires_at":null`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/mutes/p1/unmute", "", adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, dash, "POST", "/api/mutes/p1/unmute", "", adminHdr).Body.String(); !strings.Contains(b, "not muted") {
		t.Fatal(b)
	}
	if b := a2Do(t, st, "GET", "/v1/deployments/dep-m/mutes", "", nil).Body.String(); b != "p2\t0\t\n" {
		t.Fatalf("after unmute: %q", b)
	}
	// dashboard routes need a credential
	if rec := do(t, dash, "GET", "/api/mutes", "", nil); rec.Code != 401 {
		t.Fatalf("no credential -> %d", rec.Code)
	}
}
