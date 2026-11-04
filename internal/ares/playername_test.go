package ares

import (
	"testing"

	"solara/internal/mothership"
)

// A station login teaches Mothership the player's real name (mothership.UsernameLookup, wired in init).
func TestStationLoginNameReachesMothership(t *testing.T) {
	s, _ := a2TestEnv(t)
	if mothership.UsernameLookup == nil {
		t.Fatal("ares must wire mothership.UsernameLookup")
	}
	if n := mothership.UsernameLookup("27955102177488739"); n != "" {
		t.Fatalf("unknown player -> %q", n)
	}
	a2Do(t, s, "POST", "/users/log_in", `{"platform_id":"27955102177488739","userName":"drycheetah","platform":"quest"}`, nil)
	if n := mothership.UsernameLookup("27955102177488739"); n != "drycheetah" {
		t.Fatalf("after station login -> %q", n)
	}
	a2Do(t, s, "POST", "/v2/users/log_in", `{"platform_id":"27955102177488739","userName":"dry2","platform":"quest"}`, nil)
	if n := mothership.UsernameLookup("27955102177488739"); n != "dry2" {
		t.Fatalf("after rename -> %q", n)
	}
}
