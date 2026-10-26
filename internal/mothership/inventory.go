// Ports the inventory section of AUnrealFeatures.AAMothership/MothershipServer.cs:
// DecodeEquippedItems, the cosmetic catalogue (s_catalogueLock, s_catalogue,
// CosmeticCatalogue), StableId, GetInventoryClient, GetInventory and
// AddInventoryItem.
package mothership

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func inventoryItems() *docdb.Collection[models.MothershipInventoryItemDbObject] {
	return col(models.NewMothershipInventoryItemDbObject)
}

// decodeEquippedItems mirrors DecodeEquippedItems: distinct, non-empty item
// ids out of a stored player_inventory blob ("None" is not an item). A blob
// that cannot be read yields whatever was collected before the failure.
func decodeEquippedItems(storedValue *string) (ids []string) {
	ids = []string{}
	if storedValue == nil || *storedValue == "" {
		return ids
	}
	defer func() { _ = recover() }() // a blob we cannot read is the same as no items
	raw := fromBase64String(*storedValue)
	for i := range raw {
		raw[i]++
	}
	root, err := stj.Parse([]byte(utf8String(raw)))
	if err != nil {
		return ids
	}
	arr, ok := root.TryGetProperty("equippedItems")
	if !ok || arr.ValueKind() != stj.Array {
		return ids
	}
	for _, e := range arr.EnumerateArray() {
		v := ""
		if p := e.GetString(); p != nil {
			v = *p
		}
		if v == "" || v == "None" || containsStr(ids, v) {
			continue
		}
		ids = append(ids, v)
	}
	return ids
}

var (
	catalogueMu  sync.Mutex // s_catalogueLock
	catalogueAt  = dotnet.MinValue
	catalogueIds = []string{}
)

// cosmeticCatalogue mirrors CosmeticCatalogue: ids from cosmetic_catalogue.txt
// ("-<id>" blocks an id) plus every id a real client has equipped; rebuilt at
// most once a minute.
func cosmeticCatalogue() []string {
	catalogueMu.Lock()
	defer catalogueMu.Unlock()
	// DateTime.UtcNow - s_catalogue.At < 1 minute (compared in ticks: the
	// span from DateTime.MinValue overflows a time.Duration).
	if dotnet.UtcNow().Ticks()-catalogueAt.Ticks() < int64(time.Minute/100) {
		return catalogueIds
	}

	ids := []string{}
	blocked := []string{}
	add := func(id string) {
		if hash := strings.IndexByte(id, '#'); hash >= 0 {
			id = id[:hash]
		}
		id = strings.TrimSpace(id)
		if id == "" || id == "None" {
			return
		}
		if id[0] == '-' {
			if b := strings.TrimSpace(id[1:]); b != "" {
				blocked = append(blocked, b)
			}
			return
		}
		if containsStr(blocked, id) || containsStr(ids, id) {
			return
		}
		ids = append(ids, id)
	}

	// Read the file FIRST, so a "-<id>" line can keep a junk id out of the scan.
	func() {
		defer func() { _ = recover() }()
		path := filepath.Join(baseDirectory(), "cosmetic_catalogue.txt")
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			if lines, err := readAllLines(path); err == nil {
				for _, line := range lines {
					add(line)
				}
			}
		}
	}()

	func() {
		defer func() {
			if p := recover(); p != nil {
				authLog("[INVENTORY] catalogue scan failed: " + hosting.ExceptionMessage(p))
			}
		}()
		rows := userData().Find(func(d *models.MothershipUserDataDbObject) bool {
			return dbEq(d.KeyName, "player_inventory")
		})
		for _, row := range rows {
			for _, id := range decodeEquippedItems(row.Value) {
				add(id)
			}
		}
	}()

	catalogueAt, catalogueIds = dotnet.UtcNow(), ids
	authLog(fmt.Sprintf("[INVENTORY] catalogue rebuilt: %d item(s)", len(ids)))
	return ids
}

// stableId mirrors StableId: new Guid(SHA1(seed)[0..16]).ToString().
func stableId(seed string) string {
	h := sha1.Sum([]byte(seed))
	var b [16]byte
	copy(b[:], h[:16])
	return dotnet.FormatGuid(b)
}

// GetInventoryClient is the client inventory load
// ({ "Results": [ { "platform", "isPrimary", "entitlements": [...] } ] }).
func (s *MothershipServer) GetInventoryClient(c *hosting.Context) hosting.Result {
	request := c.Request
	if !isClient(request) {
		authLog("[INVENTORY] GET /v1/inventory/client -> 401 (no valid x-mothership-token)")
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}

	userId := getUserIdFromToken(request)
	if userId == "" {
		return jsonError(400, "BadRequest", "Missing token subject", "", nil)
	}

	entitlements := []any{}
	for _, item := range inventoryItems().Find(func(i *models.MothershipInventoryItemDbObject) bool {
		return dbEq(i.PlayerId, userId)
	}) {
		// skip a malformed stored item rather than fail the whole load (a null
		// ItemDataJson throws ArgumentNullException, caught the same way)
		if el, err := stj.Parse([]byte(sv(item.ItemDataJson))); err == nil {
			entitlements = append(entitlements, rewrittenElement{el})
		}
	}

	// Grant what the player wears, then the shared catalogue (so the client's
	// inventory gate opens even for a player who never joined a station).
	if len(entitlements) == 0 {
		inv := findUserData(userData(), userId, "player_inventory")
		var invValue *string
		if inv != nil {
			invValue = inv.Value
		}
		worn := decodeEquippedItems(invValue)

		granted := append([]string{}, worn...)
		for _, id := range cosmeticCatalogue() {
			if !containsStr(granted, id) {
				granted = append(granted, id)
			}
		}
		for _, id := range granted {
			entitlements = append(entitlements, stj.O(
				"entitlement_id", stableId(userId+":"+id),
				"in_game_id", id,
				"name", id,
			))
		}
		if len(entitlements) > 0 {
			authLog(fmt.Sprintf("[INVENTORY] %s: granted %d entitlement(s) (%d worn + catalogue)", userId, len(entitlements), len(worn)))
		} else {
			authLog(fmt.Sprintf("[INVENTORY] %s: nothing to grant -- catalogue is empty, so the client's ", userId) +
				"inventory gate stays shut and it will not save player_inventory")
		}
	}

	return jsonAnon(stj.O("Results", []any{stj.O("platform", "QUEST", "isPrimary", true, "entitlements", entitlements)}), 200)
}

// GetInventory returns inventory items for a player.
func (s *MothershipServer) GetInventory(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	playerId := c.Request.GetQueryParameter("player_id", "")
	items := inventoryItems().Find(func(i *models.MothershipInventoryItemDbObject) bool {
		return dbEq(i.PlayerId, playerId)
	})
	return jsonAnon(stj.O("Results", items), 200)
}

// AddInventoryItem adds or updates an inventory item for a player.
func (s *MothershipServer) AddInventoryItem(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	playerId := str(body, "player_id")
	itemId := orNewId(str(body, "item_id"))
	cl := inventoryItems()

	existing := cl.FindOne(func(i *models.MothershipInventoryItemDbObject) bool {
		return dbEq(i.PlayerId, playerId) && dbEq(i.ItemId, itemId)
	})
	if existing != nil {
		existing.ItemDataJson = strp(stj.MarshalString(body))
		cl.Update(existing)
	} else {
		item := models.NewMothershipInventoryItemDbObject()
		item.PlayerId = strp(playerId)
		item.ItemId = strp(itemId)
		item.ItemDataJson = strp(stj.MarshalString(body))
		cl.Insert(item)
	}

	allItems := cl.Find(func(i *models.MothershipInventoryItemDbObject) bool { return dbEq(i.PlayerId, playerId) })
	return jsonAnon(stj.O("Results", allItems), 200)
}
