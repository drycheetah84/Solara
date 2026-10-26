# Migrating existing Halcyon data (LiteDB) to Solara

The original stored everything in LiteDB 5.0.21 files in its working directory:
`production.db` (Release, password-protected) or `development.db` (Debug), each with a
`<file>.meta` sidecar listing collections. Solara uses its own store
(`production.solara.db` / `development.solara.db`) with the **same collection names, document
member names and `_id` ObjectIds**, so data migrates without transformation.

The process is read-only on the original files and never overwrites non-empty Solara
collections unless `--overwrite` is given.

1. **Stop the original backend** (LiteDB `Direct` mode holds an exclusive lock), and back up
   `production.db` and `production.db.meta`.
2. Export with the original LiteDB library (requires the .NET 6+ runtime):

   ```bash
   dotnet build -c Release tools/litedb-export
   export LITEDB_PASSWORD='<production DB password>'   # omit for development.db
   dotnet tools/litedb-export/bin/Release/net6.0/litedb-export.dll /path/to/production.db ./export
   unset LITEDB_PASSWORD
   ```

   This writes one `<collection>.jsonl` per collection (LiteDB extended JSON), `manifest.tsv`
   (document counts) and a copy of the `.meta` sidecar.
3. Import into Solara (from the directory Solara will run in, with secrets configured):

   ```bash
   solara import-litedb --from ./export --config Release
   ```

   It prints the document count per collection; compare with `export/manifest.tsv`.
4. Start Solara in the same working directory: `solara serve --config Release`.

Verified by `internal/docdb/import_test.go` against a fixture produced by the real LiteDB
5.0.21 with encryption (ObjectIds, `_id` order, Int64 beyond 2^53, doubles, dictionaries,
nulls, `DateTime.MinValue`, millisecond-precision dates read back as Local kind).
