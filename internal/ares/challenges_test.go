package ares

import (
	"encoding/json"
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

func TestChallengeRecords(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	st := newA2StationDbServer()

	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	keys := docdb.GetCollection(Database, true, models.NewUserApiKeyDbObject)
	for _, u := range []struct{ id, name, key string }{{"u1", "Alpha", "k1"}, {"u2", "Bravo", "k2"}, {"u3", "Charlie", "k3"}} {
		ud := models.NewUserDataResponse()
		ud.UserId, ud.Username = dashStrPtr(u.id), dashStrPtr(u.name)
		users.Insert(ud)
		k := models.NewUserApiKeyDbObject()
		k.UserId, k.ApiKey = dashStrPtr(u.id), dashStrPtr(u.key)
		keys.Insert(k)
	}
	add := func(key, user string, rec float64) string {
		body, _ := json.Marshal(map[string]any{"user_id": user, "record": rec})
		w := a2Do(t, st, "POST", "/v1/fleets/global/challenges/C1/add", string(body), map[string]string{"x-api-key": key})
		return w.Body.String()
	}
	if b := add("k1", "u1", 42.5); !strings.Contains(b, `"stored":true`) {
		t.Fatal(b)
	}
	if b := add("k1", "u1", 50); !strings.Contains(b, `"stored":false`) { // slower: best kept
		t.Fatal(b)
	}
	if b := add("k1", "u1", 40); !strings.Contains(b, `"stored":true`) { // faster: replaces
		t.Fatal(b)
	}
	add("k2", "u2", 30)
	add("k3", "u3", 55)
	// A player key posting another id is stored under the key's own user (u1), never under u2.
	if b := a2Do(t, st, "POST", "/v1/fleets/global/challenges/C2/add", `{"user_id":"u2","record":7}`, map[string]string{"x-api-key": "k1"}).Body.String(); !strings.Contains(b, `"stored":true`) {
		t.Fatal(b)
	}
	if b := a2Do(t, st, "POST", "/v1/fleets/global/challenges/C2/list?limit=5", `{"user_id":"u2"}`, nil).Body.String(); !strings.Contains(b, `"top":[{"user_id":"u1"`) || strings.Contains(b, `"me":{"user_id":"u2","userName":"Bravo","record":7`) {
		t.Fatal(b)
	}
	if w := a2Do(t, st, "POST", "/v1/fleets/global/challenges/C1/add", `{"user_id":"u1","record":1}`, nil); w.Code == 200 {
		t.Fatal("add must need a key")
	}
	if b := add("k1", "u1", -3); !strings.Contains(b, "positive") {
		t.Fatal(b)
	}

	w := a2Do(t, st, "POST", "/v1/fleets/global/challenges/C1/list?limit=2", `{"user_id":"u1","friends":["u3"]}`, map[string]string{"x-api-key": "k1"})
	var r struct {
		Top, Nearby, Friends []challengeEntry
		Me                   challengeEntry
	}
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if len(r.Top) != 2 || r.Top[0].UserId != "u2" || r.Top[0].Rank != 1 || r.Top[1].UserId != "u1" || r.Top[1].Record != 40 {
		t.Fatalf("top: %+v", r.Top)
	}
	if r.Me.UserId != "u1" || r.Me.Rank != 2 || r.Me.UserName != "Alpha" {
		t.Fatalf("me: %+v", r.Me)
	}
	if len(r.Friends) != 1 || r.Friends[0].UserId != "u3" || r.Friends[0].Rank != 3 {
		t.Fatalf("friends: %+v", r.Friends)
	}
	if len(r.Nearby) != 3 {
		t.Fatalf("nearby: %+v", r.Nearby)
	}
	for _, k := range []string{`"top"`, `"nearby"`, `"friends"`, `"me"`, `"userName"`, `"user_id"`, `"record"`, `"rank"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("response is missing %s: %s", k, w.Body.String())
		}
	}
	// An unknown challenge is an empty board, not an error.
	w = a2Do(t, st, "POST", "/v1/fleets/global/challenges/NONE/list?limit=5", `{"user_id":"u1","friends":[]}`, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"top":[]`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
