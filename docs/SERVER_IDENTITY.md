# Per-server identity tokens (Solara addition)

The original backend authenticated every game server with two **static shared secrets**
that sat on every game box's command line:

| Launch flag | Header the DLL sends | Checked by |
|---|---|---|
| `-ServerApiKey=` | `x-server-api-key` | Mothership :90 (`IsTrustedServer`, `IsAdmin`, `/v1/server/authorized`, trusted Quest login), `SpecLevelsStore.IsTrustedServer`, the :78 guard |
| `-DashboardApiKey=` | `x-api-key` | A2StationDb :78 (`LogInWithKey` master path, the :78 guard, purge/prune, board upload) |

Anyone who saw one box's command line (or a leaked `agent.cfg`) could impersonate every server,
forever. Solara replaces them with **per-server tokens** (package `internal/serverid`) while keeping
the prebuilt DLL working unchanged: the DLL still just forwards whatever it is given on its command
line.

## Tokens

* Format: `sst1_` + base64url(32 bytes from `crypto/rand`), 48 characters, only `[A-Za-z0-9_-]` —
  safe on Unreal command lines and accepted by HalcyonAgent.ps1's `Safe-Args`.
* Storage: collection `ServerTokenDbObject` in the live database. **Only the SHA-256 hash** and a
  display prefix (`sst1_` + 6 characters) are stored, with `Kind` (`server`|`agent`), `DeploymentId`,
  `StationId` (optional), `Box`, `IssuedAt`, `LastSeen`, `ExpiresAt` (sliding idle), `AbsoluteExpiry`,
  `IdleMs`, `RevokedAt`, `RevokeReason`. The token is shown once (spin-up message / CLI output) and
  never logged by Solara.
* Validation: lookup by hash (constant-time confirm), rejects revoked / idle-expired /
  absolutely-expired tokens; a successful use updates `LastSeen` and slides `ExpiresAt` at most once
  a minute.
* Lifetimes: server tokens 24 h idle / 30 days absolute (`SOLARA_SERVER_TOKEN_IDLE_HOURS`,
  `SOLARA_SERVER_TOKEN_MAX_DAYS`); agent tokens 30 days idle / 365 days absolute (CLI flags
  `--idle-days`, `--max-days`). A game server heartbeats every 5 s, so only the absolute limit
  matters while it runs: **a server running longer than `SOLARA_SERVER_TOKEN_MAX_DAYS` loses its
  identity** (in strict mode it can then no longer register or heartbeat) — raise the limit if
  servers run that long.

## Lifecycle

1. **Issue** — `HalcyonSocketServer.RequestSpinUp` (dashboard Spin Up, `POST :9095/spinup`, watchdog
   restarts) mints the deployment id, then a `server` token bound to that deployment, the chosen box
   and (dashboard path) the station. The agent receives
   `{"type":"spinup","id","map","region","args":"... -DashboardDeploymentId=<id> -ServerApiKey=<tok> -DashboardApiKey=<tok>","server_token":"<tok>"}`.
   The watchdog records the args **without** the token; a restart mints a new token.
   Manual servers: `solara server-token issue --deployment ID [--box B] [--station S]`.
2. **Use** — see the rules below.
3. **Revoke** — automatically when the agent reports the server exited, when the watchdog takes a
   deployment offline (`host.TakeDeploymentOffline`, before every restart), when it gives up, and when
   a deployment/station is deleted (`ServerWatchdog.Stop`); a failed send revokes the fresh token.
   Manually: `solara server-token revoke <prefix|id>` / `--deployment ID`, or
   `DELETE /api/server-tokens/{id|prefix}` on the dashboard (admin key).
4. **Prune** — `solara server-token prune [--grace-hours N]` deletes records revoked/expired longer
   than N hours ago (default 7 days).

`solara serve` holds the database file lock, so the CLI works only while the server is stopped (it
gives up after 10 s with a clear message). While it runs, use `GET/DELETE /api/server-tokens`.
Issuing agent tokens therefore needs a short maintenance window (or issue them before deploying).

## Authorization rules

**Strict mode** = `SOLARA_REQUIRE_SERVER_TOKENS=1`. **Compat** (default) accepts the static keys and
logs `[ServerId] DEPRECATED: static <key> accepted on <route>` at most once per hour per route.

| Surface | Accepted |
|---|---|
| Mothership :90 trusted-server checks (`x-server-api-key`) | valid `server` token; static `SERVER_API_KEY` (compat only). Automation key unchanged. |
| :78 admin tier (16 routes, SEC-F2) | `server` token (bound, see below); dashboard admin key in `x-dashboard-key`; static master/server key (compat only); user key of a station/global admin |
| :78 writer tier (16 routes) | the above, or any issued user/deployment key |
| :78 ingest (`register_server`, `update_player_count`, `server_heartbeat`) | compat: open (as before). strict: a valid `server` token (`x-server-api-key` or `x-api-key`) is required → 401 |
| :78 `log_in_with_key` | `server` token → the master-key response shape (user `HalcyonServer`, all permissions on `global`) with `user_id` `halcyon-server:<deployment>`; static master key (compat only) |
| :78 purge / prune (maintenance) | dashboard admin key (`x-dashboard-key`), static master key (compat only). **Never** server tokens |
| :78 board upload | the above, or a `server` token for that station |
| :78 spec-level store check | `server` token; static key (compat only) |
| :9095 control API (all 5 routes) | dashboard admin key (`x-dashboard-key`); static master key in `x-api-key` (compat only) → else 401 `{"error":"unauthorized"}` |
| :9100 agents | see "Agent protocol" |

**Deployment binding (:78, every route incl. reads):** when a `server` token is presented, every
`deployment_id` the request names (route parameter, query string, or top-level JSON body property)
must equal the token's deployment, and every `station_id` it names must equal the token's station
(its own binding, else the station its deployment registered under; unknown → denied). Otherwise
**403 `{"error":"forbidden"}`**. A token for deployment A therefore cannot update B's player count or
fetch B's config. Routes that name neither (e.g. `/users/{id}/api_key`) still accept a server token
like the static key did (see KNOWN_ISSUES SEC-SID-3).

**Admins in strict mode:** the master key is a game key and is rejected. Use the dashboard (:8080)
admin key — on :78 send it as `x-dashboard-key` (purge/prune, admin routes), on :9095 likewise.

## Spin-up validation (F5)

`RequestSpinUp` (all callers) rejects, with the usual
`{"success":false,"request_id":"","agent":"","error":"invalid spin-up request: ..."}`:

* `map` not matching `^[A-Za-z0-9_./-]{1,128}$` or containing `..`; `region` not
  `^[A-Za-z0-9_-]{1,32}$`; `box` not `^[A-Za-z0-9_.-]{1,64}$`; `name` over 64 characters or outside
  letters/digits/space/`_.,'!?&()#:+-`.
* `args` with more than 32 tokens / 2048 characters, any token not matching
  `^-[A-Za-z][A-Za-z0-9_]*(=[A-Za-z0-9_./:,+-]*)?$`, or a denied flag: `ExecCmds`, `exec`, `dpcvars`,
  `cvarsini`, `run`, `abslog`, `log=<value>`, `userdir`, anything ending in `ini`, `ServerApiKey`,
  `DashboardApiKey`, `DashboardDeploymentId`, `RegisterIp`, `BackendHost`, `BackendPort`,
  `MothershipHost`, `MothershipPort`, `DashboardApiUrl` (reasons in `internal/halcyonsocket/security.go`).
  Callers can no longer choose the deployment id; only the watchdog's restart path re-uses one.

## Agent protocol (:9100, F6)

* Lines are capped at 64 KiB (also the unterminated buffer) — overflow disconnects.
* `hello` may carry `"token"`: an `agent` token whose box equals the claimed `box`. An invalid,
  revoked, foreign-box or non-agent token disconnects. A heartbeat re-validates it; revocation
  disconnects the agent.
* `SOLARA_AGENT_ALLOWED_IPS` (comma-separated IPs/CIDRs) authenticates agents that cannot send a
  token (the compiled `HalcyonAllocatorAgent.exe`).
* `SOLARA_REQUIRE_AGENT_AUTH=1`: an agent with neither is disconnected, and nothing but `hello` is
  processed before an admitted hello. Default: accepted with a warning.
* A `hello` claiming a box name held by another live connection is refused unless it carries a valid
  token for that box (it then replaces the other connection) or comes from the same IP.
* `spunup`/`exited` about launches this hub sent are accepted only from the launching connection, a
  connection from the same IP, or a holder of that box's agent token. Reports about pids the hub never
  launched keep the original behaviour (the watchdog keys them by box/pid).

## Rollout

1. **Deploy Solara** (compat by default — nothing changes for running servers; spin-ups now include
   per-server tokens, which are ignored while the static key is still on the command line, see 3).
   Make sure `DASHBOARD_ADMIN_KEY` is set (or note `config/generated-dashboard_admin_key.txt`): it is
   the admin credential for :9095 and, in strict mode, for :78 maintenance.
2. **Issue agent tokens** (server stopped): `solara agent-token issue --box <box_name>` per box.
   PowerShell agent: use `deploy/windows/HalcyonAgent.ps1` and put `AgentToken = 'sst1_...'` in
   `server.config.psd1` (or `-AgentToken` / env `HALCYON_AGENT_TOKEN`). Compiled agent
   (cannot send a token): add its public IP to `SOLARA_AGENT_ALLOWED_IPS`.
3. **Drop the static keys from agent configs.** Unreal's `FParse` takes the **first** occurrence of a
   flag, and agents put their base args *before* the spin-up args — so as long as `game_args`
   (`agent.cfg`) or `DashboardApiKey` / `ExtraArgs` (`server.config.psd1`) contain
   `-DashboardApiKey=` / `-ServerApiKey=`, servers keep authenticating with the static key. The
   updated PowerShell agent drops them automatically when a spin-up carries a token; for the compiled
   agent remove both flags from `game_args` by hand. Servers started outside the agent need
   `solara server-token issue --deployment <id>` and the token on their command line.
4. **Watch the logs** for `[ServerId] DEPRECATED` and `[HalcyonSocket] WARNING: unauthenticated agent`
   lines; when they stop, **enable strict modes**: `SOLARA_REQUIRE_SERVER_TOKENS=1` and
   `SOLARA_REQUIRE_AGENT_AUTH=1`, restart. Then rotate `SERVER_API_KEY` / `ServerMasterKey`.

Before step 4 confirm (one capture or a log line) that the DLL sends `x-api-key` or
`x-server-api-key` on `register_server` / `update_player_count` / `server_heartbeat`; strict mode
requires a token there, and this could not be verified without the DLL (SEC-SID-1).
