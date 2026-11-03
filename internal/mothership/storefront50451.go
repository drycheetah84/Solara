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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/stj"
)

//go:embed a2_50451_alacarte.txt
var a2AlacarteTxt string

//go:embed a2_50451_kiosk_items.tsv
var a2KioskItemsTsv string

//go:embed a2_50451_kiosk_exclude.tsv
var a2KioskExcludeTsv string

// a2KioskTables parses the fixed kiosk items (OfferDisplayIndex -> in_game_id) and the never-sold set.
func a2KioskTables() (map[int]string, map[string]bool) {
	fixed, excluded := map[int]string{}, map[string]bool{}
	for _, line := range splitLines(a2KioskItemsTsv) {
		if line == "" || line[0] == '#' {
			continue
		}
		if f := strings.Split(strings.TrimSpace(line), "\t"); len(f) == 2 {
			if n, err := strconv.Atoi(f[0]); err == nil {
				fixed[n] = f[1]
			}
		}
	}
	for _, line := range splitLines(a2KioskExcludeTsv) {
		if line == "" || line[0] == '#' {
			continue
		}
		if f := strings.Split(strings.TrimSpace(line), "\t"); f[0] != "" {
			excluded[f[0]] = true
		}
	}
	return fixed, excluded
}

type a2Credit struct {
	InGameId, Type string
	Change         int
}

type a2Offer struct {
	Key, Name, Display string
	Index              int
	Sku                string // Meta SKU for a real-money offer ("" = bought with Mothership / in-game currency)
	Credits, Debits    []a2Credit
}

const (
	a2DriftiumId       = "HardCurrencyDriftium"
	a2HypercubeId      = "HardCurrencyTechPoints"
	a2AlacartePrice    = 800
	a2AlacarteMaxIndex = 150 // the highest placed kiosk index is 149 (PlayerStart_Hangar)
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
	// DriftiumPacks: WBP_SharedATMKiosk / WBP_SharedIAPKiosk's Driftium ATM (decompiled: GetOfferByDisplayName("DriftiumPacks"),
	// picked by DisplayIndex 0..3). Real-money packs on the live service; free here.
	for i, n := range []int{1000, 2200, 5000, 11000} {
		out = append(out, a2Offer{Key: fmt.Sprintf("DriftiumPacks:%d", n), Name: fmt.Sprintf("%d Driftium", n), Display: "DriftiumPacks", Index: i,
			Credits: []a2Credit{{a2DriftiumId, "CURRENCY", n}}})
	}
	// HypercubePacks: WBP_SharedHypercubeATM (decompiled): index 0..3 are cash packs of 200/440/1000/2200 Hypercubes (free
	// here), index 4..7 trade Driftium for 200/400/800/1600 Hypercubes at 1000/2000/4000/8000 Driftium.
	for i, n := range []int{200, 440, 1000, 2200} {
		out = append(out, a2Offer{Key: fmt.Sprintf("HypercubePacks:cash%d", n), Name: fmt.Sprintf("%d Hypercubes", n), Display: "HypercubePacks", Index: i,
			Credits: []a2Credit{{a2HypercubeId, "CURRENCY", n}}})
	}
	for i, n := range []int{200, 400, 800, 1600} {
		out = append(out, a2Offer{Key: fmt.Sprintf("HypercubePacks:driftium%d", n), Name: fmt.Sprintf("%d Hypercubes", n), Display: "HypercubePacks", Index: 4 + i,
			Credits: []a2Credit{{a2HypercubeId, "CURRENCY", n}}, Debits: []a2Credit{{a2DriftiumId, "CURRENCY", n * 5}}})
	}
	// IAP 16: Klaus Breaker Bundle (the shop's bundle kiosk)
	// Real money: Meta SKU bundle_klaus_breaker (iap.go), so no Driftium debit.
	out = append(out, a2Offer{Key: "IAP:KlausBreakerBundle", Name: "Klaus Breaker Bundle", Display: "IAP", Index: 16,
		Credits: a2KlausBreakerItems()})
	// IAP: the real bundles of DT_ItemOnlineBundleMetadata (BundleFoundersPack is a catalogue row only -- no bundle row)
	idx := 20
	for _, id := range a2BundleIds() {
		if id == "BundleFoundersPack" {
			continue
		}
		out = append(out, a2Offer{Key: "IAP:" + id, Name: id, Display: "IAP", Index: idx, Credits: []a2Credit{{id, "DURABLE", 1}}})
		idx++
	}
	// Alacarte: every kiosk index 0..a2AlacarteMaxIndex sells one item. Kiosks whose display robot shows an item get
	// exactly that item (a2_50451_kiosk_items.tsv); the rest take the next item of the newest-first wearable list
	// (a2_50451_alacarte.txt) that is not excluded (a2_50451_kiosk_exclude.tsv: tech-tree / quest / starter / staff /
	// bundle / special items) and not already sold at another kiosk.
	fixed, excluded := a2KioskTables()
	used := map[string]bool{}
	for _, id := range fixed {
		used[id] = true
	}
	pool := []string{}
	for _, line := range splitLines(a2AlacarteTxt) {
		id := strings.TrimSpace(line)
		if id == "" || id[0] == '#' || excluded[id] || used[id] {
			continue
		}
		used[id] = true
		pool = append(pool, id)
	}
	next := 0
	for idx := 0; idx <= a2AlacarteMaxIndex; idx++ {
		if idx == a2ElfKioskIndex { // the CLB Christmas kiosk: one free Circuit Spryte piece, rotating every 2 days
			id := a2ElfPiece(time.Now())
			out = append(out, a2Offer{Key: "Alacarte:" + id, Name: id, Display: "Alacarte", Index: idx,
				Credits: []a2Credit{{id, "DURABLE", 1}}})
			continue
		}
		id, ok := fixed[idx]
		if !ok {
			if next >= len(pool) {
				continue
			}
			id = pool[next]
			next++
		}
		out = append(out, a2Offer{Key: "Alacarte:" + id, Name: id, Display: "Alacarte", Index: idx,
			Credits: []a2Credit{{id, "DURABLE", 1}}, Debits: []a2Credit{{a2DriftiumId, "CURRENCY", a2AlacartePrice}}})
	}
	for k := range out {
		out[k].Sku = a2SkuForOffer(out[k].Key)
	}
	return out
}

// The CLB Christmas kiosk (CLB_Christmas BP_SharedIAPKiosk_C_0, OfferDisplayIndex 42) stands beside the Circuit Spryte
// mannequin. It gives one piece of the set away free, a new piece every 2 days: helmet, chest, shoulders, arms, pattern,
// colour, badge, then around again. The rotation starts 2026-11-03 00:00 UTC; SOLARA_ELF_START (RFC 3339) moves it.
const a2ElfKioskIndex = 42

// The mannequin's full look (SM_A2RobotD_Mannequin_CircuitSpryte: Circuit Spryte helmet / chest / shoulder pads / forearms;
// body MI_A2RobotD_Mannequin_CircuitSpryte: Mask Index 60 = PatternPepperMatrix, green/red colours = ColorPresetPeppermatrix)
// plus the set's badge.
var a2ElfRotation = []string{"WearableHelmetCircuitSpryte", "WearableChestCircuitSpryte", "WearableShoulderCircuitSpryte",
	"WearableArmsCircuitSpryte", "PatternPepperMatrix", "ColorPresetPeppermatrix", "BadgeCircuitSpryte"}

func a2ElfPiece(now time.Time) string {
	start := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	if v := strings.TrimSpace(os.Getenv("SOLARA_ELF_START")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	n := 0
	if d := now.Sub(start); d > 0 {
		n = int(d / (48 * time.Hour))
	}
	return a2ElfRotation[n%len(a2ElfRotation)]
}

// a2KlausBreakerItems is the Klaus Breaker Bundle's contents (the shop bundle kiosk, IAP display index 16).
func a2KlausBreakerItems() []a2Credit {
	out := []a2Credit{}
	for _, id := range []string{"WearableHelmetKlausBreaker", "WearableShoulderKlausBreaker", "WearableArmsKlausBreaker",
		"WearableChestKlausBreaker", "EyesBlinkKlausBreaker", "ColorPresetKlausBreaker", "BadgeKlausBreaker"} {
		out = append(out, a2Credit{id, "DURABLE", 1})
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
	displays := []string{"IAP", "Alacarte", "DriftiumPacks", "HypercubePacks"}
	byDisplay := map[string][]any{}
	for _, o := range a2Offers() {
		credits, debits := a2CreditMap(userId, o.Credits), a2CreditMap(userId, o.Debits)
		offer := stj.O(
			"OfferId", stableId("offer:"+o.Key),
			"PersonalizedCredits", credits,
			"PersonalizedDebits", debits,
			"RawCredits", credits,
			"RawDebits", debits,
			"PurchaseAllowed", true,
			"DisplayIndex", o.Index,
			"OfferName", o.Name,
		)
		if o.Sku != "" {
			// Real money: the client buys this through the Quest Platform SDK checkout (iap.go). Both or neither.
			offer = append(offer, stj.KV{K: "ExternalService", V: a2IapService}, stj.KV{K: "ExternalEntitlementId", V: o.Sku})
		}
		byDisplay[o.Display] = append(byDisplay[o.Display], offer)
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
