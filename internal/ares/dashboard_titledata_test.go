package ares

import (
	"encoding/json"
	"strings"
	"testing"

	"solara/internal/mothership"
)

func TestTitleDataAdminEditor(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	old := mothership.Database
	mothership.Database = Database
	t.Cleanup(func() { mothership.Database = old })
	s := newAresDashboardServer()

	if w := do(t, s, "PUT", "/api/titledata/motd", `{"value":"Welcome to Halcyon"}`, adminHdr); w.Code != 200 {
		t.Fatalf("admin put: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "PUT", "/api/titledata/StationBrowserFlair", `{"value":"{not json"}`, adminHdr); w.Code != 400 {
		t.Fatalf("invalid JSON for a JSON key must be refused: %d", w.Code)
	}
	if w := do(t, s, "PUT", "/api/titledata/bad%20key", `{"value":"x"}`, adminHdr); w.Code != 400 {
		t.Fatalf("bad key must be refused: %d", w.Code)
	}
	w := do(t, s, "GET", "/api/titledata", "", adminHdr)
	var out struct {
		Known  []map[string]any `json:"known"`
		Stored []map[string]any `json:"stored"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
		t.Fatalf("list: %d %v %s", w.Code, err, w.Body.String())
	}
	if len(out.Known) < 5 || len(out.Stored) != 1 || out.Stored[0]["value"] != "Welcome to Halcyon" {
		t.Fatalf("list content: %s", w.Body.String())
	}
	found := false
	for _, r := range mothership.ListTitleData() {
		found = found || (r.Key == "motd" && r.Value == "Welcome to Halcyon")
	}
	if !found {
		t.Fatal("stored row not visible to the client title-data store")
	}

	// a scoped token with every permission on every fleet is still refused
	all := []string{}
	for _, p := range dashScopedPerms {
		all = append(all, `"`+p.Name+`"`)
	}
	w = do(t, s, "POST", "/api/scoped-tokens", `{"name":"everything","permissions":[`+strings.Join(all, ",")+`],"stations":["*"]}`, adminHdr)
	var tok map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	th := map[string]string{"x-dashboard-key": tok["token"].(string)}
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if w := do(t, s, m, "/api/titledata/motd", `{"value":"x"}`, th); m != "GET" && w.Code != 403 {
			t.Fatalf("scoped %s must be refused: %d", m, w.Code)
		}
	}
	if w := do(t, s, "GET", "/api/titledata", "", th); w.Code != 403 {
		t.Fatalf("scoped list must be refused: %d", w.Code)
	}

	if w := do(t, s, "DELETE", "/api/titledata/motd", "", adminHdr); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := do(t, s, "DELETE", "/api/titledata/motd", "", adminHdr); w.Code != 404 {
		t.Fatalf("second delete: %d", w.Code)
	}
}
