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
    for keep in ('ProcessEvent_Hook', 'TerminateProcess_Hook', 'ExitProcess_Hook'):
        new = _re.sub(r'MH_CreateHookG\((?=[^;\n]*' + keep + r')', 'MH_CreateHook(', new)
    # fix the wrapper body itself (it must call the real MH_CreateHook)
    new = new.replace('return MH_CreateHookG(pTarget, pDetour, ppOriginal);',
                      'return MH_CreateHook(pTarget, pDetour, ppOriginal);')

out = os.path.join(SRV, "dllmain_47031.cpp")
open(out, "w", encoding="utf-8", newline="\n").write(new)
print("wrote", out)
print("stats:", stats, "boot-spine gate set:", spine)
print("\nchanged offsets (%d):" % len(changed))
for n, o, v, c in sorted(changed):
    print("  %-28s %s -> %s  [%s]" % (n, o, v, c))
