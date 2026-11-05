package mothership

import "testing"

// Quest sign-in carries no name: the station-login username is used, a real name is never replaced by the
// "Player_<id>" placeholder, and a station login renames existing records.
func TestPlayerNames(t *testing.T) {
	s := newTestServer(t)
	names := map[string]string{}
	old := UsernameLookup
	UsernameLookup = func(id string) string { return names[id] }
	t.Cleanup(func() { UsernameLookup = old })

	signIn := func(id string) string {
		do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"`+id+`"}`)
		rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"`+id+`","AttestationToken":"x","MetaNonce":"n"}`, "x-server-api-key", testServerKey)
		if rec.Code != 201 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		return decode(t, rec)["ExternalProviderUsername"].(string)
	}
	stored := func(id string) string {
		for _, p := range v2Players().FindAll() {
			if eqS(p.ExternalProviderId, id) {
				return orStr(p.ExternalProviderUsername, "")
			}
		}
		return "(none)"
	}

	// unknown player: placeholder (unchanged behaviour)
	if n := signIn("11112222333"); n != "Player_11112222" {
		t.Fatalf("new unknown player = %q", n)
	}
	// they log in to a station with their real name -> the Mothership record is renamed
	if c := SyncPlayerName("11112222333", " drycheetah "); c != 1 || stored("11112222333") != "drycheetah" {
		t.Fatalf("sync changed %d, stored %q", c, stored("11112222333"))
	}
	// the next Quest sign-in keeps the real name even when the lookup has nothing (no placeholder overwrite)
	if n := signIn("11112222333"); n != "drycheetah" {
		t.Fatalf("re-sign-in = %q", n)
	}
	// a known station name is used straight away for a new player, and wins over an old stored name
	names["44445555666"] = "Andromeda.BSR"
	if n := signIn("44445555666"); n != "Andromeda.BSR" {
		t.Fatalf("known name = %q", n)
	}
	names["11112222333"] = "dry_renamed"
	if n := signIn("11112222333"); n != "dry_renamed" {
		t.Fatalf("renamed = %q", n)
	}
	// a game-server login keyed by that player's Mothership PlayerId (GUID) takes the player's name...
	var pid string
	for _, p := range v2Players().FindAll() {
		if eqS(p.ExternalProviderId, "11112222333") {
			pid = orStr(p.PlayerId, "")
		}
	}
	if n := signIn(pid); n != "dry_renamed" {
		t.Fatalf("server login by PlayerId = %q", n)
	}
	// ...and follows the player's next station-login rename
	if c := SyncPlayerName("11112222333", "dry3"); c != 2 || stored(pid) != "dry3" {
		t.Fatalf("sync changed %d, server-login record %q", c, stored(pid))
	}
	names["11112222333"] = "dry3"
	// placeholder / empty names never overwrite anything
	if c := SyncPlayerName("11112222333", "Player_abc"); c != 0 || stored("11112222333") != "dry3" {
		t.Fatalf("placeholder sync changed %d, stored %q", c, stored("11112222333"))
	}
	if c := SyncPlayerName("11112222333", "  "); c != 0 {
		t.Fatalf("empty sync changed %d", c)
	}
}
