# a2-50451-mothership-schemas

> 50451 client Mothership response contracts (RapidJSON parsers in IDA) + how the 609-item cosmetic catalogue was obtained
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

50451 client parses Mothership responses with RapidJSON in the MothershipApi SDK (0x4DC..-0x4DF.. range); a
missing REQUIRED field fails the whole response ("The response was malformed, no message or statusCode found").
Find parsers: astr.py (ASCII field name -> xref fns), schema.py (recursive field/type dump), vtf.py (request
class vtable: slot +0 builds the path, +3 parses the reply). Scripts in tools/a2port/work50451.

**IDA DB:** shard1 i64 CRASHED (internal error 1228 on decompiling 0x4DCB930) and is left unpacked -- use
shard2 (backup copy in shard3). Don't decompile 0x4DCB930.

Contracts (fixed in Solara 53d12f8 + 1078b9c, 2026-10-31):
- ROOT WRAPPERS DIFFER: inventory (0x4DCDF10) and storefront (0x4DCE910) need Results as an OBJECT map
  ({"Results":{"QUEST":{...}}} / {"Results":{"IAP":{...}}}); progression (0x4DCAB40) needs an ARRAY. Check
  roots with dis.py (cmp word [v+1Eh],3=object/4=array) -- decompiling them crashes IDA (error 1228).
- inventory entitlement 0x4DDBA90: entitlement_id, in_game_id, name, quantity(int) -- all required
- userdata GET 0x4DDE8A0: + metadata_id, created_by, last_written_by (required), generation uint
- storefront 0x4DCE910: {Results:[{name, offer_display_id, offers:[{OfferId, Personalized/Raw Credits/Debits
  (object map key->{change, entitlement{entitlement_id,name,in_game_id,type}}), PurchaseAllowed bool,
  DisplayIndex int, OfferName?, ExternalService+ExternalEntitlementId both-or-neither}]}]}; bundle widget wants
  display "IAP"
- purchase/client reply {Changes:{<in_game_id>:{entitlement_id,in_game_id,name,quantity}}} -- Changes is an OBJECT map
  (0x4DCF4E0); an array is "malformed" (fixed 2026-11-02). refresh-iap is GET -> {NewInventoryChangesAvailable}
- CURRENCY NAMES: kiosks (decompiled WBP_SharedIAPKiosk etc.) find balances by entitlement name == "Driftium" /
  "TechPoints" (Hypercubes) / "Joules", NOT in_game_id. Store displays the kiosks ask for: DriftiumPacks,
  HypercubePacks, Alacarte, IAP.
- REAL-MONEY IAP: offer with ExternalService "QUEST" + ExternalEntitlementId <sku> -> client Meta checkout
  (StartClientPurchase 0x56CF6A0) -> refresh-iap; Solara verifies/consumes via graph.oculus.com S2S
  (IAP_APP_ID/IAP_APP_SECRET in config/legacy-secrets.env). SKUs: driftium_1000/2200/5000/11000,
  hypercubes_200/440/1000/2200, bundle_klaus_breaker.
- Blueprint source: BlueprintToCpp (D:/projects/BlueprintToCpp/BlueprintToCpp/bin/Release/net8.0, config = 50451
  usmap) decompiles all BPs to .cpp under that folder (A2/Content/...). Read these before guessing BP behaviour.
- progression/client + progression-tree/client: Results lists (served empty; no original track/tree data)

Cosmetic ids: NOT in pak plaintext, but the running server has DT_ItemCatalogMetadata (RowStruct
A2CatalogItemMetadata, ItemCategory @0x88: 1 Cosmetic, 2 Bundle, 5 Currency). DLL [ITEMCAT] dumps it to
%TEMP%\a2_item_catalog.tsv (620 rows); embedded in Solara as internal/mothership/a2_50451_items.tsv.

Related: [a2-50451-christmas-port](a2-50451-christmas-port.md), [solara-migration](solara-migration.md)

## Tech trees (WORKING, user-confirmed 2026-11-02; commits b875676, 4b2a59d)
- GET /v1/progression-tree/client -> {"Results":[tree state]}; tree parser 0x4DD4620 FAILS the whole fetch ("Trees Fetch failed ... malformed") if `Track` key is missing ({} or null ok); node parser 0x4DE24A0 requires non-empty `id` (key string @0x148D80558), plus tree_id + name (name = DT reward row name, e.g. reward_baseball_bat). Tree object also has `id`.
- Unlock: game server POSTs <base>/<scope>/<tree>/node/unlock {node_id, mothership_player_id} -> Solara TreeNodeAction (internal/mothership/techtree50451.go): parent order, Hypercube cost Lv1 100/Lv2 250/Lv3 500, grants item.
- Data: a2_50451_techtree.tsv (77 nodes from /Game/A2/TechTree/Data). Client bridge op `tree` dumps UA2TechTreeSystem (+65 loaded, map +72, 328-B entries); `console ax.FetchProgressionTrees` refetches.
