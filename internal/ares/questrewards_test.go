package ares

import (
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/mothership"
)

func TestQuestRewards(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	old := mothership.Database
	mothership.Database = Database
	t.Cleanup(func() { mothership.Database = old })
	st := newA2StationDbServer()

	keys := docdb.GetCollection(Database, true, models.NewUserApiKeyDbObject)
	k := models.NewUserApiKeyDbObject()
	k.UserId, k.ApiKey = dashStrPtr("u1"), dashStrPtr("player-key")
	keys.Insert(k)

	// KnownRewards: C1FD8E97-B17E4C51-8F187B40-F270341C -> 250 Driftium; D310A08B-3C834278-A3ED21B2-2419EC12 -> BadgeParkourPro6
	body := func(txn, ver string) string {
		return `{"event":"on_permanent_quest_cosmetic_grant","params":{"mothership_player_id":"p1","transaction_id":"` + txn +
			`","completed_quest_id":"0123456789abcdef0123456789abcdef","completed_permanent_reward_version":"` + ver + `"}}`
	}
	post := func(b string, hdr map[string]string) string {
		return a2Do(t, st, "POST", "/v1/deployments/dep1/quest_reward", b, hdr).Body.String()
	}
	srv := map[string]string{"x-api-key": a2TestMasterKey}

	if b := post(body("c1fd8e97-b17e-4c51-8f18-7b40f270341c", "1"), map[string]string{"x-api-key": "player-key"}); !strings.Contains(b, "forbidden") {
		t.Fatalf("a player key must not grant rewards: %s", b)
	}
	if b := post(body("c1fd8e97-b17e-4c51-8f18-7b40f270341c", "1"), srv); !strings.Contains(b, `"applied":true`) {
		t.Fatal(b)
	}
	if b := post(body("c1fd8e97-b17e-4c51-8f18-7b40f270341c", "1"), srv); !strings.Contains(b, `"applied":false`) {
		t.Fatalf("a repeated event must not grant again: %s", b)
	}
	if b := post(body("C1FD8E97B17E4C518F187B40F270341C", "2"), srv); !strings.Contains(b, `"applied":true`) {
		t.Fatalf("a new reward version grants again: %s", b)
	}
	if b := post(body("D310A08B-3C834278-A3ED21B2-2419EC12", "1"), srv); !strings.Contains(b, `"applied":true`) {
		t.Fatal(b)
	}
	if b := post(body("00000000000000000000000000000000", "1"), srv); !strings.Contains(b, "unknown reward transaction") {
		t.Fatal(b)
	}
	got := map[string]int{}
	for _, it := range mothership.OwnedItems("p1") {
		got[it.Id] = it.Quantity
	}
	if got["HardCurrencyDriftium"] != 500 || got["BadgeParkourPro6"] != 1 {
		t.Fatalf("owned: %+v", got)
	}
}
