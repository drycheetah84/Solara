# Midnight (Android) port → A2 1.0.50451

Port of the **Midnight** universal backend-redirect (`libmidnight.so`) from the previous A2
Android build to the Christmas build **A2 `1.0.50451`** (versionCode 69971476), arm64-v8a.

- **Source (from):** `C:\Users\drych\Downloads\Midnight-Android\Midnight-Android`
- **Target APK (to):** `...\APK.Easy.Tool.v1.60.Portable\1-Decompiled APKs\A2-Android-Shipping-arm64 (2)`
  (`apktool.yml` → `versionName: 1.0.50451`; this is the same version as the Windows 50451 port.)
- **Vendored port:** `tools/midnight-android/` (in this repo; upstream LICENSE preserved; credit razerfrfr).

## HTTP stack (answered: curl, not WinHttp/EOS-HTTP)

Both the Windows and Android A2 builds use **UE's statically-linked ThirdParty `libcurl/8.4.0`**
(nghttp2 1.47.0 + OpenSSL; `D:/Primary/Engine/Source/ThirdParty/...`), compiled into the main
module (`libUnreal.so` on Android, `A2-50451.exe` on Windows). No WinHttp/WinInet/EOS-HTTP.
- Windows redirect was done with a Fiddler proxy, so it never needed curl offsets.
- Android `libmidnight.so` hooks the internal `curl_easy_setopt` by offset.

Note: the APK also ships a **separate shared `libcurl.so` = `7.79.0-DEV`** (used by Play
Services / OVR). The game does **not** route HTTP through it (no `curl_easy_setopt` import in
`libUnreal.so`), so it is irrelevant to the hook. Using it as the signature reference is what
made the first scan fail (wrong curl version).

## What changed vs the old build

| target | old build | **50451** |
|---|---|---|
| UE `curl_easy_setopt`  | `libUnreal.so + 0x96aa490` | **`libUnreal.so + 0xa2b84bc`** |
| EOS `curl_easy_setopt` | `libEOSSDK.so + 0x13fa898` | **`libUnreal.so + 0xa324f04`** |

Key structural change: **there is no `libEOSSDK.so`** in 50451. The EOS SDK (with its own bundled
static libcurl) is now **merged into `libUnreal.so`**, so both `curl_easy_setopt` copies live in
`libUnreal.so`. Midnight hooks both (see `src/hooks.cpp` `targets[]`).

## How the offsets were found (reproducible, headless)

`curl_easy_setopt` is stripped (only `.dynsym`, which does not contain it). Decisive method =
**caller tally**: `CURLOPT_URL = 10002` is unique, so every site doing `mov w1, #10002` followed
shortly by `bl X` calls `curl_easy_setopt`. Tallying `X` across several `CURLOPT_*` constants:

- `0xa2b84bc` — called with 6 distinct options (URL, WRITEDATA, POSTFIELDS, HTTPHEADER, PROXY,
  READDATA), 12 sites → UE's `curl_easy_setopt` (thin variadic wrapper → `Curl_vsetopt`).
- `0xa324f04` — called with URL + POSTFIELDS → EOS's `curl_easy_setopt` (`Curl_vsetopt` inlined;
  the big option jump-table, `add w8, w1, #-10002` → CURLOPT_URL at table index 0).

Both were cross-checked by disassembly: variadic `__va_list` prologue (save x2–x7 + q0–q7,
`gr_offs=-48`, `vr_offs=-128`) and the `if(!curl) return 43` null path (`cbz x0 → mov w0,#0x2b`
for `0xa2b84bc`). The `libcurl.so` reference run located its own exported `setopt@0xa9abc`,
validating the scanner.

Offsets are **vaddr file offsets** (PIE, first PT_LOAD vaddr=0), which equal the runtime offset
from the lib's mmap base — exactly what `base + off` in `hooks.cpp` needs.

Scanner: `tools/midnight-android/port/scan_callers.py` (+ `scan5.py`, `readstr.py`).
Tooling: NDK `llvm-objdump` / `llvm-readelf` / `llvm-nm`
(`...Unity\Hub\Editor\6000.5.10f1\...\AndroidPlayer\NDK\toolchains\llvm\...`).

## Build & inject (unchanged from upstream)
`build.bat` (Windows) → `libs/arm64-v8a/libmidnight.so`; inject per upstream README
(load `midnight` in `GameActivity.smali onCreate`). Redirect backends in `includes/opts.h`
(`209.126.2.136:50/:78/:90`).

## Status
- [x] Build identity confirmed (50451, arm64-v8a) and HTTP stack identified (libcurl 8.4.0).
- [x] Both `curl_easy_setopt` offsets located, disassembly-verified, and applied to `hooks.cpp`.
- [ ] Rebuild `libmidnight.so` with NDK (requires ndk-build availability).
- [ ] Runtime verification (inject into the 50451 APK on-device; confirm `[Midnight] Redirected`
      log lines) — needs an on-device/emulator run; external blocker, cannot be driven headless.
