// Ports the offers / purchase / entitlement / transaction / storefront
// endpoints of AUnrealFeatures.AAMothership/MothershipServer.cs:
// AutomationCreateOffer, AutomationGetOffers, AutomationCreateOfferBinding,
// AutomationGetOfferBindings, AutomationCommitOfferBinding,
// AutomationCreateOfferDisplay, AutomationGetOfferDisplays, ClientPurchase,
// ClientRefreshIap, ServerPurchase, AutomationCreateEntitlement,
// AutomationGetEntitlements, AutomationUpdateEntitlement,
// AutomationCreateTransaction, AutomationGetTransactions,
// AutomationTransactionSunset and ClientGetStorefront.
package mothership

import (
	"fmt"
	"sync"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// elementOr mirrors `body.TryGetValue(key, out var e) ? e.ToString() : def`.
func elementOr(b jsonBody, key, def string) string {
	if e, ok := b.TryGetValue(key); ok {
		return e.ToString()
	}
	return def
}

func offers() *docdb.Collection[models.MothershipOfferDbObject] {
	return col(models.NewMothershipOfferDbObject)
}

func offerBindings() *docdb.Collection[models.MothershipOfferBindingDbObject] {
	return col(models.NewMothershipOfferBindingDbObject)
}

func offerDisplays() *docdb.Collection[models.MothershipOfferDisplayDbObject] {
	return col(models.NewMothershipOfferDisplayDbObject)
}

func entitlements() *docdb.Collection[models.MothershipEntitlementDbObject] {
	return col(models.NewMothershipEntitlementDbObject)
}

func transactions() *docdb.Collection[models.MothershipTransactionCatalogDbObject] {
	return col(models.NewMothershipTransactionCatalogDbObject)
}

func (s *MothershipServer) AutomationCreateOffer(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	offer := models.NewMothershipOfferDbObject()
	offer.OfferId = strp(newId())
	offer.Name = strp(str(body, "name"))
	offer.TitleId = strp(str(body, "titleId"))
	offer.EnvId = strp(str(body, "envId"))
	offer.TransactionId = strp(str(body, "transaction_id"))
	offer.BundlePricingJson = strp(elementOr(body, "bundle_pricing", "{}"))
	offer.DiscountPercent = intOr(body, "discount_percent", 0)
	offers().Insert(offer)
	return jsonT(offer, 200)
}

func (s *MothershipServer) AutomationGetOffers(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", offers().FindAll()), 200)
}

func (s *MothershipServer) AutomationCreateOfferBinding(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	binding := models.NewMothershipOfferBindingDbObject()
	binding.OfferBindingId = strp(newId())
	binding.TitleId = strp(str(body, "title_id"))
	binding.EnvId = strp(str(body, "env_id"))
	binding.DeploymentId = strp(str(body, "deployment_id"))
	binding.OfferDisplayId = strp(str(body, "offer_display_id"))
	binding.OfferId = strp(str(body, "offer_id"))
	binding.Committed = boolOr(body, "committed", false)
	binding.DisplayIndex = intOr(body, "display_index", 0)
	offerBindings().Insert(binding)
	return jsonT(binding, 200)
}

func (s *MothershipServer) AutomationGetOfferBindings(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", offerBindings().FindAll()), 200)
}

// AutomationCommitOfferBinding sets the committed flag on an offer binding.
func (s *MothershipServer) AutomationCommitOfferBinding(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	bindingId := str(body, "offer_binding_id")
	cl := offerBindings()
	binding := cl.FindOne(func(b *models.MothershipOfferBindingDbObject) bool {
		return dbEq(b.OfferBindingId, bindingId)
	})
	if binding == nil {
		return errorResult("Binding not found", 404)
	}
	binding.Committed = boolOr(body, "committed", true)
	cl.Update(binding)
	return jsonT(binding, 200)
}

func (s *MothershipServer) AutomationCreateOfferDisplay(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	display := models.NewMothershipOfferDisplayDbObject()
	display.OfferDisplayId = strp(newId())
	display.Name = strp(str(body, "name"))
	display.Description = strp(str(body, "description"))
	offerDisplays().Insert(display)
	return jsonT(display, 200)
}

func (s *MothershipServer) AutomationGetOfferDisplays(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", offerDisplays().FindAll()), 200)
}

// ClientPurchase processes a client purchase by offer ID (requires the admin
// key in the original as well).
func (s *MothershipServer) ClientPurchase(c *hosting.Context) hosting.Result {
	// Solara: a player buying one of the 50451 storefront's offers (storefront50451.go). Body {OfferDisplayId,
	// OfferDisplayIndex, OfferId}; reply {"Changes": [entitlement]} (parser 0x4DCF4E0 -> 0x4DD9F30, quantity int).
	if !isAdmin(c.Request) && isClient(c.Request) {
		userId := getUserIdFromToken(c.Request)
		offerId := str(parseBody(c.Request), "OfferId")
		if o := a2FindOffer(offerId); o != nil {
			changes, err := a2ApplyPurchase(userId, o)
			if err != "" {
				authLog(fmt.Sprintf("[STORE] %s could not buy %s: %s", userId, o.Key, err))
				return jsonError(400, "InsufficientFunds", err, "", nil)
			}
			authLog(fmt.Sprintf("[STORE] %s purchased %s", userId, o.Key))
			return jsonAnon(stj.O("Changes", changes), 200)
		}
		return jsonError(404, "NotFound", "Offer not found", "", nil)
	}
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	offerId := str(body, "OfferId")
	offer := offers().FindOne(func(o *models.MothershipOfferDbObject) bool { return dbEq(o.OfferId, offerId) })
	if offer == nil {
		return errorResult("Offer not found", 404)
	}
	return jsonAnon(stj.O("Changes", stj.O("purchased", true, "offer_id", offerId, "timestamp", utcNowO())), 200)
}

func (s *MothershipServer) ClientRefreshIap(c *hosting.Context) hosting.Result {
	// Solara: a player gets the 50451 shape ({"NewInventoryChangesAvailable": bool}, parser 0x4DCFF90; the client
	// sends GET). Nothing comes from a platform store here, so there are never new changes.
	if !isAdmin(c.Request) && isClient(c.Request) {
		return jsonAnon(stj.O("NewInventoryChangesAvailable", false), 200)
	}
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("status", "refreshed", "timestamp", utcNowO()), 200)
}

func (s *MothershipServer) ServerPurchase(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	transactionId := newId()
	return jsonAnon(stj.O(
		"Results", []any{stj.O("transaction_id", transactionId, "status", "completed", "timestamp", utcNowO())},
	), 200)
}

func (s *MothershipServer) AutomationCreateEntitlement(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	ent := models.NewMothershipEntitlementDbObject()
	ent.EntitlementId = strp(newId())
	ent.Name = strp(str(body, "name"))
	if t := str(body, "type"); len(t) > 0 {
		ent.Type = strp(t)
	} else {
		ent.Type = strp("DURABLE")
	}
	ent.ItemClass = strp(str(body, "item_class"))
	entitlements().Insert(ent)
	return jsonT(ent, 200)
}

func (s *MothershipServer) AutomationGetEntitlements(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	entitlementId, _ := c.Request.Query("entitlement_id")
	cl := entitlements()
	if entitlementId != "" {
		item := cl.FindOne(func(e *models.MothershipEntitlementDbObject) bool {
			return dbEq(e.EntitlementId, entitlementId)
		})
		if item != nil {
			return jsonT(item, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

func (s *MothershipServer) AutomationUpdateEntitlement(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	entitlementId := str(body, "entitlement_id")
	cl := entitlements()
	item := cl.FindOne(func(e *models.MothershipEntitlementDbObject) bool {
		return dbEq(e.EntitlementId, entitlementId)
	})
	if item == nil {
		return errorResult("Entitlement not found", 404)
	}
	if n := str(body, "name"); len(n) > 0 {
		item.Name = strp(n)
	}
	if t := str(body, "type"); len(t) > 0 {
		item.Type = strp(t)
	}
	if ic := str(body, "item_class"); len(ic) > 0 {
		item.ItemClass = strp(ic)
	}
	cl.Update(item)
	return jsonAnon(stj.O("result", item), 200)
}

func (s *MothershipServer) AutomationCreateTransaction(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	tx := models.NewMothershipTransactionCatalogDbObject()
	tx.TransactionId = strp(newId())
	tx.Name = strp(str(body, "name"))
	tx.OperationsJson = strp(elementOr(body, "operations", "[]"))
	transactions().Insert(tx)
	return jsonT(tx, 200)
}

func (s *MothershipServer) AutomationGetTransactions(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	transactionId, _ := c.Request.Query("transaction_id")
	cl := transactions()
	if transactionId != "" {
		item := cl.FindOne(func(t *models.MothershipTransactionCatalogDbObject) bool {
			return dbEq(t.TransactionId, transactionId)
		})
		if item != nil {
			return jsonT(item, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

// AutomationTransactionSunset returns an empty sunset list.
func (s *MothershipServer) AutomationTransactionSunset(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", []any{}), 200)
}

// ClientGetStorefront returns the storefront with offers and displays.
func (s *MothershipServer) ClientGetStorefront(c *hosting.Context) hosting.Result {
	// Solara: a player (x-mothership-token) gets the 50451 storefront. The original admitted admins only,
	// so every client fetch was a 401 ("Could not get the store for the user") and the bundle UI had no offers.
	if !isAdmin(c.Request) && isClient(c.Request) {
		return jsonAnon(stj.O("Results", a2Storefront(getUserIdFromToken(c.Request))), 200)
	}
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	o := offers().FindAll()
	d := offerDisplays().FindAll()
	return jsonAnon(stj.O("Results", stj.O("offers", o, "featured", []any{}, "displays", d)), 200)
}

var purchaseMu sync.Mutex

// currencyBalance is the player's balance of a currency: the stored row, else the allowlisted default, else 0.
func currencyBalance(playerId, id string) int {
	if q, ok := storedQuantity(playerId, id); ok {
		return q
	}
	if fullCatalogueUser(playerId) {
		return a2CurrencyQuantity
	}
	return 0
}

// a2ApplyPurchase charges an offer's debits and grants its credits, and returns the changed entitlements with their
// new totals (the 50451 purchase reply, parser 0x4DCF4E0 -> 0x4DD9F30). "" error = done.
func a2ApplyPurchase(playerId string, o *a2Offer) ([]any, string) {
	purchaseMu.Lock()
	defer purchaseMu.Unlock()
	totals := map[string]int{}
	for _, d := range o.Debits {
		if d.Type != "CURRENCY" {
			continue
		}
		b := currencyBalance(playerId, d.InGameId)
		if b < d.Change {
			return nil, fmt.Sprintf("not enough %s: %d < %d", a2EntitlementName(d.InGameId), b, d.Change)
		}
		totals[d.InGameId] = b - d.Change
	}
	for _, c := range o.Credits {
		if c.Type == "CURRENCY" {
			b, ok := totals[c.InGameId]
			if !ok {
				b = currencyBalance(playerId, c.InGameId)
			}
			totals[c.InGameId] = b + c.Change
		}
	}
	changes := []any{}
	for id, q := range totals {
		GrantItem(playerId, id, q)
		changes = append(changes, stj.O("entitlement_id", stableId(playerId+":"+id), "in_game_id", id, "name", a2EntitlementName(id), "quantity", q))
	}
	for _, c := range o.Credits {
		if c.Type == "CURRENCY" {
			continue
		}
		if _, owned := storedQuantity(playerId, c.InGameId); !owned {
			GrantItem(playerId, c.InGameId, 1)
		}
		changes = append(changes, stj.O("entitlement_id", stableId(playerId+":"+c.InGameId), "in_game_id", c.InGameId,
			"name", a2EntitlementName(c.InGameId), "quantity", 1))
	}
	return changes, ""
}
