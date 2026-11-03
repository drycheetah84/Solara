# SIGNAL // CYPHER — assets to export (FModel, A2 50451, UE 5.5)

Export settings: **meshes → glTF 2.0** (or UEFormat if you use the UEFormat Blender add-on), **textures → PNG**,
**animations → UEFormat/PSA**. Keep FModel's folder structure (`Exports/A2/Content/...`) so paths stay identical.

These exports are for building and placing things in the UE 5.5 project. Assets that already exist in the game do
NOT have to ship in our patch pak: the event level can reference the game's own copies by path. Only new content
(Cypher's finish, event props, new sequences) ships.

## Beat 1–2 · Gathering / EREBUS wakes (alarms, screens, crystal)
| Asset | Path | Why |
|---|---|---|
| Crystal shard L | `/Game/A2/Enviro/Sets/Parkour/Structures/Crystal/SM_PKR_Crystal_ShardLarge01a` | the green crystal (Quarry) surging |
| Crystal shard M | `/Game/A2/Enviro/Sets/Parkour/Structures/Crystal/SM_PKR_Crystal_ShardMed01a` | |
| Crystal materials | same folder: `MI_Parallax_CrystalEdge01a`, `MI_Parallax_CrystalEdge02a`, `MI_Parallax_CrystalEdgeMOVER01/02`, `MM_Parallax_CrystalEdge01a`, `T_Parallax_Crystal01a` | recolour to the canon green |
| Warning sign | `/Game/A2/Effects/Scraprun/Signs/WarningCollapse/SM_VFX_SCR_Sign_Collapse02a` (+ `MI_`/`T_` beside it) | `[EREBUS PROTOCOL ACTIVE]` signage |
| Spotlights | `/Game/A2/Effects/Scraprun/Exterior_Spotlights01/SM_SpotlightSCP01a`, `MI_VFX_SpotlightSCP01a`, `BP_VFX_EntranceSpotlightsSCP01` | searchlights sweeping the plaza |
| Holo plaque | `/Game/A2/Effects/GeneralFX/Holograms/Holo_Kiosk/SM_VFX_HoloPlaque01a`, `MI_VFX_HologramProjectorPlaque01a`, `MI_VFX_PlaqueTextProjection01a` | CODEX / Vector broadcast panels |
| Holo projector | `/Game/A2/Effects/GeneralFX/Holograms/CustomizationConsole/SM_HoloProjector_Base_Small_01`, `MI_VFX_CosmeticLargeProjector01a`, `T_VFX_CharacterGrad01a` | hologram of Vector / Cypher's record |

## Beat 3 · The jump (skybox transfer — the centrepiece)
Export the **whole folder** `/Game/A2/Effects/Station/SkyboxTransfer/01/`:
`SQ_VFX_SkyboxTransfer01a` (the sequence), `BP_VFX_SBT_LightingChange01a`, `BP_VFX_SBT_PlayerPortal01a`,
`BP_VFX_SBT_ShipPortal01a`, `SM_PortalEdge`, `SM_VFX_WallPlane01a`, `MM_BaseShip`, `MI_Baseship_DefaultMID`,
`MM_VFX_SkyboxTransferDF01a`, `MI_VFX_DF2_Stage1/2/3`, `MI__VFX_SBT_ThrusterBig_01a`, `MI_Portal*`, `MI_SBT_PortalStencil`,
`MI_Screen_Navigation01`, `MI_Skybox_Particle01a/02a/03a`, `T_EdgeMask`, `T_Portal01_Flipbook`.
**Also search FModel for `BaseShip`**: the ship mesh itself is not in our JSON dump (only its material is).
Plus the skybox/sky assets: search FModel for `Skybox`, `SkySphere`, `Nebula` under `/Game/A2/Enviro` and `/Game/A2/Maps/Station*`
— we need the current sky and something pink/purple for the "arrival" (canon: the Nebular Cradle).

## Beat 4 · The signal (Echo Bots, broadcast)
| Asset | Path | Why |
|---|---|---|
| **Echo Bot** | `/Game/A2/Characters/TransmissionEchoReceiver/SM_TransmissionEchoReceiverA`, `MI_TransmissionBot_Fade`, `Texture/SK_TransmissionEchoReceiverA_T_TransmissionEchoReceiverA_BaseColor`, `Texture/T_SK_TransmissionEchoReceiverA_EmissiveMask` | canon Echo Bots: the pre-event scavenger hunt + receivers lighting up |
| Echo receiver BP | `/Game/A2/Audio/BP_Echo_Receiver` | how the game already plays "echo" audio |
| Spectator bot | `/Game/A2/Characters/SpectatorBot/SM_SpectaterBot_Av02`, `MI_SpectatorBot_1a`, `Textures/T_SpectatorBotAv01_Alb_e`, `Textures/T_SpectatorBotAv01_E` | camera drones filming the arrival (Drift TV) |
| Tow bot | `/Game/A2/Characters/TowBot/SM_Towbot_lowpoly`, `MI_TowBot_1a`, `T_TowBot_BC`, `T_TowBot_E` | maintenance bots scrambling during the alarm |

## Beat 5 · Cypher returns
| Asset | Path | Why |
|---|---|---|
| **Old robot body** | `/Game/A2/Characters/A2RobotC/SK_A2RobotC` (+ `SK_A2RobotC_Skeleton`, `SK_A2RobotC_PhysicsAsset`) | an older series robot = Cypher, "old Series 8" |
| Its idle anim | `/Game/A2/Characters/A2RobotC/Animations/AS_A2RobotC_Idle` (+ `ABP_A2RobotC`) | base idle to layer my animation on |
| Its textures | `/Game/A2/Characters/A2RobotC/Textures/T_A2RobotC_ALB_E`, `T_A2RobotC_E`, `T_A2RobotC_MaskA`, `T_A2RobotC_MaskAtlasA`, **`T_A2RobotC_Numbers`**, **`T_A2RobotC_TeamLogosA`**, `Materials/MI_A2RobotC` | jersey **#17** + **Rust Runners** logo |
| Rust Runners gear | `/Game/A2/Characters/Cosmetics/Back/SM_COS_FoamHand_RustRunnersA`, `Materials/MI_COS_FoamHand_RustRunnersA`, `Textures/T_COS_FoamHand_RustRunners_ALB`, `Textures/T_COS_FoamHand_RustRunners_MaskA` | team colours/reference for Cypher's Finish |
| Paint masks | `/Game/A2/Characters/A2RobotD/Textures/T_A2RobotD_Mask_RebarRunnerA`, `T_A2RobotD_Mask_Boneyard`, **`T_A2RobotD_Mask_BlitzA`**, `T_A2RobotD_Mask_Default`, `/Game/A2/Characters/A2RobotD/Materials/` (all MI_A2RobotD_*) | base for the new "Cypher" finish; Blitz = his canon rival |
| Player robot | `/Game/A2/Characters/A2RobotD/SK_A2RobotD` (+ `_Skeleton` — needed for its **30 sockets**) | crowd/extra robots, cosmetics sockets |
| Ghost mesh | `/Game/A2/Characters/GhostMesh/SM_GhostMesh`, `M_GhostMesh`, `MI_GhostMesh` + its textures | Cypher materialising out of the signal (hologram → solid) |

## Beat 6–7 · Stand-down + finale
| Asset | Path | Why |
|---|---|---|
| **Spark** set | `/Game/A2/Characters/Cosmetics/SpecialSets/Spark/` — `SM_COS_SparkChestA`, `SM_COS_SparkForearmA`, `SM_COS_SparkHelmetA`, `SM_COS_SparkShoulderPadA` + `Materials/`, `Textures/` | canon commentator on stage |
| **Crankshaft** set | `/Game/A2/Characters/Cosmetics/SpecialSets/Crankshaft/` — `SM_COS_CrankshaftChestA`, `ClipboardA`, `ForearmA`, `HelmetA`, `MicA`, `ShoulderPadA` + `Materials/`, `Textures/` | his deadpan co-host |
| Fireworks | `/Game/A2/Effects/Station/Fireworks/BP_StationFireworks`, `SM_VFX_FirePlane01a`, `MI_FlipbookSpriteAlpha01a`, `T_VFX_FireworkSheet01a`, `/Game/A2/Effects/CTF/Celebration/MI_VFX_CTFFireworks01a` | finale (game also has `AA2StationFireworksActor`) |
| Lasers | `/Game/A2/Effects/Deathrun/FanFare/MI_VFX_FlipbookLaser01a`, `T_VFX_LaserSheet01a` | stage fanfare |
| Ship ramp / shuttle | `/Game/A2/Enviro/Props/LandingPad/SM_Ship_Ramp_01`, `/Game/A2/Enviro/Props/SpaceShips/SM_SpawnShuttle_01d_NoDoors`, `MI_SpawnShuttle`, `Textures/T_SpawnShuttle_01a_Color`, `T_SpawnShuttle_Emissive` | optional: Cypher arrives by shuttle instead of drifting in |

## Not in our dump — search FModel for these
- `BaseShip` mesh (beat 3).
- Driftball court / stadium screens: search `TKB_` or `Driftball` under `/Game/A2/Enviro/Sets` and `/Game/A2/Maps/Districts/TackleBall` (stage for beat 5–7).
- Plaza/hub where everyone gathers (the map we place the show in): `/Game/A2/Maps/Station_Prime` and its sublevels — export the **map** too (FModel can export a level's actor list) so I can position things.
- Station sky: `Skybox`/`Sky` meshes, materials and textures.

Send me the export folder path when it's done; I'll import into the UE 5.5 project and check every path matches.
