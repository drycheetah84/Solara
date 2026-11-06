# solara-cloudflare-tunnel

> Solara backend exposed under *.halcy0n.net via a named Cloudflare Tunnel (halcyon); hostnames, ports, caveats
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

**MOVED 2026-11-01 to AWS:** Solara + the tunnel connector now run on the Ubuntu EC2 box
(ssh -i ~/Downloads/Solara.pem ubuntu@ec2-18-222-37-52.us-east-2.compute.amazonaws.com, private
172.31.9.99): systemd `solara` (/opt/solara, MOTHERSHIP_INSECURE=1) and `cloudflared`
(/etc/cloudflared/config.yml). Home PC tunnel stopped. User prefers tunnels over raw DNS/open ports
("its safer"). Admin dashboard (Next, repo web/dashboard, commit 93c53ae) = systemd
`halcyon-dashboard` on 127.0.0.1:3000 (/opt/dashboard, built ON the box: npm ci + next build,
standalone); dash.halcy0n.net -> :3000; Solara :8080 bound 127.0.0.1; shared admin key in
/etc/halcyon/dashboard.env (copy: config/generated-aws-dashboard_admin_key.txt, git-ignored).
WinRM control of the Windows box: `python3 ~/wr.py` on Ubuntu (stdin = PowerShell; sshd broken on 2019).
Windows game box = 172.31.37.84 (agent -> hub 172.31.9.99:9100; bundle served by
`halcyon-dist` on 172.31.9.99:8000, bootstrap.ps1). Caddy installed but disabled.

Solara's backend is exposed through **Cloudflare Tunnel**, one hostname per service under
the user's **`halcy0n.net`** zone (already authorized by `~/.cloudflared/cert.pem`, an Argo
token — same zone as the user's other tunnels gta5/redstone). cloudflared installed at
`C:\Program Files (x86)\cloudflared\cloudflared.exe` (via winget `Cloudflare.cloudflared`).

**Tunnel `halcyon`** id `95306d5c-bfed-4553-bc71-9b9966f4cf63`; config
`~/.cloudflared/halcyon.yml` (committed copy: `deploy/cloudflare/halcyon.yml`); creds
`~/.cloudflared/95306d5c-…json` (secret, not in git). Hostname → Solara local port:
- `mothership-prod.halcy0n.net` → :90 (Mothership login/auth; game `*.aa-mothership.com`)
- `station.halcy0n.net` → :78 (A2StationDb/quests/deployments; game `api.oriondrift.net`)
- `epicgames.halcy0n.net` → :50 (EOS gateway/server list; game EOS/`*.epicgames.com`)
- `epicgames-ws.halcy0n.net` → :80 (EOS websocket)
- `dash.halcy0n.net` → :8080 (Ares dashboard admin)

Services come from `bin/solara.exe serve` (host.go AddServer order). Orchestrator
`:9095`/`:9100` is intentionally NOT tunneled (private). Verified live 2026-10-30:
mothership-prod → 200; station/epicgames/epicgames-ws → 404 (origin reached, root not a route); dash → 500.
(Renamed 2026-10-30: mothership→mothership-prod, eos→epicgames, eosws→epicgames-ws.)

**Caveats:** (1) game UDP `7777-7787` CANNOT go through a CF tunnel (no arbitrary UDP) —
needs a public IP:port. (2) `api.halcy0n.net` already A-records to `157.173.194.216`, so :78
uses `station.*`; repoint api.* to the tunnel if that name is wanted. (3) start-as-service:
`cloudflared --config ~/.cloudflared/halcyon.yml service install`; solara via NSSM/scheduled task.

To point the game here, update the redirect (Midnight [a2-50451-christmas-port](a2-50451-christmas-port.md) opts.h, the
in-memory DashboardApiUrl config patch, or Fiddler) from `http://<ip>:<port>` to the matching
`https://<svc>.halcy0n.net`. See [a2-revival-context](a2-revival-context.md), `deploy/cloudflare/README.md`.

Board images (2026-11-01): Ubuntu solara needs BOARD_PUBLIC_BASE=https://station.halcy0n.net (drop-in
/etc/systemd/system/solara.service.d/board.conf) -- without it board URLs default to the dead old IP
http://209.126.2.136:78 and boards never change. Station config (board URLs) is read by the game server only at
dashboard init, so new boards need a game-server restart.

**TWO game boxes (2026-11-04):** win-gamebox-1 = 172.31.37.84 (`python3 ~/wr.py`, 1 server) and **win-gamebox-2** = public 98.92.201.29 (`python3 ~/wr2.py`, 4 servers, most players). A DLL deploy must hit BOTH. Box 2 cannot reach the hub private IP: fetch the bundle via the hub public IP `http://18.222.37.52:8000/HalcyonA2.dll`, and download BEFORE killing A2-Win64-Shipping (then Move-Item into C:\Halcyon\HalcyonA2.dll). After a kill the watchdog relaunches each server once heartbeats go quiet (~2-3 min); check `GET 127.0.0.1:9095/watchdog` (x-dashboard-key from /etc/halcyon/dashboard.env).

**MOVED 2026-11-05 to AZURE (AWS account banned 2026-11-04; EC2 data unrecoverable so far).** Resource group
`halcyon-rg` (eastus, Pay-As-You-Go, CLI: `C:\Program Files\Microsoft SDKs\Azure\CLI2\wbin\az.cmd`, quota 10 vCPU;
D-series quota needs MFA login, a request was throttled -- retry for D8as_v5 then resize game1).
- Hub `halcyon-hub` B2ms Ubuntu 24.04, zone 2, public 20.51.185.231, private 10.10.1.4.
  `ssh -i ~/.ssh/solara_azure ubuntu@20.51.185.231` (NSG allows only the owner IPs 71.80.193.144 + 104.243.245.183;
  update `hub-nsg`/`game-nsg` rules if the owner IP changes). Same layout as AWS: systemd `solara` (/opt/solara,
  env drop-ins dashboard.conf/board.conf, /etc/halcyon/dashboard.env), `halcyon-dashboard` (:3000), `cloudflared`
  (same tunnel id/creds, /etc/cloudflared/config.yml), `halcyon-dist` (python http.server 10.10.1.4:8000 =
  /opt/halcyon-dist). 4 GB swap. Nightly DB copy to /opt/solara/backups (14 days, NOT off-box yet).
- Game box `halcyon-game1` F8s_v2 Windows 2022, zone 3, public 52.186.172.63, private 10.10.1.10; admin creds in
  config/generated-azure-gamebox-admin.txt (git-ignored). Run commands with `az vm run-command invoke -g halcyon-rg
  -n halcyon-game1 --command-id RunPowerShellScript --scripts @file.ps1` (no WinRM needed). C:\Halcyon layout as
  before; agent task HalcyonAgent -HubHost 10.10.1.4 -Box win-gamebox-1 -Capacity 5. 5 servers ~1.3 GB RAM each.
- DLL deploy now: scp DLL to hub /opt/halcyon-dist, then on game1 iwr http://10.10.1.4:8000/HalcyonA2.dll.
- **DLL deploy method (2026-11-05):** a loaded DLL cannot be overwritten (silent failure) and the agent caches its
  DllPath, so: rename the live `C:\Halcyon\HalcyonA2.dll` (allowed while mapped), copy the new one to that path, then
  stop servers ONE AT A TIME and wait for `injected pid N` in `C:\Halcyon\logsgent.log` before the next (a server
  killed while another is initialising takes that one down too). Verify by process START TIME, not by hashing the
  module path (that reads the new file on disk, not the loaded image). Run box scripts with `powershell -File`.
- **Watchdog speed (2026-11-05, c7f039b):** the agent reports a server exit within ~1 s and the watchdog relaunches
  within one 10 s tick (measured: 2 s). Before, the agent forgot its servers on every hub reconnect (= every Solara
  restart), so restarts took the 180 s silent path (~4 min each). The agent can be swapped live: stopping the
  HalcyonAgent task leaves its game servers running and the new agent adopts them.
- **Every game server MUST run the SpecEditor flags `-SpecEdit -SpecEditSandbox`** (in `Args` of
  `deploy/azure/server.config.psd1`). The agent launches servers from the box's OWN copy,
  `C:\Halcyon\server.config.psd1` (bootstrap downloads it once), so EVERY deploy also scps
  `deploy/azure/server.config.psd1` to /opt/halcyon-dist and iwr's it to `C:\Halcyon\server.config.psd1`.
  Flags only apply to servers started after that: restart them. Then verify on game1 that every server has them:
  `Get-CimInstance Win32_Process -Filter "Name='A2-Win64-Shipping.exe'" | % CommandLine` must show `-SpecEdit` and
  `-SpecEditSandbox` on each line. Without them the editor client's placements/exports are silently ignored
  (the client gets no error; only `%TEMP%\HalcyonA2*.log` on the box says why: `[SPECEDIT] REFUSED ...`).
- DB was seeded from the repo production.solara.db (2026-10-31): Nov 1-4 AWS data lost; stations Ranked/Clubhouse/
  Freerunners recreated (new ids), their station config lost. Discord login RESTORED 2026-11-05 (new client secret; role map, role names and fleet owners rebuilt from history; owners remapped to the new station ids) -- was: needed a new client secret +
  DISCORD_ROLE_MAP/FLEET_OWNERS/ROLE_NAMES (client id + guild id restored in dashboard.env).
- **Box 2 (2026-11-05):** `halcyon-game2` E8bds_v5 (8 vCPU / 64 GB) Windows 2022, **centralus** zone 1, public
  172.212.202.163, private 10.20.1.10, VNet `halcyon-vnet-c` (10.20.0.0/16) peered to `halcyon-vnet`, NSG `game-nsg-c`.
  Agent `win-gamebox-2` capacity 8, kept EMPTY as standby (spin-ups with no box land there once box 1 is full).
  Hub allowlist SOLARA_AGENT_ALLOWED_IPS=10.10.1.10,10.20.1.10. Same admin creds file. Measured on box 1:
  idle server ~0.4 core, +~2% core per player, game thread caps a server near 1 core (~25-30 players).

**2026-11-06 inject timing trap:** the box agent injects HalcyonA2.dll ~60 s after a server process starts. The deploy
script's module check (~20 s after a replacement) can show an empty module for a server that is still fine -- re-check
after 90 s, never kill it. A killed-while-injecting replacement gets retired by the watchdog ("spin-up failed") and then
"holding: never heartbeated"; `/watchdog/resume` alone will not relaunch it and `/spinup` rejects
`-DashboardDeploymentId`. Recovery: `POST /api/stations/{station_id}/spinup {"box":..,"name":..}` (new deployment id,
same station) then `POST :9095/watchdog/retire {"deployment_id": <dead>}`. Don't spin up while another launch is in
flight on the same box.
