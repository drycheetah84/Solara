#!/usr/bin/env python3
"""Produce Server/dllmain_47031.cpp from Server/dllmain.cpp (the 22284 source)
by rewriting the build-specific offset constants to their resolved 47031 values.

Rules:
  - resolved (high/med): replace the value, append '// [47031 CONF] was 0xOLD (why)'
  - unresolved:          keep the 22284 value, append
                         '// TODO-47031 UNRESOLVED (kept 22284 value) -- <note>'
Nothing is invented: only values with a proof in offsets-47031-final.json change.
Engine globals + ProcessEvent are NOT here (they come from the SDK repoint).
"""
import json, re, os

SRV = r"C:\Users\drych\ProjectDrift\Solara\Server"
A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
final = json.load(open(os.path.join(A, "offsets-47031-final.json")))
resolved = final["resolved"]          # name -> {val, conf, why}
unresolved = final.get("unresolved", {})

src = open(os.path.join(SRV, "dllmain.cpp"), encoding="utf-8", errors="replace").read()

# match:  <ws>NAME = 0xHEX;   (works for constexpr uintptr_t NAME = 0x..;)
pat = re.compile(r'(\b([A-Za-z_]\w*_(?:RVA|Off))\s*=\s*)0x([0-9A-Fa-f]+)(\s*;)([^\n]*)')

stats = {"resolved": 0, "unresolved": 0, "untouched": 0}
changed = []


def repl(m):
    pre, name, oldhex, semi, tail = m.group(1), m.group(2), m.group(3), m.group(4), m.group(5)
    old = int(oldhex, 16)
    if name in resolved:
        r = resolved[name]
        newv = int(r["val"], 16)
        stats["resolved"] += 1
        if newv == old:
            note = "  // [47031 %s] SAME as 22284 (%s)" % (r["conf"], r["why"][:48])
        else:
            note = "  // [47031 %s] was 0x%X (%s)" % (r["conf"], old, r["why"][:48])
            changed.append((name, "0x%X" % old, r["val"], r["conf"]))
        return "%s0x%X%s%s" % (pre, newv, semi, note)
    else:
        stats["unresolved"] += 1
        return "%s0x%X%s  // TODO-47031 UNRESOLVED (kept 22284 value; resolve before enabling)" % (
            pre, old, semi)


new = pat.sub(repl, src)

# Boot-spine gate: for 47031 bring-up force pass-through PE hook until the
# gameplay offsets are resolved+verified at runtime. Explicit + reversible.
new2 = new.replace(
    "static bool g_bootSpineOnly = false;",
    "static bool g_bootSpineOnly = true;   // [47031 BRINGUP] PE hook pass-through until gameplay offsets verified at runtime (was false on 22284)")
spine = (new2 != new)
new = new2

# [47031 BRINGUP] Hook-install gate: skip every MinHook install whose target
# offset is not yet verified at runtime, so a stale 22284 address can never have
# a trampoline written over it. ProcessEvent (valid SDK offset, pass-through) and
# Terminate/Exit (kernel32) stay unconditional.
import re as _re
GATE = (
    '#pragma comment(lib, "iphlpapi.lib")\n\n'
    '// [47031 BRINGUP] Master gate for hooks whose target offset is not yet\n'
    '// verified against the live 47031 process. OFF => the install is SKIPPED\n'
    '// (no trampoline at a stale address) and Orig is nulled. Flip true / per-\n'
    '// offset as offsets are confirmed at runtime.\n'
    'static bool g_gameplayHooks = false;\n'
    'static inline MH_STATUS MH_CreateHookG(LPVOID pTarget, LPVOID pDetour, LPVOID* ppOriginal)\n'
    '{\n'
    '    if (!g_gameplayHooks) { if (ppOriginal) *ppOriginal = nullptr; return MH_OK; }\n'
    '    return MH_CreateHook(pTarget, pDetour, ppOriginal);\n'
    '}\n'
)
if '#pragma comment(lib, "iphlpapi.lib")' in new and 'MH_CreateHookG' not in new:
    new = new.replace('#pragma comment(lib, "iphlpapi.lib")\n', GATE, 1)
    # gate every install, then restore the always-valid three
    new = new.replace('MH_CreateHook(', 'MH_CreateHookG(')
    # WorldGetNetMode_Hook: resolved to 0x41F5A60 and required for the server map
    # open (forces DedicatedServer during world init). Install it (un-gated).
    for keep in ('ProcessEvent_Hook', 'TerminateProcess_Hook', 'ExitProcess_Hook',
                 'WorldGetNetMode_Hook'):
        new = _re.sub(r'MH_CreateHookG\((?=[^;\n]*' + keep + r')', 'MH_CreateHook(', new)
    # fix the wrapper body itself (it must call the real MH_CreateHook)
    new = new.replace('return MH_CreateHookG(pTarget, pDetour, ppOriginal);',
                      'return MH_CreateHook(pTarget, pDetour, ppOriginal);')
    # page-writable helper (guards raw stores to stale .data addresses)
    new = new.replace(
        '    return MH_CreateHook(pTarget, pDetour, ppOriginal);\n}\n',
        '    return MH_CreateHook(pTarget, pDetour, ppOriginal);\n}\n'
        'static inline bool HxPageWritable(const void* p)\n'
        '{\n'
        '    MEMORY_BASIC_INFORMATION mbi{};\n'
        '    if (!VirtualQuery(p, &mbi, sizeof(mbi)) || mbi.State != MEM_COMMIT) return false;\n'
        '    const DWORD w = PAGE_READWRITE | PAGE_WRITECOPY | PAGE_EXECUTE_READWRITE | PAGE_EXECUTE_WRITECOPY;\n'
        '    return (mbi.Protect & w) != 0;\n'
        '}\n', 1)

# Guard the 22284 gc-cvar backing-int raw stores (stale RVAs fault on 47031).
_GC_OLD = ('''        const int wasP = *reinterpret_cast<int*>(b + 0x94C76B8);
        const int wasC = *reinterpret_cast<int*>(b + 0x94C8444);
        *reinterpret_cast<int*>(b + 0x94C76B8) = g_parallelGC ? 1 : 0;   // gc.AllowParallelGC
        *reinterpret_cast<int*>(b + 0x94C8444) = 0;   // gc.CreateGCClusters = 0
        printf("[HalcyonA2] gc cvar backing vars set DIRECTLY: AllowParallelGC %d->0, CreateGCClusters %d->0\\n",
            wasP, wasC);''')
_GC_NEW = ('''        void* pa = reinterpret_cast<void*>(b + 0xA480348);   // [47031] gc.AllowParallelGC backing int (was 22284 0x94C76B8)
        void* pc = reinterpret_cast<void*>(b + 0xA4810D4);   // [47031] gc.CreateGCClusters backing int (was 22284 0x94C8444)
        if (HxPageWritable(pa) && HxPageWritable(pc)) {
            const int wasP = *reinterpret_cast<int*>(pa);
            const int wasC = *reinterpret_cast<int*>(pc);
            *reinterpret_cast<int*>(pa) = g_parallelGC ? 1 : 0;   // gc.AllowParallelGC
            *reinterpret_cast<int*>(pc) = 0;   // gc.CreateGCClusters = 0
            printf("[HalcyonA2] gc cvar backing vars set DIRECTLY: AllowParallelGC %d->0, CreateGCClusters %d->0\\n",
                wasP, wasC);
        } else {
            printf("[HalcyonA2] gc cvar backing-int RVAs are stale/non-writable on 47031 -> skipped (console cmds above still apply)\\n");
        }''')
if _GC_OLD in new:
    new = new.replace(_GC_OLD, _GC_NEW, 1)

# Disable parallel GC + clustering at the very start of Main (resolved 47031
# cvar backing ints) — the cluster ref-pass crashes during early load and the
# console-command path does not apply in time.
_EARLY_ANCHOR = '    PatchEntitlementExit(GetBase(), "load");\n    PatchBallSpawnerNullEntity(GetBase());\n'
_EARLY_ADD = _EARLY_ANCHOR + (
    '\n    // [47031 BRINGUP] kill parallel GC + clustering ASAP (cvar backing ints, resolved 47031 RVAs).\n'
    '    {\n'
    '        const uintptr_t b0 = GetBase();\n'
    '        void* pa = reinterpret_cast<void*>(b0 + 0xA480348);   // gc.AllowParallelGC\n'
    '        void* pc = reinterpret_cast<void*>(b0 + 0xA4810D4);   // gc.CreateGCClusters\n'
    '        if (HxPageWritable(pa)) *reinterpret_cast<int*>(pa) = 0;\n'
    '        if (HxPageWritable(pc)) *reinterpret_cast<int*>(pc) = 0;\n'
    '    }\n'
    '\n    // [47031 SOLARA REDIRECT] DashboardApiUrl default ascii @ RVA 0x924A700\n'
    '    // "https://api.oriondrift.net" -> local Solara A2StationDb (http://127.0.0.1:78).\n'
    '    {\n'
    '        char* url = reinterpret_cast<char*>(GetBase() + 0x924A700);\n'
    '        static const char repl[] = "http://127.0.0.1:78";\n'
    '        DWORD op = 0;\n'
    '        if (VirtualProtect(url, 32, PAGE_READWRITE, &op)) {\n'
    '            SecureZeroMemory(url, 27);\n'
    '            memcpy(url, repl, sizeof(repl));\n'
    '            VirtualProtect(url, 32, op, &op);\n'
    '            printf("[HalcyonA2][REDIRECT] DashboardApiUrl default -> %s\\n", url);\n'
    '            HxLog("[HalcyonA2][REDIRECT] DashboardApiUrl default -> %s\\n", url);\n'
    '        }\n'
    '    }\n')
if _EARLY_ANCHOR in new and '0xA480348) ' not in new.split(_EARLY_ANCHOR)[0][-400:]:
    new = new.replace(_EARLY_ANCHOR, _EARLY_ADD, 1)

# [BRINGUP] gate the auto-open (it currently crashes 47031 map-load serialization).
_OPEN_OLD = ('''    SDK::UKismetSystemLibrary::ExecuteConsoleCommand(
        world,
        SDK::FString(L"open /Game/A2/Maps/Station_Prime/Station_Prime_P.umap"),
        nullptr);
    printf("[HalcyonA2] issued: open Station_Prime_P\\n");''')
_OPEN_NEW = ('''    bool doOpen = false;
    {
        wchar_t sp[MAX_PATH]{}; GetTempPathW(MAX_PATH, sp); wcscat_s(sp, L"halcyon_open");
        doOpen = (GetFileAttributesW(sp) != INVALID_FILE_ATTRIBUTES);
    }
    if (!doOpen) {
        printf("[HalcyonA2] BRINGUP: SKIPPING auto-open of Station_Prime_P (create %%TEMP%%\\\\halcyon_open to enable)\\n");
    } else {
        SDK::UKismetSystemLibrary::ExecuteConsoleCommand(
            world,
            SDK::FString(L"open /Game/A2/Maps/Station_Prime/Station_Prime_P.umap"),
            nullptr);
        printf("[HalcyonA2] issued: open Station_Prime_P\\n");
    }''')
if _OPEN_OLD in new:
    new = new.replace(_OPEN_OLD, _OPEN_NEW, 1)

# [BRINGUP] gate the station self-init / deployment-fetch retry loop (reads 3 stale
# 22284 inline addrs -> derefs garbage, crashes every retry).
_DF_OLD = ('''    std::thread([]() {
        for (int i = 0; i < 600 && !g_deployFetchDone; ++i) { SafeTriggerDeploymentFetch(); Sleep(200); }
        }).detach();
    printf("[HalcyonA2] [PORT 22284] station self-init thread started (sub_54B13D0)\\n");''')
_DF_NEW = ('''    if (wcsstr(GetCommandLineW(), L"-HalcyonDeployFetch")) {
        std::thread([]() {
            for (int i = 0; i < 600 && !g_deployFetchDone; ++i) { SafeTriggerDeploymentFetch(); Sleep(200); }
            }).detach();
        printf("[HalcyonA2] [PORT 22284] station self-init thread started (sub_54B13D0)\\n");
    } else {
        printf("[HalcyonA2] BRINGUP: station self-init (deployment fetch) GATED OFF (stale offsets; -HalcyonDeployFetch to force)\\n");
    }''')
if _DF_OLD in new:
    new = new.replace(_DF_OLD, _DF_NEW, 1)

# VEH re-entrancy guard (a fault inside the crash reporter must not infinite-recurse).
_VEH_OLD = ('''        code != 0xC0000374u /* [EXITTRACE] heap corruption */ && code != 0xC00000FDu /* stack overflow (alt) */)
        return EXCEPTION_CONTINUE_SEARCH;
    const uintptr_t base = GetBase();''')
_VEH_NEW = ('''        code != 0xC0000374u /* [EXITTRACE] heap corruption */ && code != 0xC00000FDu /* stack overflow (alt) */)
        return EXCEPTION_CONTINUE_SEARCH;
    static thread_local int s_inVeh = 0;
    if (s_inVeh) return EXCEPTION_CONTINUE_SEARCH;   // the reporter itself faulted -> don't recurse
    s_inVeh = 1;   // reset at every return below (no RAII: this fn uses __try, C2712)
    const uintptr_t base = GetBase();''')
if _VEH_OLD in new:
    new = new.replace(_VEH_OLD, _VEH_NEW, 1)
    # reset the guard before each return after it is set
    new = new.replace('if (inSelf && g_probeGuard > 0) return EXCEPTION_CONTINUE_SEARCH;',
                      'if (inSelf && g_probeGuard > 0) { s_inVeh = 0; return EXCEPTION_CONTINUE_SEARCH; }', 1)
    new = new.replace('if (InterlockedIncrement(&s_n) > 12) return EXCEPTION_CONTINUE_SEARCH;',
                      'if (InterlockedIncrement(&s_n) > 12) { s_inVeh = 0; return EXCEPTION_CONTINUE_SEARCH; }', 1)
    new = new.replace(
        'HxLog("[HalcyonA2][CRASH] ***** end (GAME rvas -> IDA; SELF rvas -> HalcyonA2.map) *****\\n\\n");\n    return EXCEPTION_CONTINUE_SEARCH;',
        'HxLog("[HalcyonA2][CRASH] ***** end (GAME rvas -> IDA; SELF rvas -> HalcyonA2.map) *****\\n\\n");\n    s_inVeh = 0;\n    return EXCEPTION_CONTINUE_SEARCH;', 1)

out = os.path.join(SRV, "dllmain_47031.cpp")
open(out, "w", encoding="utf-8", newline="\n").write(new)
print("wrote", out)
print("stats:", stats, "boot-spine gate set:", spine)
print("\nchanged offsets (%d):" % len(changed))
for n, o, v, c in sorted(changed):
    print("  %-28s %s -> %s  [%s]" % (n, o, v, c))
