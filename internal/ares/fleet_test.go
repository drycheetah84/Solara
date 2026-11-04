package ares

import (
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

// Netvars from the dashboard: ?fleet=1 sets / removes a station config key on every station; without it only the
// page's station changes (the ported per-station behaviour).
func TestStationConfigFleetWide(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	dash := newAresDashboardServer()
	col := docdb.GetCollection(Database, true, models.NewStationDbObject)
	for _, id := range []string{"st-a", "st-b", "st-c"} {
		s := models.NewStationDbObject()
		s.StationId = dashStrPtr(id)
		col.Insert(s)
	}
	cfg := func(id string) map[string]*string {
		return col.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, id) }).Config
	}
	val := func(id, k string) string {
		if v, ok := cfg(id)[k]; ok && v != nil {
			return *v
		}
		return "(unset)"
	}

	if w := do(t, dash, "PATCH", "/api/stations/st-a/config", `{"only_a":"1"}`, adminHdr); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if val("st-a", "only_a") != "1" || val("st-b", "only_a") != "(unset)" {
		t.Fatalf("per-station patch leaked: b=%s", val("st-b", "only_a"))
	}
	if w := do(t, dash, "PATCH", "/api/stations/st-a/config?fleet=1", `{"gravity":"0.5"}`, adminHdr); w.Code != 200 || !strings.Contains(w.Body.String(), `"gravity":"0.5"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, id := range []string{"st-a", "st-b", "st-c"} {
		if val(id, "gravity") != "0.5" {
			t.Fatalf("%s gravity = %s", id, val(id, "gravity"))
		}
	}
	if b := do(t, dash, "DELETE", "/api/stations/st-b/config?key=gravity&fleet=1", "", adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	for _, id := range []string{"st-a", "st-b", "st-c"} {
		if val(id, "gravity") != "(unset)" {
			t.Fatalf("%s still has gravity", id)
		}
	}
	if val("st-a", "only_a") != "1" {
		t.Fatal("fleet delete removed an unrelated key")
	}
	if w := do(t, dash, "PATCH", "/api/stations/st-a/config?fleet=1", `{"x":"1"}`, nil); w.Code == 200 {
		t.Fatal("fleet patch must need the dashboard key")
	}
}
