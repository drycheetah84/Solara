# a2-client-mcp-capture

> How to drive the A2 Windows client remotely (free-cam + screenshots) via the dsound.dll MCP loopback bridge
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

**LATEST 50451 (2026-10-31): client JOINS the 50451 server, fully driven by Claude.** Source `tools/specedit50451/`
(build: `cmd /c build50451.cmd` from PowerShell). Client install = hardlinked copy
`Downloads\33526083366990015\client50451\` (cp -al, original untouched) + dsound.dll. Launch from bash as a
background child: `./A2-Win64-Shipping.exe -windowed -ResX=1280 -ResY=720 -log -SpecEditConnect=127.0.0.1:7777`
(works; the old "dies before UE init" note no longer holds). Bridge wire format: `id\tcmd\t<hex(arg0)>\t<hex(arg1)>...`
(args are HEX-encoded). Helper script at scratchpad mcp.py. Free-cam needs `op enter` first (editor camera), then
`camera x|y|z|pitch|yaw`. `op shot clean:<FULL path.bmp>` (a bare `clean:name` writes into the exe dir). New `op join <ip:port>`.

The A2 **Windows client is flatscreen by design — NOT VR**. Do NOT pass `-nohmd` (it's meant to
run without an HMD; -nohmd is wrong). It renders to a normal desktop window.

**Remote-drive capability is already BUILT** in the SpecEditor client DLL `dsound.dll` (source
`tools/specedit47031/`, see [a2-47031-speceditor-port](a2-47031-speceditor-port.md)). No new DLL/MCP needed. When the client
is in a world (`worldReady`) it opens a loopback MCP bridge:
- TCP `127.0.0.1:47650-47654`; chosen port written to `%TEMP%\halcyon_mcp_port.txt`.
- Line protocol: send `<id>\t<cmd>\t<args...>\n`, get a JSON reply line. (`se_mcp_net.cpp`,
  dispatch `se_mcp_bridge.inc`; `op` runs a ScriptOp and returns the log it produced.)
- **Move** (free-fly CameraActor, set as the view target): `op camto <x> <y> <z>`,
  `op camlook <pitch> <yaw>`, `op camfly <vx> <vy> <vz> <sec>`. Pose read via snapshot/CamPose.
- **Screenshot**: `op shot <name|full.bmp>` (editor UI included) or `op shot clean:<name>` (scene
  only, as players see it). Written as BMP by the DX12 present hook (`se_render.cpp` WriteBmp/ShotRecord).
  Convert BMP->PNG afterward (System.Drawing) before sending to the user.
- Server-side spectator (free observer instead of forced real-player): launch the server game
  instance with **`-NoSpecBlock`** (default `g_specBlock=true` forces real player; flag disables it).

**Builds:** 47031 client+DLLs deployed at `C:\Users\drych\Downloads\32586841537580874\32586841537580874\`
(`A2\Binaries\Win64\A2-Win64-Shipping.exe` + dsound.dll + version.dll). The 50451 DLL is NOT
inject-safe yet (stale RVAs) — use 47031 for live runtime. Launcher `A2.exe` at the build root.

**LAUNCH GOTCHA (2026-10-30):** launching `A2-Win64-Shipping.exe` directly from Claude's
PowerShell/Bash tool session **dies before UE init** (no log, no window) — the game needs its
normal launcher/EOS bootstrap / an interactive desktop. So the USER launches the client and gets
into a station; THEN Claude attaches to the bridge (read the port file) and drives cam+shot fully
headless. Reaching a station (login/matchmake) needs the user; flying + capture is Claude's.
Logs: `%LOCALAPPDATA%\A2\Saved\Logs\A2.log`. See [solara-cloudflare-tunnel](solara-cloudflare-tunnel.md), [a2-revival-context](a2-revival-context.md).
