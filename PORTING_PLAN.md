# Porting plan (as executed)

1. Reconnaissance of the original tree; baseline build/test (VALIDATION_REPORT §1).
2. Graphify graph of the original (code AST), later extended with Go + `ported_to` links.
3. Ground-truth oracles on the original runtime (STJ, LiteDB, route table, models).
4. Foundation in Go: dotnet, stj, hosting, docdb, di, logging, secrets, host.
5. Parallel module ports against docs/PORTING_GUIDE.md: EOSSDK, HalcyonSocket, Mothership,
   Ares station API (+ models, stores), Ares dashboard.
6. Integration: host wiring (Program.cs), cross-module hooks, framework fixes found by ports.
7. Verification: per-package tests, ported Drafters suite, differential harness (original
   in-process vs Solara), LiteDB round-trip oracles → fixed LiteDB string semantics.
8. Audit + documentation (inventory, matrix, known issues, validation, deployment).
Remaining work: MIGRATION_STATUS.md "Open items".
