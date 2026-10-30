#pragma once
#include <string>
#include <cstdint>

// [A2] per-domain redirect targets. api.oriondrift.net = station/dashboard backend,
// aa-mothership.com = mothership backend (these are separate ports on your box, like the
// Windows redirector's :78 / :90). Set to whatever the Android device can actually reach
// (public host:port). If one backend serves both, set them to the same value.
// Redirect to the Solara backend via its Cloudflare Tunnel hostnames (one per service,
// HTTPS:443 at the Cloudflare edge; the tunnel forwards to the local Solara ports). Cloudflare
// presents a valid *.halcy0n.net cert, so TLS verifies -> BYPASS_SSL can stay false.
// Contains() suffix-match on the game's original host decides which backend:
constexpr const char* EPIC_BACKEND       = "https://epicgames.halcy0n.net";       // *epicgames*     -> :50 (EOS)
constexpr const char* MOTHERSHIP_BACKEND = "https://mothership-prod.halcy0n.net";  // *aa-mothership* -> :90
constexpr const char* ORION_BACKEND      = "https://station.halcy0n.net";          // *oriondrift*    -> :78 (A2StationDb)
// (Raw VPS fallback, if the tunnel is down: http://209.126.2.136:50 / :90 / :78 respectively.)
constexpr const char* BACKEND_URL = ORION_BACKEND; // legacy single-target (unused by the per-domain path)

constexpr bool BYPASS_SSL = false; // only works with curl versions (aka if it doesnt use processrequest i cba bypassing ssl for this)
constexpr bool ENABLE_LOGS = true; // to debug stuff js keep it disabled for release

// if you need any offsets either find them using ida or choose a version from offsets.txt file
//
// ============================================================================
// [A2 1.0.50451 - Android arm64-v8a]  ACTIVE PATH = Hooks::CurlHook (USE_PR=false).
// The CurlHook path does NOT read the addresses below; it hooks curl_easy_setopt
// by hardcoded offset inside src/hooks.cpp. For this build those offsets are:
//     libUnreal.so + 0xa2b84bc   (UE  curl_easy_setopt)
//     libUnreal.so + 0xa324f04   (EOS curl_easy_setopt - EOS SDK is merged into libUnreal.so)
// There is no separate libEOSSDK.so in this build. See tools/a2port/PORT-MIDNIGHT-50451.md
// for how these were located. The FCurlHookOpts / EOSFCurlHookOpts addresses below are only
// used by the UE-native ProcessRequest path (USE_PR=true) and are left at 0 (path disabled).
// ============================================================================
namespace FCurlHookOpts {
    constexpr bool USE_PR = false; // only enable this if ur doing 18.40+
    constexpr uintptr_t PR_ADDR = 0x0; // set this to the correct address (processrequest)
    constexpr uintptr_t GETURL_ADDR = 0x0; // set this to the correct address (geturl its mostly 0x70 until like s25/26+)
    constexpr uintptr_t SETURL_ADDR = 0x0; // set this to the correct address (seturl)
}

namespace EOSFCurlHookOpts {
    constexpr bool USE_EOS = false; // only enable this if ur doing s22+
    constexpr uintptr_t PR_ADDR = 0x0; // set this to the correct address (processrequest)
    constexpr uintptr_t GETURL_ADDR = 0x0; // set this to the correct address (geturl its 0x178 in almost any build)
    constexpr uintptr_t SETURL_ADDR = 0x0; // set this to the correct address (seturl)
}