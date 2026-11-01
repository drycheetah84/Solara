// XINPUT1_4.dll forwarding (PROXY_XINPUT build). Named exports keep their real prototypes; the undocumented
// ordinal-only exports (100-104, 108) take at most 4 integer/pointer args, so a 4-argument forwarder is exact on x64.
static HMODULE RealXInput()
{
    static HMODULE s_mod = nullptr;
    if (!s_mod)
    {
        wchar_t p[MAX_PATH];
        const UINT n = GetSystemDirectoryW(p, MAX_PATH);
        if (n && n < MAX_PATH - 20) { wcscat_s(p, L"\\XInput1_4.dll"); s_mod = LoadLibraryW(p); }
    }
    return s_mod;
}
static FARPROC RealX(LPCSTR nameOrOrdinal) { HMODULE m = RealXInput(); return m ? GetProcAddress(m, nameOrOrdinal) : nullptr; }
#define XFWD(name, params, args)                                                         \
    extern "C" DWORD WINAPI P_##name params                                              \
    {                                                                                    \
        static auto fn = reinterpret_cast<DWORD(WINAPI*) params>(RealX(#name));          \
        return fn ? fn args : ERROR_DEVICE_NOT_CONNECTED;                                \
    }
XFWD(XInputGetState, (DWORD i, void* s), (i, s))
XFWD(XInputSetState, (DWORD i, void* v), (i, v))
XFWD(XInputGetCapabilities, (DWORD i, DWORD f, void* c), (i, f, c))
XFWD(XInputGetAudioDeviceIds, (DWORD i, LPWSTR r, UINT* rc, LPWSTR c, UINT* cc), (i, r, rc, c, cc))
XFWD(XInputGetBatteryInformation, (DWORD i, BYTE t, void* b), (i, t, b))
XFWD(XInputGetKeystroke, (DWORD i, DWORD r, void* k), (i, r, k))
extern "C" void WINAPI P_XInputEnable(BOOL e)
{
    static auto fn = reinterpret_cast<void(WINAPI*)(BOOL)>(RealX("XInputEnable"));
    if (fn) fn(e);
}
#define XORD(n)                                                                          \
    extern "C" DWORD WINAPI P_Ord##n(void* a, void* b, void* c, void* d)                \
    {                                                                                    \
        static auto fn = reinterpret_cast<DWORD(WINAPI*)(void*, void*, void*, void*)>(   \
            RealX(reinterpret_cast<LPCSTR>(static_cast<ULONG_PTR>(n))));                 \
        return fn ? fn(a, b, c, d) : ERROR_DEVICE_NOT_CONNECTED;                         \
    }
XORD(100) XORD(101) XORD(102) XORD(103) XORD(104) XORD(108)
