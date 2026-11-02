package mothership

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTechTrees(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "someone-else")
	s := newTestServer(t)
	tok := tokenFor("tinkerer", time.Now().Add(time.Hour).Unix())
	get := func() (out struct {
		Results []struct {
			PlayerId        string
			Tree            struct{ Name string }
			NodeDefinitions []struct {
				Name     string `json:"name"`
				NodeId   string `json:"id"`
				TreeId   string `json:"tree_id"`
				Unlocked bool   `json:"unlocked"`
			}
		}
	}) {
		b := do(t, s, "GET", "/v1/progression-tree/client", "", "x-mothership-token", tok).Body.Bytes()
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err, string(b))
		}
		return
	}
	trees := get()
	nodes := 0
	var root, child, rootTree string
	for _, tr := range trees.Results {
		nodes += len(tr.NodeDefinitions)
		if tr.Tree.Name == "Sports" {
			for _, n := range tr.NodeDefinitions {
				switch n.Name {
				case "reward_baseball_bat":
					root, rootTree = n.NodeId, n.TreeId
				case "reward_cricket_bat":
					child = n.NodeId
				}
			}
		}
	}
	if len(trees.Results) != 4 || nodes != 77 || root == "" || child == "" {
		t.Fatalf("trees=%d nodes=%d root=%q child=%q", len(trees.Results), nodes, root, child)
	}
	unlock := func(node string) (int, string) {
		w := do(t, s, "POST", "/v1/progression-tree/client/"+rootTree+"/node/unlock", `{"node_id":"`+node+`"}`, "x-mothership-token", tok)
		return w.Code, w.Body.String()
	}
	if code, b := unlock(root); code == 200 || !strings.Contains(b, "not enough Hypercubes") { // Lv1 = 100 Hypercubes
		t.Fatalf("%d %s", code, b)
	}
	GrantItem("tinkerer", a2HypercubeId, 400)
	if code, b := unlock(child); code == 200 || !strings.Contains(b, "first") { // parent before child
		t.Fatalf("%d %s", code, b)
	}
	if code, b := unlock(root); code != 200 || !strings.Contains(b, `"InventoryRefreshRequired":true`) {
		t.Fatalf("%d %s", code, b)
	}
	got := map[string]int{}
	for _, it := range OwnedItems("tinkerer") {
		got[it.Id] = it.Quantity
	}
	if got[a2HypercubeId] != 300 || got["WieldableBaseballBat"] != 1 {
		t.Fatalf("owned: %+v", got)
	}
	// The game server unlocks for a player by mothership_player_id.
	w := do(t, s, "POST", "/v1/progression-tree/server/"+rootTree+"/node/unlock", `{"node_id":"`+child+`","mothership_player_id":"tinkerer"}`,
		"x-server-api-key", testServerKey)
	if w.Code != 200 {
		t.Fatalf("server unlock: %d %s", w.Code, w.Body.String())
	}
	unlocked := 0
	for _, tr := range get().Results {
		for _, n := range tr.NodeDefinitions {
			if n.Unlocked {
				unlocked++
			}
		}
	}
	if unlocked != 2 {
		t.Fatalf("unlocked nodes: %d", unlocked)
	}
}
