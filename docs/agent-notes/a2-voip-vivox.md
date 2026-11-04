# a2-voip-vivox

> A2 voice chat (Vivox) -- WORKING on 50451 since 2026-11-01 via the old mt2p server + 22284 key (client ini + DLL); worked on 20996/22284
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

User confirmed (2026-11-01): VOIP WORKED on 20996 and 22284 — so the Vivox account/issuer
(20066-a2-61679-udash, mt2p.vivox.com) is still live. Don't question whether Vivox works; port the fixes.

DLL pieces (dllmain_*.cpp, [VOIPFIX] / "Vivox login-token f claim fix"): subsystem Issuer/SigningKey
FString writes (22284 offsets 0xF0/0x138), ReqLogin hook (RequestVivoxLoginToken_Implementation, fills
the empty "f" claim), JwtBuild hook, JoinBuild hook, SendJoin hook (ReceiveChannelJoinTokens). The client
build's UAxVOIPSubsystem::BuildChannelUrl / GetUserUrl are empty stubs (server code compiled out) —
the DLL builds the channel/user URIs itself.

50451 status (2026-11-01): ReqLogin (22284 0x543C7B0) and SendJoin (0x52EE2E0) still HX_UNRES = off;
server logs "Tried to get Station Channel URL / User URL, but we're not running a server"; PC client never
attempts Account.Login.

50451 FIXES (2026-11-01, deployed): 50451 is a DIFFERENT Vivox app -- ini [/Script/A2.A2VOIPSubsystem] Domain
mtu1xp.vivox.com (not mt2p), different SigningKey (user supplied the ini; key now in dllmain_50451.cpp
kVoipSigningKey), same Issuer. Subsystem Issuer @0x1F0 / SigningKey @0x250 (builder sub_1453DD330 reads
a1+496/a1+592). ReqLogin = 0x53DF3F0, SendJoin = 0x53DEC00, JoinBuild 0x53DCB10 and JwtBuild 0x53E51D0
verified. TRAP: dllmain_50451's MH_CreateHookG returns MH_OK WITHOUT creating the hook when
g_gameplayHooks is false (it is, on 50451) -- "MH_OK" in the log proves nothing. The 4 voice hooks now use plain
MH_CreateHook (enabled by MH_ALL_HOOKS at end of Main). Quest log (adb) before that fix: Account.AnonymousLogin
20123 with token claim "f": "" (signature accepted, so key/issuer are right).

Related: [a2-50451-christmas-port](a2-50451-christmas-port.md), [a2-rollback-netmode-fix](a2-rollback-netmode-fix.md)

WORKING on 50451 (2026-11-01, user confirmed in-game): client pak ini switched to Domain mt2p.vivox.com /
Server https://mt2p.www.vivox.com/api2 / the 22284 SigningKey (APK+OBB 69971477), server DLL kVoipDomain(A) and
all channel URIs @mt2p.vivox.com. The new Unity Vivox app (mtu1xp / unity.vivox.com) rejects both keys -- don't go back.
