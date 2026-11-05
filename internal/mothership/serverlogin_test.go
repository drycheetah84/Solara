package mothership

import (
	"strings"
	"testing"

	"solara/internal/mothership/models"
)

// The cosmetic guard signs in with a player's Mothership id (a server-login record) and must read THAT player's
// inventory; the login itself still creates/uses the server-login record (the DLL's own player_inventory store).
func TestServerLoginReadsRealPlayersInventory(t *testing.T) {
	s := newTestServer(t)
	real := "aaaa1111-2222-3333-4444-555566667777"
	p := models.NewMothershipV2PlayerDbObject()
	p.PlayerId, p.ExternalProviderId, p.ExternalService, p.IsPrimaryId = strp(real), strp("28937999885827503"), strp("QUEST"), true
	v2Players().Insert(p)
	GrantItem(real, "WearableChestCircuitSpryte", 1)

	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"`+real+`"}`)
	rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"`+real+`","AttestationToken":"x","MetaNonce":"x"}`, "x-server-api-key", testServerKey)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	m := decode(t, rec)
	if m["PlayerId"] == real {
		t.Fatal("server login must keep its own record")
	}
	tok := m["Token"].(string)
	inv := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok, "x-server-api-key", testServerKey)
	if inv.Code != 200 || !strings.Contains(inv.Body.String(), `"in_game_id":"WearableChestCircuitSpryte"`) {
		t.Fatal(inv.Code, inv.Body.String())
	}
	// without the server key the token's own (empty) player is read, as before
	inv = do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok)
	if strings.Contains(inv.Body.String(), "CircuitSpryte") {
		t.Fatal("untrusted caller read another player's inventory")
	}
	if serverLoginRealPlayer(real) != real || serverLoginRealPlayer("nobody") != "nobody" {
		t.Fatal("non server-login ids must pass through")
	}
}
