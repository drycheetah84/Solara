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
