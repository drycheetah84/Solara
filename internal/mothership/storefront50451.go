// Solara extension: the 50451 ("pod-cosmetics") client storefront.
//
// GET /v1/storefront/client is parsed by 0x4DCE910 as {"Results": [offer display]}; each display (0x4DD7D00) is
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

import "solara/internal/stj"

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

func a2Storefront(userId string) []any {
	offersList := []any{}
	for i, id := range a2BundleIds() {
		credit := stj.O(id, stj.O(
			"change", 1,
			"entitlement", stj.O(
				"entitlement_id", stableId(userId+":"+id),
				"name", id,
				"in_game_id", id,
				"type", "Durable",
			),
		))
		offersList = append(offersList, stj.O(
			"OfferId", stableId("offer:"+id),
			"PersonalizedCredits", credit,
			"PersonalizedDebits", stj.O(),
			"RawCredits", credit,
			"RawDebits", stj.O(),
			"PurchaseAllowed", true,
			"DisplayIndex", i,
			"OfferName", id,
		))
	}
	return []any{stj.O(
		"name", "IAP",
		"offer_display_id", stableId("offer_display:IAP"),
		"offers", offersList,
	)}
}
