# Building, testing and deploying Solara

## Requirements

| Need | Version | Why |
|---|---|---|
| Go | ≥ 1.25 (module pins `go 1.25`; developed with 1.27) | build/run Solara |
| .NET SDK | 6+ (SDK 10 used; .NET 6 runtime required) | **tools only**: LiteDB export, differential oracle, route/fixture oracles |
| C toolchain | optional | only for `go test -race` (cgo) |

No native/Unreal toolchain is needed: the backend has no native components (see ARCHITECTURE.md).

## Build

```bash
go build -o bin/solara ./cmd/solara            # Linux
go build -o bin/solara.exe ./cmd/solara        # Windows
GOOS=linux GOARCH=amd64 go build -o bin/solara ./cmd/solara   # cross-compile for the VPS
```

## Secrets (once per deployment)

```bash
tools/scripts/extract-legacy-secrets.sh <path-to-original backend/src>   # writes config/legacy-secrets.env (0600)
bin/solara check-secrets
```

Ship `config/legacy-secrets.env` next to the binary (`<install>/config/legacy-secrets.env`) or set
`SOLARA_<NAME>` environment variables. Never commit it.

## Run

```bash
cd <working dir that holds the database and logs>
<install>/solara serve                 # Release behaviour: production.solara.db (encrypted)
<install>/solara serve --config Debug  # development.solara.db (as run-backend.cmd did)
```

Runtime environment variables are unchanged from the original (docs/CONFIGURATION.md):
`MOTHERSHIP_INSECURE`, `MOTHERSHIP_ALLOW_NONSTORE`, `HALCYON_SERVER_API_KEY`,
`DASHBOARD_AUTH_ENFORCE`, `DASHBOARD_ADMIN_KEY`, `DASHBOARD_SESSION_SECRET`,
`BOARD_PUBLIC_BASE`, `MOTHERSHIP_SELF_ELEVATE`, watchdog tunables.

Copy `cosmetic_catalogue.txt` and `auto_complete_quests.txt` (from the original `backend/`)
next to the binary, as the .NET publish did.

Ports bound: 90, 78, 8080, 50, 80, 9095, 9100 (all interfaces). On Linux, binding <1024
needs `CAP_NET_BIND_SERVICE` (`setcap 'cap_net_bind_service=+ep' bin/solara`) or root; on
Windows no URL ACLs are required (Go uses plain sockets, not HTTP.sys). A port already in use
aborts startup, as in the original.

### systemd (replacing the .NET unit written by install.sh)

```ini
[Service]
User=halcyon
WorkingDirectory=/opt/halcyon/backend
EnvironmentFile=/etc/halcyon/backend.env
ExecStart=/opt/halcyon/bin/solara serve --config Release
AmbientCapabilities=CAP_NET_BIND_SERVICE
Restart=always
```

Shutdown: SIGTERM/SIGINT stops servers and closes the database (original had no graceful
shutdown). Restart is safe at any time (bbolt is crash-consistent).

## Migrate existing data

See docs/DATA_MIGRATION.md (`tools/litedb-export` → `solara import-litedb`).

## Tests

```bash
gofmt -l . ; go vet ./... ; go test ./...             # unit + compatibility tests
go test -race ./...                                    # needs cgo (C compiler)
```

Differential tests against the original (needs .NET 6 and the built original):

```bash
dotnet build reference/halcyon/backend/OrionDriftBackend.sln -c Debug
dotnet build -c Release tools/difftest-oracle
cp -n reference/halcyon/backend/src/AUnrealFeatures.Ares/bin/Debug/net6.0/*.dll tools/difftest-oracle/bin/Release/net6.0/
go build -o bin/difftest ./tools/difftest
bin/difftest all tools/difftest/scenarios tools/difftest-oracle/bin/Release/net6.0/difftest-oracle.dll
```

Regenerate the authoritative route table: `dotnet run --project tools/routes > docs/original_routes.tsv`.

## Logging

Console + `logs/log-YYYYMMDD[_NNN].txt` under the working directory, same templates as the
original Serilog setup.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `legacy secrets not configured` at start | run the extraction script / set `SOLARA_<NAME>` |
| `Invalid password` opening the DB | `PRODUCTION_DB_PASSWORD` differs from the one used at import |
| `bind: address already in use` | another process (or the old .NET backend) holds one of the ports |
| no stations listed in game | build version mismatch (BuildVersion constant) — same as original |
