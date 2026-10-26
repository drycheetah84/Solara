# Migration inventory (original Halcyon → Solara)

Status vocabulary: analyzed → implemented → compiled → unit-tested → integration-tested →
compatibility-tested (differential vs the original runtime) → fully verified; or blocked.
"Replaced" = an equivalent Go construct with demonstrated compatibility.

## Non-endpoint features

| ID | Original | Behaviour | Go | Status / evidence |
|---|---|---|---|---|
| HOST-001 | Hosting/Http/AstraHttpServer*.cs | listener loop, sequential processing, preprocessors, CORS, OPTIONS, routing regex, slash collapsing, 404/500 semantics | internal/hosting/server.go | compatibility-tested (all sweeps; hosting tests) |
| HOST-002 | Http/AstraHttpRequest.cs | headers (case-insensitive, comma-joined), queries (NameValueCollection semantics incl. null-key throw), JsonBody/FormBody, Remote (Cf-Connecting-Ip/X-Real-Ip) | internal/hosting/request.go | unit-tested + differential |
| HOST-003 | Http/AstraHttpResponse.cs, Actions/*.cs, AstraHttpActionResult.cs | result factory, content-type rules, 500 HTML document (byte-identical asset) | internal/hosting/results.go, assets/ | unit-tested + differential |
| HOST-004 | Http/Binding/BindableExtensions.cs | [FromQuery/Header/Form/Body] TypeConverter semantics; route Convert.ChangeType | internal/hosting/binding.go (+ per-handler calls) | unit-tested + differential |
| HOST-005 | Preprocessors/Default/HttpStaticFilesProcessor.cs | static files (no server registers it) | internal/hosting/static_files.go | implemented, compiled (unreachable in original) |
| HOST-006 | Preprocessors/Default/HttpStorageContainerProcessor.cs | storage containers (no server registers it), defects preserved | internal/hosting/storage_container.go | unit-tested (model JSON vs oracle) |
| HOST-007 | Hosting/ModuleInitialization.cs | Serilog setup, MOTHERSHIP_SELF_ELEVATE, IsElevated | internal/logging, internal/host/module_initialization.go, elevate_*.go | implemented; elevation path not exercised (needs UAC) |
| HOST-008 | Application/HostApplication.cs | ConfigureServices/AddServer/RunAsync, PopulateArguments | internal/host/host.go, internal/di | integration-tested (e2e start, 7 listeners) — PopulateArguments: replaced (no handler takes a service-typed parameter; DB services resolve to null per DI-001) |
| HOST-009 | WebSockets/AstraWebSocketServer.cs & co. | WebSocket server base class — **no subclass exists** | not ported (unreachable); WS handled via HandleWebSocketAsync servers (EOS) | **blocked on user authorization** to drop unreachable code (CLAUDE.md §3.1) |
| HOST-010 | SDK/ScopedReference.cs, TypeExtensions.cs, StringExtensions.cs | helpers | Go idioms; CreateRandomString → dotnet.CreateRandomString | replaced |
| DI-001 | DependencyInjection/*.cs | singleton container, eager init order, impl-object quirk | internal/di | integration-tested (host start order observed) |
| DB-001 | Database/AstraDatabase*.cs | LiteDB wrapper, collection naming, .meta sidecar (BinaryWriter) | internal/docdb | unit-tested + LiteDB fixture import + round-trip oracles (47 models) |
| DB-002 | LiteDB 5.0.21 semantics | _id order, ms dates (Local), null omission, EmptyStringToNull, TrimWhitespace, IgnoreCase collation in predicates | internal/docdb + per-model *string | compatibility-tested (oracle round-trips; difftest) |
| DB-003 | production.db / development.db data | existing records | tools/litedb-export + `solara import-litedb` | tested on real encrypted LiteDB fixture; not run on production data (not available here) |
| SOCKET-001 | HalcyonSocket/AgentHub.cs | TCP :9100 newline-JSON agent protocol | internal/halcyonsocket/agenthub.go | compatibility-tested (replay vs .NET oracle, fake agent round-trip) |
| SOCKET-002 | HalcyonSocket/ServerWatchdog.cs | liveness watchdog, restart/give-up, TakeDeploymentOffline hook | internal/halcyonsocket/watchdog.go, host.TakeDeploymentOffline | unit-tested (oracle line-by-line, injected clock) |
| SOCKET-003 | EOSSDK/StompHelper.cs | STOMP over WebSocket (v12.stomp) | internal/eos/stomp.go, websocket.go | unit-tested (end-to-end WS session) |
| SOCKET-004 | EOSSDK/LobbyWsHelper.cs | lobby WebSocket protocol, broadcasts | internal/eos/lobbyws.go | unit-tested (end-to-end WS session) |
| EOS-001 | EOSSDK/EosJwtFactory.cs | RS256 tokens, JWKS, PUID/OUID helpers | internal/eos/jwt.go | unit-tested (oracle fixtures, sign/verify) |
| EOS-002 | EOSSDK/EosObservabilityStore.cs | telemetry/WS client store | internal/eos/observability.go | unit-tested |
| EOS-003 | EOSSDK/RtcHelper.cs | RTC room tokens | internal/eos/rtc.go | unit-tested |
| EOS-004 | EOSSDK/Responses/*.json | embedded endpoint/matchmaking documents | internal/eos/responses (byte-identical, go:embed) | verified (byte compare) |
| EOS-005 | EOSSDK persistence | sessions/lobbies JSON next to binary | internal/eos/gateway.go | unit-tested (byte-equal indented output) |
| ARES-001 | Ares/Servers/NetvarDumpStore.cs, NetvarOverridesFile.cs, SpecLevelsStore.cs, StationAcl.cs, DrafterPermissions.cs | file-backed stores, ACL push, drafter grants | internal/ares/*.go | unit-tested; original Drafters suite 24/24 |
| ARES-002 | Ares/Servers/DashboardAuthPreprocessor.cs | dashboard auth (admin key, session cookie JWT) | internal/ares/dashboard_auth.go | compatibility-tested (sweeps, live socket check) |
| ARES-003 | Ares/Dashboard/index.html | embedded UI | internal/ares/assets/index.html (byte-identical) | verified |
| ARES-004 | FuzzySharp 2.0.2 ExtractTop | user search ranking | internal/ares/fuzzysharp.go | unit-tested (28 oracle cases) |
| MS-001 | MothershipServer Meta graph integration | attestation verify, nonce checks, device bans, 4s client | internal/mothership/meta.go | unit-tested against a fake Meta server; **live Meta API not exercised (no credentials/network contract test)** |
| CONFIG-001 | env vars, files, baked secrets | see docs/CONFIGURATION.md | same names; secrets externalised | implemented; check-secrets + e2e start |
| PROGRAM-001 | Ares/Program.cs | startup order, collection pre-registration, hooks | internal/host/host.go, cmd/solara | integration-tested (e2e run log shows original order) |
| TEST-001 | backend/tests/Drafters | 24 authorization checks | internal/ares/drafters_original_test.go | passing 24/24 |

## Original components intentionally not ported (external clients / tooling)

| Item | Reason |
|---|---|
| backend/dashboard (Next.js) | external client of the API; unchanged; its routes were checked against the Go route tables |
| windows/HalcyonA2.dll, *.ps1, allocator | game-server side (native payload + agent); external clients; wire contracts preserved |
| install.sh / install.ps1 / Redeploy-Vps.ps1 / deploy/*.sh | .NET deployment scripts; Solara deployment in docs/BUILD_AND_DEPLOY.md (scripts themselves not rewritten — see KNOWN_ISSUES DEPLOY-001) |

## HTTP endpoints (API-001 … API-282)

All rows: status **compatibility-tested** — each endpoint was replayed unauthenticated and with full credentials through the original (in-process oracle) and Solara with identical normalised results (VALIDATION_REPORT.md §3). Go location = file containing the registration.

| ID | Server | Method | Route | C# method | Go registration |
|---|---|---|---|---|---|
| API-001 | MothershipServer | GET | `/v1/userdata/client` | GetUserDataClient | internal/mothership/routes.go |
| API-002 | MothershipServer | POST | `/v1/userdata/client` | PostUserDataClient | internal/mothership/routes.go |
| API-003 | MothershipServer | DELETE | `/v1/userdata` | DeleteUserData | internal/mothership/routes.go |
| API-004 | MothershipServer | POST | `/v1/userdata/metadata/automation` | CreateUserDataMetadata | internal/mothership/routes.go |
| API-005 | MothershipServer | GET | `/v1/userdata/metadata/automation/list` | ListUserDataMetadata | internal/mothership/routes.go |
| API-006 | MothershipServer | POST | `/v1/title-data` | PostTitleData | internal/mothership/routes.go |
| API-007 | MothershipServer | GET | `/v1/title-data` | GetTitleData | internal/mothership/routes.go |
| API-008 | MothershipServer | DELETE | `/v1/title-data` | DeleteTitleData | internal/mothership/routes.go |
| API-009 | MothershipServer | GET | `/v1/title-data/client` | ClientGetTitleData | internal/mothership/routes.go |
| API-010 | MothershipServer | POST | `/v1/title-data/automation` | AutomationSetTitleData | internal/mothership/routes.go |
| API-011 | MothershipServer | GET | `/v1/title-data/automation` | AutomationGetTitleData | internal/mothership/routes.go |
| API-012 | MothershipServer | DELETE | `/v1/title-data/automation` | AutomationDeleteTitleData | internal/mothership/routes.go |
| API-013 | MothershipServer | POST | `/v1/offers/automation` | AutomationCreateOffer | internal/mothership/routes.go |
| API-014 | MothershipServer | GET | `/v1/offers/automation` | AutomationGetOffers | internal/mothership/routes.go |
| API-015 | MothershipServer | POST | `/v1/offerbindings/automation` | AutomationCreateOfferBinding | internal/mothership/routes.go |
| API-016 | MothershipServer | GET | `/v1/offerbindings/automation` | AutomationGetOfferBindings | internal/mothership/routes.go |
| API-017 | MothershipServer | POST | `/v1/offerbindings/automation/commit` | AutomationCommitOfferBinding | internal/mothership/routes.go |
| API-018 | MothershipServer | POST | `/v1/offerdisplays/automation` | AutomationCreateOfferDisplay | internal/mothership/routes.go |
| API-019 | MothershipServer | GET | `/v1/offerdisplays/automation` | AutomationGetOfferDisplays | internal/mothership/routes.go |
| API-020 | MothershipServer | POST | `/v1/purchase/client` | ClientPurchase | internal/mothership/routes.go |
| API-021 | MothershipServer | POST | `/v1/purchase/client/refresh-iap` | ClientRefreshIap | internal/mothership/routes.go |
| API-022 | MothershipServer | POST | `/v1/purchase` | ServerPurchase | internal/mothership/routes.go |
| API-023 | MothershipServer | POST | `/v1/entitlement-catalog/automation` | AutomationCreateEntitlement | internal/mothership/routes.go |
| API-024 | MothershipServer | GET | `/v1/entitlement-catalog/automation` | AutomationGetEntitlements | internal/mothership/routes.go |
| API-025 | MothershipServer | PUT | `/v1/entitlement-catalog/automation` | AutomationUpdateEntitlement | internal/mothership/routes.go |
| API-026 | MothershipServer | POST | `/v1/transaction-catalog/automation` | AutomationCreateTransaction | internal/mothership/routes.go |
| API-027 | MothershipServer | GET | `/v1/transaction-catalog/automation` | AutomationGetTransactions | internal/mothership/routes.go |
| API-028 | MothershipServer | GET | `/v1/transaction-catalog/automation/sunset` | AutomationTransactionSunset | internal/mothership/routes.go |
| API-029 | MothershipServer | GET | `/v1/inventory/client` | GetInventoryClient | internal/mothership/routes.go |
| API-030 | MothershipServer | GET | `/v1/inventory` | GetInventory | internal/mothership/routes.go |
| API-031 | MothershipServer | POST | `/v1/inventory` | AddInventoryItem | internal/mothership/routes.go |
| API-032 | MothershipServer | GET | `/v1/storefront/client` | ClientGetStorefront | internal/mothership/routes.go |
| API-033 | MothershipServer | POST | `/v1/progression` | CreateProgression | internal/mothership/routes.go |
| API-034 | MothershipServer | GET | `/v1/progression` | GetProgression | internal/mothership/routes.go |
| API-035 | MothershipServer | DELETE | `/v1/progression` | DeleteProgression | internal/mothership/routes.go |
| API-036 | MothershipServer | POST | `/v1/progression/automation` | AutomationCreateProgressionTrack | internal/mothership/routes.go |
| API-037 | MothershipServer | GET | `/v1/progression/automation` | AutomationGetProgressionTracks | internal/mothership/routes.go |
| API-038 | MothershipServer | DELETE | `/v1/progression/automation` | AutomationDeleteProgressionTrack | internal/mothership/routes.go |
| API-039 | MothershipServer | POST | `/v1/progression/automation/bindings` | AutomationCreateProgressionBinding | internal/mothership/routes.go |
| API-040 | MothershipServer | GET | `/v1/progression/automation/bindings` | AutomationGetProgressionBindings | internal/mothership/routes.go |
| API-041 | MothershipServer | POST | `/v1/progression/server` | ServerProgression | internal/mothership/routes.go |
| API-042 | MothershipServer | POST | `/v1/progression-tree` | CreateProgressionTree | internal/mothership/routes.go |
| API-043 | MothershipServer | GET | `/v1/progression-tree` | GetProgressionTree | internal/mothership/routes.go |
| API-044 | MothershipServer | DELETE | `/v1/progression-tree` | DeleteProgressionTree | internal/mothership/routes.go |
| API-045 | MothershipServer | POST | `/v1/progression-tree/automation` | AutomationCreateProgressionTree | internal/mothership/routes.go |
| API-046 | MothershipServer | GET | `/v1/progression-tree/automation` | AutomationGetProgressionTrees | internal/mothership/routes.go |
| API-047 | MothershipServer | DELETE | `/v1/progression-tree/automation` | AutomationDeleteProgressionTree | internal/mothership/routes.go |
| API-048 | MothershipServer | POST | `/v1/progression-tree/automation/bindings` | AutomationCreateTreeBinding | internal/mothership/routes.go |
| API-049 | MothershipServer | GET | `/v1/progression-tree/automation/bindings` | AutomationGetTreeBindings | internal/mothership/routes.go |
| API-050 | MothershipServer | DELETE | `/v1/progression-tree/automation/bindings` | AutomationDeleteTreeBinding | internal/mothership/routes.go |
| API-051 | MothershipServer | POST | `/v1/{platform}/analytics/event/batch` | PlatformAnalyticsBatch | internal/mothership/routes.go |
| API-052 | MothershipServer | GET | `/v1/permissions/client` | ClientPermissions | internal/mothership/routes.go |
| API-053 | MothershipServer | GET | `/v1/permissions/server` | ServerPermissions | internal/mothership/routes.go |
| API-054 | MothershipServer | POST | `/v1/server/automation/api_key` | CreateApiKey | internal/mothership/routes.go |
| API-055 | MothershipServer | GET | `/v1/server/automation/api_key` | GetApiKeys | internal/mothership/routes.go |
| API-056 | MothershipServer | POST | `/v1/notifications/server/send` | ServerSendNotification | internal/mothership/routes.go |
| API-057 | MothershipServer | GET | `/` | Index | internal/mothership/routes.go |
| API-058 | MothershipServer | GET | `/health` | Health | internal/mothership/routes.go |
| API-059 | MothershipServer | GET | `/debug/storage` | DebugStorage | internal/mothership/routes.go |
| API-060 | MothershipServer | GET | `/storage/{key}` | GetStorage | internal/mothership/routes.go |
| API-061 | MothershipServer | POST | `/storage/{key}` | CreateStorageItem | internal/mothership/routes.go |
| API-062 | MothershipServer | POST | `/api/GetFriendsV2` | GetFriendsV2 | internal/mothership/routes.go |
| API-063 | MothershipServer | POST | `/api/RequestFriend` | RequestFriend | internal/mothership/routes.go |
| API-064 | MothershipServer | POST | `/api/RemoveFriend` | RemoveFriend | internal/mothership/routes.go |
| API-065 | MothershipServer | POST | `/api/SetPrivacyState` | SetPrivacyState | internal/mothership/routes.go |
| API-066 | MothershipServer | POST | `/v1/client/player/auth/Insecure_1` | ClientAuthInsecure1 | internal/mothership/routes.go |
| API-067 | MothershipServer | POST | `/v1/client/player/auth/Insecure_2` | ClientAuthInsecure2 | internal/mothership/routes.go |
| API-068 | MothershipServer | POST | `/v1/client/player/auth/GOOGLE` | ClientAuthGoogle | internal/mothership/routes.go |
| API-069 | MothershipServer | POST | `/v1/client/player/auth/APPLE` | ClientAuthApple | internal/mothership/routes.go |
| API-070 | MothershipServer | POST | `/v1/client/player/auth/QUEST` | ClientAuthQuest | internal/mothership/routes.go |
| API-071 | MothershipServer | POST | `/v1/client/player/auth/RIFT` | ClientAuthRift | internal/mothership/routes.go |
| API-072 | MothershipServer | GET | `/v1/server/authorized` | ServerAuthorized | internal/mothership/routes.go |
| API-073 | MothershipServer | POST | `/v1/client/analytics/event/batch` | AnalyticsBatch | internal/mothership/routes.go |
| API-074 | MothershipServer | POST | `/v2/player/client/auth/begin/QUEST` | BeginQuestAuth | internal/mothership/routes.go |
| API-075 | MothershipServer | POST | `/v2/player/client/auth/complete/QUEST` | CompleteQuestAuth | internal/mothership/routes.go |
| API-076 | MothershipServer | GET | `/v2/player/client/auth/begin/STEAM` | BeginSteamAuth | internal/mothership/routes.go |
| API-077 | MothershipServer | POST | `/v2/player/client/auth/complete/STEAM` | CompleteSteamAuth | internal/mothership/routes.go |
| API-078 | MothershipServer | POST | `/v1/server/player/auth/verify_token` | VerifyToken | internal/mothership/routes.go |
| API-079 | MothershipServer | POST | `/v1/server/player/auth/association` | CreateAssociation | internal/mothership/routes.go |
| API-080 | MothershipServer | GET | `/v1/server/player/auth/associations/{player_id}` | GetAssociations | internal/mothership/routes.go |
| API-081 | MothershipServer | DELETE | `/v1/server/player/auth/associations/{player_id}` | DeleteAssociations | internal/mothership/routes.go |
| API-082 | MothershipServer | GET | `/v1/automation/player/auth/players` | AutomationGetPlayers | internal/mothership/routes.go |
| API-083 | MothershipServer | POST | `/v1/automation/player/auth/players` | AutomationValidatePlayer | internal/mothership/routes.go |
| API-084 | MothershipServer | POST | `/v1/automation/player/auth/players/tags` | AutomationUpdatePlayerTags | internal/mothership/routes.go |
| API-085 | MothershipServer | POST | `/v1/automation/player/auth/links` | AutomationCreateLink | internal/mothership/routes.go |
| API-086 | MothershipServer | POST | `/v1/automation/player/auth/links/set_primary` | AutomationSetPrimaryLink | internal/mothership/routes.go |
| API-087 | MothershipServer | GET | `/v1/automation/player/auth/links/{platform}` | AutomationGetLinks | internal/mothership/routes.go |
| API-088 | MothershipServer | DELETE | `/v1/automation/player/auth/links/{platform}/{link_id}` | AutomationDeleteLink | internal/mothership/routes.go |
| API-089 | MothershipServer | GET | `/v1/automation/player/auth/player/all_associations` | AutomationAllAssociations | internal/mothership/routes.go |
| API-090 | MothershipServer | DELETE | `/v1/automation/player/auth/associations/{platform}/{assoc_id}` | AutomationDeleteAssociation | internal/mothership/routes.go |
| API-091 | MothershipServer | POST | `/v1/{platform}/player/auth/explicit_link` | ExplicitLink | internal/mothership/routes.go |
| API-092 | MothershipServer | POST | `/v1/moderation/client/report` | ClientCreateReport | internal/mothership/routes.go |
| API-093 | MothershipServer | POST | `/v1/moderation/server/report` | ServerCreateReport | internal/mothership/routes.go |
| API-094 | MothershipServer | POST | `/v1/moderation/server/ban` | ServerCreateBan | internal/mothership/routes.go |
| API-095 | MothershipServer | POST | `/v1/moderation/server/bans/bulk` | ServerBulkBans | internal/mothership/routes.go |
| API-096 | MothershipServer | GET | `/v1/moderation` | ListModeration | internal/mothership/routes.go |
| API-097 | MothershipServer | GET | `/v1/moderation/automation` | GetModerationAutomation | internal/mothership/routes.go |
| API-098 | MothershipServer | POST | `/v1/moderation/automation` | PostModerationAutomation | internal/mothership/routes.go |
| API-099 | MothershipServer | DELETE | `/v1/moderation/automation` | DeleteModerationAutomation | internal/mothership/routes.go |
| API-100 | MothershipServer | POST | `/v1/server/shared-group` | ServerSharedGroup | internal/mothership/routes.go |
| API-101 | MothershipServer | GET | `/v1/shared-group/{group_id}` | GetSharedGroup | internal/mothership/routes.go |
| API-102 | MothershipServer | POST | `/v1/shared-group/{group_id}` | UpdateSharedGroup | internal/mothership/routes.go |
| API-103 | MothershipServer | DELETE | `/v1/shared-group/{group_id}` | DeleteSharedGroup | internal/mothership/routes.go |
| API-104 | MothershipServer | GET | `/v1/shared-group/{group_id}/data` | GetSharedGroupData | internal/mothership/routes.go |
| API-105 | MothershipServer | POST | `/v1/shared-group/{group_id}/members` | AddSharedGroupMembers | internal/mothership/routes.go |
| API-106 | MothershipServer | DELETE | `/v1/shared-group/{group_id}/members` | RemoveSharedGroupMembers | internal/mothership/routes.go |
| API-107 | MothershipServer | POST | `/v1/server/player/auth/bulk_player_lookup` | BulkPlayerLookup | internal/mothership/routes.go |
| API-108 | MothershipServer | POST | `/v1/server/player/auth/bulk_link_lookup` | BulkLinkLookup | internal/mothership/routes.go |
| API-109 | MothershipServer | POST | `/v1/title/automation` | AutomationCreateTitle | internal/mothership/routes.go |
| API-110 | MothershipServer | GET | `/v1/title/automation` | AutomationGetTitles | internal/mothership/routes.go |
| API-111 | MothershipServer | POST | `/v1/env/automation` | AutomationCreateEnvironment | internal/mothership/routes.go |
| API-112 | MothershipServer | GET | `/v1/env/automation` | AutomationGetEnvironments | internal/mothership/routes.go |
| API-113 | MothershipServer | POST | `/v1/deployment/automation` | AutomationCreateDeployment | internal/mothership/routes.go |
| API-114 | MothershipServer | GET | `/v1/deployment/automation` | AutomationGetDeployments | internal/mothership/routes.go |
| API-115 | A2StationDbServer | GET | `/v1/region` | GetRegion | internal/ares/a2station_server.go |
| API-116 | A2StationDbServer | POST | `/v2/users/log_in` | UserPlayerLoginV2 | internal/ares/a2station_server.go |
| API-117 | A2StationDbServer | POST | `/v1/users/log_in_with_key` | LogInWithKeyV1 | internal/ares/a2station_server.go |
| API-118 | A2StationDbServer | POST | `/stations/{station_id}/deployments` | CreateDeployment | internal/ares/a2station_server.go |
| API-119 | A2StationDbServer | GET | `/stations/{station_id}/deployments` | GetStationDeploymentsV0 | internal/ares/a2station_server.go |
| API-120 | A2StationDbServer | GET | `/v1/stations/{station_id}/deployments` | GetStationDeploymentsV1 | internal/ares/a2station_server.go |
| API-121 | A2StationDbServer | POST | `/deployments/{deployment_id}/api_key` | CreateDeploymentApiKey | internal/ares/a2station_server.go |
| API-122 | A2StationDbServer | POST | `/deployments/{deployment_id}/server_events` | AddServerEvent | internal/ares/a2station_server.go |
| API-123 | A2StationDbServer | PATCH | `/deployments/{deployment_id}` | UpdateDeployment | internal/ares/a2station_server.go |
| API-124 | A2StationDbServer | DELETE | `/deployments/{deployment_id}` | DeleteDeployment | internal/ares/a2station_server.go |
| API-125 | A2StationDbServer | GET | `/deployments/{deployment_id}/config` | GetDeploymentConfigV0 | internal/ares/a2station_server.go |
| API-126 | A2StationDbServer | GET | `/v1/deployments/{deployment_id}` | GetDeploymentV1 | internal/ares/a2station_server.go |
| API-127 | A2StationDbServer | GET | `/v1/deployments/{deployment_id}/netvar_overrides` | GetNetvarOverrides | internal/ares/a2station_server.go |
| API-128 | A2StationDbServer | GET | `/v1/spec/levels` | SpecListLevels | internal/ares/a2station_server.go |
| API-129 | A2StationDbServer | GET | `/v1/spec/levels/{name}` | SpecGetLevel | internal/ares/a2station_server.go |
| API-130 | A2StationDbServer | PUT | `/v1/spec/levels/{name}` | SpecPutLevel | internal/ares/a2station_server.go |
| API-131 | A2StationDbServer | POST | `/v1/spec/levels/{name}/loaded` | SpecSetLoaded | internal/ares/a2station_server.go |
| API-132 | A2StationDbServer | POST | `/v1/spec/boot` | SpecBoot | internal/ares/a2station_server.go |
| API-133 | A2StationDbServer | GET | `/v1/spec/desired` | SpecDesired | internal/ares/a2station_server.go |
| API-134 | A2StationDbServer | POST | `/v1/spec/status` | SpecStatus | internal/ares/a2station_server.go |
| API-135 | A2StationDbServer | GET | `/v1/deployments/{deployment_id}/config` | GetDeploymentConfigV1 | internal/ares/a2station_server.go |
| API-136 | A2StationDbServer | POST | `/v1/deployments/{deployment_id}/config` | SetDeploymentConfigV1 | internal/ares/a2station_server.go |
| API-137 | A2StationDbServer | DELETE | `/v1/deployments/{deployment_id}/config` | DeleteDeploymentConfigV1 | internal/ares/a2station_server.go |
| API-138 | A2StationDbServer | GET | `/v1/deployments/{deployment_id}/server_events` | GetDeploymentServerEvents | internal/ares/a2station_server.go |
| API-139 | A2StationDbServer | GET | `/v1/spec/drafters/{deployment_id}` | ServerDrafters | internal/ares/a2station_server.go |
| API-140 | A2StationDbServer | POST | `/register_server` | RegisterServer | internal/ares/a2station_server.go |
| API-141 | A2StationDbServer | POST | `/update_player_count` | UpdatePlayerCount | internal/ares/a2station_server.go |
| API-142 | A2StationDbServer | POST | `/server_heartbeat` | ServerHeartbeat | internal/ares/a2station_server.go |
| API-143 | A2StationDbServer | POST | `/v1/server/admin/purge_stations` | PurgeStations | internal/ares/a2station_server.go |
| API-144 | A2StationDbServer | POST | `/v1/server/admin/prune_sessions` | PruneSessions | internal/ares/a2station_server.go |
| API-145 | A2StationDbServer | GET | `/users` | ListAllUsersV0 | internal/ares/a2station_server.go |
| API-146 | A2StationDbServer | POST | `/users` | CreateOrUpdateUser | internal/ares/a2station_server.go |
| API-147 | A2StationDbServer | GET | `/v1/users` | ListAllUsersV1 | internal/ares/a2station_server.go |
| API-148 | A2StationDbServer | DELETE | `/users/{user_id}` | DeleteUser | internal/ares/a2station_server.go |
| API-149 | A2StationDbServer | GET | `/users/{user_id}` | GetUserV0 | internal/ares/a2station_server.go |
| API-150 | A2StationDbServer | GET | `/v1/users/{user_id}` | GetUserV1 | internal/ares/a2station_server.go |
| API-151 | A2StationDbServer | POST | `/users/log_in_server` | UserServerLogin | internal/ares/a2station_server.go |
| API-152 | A2StationDbServer | POST | `/users/log_in_with_key` | LogInWithKey | internal/ares/a2station_server.go |
| API-153 | A2StationDbServer | POST | `/users/log_in` | UserPlayerLogin | internal/ares/a2station_server.go |
| API-154 | A2StationDbServer | POST | `/users/{user_id}/api_key` | CreateUserApiKey | internal/ares/a2station_server.go |
| API-155 | A2StationDbServer | POST | `/stations/create` | CreateStation | internal/ares/a2station_server.go |
| API-156 | A2StationDbServer | GET | `/stations` | FetchAllStationsV0 | internal/ares/a2station_server.go |
| API-157 | A2StationDbServer | GET | `/v1/stations` | FetchAllStationsV1 | internal/ares/a2station_server.go |
| API-158 | A2StationDbServer | GET | `/stations/{station_id}` | GetStation | internal/ares/a2station_server.go |
| API-159 | A2StationDbServer | GET | `/stations/player_count` | GetPlayerCount | internal/ares/a2station_server.go |
| API-160 | A2StationDbServer | PATCH | `/stations/{station_id}` | UpdateStation | internal/ares/a2station_server.go |
| API-161 | A2StationDbServer | DELETE | `/stations/{station_id}` | DeleteStation | internal/ares/a2station_server.go |
| API-162 | A2StationDbServer | POST | `/v1/board/upload` | UploadBoardImage | internal/ares/a2station_server.go |
| API-163 | A2StationDbServer | GET | `/board/{file}` | GetBoardImage | internal/ares/a2station_server.go |
| API-164 | A2StationDbServer | GET | `/v1/board/keys` | GetBoardKeys | internal/ares/a2station_server.go |
| API-165 | A2StationDbServer | GET | `/stations/{station_id}/config` | GetStationConfig | internal/ares/a2station_server.go |
| API-166 | A2StationDbServer | POST | `/stations/{station_id}/config` | SetStationConfig | internal/ares/a2station_server.go |
| API-167 | A2StationDbServer | DELETE | `/stations/{station_id}/config` | DeleteStationConfig | internal/ares/a2station_server.go |
| API-168 | A2StationDbServer | GET | `/stations/{station_id}/online` | GetStationOnline | internal/ares/a2station_server.go |
| API-169 | A2StationDbServer | GET | `/stations/{station_id}/users` | GetAllUsersInStationV0 | internal/ares/a2station_server.go |
| API-170 | A2StationDbServer | GET | `/v1/stations/{station_id}/users` | GetAllUsersInStationV1 | internal/ares/a2station_server.go |
| API-171 | A2StationDbServer | GET | `/stations/{station_id}/users/{user_id}/roles` | GetStationUserRoles | internal/ares/a2station_server.go |
| API-172 | A2StationDbServer | POST | `/stations/{station_id}/users/{user_id}/roles/{role_id}` | AddStationUserRole | internal/ares/a2station_server.go |
| API-173 | A2StationDbServer | POST | `/stations/{station_id}/users/{user_id}/grant/{role_name}` | GrantStationUserRoleByName | internal/ares/a2station_server.go |
| API-174 | A2StationDbServer | DELETE | `/stations/{station_id}/users/{user_id}/role/{role_id}` | DeleteStationUserRole | internal/ares/a2station_server.go |
| API-175 | A2StationDbServer | POST | `/v1/stations/{station_id}/user_roles` | AddStationUserRoleByName | internal/ares/a2station_server.go |
| API-176 | A2StationDbServer | GET | `/stations/{station_id}/roles` | GetStationRoles | internal/ares/a2station_server.go |
| API-177 | A2StationDbServer | POST | `/stations/{station_id}/roles` | CreateStationRole | internal/ares/a2station_server.go |
| API-178 | A2StationDbServer | DELETE | `/stations/{station_id}/roles/{role_id}` | DeleteStationRole | internal/ares/a2station_server.go |
| API-179 | A2StationDbServer | PATCH | `/stations/{station_id}/roles/{role_id}` | UpdateStationRole | internal/ares/a2station_server.go |
| API-180 | A2StationDbServer | PATCH | `/stations/{station_id}/roles/{role_id}/permissions` | AssignPerms | internal/ares/a2station_server.go |
| API-181 | A2StationDbServer | GET | `/v1/stations/{station_id}/roles/{role_id}/users` | GetRoleUsers | internal/ares/a2station_server.go |
| API-182 | A2StationDbServer | GET | `/stations/{station_id}/server_events` | GetServerEventsV0 | internal/ares/a2station_server.go |
| API-183 | A2StationDbServer | GET | `/v1/stations/{station_id}/server_events` | GetServerEventsV1 | internal/ares/a2station_server.go |
| API-184 | A2StationDbServer | POST | `/v1/stations/{station_id}/users/{user_id}/ban` | BanStationUser | internal/ares/a2station_server.go |
| API-185 | A2StationDbServer | PATCH | `/v1/stations/{station_id}/users/{user_id}/unban` | UnbanStationUser | internal/ares/a2station_server.go |
| API-186 | A2StationDbServer | GET | `/v1/stations/{station_id}/bans` | GetStationBans | internal/ares/a2station_server.go |
| API-187 | A2StationDbServer | POST | `/v1/stations/{station_id}/event` | CreateStationEvent | internal/ares/a2station_server.go |
| API-188 | A2StationDbServer | GET | `/v1/stations/{station_id}/events` | GetStationEvents | internal/ares/a2station_server.go |
| API-189 | A2StationDbServer | GET | `/v1/events/{event_id}` | GetStationEvent | internal/ares/a2station_server.go |
| API-190 | A2StationDbServer | DELETE | `/v1/stations/{station_id}/event/{event_id}` | DeleteStationEvent | internal/ares/a2station_server.go |
| API-191 | A2StationDbServer | PATCH | `/v1/stations/{station_id}/event/{event_id}` | UpdateStationEvent | internal/ares/a2station_server.go |
| API-192 | A2StationDbServer | POST | `/v1/stations/{station_id}/event/{event_id}/users/{user_id}/signup` | SignupStationEvent | internal/ares/a2station_server.go |
| API-193 | A2StationDbServer | GET | `/v1/stations/{station_id}/event/{event_id}/signups` | GetStationEventSignups | internal/ares/a2station_server.go |
| API-194 | A2StationDbServer | DELETE | `/v1/stations/{station_id}/event/{event_id}/signup` | DeleteStationEventSignup | internal/ares/a2station_server.go |
| API-195 | A2StationDbServer | GET | `/v1/stations/{station_id}/event/{event_id}/config` | GetStationEventConfig | internal/ares/a2station_server.go |
| API-196 | A2StationDbServer | POST | `/v1/stations/{station_id}/event/{event_id}/config` | SetStationEventConfig | internal/ares/a2station_server.go |
| API-197 | A2StationDbServer | DELETE | `/v1/stations/{station_id}/event/{event_id}/config` | DeleteStationEventConfig | internal/ares/a2station_server.go |
| API-198 | A2StationDbServer | GET | `/v2/fleets` | FetchAllFleetsV2 | internal/ares/a2station_server.go |
| API-199 | A2StationDbServer | GET | `/v2/fleets/{fleet_id}` | GetFleetV2 | internal/ares/a2station_server.go |
| API-200 | A2StationDbServer | GET | `/v2/stations` | FetchAllStationsV2 | internal/ares/a2station_server.go |
| API-201 | AresDashboardServer | GET | `/` | Index | internal/ares/dashboard_server.go |
| API-202 | AresDashboardServer | GET | `/auth/login` | AuthLogin | internal/ares/dashboard_server.go |
| API-203 | AresDashboardServer | GET | `/auth/meta` | AuthMetaCallback | internal/ares/dashboard_server.go |
| API-204 | AresDashboardServer | GET | `/api/stats` | GetStats | internal/ares/dashboard_server.go |
| API-205 | AresDashboardServer | GET | `/api/users` | GetUsers | internal/ares/dashboard_server.go |
| API-206 | AresDashboardServer | POST | `/api/users/{user_id}/grant_admin` | GrantAdmin | internal/ares/dashboard_server.go |
| API-207 | AresDashboardServer | DELETE | `/api/users/{user_id}/grant_admin` | RevokeAdmin | internal/ares/dashboard_server.go |
| API-208 | AresDashboardServer | POST | `/api/stations/{station_id}/board` | UploadStationBoard | internal/ares/dashboard_server.go |
| API-209 | AresDashboardServer | GET | `/api/board/keys` | BoardKeys | internal/ares/dashboard_server.go |
| API-210 | AresDashboardServer | GET | `/api/stations/{station_id}/board` | GetStationBoard | internal/ares/dashboard_server.go |
| API-211 | AresDashboardServer | DELETE | `/api/stations/{station_id}/board` | ClearStationBoard | internal/ares/dashboard_server.go |
| API-212 | AresDashboardServer | GET | `/api/netvars/dumps` | ListNetvarDumps | internal/ares/dashboard_server.go |
| API-213 | AresDashboardServer | GET | `/api/netvars/dump` | GetNetvarDump | internal/ares/dashboard_server.go |
| API-214 | AresDashboardServer | GET | `/api/stations/{station_id}/config` | GetStationConfig | internal/ares/a2station_server.go |
| API-215 | AresDashboardServer | PATCH | `/api/stations/{station_id}/config` | PatchStationConfig | internal/ares/dashboard_server.go |
| API-216 | AresDashboardServer | GET | `/api/spec/levels` | GetSpecLevels | internal/ares/dashboard_server.go |
| API-217 | AresDashboardServer | GET | `/api/spec/levels/{name}` | GetSpecLevel | internal/ares/dashboard_server.go |
| API-218 | AresDashboardServer | PATCH | `/api/spec/levels/{name}` | PatchSpecLevel | internal/ares/dashboard_server.go |
| API-219 | AresDashboardServer | DELETE | `/api/spec/levels/{name}` | DeleteSpecLevel | internal/ares/dashboard_server.go |
| API-220 | AresDashboardServer | GET | `/api/stations/{station_id}/whitelist` | GetStationWhitelist | internal/ares/dashboard_server.go |
| API-221 | AresDashboardServer | PUT | `/api/stations/{station_id}/whitelist` | PutStationWhitelist | internal/ares/dashboard_server.go |
| API-222 | AresDashboardServer | DELETE | `/api/stations/{station_id}/config` | DeleteStationConfigKey | internal/ares/dashboard_server.go |
| API-223 | AresDashboardServer | GET | `/api/stations` | GetStations | internal/ares/dashboard_server.go |
| API-224 | AresDashboardServer | DELETE | `/api/stations/{station_id}` | DeleteStationApi | internal/ares/dashboard_server.go |
| API-225 | AresDashboardServer | GET | `/api/deployments` | GetDeployments | internal/ares/dashboard_server.go |
| API-226 | AresDashboardServer | GET | `/api/events` | GetEvents | internal/ares/dashboard_server.go |
| API-227 | AresDashboardServer | POST | `/api/stations` | CreateStationApi | internal/ares/dashboard_server.go |
| API-228 | AresDashboardServer | POST | `/api/stations/{station_id}/deployments` | CreateDeploymentApi | internal/ares/dashboard_server.go |
| API-229 | AresDashboardServer | POST | `/api/stations/{station_id}/spinup` | SpinUpDeployment | internal/ares/dashboard_server.go |
| API-230 | AresDashboardServer | GET | `/api/agents` | GetAgents | internal/ares/dashboard_server.go |
| API-231 | AresDashboardServer | POST | `/api/stations/{station_id}/events` | CreateEventApi | internal/ares/dashboard_server.go |
| API-232 | AresDashboardServer | GET | `/api/stations/{station_id}/events` | GetStationEventsApi | internal/ares/dashboard_server.go |
| API-233 | AresDashboardServer | DELETE | `/api/stations/{station_id}/events/{event_id}` | DeleteEventApi | internal/ares/dashboard_server.go |
| API-234 | AresDashboardServer | GET | `/api/stations/{station_id}/deployments` | GetStationDeploymentsApi | internal/ares/dashboard_server.go |
| API-235 | AresDashboardServer | POST | `/api/users/{user_id}/ban` | BanUser | internal/ares/dashboard_server.go |
| API-236 | AresDashboardServer | PATCH | `/api/users/{user_id}/unban` | UnbanUser | internal/ares/dashboard_server.go |
| API-237 | AresDashboardServer | GET | `/api/stations/{station_id}/bans` | GetStationBans | internal/ares/a2station_server.go |
| API-238 | AresDashboardServer | GET | `/api/stations/{station_id}/roles` | GetStationRolesApi | internal/ares/dashboard_server.go |
| API-239 | AresDashboardServer | POST | `/api/stations/{station_id}/roles` | CreateStationRoleApi | internal/ares/dashboard_server.go |
| API-240 | AresDashboardServer | DELETE | `/api/stations/{station_id}/roles/{role_id}` | DeleteStationRoleApi | internal/ares/dashboard_server.go |
| API-241 | AresDashboardServer | PATCH | `/api/stations/{station_id}/roles/{role_id}/permissions` | UpdateRolePermissionsApi | internal/ares/dashboard_server.go |
| API-242 | AresDashboardServer | POST | `/api/stations/{station_id}/users/{user_id}/roles/{role_id}` | AssignRoleApi | internal/ares/dashboard_server.go |
| API-243 | AresDashboardServer | DELETE | `/api/stations/{station_id}/users/{user_id}/roles/{role_id}` | RemoveRoleApi | internal/ares/dashboard_server.go |
| API-244 | AresDashboardServer | GET | `/api/sessions` | GetSessionsApi | internal/ares/dashboard_server.go |
| API-245 | AresDashboardServer | POST | `/api/sessions` | AddSessionApi | internal/ares/dashboard_server.go |
| API-246 | AresDashboardServer | DELETE | `/api/sessions/{session_id}` | RemoveSessionApi | internal/ares/dashboard_server.go |
| API-247 | AresDashboardServer | GET | `/api/telemetry` | GetTelemetry | internal/ares/dashboard_server.go |
| API-248 | AresDashboardServer | GET | `/api/ws-clients` | GetWsClients | internal/ares/dashboard_server.go |
| API-249 | AresDashboardServer | GET | `/api/ws-clients/{session_id}/messages` | GetWsMessages | internal/ares/dashboard_server.go |
| API-250 | AresDashboardServer | GET | `/api/stations/{station_id}/members` | GetStationMembers | internal/ares/dashboard_server.go |
| API-251 | AresDashboardServer | GET | `/api/drafters/{scope}` | GetDrafters | internal/ares/dashboard_server.go |
| API-252 | AresDashboardServer | POST | `/api/drafters/{scope}/{user_id}` | GrantDrafter | internal/ares/dashboard_server.go |
| API-253 | AresDashboardServer | DELETE | `/api/drafters/{scope}/{user_id}` | RevokeDrafter | internal/ares/dashboard_server.go |
| API-254 | EosGatewayServer | POST | `/auth/v1/accounts/deviceid` | DeviceIdLogin | internal/eos/gateway.go |
| API-255 | EosGatewayServer | POST | `/auth/v1/oauth/token` | OAuthToken | internal/eos/gateway.go |
| API-256 | EosGatewayServer | GET | `/auth/v1/oauth/.well-known/jwks.json` | Jwks | internal/eos/gateway.go |
| API-257 | EosGatewayServer | POST | `/auth/v1/turn/credentials` | TurnCredentials | internal/eos/gateway.go |
| API-258 | EosGatewayServer | GET | `/rtc/v1/{deployment}/room/{roomName}` | RtcRoomToken | internal/eos/gateway.go |
| API-259 | EosGatewayServer | GET | `/sdk/v1/default` | DefaultEndpoints | internal/eos/gateway.go |
| API-260 | EosGatewayServer | GET | `/sdk/v1/product/{product_id}` | CustomEndpoints | internal/eos/gateway.go |
| API-261 | EosGatewayServer | POST | `/matchmaking/v1/{deployment_id}/filter` | MatchmakingFilter | internal/eos/gateway.go |
| API-262 | EosGatewayServer | POST | `/lobby/v1/{deployment_id}/lobbies/filter` | LobbyFilter | internal/eos/gateway.go |
| API-263 | EosGatewayServer | POST | `/lobby/v1/{deployment_id}/lobbies` | LobbyCreate | internal/eos/gateway.go |
| API-264 | EosGatewayServer | GET | `/lobby/v1/{deployment_id}/lobbies/{lobby_id}` | LobbyGet | internal/eos/gateway.go |
| API-265 | EosGatewayServer | PUT | `/lobby/v1/{deployment_id}/lobbies/{lobby_id}` | LobbyUpdate | internal/eos/gateway.go |
| API-266 | EosGatewayServer | POST | `/lobby/v1/{deployment_id}/lobbies/{lobby_id}/members/{product_user_id}` | LobbyJoin | internal/eos/gateway.go |
| API-267 | EosGatewayServer | DELETE | `/lobby/v1/{deployment_id}/lobbies/{lobby_id}/members/{product_user_id}` | LobbyLeave | internal/eos/gateway.go |
| API-268 | EosGatewayServer | POST | `/matchmaking/v1/{deployment_id}/sessions/{session_id}/publicplayers/{product_user_id}` | JoinSession | internal/eos/gateway.go |
| API-269 | EosGatewayServer | DELETE | `/matchmaking/v1/{deployment_id}/sessions/{session_id}/publicplayers/{product_user_id}` | LeaveSession | internal/eos/gateway.go |
| API-270 | EosGatewayServer | POST | `/user/v3/product-users/search` | ProductUserSearch | internal/eos/gateway.go |
| API-271 | EosGatewayServer | POST | `/telemetry/data/datarouter/api/v1/public/data` | Telemetry | internal/eos/gateway.go |
| API-272 | EosGatewayServer | POST | `/datarouter/api/v1/public/data/clients` | DataRouterEvent | internal/eos/gateway.go |
| API-273 | EosGatewayServer | GET | `/datastorage/v2/access/deployment/{deployment_id}/private/{product_user_id}/{filename}` | PlayerDataStorageAccess | internal/eos/gateway.go |
| API-274 | EosGatewayServer | GET | `/datastorage/v2/data/deployment/{deployment_id}/private/{product_user_id}/{filename}` | PlayerDataStorageGet | internal/eos/gateway.go |
| API-275 | EosGatewayServer | PUT | `/datastorage/v2/data/deployment/{deployment_id}/private/{product_user_id}/{filename}` | PlayerDataStoragePut | internal/eos/gateway.go |
| API-276 | EosGatewayServer | POST | `/epic/presence/internal/v1/_/{account_id}/presence/{connection_id}` | UpdatePresence | internal/eos/gateway.go |
| API-277 | EosWsServer | GET | `/` | Root | internal/eos/wsserver.go |
| API-278 | HalcyonSocketServer | GET | `/agents` | ListAgents | internal/halcyonsocket/server.go |
| API-279 | HalcyonSocketServer | POST | `/spinup` | SpinUp | internal/halcyonsocket/server.go |
| API-280 | HalcyonSocketServer | GET | `/watchdog` | WatchdogStatus | internal/halcyonsocket/server.go |
| API-281 | HalcyonSocketServer | POST | `/watchdog/retire` | WatchdogRetire | internal/halcyonsocket/server.go |
| API-282 | HalcyonSocketServer | POST | `/watchdog/resume` | WatchdogResume | internal/halcyonsocket/server.go |

## Data models (MODEL-*)

| ID | C# model file | Go type location | Evidence |
|---|---|---|---|
| MODEL-001 | AUnrealFeatures.AAMothership/Models/MothershipAnalyticsEventDbObject.cs | `MothershipAnalyticsEventDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-002 | AUnrealFeatures.AAMothership/Models/MothershipApiKeyDbObject.cs | `MothershipApiKeyDbObject` → internal/mothership/models/auth.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-003 | AUnrealFeatures.AAMothership/Models/MothershipAssociationDbObject.cs | `MothershipAssociationDbObject` → internal/mothership/models/auth.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-004 | AUnrealFeatures.AAMothership/Models/MothershipBanDbObject.cs | `MothershipBanDbObject` → internal/mothership/models/moderation.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-005 | AUnrealFeatures.AAMothership/Models/MothershipDeploymentDbObject.cs | `MothershipDeploymentDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-006 | AUnrealFeatures.AAMothership/Models/MothershipEntitlementDbObject.cs | `MothershipEntitlementDbObject` → internal/mothership/models/commerce.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-007 | AUnrealFeatures.AAMothership/Models/MothershipEnvironmentDbObject.cs | `MothershipEnvironmentDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-008 | AUnrealFeatures.AAMothership/Models/MothershipFriendEntryDbObject.cs | `MothershipFriendEntryDbObject` → internal/mothership/models/social.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-009 | AUnrealFeatures.AAMothership/Models/MothershipInventoryItemDbObject.cs | `MothershipInventoryItemDbObject` → internal/mothership/models/commerce.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-010 | AUnrealFeatures.AAMothership/Models/MothershipLinkDbObject.cs | `MothershipLinkDbObject` → internal/mothership/models/auth.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-011 | AUnrealFeatures.AAMothership/Models/MothershipMuteDbObject.cs | `MothershipMuteDbObject` → internal/mothership/models/moderation.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-012 | AUnrealFeatures.AAMothership/Models/MothershipNotificationDbObject.cs | `MothershipNotificationDbObject` → internal/mothership/models/social.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-013 | AUnrealFeatures.AAMothership/Models/MothershipOfferBindingDbObject.cs | `MothershipOfferBindingDbObject` → internal/mothership/models/commerce.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-014 | AUnrealFeatures.AAMothership/Models/MothershipOfferDbObject.cs | `MothershipOfferDbObject` → internal/mothership/models/commerce.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-015 | AUnrealFeatures.AAMothership/Models/MothershipOfferDisplayDbObject.cs | `MothershipOfferDisplayDbObject` → internal/mothership/models/commerce.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-016 | AUnrealFeatures.AAMothership/Models/MothershipPlayerDbObject.cs | `MothershipPlayerDbObject` → internal/mothership/models/auth.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-017 | AUnrealFeatures.AAMothership/Models/MothershipPrivacyStateDbObject.cs | `MothershipPrivacyStateDbObject` → internal/mothership/models/social.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-018 | AUnrealFeatures.AAMothership/Models/MothershipProgressionTrackBindingDbObject.cs | `MothershipProgressionTrackBindingDbObject` → internal/mothership/models/progression.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-019 | AUnrealFeatures.AAMothership/Models/MothershipProgressionTrackDbObject.cs | `MothershipProgressionTrackDbObject` → internal/mothership/models/progression.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-020 | AUnrealFeatures.AAMothership/Models/MothershipProgressionTreeBindingDbObject.cs | `MothershipProgressionTreeBindingDbObject` → internal/mothership/models/progression.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-021 | AUnrealFeatures.AAMothership/Models/MothershipProgressionTreeDbObject.cs | `MothershipProgressionTreeDbObject` → internal/mothership/models/progression.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-022 | AUnrealFeatures.AAMothership/Models/MothershipReportDbObject.cs | `MothershipReportDbObject` → internal/mothership/models/moderation.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-023 | AUnrealFeatures.AAMothership/Models/MothershipSessionDbObject.cs | `MothershipSessionDbObject` → internal/mothership/models/auth.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-024 | AUnrealFeatures.AAMothership/Models/MothershipSharedGroupDbObject.cs | `MothershipSharedGroupDbObject` → internal/mothership/models/social.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-025 | AUnrealFeatures.AAMothership/Models/MothershipTitleDataDbObject.cs | `MothershipTitleDataDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-026 | AUnrealFeatures.AAMothership/Models/MothershipTitleDbObject.cs | `MothershipTitleDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-027 | AUnrealFeatures.AAMothership/Models/MothershipTransactionCatalogDbObject.cs | `MothershipTransactionCatalogDbObject` → internal/mothership/models/commerce.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-028 | AUnrealFeatures.AAMothership/Models/MothershipUserDataDbObject.cs | `MothershipUserDataDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-029 | AUnrealFeatures.AAMothership/Models/MothershipUserDataMetadataDbObject.cs | `MothershipUserDataMetadataDbObject` → internal/mothership/models/content.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-030 | AUnrealFeatures.AAMothership/Models/MothershipV2PlayerDbObject.cs | `MothershipV2PlayerDbObject` → internal/mothership/models/auth.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-031 | AUnrealFeatures.Ares/Models/ApiKeyResponse.cs | `ApiKeyResponse` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-032 | AUnrealFeatures.Ares/Models/AssignPermsBody.cs | `AssignPermsBody` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-033 | AUnrealFeatures.Ares/Models/BanRequest.cs | `BanRequest` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-034 | AUnrealFeatures.Ares/Models/BanResponse.cs | `BanResponse` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-035 | AUnrealFeatures.Ares/Models/ClientLoginDeployment.cs | `ClientLoginDeployment` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-036 | AUnrealFeatures.Ares/Models/ClientLoginRequest.cs | `ClientLoginRequest` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-037 | AUnrealFeatures.Ares/Models/ClientLoginResponse.cs | `ClientLoginResponse` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-038 | AUnrealFeatures.Ares/Models/ClientLoginV2Response.cs | `ClientLoginV2Response` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-039 | AUnrealFeatures.Ares/Models/CreateDeploymentResponse.cs | `CreateDeploymentResponse` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-040 | AUnrealFeatures.Ares/Models/CreateRoleRequest.cs | `CreateRoleRequest` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-041 | AUnrealFeatures.Ares/Models/CreateStationRequest.cs | `CreateStationRequest` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-042 | AUnrealFeatures.Ares/Models/CreateStationResponse.cs | `CreateStationResponse` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-043 | AUnrealFeatures.Ares/Models/DeploymentDbObject.cs | `DeploymentDbObject` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-044 | AUnrealFeatures.Ares/Models/DeploymentRequest.cs | `DeploymentRequest` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-044 | AUnrealFeatures.Ares/Models/DeploymentRequest.cs | `DeploymentExpanded` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-045 | AUnrealFeatures.Ares/Models/DeploymentsResponsePage.cs | `DeploymentsResponsePage` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-046 | AUnrealFeatures.Ares/Models/EventSignupDbObject.cs | `EventSignupDbObject` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-047 | AUnrealFeatures.Ares/Models/EventSignupResponse.cs | `EventSignupResponse` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-048 | AUnrealFeatures.Ares/Models/FleetResponse.cs | `FleetResponse` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-049 | AUnrealFeatures.Ares/Models/FleetResponsePage.cs | `FleetResponsePage` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-050 | AUnrealFeatures.Ares/Models/FleetStationResponse.cs | `FleetStationResponse` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-051 | AUnrealFeatures.Ares/Models/HTTPValidationError.cs | `ValidationError` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-051 | AUnrealFeatures.Ares/Models/HTTPValidationError.cs | `HTTPValidationError` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-052 | AUnrealFeatures.Ares/Models/LogInServerRequest.cs | `LogInServerRequest` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-053 | AUnrealFeatures.Ares/Models/LogInServerResponse.cs | `UserPermission` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-053 | AUnrealFeatures.Ares/Models/LogInServerResponse.cs | `LogInServerResponse` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-054 | AUnrealFeatures.Ares/Models/PagedResponse.cs | `PageData` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-054 | AUnrealFeatures.Ares/Models/PagedResponse.cs | `PagedResponse` → internal/ares/models/paged.go (generic `PagedResponse[T]`) | paging arithmetic oracle fixtures |
| MODEL-055 | AUnrealFeatures.Ares/Models/RegionResponse.cs | `RegionResponse` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-056 | AUnrealFeatures.Ares/Models/RegisterServerModels.cs | `RegisterServerRequest` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-056 | AUnrealFeatures.Ares/Models/RegisterServerModels.cs | `RegisterServerResponse` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-056 | AUnrealFeatures.Ares/Models/RegisterServerModels.cs | `UpdatePlayerCountRequest` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-057 | AUnrealFeatures.Ares/Models/RoleData.cs | `RoleData` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-057 | AUnrealFeatures.Ares/Models/RoleData.cs | `RoleResponse` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-057 | AUnrealFeatures.Ares/Models/RoleData.cs | `RolesResponse` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-058 | AUnrealFeatures.Ares/Models/ServerDeploymentResponse.cs | `ServerDeploymentResponse` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-059 | AUnrealFeatures.Ares/Models/ServerDeploymentResponsePage.cs | `ServerDeploymentResponsePage` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-060 | AUnrealFeatures.Ares/Models/ServerEventDbObject.cs | `ServerEventDbObject` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-061 | AUnrealFeatures.Ares/Models/ServerEventRequest.cs | `ServerEventRequest` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-062 | AUnrealFeatures.Ares/Models/ServerEventResponse.cs | `ServerEventResponse` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-063 | AUnrealFeatures.Ares/Models/ServerEventResponsePage.cs | `ServerEventResponsePage` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-064 | AUnrealFeatures.Ares/Models/StationDbObject.cs | `StationDbObject` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-065 | AUnrealFeatures.Ares/Models/StationDeploymentResponse.cs | `StationDeploymentResponse` → internal/ares/models/deployments.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-066 | AUnrealFeatures.Ares/Models/StationEventDbObject.cs | `StationEventDbObject` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-067 | AUnrealFeatures.Ares/Models/StationEventSignupInfo.cs | `StationEventSignupInfo` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-068 | AUnrealFeatures.Ares/Models/StationEventSignups.cs | `StationEventSignups` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-069 | AUnrealFeatures.Ares/Models/StationEvents.cs | `StationEvent` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-069 | AUnrealFeatures.Ares/Models/StationEvents.cs | `StationEvents` → internal/ares/models/events.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-070 | AUnrealFeatures.Ares/Models/StationPerm.cs | `StationPerm` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-071 | AUnrealFeatures.Ares/Models/StationResponse.cs | `StationResponse` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-072 | AUnrealFeatures.Ares/Models/StationResponsePage.cs | `StationResponsePage` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-073 | AUnrealFeatures.Ares/Models/StationsPageV2.cs | `StationsPageV2` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-074 | AUnrealFeatures.Ares/Models/SuccessBoolean.cs | `SuccessBoolean` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-075 | AUnrealFeatures.Ares/Models/UpdateStationRequest.cs | `UpdateStationRequest` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-076 | AUnrealFeatures.Ares/Models/UpdateStationResponse.cs | `UpdateStationResponse` → internal/ares/models/stations.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-077 | AUnrealFeatures.Ares/Models/UserApiKeyDbObject.cs | `UserApiKeyDbObject` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-078 | AUnrealFeatures.Ares/Models/UserData.cs | `UserData` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-079 | AUnrealFeatures.Ares/Models/UserDataResponse.cs | `UserDataResponse` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-080 | AUnrealFeatures.Ares/Models/UserDataV1Response.cs | `UserDataV1Response` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-081 | AUnrealFeatures.Ares/Models/UserRequest.cs | `UserRequest` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-082 | AUnrealFeatures.Ares/Models/UserRoleByNameRequest.cs | `UserRoleByNameRequest` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-083 | AUnrealFeatures.Ares/Models/UserRoleByNameResponse.cs | `UserRoleByNameResponse` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-084 | AUnrealFeatures.Ares/Models/UsersResponse.cs | `UsersResponse` → internal/ares/models/users.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-085 | AUnrealFeatures.Ares/Models/UsersResponsePage.cs | `UsersResponsePage` → internal/ares/models/paged.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-086 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosLobbyModels.cs | `EosLobbyFilterRequest` → internal/eos/models/lobby.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-086 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosLobbyModels.cs | `EosLobbyResponse` → internal/eos/models/lobby.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-086 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosLobbyModels.cs | `EosLobbySession` → internal/eos/models/lobby.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-087 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosMatchmakingModels.cs | `EosFilterCriterion` → internal/eos/models/matchmaking.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-087 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosMatchmakingModels.cs | `EosFilterRequest` → internal/eos/models/matchmaking.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-087 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosMatchmakingModels.cs | `EosMatchmakingResponse` → internal/eos/models/matchmaking.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-088 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosOAuthTokenResponse.cs | `EosOAuthTokenResponse` → internal/eos/models/misc.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-089 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosSessionInfo.cs | `EosSessionInfo` → internal/eos/models/misc.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-090 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosTelemetryRequest.cs | `EosTelemetryRequest` → internal/eos/models/misc.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-090 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosTelemetryRequest.cs | `EosTelemetryEvent` → internal/eos/models/misc.go | JSON oracle fixtures + LiteDB round-trip |
| MODEL-091 | AUnrealFeatures.EOSSDK/AUnrealFeatures.EOSSDK/Models/EosTurnCredentialsResponse.cs | `EosTurnCredentialsResponse` → internal/eos/models/misc.go | JSON oracle fixtures + LiteDB round-trip |
