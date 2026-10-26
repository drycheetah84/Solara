# Graph Report - Solara  (2026-10-25)

## Corpus Check
- 167 files · ~136,207 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 5 file(s) not represented in the graph (top: (none) 2, .log 2, .tsv 1)

## Summary
- 2443 nodes · 10045 edges · 120 communities (93 shown, 27 thin omitted)
- Extraction: 80% EXTRACTED · 20% INFERRED · 0% AMBIGUOUS · INFERRED: 1964 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- Result
- D
- TestModelsMatchOracle
- dotnet/misc.go
- A2StationDbServer
- O
- spec_levels_store.go
- a2netcompat.go
- difftest/main.go
- Request
- results.go
- parseBody
- AstraDbObject
- FakeResponse
- a2Users
- Console
- helpers.go
- FakeRequest
- host.go
- P
- gateway.go
- dashboard_dtos.go
- Element
- LobbyWsHelperRunAsync
- AgentHub
- WebSocket
- P
- encode
- Dictionary
- Dict
- fuzzysharp.go
- Collection
- handlers_test.go
- DateTime
- .evaluateOne
- Database
- Server
- models.go
- NewGuidN
- TestAgentHubProtocolOracle
- .CompleteQuestAuth
- TestWatchdogOracle
- matchmaking.go
- MustParse
- testing.T
- MarshalString
- jsonnode.go
- ExceptionMessage
- deployments.go
- assign
- .SpecSetLoaded
- di.go
- 6. .NET semantics helpers (package `solara/internal/dotnet`)
- Throw
- ExtractPuidFromBearer
- verifyQuestAttestation
- jsonBody
- jsonT
- Build
- Parse
- TestOriginalDraftersSuite
- 10. PHASE SIX — EXECUTION STRATEGY
- Manager
- Response
- .TryPreprocessRequest
- Logger
- NewServer
- Building, testing and deploying Solara
- ServerWatchdog
- TestNetCompatHelpers
- setTitleData
- TestA2NetvarDumpStoreAndAcl
- MASTER ENGINEERING PROMPT — HALCYON SERVER BACKEND + UNREAL MODULES → GO
- 13. REQUIRED DELIVERABLES
- 6.1 Investigate every module
- FakeContext
- TestApplyAutoCompletedQuestsMatchesOracle
- Solara porting guide (C# Halcyon → Go)
- WsClientSession
- BindString
- ImportLiteDBExport
- Configuration contract (original → Solara)
- ConcurrentDictionary
- 7. PHASE THREE — PORT THE BACKEND TO GO
- FromTime
- StorageContainerProcessor
- Get
- .Main
- 11. MIGRATION COVERAGE AUDIT
- EosWsServer
- .Main
- Solara architecture
- 3. NON-NEGOTIABLE MIGRATION RULES
- 4. MANDATORY GRAPHIFY SKILL WORKFLOW
- 5. PHASE ONE — COMPLETE SOURCE RECONNAISSANCE
- 8. PHASE FOUR — STRICT API AND PROTOCOL COMPATIBILITY
- 9. PHASE FIVE — BUILD AND TEST THE MIGRATED SYSTEM
- difftest-oracle.csproj
- litedb-export.csproj
- oracle-ares-db.csproj
- oracle.csproj
- Exception
- Host
- o.csproj
- oracle-ares.csproj
- oracle-eos.csproj
- oracle-halcyonsocket.csproj
- oracle-mothership-db.csproj
- oracle-mothership.csproj
- routes.csproj
- extract-legacy-secrets.sh
- DATA_MIGRATION.md
- dotnetTransport
- TestMain
- JsonError
- SyntaxError
- solara

## God Nodes (most connected - your core abstractions)
1. `Result` - 356 edges
2. `Context` - 306 edges
3. `Ok()` - 145 edges
4. `O()` - 144 edges
5. `MothershipServer` - 124 edges
6. `DateTime` - 123 edges
7. `A2StationDbServer` - 94 edges
8. `unauthorized()` - 88 edges
9. `isAdmin()` - 81 edges
10. `jsonAnon()` - 81 edges

## Surprising Connections (you probably didn't know these)
- `7. Secrets` --references--> `Get()`  [INFERRED]
  docs/PORTING_GUIDE.md → internal/secrets/secrets.go
- `8. Fidelity rules` --references--> `Server`  [INFERRED]
  docs/PORTING_GUIDE.md → internal/hosting/server.go
- `1. What the original actually is (verified from source)` --references--> `A2StationDbServer`  [INFERRED]
  ARCHITECTURE.md → internal/ares/a2station_server.go
- `1. What the original actually is (verified from source)` --references--> `EosGatewayServer`  [INFERRED]
  ARCHITECTURE.md → internal/eos/gateway.go
- `1. What the original actually is (verified from source)` --references--> `EosWsServer`  [INFERRED]
  ARCHITECTURE.md → internal/eos/wsserver.go

## Import Cycles
- None detected.

## Communities (120 total, 27 thin omitted)

### Community 0 - "Result"
Cohesion: 0.07
Nodes (38): 1. What the original actually is (verified from source), AresDashboardServer, HalcyonSocketServer, dashboardChangeDrafter(), dashboardDrafterError(), dashActionError(), dashFindUser(), dashTryParseInt() (+30 more)

### Community 1 - "D"
Cohesion: 0.15
Nodes (9): D, A, B, F, G, H, I, Id (+1 more)

### Community 2 - "TestModelsMatchOracle"
Cohesion: 0.04
Nodes (98): ClientLoginRequest, ServerEventResponse, StationEvent, UserPermission, NewAssignPermsBody(), NewClientLoginDeployment(), NewClientLoginRequest(), NewClientLoginResponse() (+90 more)

### Community 3 - "dotnet/misc.go"
Cohesion: 0.15
Nodes (8): IA2StationDbServer, StompHeader, ContainerMessageResult, ContainerMessageResultOf, BindInt64(), BindNullableInt(), parseIntConverter(), KV

### Community 4 - "A2StationDbServer"
Cohesion: 0.08
Nodes (43): a2KV, A2StationDbServer, a2Coalesce(), a2EqFoldPtr(), a2EqFoldS(), a2Header(), a2IsNullOrEmpty(), a2IsNullOrWhiteSpace() (+35 more)

### Community 5 - "O"
Cohesion: 0.12
Nodes (10): errorResult(), isAdmin(), jsonAnon(), unauthorized(), deleteTrack(), deleteTree(), trees(), dbEq() (+2 more)

### Community 6 - "spec_levels_store.go"
Cohesion: 0.08
Nodes (52): a2SpecIndex, Dump, SpecLevelsStoreEntry, SpecLevelsStoreLevelInfo, a2BaseDirectory(), a2ContainsS(), a2InvalidFileNameChar(), a2RemoveS() (+44 more)

### Community 8 - "a2netcompat.go"
Cohesion: 0.08
Nodes (40): a2CopyMap(), a2IsNumberWhite(), a2IsWhiteSpace(), a2ParseBool(), a2ParseDateTime(), a2ParseInt(), a2ParseNumberAny(), a2Query() (+32 more)

### Community 9 - "difftest/main.go"
Cohesion: 0.09
Nodes (10): result, scenario, step, all(), bodiesEqual(), compareFiles(), main(), normalize() (+2 more)

### Community 10 - "Request"
Cohesion: 0.05
Nodes (29): DashboardAuthPreprocessor, NameValues, DashboardAuthPreprocessorAdminKeyForDisplay(), DashboardAuthPreprocessorEnforced(), DashboardAuthPreprocessorHasValidAdminKey(), dashboardAuthTypeInit(), dashboardB64Url(), dashboardB64UrlDecode() (+21 more)

### Community 11 - "results.go"
Cohesion: 0.07
Nodes (44): contentResult, Decimal, FormatDouble(), formatFloat(), FormatSingle(), Accepted(), BadGateway(), Conflict() (+36 more)

### Community 12 - "parseBody"
Cohesion: 0.10
Nodes (31): UtcNow(), offerBindings(), offerDisplays(), col(), intOr(), newId(), parseBody(), strp() (+23 more)

### Community 13 - "AstraDbObject"
Cohesion: 0.06
Nodes (43): AstraDbObject, upsertV1Player(), entitlements(), offers(), transactions(), privacyCol(), MothershipPlayerDbObject, NewMothershipApiKeyDbObject() (+35 more)

### Community 14 - "FakeResponse"
Cohesion: 0.11
Nodes (6): FakeResponse, Content, ContentType, Headers, HeadersAtApply, StatusCode

### Community 15 - "a2Users"
Cohesion: 0.09
Nodes (25): a2StrSet, a2ContainsStr(), a2EqPtr(), a2EqS(), a2NullReference(), a2RemoveStr(), a2ExpandRoles(), a2GetPermissionsForUser() (+17 more)

### Community 16 - "Console"
Cohesion: 0.08
Nodes (36): a2CurrentDir(), a2Deref(), a2ErrMessage(), a2ExceptionMessage(), a2IsBlank(), a2IsLetterOrDigit(), a2BoardPublicBaseEnv(), a2BoardUploadDir() (+28 more)

### Community 17 - "helpers.go"
Cohesion: 0.11
Nodes (30): containsStr(), randomBytes(), base64Url(), ctEquals(), decodeBase64UrlToString(), fixedTimeEquals(), fromBase64String(), generateJwt() (+22 more)

### Community 18 - "FakeRequest"
Cohesion: 0.12
Nodes (12): FakeRequest, Body, FormBody, Headers, Host, JsonBody, Method, Origin (+4 more)

### Community 21 - "gateway.go"
Cohesion: 0.13
Nodes (36): nameSet, dashSessionsSnapshot(), TestLobbyReaper(), TestSessionsAndMatchmakingFilter(), ClearSessions(), DeploymentId(), ensureGatewayStatic(), gatewayStaticInit() (+28 more)

### Community 22 - "dashboard_dtos.go"
Cohesion: 0.08
Nodes (37): DashboardActionResult, DashboardBan, DashboardBanRequest, DashboardCreateDeployment, DashboardCreateEvent, DashboardCreateRole, DashboardCreateStation, DashboardDeployment (+29 more)

### Community 23 - "Element"
Cohesion: 0.11
Nodes (11): 3. JSON (package `solara/internal/stj`), NewEosLobbyPlatforms(), NewEosTelemetryRequest(), FromElement(), formatEx(), Element, ValueKind, invalidOp() (+3 more)

### Community 24 - "LobbyWsHelperRunAsync"
Cohesion: 0.15
Nodes (21): EosGatewayServer, addEosHeaders(), eosError(), lobbyInfoEnvelope(), BroadcastAsync(), buildMemberDataMap(), containsString(), findLobby() (+13 more)

### Community 25 - "AgentHub"
Cohesion: 0.11
Nodes (6): AgentConnection, AgentHub, getInt(), getStr(), newAgentConnection(), NewAgentHub()

### Community 26 - "WebSocket"
Cohesion: 0.12
Nodes (22): StompHeaders, WebSocket, WebSocketMessageType, WebSocketState, TestObservabilityStore(), TestStompFrames(), addLobbyConnection(), sendError() (+14 more)

### Community 28 - "encode"
Cohesion: 0.11
Nodes (27): dfield, DateToStore(), decodeDoc(), decVal(), dfields(), encodeDoc(), encVal(), isNil() (+19 more)

### Community 30 - "Dict"
Cohesion: 0.09
Nodes (23): checkOracle(), esc(), fullSession(), populatedLobby(), s(), TestIndentedPersistenceFormat(), TestModelJSONPopulated(), TestObservabilityRecordsJSON() (+15 more)

### Community 31 - "fuzzysharp.go"
Cohesion: 0.15
Nodes (21): a2EditOp, a2EditType, a2FuzzyResult, a2MatchingBlock, a2MinHeap, a2EditOps(), a2EditOpsFromCostMatrix(), a2ExtractTop() (+13 more)

### Community 32 - "Collection"
Cohesion: 0.14
Nodes (7): DbObject, 5. Persistence (package `solara/internal/docdb`), HasPrefixFold(), Collection, idOf(), ObjectId, NewObjectId()

### Community 33 - "handlers_test.go"
Cohesion: 0.21
Nodes (27): beginQuest(), itoa(), jesc(), newFakeGraph(), questClaims(), tampered(), TestAutomationCrud(), TestBansAndModeration() (+19 more)

### Community 34 - "DateTime"
Cohesion: 0.13
Nodes (11): ImportedWidget, Widget, Kind, DateFromStore(), NewWidget(), FromTicks(), FromUnixMilliseconds(), FromUnixSeconds() (+3 more)

### Community 35 - ".evaluateOne"
Cohesion: 0.17
Nodes (12): lockGuard, Watched, isNullOrWhiteSpace(), isNullOrWhiteSpaceP(), isWhiteSpace(), msBetween(), utf16Len(), utf16Prefix() (+4 more)

### Community 36 - "Database"
Cohesion: 0.11
Nodes (8): Bytedata, CollectionMetadata, NewBytedata(), read7(), write7(), Database, Open(), TestDbRoundTripMatchesLiteDB()

### Community 37 - "Server"
Cohesion: 0.12
Nodes (10): Handler, Preprocessor, ExceptionText(), NoContent(), compileRoute(), Endpoint, Server, do() (+2 more)

### Community 38 - "models.go"
Cohesion: 0.14
Nodes (22): AgentInfo, AgentListResult, SpinUpBody, SpinUpResult, SuccessResult, WatchdogBody, WatchdogRow, WatchdogStatusResult (+14 more)

### Community 39 - "NewGuidN"
Cohesion: 0.13
Nodes (17): NewGuidN(), TestDeterministicIds(), boolPtr(), steamIdFromTicket(), userNamesSet(), b64url(), ClientToken(), ExtractClientId() (+9 more)

### Community 40 - "TestAgentHubProtocolOracle"
Cohesion: 0.16
Nodes (20): capture, fakeClock, newAgentHubAddr(), TestAgentHubProtocolOracle(), TestAgentHubReap(), compareLines(), dialAgent(), installCapture() (+12 more)

### Community 41 - ".CompleteQuestAuth"
Cohesion: 0.13
Nodes (19): boolStr(), v2Players(), TestDbPredicateNullSemantics(), isTrustedServer(), leftUTF16(), verifyMetaUserNonce(), friendsCol(), MothershipV2PlayerDbObject (+11 more)

### Community 42 - "TestWatchdogOracle"
Cohesion: 0.17
Nodes (13): fakeAgent, wdHarness, FormatBool(), parseInt32(), strp(), env(), newWdHarness(), TestWatchdogDecisionTimings() (+5 more)

### Community 43 - "matchmaking.go"
Cohesion: 0.17
Nodes (19): TestModelJSONDefaults(), NewEosLobbyFilterRequest(), NewEosLobbyResponse(), EosFilterCriterion, NewEosFilterCriterion(), NewEosFilterRequest(), NewEosMatchmakingResponse(), NewEosMatchmakingSession() (+11 more)

### Community 44 - "MustParse"
Cohesion: 0.15
Nodes (18): bearerFor(), do(), readJSON(), TestDefaultEndpointsBytes(), TestJwtSignVerify(), TestLobbyHTTP(), TestLobbyWebSocket(), TestOAuthToken() (+10 more)

### Community 45 - "testing.T"
Cohesion: 0.24
Nodes (19): a2Do(), a2Json(), a2Lines(), a2TestEnv(), TestA2AuthAndKeys(), TestA2DeploymentFetchWithStationConfig(), TestA2DrafterFeed(), TestA2FuzzyMatchesOracle() (+11 more)

### Community 46 - "MarshalString"
Cohesion: 0.13
Nodes (20): itoa(), dbrtRoundTrip(), itoa(), loadFixtures(), TestCreatePagingMatchesOracle(), CollectionName(), TestCollectionName(), TestImportLiteDBExport() (+12 more)

### Community 47 - "jsonnode.go"
Cohesion: 0.20
Nodes (12): intNode(), newArray(), newObject(), nodeFromElement(), parseNode(), strNode(), IsNullOrEmpty(), isNullOrEmpty() (+4 more)

### Community 48 - "ExceptionMessage"
Cohesion: 0.23
Nodes (12): a2PanicError, stompSubprotocol(), strPtr(), IsLobbyPath(), hasSuffixFold(), IsStompPath(), acceptWebSocket(), NewRequest() (+4 more)

### Community 49 - "deployments.go"
Cohesion: 0.16
Nodes (17): DeploymentExpanded, NewCreateDeploymentResponse(), NewDeploymentExpanded(), NewDeploymentRequest(), NewRegisterServerRequest(), NewRegisterServerResponse(), NewServerDeploymentResponse(), NewServerHeartbeatRequest() (+9 more)

### Community 50 - "assign"
Cohesion: 0.15
Nodes (12): Named, DropCollection(), typeNames(), assign(), convErr(), dotnetName(), matchField(), numText() (+4 more)

### Community 51 - ".SpecSetLoaded"
Cohesion: 0.21
Nodes (9): a2Itoa(), a2Trim(), a2Bit(), a2DecodeUTF8NoBOM(), a2SpecBadName(), a2SpecErr(), a2SpecText(), a2SpecUnauthorized() (+1 more)

### Community 52 - "di.go"
Cohesion: 0.26
Nodes (7): Builder, CompositeProvider, Provider, Service, ServiceHost, NewCompositeProvider(), NewServiceHost()

### Community 54 - "6. .NET semantics helpers (package `solara/internal/dotnet`)"
Cohesion: 0.14
Nodes (5): 6. .NET semantics helpers (package `solara/internal/dotnet`), Session, CreateRandomString(), NewGuid(), newSession()

### Community 55 - "Throw"
Cohesion: 0.18
Nodes (12): TelemetryEvent, a2Prefix(), Throw(), TestModelDeserializeDefaults(), playerDataFilePath(), playerDataRoot(), fileExists(), isRooted() (+4 more)

### Community 56 - "ExtractPuidFromBearer"
Cohesion: 0.17
Nodes (14): base64UrlDecode(), eqNullable(), evalOp(), ExtractPuidFromBearer(), matchesLobbyCriteria(), IdentifyCaller(), readResource(), userNamesGet() (+6 more)

### Community 57 - "verifyQuestAttestation"
Cohesion: 0.18
Nodes (14): banDevice(), escapeDataString(), failedAttestation(), formUrlEncode(), httpErrorMessage(), newHttp8(), send(), verifyMetaUserNonceWith() (+6 more)

### Community 58 - "jsonBody"
Cohesion: 0.20
Nodes (11): elementOr(), boolOr(), str(), MothershipProgressionTrackDbObject, MothershipProgressionTreeDbObject, NewMothershipProgressionTrackDbObject(), NewMothershipProgressionTreeDbObject(), newTrack() (+3 more)

### Community 59 - "jsonT"
Cohesion: 0.29
Nodes (7): isAuthorized(), jsonT(), strList(), MothershipSharedGroupDbObject, NewMothershipSharedGroupDbObject(), findGroup(), sharedGroups()

### Community 60 - "Build"
Cohesion: 0.20
Nodes (14): main(), serve(), NewAresDashboardServer(), NewHalcyonSocketServer(), Build(), dbName(), OpenDatabase(), PreRegisterCollections() (+6 more)

### Community 61 - "Parse"
Cohesion: 0.36
Nodes (4): a2FormEscape(), a2VerifyNonceWith(), Parse(), parser

### Community 62 - "TestOriginalDraftersSuite"
Cohesion: 0.35
Nodes (14): newAresDashboardServer(), do(), resetDashAuth(), TestDashboardAuthLoginHtml(), TestDashboardAuthPreprocessor(), TestDashboardDraftersRequireAdminKey(), TestDashboardIndexHtmlByteIdentical(), TestDashboardMiscEndpoints() (+6 more)

### Community 63 - "10. PHASE SIX — EXECUTION STRATEGY"
Cohesion: 0.15
Nodes (13): 10. PHASE SIX — EXECUTION STRATEGY, Step 10: Verify, Step 11: Audit, Step 12: Deliver, Step 1: Inspect, Step 2: Graph, Step 3: Baseline, Step 4: Inventory (+5 more)

### Community 64 - "Manager"
Cohesion: 0.23
Nodes (10): Builder, Descriptor, Options, Manager, NewBuilder(), NewManager(), newManager(), ModuleInitialize() (+2 more)

### Community 66 - ".TryPreprocessRequest"
Cohesion: 0.22
Nodes (4): StaticFilesProcessor, PreprocessorResult, findByStem(), MimeType()

### Community 67 - "Logger"
Cohesion: 0.26
Nodes (3): Logger, render(), rollingFile

### Community 68 - "NewServer"
Cohesion: 0.18
Nodes (9): NewA2StationDbServer(), newA2StationDbServer(), NewServer(), TestStaticConfig(), envSwitch(), LoadServerApiKey(), MarkDashboardSession(), newMothershipServer() (+1 more)

### Community 69 - "Building, testing and deploying Solara"
Cohesion: 0.18
Nodes (10): Build, Building, testing and deploying Solara, Logging, Migrate existing data, Requirements, Run, Secrets (once per deployment), systemd (replacing the .NET unit written by install.sh) (+2 more)

### Community 70 - "ServerWatchdog"
Cohesion: 0.33
Nodes (7): knobs, ServerWatchdog, SpinUpFunc, newHalcyonSocketServer(), currentKnobs(), NewServerWatchdog(), newServerWatchdog()

### Community 71 - "TestNetCompatHelpers"
Cohesion: 0.20
Nodes (10): esc(), itoa(), TestDeploymentIdRegex(), TestNetCompatHelpers(), containsASCIIFold(), decodeUTF8(), formatF0(), maximalSubpart() (+2 more)

### Community 72 - "setTitleData"
Cohesion: 0.25
Nodes (6): MothershipTitleDataDbObject, NewMothershipTitleDataDbObject(), deleteTitleData(), findTitleData(), setTitleData(), titleData()

### Community 73 - "TestA2NetvarDumpStoreAndAcl"
Cohesion: 0.24
Nodes (10): DrafterGrant, TestA2NetvarDumpStoreAndAcl(), TestA2StoreModelsMatchOracle(), DrafterPermissionsEffective(), DrafterPermissionsList(), DrafterPermissionsSet(), NewDrafterGrant(), ContainsFold() (+2 more)

### Community 75 - "MASTER ENGINEERING PROMPT — HALCYON SERVER BACKEND + UNREAL MODULES → GO"
Cohesion: 0.20
Nodes (9): 12. CODE QUALITY AND REPOSITORY RULES, 14. WORKING STYLE AND AUTONOMY, 15. FINAL ACCEPTANCE CRITERIA, 16. YOUR FIRST ACTIONS, 1. ROLE AND MISSION, 2. SOURCE PROJECT — AUTHORITATIVE LOCATION, Initial requirements, MASTER ENGINEERING PROMPT — HALCYON SERVER BACKEND + UNREAL MODULES → GO (+1 more)

### Community 76 - "13. REQUIRED DELIVERABLES"
Cohesion: 0.20
Nodes (10): 13. REQUIRED DELIVERABLES, A. Complete Go implementation, B. Unreal compatibility layer, C. Complete migration inventory, D. Graphify knowledge graph, E. Compatibility matrix, F. Automated tests, G. Build and deployment documentation (+2 more)

### Community 77 - "6.1 Investigate every module"
Cohesion: 0.20
Nodes (10): 6.1 Investigate every module, 6.2 Decide what runs in Go and what remains as a compatibility bridge, 6.3 Preserve all public integration contracts, 6. PHASE TWO — DETERMINE THE CORRECT GO ARCHITECTURE, `AIUnrealFeatures.AAMothership`, `AIUnrealFeatures.Ares`, `AIUnrealFeatures.DependencyInjection`, `AIUnrealFeatures.EOSSDK` (+2 more)

### Community 78 - "FakeContext"
Cohesion: 0.20
Nodes (6): FakeContext, Req, Request, Resp, Response, Session

### Community 79 - "TestApplyAutoCompletedQuestsMatchesOracle"
Cohesion: 0.24
Nodes (10): FormatGuid(), stableId(), S(), loadOracle(), TestApplyAutoCompletedQuestsMatchesOracle(), TestHelpersMatchOracle(), TestModelsMatchOracle(), sv() (+2 more)

### Community 80 - "Solara porting guide (C# Halcyon → Go)"
Cohesion: 0.22
Nodes (7): 0. Tooling hazard — read first, 1. Packages and naming (deterministic, so modules can reference each other before they exist), 2. Models (JSON and storage contract — ALL FIELDS MUST STAY THE SAME), 4. HTTP (package `solara/internal/hosting`), 7. Secrets, 8. Fidelity rules, Solara porting guide (C# Halcyon → Go)

### Community 81 - "WsClientSession"
Cohesion: 0.33
Nodes (4): WsClientSession, WsMessage, GetClients(), Marshal()

### Community 82 - "BindString"
Cohesion: 0.22
Nodes (4): BindBool(), BindInt(), BindString(), TestBinding()

### Community 83 - "ImportLiteDBExport"
Cohesion: 0.25
Nodes (7): importLiteDB(), ImportReport, ImportLiteDBExport(), Done (with evidence), In progress, Migration status (continuation state), Next concrete tasks

### Community 84 - "Configuration contract (original → Solara)"
Cohesion: 0.25
Nodes (7): Build-time constants, Configuration contract (original → Solara), Database selection, Environment variables, Files next to the binary (`AppContext.BaseDirectory` → executable directory), Ports (compiled constants, unchanged), Secrets that were hard-coded in the C# source

### Community 86 - "7. PHASE THREE — PORT THE BACKEND TO GO"
Cohesion: 0.29
Nodes (7): 7.1 Use idiomatic Go without changing behavior, 7.2 Port actual business logic, 7.3 Preserve asynchronous behavior, 7.4 Preserve configuration, 7.5 Preserve persistence, 7.6 Preserve errors and observability, 7. PHASE THREE — PORT THE BACKEND TO GO

### Community 87 - "FromTime"
Cohesion: 0.43
Nodes (5): FromTime(), ParseSTJ(), wallTicks(), formatMs(), netAddYears()

### Community 88 - "StorageContainerProcessor"
Cohesion: 0.43
Nodes (3): ContainerInformation, ContainerInformationSensitive, StorageContainerProcessor

### Community 89 - "Get"
Cohesion: 0.29
Nodes (7): a2QuestAppSecret(), a2RiftAppSecret(), rtcApiKey(), rtcApiSecret(), automationKey(), jwtSecret(), Get()

### Community 91 - "11. MIGRATION COVERAGE AUDIT"
Cohesion: 0.33
Nodes (6): 11.1 Symbol-level comparison, 11.2 Branch-level comparison, 11.3 Field-level comparison, 11.4 Dependency-level comparison, 11.5 No false completion, 11. MIGRATION COVERAGE AUDIT

### Community 92 - "EosWsServer"
Cohesion: 0.40
Nodes (6): EosWsServer, routes(), TestEosWsServerRoot(), TestRouteTables(), NewEosWsServer(), newEosWsServer()

### Community 94 - "Solara architecture"
Cohesion: 0.40
Nodes (4): 2. Migration boundary decision, 3. Cross-module dependency graph (preserved), 4. Behaviour-preserving design choices, Solara architecture

### Community 95 - "3. NON-NEGOTIABLE MIGRATION RULES"
Cohesion: 0.40
Nodes (5): 3.1 No omitted functionality, 3.2 Every field must remain compatible, 3.3 No fake implementations, 3.4 Preserve behavior, not just apparent intent, 3. NON-NEGOTIABLE MIGRATION RULES

### Community 96 - "4. MANDATORY GRAPHIFY SKILL WORKFLOW"
Cohesion: 0.40
Nodes (5): 4.1 Discover and initialize Graphify, 4.2 Build a graph of the original implementation, 4.3 Use the graph during implementation, 4.4 Persist knowledge across sessions, 4. MANDATORY GRAPHIFY SKILL WORKFLOW

### Community 97 - "5. PHASE ONE — COMPLETE SOURCE RECONNAISSANCE"
Cohesion: 0.40
Nodes (5): 5.1 Inventory the repository, 5.2 Establish the original build baseline, 5.3 Trace the complete execution flow, 5.4 Create a definitive feature inventory, 5. PHASE ONE — COMPLETE SOURCE RECONNAISSANCE

### Community 98 - "8. PHASE FOUR — STRICT API AND PROTOCOL COMPATIBILITY"
Cohesion: 0.40
Nodes (5): 8.1 HTTP compatibility, 8.2 Socket and binary protocol compatibility, 8.3 Serialization compatibility, 8.4 Authentication and security compatibility, 8. PHASE FOUR — STRICT API AND PROTOCOL COMPATIBILITY

### Community 99 - "9. PHASE FIVE — BUILD AND TEST THE MIGRATED SYSTEM"
Cohesion: 0.40
Nodes (5): 9.1 Create a migration test suite, 9.2 Differential testing, 9.3 Build verification, 9.4 Performance and resource management, 9. PHASE FIVE — BUILD AND TEST THE MIGRATED SYSTEM

### Community 100 - "difftest-oracle.csproj"
Cohesion: 0.40
Nodes (4): Lib.Harmony (2.3.3), net6.0, LiteDB (5.0.21), Microsoft.NET.Sdk

### Community 101 - "litedb-export.csproj"
Cohesion: 0.50
Nodes (3): net6.0, LiteDB (5.0.21), Microsoft.NET.Sdk

### Community 102 - "oracle-ares-db.csproj"
Cohesion: 0.50
Nodes (3): net6.0, LiteDB (5.0.21), Microsoft.NET.Sdk

### Community 103 - "oracle.csproj"
Cohesion: 0.50
Nodes (3): net6.0, LiteDB (5.0.21), Microsoft.NET.Sdk

## Knowledge Gaps
- **149 isolated node(s):** `solara`, `IA2StationDbServer`, `dashboardSpecLevelPatchBody`, `dashboardWhitelistBody`, `StompHeader` (+144 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 328 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **27 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `DateTime` connect `DateTime` to `.wall`, `6. .NET semantics helpers (package `solara/internal/dotnet`)`, `FromTime`?**
  _High betweenness centrality (0.006) - this node is a cross-community bridge._
- **What connects `solara`, `IA2StationDbServer`, `dashboardSpecLevelPatchBody` to the rest of the system?**
  _149 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Result` be split into smaller, more focused modules?**
  _Cohesion score 0.06859504132231405 - nodes in this community are weakly interconnected._
- **Why does `newManager()` connect `Manager` to `Get`, `Build`?**
  _High betweenness centrality (0.005) - this node is a cross-community bridge._
- **Should `TestModelsMatchOracle` be split into smaller, more focused modules?**
  _Cohesion score 0.04010989010989011 - nodes in this community are weakly interconnected._
- **Why does `Build()` connect `Build` to `Manager`, `NewServer`, `Host`, `MustParse`, `Console`, `helpers.go`, `di.go`, `EosWsServer`?**
  _High betweenness centrality (0.005) - this node is a cross-community bridge._
- **Should `A2StationDbServer` be split into smaller, more focused modules?**
  _Cohesion score 0.07992007992007992 - nodes in this community are weakly interconnected._