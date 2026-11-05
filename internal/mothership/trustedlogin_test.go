package mothership

import (
	"encoding/base64"
	"strings"
	"testing"

	"solara/internal/mothership/models"
)

// A trusted server signing in with an existing player's Mothership id gets THAT player (the cosmetic guard
// reads their inventory), and no junk QUEST player is created. A client doing the same gets the normal flow.
func TestQuestAuthTrustedServerExistingPlayerId(t *testing.T) {
	s := newTestServer(t)
	p := models.NewMothershipV2PlayerDbObject()
	p.PlayerId, p.ExternalProviderId, p.ExternalService, p.IsPrimaryId = strp("aaaa1111-2222-3333-4444-555566667777"), strp("28937999885827503"), strp("QUEST"), true
	p.Token = strp("stored-token")
	v2Players().Insert(p)
	before := len(v2Players().FindAll())

	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"aaaa1111-2222-3333-4444-555566667777"}`)
	rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"aaaa1111-2222-3333-4444-555566667777","AttestationToken":"x","MetaNonce":"x"}`, "x-server-api-key", testServerKey)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	m := decode(t, rec)
	if m["PlayerId"] != "aaaa1111-2222-3333-4444-555566667777" {
		t.Fatal(m)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(m["Token"].(string), ".")[1])
	if !strings.HasPrefix(string(payload), `{"sub":"aaaa1111-2222-3333-4444-555566667777","did":"`) ||
		!strings.Contains(string(payload), `"externalServiceId":"28937999885827503"`) {
		t.Fatal(string(payload))
	}
	if n := len(v2Players().FindAll()); n != before {
		t.Fatalf("players %d -> %d: a record was created", before, n)
	}
	if got := v2Players().FindOne(func(x *models.MothershipV2PlayerDbObject) bool { return eqS(x.PlayerId, "aaaa1111-2222-3333-4444-555566667777") }); sv(got.Token) != "stored-token" || sv(got.ExternalProviderId) != "28937999885827503" {
		t.Fatal("stored record changed")
	}
	// the token works as that player
	tok := m["Token"].(string)
	rec = do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}

	// without the server key the id is treated as a Meta id as before (a new player)
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"aaaa1111-2222-3333-4444-555566667777"}`)
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"aaaa1111-2222-3333-4444-555566667777","AttestationToken":"x","MetaNonce":"x"}`)
	if rec.Code == 201 && decode(t, rec)["PlayerId"] == "aaaa1111-2222-3333-4444-555566667777" {
		t.Fatal("untrusted caller signed in as an existing player by id")
	}
}
