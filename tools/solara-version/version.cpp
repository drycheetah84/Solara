// version.dll / XINPUT1_4.dll proxy for the A2 PC client (build 1.0.50451): redirects the game's own HTTP traffic to Solara.
// Build both (build.cmd); use XINPUT1_4.dll -- on this client version.dll is pulled in from System32 only, so a
// game-folder version.dll is never loaded (verified 2026-11-01). The exe delay-imports XINPUT1_4 itself.
//
// Drop beside A2-Win64-Shipping.exe (A2\Binaries\Win64). version.dll is not a KnownDLL, so Windows loads this copy
// from the game folder; every real version.dll export is forwarded to %SystemRoot%\System32\version.dll.
//
// What it changes: the client sends ALL of its HTTP through its statically linked libcurl (the exe imports no
// WinHTTP), so the one hook is libcurl's curl_easy_setopt at RVA 0x79AFEB0. For CURLOPT_URL only, it rewrites
//     *oriondrift.net      -> https://station.halcy0n.net        (station / dashboard API, Solara :78)
//     *aa-mothership.com   -> https://mothership-prod.halcy0n.net (Mothership + index.json, Solara :90)
// keeping the path and query -- the same per-domain mapping Midnight uses on Quest. TLS stays verified: the
// tunnel serves a real *.halcy0n.net certificate.
//
// Safety: it hooks only after checking the bytes at the RVA are curl_easy_setopt's prologue (another build ->
// no hook, logged). EOS and Vivox use their own HTTP stacks inside their own DLLs and are not touched.
// Log: %TEMP%\solara_version.log
#include <windows.h>
#include <cstdio>
#include <cstdarg>
#include <cstring>
#include <string>
#include "MinHook.h"

// ---------------------------------------------------------------------------------------------------------------
// Logging
static CRITICAL_SECTION g_logCs;
static void Log(const char* fmt, ...)
{
    char path[MAX_PATH];
    if (!GetTempPathA(MAX_PATH, path)) return;
    strcat_s(path, "solara_version.log");
    EnterCriticalSection(&g_logCs);
    FILE* f = nullptr;
    if (!fopen_s(&f, path, "a") && f)
    {
        SYSTEMTIME t; GetLocalTime(&t);
        fprintf(f, "[%02d:%02d:%02d] ", t.wHour, t.wMinute, t.wSecond);
        va_list ap; va_start(ap, fmt); vfprintf(f, fmt, ap); va_end(ap);
        fputc('\n', f);
        fclose(f);
    }
    LeaveCriticalSection(&g_logCs);
}

// ---------------------------------------------------------------------------------------------------------------
// URL rewrite
static constexpr uintptr_t kCurlEasySetoptRva = 0x79AFEB0;      // A2 1.0.50451 (verified: null-handle -> 43, Curl_vsetopt)
static constexpr int kCurlOptUrl = 10002;                        // CURLOPT_URL (CURLOPTTYPE_STRINGPOINT + 2)
static const unsigned char kSetoptPrologue[] = {                 // mov [rsp+10h],edx / mov [rsp+18h],r8 / mov [rsp+20h],r9
    0x89, 0x54, 0x24, 0x10, 0x4C, 0x89, 0x44, 0x24, 0x18, 0x4C, 0x89, 0x4C, 0x24, 0x20 };

struct Route { const char* domain; const char* target; };
static const Route kRoutes[] = {
    { "oriondrift.net",    "https://station.halcy0n.net" },
    { "aa-mothership.com", "https://mothership-prod.halcy0n.net" },
};

static bool HostMatches(const std::string& host, const char* domain)
{
    const size_t d = strlen(domain);
    if (host.size() == d) return _stricmp(host.c_str(), domain) == 0;
    return host.size() > d && host[host.size() - d - 1] == '.' && _stricmp(host.c_str() + host.size() - d, domain) == 0;
}

// "scheme://host[:port]rest" -> target + rest, when host is (a subdomain of) a routed domain. Empty = unchanged.
static std::string Rewrite(const char* url)
{
    if (!url) return {};
    const char* sep = strstr(url, "://");
    if (!sep || sep - url > 8) return {};
    const char* h = sep + 3;
    const char* e = h;
    while (*e && *e != '/' && *e != '?' && *e != '#') ++e;
    std::string hostPort(h, e - h);
    if (hostPort.find('@') != std::string::npos) return {};      // userinfo: not ours
    std::string host = hostPort.substr(0, hostPort.find(':'));
    for (const Route& r : kRoutes)
        if (HostMatches(host, r.domain))
            return std::string(r.target) + e;
    return {};
}

using SetoptFn = int(__cdecl*)(void*, int, void*);
static SetoptFn g_origSetopt = nullptr;
static volatile LONG g_rewrites = 0;

// curl_easy_setopt(CURL*, CURLoption, ...): on x64 the one variadic argument arrives in r8 like a normal third
// parameter, and every option this game passes is a single pointer/long/off_t, so forwarding (h, opt, p) is exact.
// libcurl copies a CURLOPT_URL string (Curl_setstropt), so the rewritten buffer only has to outlive this call.
static int __cdecl SetoptHook(void* handle, int option, void* param)
{
    if (option == kCurlOptUrl && param)   // param is the game's own NUL-terminated URL string
    {
        const char* url = static_cast<const char*>(param);
        std::string out = Rewrite(url);
        if (!out.empty())
        {
            const LONG n = InterlockedIncrement(&g_rewrites);
            if (n <= 200) Log("redirect %s -> %s", url, out.c_str());
            return g_origSetopt(handle, option, const_cast<char*>(out.c_str()));
        }
    }
    return g_origSetopt(handle, option, param);
}

static void InstallHook()
{
    auto* exe = reinterpret_cast<unsigned char*>(GetModuleHandleW(nullptr));
    char exePath[MAX_PATH] = {};
    GetModuleFileNameA(nullptr, exePath, MAX_PATH);
    if (!strstr(exePath, "A2-Win64-Shipping")) { Log("not the A2 client (%s): no hook", exePath); return; }

    unsigned char* target = exe + kCurlEasySetoptRva;
    __try
    {
        if (memcmp(target, kSetoptPrologue, sizeof(kSetoptPrologue)) != 0)
        {
            Log("curl_easy_setopt prologue mismatch at RVA 0x%llX: not build 50451, no hook (redirect OFF)",
                static_cast<unsigned long long>(kCurlEasySetoptRva));
            return;
        }
    }
    __except (EXCEPTION_EXECUTE_HANDLER) { Log("cannot read RVA 0x%llX: no hook", static_cast<unsigned long long>(kCurlEasySetoptRva)); return; }

    MH_STATUS s = MH_Initialize();
    if (s != MH_OK && s != MH_ERROR_ALREADY_INITIALIZED) { Log("MH_Initialize failed (%d)", s); return; }
    s = MH_CreateHook(target, reinterpret_cast<void*>(&SetoptHook), reinterpret_cast<void**>(&g_origSetopt));
    if (s != MH_OK) { Log("MH_CreateHook failed (%d)", s); return; }
    s = MH_EnableHook(target);
    if (s != MH_OK) { Log("MH_EnableHook failed (%d)", s); return; }
    Log("redirect ON: curl_easy_setopt hooked at %p (oriondrift.net -> station.halcy0n.net, aa-mothership.com -> mothership-prod.halcy0n.net)", target);
}

// ---------------------------------------------------------------------------------------------------------------
#ifdef PROXY_XINPUT
#include "xinput_fwd.inc"
#else
// version.dll forwarding (resolved lazily from the system copy; never LoadLibrary inside DllMain)
static HMODULE RealVersion()
{
    static HMODULE s_mod = nullptr;
    if (!s_mod)
    {
        wchar_t p[MAX_PATH];
        const UINT n = GetSystemDirectoryW(p, MAX_PATH);
        if (n && n < MAX_PATH - 20) { wcscat_s(p, L"\\version.dll"); s_mod = LoadLibraryW(p); }
    }
    return s_mod;
}
static FARPROC Real(const char* name)
{
    HMODULE m = RealVersion();
    return m ? GetProcAddress(m, name) : nullptr;
}
#define FWD(ret, fail, name, params, args)                                              \
    extern "C" ret WINAPI P_##name params                                               \
    {                                                                                   \
        static auto fn = reinterpret_cast<ret(WINAPI*) params>(Real(#name));            \
        if (!fn) { SetLastError(ERROR_PROC_NOT_FOUND); return fail; }                   \
        return fn args;                                                                 \
    }

FWD(BOOL, FALSE, GetFileVersionInfoA, (LPCSTR f, DWORD h, DWORD l, LPVOID d), (f, h, l, d))
FWD(BOOL, FALSE, GetFileVersionInfoW, (LPCWSTR f, DWORD h, DWORD l, LPVOID d), (f, h, l, d))
FWD(BOOL, FALSE, GetFileVersionInfoExA, (DWORD fl, LPCSTR f, DWORD h, DWORD l, LPVOID d), (fl, f, h, l, d))
FWD(BOOL, FALSE, GetFileVersionInfoExW, (DWORD fl, LPCWSTR f, DWORD h, DWORD l, LPVOID d), (fl, f, h, l, d))
FWD(DWORD, 0, GetFileVersionInfoSizeA, (LPCSTR f, LPDWORD h), (f, h))
FWD(DWORD, 0, GetFileVersionInfoSizeW, (LPCWSTR f, LPDWORD h), (f, h))
FWD(DWORD, 0, GetFileVersionInfoSizeExA, (DWORD fl, LPCSTR f, LPDWORD h), (fl, f, h))
FWD(DWORD, 0, GetFileVersionInfoSizeExW, (DWORD fl, LPCWSTR f, LPDWORD h), (fl, f, h))
FWD(BOOL, FALSE, GetFileVersionInfoByHandle, (DWORD fl, HANDLE h, LPVOID* d, LPDWORD l), (fl, h, d, l))
FWD(DWORD, 0, VerFindFileA, (DWORD a, LPCSTR b, LPCSTR c, LPCSTR d, LPSTR e, PUINT f, LPSTR g, PUINT h), (a, b, c, d, e, f, g, h))
FWD(DWORD, 0, VerFindFileW, (DWORD a, LPCWSTR b, LPCWSTR c, LPCWSTR d, LPWSTR e, PUINT f, LPWSTR g, PUINT h), (a, b, c, d, e, f, g, h))
FWD(DWORD, 0, VerInstallFileA, (DWORD a, LPCSTR b, LPCSTR c, LPCSTR d, LPCSTR e, LPCSTR f, LPSTR g, PUINT h), (a, b, c, d, e, f, g, h))
FWD(DWORD, 0, VerInstallFileW, (DWORD a, LPCWSTR b, LPCWSTR c, LPCWSTR d, LPCWSTR e, LPCWSTR f, LPWSTR g, PUINT h), (a, b, c, d, e, f, g, h))
FWD(DWORD, 0, VerLanguageNameA, (DWORD l, LPSTR s, DWORD n), (l, s, n))
FWD(DWORD, 0, VerLanguageNameW, (DWORD l, LPWSTR s, DWORD n), (l, s, n))
FWD(BOOL, FALSE, VerQueryValueA, (LPCVOID b, LPCSTR s, LPVOID* p, PUINT l), (b, s, p, l))
FWD(BOOL, FALSE, VerQueryValueW, (LPCVOID b, LPCWSTR s, LPVOID* p, PUINT l), (b, s, p, l))
#endif

BOOL APIENTRY DllMain(HMODULE mod, DWORD reason, LPVOID)
{
    if (reason == DLL_PROCESS_ATTACH)
    {
        DisableThreadLibraryCalls(mod);
        InitializeCriticalSection(&g_logCs);
        // MinHook only patches code and allocates a trampoline (no loader calls), so hooking here is safe -- and
        // early enough that the client's first request (index.json, at boot) is already redirected.
        InstallHook();
    }
    return TRUE;
}
