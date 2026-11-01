// Solara extension: the 50451 ("pod-cosmetics") client storefront.
//
// GET /v1/storefront/client is parsed by 0x4DCE910 as {"Results": {"<display name>": offer display}}; each display (0x4DD7D00) is
// {name, offer_display_id, offers: [offer]} and each offer (0x4DDC260) is
//
//	{OfferId, PersonalizedCredits{}, PersonalizedDebits{}, RawCredits{}, RawDebits{}, PurchaseAllowed bool,
//	 DisplayIndex int, OfferName?, ExternalService?+ExternalEntitlementId? (both or neither)}
//
// where a credits/debits object (0x4DE5200) maps a key to {change, entitlement{entitlement_id, name,
// in_game_id, type}} (0x4DD9370); an empty object is valid.
//
// The bundle widget (WBP_IAP_Bundle) looks for the display named "IAP". It offers the catalogue's bundles,
// free (no debits), crediting the bundle entitlement. Every catalogue item is already granted by
// /v1/inventory/client, so a player owns them either way.
package mothership

import (
	"fmt"

	"solara/internal/stj"
)

func a2BundleIds() []string {
	out := []string{}
	for _, line := range splitLines(a2ItemsTsv) {
		if len(line) > 2 && line[0] != '#' && line[len(line)-2:] == "\t2" {
			out = append(out, line[:len(line)-2])
		}
	}
	return out
}

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			l := s[start:i]
			if len(l) > 0 && l[len(l)-1] == '\r' {
				l = l[:len(l)-1]
			}
			out = append(out, l)
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// a2Offer is one storefront offer: buying it credits Change of InGameId (entitlement type DURABLE / CURRENCY --
// the credit parser 0x4DD9370 accepts only the upper-case DURABLE, CONSUMABLE, CURRENCY; anything else fails
// the whole storefront).
type a2Offer struct {
	Key, InGameId, Type string
	Change              int
}

const a2DriftiumId = "HardCurrencyDriftium"

func a2Offers() []a2Offer {
	out := []a2Offer{}
	for _, id := range a2BundleIds() {
		out = append(out, a2Offer{Key: id, InGameId: id, Type: "DURABLE", Change: 1})
	}
	for _, n := range []int{500, 1200, 2500, 6500} {
		out = append(out, a2Offer{Key: fmt.Sprintf("Driftium%d", n), InGameId: a2DriftiumId, Type: "CURRENCY", Change: n})
	}
	return out
}

func a2FindOffer(offerId string) *a2Offer {
	for _, o := range a2Offers() {
		if stableId("offer:"+o.Key) == offerId {
			o := o
			return &o
		}
	}
	return nil
}

func a2Storefront(userId string) stj.Obj {
	offersList := []any{}
	for i, o := range a2Offers() {
		credit := stj.O(o.InGameId, stj.O(
			"change", o.Change,
			"entitlement", stj.O(
				"entitlement_id", stableId(userId+":"+o.InGameId),
				"name", o.InGameId,
				"in_game_id", o.InGameId,
				"type", o.Type,
			),
		))
		offersList = append(offersList, stj.O(
			"OfferId", stableId("offer:"+o.Key),
			"PersonalizedCredits", credit,
			"PersonalizedDebits", stj.O(),
			"RawCredits", credit,
			"RawDebits", stj.O(),
			"PurchaseAllowed", true,
			"DisplayIndex", i,
			"OfferName", o.Key,
		))
	}
	// root 0x4DCE910: Results is an OBJECT keyed by display name, each member an offer display (0x4DD7D00)
	return stj.O("IAP", stj.O(
		"name", "IAP",
		"offer_display_id", stableId("offer_display:IAP"),
		"offers", offersList,
	))
}
