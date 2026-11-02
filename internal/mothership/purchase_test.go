package mothership

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"solara/internal/mothership/models"
	"solara/internal/secrets"
)

func TestPurchaseEconomy(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "someone-else")
	s := newTestServer(t)
	tok := tokenFor("buyer", time.Now().Add(time.Hour).Unix())
	buy := func(key string) (int, string) {
		w := do(t, s, "POST", "/v1/purchase/client", `{"OfferId":"`+stableId("offer:"+key)+`"}`, "x-mothership-token", tok)
		return w.Code, w.Body.String()
	}
	// The ATMs' displays exist (GetOfferByDisplayName("DriftiumPacks" / "HypercubePacks")); real-money packs carry the SKU.
	sf := do(t, s, "GET", "/v1/storefront/client", "", "x-mothership-token", tok).Body.String()
	if !strings.Contains(sf, `"DriftiumPacks":{"name":"DriftiumPacks"`) || !strings.Contains(sf, `"HypercubePacks":{"name":"HypercubePacks"`) ||
		!strings.Contains(sf, `"ExternalService":"QUEST","ExternalEntitlementId":"driftium_2200"`) ||
		!strings.Contains(sf, `"ExternalEntitlementId":"bundle_klaus_breaker"`) {
		t.Fatalf("storefront: %.400s", sf)
	}
	// A real-money offer can never be bought through Mothership.
	if code, b := buy("DriftiumPacks:1000"); code == 200 {
		t.Fatalf("real-money pack granted for free: %s", b)
	}
	// No Driftium yet: a Driftium-priced Hypercube pack is refused.
	if code, b := buy("HypercubePacks:driftium200"); code == 200 {
		t.Fatalf("bought without funds: %s", b)
	}
	GrantItem("buyer", a2DriftiumId, 1500)
	if code, b := buy("HypercubePacks:driftium200"); code != 200 || !strings.Contains(b, `{"Changes":{"`) || !strings.Contains(b, `"name":"Driftium","quantity":500`) ||
		!strings.Contains(b, `"name":"TechPoints","quantity":200`) {
		t.Fatalf("%d %s", code, b)
	}
	inv := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok).Body.String()
	if !strings.Contains(inv, `"in_game_id":"HardCurrencyTechPoints","name":"TechPoints","quantity":200`) {
		t.Fatalf("inventory after purchase: %.500s", inv)
	}
}

func TestRefreshIapPaysOutMetaPurchases(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "someone-else")
	s := newTestServer(t)
	secrets.Set("IAP_APP_ID", "app1")
	secrets.Set("IAP_APP_SECRET", "sec1")
	t.Cleanup(func() { secrets.Set("IAP_APP_ID", ""); secrets.Set("IAP_APP_SECRET", "") })

	p := models.NewMothershipV2PlayerDbObject()
	p.PlayerId, p.ExternalProviderId = strp("buyer"), strp("meta-77")
	v2Players().Insert(p)

	var mu sync.Mutex
	owned := map[string]bool{"driftium_2200": true, "bundle_klaus_breaker": true}
	var calls []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		defer mu.Unlock()
		if r.Form.Get("access_token") != "OC|app1|sec1" || r.Form.Get("user_id") != "meta-77" || !strings.HasPrefix(r.URL.Path, "/app1/") {
			w.WriteHeader(400)
			return
		}
		op, sku := strings.TrimPrefix(r.URL.Path, "/app1/"), r.Form.Get("sku")
		calls = append(calls, op+":"+sku)
		switch {
		case op == "verify_entitlement" && owned[sku]:
			_, _ = w.Write([]byte(`{"success":true}`))
		case op == "consume_entitlement" && owned[sku]:
			delete(owned, sku) // consumed: no longer owned
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"not owned"}}`))
		}
	}))
	defer fake.Close()
	old := metaGraphBase
	metaGraphBase = fake.URL
	t.Cleanup(func() { metaGraphBase = old })

	tok := tokenFor("buyer", time.Now().Add(time.Hour).Unix())
	refresh := func() string {
		return do(t, s, "GET", "/v1/purchase/client/refresh-iap", "", "x-mothership-token", tok).Body.String()
	}
	if b := refresh(); !strings.Contains(b, `"NewInventoryChangesAvailable":true`) {
		t.Fatal(b)
	}
	got := map[string]int{}
	for _, it := range OwnedItems("buyer") {
		got[it.Id] = it.Quantity
	}
	if got[a2DriftiumId] != 2200 || got["WearableHelmetKlausBreaker"] != 1 || got[a2HypercubeId] != 0 {
		t.Fatalf("owned after refresh: %+v", got)
	}
	// The consumable was consumed before it was credited; nothing pays out a second time.
	mu.Lock()
	joined := strings.Join(calls, ",")
	mu.Unlock()
	if !strings.Contains(joined, "verify_entitlement:driftium_2200,consume_entitlement:driftium_2200") ||
		strings.Contains(joined, "consume_entitlement:bundle_klaus_breaker") {
		t.Fatalf("calls: %s", joined)
	}
	if b := refresh(); !strings.Contains(b, `"NewInventoryChangesAvailable":false`) {
		t.Fatalf("second refresh: %s", b)
	}
	got = map[string]int{}
	for _, it := range OwnedItems("buyer") {
		got[it.Id] = it.Quantity
	}
	if got[a2DriftiumId] != 2200 {
		t.Fatalf("paid out twice: %+v", got)
	}
}

func TestQuestRewardsFromSavedProgress(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "someone-else")
	s := newTestServer(t)
	tok := tokenFor("quester", time.Now().Add(time.Hour).Unix())
	// CB4871AC... pays C1FD8E97... = 250 Driftium (Quests.quests -> KnownRewards); 908277... pays 9E69AE06... = 5 Hypercubes.
	save := func(quests string) {
		blob := encodeQuestBlob(`{"lastUpdate":"0001.01.01-00.00.00","quests":[` + quests + `],"savedQuestProgressionVersion":3}`)
		do(t, s, "POST", "/v1/userdata/client", `{"key_name":"player_quests","value":"`+blob+`"}`, "x-mothership-token", tok)
	}
	owned := func() map[string]int {
		m := map[string]int{}
		for _, it := range OwnedItems("quester") {
			m[it.Id] = it.Quantity
		}
		return m
	}
	save(`{"iD":"CB4871AC41992398F12F968C3BEA70FE","progress":3,"completedVersion":0}`) // in progress: nothing
	if got := owned(); got[a2DriftiumId] != 0 {
		t.Fatalf("uncompleted quest paid out: %+v", got)
	}
	save(`{"iD":"CB4871AC41992398F12F968C3BEA70FE","progress":255,"completedVersion":1},` +
		`{"iD":"908277524E7EB291814D229BF63C715A","progress":255,"completedVersion":1}`)
	if got := owned(); got[a2DriftiumId] != 250 || got[a2HypercubeId] != 5 {
		t.Fatalf("after completion: %+v", got)
	}
	save(`{"iD":"CB4871AC41992398F12F968C3BEA70FE","progress":255,"completedVersion":1}`) // saved again: no double pay
	if got := owned(); got[a2DriftiumId] != 250 {
		t.Fatalf("paid twice: %+v", got)
	}
}
