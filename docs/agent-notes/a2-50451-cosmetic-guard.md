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

## 2026-11-05 regression: guard stripped BOUGHT cosmetics for everyone (fixed in the backend, a79b553)

Symptom: other players saw everyone in defaults; the log said `inventory http=200, 4 owned id(s)` (= only the
currencies) and `tried to wear item(s) they do not own -> removed: WearableChestCircuitSpryte` for players whose
backend log shows `[STORE] ... purchased Alacarte:WearableChestCircuitSpryte`.

Cause: `MothershipLogin(uid)` in the DLL signs in through `/v2/player/client/auth/complete/QUEST` with the player's
**Mothership player id**. That endpoint treats `UserId` as a **Meta** user id, so it minted a new empty player
(and a junk V2 record with ExternalProviderId = the Mothership id) on every guard fetch. The token was for that
empty player -> no owned items. The same applies to CosSaveWorker / CosLoadWorker (`player_cosmetics` was being
saved under the junk players). Quests and currency pass the Meta org id and were not affected.

Fix (backend only, no game-server restart): `trustedlogin.go` -- a trusted-server QUEST login whose UserId is an
existing PlayerId gets that player's token; nothing is written. Verify with the server key on the hub: the
player's `/v1/inventory/client` must list their bought items. Junk records from before are left alone (never
matched now). Players stripped before the fix get their look back when they re-equip or rejoin (the guard cache
refreshes within 60 s).
