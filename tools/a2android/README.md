# A2 Android (Quest) OBB / pak patching

Edit a file inside `A2-Android_ASTC.pak` (e.g. `A2/Config/DefaultEngine.ini` app ids) and rebuild the OBB so the engine accepts it.

    py -3 -I pakpatch.py <orig.pak> <out.pak> A2/Config/DefaultEngine.ini <edited.ini>
    py -3 -I obbclone.py <orig.obb> <out.obb> A2/Content/Paks/A2-Android_ASTC.pak <out.pak>
    py -3 -I ziplayout.py <out.obb>     # local/central extra lengths must match (36/36)

Then set the OBB size (`const-wide v2, 0x<size>L`) in `smali/com/AnotherAxiom/A2/a.smali`, recompile, `adb install -r`, push the OBB as
`/sdcard/Android/obb/<pkg>/main.<versionCode>.<pkg>.obb`.

Gotchas (each one cost a boot loop):
- Configs in the pak are Oodle-compressed: never hex-edit text in place. `pakpatch.py` appends an uncompressed entry and rebuilds
  encoded entries, path-hash index, directory index, index SHA1s and footer (v11, unsigned, unencrypted index).
- Every OBB entry must be STORED (method 0) — Unreal reads paks straight out of the zip.
- Don't write the OBB with Python `zipfile`: past 2 GB it emits local extra=0 / central extra=12, and Unreal computes data offsets
  from the central record -> utoc read 12 bytes late -> "TOC header magic mismatch (Corrupt Toc)". `obbclone.py` copies the
  original headers byte-for-byte.
- The engine log is at `/sdcard/Android/data/<pkg>/files/UnrealGame/A2/A2/Saved/Logs/A2.log` (Shipping still writes it).
  "Game files required to initialize the global shader…" on a splash hang = pak/OBB unreadable.
- Oodle: `OODLE_DLL` env var, default Epic's `oodle-data-shared.dll`.
