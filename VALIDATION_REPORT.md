# Validation report — Solara vs. original Halcyon

Date: 2026-10-25. Every figure below comes from a command actually run in this session.

## 1. Original baseline

| Check | Result |
|---|---|
| `dotnet build OrionDriftBackend.sln -c Debug` (snapshot in reference/) | **0 errors, 224 warnings** |
| `tests/Drafters` (only original test project) | **PASS: 24 Drafter authorization checks** |
| Running the original's listeners | not possible here (privileged `http://+:port/` prefixes need elevation/URL ACL) → original executed **in-process** for differential tests (ENV-002) |

## 2. Solara build and unit/compatibility tests

| Check | Result |
|---|---|
| `gofmt -l cmd internal tools` | 0 files |
| `go vet ./...` | clean |
| `go test -count=1 ./...` | **102 top-level tests passed, 0 failed, 8 packages** |
| `go test -race ./...` | **not run** — no C compiler (ENV-001) |
| Ported original suite (`internal/ares/drafters_original_test.go`) | **24/24** checks pass |
| Placeholder scan (`TODO`, `FIXME`, `not implemented` in non-test Go) | none |

Ground truth used by tests (all produced by the original .NET 6 runtime / original assemblies):

* `tools/oracle` — System.Text.Json escaping, number formats, DateTime kinds, JsonElement
  semantics; LiteDB collation (case-insensitive predicates), CRUD semantics, date/ms/Local
  behaviour, `EmptyStringToNull`/`TrimWhitespace` defaults.
* `tools/routes` — authoritative route table (`docs/original_routes.tsv`, 282 endpoints);
  every Go server's registration order is asserted equal.
* `tools/oracle-eos`, `tools/oracle-mothership`, `tools/oracle-ares`, `tools/oracle-halcyonsocket`
  — byte-exact JSON of every model (default + populated), JWT layout, quest blob, paging,
  fuzzy search (28 cases), agent-protocol replay, watchdog decisions.
* `tools/oracle-mothership-db`, `tools/oracle-ares-db` — LiteDB 5.0.21 store→reload→serialize
  round-trips for all **47 persisted models** (30 Mothership + 17 Ares cases), byte-equal.
* `internal/docdb/testdata/export` — a real encrypted LiteDB file exported by
  `tools/litedb-export` and imported by Solara (ids, order, Int64, dates, MinValue, nulls).

## 3. Differential testing (original vs Solara, same requests)

Harness: `tools/difftest-oracle` runs each scenario through the **original** servers
in-process (HttpListener.Start neutralised by Harmony; real constructors, routing, binding,
handlers, preprocessors, LiteDB via the real `AstraDatabase`); `tools/difftest` runs the same
scenario through Solara (`hosting.DisableListen`; real constructors/handlers, docdb) and compares
status code, content type, response headers, and body.

Normalisations (nondeterministic by construction only): GUIDs, 24-hex ObjectIds, 16-hex random
ids, ≥32-hex tokens, JWTs, ISO timestamps, epoch numbers, EOS correlation ids, attestation
nonces, random `Server_xxxxxx` default names, the per-process RSA modulus in JWKS, TURN HMAC
passwords (time-based username), and .NET stack traces in 500 pages (HOST-005). JSON bodies that
differ only in object key order are counted separately (DB-006).

| Scenario set | Steps | Differing | Equal modulo key order |
|---|---|---|---|
| sweep-anon (every endpoint, no credentials) — 6 servers | 282 | 0 | 4 |
| sweep-authed (every endpoint, all credentials, `{}` bodies) — 6 servers | 282 | 0 | 3 |
| smoke | 7 | 0 | 1 |
| flow-gameserver (register → player count → heartbeat → deployment config → EOS matchmaking → dashboard views → prune) | 15 | 0 | 2 |
| flow-mothership (Quest begin/complete with trusted key → token → user data, inventory, title data, server-authorized) | 11 | 0 | 0 |
| **Total** | **597** | **0** | **10** |

History: the first full run found 44 differences; the fixes were real defects in the port
(LiteDB `EmptyStringToNull`/trim semantics — 30+ Mothership responses and Ares role/config
values) plus missing normalisations. Run logs: `docs/validation/difftest-run1.log`, `docs/validation/difftest-run2.log`.

## 4. Live runtime check (real sockets)

`bin/solara.exe serve --config Debug` in a fresh directory: all seven listeners started in the
original order (78, 8080, 90, 50, 80, 9095 HTTP, 9100 TCP); `.solara.db` + `.meta` created.
Live requests: `POST :78/register_server` → 200 JSON with CORS headers; `GET :90/v1/title-data`
→ 401 JSON; `POST :50/matchmaking/v1/halcyon/filter` → 200 listing the just-registered server
(cross-module state); `:8080/api/stations` → 401 `{"error":"unauthorized"}` **without** CORS
headers (matches original HOST-007); `OPTIONS` → 204; `/` → embedded dashboard (text/html).

## 5. Coverage figures (with denominators)

| Measure | Figure |
|---|---|
| HTTP endpoints ported / registered in original order | 282 / 282 |
| HTTP endpoints compatibility-tested (differential, 2 auth modes) | 282 / 282 |
| C# model types (all Models/*.cs files) with a Go type | 105 / 105 (MIGRATION_INVENTORY MODEL rows, 0 missing) |
| Persisted models with LiteDB round-trip byte-equality | 47 / 47 |
| Original callable symbols (graph) linked to a Go symbol by name | 783 / 1058; the 275 others reviewed by category: renamed private helpers (spot-checked), static classes → package functions, marker interfaces / attributes / per-status result classes replaced by Go constructs, and the unreachable `AstraWebSocketServer` (HOST-009) |
| Original test checks passing in Go | 24 / 24 |
| Integration with live external services (Meta API, game-server DLL, Windows agent, production data) | 0 / 4 exercised live — protocol-level tests only (ENV-003) |

## 5b. After security hardening (difftest-run3)

All scenarios still match the original except `sweep-anon-A2StationDbServer`: 34 of 86 steps differ, all intended (32 guarded routes now 401 for anonymous callers; 2 reads that no longer show data the blocked anonymous writes created). Authenticated sweeps and both flows: 0 differences. A harness defect was also fixed (per-scenario cleanup of persisted EOS state had never been invoked). Go tests: all packages pass, including the new regression tests for F1, F2 and F4.

## 5c. After dashboard hardening (difftest-run4)

Differences vs the original are now exactly: the 34 station-API steps from 5b; every dashboard response differing only by the added security headers (106 sweep steps + 4 flow + 1 smoke), and `GET /` serving the hardened page. All Mothership, EOS, HalcyonSocket and authenticated station-API steps: 0 differences. New tests: brute-force lockout, generated-key file, security headers, log redaction, bind address, page encoding (`node --check` + attack strings against the page helpers).

## 5d. Server identity tokens + snapshots (difftest-run5)

Differences vs the original: unchanged from 5c, plus the 5 anonymous `:9095` HalcyonSocket requests now 401 (authenticated sweep: 0 differences). All packages pass (`serverid`, `snapshot` added). Live start: all listeners + startup snapshot (24 ms); CLI `snapshot list/verify/create/restore` exercised end-to-end offline (restore took a `pre-restore` safety snapshot; verification OK). Not verified against the real DLL / compiled agent (see docs/SERVER_IDENTITY.md risks).

## 6. What is NOT verified

* Race detector (ENV-001); behaviour under real concurrent load / performance baselines
  (no load test was run — the original serialises requests per server, and so does Solara).
* Live Meta graph API, the real HalcyonA2.dll and PowerShell agent, production LiteDB data.
* Self-elevation path (`MOTHERSHIP_SELF_ELEVATE`) — requires an interactive UAC/sudo prompt.
* Linux deployment: the binary cross-compiles (static ELF x86-64) and `GOOS=linux go vet ./...`
  is clean, but it was not executed on Linux here.
