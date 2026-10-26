# Database snapshots

Point-in-time copies of the Solara document store that you can roll back to
after a bad deploy, an operator mistake or an attack. Code: `internal/snapshot`
(library), `internal/docdb/snapshot_support.go` (store hooks),
`internal/host/snapshots.go` (server scheduler), `cmd/solara/cmd_snapshot.go` (CLI).

## What is captured

Each snapshot is a directory `<snapshots dir>/<UTC yyyymmddThhmmssZ>_<label>/`:

| File | Source |
|---|---|
| `production.solara.db` (or `development.solara.db`) | Consistent copy of the live bbolt file, taken inside one bbolt read transaction (`tx.WriteTo`). Writers keep running; the copy is exactly the committed state at the start of the transaction. |
| `production.solara.db.meta` | The collection sidecar, captured under the same lock that guards collection creation/drop, so it matches the database copy. |
| `sessions.json`, `lobbies.json` | EOS gateway state from the executable directory, when present (the scheduler always captures them; the CLI unless `--no-eos`). |
| `manifest.json` | `format` (`solara-snapshot/1`), `created_at` (RFC3339 UTC), `label`, `reason`, `db_name`, `db_file`, `encrypted`, `files[]` (`name`, `size`, `sha256`), `collections[]` (`name`, `documents` — bbolt key counts, payloads are never decrypted), `source_host`, `eos_captured`, `meta_captured`. |
| `verify.json` | Cached result of the last verification (not part of the hashed set). |

Snapshots are written to a `.tmp-<name>` directory, every file is fsynced, and
the directory is renamed into place — a crash never leaves a half-written
snapshot that looks complete (stale `.tmp-*` directories are ignored and may be
deleted by hand).

**Not captured:** `playerdata/` (EOS player data blobs), `logs/`, `config/`,
secrets. Back those up separately if you need them.

## Encryption

Snapshots are byte copies. The Release store is AES-256-GCM encrypted with a key
derived from `PRODUCTION_DB_PASSWORD`; its snapshots stay encrypted and are never
decrypted. Keep the password: a snapshot is useless without it (and so is the
live database). Verification with a password re-derives the key from the stored
salt and checks the `__solara` check value — it proves the password is right
without decrypting any document.

## Schedule and retention (server)

While `solara serve` runs, the scheduler takes:

- one snapshot labelled `startup` when the server starts (`SOLARA_SNAPSHOT_ON_START=0` disables);
- one snapshot labelled `auto` every `SOLARA_SNAPSHOT_INTERVAL_HOURS` (default 6, `0` disables).

Each snapshot is verified right after it is written (result cached in
`verify.json`) and the directory is then pruned to `SOLARA_SNAPSHOT_KEEP`
(default 28 = one week at 6 h). Failures are logged under the `Snapshots`
logger and never stop the server. Shutdown waits for an in-progress snapshot
before the database is closed.

Pruning rules (`solara snapshot prune` and the scheduler):

1. the newest N snapshots are kept;
2. the newest snapshot whose cached verification passed is never deleted;
3. snapshots labelled `keep` or `pre-restore` are never deleted unless `--force`;
4. directories without a readable manifest are left alone.

Disk use: each snapshot is roughly the size of the database file. Plan for
`KEEP × db size` plus headroom.

## CLI

```
solara snapshot create  [--label L] [--reason R] [--config Release|Debug] [--dir D] [--no-eos]
solara snapshot list    [--dir D] [--verify]
solara snapshot verify  <name|latest> [--dir D]
solara snapshot restore <name|latest> [--config Release|Debug] [--include-eos] [--yes] [--dir D]
solara snapshot prune   [--keep N] [--force] [--dir D]
```

Run the CLI from the server's working directory (where `production.solara.db`
lives) so the paths match. Release needs `PRODUCTION_DB_PASSWORD` configured
(same secret store as `serve`).

### CLI vs. a running server

bbolt allows one process per database file: a read-write handle (the running
server) holds an exclusive file lock and a read-only open needs a shared lock,
which conflicts on both Windows (`LockFileEx`) and Linux (`flock`). So while
Solara runs, the CLI **cannot** read the database: `snapshot create` and
`snapshot restore` detect the lock within ~2 s and print a message instead of
hanging. Live snapshots therefore come from the server's scheduler (restart the
server, or wait for the next interval, to get a fresh one); offline snapshots
come from the CLI with the service stopped. `list`, `verify` and `prune` only
touch the snapshot directory and work at any time.

Use `--label keep` for snapshots you want retained indefinitely (e.g. before a
release).

## Restore procedure

1. Stop the service (`systemctl stop solara` / stop the Windows service or console).
2. `solara snapshot list --verify` and pick a snapshot (or use `latest`).
3. `solara snapshot restore <name> [--include-eos]` and type `restore` to confirm
   (`--yes` skips the prompt for scripts).
4. Start the service.

What restore does, in order — any failure before step 5 leaves the live files untouched:

1. refuses if the live database is locked (server still running);
2. verifies the snapshot (hashes, bbolt structural check, document counts, and
   for an encrypted snapshot that the configured password decrypts it). A wrong
   or missing password, or an unencrypted snapshot for a password-protected
   database, is a refusal;
3. refuses a snapshot of a different database (production vs development);
4. takes a safety snapshot of the current state labelled `pre-restore`
   (database, sidecar and EOS files) — protected from pruning;
5. stages verified copies next to the live files, moves the live files aside as
   `*.replaced-<ts>`, renames the copies into place, re-checks the installed
   store, and only then deletes the `*.replaced-<ts>` files. Any error rolls the
   renames back.

`--include-eos` also restores `sessions.json` / `lobbies.json`; a file that did
not exist when the snapshot was taken is removed (it is preserved in the
`pre-restore` snapshot). Without the flag the EOS files are left alone. If a
snapshot has no `.meta` file the live sidecar is kept.

To undo a restore, restore the `pre-restore` snapshot it created.

Note: the `.meta` sidecar stores the absolute database path it was created
with (as the original AstraDatabaseBytedata did). Restore into the same
directory the snapshot was taken from; when moving to another machine or
directory, use the same working-directory path.

## Disaster recovery and off-box copies

Snapshots on the same disk protect against bad deploys and application-level
damage, not against losing the machine or an attacker with filesystem access.
Copy them off the box regularly, for example:

```
# Linux (cron, after the 6-hourly snapshot)
rsync -a --ignore-existing /opt/solara/snapshots/ backup@vault:/srv/solara-snapshots/
# Windows
robocopy C:\Solara\snapshots \\vault\solara-snapshots /E /XC /XN /XO
```

Prefer an append-only / versioned target (object storage with object lock,
a backup account that cannot delete) so a compromised server cannot erase the
off-box history. Copied snapshots stay encrypted; store the database password
separately from them. Run `solara snapshot verify <name> --dir <copy>` on the
backup side to confirm integrity, and periodically rehearse a restore on a
spare machine.
