# Configuration contract (original → Solara)

Every setting the original read is still read by Solara under the **same name, default and
precedence**. Compile-time constants of the original remain constants unless noted.

## Environment variables

| Variable | Read by | Original semantics | Solara |
|---|---|---|---|
| `MOTHERSHIP_SELF_ELEVATE` | ModuleInitialization | `1`/`true` → restart elevated (runas / sudo) | same (`internal/host`) |
| `MOTHERSHIP_INSECURE` | MothershipServer | `1`/`true`/`TRUE`/`yes` → disable all identity checks (auth bypass) + red console banner | same |
| `MOTHERSHIP_ALLOW_NONSTORE` | MothershipServer | same value set → relax only the store-recognized check | same |
| `HALCYON_SERVER_API_KEY` | MothershipServer, SpecLevelsStore | game-server shared key; then `server_api_key.txt` next to the binary; then baked fallback | same order; baked fallback from `config/legacy-secrets.env` |
| `DASHBOARD_AUTH_ENFORCE` | DashboardAuthPreprocessor | dashboard auth switch | same |
| `DASHBOARD_ADMIN_KEY`, `DASHBOARD_SESSION_SECRET` | DashboardAuthPreprocessor | dashboard login secrets (runtime env) | same |
| `BOARD_PUBLIC_BASE` | A2StationDbServer, DashboardServer | public base URL for board links | same |
| ServerWatchdog tunables | ServerWatchdog | `int.TryParse(env) && v > 0 ? v : fallback` | same (names listed in `internal/halcyonsocket`) |

`install.sh` also wrote `DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1`, `HALCYON_PUBLIC_HOST`,
`HALCYON_BUILD_VERSION`, `META_APP_ID`, `META_APP_SECRET` into the service environment; the
original backend source does not read the last four (they were consumed by sed-patching the
source before build). Solara likewise ignores them (see "Build-time constants").

## Files next to the binary (`AppContext.BaseDirectory` → executable directory)

| File | Purpose |
|---|---|
| `server_api_key.txt` | game-server API key (second in precedence) |
| `cosmetic_catalogue.txt`, `auto_complete_quests.txt` | Mothership data files (copied from `backend/`) |
| `sessions*.json`, `lobbies*.json` | EOS gateway persisted session/lobby state |
| `config/legacy-secrets.env` | **Solara only**: secrets the original hard-coded (see below) |

Files relative to the working directory: `logs/log-YYYYMMDD*.txt`, the database files, and
the station stores' directories (netvar dumps/overrides, spec levels — same paths as original).

## Secrets that were hard-coded in the C# source

`JWT_SECRET`, `META_APP_SECRET`, `AUTOMATION_KEY`, `SERVER_API_KEY` (legacy fallback),
`ServerMasterKey`, `QUEST_APP_SECRET`, `TURN_SECRET`, `RTC_API_KEY`, `RTC_API_SECRET`,
`PRODUCTION_DB_PASSWORD`. Solara reads them from `SOLARA_<NAME>` env vars, else
`config/legacy-secrets.env` (generated from the original source by
`tools/scripts/extract-legacy-secrets.sh`; git-ignored). Values must stay identical to keep
existing JWTs, the game-server DLL's key and the encrypted database valid. `solara serve`
refuses to start if any is missing (`solara check-secrets` lists them).

## Build-time constants

`install.sh` sed-patched `QUEST_EXPECTED_PACKAGE`, `QUEST_EXPECTED_CERT_SHA`
(MothershipServer) and `BuildVersion`/`BuildIdNum` (A2StationDbServer) before compiling.
Solara keeps these as Go constants with the values currently in the source; change them in
`internal/mothership` / `internal/ares` and rebuild (same workflow as before).

## Database selection

Original: `#if DEBUG` → `development.db` (no password), else `production.db` (password).
Solara: `solara serve --config Debug|Release` (default **Release** → `production.solara.db`,
encrypted with `PRODUCTION_DB_PASSWORD`; Debug → `development.solara.db`, unencrypted).
`run-backend.cmd` used `-c Debug`; the installers published Release.

## Ports (compiled constants, unchanged)

:90 Mothership · :78 A2StationDb · :8080 AresDashboard · :50 EOS gateway (+WS) · :80 EOS WS ·
:9095 HalcyonSocket HTTP control · :9100 AgentHub TCP. All bind on all interfaces.

## Security options added by Solara

| Variable / file | Effect |
|---|---|
| `DASHBOARD_BIND_ADDRESS` | Interface for the :8080 dashboard API (default: all). Use `127.0.0.1` when only the local Next.js dashboard should reach it. |
| `config/generated-dashboard_admin_key.txt`, `config/generated-dashboard_session_secret.txt` | Written (0600) when the corresponding env var is unset; the value is no longer printed. |
| Dashboard lockout | 10 failed credential attempts / 5 min per client → 429 for 15 min (constants in `internal/ares/dashboard_hardening.go`). |
| Request limits (all servers) | body ≤ 64 MiB (413), 2 min to send a body, 2 min to read a response, 2 min idle keep-alive (`internal/hosting/server.go`). |
| Station API (:78) keys | Guarded routes need `x-api-key` (master key or an issued user/deployment key) or `x-server-api-key` — see KNOWN_ISSUES SEC-F2. |
| `SOLARA_REQUIRE_SERVER_TOKENS` | `1`/`true`/`yes`: strict mode — the static game keys (`SERVER_API_KEY`/`HALCYON_SERVER_API_KEY`, `ServerMasterKey`) are rejected wherever they authenticate a game server (Mothership, :78, :9095), and :78 ingest routes require a per-server token. Default: compat (static keys accepted, deprecation warning once/hour/route). See docs/SERVER_IDENTITY.md. |
| `SOLARA_SERVER_TOKEN_IDLE_HOURS` | Idle timeout of per-server tokens (default `24`; sliding, refreshed by use). |
| `SOLARA_SERVER_TOKEN_MAX_DAYS` | Absolute lifetime of per-server tokens (default `30`). Agent tokens: 30 days idle / 365 days absolute unless set with the CLI. |
| `SOLARA_REQUIRE_AGENT_AUTH` | `1`: :9100 agents must send a valid agent token in `hello` or connect from `SOLARA_AGENT_ALLOWED_IPS`; others are disconnected. Default: accepted with a warning. |
| `SOLARA_AGENT_ALLOWED_IPS` | Comma-separated IPs / CIDRs whose agents count as authenticated (for the compiled agent, which cannot send a token). |
| HalcyonSocket :9095 | All 5 routes need the dashboard admin key in `x-dashboard-key` (or `ServerMasterKey` in `x-api-key`, compat mode only) → else 401. |
| `x-dashboard-key` on :78 | The dashboard admin key is also an admin credential on the station API (admin tier, purge/prune) — the strict-mode replacement for admins' `ServerMasterKey`. |

## Database snapshots (Solara addition — see docs/SNAPSHOTS.md)

| Variable | Default | Effect |
|---|---|---|
| `SOLARA_SNAPSHOT_DIR` | `snapshots/` in the working directory | Where snapshots are written / listed / restored from (server scheduler and `solara snapshot`). |
| `SOLARA_SNAPSHOT_INTERVAL_HOURS` | `6` | Automatic snapshot interval while the server runs; fractions allowed; `0` disables periodic snapshots. |
| `SOLARA_SNAPSHOT_KEEP` | `28` | After each automatic snapshot, prune to this many (protected snapshots excepted); `0` disables automatic pruning. |
| `SOLARA_SNAPSHOT_ON_START` | on | `0`/`false`/`no`/`off` disables the `startup` snapshot taken when the server starts. |
