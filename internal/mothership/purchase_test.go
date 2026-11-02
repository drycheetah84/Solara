package mothership

import (
	"strings"
	"testing"
	"time"
)

func TestPurchaseEconomy(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "someone-else")
	s := newTestServer(t)
	tok := tokenFor("buyer", time.Now().Add(time.Hour).Unix())
	buy := func(key string) (int, string) {
		w := do(t, s, "POST", "/v1/purchase/client", `{"OfferId":"`+stableId("offer:"+key)+`"}`, "x-mothership-token", tok)
		return w.Code, w.Body.String()
	}
	// The ATMs' displays exist (GetOfferByDisplayName("DriftiumPacks" / "HypercubePacks")).
	sf := do(t, s, "GET", "/v1/storefront/client", "", "x-mothership-token", tok).Body.String()
	if !strings.Contains(sf, `"DriftiumPacks":{"name":"DriftiumPacks"`) || !strings.Contains(sf, `"HypercubePacks":{"name":"HypercubePacks"`) {
		t.Fatalf("storefront displays: %.300s", sf)
	}
	// No Driftium yet: a Driftium-priced Hypercube pack is refused.
	if code, b := buy("HypercubePacks:driftium200"); code == 200 {
		t.Fatalf("bought without funds: %s", b)
	}
	// A free Driftium pack, then the Hypercube pack: 1000 - 1000 = 0 Driftium, 200 Hypercubes.
	if code, b := buy("DriftiumPacks:1000"); code != 200 || !strings.Contains(b, `"name":"Driftium","quantity":1000`) {
		t.Fatalf("%d %s", code, b)
	}
	if code, b := buy("HypercubePacks:driftium200"); code != 200 || !strings.Contains(b, `"name":"Driftium","quantity":0`) ||
		!strings.Contains(b, `"name":"TechPoints","quantity":200`) {
		t.Fatalf("%d %s", code, b)
	}
	inv := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok).Body.String()
	if !strings.Contains(inv, `"in_game_id":"HardCurrencyTechPoints","name":"TechPoints","quantity":200`) {
		t.Fatalf("inventory after purchase: %.500s", inv)
	}
}
