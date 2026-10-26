# A2 Pak Config Editor

A local tool for editing `DefaultEngine.ini` / `DefaultGame.ini` (and other
`*/Config/*.ini` entries) inside an Unreal Engine `.pak` file, and writing the
pak back without damaging it.

- Backend: `internal/pak` (pak v8 and v11 reader/writer/verifier, Oodle via
  a user-supplied DLL, safe save, ini handling) and `cmd/pak-editor` (local
  HTTP server).
- Frontend: React app in `web/pak-editor/`, built into `cmd/pak-editor/ui/`
  and embedded into the binary with `go:embed`.

## Build and run

```sh
# 1. frontend (optional: without it a placeholder page is served)
cd web/pak-editor && npm install && npm run build   # outputs to cmd/pak-editor/ui
# 2. backend
go build -o pak-editor.exe ./cmd/pak-editor
./pak-editor.exe              # opens http://127.0.0.1:<random port>/?t=<token>
./pak-editor.exe --port 8765 --no-browser
```

In the UI, paste the game folder (e.g. `...\AnotherAxiom-A2-30907\1.0.30907`),
the `A2` project folder, the `Content` folder, the `Paks` folder or the `.pak`
path itself. The scanner looks for `**/Content/Paks/*.pak` up to 6 levels deep.
The pak list shows each pak's version and compression.

**Close the game and its launcher before saving.**

## Supported formats

| Pak | Version | Index | Compression | Read | Edit |
|---|---|---|---|---|---|
| PC build (`A2-Windows.pak`, 30907) | 8 | legacy (full `FPakEntry` list) | none | yes | uncompressed entries |
| Quest build (`A2-Android_ASTC.pak`, 35232627) | 11 | encoded entries + path hash index + full directory index | Oodle (955 of 2832 entries, incl. both ini files) | yes (compressed entries need the Oodle DLL) | any unencrypted entry |
| other versions (1-7, 9, 10, 12+) | | | | reported as unsupported | no |
| encrypted index or entries | | | | no | no |

Only the `Oodle` compression method can be decoded; entries using another
method fail with a clear "unsupported compression" error. IoStore containers
(`.utoc` / `.ucas`) are not touched.

## Oodle setup (Quest / Oodle-compressed paks)

The Oodle DLL is **not** shipped with or copied into this tool. Point it at
one you already have:

- `oo2core_9_win64.dll` (Unreal Engine install:
  `Engine/Binaries/ThirdParty/Oodle/Win64/`, or a game that ships it), or
- UE5's `oodle-data-shared.dll` (exports `OodleLZ_Decompress` and
  `OodleLZ_Compress`).

The DLL is looked up in this order:

1. the path saved in the UI: header button **Oodle settings** (start screen)
   or **Oodle** (editor). The dialog also opens by itself when a compressed
   file cannot be read;
2. the `PAK_EDITOR_OODLE_DLL` environment variable;
3. `oo2core_*_win64.dll` / `oodle-data-shared.dll` next to `pak-editor.exe`.

A path is only saved after the DLL was loaded, exports `OodleLZ_Decompress`
and (if it exports `OodleLZ_Compress`) passed a compress/decompress
self-test. The setting is stored in `%APPDATA%\pak-editor\settings.json`
(`{"oodleDll": "..."}`); clearing it returns to auto-detection. The DLL is
called through `syscall` (no cgo); non-Windows builds report Oodle as
unavailable. Loading a DLL runs its code inside pak-editor, so only use a
DLL from a source you trust. The console prints which DLL is in use at
startup.

## Recompress option (v11)

By default an edited entry is written **uncompressed** (compression method 0,
no blocks). UE reads uncompressed entries inside a compressed pak normally,
and no Oodle code is involved in writing, so this is the safest choice. The
pak grows by roughly the compression savings of the edited files (about
8 KB for `DefaultGame.ini`).

The review dialog offers **Recompress edited files with Oodle** (off by
default; API `"recompress": true`). Edited entries that were
Oodle-compressed are then compressed again with Oodle **Kraken, level
Normal** (the original blocks are Kraken too, block header `8c 06`), split
at the entry's original block size (64 KiB for multi-block entries). Every
block is decompressed again and compared before the pak is written, and the
verification step decompresses the edited entries once more. Entries that
were uncompressed stay uncompressed, and an entry whose compressed form would
not be smaller is stored uncompressed. The compressed bytes differ from
UnrealPak's output (other compression level), which does not matter to the
reader.

## Quest builds: the pak lives inside an OBB

Quest/Android builds ship the pak inside the APK expansion file
(`main.<version>.<package>.obb`, a zip archive) or in
`/sdcard/Android/obb/<package>/`. This tool only edits a `.pak` file on
disk: extract the pak first, edit it here, then put the edited pak back into
the OBB (or obb folder) **the same way it came out**: same path inside the
archive, stored the same way (e.g. "store"/uncompressed if it was stored).
Then deploy it to the headset again. Repacking the OBB/APK, signing and
pushing with adb are out of scope for this tool.

## Safety model

Every save (and restore) goes through the same pipeline in
`internal/pak/save.go`:

1. **Pre-checks**: entries must exist and be unencrypted (v8: also
   uncompressed); the pak must not be open by another process (Windows: a
   test open with DELETE access); free disk space must cover the temp file
   plus backups plus a 64 MB margin. The server also refuses if the entry
   changed on disk since the UI loaded it (SHA-1 compare, HTTP 409), if the
   edited text has validation errors (422), or if nothing changed (400).
2. **Write** a complete new pak to `<pak>.solara-tmp` in the same folder,
   then fsync it.
3. **Verify** the temp file fully: footer and index hash (v11: also the path
   hash index and full directory index SHA-1s, every path hash against the
   FNV-64 of its entry name, the pruned directory index against the entry
   names), every entry's inline header against the index, every entry's
   SHA-1 (over the stored, i.e. possibly compressed, bytes, as UE does), same
   version, mount point, entry order, physical order, flags and (v11) index
   structure as the source, every untouched entry byte-identical to the
   source, and the replaced entries equal to the intended new bytes
   (decompressed first when recompressed).
4. **Back up** the current pak: the first time ever, `<pak>.original` (never
   overwritten or deleted by the tool); every time, a timestamped
   `<pak>.bak-YYYYMMDD-HHMMSS`. Backups are written via a `.partial` file and
   checked by SHA-256. Only the newest 10 timestamped backups are kept.
5. **Swap**: atomic `MoveFileEx(REPLACE_EXISTING | WRITE_THROUGH)` of the temp
   file over the pak. If the pak is in use the save aborts with "close the
   game / launcher first"; the original stays untouched and the temp file is
   removed.

If any step fails, the original pak is not modified. A crash during writing
leaves only a stale `.solara-tmp`, which the next save removes.

**Restore** (`POST /api/restore`) only accepts one of the pak's own backups,
verifies it, copies it to the temp file, verifies the copy, backs up the
current pak and swaps, exactly like a save.

## Format notes (A2-Windows.pak, build 30907)

- Pak version 8, 221-byte footer (5 compression-name slots, all empty),
  legacy index, mount point `../../../`, 4281 entries, all uncompressed and
  unencrypted, CompressionBlockSize 0.
- **Physical order differs from index order** (e.g. `A2/AssetRegistry.bin`
  is index #1 but sits at offset 137360). The writer walks entries in
  on-disk order and writes the index in index order.
- Entry data is contiguous from offset 0 up to the index; there are no gaps.
  Gaps (if any pak had them) are copied verbatim.
- Each entry region is a 53-byte inline FPakEntry (offset field 0) followed
  by the data.
- Rebuilding the real pak with no edits is byte-identical (SHA-256
  `7d8d459f45bad0c8a4765226d29dfb18f34a2866cbb6bb57613f19e15698c1d5`).
- Both ini files are UTF-8 without BOM, CRLF, with a trailing newline. The
  editor gets LF text and saves CRLF again.
- The shipped `DefaultEngine.ini` has two lines with an extra `)` (lines 1533
  and 1534, `+DeltaCompressionConfigs`); the validator reports these as
  warnings, which do not block saving.

## Format notes (A2-Android_ASTC.pak, Quest build 35232627)

- Version 11, 221-byte footer, compression slot 1 = `Oodle`, index not
  encrypted, mount `../../../`, path hash seed `0xabd9dff0`, 2832 entries
  (1877 uncompressed, 955 Oodle/Kraken), 692 directories.
- Layout: entry data `[0, 73803535)` without gaps, then primary index
  (42170 B), path hash index (65963 B), full directory index (106264 B),
  footer. The writer keeps the relative order and any gap bytes.
- Primary index: mount, entry count, seed, `bHasPathHashIndex` (u32) +
  offset/size/SHA-1, `bHasFullDirectoryIndex` (u32) + offset/size/SHA-1,
  42056 B of encoded entries, 0 non-encoded entries, no trailer. The
  encoded-entry order differs from the directory index order; both are
  preserved.
- Path hash index: 2832 `{u64 hash, i32 location}` records followed by a
  **pruned directory index** (264 directories / 908 files, also with
  locations, which are remapped on rebuild). Hash = FNV-1a-64 over the
  UTF-16LE bytes of the lower-cased mount-relative path, offset basis
  `0xcbf29ce484222325 + seed`. All 2832 stored hashes match.
- Encoded entries: bits 31/30/29 = offset / uncompressed size / size fit in
  32 bits, bits 23-28 method index, bit 22 encrypted, bits 6-21 block count,
  bits 0-5 block size >> 11 (`0x3f` = full u32 follows). Single-block
  entries store 0 in the block-size bits (the decoder uses the uncompressed
  size) and no block sizes; multi-block entries store 32 (64 KiB) and one
  u32 compressed size per block (blocks are contiguous after the inline
  header).
- Inline headers are the same FPakEntry as v8 (53 B, +4 +16 per block for
  compressed entries, block offsets relative to the entry). For single-block
  entries the inline compression block size equals the uncompressed size.
  The SHA-1 covers the stored (compressed) bytes.
- `DefaultEngine.ini`: 16665 B stored / 124968 B raw, 2 blocks;
  `DefaultGame.ini`: 3513 B / 11819 B, 1 block; both CRLF UTF-8.
- No-op rebuild is byte-identical (SHA-256
  `61086cc5b888ae1ac50f614242458e1e254ea8b9bb4db66e6d9800f392c5a088`).
- The hand-modified `A2-Android_ASTC.pak` next to the `.orig` differs only
  in `DefaultEngine.ini` (same stored size, all hashes valid):
  `[OnlineSubsystemOculus] MobileAppId` `7181769438517329` ->
  `2236834297114138`.

## Limitations

- Only pak **versions 8 and 11** with an unencrypted index can be opened;
  other versions are detected and reported as unsupported.
- Only **unencrypted** entries can be replaced (v8: also uncompressed).
  Encrypted entries are copied through unchanged. Reading compressed v11
  entries needs the Oodle DLL; replacing them uncompressed does not (the
  server still needs it to load the original text for editing).
- The IoStore container files (`.utoc` / `.ucas`, `global.*`) are not read or
  changed.
- The game must be closed while saving.
- It is unknown whether the game or its launcher checks pak integrity some
  other way (signatures, a launcher file check, anti-cheat). A launcher
  "verify files" step may replace the edited pak with a fresh download. Use
  `.original` or Restore to undo.
- Possible future alternative: write a small patch pak (`A2-Windows_P.pak`)
  containing only the edited ini files and leave the original pak alone. This
  depends on the engine's patch-pak priority rules and on the game accepting
  unsigned paks, and has not been tested.

## HTTP API

The server binds only to `127.0.0.1`. Every `/api` request needs the header
`X-Session-Token: <token from the URL>` (else 401). The `Host` header must be
`127.0.0.1:<port>` or `localhost:<port>` (else 403; this blocks DNS
rebinding). No CORS headers are sent. POST bodies must be `application/json`
and at most 16 MiB. Errors are `{"error": "message"}`, some with a
machine-readable `"code"`.

| Method | Path | Body / query | Response |
|---|---|---|---|
| POST | `/api/scan` | `{"path"}` | `{"root", "paks": [{"path","name","size","version","entries","supported","configFiles","compressionMethods","oodleRequired","oodleAvailable","error"?}]}` |
| GET | `/api/file` | `?pak=&name=` | `{"name","text","lineEnding","bom","size","sha1","sections","issues","compression"}`; 422 `{"error","code":"oodle_required"}` when the entry is Oodle-compressed and no DLL is loaded |
| POST | `/api/validate` | `{"text"}` | `{"issues","sections"}` |
| POST | `/api/save` | `{"pak","files":{name:text},"expectSha1":{name:sha1},"recompress"?:bool}` | 202 `{"jobId"}`; 409 stale, 422 `{"error","issues":[{"file","line","severity","message"}]}`, 400 `"no changes"` |
| GET | `/api/jobs/{id}` | | `{"state","phase","progress","error"?,"result"?:{"backup","originalBackup","newSize","changed","recompressed"}}` |
| GET | `/api/backups` | `?pak=` | `{"backups":[{"path","name","size","modTime","kind"}]}` |
| POST | `/api/restore` | `{"pak","backup"}` | 202 `{"jobId"}` |
| GET | `/api/settings` | | `{"oodleDll","oodleAvailable","oodlePath","oodleSource","oodleCanCompress","oodleSupported","oodleError"?}` |
| POST | `/api/settings` | `{"oodleDll": path}` (`""` = auto-detect) | same as GET; 400 `{"error","code":"oodle_invalid"}` if the DLL cannot be loaded or lacks `OodleLZ_Decompress` |

Only one save or restore can run at a time (a second one gets 409). `sha1`
is the entry's index SHA-1 (of the stored, possibly compressed, bytes).
`compressionMethods` is the footer's non-empty method names; `oodleRequired`
is true when a config file is Oodle-compressed; `compression` is the entry's
method name (`""` = stored uncompressed). `recompress` is refused for v8
paks (400) and without a loaded DLL (422 `oodle_required`).

## Tests

```sh
go test -count=1 ./internal/pak/... ./cmd/pak-editor/...
# opt-in, uses a temp copy; the real pak is only read:
A2_PAK_TEST="C:/.../A2/Content/Paks/A2-Windows.pak" go test -count=1 -run 'TestRealPak$' -v ./internal/pak/
# v11 / Quest: (a) no-op rebuild identical, (b) DefaultGame.ini edit stored
# uncompressed, (c) the same with recompress, (d) DefaultEngine.ini (2 blocks)
# round trip. (b)-(d) need the DLL; PAK_EDITOR_OODLE_DLL also enables the
# synthetic real-Oodle test.
A2_PAK11_TEST="C:/.../A2-Android_ASTC.pak.orig" PAK_EDITOR_OODLE_DLL="C:/.../oo2core_9_win64.dll" \
  go test -count=1 -run 'TestRealPak11|Oodle' -v ./internal/pak/
# UI
cd web/pak-editor && npx tsc --noEmit && npm test
```

The synthetic v11 tests build paks with `pak.WriteNew(..., &WriteOptions{Version: 11})`
and use a flate-based stand-in codec registered as "Oodle", so the
compressed code paths (multi-block entries, recompress, verification) are
covered without the DLL.

## pakinspect

```sh
# summary of one pak, or the differing entries of two (with unified diffs of
# small text entries, decompressed when Oodle is available)
go run ./tools/pakinspect [--verify] [--oodle <dll>] a.pak [b.pak]
```
