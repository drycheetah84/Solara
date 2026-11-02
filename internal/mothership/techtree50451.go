// Solara addition: the 50451 tech trees (Mainframe / Upgrade Hive progression pods).
//
// The client (UA2TechTreeSystem::HydrateNodes, 0x57CD420) fetches GET /v1/progression-tree/client and, per tree, matches
// each data-table node (/Game/A2/TechTree/Data, by row name: reward_baseball_bat, ...) to a backend node with the same
// "name"; it copies the backend ids (MothershipNodeId / MothershipTreeId) and bPurchased (= node "unlocked") onto the
// board. Without backend nodes every node is "missing from mothership" and unavailable.
//
// Tree state object (root parser 0x4DD4620; served in Results[] and as the unlock reply):
//
//	{PlayerId, InventoryRefreshRequired: bool, Tree: {env_id, title_id, name, track_id},
//	 NodeDefinitions: [{tree_id, name, prerequisite_nodes{}, prerequisite_entitlements{}, prerequisite_levels{},
//	                    cost: {<key>: {Delta, Entitlement{entitlement_id, name, in_game_id, title_id, env_id, type}}},
//	                    transaction: {transaction_id, name, title_id, env_id, inventory_changes: {<key>: {Delta, Entitlement}}},
//	                    unlocked: bool}],
//	 Track: {...} (REQUIRED key; {} or null accepted). Every node and the Tree also carry "id" (node id must be non-empty)}
//
// Unlock: the game server calls POST <base>/<scope>/<tree id>/node/unlock (0x4DF7060; also /node/forceUnlock) with
// {node_id, mothership_player_id}; /node/lock (0x4DF1EB0) adds refund_costs / rewind_rewards. Solara enforces the
// parent-node order and the Hypercube price (socket tiers: Lv1 100, Lv2 250, Lv3 500; nodes without a socket are free),
// grants the node's item, and records the unlock. v1 sends empty prerequisite objects (the parent order is enforced here).
package mothership

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

//go:embed a2_50451_techtree.tsv
var a2TechTreeTsv string

type a2TreeNode struct {
	Graph, GraphName, Name, Parent, Item string
	Cost                                 int
}

type a2Tree struct {
	Row, Name string
	Nodes     []*a2TreeNode
}

// TechTreeUnlockDbObject records one unlocked node for a player.
type TechTreeUnlockDbObject struct {
	docdb.AstraDbObject
	PlayerId   string `json:"player_id"`
	Graph      string `json:"graph"`
	Node       string `json:"node"`
	UnlockedAt int64  `json:"unlocked_at"`
}

func NewTechTreeUnlockDbObject() *TechTreeUnlockDbObject { return &TechTreeUnlockDbObject{} }

var (
	a2TreesOnce sync.Once
	a2TreesList []*a2Tree
	techTreeMu  sync.Mutex
)

func a2SocketCost(tier string) int {
	switch {
	case strings.HasSuffix(tier, "Lv1"):
		return 100
	case strings.HasSuffix(tier, "Lv2"):
		return 250
	case strings.HasSuffix(tier, "Lv3"), strings.Contains(tier, "PVP"):
		return 500
	}
	return 0
}

func a2Trees() []*a2Tree {
	a2TreesOnce.Do(func() {
		byRow := map[string]*a2Tree{}
		for _, line := range splitLines(a2TechTreeTsv) {
			if line == "" || line[0] == '#' {
				continue
			}
			f := strings.Split(line, "\t")
			if len(f) != 6 {
				continue
			}
			t := byRow[f[0]]
			if t == nil {
				t = &a2Tree{Row: f[0], Name: f[1]}
				byRow[f[0]] = t
				a2TreesList = append(a2TreesList, t)
			}
			n := &a2TreeNode{Graph: f[0], GraphName: f[1], Name: f[2], Item: f[4]}
			if f[3] != "-" {
				n.Parent = f[3]
			}
			for _, s := range strings.Split(f[5], ",") {
				n.Cost += a2SocketCost(s)
			}
			t.Nodes = append(t.Nodes, n)
		}
	})
	return a2TreesList
}

func a2TreeId(t *a2Tree) string     { return stableId("tree:" + t.Row) }
func a2NodeId(n *a2TreeNode) string { return stableId("node:" + n.Graph + ":" + n.Name) }

func treeUnlocks() *docdb.Collection[TechTreeUnlockDbObject] { return col(NewTechTreeUnlockDbObject) }

func unlockedNodes(playerId string) map[string]bool {
	out := map[string]bool{}
	for _, u := range treeUnlocks().Find(func(u *TechTreeUnlockDbObject) bool { return dbEq(&u.PlayerId, playerId) }) {
		out[u.Graph+":"+u.Node] = true
	}
	return out
}

func a2Entitlement(playerId, id, typ string) stj.Obj {
	return stj.O("entitlement_id", stableId(playerId+":"+id), "name", a2EntitlementName(id), "in_game_id", id,
		"title_id", "", "env_id", "", "type", typ)
}

func a2TreeState(playerId string, t *a2Tree, refresh bool) stj.Obj {
	have := unlockedNodes(playerId)
	defs := []any{}
	for _, n := range t.Nodes {
		cost := stj.O()
		if n.Cost > 0 {
			cost = append(cost, stj.KV{K: a2HypercubeId, V: stj.O("Delta", n.Cost, "Entitlement", a2Entitlement(playerId, a2HypercubeId, "CURRENCY"))})
		}
		changes := stj.O()
		if n.Item != "" {
			changes = append(changes, stj.KV{K: n.Item, V: stj.O("Delta", 1, "Entitlement", a2Entitlement(playerId, n.Item, "DURABLE"))})
		}
		defs = append(defs, stj.O(
			"tree_id", a2TreeId(t),
			"name", n.Name,
			"id", a2NodeId(n), // required, non-empty (key at 0x148D80558)
			"prerequisite_nodes", stj.O(),
			"prerequisite_entitlements", []any{},
			"prerequisite_levels", []any{},
			"cost", cost,
			"transaction", stj.O("transaction_id", stableId("tx:"+n.Graph+":"+n.Name), "name", n.Name, "title_id", "", "env_id", "",
				"inventory_changes", changes),
			"unlocked", have[n.Graph+":"+n.Name],
		))
	}
	return stj.O("PlayerId", playerId, "InventoryRefreshRequired", refresh,
		"Tree", stj.O("id", a2TreeId(t), "env_id", "", "title_id", "", "name", t.Name, "track_id", ""),
		"NodeDefinitions", defs,
		"Track", stj.O()) // required key: a missing Track fails the whole tree ("malformed"); {} or null = no track
}

// ClientGetProgressionTrees serves every tech tree with the player's unlocks.
func (s *MothershipServer) ClientGetProgressionTrees(c *hosting.Context) hosting.Result {
	if !isClient(c.Request) && !isAdmin(c.Request) {
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}
	playerId := getUserIdFromToken(c.Request)
	out := []any{}
	for _, t := range a2Trees() {
		out = append(out, a2TreeState(playerId, t, false))
	}
	return jsonAnon(stj.O("Results", out), 200)
}

// a2FindNode finds a node by its backend id (unique across trees) or its row name; the tree segment of the unlock URL is
// not needed (and its position in "%s/%s/%s/node/unlock" is not verified), so it is ignored.
func a2FindNode(_, nodeId string) (*a2Tree, *a2TreeNode) {
	for _, t := range a2Trees() {
		for _, n := range t.Nodes {
			if strings.EqualFold(a2NodeId(n), nodeId) || strings.EqualFold(n.Name, nodeId) {
				return t, n
			}
		}
	}
	return nil, nil
}

// UnlockTechTreeNode unlocks one node for a player: parent first, Hypercube price charged, item granted. force skips
// the price and the parent check (admin forceUnlock).
func UnlockTechTreeNode(playerId, treeId, nodeId string, force bool) (*a2Tree, string) {
	t, n := a2FindNode(treeId, nodeId)
	if n == nil {
		return nil, "unknown node"
	}
	techTreeMu.Lock()
	defer techTreeMu.Unlock()
	have := unlockedNodes(playerId)
	if have[n.Graph+":"+n.Name] {
		return t, ""
	}
	if !force && n.Parent != "" && !have[n.Graph+":"+n.Parent] {
		return nil, "unlock " + n.Parent + " first"
	}
	if !force && n.Cost > 0 {
		purchaseMu.Lock()
		b := currencyBalance(playerId, a2HypercubeId)
		if b < n.Cost {
			purchaseMu.Unlock()
			return nil, fmt.Sprintf("not enough Hypercubes: %d < %d", b, n.Cost)
		}
		GrantItem(playerId, a2HypercubeId, b-n.Cost)
		purchaseMu.Unlock()
	}
	if n.Item != "" {
		if _, owned := storedQuantity(playerId, n.Item); !owned {
			GrantItem(playerId, n.Item, 1)
		}
	}
	treeUnlocks().Insert(&TechTreeUnlockDbObject{PlayerId: playerId, Graph: n.Graph, Node: n.Name, UnlockedAt: time.Now().UnixMilli()})
	authLog(fmt.Sprintf("[TECHTREE] %s unlocked %s/%s (cost %d, force=%v)", playerId, n.Graph, n.Name, n.Cost, force))
	return t, ""
}

// LockTechTreeNode removes an unlock (admin "lock"); refund gives the price back.
func LockTechTreeNode(playerId, treeId, nodeId string, refund bool) (*a2Tree, string) {
	t, n := a2FindNode(treeId, nodeId)
	if n == nil {
		return nil, "unknown node"
	}
	techTreeMu.Lock()
	defer techTreeMu.Unlock()
	for _, u := range treeUnlocks().Find(func(u *TechTreeUnlockDbObject) bool {
		return dbEq(&u.PlayerId, playerId) && u.Graph == n.Graph && u.Node == n.Name
	}) {
		treeUnlocks().Delete(u.Id)
		if refund && n.Cost > 0 {
			purchaseMu.Lock()
			GrantItem(playerId, a2HypercubeId, currencyBalance(playerId, a2HypercubeId)+n.Cost)
			purchaseMu.Unlock()
		}
	}
	return t, ""
}

// TreeNodeAction handles POST /v1/progression-tree/{scope}/{tree_id}/node/{action} (unlock / forceUnlock / lock).
func (s *MothershipServer) TreeNodeAction(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	playerId := ""
	switch {
	case isAdmin(c.Request): // the game server (or an admin) acts for the player named in the body
		playerId = strings.TrimSpace(str(body, "mothership_player_id"))
	case isClient(c.Request):
		playerId = getUserIdFromToken(c.Request)
	default:
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}
	if playerId == "" {
		return jsonError(400, "BadRequest", "mothership_player_id required", "", nil)
	}
	treeId, nodeId, action := c.Param("tree_id"), str(body, "node_id"), c.Param("action")
	var t *a2Tree
	var errMsg string
	switch action {
	case "unlock":
		t, errMsg = UnlockTechTreeNode(playerId, treeId, nodeId, false)
	case "forceUnlock":
		if !isAdmin(c.Request) {
			return unauthorized()
		}
		t, errMsg = UnlockTechTreeNode(playerId, treeId, nodeId, true)
	case "lock":
		if !isAdmin(c.Request) {
			return unauthorized()
		}
		t, errMsg = LockTechTreeNode(playerId, treeId, nodeId, boolOr(body, "refund_costs", false))
	default:
		return jsonError(404, "NotFound", "unknown node action", "", nil)
	}
	if errMsg != "" {
		authLog(fmt.Sprintf("[TECHTREE] %s %s %s: %s", playerId, action, nodeId, errMsg))
		return jsonError(400, "BadRequest", errMsg, "", nil)
	}
	return jsonAnon(a2TreeState(playerId, t, true), 200)
}
