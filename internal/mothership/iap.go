// Solara addition: real-money in-app purchases through the Meta (Quest) store.
//
// Client flow (50451, A2StoreSubsystem::StartClientPurchase 0x56CF6A0): an offer whose ExternalService is "QUEST" (or
// "RIFT") and whose ExternalEntitlementId is non-empty is NOT bought through Mothership; the client hands the
// ExternalEntitlementId (the Meta SKU) to the Platform SDK checkout (0x4C7D620) and afterwards asks the backend to
// refresh (GET /v1/purchase/client/refresh-iap -> {"NewInventoryChangesAvailable": bool}), then reloads its inventory.
//
// On refresh Solara asks Meta, server to server, which of our SKUs the player owns (verify_entitlement, app access
// token OC|APP_ID|APP_SECRET) and pays them out:
//   - consumables (currency packs) are CONSUMED first and credited only when the consume succeeded, so one purchase can
//     never pay out twice;
//   - durables (bundles) are granted once (stored rows are idempotent) and never consumed.
//
// Secrets: IAP_APP_ID / IAP_APP_SECRET (config/legacy-secrets.env or SOLARA_IAP_APP_ID / SOLARA_IAP_APP_SECRET).
package mothership

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"solara/internal/mothership/models"
	"solara/internal/secrets"
)

// a2IapService is the ExternalService the 50451 client routes to the Quest Platform SDK checkout.
const a2IapService = "QUEST"

type a2IapSku struct {
	Sku     string
	Credits []a2Credit
	Durable bool
}

// metaGraphBase is the Meta server-to-server API root (a variable so tests can point it at a fake).
var metaGraphBase = "https://graph.oculus.com"

var metaHTTP = &http.Client{Timeout: 10 * time.Second}

// a2IapSkus lists every real-money SKU configured on the Meta store page.
func a2IapSkus() []a2IapSku {
	out := []a2IapSku{}
	for _, n := range []int{1000, 2200, 5000, 11000} {
		out = append(out, a2IapSku{Sku: fmt.Sprintf("driftium_%d", n), Credits: []a2Credit{{a2DriftiumId, "CURRENCY", n}}})
	}
	for _, n := range []int{200, 440, 1000, 2200} {
		out = append(out, a2IapSku{Sku: fmt.Sprintf("hypercubes_%d", n), Credits: []a2Credit{{a2HypercubeId, "CURRENCY", n}}})
	}
	out = append(out, a2IapSku{Sku: "bundle_klaus_breaker", Credits: a2KlausBreakerItems(), Durable: true})
	return out
}

// metaAccountId returns the Meta user id linked to a Mothership player ("" = none).
func metaAccountId(playerId string) string {
	if p := v2Players().FindOne(func(p *models.MothershipV2PlayerDbObject) bool { return dbEq(p.PlayerId, playerId) }); p != nil &&
		sv(p.ExternalProviderId) != "" {
		return sv(p.ExternalProviderId)
	}
	if p := col(models.NewMothershipPlayerDbObject).FindOne(func(p *models.MothershipPlayerDbObject) bool {
		return dbEq(p.MothershipPlayerId, playerId)
	}); p != nil {
		return sv(p.ExternalAccountId)
	}
	return ""
}

// metaS2S calls one Meta server-to-server entitlement endpoint ("verify_entitlement" / "consume_entitlement").
func metaS2S(op, metaUser, sku string) (bool, error) {
	appId, secret := secrets.Get("IAP_APP_ID"), secrets.Get("IAP_APP_SECRET")
	if appId == "" || secret == "" {
		return false, fmt.Errorf("IAP_APP_ID / IAP_APP_SECRET not configured")
	}
	form := url.Values{"access_token": {"OC|" + appId + "|" + secret}, "user_id": {metaUser}, "sku": {sku}}
	resp, err := metaHTTP.PostForm(metaGraphBase+"/"+url.PathEscape(appId)+"/"+op, form)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var r struct {
		Success bool `json:"success"`
	}
	_ = json.Unmarshal(body, &r)
	if resp.StatusCode != 200 {
		// Meta answers "not owned" with an error body; that is a normal false, not a failure.
		return false, nil
	}
	return r.Success, nil
}

// RefreshIap pays out every Meta purchase the player owns that has not been paid yet. True = the inventory changed.
func RefreshIap(playerId string) bool {
	metaUser := metaAccountId(playerId)
	if metaUser == "" {
		return false
	}
	changed := false
	for _, s := range a2IapSkus() {
		owned, err := metaS2S("verify_entitlement", metaUser, s.Sku)
		if err != nil {
			authLog("[IAP] verify " + s.Sku + " for " + playerId + ": " + err.Error())
			return changed
		}
		if !owned {
			continue
		}
		if s.Durable {
			for _, c := range s.Credits {
				if _, have := storedQuantity(playerId, c.InGameId); !have {
					GrantItem(playerId, c.InGameId, 1)
					changed = true
				}
			}
			authLog("[IAP] " + playerId + " owns " + s.Sku)
			continue
		}
		// Consumable: consume FIRST, credit only on success -- a failed consume stays owned and is retried next refresh.
		ok, err := metaS2S("consume_entitlement", metaUser, s.Sku)
		if err != nil || !ok {
			authLog(fmt.Sprintf("[IAP] consume %s for %s failed (ok=%v err=%v) -- not credited", s.Sku, playerId, ok, err))
			continue
		}
		purchaseMu.Lock()
		for _, c := range s.Credits {
			GrantItem(playerId, c.InGameId, currencyBalance(playerId, c.InGameId)+c.Change)
		}
		purchaseMu.Unlock()
		changed = true
		authLog("[IAP] " + playerId + " bought " + s.Sku + " (consumed, credited)")
	}
	return changed
}

// a2SkuForOffer maps a storefront offer to its Meta SKU ("" = not a real-money offer).
func a2SkuForOffer(key string) string {
	switch {
	case strings.HasPrefix(key, "DriftiumPacks:"):
		return "driftium_" + strings.TrimPrefix(key, "DriftiumPacks:")
	case strings.HasPrefix(key, "IAP:Driftium"):
		return "driftium_" + strings.TrimPrefix(key, "IAP:Driftium")
	case strings.HasPrefix(key, "HypercubePacks:cash"):
		return "hypercubes_" + strings.TrimPrefix(key, "HypercubePacks:cash")
	case key == "IAP:KlausBreakerBundle":
		return "bundle_klaus_breaker"
	}
	return ""
}
