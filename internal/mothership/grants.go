package mothership

// Solara: give / take cosmetics and currency from the admin dashboard.
//
// A grant is a stored inventory row (the same collection AddInventoryItem writes) whose item_data is a complete
// 50451 entitlement -- {entitlement_id, in_game_id, name, quantity} -- so GetInventoryClient hands it to the client
// as-is. It works for every player, allowlisted or not: for a non-allowlisted player it is exactly what they own;
// for an allowlisted one a stored row overrides the catalogue's default quantity (e.g. a currency balance).
// The client reads its inventory at login, so a grant shows up the next time that player starts the game.

import (
	"sort"
	"strings"

	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// CatalogueItem is one grantable item from the 50451 item catalogue.
type CatalogueItem struct {
	Id       string `json:"id"`
	Currency bool   `json:"currency"`
}

// GrantableItems lists the embedded 50451 catalogue (cosmetics, bundles, currencies...), currencies first.
func GrantableItems() []CatalogueItem {
	ids, cur := a2CatalogueItems()
	out := make([]CatalogueItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, CatalogueItem{Id: id, Currency: cur[id]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Currency && !out[j].Currency })
	return out
}

// IsGrantable reports whether id is in the embedded catalogue.
func IsGrantable(id string) bool {
	ids, _ := a2CatalogueItems()
	return containsStr(ids, id)
}

// ResolvePlayerId maps a dashboard user id (a Meta account id) or a Mothership player id to the Mothership
// player id the inventory is keyed by. "" = unknown (the player has never logged in to Mothership).
func ResolvePlayerId(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if p := v2Players().FindOne(func(p *models.MothershipV2PlayerDbObject) bool { return dbEq(p.ExternalProviderId, id) }); p != nil {
		return sv(p.PlayerId)
	}
	if p := v2Players().FindOne(func(p *models.MothershipV2PlayerDbObject) bool { return dbEq(p.PlayerId, id) }); p != nil {
		return id
	}
	if p := col(models.NewMothershipPlayerDbObject).FindOne(func(p *models.MothershipPlayerDbObject) bool {
		return dbEq(p.ExternalAccountId, id) || dbEq(p.MothershipPlayerId, id)
	}); p != nil {
		return sv(p.MothershipPlayerId)
	}
	return ""
}

// OwnedItem is one stored (granted) inventory row.
type OwnedItem struct {
	Id       string `json:"id"`
	Quantity int    `json:"quantity"`
	Currency bool   `json:"currency"`
}

// OwnedItems lists the player's stored inventory rows that are real entitlements.
func OwnedItems(playerId string) []OwnedItem {
	_, cur := a2CatalogueItems()
	out := []OwnedItem{}
	for _, it := range inventoryItems().Find(func(i *models.MothershipInventoryItemDbObject) bool { return dbEq(i.PlayerId, playerId) }) {
		el, err := stj.Parse([]byte(sv(it.ItemDataJson)))
		if err != nil {
			continue
		}
		v, ok := el.TryGetProperty("in_game_id")
		if !ok || v.ValueKind() != stj.String {
			continue
		}
		id := sv(v.GetString())
		q := 1
		if qv, ok := el.TryGetProperty("quantity"); ok && qv.ValueKind() == stj.Number {
			q = int(qv.GetInt32())
		}
		out = append(out, OwnedItem{Id: id, Quantity: q, Currency: cur[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}

// GrantItem stores (or replaces) the entitlement for itemId with the given quantity (currencies: the balance).
func GrantItem(playerId, itemId string, quantity int) {
	cl := inventoryItems()
	data := stj.MarshalString(stj.O("entitlement_id", stableId(playerId+":"+itemId), "in_game_id", itemId,
		"name", a2EntitlementName(itemId), "quantity", quantity))
	if existing := cl.FindOne(func(i *models.MothershipInventoryItemDbObject) bool {
		return dbEq(i.PlayerId, playerId) && dbEq(i.ItemId, itemId)
	}); existing != nil {
		existing.ItemDataJson = strp(data)
		cl.Update(existing)
		return
	}
	it := models.NewMothershipInventoryItemDbObject()
	it.PlayerId, it.ItemId, it.ItemDataJson = strp(playerId), strp(itemId), strp(data)
	cl.Insert(it)
}

// RevokeItem deletes the player's stored row for itemId; false = they had none.
func RevokeItem(playerId, itemId string) bool {
	cl := inventoryItems()
	it := cl.FindOne(func(i *models.MothershipInventoryItemDbObject) bool {
		return dbEq(i.PlayerId, playerId) && dbEq(i.ItemId, itemId)
	})
	if it == nil {
		return false
	}
	cl.Delete(it.Id)
	return true
}

// HasFullCatalogue reports whether the player is on the full-cosmetics allowlist.
func HasFullCatalogue(playerId string) bool { return fullCatalogueUser(playerId) }
