# a2-android-obb-patching

> Quest A2 client — how to edit pak configs + rebuild OBB so it boots; where the engine log is; size-check smali
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

Quest client package com.AnotherSaxaphone.GayTwo (versionCode 69971477 since 2026-11-01; was 69971476), decompiled tree in APK Easy Tool "1-Decompiled APKs/A2-Android-Shipping-arm64 (2)"; user recompiles, Claude installs with `adb install -r` (keeps OBB/data). Booted + reached Solara 2026-10-31 (midnight redirects index.json/v1/region).

- Check the ENGINE log first: /sdcard/Android/data/<pkg>/files/UnrealGame/A2/A2/Saved/Logs/A2.log (Shipping writes it; logcat has almost nothing native). User was annoyed I didn't read logs early.
- Tools: tools/a2android (pakpatch.py, obbclone.py, ziplayout.py). Never use Python zipfile to write the OBB (zip extra mismatch past 2GB -> Corrupt Toc). Configs in pak are Oodle — no in-place hex edits.
- OBB size check: `const-wide v2` in smali/com/AnotherSaxaphone/GayTwo/a.smali (was com/AnotherAxiom/A2 before 69971477) must equal the OBB byte size; bVerifyOBBOnStartUp=false in manifest (downloader activity hangs on Quest).
- midnight loadLibrary is at the top of GameActivity.onCreate.
- Untouched original OBB: Downloads/net6.0/apps/7181769438517329/26404467455820906/obbs/main.69971476.com.AnotherAxiom.A2.obb.

Related: [solara-cloudflare-tunnel](solara-cloudflare-tunnel.md), [a2-50451-christmas-port](a2-50451-christmas-port.md)

2026-11-01 build 69971477 (mt2p Vivox): pak a2pak_vivox, OBB Downloads/obb_vivox/main.69971477..., APK Downloads/apk_vivox/A2-GayTwo-69971477-signed.apk,
tree "A2-Android-Shipping-arm64 (3)". Package com/AnotherAxiom/A2 smali renamed to com/AnotherSaxaphone/GayTwo and the two
provider authorities renamed (duplicate authorities = can't coexist with the original app). App Lab key = keystore alias
`dre1mer` (NOT dre1merr; cert sha256 db0b8c3d...). pak11.py's default OODLE path is mangled -- pass OODLE_DLL=C:\Users\drych\a7_recovery\tools\oodle\oodle-data-shared.dll.
