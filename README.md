# Solara

Go port of the **Halcyon** backend (Orion Drift / A2 private server): Mothership (:90),
station API (:78), dashboard API (:8080), EOS gateway (:50/:80), HalcyonSocket orchestrator
(:9095/:9100) — same routes, JSON, protocols, configuration and data model as the original
.NET 6 implementation.

```bash
tools/scripts/extract-legacy-secrets.sh /path/to/halcyon/backend/src   # once
go build -o bin/solara ./cmd/solara
bin/solara serve                       # Release behaviour (production.solara.db)
```

| Document | Contents |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | what the original is, migration boundary, design decisions |
| [MIGRATION_INVENTORY.md](MIGRATION_INVENTORY.md) | every endpoint/model/feature → Go location + status |
| [COMPATIBILITY_MATRIX.md](COMPATIBILITY_MATRIX.md) | contract classes → mechanism → proof |
| [VALIDATION_REPORT.md](VALIDATION_REPORT.md) | baseline, tests, differential results, coverage, gaps |
| [KNOWN_ISSUES.md](KNOWN_ISSUES.md) | preserved original defects, deviations, limits |
| [MIGRATION_STATUS.md](MIGRATION_STATUS.md) | continuation state / next steps |
| [docs/BUILD_AND_DEPLOY.md](docs/BUILD_AND_DEPLOY.md) | build, run, deploy, test, troubleshoot |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | env vars, files, secrets, ports |
| [docs/DATA_MIGRATION.md](docs/DATA_MIGRATION.md) | LiteDB → Solara data migration |
| [docs/PORTING_GUIDE.md](docs/PORTING_GUIDE.md) | conventions used for the port |

Legacy identifiers (C# type/route names, `x-*` headers, `HALCYON_*`/`MOTHERSHIP_*` env vars,
"Astra" log buckets) are kept wherever renaming would break compatibility.
The knowledge graph (original + Go, with `ported_to` links) is in `graphify-out/`.
