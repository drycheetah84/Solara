package mothership

import (
	"strings"
	"testing"
)

func TestGlobalBan(t *testing.T) {
	s := newTestServer(t)
	const meta = "28019295557747399"
	a := decode(t, do(t, s, "POST", "/v1/client/player/auth/RIFT", `{"UserId":"`+meta+`","Nonce":"x"}`))
	pid, tok := a["PlayerId"].(string), a["Token"].(string)
	if pid == "" || tok == "" {
		t.Fatal(a)
	}
	// the live token works before the ban
	if rec := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok); rec.Code != 200 {
		t.Fatalf("token before ban: %d", rec.Code)
	}

	// ban by Meta id -> resolves to the Mothership player
	ids, msg := AddBan(meta, 60, "slurs  in\tvoice", "test")
	if len(ids) != 2 || ids[0] != pid || ids[1] != meta || msg != "" {
		t.Fatal(ids, msg)
	}
	// login refused, in the client's Mothership error shape, with the reason
	rec := do(t, s, "POST", "/v1/client/player/auth/RIFT", `{"UserId":"`+meta+`","Nonce":"y"}`)
	body := rec.Body.String()
	if rec.Code != 403 || !strings.Contains(body, `"statusCode":403`) || !strings.Contains(body, `"MothershipErrorCode":"PLAYER_BANNED"`) ||
		!strings.Contains(body, `"ClientMessage":"You are banned from Halcyon for 60 more minutes. Reason: slurs in voice"`) {
		t.Fatalf("%d %s", rec.Code, body)
	}
	// the existing session is dead
	if rec := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok); rec.Code != 401 {
		t.Fatalf("token after ban: %d", rec.Code)
	}
	// permanent wording; lifting by Meta id ends it at once
	AddBan(pid, 0, "", "test")
	if b := activeBan(pid); b == nil || !strings.Contains(BanMessage(b), "permanently") {
		t.Fatal("permanent ban", b)
	}
	if n := LiftBans(meta); n != 3 {
		t.Fatalf("lifted %d, want 3", n)
	}
	if rec := do(t, s, "POST", "/v1/client/player/auth/RIFT", `{"UserId":"`+meta+`","Nonce":"z"}`); rec.Code != 201 {
		t.Fatalf("login after unban: %d %s", rec.Code, rec.Body.String())
	}
	if _, msg := AddBan(" ", 5, "", "test"); msg != "player_id required" {
		t.Fatal(msg)
	}
}
