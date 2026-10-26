# Solara architecture

Solara is the Go port of the **Halcyon** backend (`OrionDriftBackend.sln`, .NET 6), the
private-server backend for Orion Drift / A2 build 22284. Original (read-only):
`C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\halcyon-server\backend\src`
(snapshot in `reference/halcyon/`, git-ignored).

## 1. What the original actually is (verified from source)

The six "AUnrealFeatures.*" directories are **.NET 6 class libraries / one executable**, not
Unreal Engine C++ modules — there are no `.Build.cs`, `.Target.cs`, `.uplugin`, UCLASS/USTRUCT,
or C++ sources anywhere in the backend tree. "Unreal" refers to the game they serve.

| Original project | Kind | Role | Solara package |
|---|---|---|---|
| AUnrealFeatures.DependencyInjection | lib | singleton DI container (constructor/attribute injection) | `internal/di` |
| AUnrealFeatures.Hosting | lib | HttpListener-based HTTP framework, results, binding, preprocessors, WebSocket server base, LiteDB wrapper (`AstraDatabase*`), Serilog setup | `internal/hosting`, `internal/docdb`, `internal/logging`, `internal/host` (module init) |
| AUnrealFeatures.AAMothership | lib | `MothershipServer` :90 — game client login, JWT, Meta attestation, player data, bans, moderation | `internal/mothership` |
| AUnrealFeatures.Ares | exe | `Program` host; `A2StationDbServer` :78 (game-server DLL API); `AresDashboardServer` :8080 (+ auth preprocessor); station/netvar/spec-level/drafter stores | `cmd/solara`, `internal/host`, `internal/ares` |
| AUnrealFeatures.EOSSDK | lib | `EosGatewayServer` :50 (+WS), `EosWsServer` :80 — a stand-in for Epic Online Services (auth, JWKS, matchmaking, sessions, lobbies over WS/STOMP, RTC tokens, telemetry) | `internal/eos` |
| AUnrealFeatures.HalcyonSocket | lib | `HalcyonSocketServer` :9095 HTTP control, `AgentHub` :9100 TCP agent protocol, `ServerWatchdog` | `internal/halcyonsocket` |

Other components in the repository and how they are treated:

* `backend/dashboard/` — Next.js UI. An **external client** of the :8080/:78 API; unchanged.
  Its requests are part of the compatibility contract.
* `windows/HalcyonA2.dll`, `windows/*.ps1`, allocator — the game-server side (injected native
  payload + PowerShell agent). **External clients**; their wire contracts (HTTP to :78/:90,
  TCP agent protocol to :9100) are preserved, the binaries are not ported.
* `install.sh`, `install.ps1`, `Redeploy-Vps.ps1`, `deploy/*.sh` — deployment for the .NET
  build; Solara's build/deploy is documented in `docs/BUILD_AND_DEPLOY.md`.
* `tests/Drafters` — the only original test (24 checks) → ported as a Go test.

## 2. Migration boundary decision

**Everything ports to Go; no native/Unreal bridge is required.** Rationale: no component is
loaded by Unreal or exposes a native ABI; all integrations are over HTTP/WebSocket/TCP.
The only non-Go artifacts kept are **developer tools**, not runtime components:

* `tools/litedb-export` (.NET) — reads the original LiteDB files with LiteDB itself for data
  migration (no Go LiteDB implementation exists; reimplementing its page format would risk
  corrupting production data).
* `tools/routes`, `tools/oracle*` (.NET) — differential-test oracles that load the built
  original assemblies to capture ground truth (route tables, exact JSON bytes).

## 3. Cross-module dependency graph (preserved)

```
cmd/solara ──> internal/host ──> ares ──> eos, halcyonsocket, mothership
                                  │
            hooks wired by host:  halcyonsocket.TakeDeploymentOffline -> eos + ares DB
                                  StationAcl push -> eos.SetStationWhitelist
eos, halcyonsocket, mothership never import ares (same as the C# project references).
All servers ──> hosting, stj, docdb, dotnet, logging, secrets
```

## 4. Behaviour-preserving design choices

| Concern | Original | Solara | Evidence |
|---|---|---|---|
| Request concurrency | one listener loop per server; each request awaited before the next | per-server mutex around the pipeline; WebSockets on their own goroutine | `internal/hosting/server.go` |
| Routing | reflection order, regex `{name}`→`[^/]+`, slashes collapsed, method must match (else 404) | identical; route tables asserted equal to reflection dump `docs/original_routes.tsv` | per-package route tests |
| Results/content types | `AstraHttpActionResult` rules (null→text/plain empty, primitives→ToString, byte[]→octet-stream, else JSON) | `hosting.Result` factory with the same rules | `hosting` tests |
| Errors | 500 HTML page (`HtmlDocumentCache`, byte-identical asset) with "Error while processing X<br><br>ex" | same page; exception text = .NET type + message (stack trace not reproduced) | `server_test.go` |
| JSON | System.Text.Json defaults: exact names, `\u0022`-style escaping of `"<>&'+\``, non-ASCII escaped, .NET double format, DateTime kinds, case-sensitive binding, JsonElement semantics | `internal/stj` custom encoder/decoder | byte-equality tests against oracle output |
| DateTime | ticks + Kind; LiteDB round-trip = ms precision, Kind Local | `dotnet.DateTime` (ticks+kind), codec reproduces LiteDB | `docdb` tests |
| Persistence | LiteDB 5.0.21 (encrypted with password in Release) | bbolt store, same collection names (`_<md5[:7]>_<type>`), same document member names, `_id` ObjectId order, nulls omitted, AES-256-GCM when a password is configured; **LiteDB predicate collation (case-insensitive) reproduced explicitly** | `docdb` tests, LiteDB fixture import test |
| Data migration | — | `tools/litedb-export` + `solara import-litedb` (non-destructive) | `import_test.go` with a real encrypted LiteDB fixture |
| DI | singletons, eager init in registration order, impl-object quirk | `internal/di` with the same semantics | — |
| Config | compile-time constants + env vars (MOTHERSHIP_*, DASHBOARD_*, HALCYON_*) | same env vars; constants kept; baked secrets externalised to `config/legacy-secrets.env` (same values) | `docs/CONFIGURATION.md` |
| Logging | Serilog console + daily file `logs/log-*.txt`, buckets per class | same templates/locations (`internal/logging`) | — |
| Debug/Release DB choice | `#if DEBUG` → development.db, else production.db | `solara serve --config Debug|Release` (default Release) | `internal/host` |

Deliberate, documented deviations are listed in `KNOWN_ISSUES.md`.
