# solara-migration

> Solara = Go port of the .NET 6 Halcyon backend; where state, oracles and open items live
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

Solara (C:\Users\drych\ProjectDrift\Solara) is the Go port of the Halcyon backend whose original is the .NET 6 solution at OneDrive\Documents\Unreal Projects\rigel-server\halcyon-server\backend (read-only; snapshot in reference/, git-ignored). The "AUnrealFeatures.*" dirs are .NET projects, not Unreal C++ — no native bridge needed.

Session 1 (2026-10-25) finished the port: 282/282 endpoints, differential harness 597 steps / 0 diffs, 47 LiteDB round-trip oracles. Open items live in MIGRATION_STATUS.md (deploy scripts, -race needs a C compiler, live-dependency staging checks, HOST-009 authorization).

**Why:** future sessions must resume from repo docs + graphify-out, not conversation memory (CLAUDE.md §4.4).

**How to apply:** start from MIGRATION_STATUS.md and docs/PORTING_GUIDE.md. Non-obvious traps: the tool layer decodes backslash-u+4hex in written text; LiteDB predicates are case-insensitive and EmptyStringToNull/TrimWhitespace apply; `graphify update` honors .gitignore (drops reference/) — rebuild with `graphify extract . --code-only --no-gitignore --force` then tools/scripts/graph_migration_links.py.
