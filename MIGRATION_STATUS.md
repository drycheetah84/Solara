# Migration status (continuation state)

Last updated: 2026-10-25, end of session 1.

## State
All six original projects are ported to Go and verified as described in VALIDATION_REPORT.md:
282/282 endpoints registered in original order and differential-tested (597 steps, 0
differences after normalisation), 47/47 persisted models round-trip byte-equal with LiteDB
5.0.21, original Drafters suite 24/24, 102 Go tests passing, live socket start verified.

## Open items (in priority order)
0. **Security** — F1–F6 addressed (SEC-F*, SEC-DASH-*, SEC-SID-*); F7/F8 (Mothership data exposure / cross-player delete) and L1–L5 still open. Roll out server/agent tokens per docs/SERVER_IDENTITY.md and capture real DLL traffic before enabling strict modes. Confirm the game client still works against the guarded :78 routes in staging.
1. **DEPLOY-001** — rewrite `install.sh` / `install.ps1` / `Redeploy-Vps.ps1` to deploy the
   Go binary (currently documented only in docs/BUILD_AND_DEPLOY.md).
2. **ENV-001** — run `go test -race ./...` on a machine with a C compiler.
3. **ENV-003** — staging checks with live dependencies: Meta graph API with real credentials,
   the deployed HalcyonA2.dll against :78/:90, the PowerShell allocator agent against :9100,
   and an import of a copy of the production LiteDB (`docs/DATA_MIGRATION.md`), then replay
   `tools/difftest` flows against both stacks.
4. **HOST-009** — decide whether the unreachable `AstraWebSocketServer` base class should be
   ported anyway (needs user authorization to drop).
5. **DB-006** — if exact JSON key order of `Dictionary` members is ever required, switch those
   members to an insertion-ordered map (`stj.Obj`-style) in `internal/ares/models`.
6. Extend `tools/difftest/scenarios` with more stateful flows (moderation, events/signups,
   offers/inventory, lobbies over WS) — the harness supports `${secret:}`, `${var:}` and
   `capture`.

## How to resume
* Read docs/PORTING_GUIDE.md and KNOWN_ISSUES.md; query the graph:
  `graphify query "<symbol>"` (graphify-out/graph.json has original + Go nodes and
  `ported_to` edges; rebuild with `graphify extract . --code-only --no-gitignore --force`
  then `tools/scripts/graph_migration_links.py` with the graphify interpreter).
* Rebuild the original for oracles: `dotnet build reference/halcyon/backend/OrionDriftBackend.sln -c Debug`
  (the reference snapshot is git-ignored; recreate it from the original path if absent,
  excluding `.env`).
* Re-run everything: `go vet ./... && go test ./...` and `bin/difftest all ...` (see
  docs/BUILD_AND_DEPLOY.md).
