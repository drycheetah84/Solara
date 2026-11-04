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
