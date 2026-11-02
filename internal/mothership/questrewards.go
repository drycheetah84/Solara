// Solara addition: quest completion rewards.
//
// On a quest completion the 50451 game server's UA2ProgressionClientBinder::ProcessReward (0x5706BB0) emits a
// Mothership *event* -- "on_permanent_quest_cosmetic_grant" {completed_quest_id, completed_quest_version,
// completed_permanent_reward_version, mothership_player_id, transaction_id} -- through the Mothership events
// subsystem (0x568C8B0). The live backend answered that event by running the reward transaction. There is no
// HTTP route for events in the game, so on our server it never left the process and nobody got a reward. The
// server DLL now forwards the event to the StationDb (POST /v1/deployments/{id}/quest_reward) and this applies it.
//
// What a transaction grants comes from /Game/A2/Progression/Core/KnownRewards (PermanentProgressionRewardTable),
// embedded as a2_50451_known_rewards.tsv: cosmetics are added once; currencies are added to the stored balance.
// A grant is recorded per (player, quest, reward version, transaction), so a repeated event grants nothing.
package mothership

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
)

//go:embed a2_50451_known_rewards.tsv
var a2KnownRewardsTsv string

// RewardChange is one inventory change of a reward transaction.
type RewardChange struct {
	ItemId   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

// QuestRewardGrantDbObject records that a reward was applied (idempotency).
type QuestRewardGrantDbObject struct {
	docdb.AstraDbObject
	PlayerId      string `json:"player_id"`
	QuestId       string `json:"quest_id"`
	RewardVersion string `json:"reward_version"`
	TransactionId string `json:"transaction_id"`
	GrantedAt     int64  `json:"granted_at"`
}

func NewQuestRewardGrantDbObject() *QuestRewardGrantDbObject { return &QuestRewardGrantDbObject{} }

var (
	a2RewardsOnce sync.Once
	a2Rewards     map[string][]RewardChange
	questRewardMu sync.Mutex
)

// normTxn reduces any GUID spelling (hyphens, braces, case) to 32 upper-case hex digits.
func normTxn(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r - 32)
		}
	}
	return b.String()
}

func knownRewards() map[string][]RewardChange {
	a2RewardsOnce.Do(func() {
		a2Rewards = map[string][]RewardChange{}
		for _, line := range strings.Split(a2KnownRewardsTsv, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' {
				continue
			}
			f := strings.Split(line, "\t")
			if len(f) != 3 {
				continue
			}
			q, err := strconv.Atoi(f[2])
			if err != nil || q <= 0 {
				continue
			}
			k := normTxn(f[0])
			a2Rewards[k] = append(a2Rewards[k], RewardChange{ItemId: f[1], Quantity: q})
		}
	})
	return a2Rewards
}

// RewardChanges returns what a reward transaction grants (nil = unknown transaction).
func RewardChanges(transactionId string) []RewardChange {
	return knownRewards()[normTxn(transactionId)]
}

// storedQuantity returns the player's stored quantity of itemId, and whether a row exists.
func storedQuantity(playerId, itemId string) (int, bool) {
	for _, it := range OwnedItems(playerId) {
		if it.Id == itemId {
			return it.Quantity, true
		}
	}
	return 0, false
}

// ApplyQuestReward grants a quest reward transaction to a Mothership player. It returns what was granted
// (empty when the grant was already applied) or a non-empty error.
func ApplyQuestReward(playerId, transactionId, questId, rewardVersion string) ([]RewardChange, string) {
	playerId = strings.TrimSpace(playerId)
	txn := normTxn(transactionId)
	if playerId == "" || len(txn) != 32 {
		return nil, "mothership_player_id and transaction_id are required"
	}
	changes := RewardChanges(txn)
	if changes == nil {
		return nil, "unknown reward transaction " + txn
	}
	quest := normTxn(questId)
	cl := col(NewQuestRewardGrantDbObject)
	questRewardMu.Lock()
	defer questRewardMu.Unlock()
	if cl.FindOne(func(g *QuestRewardGrantDbObject) bool {
		return dbEq(&g.PlayerId, playerId) && g.TransactionId == txn && g.QuestId == quest && g.RewardVersion == rewardVersion
	}) != nil {
		return []RewardChange{}, ""
	}
	_, cur := a2CatalogueItems()
	for _, ch := range changes {
		if cur[ch.ItemId] {
			base, ok := storedQuantity(playerId, ch.ItemId)
			if !ok && fullCatalogueUser(playerId) {
				base = a2CurrencyQuantity // the allowlisted default balance
			}
			GrantItem(playerId, ch.ItemId, base+ch.Quantity)
		} else if _, ok := storedQuantity(playerId, ch.ItemId); !ok {
			GrantItem(playerId, ch.ItemId, 1)
		}
	}
	cl.Insert(&QuestRewardGrantDbObject{PlayerId: playerId, QuestId: quest, RewardVersion: rewardVersion, TransactionId: txn,
		GrantedAt: time.Now().UnixMilli()})
	authLog("[QUEST-REWARD] " + playerId + ": transaction " + txn + " (quest " + quest + " v" + rewardVersion + ") granted")
	return changes, ""
}
