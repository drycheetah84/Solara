package ares

import (
	"strings"
	"testing"
	"time"

	"solara/internal/docdb"
	"solara/internal/mothership"
	mmodels "solara/internal/mothership/models"
)

func TestDashboardGrants(t *testing.T) {
	oldLoc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = oldLoc })
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "nobody")
	resetDashAuth(t, nil)
	withTempDB(t)
	old := mothership.Database
	mothership.Database = Database
	t.Cleanup(func() { mothership.Database = old })
	s := newAresDashboardServer()

	pl := mmodels.NewMothershipV2PlayerDbObject()
	pl.PlayerId, pl.ExternalProviderId = dashStrPtr("P1"), dashStrPtr("4242")
	docdb.GetCollection(Database, true, mmodels.NewMothershipV2PlayerDbObject).Insert(pl)

	items := mothership.GrantableItems()
	if len(items) < 600 || !items[0].Currency {
		t.Fatalf("catalogue: %d items, first %+v", len(items), items[0])
	}
	cos := ""
	for _, it := range items {
		if !it.Currency {
			cos = it.Id
			break
		}
	}
	cur := items[0].Id

	if b := do(t, s, "POST", "/api/users/4242/inventory", `{"item_id":"`+cos+`"}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, s, "POST", "/api/users/4242/inventory", `{"item_id":"`+cur+`","quantity":5000}`, adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, s, "POST", "/api/users/4242/inventory", `{"item_id":"NotARealItem"}`, adminHdr).Body.String(); !strings.Contains(b, "unknown item_id") {
		t.Fatal(b)
	}
	if b := do(t, s, "POST", "/api/users/9999/inventory", `{"item_id":"`+cos+`"}`, adminHdr).Body.String(); !strings.Contains(b, "never logged in") {
		t.Fatal(b)
	}
	b := do(t, s, "GET", "/api/users/4242/inventory", "", adminHdr).Body.String()
	if !strings.Contains(b, `"player_id":"P1"`) || !strings.Contains(b, `"id":"`+cos+`","quantity":1`) || !strings.Contains(b, `"id":"`+cur+`","quantity":5000`) {
		t.Fatal(b)
	}
	// re-granting a currency replaces the balance
	do(t, s, "POST", "/api/users/4242/inventory", `{"item_id":"`+cur+`","quantity":7}`, adminHdr)
	if b := do(t, s, "GET", "/api/users/P1/inventory", "", adminHdr).Body.String(); !strings.Contains(b, `"id":"`+cur+`","quantity":7`) {
		t.Fatal(b)
	}
	if b := do(t, s, "DELETE", "/api/users/4242/inventory/"+cos, "", adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, s, "GET", "/api/users/4242/inventory", "", adminHdr).Body.String(); strings.Contains(b, `"id":"`+cos+`"`) {
		t.Fatal(b)
	}
	if w := do(t, s, "GET", "/api/catalog/items", "", nil); w.Code == 200 {
		t.Fatal("catalog must need the dashboard key")
	}
}
