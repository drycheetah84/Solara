# A2 22284 -> 47031 port -- consolidated result (honest)

## Verified engine globals (47031 reflection dump, ground truth)

| global | rva |
|---|---|
| GObjects | 0xA7C3A00 |
| GNames | 0xA6DFBC0 |
| GWorld | 0xA9E0968 |
| AppendString | 0x12AD4E0 |
| ProcessEvent(func) | 0x14DBF40 |

Struct offsets + these globals port by repointing the SDK include to `C:\Dumper-7.5.3-47031+++A2+hotfix-A2\CppSDK\SDK`.

## Verified function impls (cross-validated: reflection thunk -> called impl, matching size + callees/strings)

| site | 22284 | 47031 | evidence |
|---|---|---|---|
| UpdGameState_RVA | 0x53CB6D0 | 0x55a06c0 | callee_ov=1 str_ov=1 size 174/179 |
| StartTimerCd_RVA | 0x53C4060 | 0x559abd0 | callee_ov=1 str_ov=5 size 700/682 |
| IsRunningSimulate_RVA | 0x531EF96 | 0x54a1930 | callee_ov=1 str_ov=0 size 132/132 |

## UFunction sites: authoritative thunk RVA known, impl needs 1 IDA decompile

FunctionsInfo gives the exec/UFunction RVA exactly (reflection). dllmain hooks the `_Implementation`, which is a specific call inside that thunk -- read the thunk's decompilation to pick it (featureless/common-helper collisions make this unsafe to automate).

| site | 22284 impl | 47031 thunk (verified) | auto impl guess (UNVERIFIED) | conf |
|---|---|---|---|---|
| ReqLogin_RVA | 0x543C7B0 | 0x53dabb0 | 0x1177030 | med |
| SendPhysImpl_RVA | 0x5503050 | 0x54f8b00 | 0x14d3ba0 | low |
| NotifyLeftArena_RVA | 0x5502B00 | 0x54f82b0 | 0x1177030 | med |
| GolfBallInCup_RVA | 0x53DC320 | 0x54a9200 | 0x55b3900 | med |
| GolfOverlap_RVA | 0x53F03B0 | 0x54a9890 | 0x14de5f0 | med |
| GiveCheck_RVA | 0x4723AA0 | 0x49e79c0 | 0x49e79c0 | low |
| A2GetNetMode_RVA | 0x54AF360 | 0x22ac640 | 0x14db9c0 | med |
| GetLocalInst_RVA | 0x54EAC60 | 0x54f2b70 | 0x5810b30 | med |
| GetPlayerPawn_RVA | 0x3D24C40 | 0x3ae0310 | 0x3ec7c90 | med |
| inline_14200_054e4470 | 0x54e4470 | 0x542d3f0 | 0x542d3f0 | low |
| inline_14202_054e17e0 | 0x54e17e0 | 0x542ce00 | 0x5502140 | med |
| inline_15200_05502070 | 0x5502070 | 0x54f7430 | 0x14922f0 | low |
| inline_15230_046902f0 | 0x46902f0 | 0x49b3000 | 0x14d3ba0 | med |

## Internal sub_ functions, data globals, patch sites: need IDA / runtime

- 59 internal functions (no reflection name): resolve by string xref / decompile-compare / runtime.
- 14 data globals: resolve by xref from a resolved function.

## Honest status

- Verified globals: 5. Verified impls: 3. UFunction thunks located (impl pending 1 decompile each): 13. Internal/data/patch needing IDA+runtime: ~73.
- A runnable DLL is NOT achievable from this automated pass: most code RVAs and all patch sites are unresolved or unverified, and the only correctness proof is runtime injection + the crash-stabilization loop. Byte-signature guesses were shown WRONG where they overlapped reflection (e.g. GiveCheck), so they must not be baked.
- Nothing was written into dllmain.cpp. The SDK repoint + verified globals + verified impls are safe to apply; everything else needs the interactive IDA+game session.
