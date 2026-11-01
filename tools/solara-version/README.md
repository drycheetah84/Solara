# A2 PC client redirect proxy (build 1.0.50451)

Sends the PC client's own HTTP to Solara, like Midnight does on Quest:

| game host | goes to |
|---|---|
| `*oriondrift.net` (station / dashboard API) | `https://station.halcy0n.net` |
| `*aa-mothership.com` (Mothership, index.json) | `https://mothership-prod.halcy0n.net` |

Path and query are kept; TLS stays verified (the tunnel has a real `*.halcy0n.net` cert).

**Install:** copy `build\XINPUT1_4.dll` into `A2\Binaries\Win64\` next to `A2-Win64-Shipping.exe`. Works alongside the
SpecEditor `dsound.dll`. Log: `%TEMP%\solara_version.log`.

**Why XINPUT1_4 and not version.dll:** on this client `version.dll` is only ever loaded from System32 (a game-folder
copy is ignored -- tested 2026-11-01). The exe delay-imports `XINPUT1_4.dll` itself, so a game-folder copy loads; it
forwards every XInput export (named and ordinals 100-104/108) to the real one. `build\version.dll` is still built for
games that do load it.

**How:** the client sends all HTTP through its static libcurl (no WinHTTP import). The proxy hooks `curl_easy_setopt`
(RVA `0x79AFEB0`) and rewrites `CURLOPT_URL` only. It checks the function's prologue bytes first; on any other build
it logs "prologue mismatch" and does nothing.

Build: `build.cmd` (VS 18 + the MinHook sources from the HalcyonA2 project).
