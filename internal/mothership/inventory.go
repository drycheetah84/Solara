// Ports the inventory section of AUnrealFeatures.AAMothership/MothershipServer.cs:
// DecodeEquippedItems, the cosmetic catalogue (s_catalogueLock, s_catalogue,
// CosmeticCatalogue), StableId, GetInventoryClient, GetInventory and
// AddInventoryItem.
package mothership

import (
	"crypto/sha1"
	_ "embed"
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

// Solara extension: every item id the 50451 game knows (DT_ItemCatalogMetadata rows, dumped live by the
// server DLL's [ITEMCAT] pass), "<in_game_id>	<EA2CatalogItemCategory>". Added to the catalogue after the
// file's lines (so a "-<id>" line there still blocks one); currencies (category 5) get a balance.
//
//go:embed a2_50451_items.tsv
var a2ItemsTsv string

const a2CurrencyQuantity = 1000000

var (
	a2ItemsOnce     sync.Once
	a2ItemsIds      []string
	a2ItemsCurrency map[string]bool
)

func a2CatalogueItems() ([]string, map[string]bool) {
	a2ItemsOnce.Do(func() { a2ItemsIds, a2ItemsCurrency = parseA2Items() })
	return a2ItemsIds, a2ItemsCurrency
}

func parseA2Items() (ids []string, currency map[string]bool) {
	currency = map[string]bool{}
	for _, line := range strings.Split(a2ItemsTsv, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		ids = append(ids, f[0])
		if len(f) > 1 && f[1] == "5" {
			currency[f[0]] = true
		}
	}
	return ids, currency
}

// a2EntitlementName is the entitlement "name" the 50451 client expects. Currencies are looked up BY NAME, not by
// in_game_id: the kiosks (WBP_SharedIAPKiosk / WBP_SharedATMKiosk / WBP_SharedHypercubeATM / WBP_IAP_Bundle, decompiled)
// scan A2StoreSubsystem's inventory for Name == "Driftium" / "TechPoints" and show that entry's Quantity -- the live
// service named currency entitlements after the id without its "HardCurrency" prefix. Everything else keeps its id.
func a2EntitlementName(id string) string {
	if isCurrencyId(id) {
		return strings.TrimPrefix(id, "HardCurrency")
	}
	return id
}

func isCurrencyId(id string) bool {
	_, cur := a2CatalogueItems()
	return cur[id]
}

// a2CurrencyAliases maps a currency's in_game_id to the extra key the 50451 client also looks it up by. The store
// subsystem keys its inventory map by in_game_id, and AA2ProgressionPodActor::UpdateCurrency (0x57EA740, the tech-tree
// pods' Hypercube counter) looks up the key "TechPoints" -- not HardCurrencyTechPoints -- so Hypercubes are also sent
// under that id (same quantity). The kiosks match by name instead (a2EntitlementName), so they are unaffected.
var a2CurrencyAliases = map[string]string{"HardCurrencyTechPoints": "TechPoints"}

// currencyAlias returns the alias entitlement for a currency entitlement (nil = none).
func currencyAlias(userId, id string, quantity int) stj.Obj {
	alias, ok := a2CurrencyAliases[id]
	if !ok {
		return nil
	}
	return stj.O("entitlement_id", stableId(userId+":"+alias), "in_game_id", alias, "name", a2EntitlementName(id), "quantity", quantity)
}

// withCurrencyAliases appends the alias entitlement of every aliased currency in the list.
func withCurrencyAliases(userId string, ents []any) []any {
	out := ents
	for _, e := range ents {
		o, ok := e.(stj.Obj)
		if !ok {
			continue
		}
		var id string
		q := 0
		for _, kv := range o {
			switch kv.K {
			case "in_game_id":
				id, _ = kv.V.(string)
			case "quantity":
				if n, ok := kv.V.(int); ok {
					q = n
				}
			}
		}
		if a := currencyAlias(userId, id, q); a != nil {
			out = append(out, a)
		}
	}
	return out
}

func a2ItemQuantity(id string) int {
	if _, cur := a2CatalogueItems(); cur[id] {
		return a2CurrencyQuantity
	}
	return 1
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

	if ids50, _ := a2CatalogueItems(); len(ids50) > 0 {
		for _, id := range ids50 {
			add(id)
		}
	}

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
	// Solara: our game-server DLL reads a player's inventory (the cosmetic guard) by signing in with their
	// Mothership player id, which the QUEST login files under a separate "server-login" record. Read the real
	// player's inventory for it, or the guard sees nobody owning anything and strips bought cosmetics.
	if isTrustedServer(request) {
		userId = serverLoginRealPlayer(userId)
	}

	entitlements := []any{}
	have := map[string]bool{}
	for _, item := range inventoryItems().Find(func(i *models.MothershipInventoryItemDbObject) bool {
		return dbEq(i.PlayerId, userId)
	}) {
		// skip a malformed stored item rather than fail the whole load (a null
		// ItemDataJson throws ArgumentNullException, caught the same way)
		if el, err := stj.Parse([]byte(sv(item.ItemDataJson))); err == nil {
			if v, ok := el.TryGetProperty("in_game_id"); !ok || v.ValueKind() != stj.String {
				continue // not an entitlement: one malformed entry fails the whole 50451 inventory load
			}
			// 50451 entitlement parser (0x4DDBA90) requires entitlement_id, in_game_id, name AND an int
			// quantity -- one entitlement without it fails the whole inventory load.
			if idv, _ := el.TryGetProperty("in_game_id"); isCurrencyId(sv(idv.GetString())) {
				// stored currency rows (dashboard grants, quest rewards): re-emit with the client's lookup name
				id := sv(idv.GetString())
				q := 0
				if qv, ok := el.TryGetProperty("quantity"); ok && qv.ValueKind() == stj.Number {
					q = int(qv.GetInt32())
				}
				entitlements = append(entitlements, stj.O("entitlement_id", stableId(userId+":"+id), "in_game_id", id,
					"name", a2EntitlementName(id), "quantity", q))
			} else if _, ok := el.TryGetProperty("quantity"); ok {
				entitlements = append(entitlements, rewrittenElement{el})
			} else {
				g := func(k string) string {
					if v, ok := el.TryGetProperty(k); ok && v.ValueKind() == stj.String {
						return sv(v.GetString())
					}
					return ""
				}
				entitlements = append(entitlements, stj.O("entitlement_id", g("entitlement_id"), "in_game_id", g("in_game_id"),
					"name", a2EntitlementName(g("in_game_id")), "quantity", a2ItemQuantity(g("in_game_id"))))
			}
			if v, ok := el.TryGetProperty("in_game_id"); ok && v.ValueKind() == stj.String {
				have[sv(v.GetString())] = true
			}
		}
	}

	// Solara: the full catalogue is only for the allowlisted accounts (fullCatalogueUser). Everyone else gets
	// what they really own (the stored items above) plus every currency at 0, which keeps the client's
	// inventory gate open without handing out cosmetics.
	if !fullCatalogueUser(userId) {
		ids, cur := a2CatalogueItems()
		for _, id := range ids {
			if cur[id] && !have[id] {
				entitlements = append(entitlements, stj.O("entitlement_id", stableId(userId+":"+id),
					"in_game_id", id, "name", a2EntitlementName(id), "quantity", 0))
			}
		}
		authLog(fmt.Sprintf("[INVENTORY] %s: not allowlisted -> %d owned entitlement(s) + currencies", userId, len(have)))
		return jsonAnon(stj.O("Results", stj.O("QUEST", stj.O("platform", "QUEST", "isPrimary", true, "entitlements", withCurrencyAliases(userId, entitlements)))), 200)
	}

	// Grant what the player wears, then the shared catalogue (so the client's
	// inventory gate opens even for a player who never joined a station).
	// Solara: always merged (stored items no longer hide the catalogue).
	{
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
			if have[id] {
				continue
			}
			entitlements = append(entitlements, stj.O(
				"entitlement_id", stableId(userId+":"+id),
				"in_game_id", id,
				"name", a2EntitlementName(id),
				"quantity", a2ItemQuantity(id),
			))
		}
		if len(entitlements) > 0 {
			authLog(fmt.Sprintf("[INVENTORY] %s: granted %d entitlement(s) (%d worn + catalogue)", userId, len(entitlements), len(worn)))
		} else {
			authLog(fmt.Sprintf("[INVENTORY] %s: nothing to grant -- catalogue is empty, so the client's ", userId) +
				"inventory gate stays shut and it will not save player_inventory")
		}
	}

	// 50451 root parser (0x4DCDF10): Results is an OBJECT keyed by platform (cmp [v+1Eh],3), each member an
	// object -> 0x4DDD070. An array fails the whole load ("malformed, no message or statusCode found").
	return jsonAnon(stj.O("Results", stj.O("QUEST", stj.O("platform", "QUEST", "isPrimary", true, "entitlements", withCurrencyAliases(userId, entitlements)))), 200)
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

// defaultFullCatalogueIds are the Meta account ids that get every cosmetic: drycheetah (main and spec
// accounts) and unreleasedd. SOLARA_FULL_INVENTORY_IDS (comma-separated Meta or Mothership player ids)
// replaces the list when set.
var defaultFullCatalogueIds = []string{"28147570424869138", "27955102177488739", "28327711050183927"}

func fullCatalogueIds() []string {
	if v := strings.TrimSpace(os.Getenv("SOLARA_FULL_INVENTORY_IDS")); v != "" {
		out := []string{}
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return defaultFullCatalogueIds
}

// fullCatalogueUser reports whether the Mothership player id (the token subject) is allowlisted, either
// directly or through any platform account (v2 ExternalProviderId / v1 ExternalAccountId) linked to it.
func fullCatalogueUser(playerId string) bool {
	allow := fullCatalogueIds()
	if containsStr(allow, playerId) {
		return true
	}
	for _, p := range v2Players().Find(func(p *models.MothershipV2PlayerDbObject) bool { return dbEq(p.PlayerId, playerId) }) {
		if containsStr(allow, sv(p.ExternalProviderId)) {
			return true
		}
	}
	for _, p := range col(models.NewMothershipPlayerDbObject).Find(func(p *models.MothershipPlayerDbObject) bool {
		return dbEq(p.MothershipPlayerId, playerId)
	}) {
		if containsStr(allow, sv(p.ExternalAccountId)) {
			return true
		}
	}
	return false
}
