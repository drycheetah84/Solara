// SolaraRedirect — minimal in-process redirect for A2 (UE 5.5.3 build 47031).
//
// A2's dashboard/login API base URL is the default of the DashboardApiUrl config
// value: the .rdata ASCII string "https://api.oriondrift.net" at RVA 0x924A700.
// The game reads it (at login) to build /v2/users/log_in etc. The real server is
// dead (Internal Server Error) which crashes the client. We overwrite the default
// with the local Solara A2StationDb (http://127.0.0.1:78) so the game talks to
// Solara instead. No hooks, no signatures, no TLS — libcurl just does plain HTTP
// to localhost. Build-matched to 47031 via the fixed RVA.
//
// Build: cl /LD /O2 /EHsc SolaraRedirect.cpp /link /OUT:SolaraRedirect.dll
// Inject early (at the menu, before login). x64 only.
#include <windows.h>
#include <cstdio>
#include <cstring>
#include <cstdint>

// RVA of the ASCII "https://api.oriondrift.net" default (47031). 26 bytes + NUL.
static constexpr uintptr_t kDashboardApiUrlRVA = 0x924A700;
static const char kLocal[] = "http://127.0.0.1:78";   // Solara A2StationDb (serves /v2/users/log_in)

static void PatchString(uintptr_t base, uintptr_t rva, const char* repl, size_t replLenInclNul, size_t origLen)
{
    char* p = reinterpret_cast<char*>(base + rva);
    DWORD old = 0;
    if (!VirtualProtect(p, 64, PAGE_READWRITE, &old)) {
        printf("[SolaraRedirect] VirtualProtect FAILED @ %p (err %lu)\n", p, GetLastError());
        return;
    }
    char before[64]{}; memcpy(before, p, 63);
    SecureZeroMemory(p, origLen + 1);
    memcpy(p, repl, replLenInclNul);   // includes the NUL terminator
    VirtualProtect(p, 64, old, &old);
    printf("[SolaraRedirect] @%p  was '%s'  now '%s'\n", p, before, p);
}

static DWORD WINAPI Run(LPVOID)
{
    AllocConsole();
    FILE* f = nullptr; freopen_s(&f, "CONOUT$", "w", stdout);
    SetConsoleTitleA("SolaraRedirect");

    uintptr_t base = reinterpret_cast<uintptr_t>(GetModuleHandleW(nullptr)); // main exe base (ASLR-correct)
    printf("[SolaraRedirect] game base = 0x%p\n", reinterpret_cast<void*>(base));
    PatchString(base, kDashboardApiUrlRVA, kLocal, sizeof(kLocal), 26);
    printf("[SolaraRedirect] done — DashboardApiUrl now points at Solara :78\n");
    return 0;
}

BOOL APIENTRY DllMain(HMODULE hModule, DWORD reason, LPVOID)
{
    if (reason == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(hModule);
        CreateThread(nullptr, 0, Run, nullptr, 0, nullptr);
    }
    return TRUE;
}
