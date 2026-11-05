# a2-50451-cosmetic-guard

> 2026-11-05. "All cosmetics" exploit and the server-side fix ([COSGUARD] in `Server/dllmain_50451.cpp`).

## The exploit (reported by a player who ran it)

A modded client calls, for every catalogue item it lacks (up to 573 at cosmetics-screen init):
`GetItemQuantityById` -> `AddItemToInventoryById` (local) -> `/Script/A2.VRPawn.Server_AddItemToInventoryById(FName)`
-> `ClientWriteInventoryPersistentDataThrottled`, then equips through the normal `ClientEquipItemById` flow.

- `Server_AddItemToInventoryById` grants nothing: its server implementation is an empty stub (vtable +2536 ->
  0x10F9940, see `[CURRENCY]` notes).
- The hole was `A2PlayerEntity::Server_SetEquippedItems(TArray<FName>)` (20 slots, `EA2ItemSlot`): it took any ids,
  the server replicated the look to everyone, and our `[COSMETIC]` persistence saved it to `player_inventory` and
  restored it on every join. Solara's `/v1/inventory/client` was already correct (owned items only, except the
  allowlisted accounts in `internal/mothership/inventory.go`).

## The fix

Every `Server_SetEquippedItems` (the client's, and our own restore, which re-enters `ProcessEvent`) is checked
before the server applies it. Allowed: free items, owned items (the player's own `/v1/inventory/client`, fetched as
them via `MothershipLogin`, cached 60 s, retried every 10 s on failure), and slots 18/19 (teams, set by the game).
Anything else becomes the slot default (0 ChassisRobotD, 3 PatternDefault, 4 EyesBlink, 6 HeartBallPatternDefault,
8 ColorPresetDefault, else None) in place. While the inventory is still loading, the list is stripped but never saved
and re-applied once ownership is known. Unowned `Server_AddItemToInventoryById` calls are dropped (they did nothing
anyway). `-NoCosGuard` disables it.

Free set = `DT_ItemOnlineBundleMetadata` rows `BundleDefaultInventory` (19) + `BundleDefaultEquippedItems` (7), from
the 50451 export (`Output/Exports/A2/Content/A2/Core/Systems/`). The game has no per-item "default" flag.

Local rig only: `%TEMP%\rbtest_equip.txt` = item id equips it (slot 9) on every player through the guarded path.
Logs: `[COSGUARD] ... tried to wear item(s) they do not own -> removed: ...`.
Limit: the exploiter still SEES the items on their own client; nobody else does, and nothing is saved.

## 2026-11-05 regression: guard stripped BOUGHT cosmetics for everyone (fixed in the backend, 718dd90)

Symptom: other players saw everyone in defaults; the log said `inventory http=200, 4 owned id(s)` (= only the
currencies) and `tried to wear item(s) they do not own -> removed: WearableChestCircuitSpryte` for players whose
backend log shows `[STORE] ... purchased Alacarte:WearableChestCircuitSpryte`.

Cause: `MothershipLogin(uid)` in the DLL signs in through `/v2/player/client/auth/complete/QUEST` with the player's
**Mothership player id** as the "Meta" UserId. That lands on a separate **server-login record** (ExternalService
QUEST, ExternalProviderId = the real PlayerId, its own PlayerId). The guard then read that record's (empty) inventory.

**That server-login record is ALSO the DLL's private store**: CosSaveWorker / CosLoadWorker save `player_inventory`
there. Do NOT "fix" the login to return the real player (a79b553 did, for 7 minutes): the DLL then loaded the
client's own `player_inventory` and saved over it (3 accounts, 23:41-23:48 UTC; the client re-saves on its next
look change). Fix kept: logins unchanged; only `GET /v1/inventory/client` from a trusted server maps a
server-login record to the real player (`internal/mothership/serverlogin.go`).

Verify (hub, server key from a file, never printed): begin+complete QUEST with the Mothership id, then GET
/v1/inventory/client with both the token and the server key -> must list the player's bought items, while the
login's PlayerId stays the server-login record. Players stripped before the fix get their look back when they
re-equip or rejoin (the guard cache refreshes within 60 s).
