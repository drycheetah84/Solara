// Solara extension: the 50451 ("pod-cosmetics") client storefront.
//
// GET /v1/storefront/client is parsed by 0x4DCE910 as {"Results": {"<display name>": offer display}}; each display
// (0x4DD7D00) is {name, offer_display_id, offers: [offer]} and each offer (0x4DDC260) is
//
//	{OfferId, PersonalizedCredits{}, PersonalizedDebits{}, RawCredits{}, RawDebits{}, PurchaseAllowed bool,
//	 DisplayIndex int, OfferName?, ExternalService?+ExternalEntitlementId? (both or neither)}
//
// where a credits/debits object (0x4DE5200) maps a key to {change, entitlement{entitlement_id, name, in_game_id,
// type}} (0x4DD9370; type must be the upper-case DURABLE / CONSUMABLE / CURRENCY or the whole store fails).
//
// Kiosks (pak export, /Game/UI/IAP + Maps/Shop/SHP_December2025):
//
//	BP_SharedIAPKiosk       OfferDisplayName "Alacarte", per-instance OfferDisplayIndex (7..132 in the shop)
//	BP_SharedIAPBundleKiosk TargetBackendCategoryName "IAP", TargetOfferDisplayIndex 16 ("Klaus Breaker Bundle")
//	WBP_SharedATMKiosk      Driftium packs 1000 / 2200 / 5000 / 11000
//
// The live service's per-index assignment is gone: Alacarte index i is the i-th newest wearable
// (a2_50451_alacarte.txt), priced in Driftium; IAP holds the Driftium packs (0-3), the Klaus Breaker bundle (16)
// and the default bundles.
package mothership

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"solara/internal/stj"
)

//go:embed a2_50451_alacarte.txt
var a2AlacarteTxt string

type a2Credit struct {
	InGameId, Type string
	Change         int
}

type a2Offer struct {
	Key, Name, Display string
	Index              int
	Credits, Debits    []a2Credit
}

const (
	a2DriftiumId       = "HardCurrencyDriftium"
	a2AlacartePrice    = 800
	a2AlacarteMaxIndex = 140
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
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.TrimRight(l, "\r"))
	}
	return out
}

var (
	a2OffersOnce sync.Once
	a2OffersList []a2Offer
)

func a2Offers() []a2Offer {
	a2OffersOnce.Do(func() { a2OffersList = buildA2Offers() })
	return a2OffersList
}

func buildA2Offers() []a2Offer {
	out := []a2Offer{}
	// IAP: Driftium packs (the ATM's four buttons)
	for i, n := range []int{1000, 2200, 5000, 11000} {
		out = append(out, a2Offer{Key: fmt.Sprintf("IAP:Driftium%d", n), Name: fmt.Sprintf("%d Driftium", n), Display: "IAP", Index: i,
			Credits: []a2Credit{{a2DriftiumId, "CURRENCY", n}}})
	}
	// IAP 16: Klaus Breaker Bundle (the shop's bundle kiosk)
	klaus := []a2Credit{}
	for _, id := range []string{"WearableHelmetKlausBreaker", "WearableShoulderKlausBreaker", "WearableArmsKlausBreaker",
		"WearableChestKlausBreaker", "EyesBlinkKlausBreaker", "ColorPresetKlausBreaker", "BadgeKlausBreaker"} {
		klaus = append(klaus, a2Credit{id, "DURABLE", 1})
	}
	out = append(out, a2Offer{Key: "IAP:KlausBreakerBundle", Name: "Klaus Breaker Bundle", Display: "IAP", Index: 16,
		Credits: klaus, Debits: []a2Credit{{a2DriftiumId, "CURRENCY", 2200}}})
	// IAP: the real bundles of DT_ItemOnlineBundleMetadata (BundleFoundersPack is a catalogue row only -- no bundle row)
	idx := 20
	for _, id := range a2BundleIds() {
		if id == "BundleFoundersPack" {
			continue
		}
		out = append(out, a2Offer{Key: "IAP:" + id, Name: id, Display: "IAP", Index: idx, Credits: []a2Credit{{id, "DURABLE", 1}}})
		idx++
	}
	// Alacarte: index i -> i-th newest wearable
	i := 0
	for _, line := range splitLines(a2AlacarteTxt) {
		id := strings.TrimSpace(line)
		if id == "" || id[0] == '#' {
			continue
		}
		if i > a2AlacarteMaxIndex {
			break
		}
		out = append(out, a2Offer{Key: "Alacarte:" + id, Name: id, Display: "Alacarte", Index: i,
			Credits: []a2Credit{{id, "DURABLE", 1}}, Debits: []a2Credit{{a2DriftiumId, "CURRENCY", a2AlacartePrice}}})
		i++
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

func a2CreditMap(userId string, cs []a2Credit) stj.Obj {
	m := stj.O()
	for _, c := range cs {
		m = append(m, stj.KV{K: c.InGameId, V: stj.O(
			"change", c.Change,
			"entitlement", stj.O(
				"entitlement_id", stableId(userId+":"+c.InGameId),
				"name", a2EntitlementName(c.InGameId),
				"in_game_id", c.InGameId,
				"type", c.Type,
			),
		)})
	}
	return m
}

func a2Storefront(userId string) stj.Obj {
	displays := []string{"IAP", "Alacarte"}
	byDisplay := map[string][]any{}
	for _, o := range a2Offers() {
		credits, debits := a2CreditMap(userId, o.Credits), a2CreditMap(userId, o.Debits)
		byDisplay[o.Display] = append(byDisplay[o.Display], stj.O(
			"OfferId", stableId("offer:"+o.Key),
			"PersonalizedCredits", credits,
			"PersonalizedDebits", debits,
			"RawCredits", credits,
			"RawDebits", debits,
			"PurchaseAllowed", true,
			"DisplayIndex", o.Index,
			"OfferName", o.Name,
		))
	}
	// root 0x4DCE910: Results is an OBJECT keyed by display name, each member an offer display (0x4DD7D00)
	res := stj.O()
	for _, d := range displays {
		offers := byDisplay[d]
		if offers == nil {
			offers = []any{}
		}
		res = append(res, stj.KV{K: d, V: stj.O(
			"name", d,
			"offer_display_id", stableId("offer_display:"+d),
			"offers", offers,
		)})
	}
	return res
}
