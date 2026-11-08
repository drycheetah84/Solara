# Deploying HalcyonA2.dll to the Windows game boxes

How to ship a new server DLL (and the server config) to the Azure game boxes. **No credentials are in this repo.**
Access (hub SSH key, game box login) is in [`deploy/ACCESS.md`](../deploy/ACCESS.md). The owner still has to allowlist your IP.

## Rules first

- **The owner picks the time.** A restart kicks everyone on that server (the client has no reconnect path). Check
  player counts first (dashboard Deployments page, or `GET :9095/watchdog` on the hub) and wait for a go-ahead.
- Restart **one server at a time** and wait for the agent to relaunch and inject it. A server killed while another
  one is starting can take that one down too.
- Every server must run `-SpecEdit -SpecEditSandbox`. Those come from `deploy/azure/server.config.psd1`, so every
  deploy also ships that file. The script checks the flags at the end.
- All servers run `-ServerName=Halcyon`. To leave a busy station alone, skip it by **deployment id**, never by name.

## Layout

Rebuilt 2026-11-08 on a new Azure subscription (same names and private IPs, new public IPs; the old 20.51.185.231 hub is gone).

| Machine | Address | Role |
|---|---|---|
| Hub `halcyon-hub` (Ubuntu) | public 20.55.26.98, private 10.10.1.4 | Solara backend, dashboard, `halcyon-dist` file server on `10.10.1.4:8000` (= `/opt/halcyon-dist`) |
| Game box 1 `halcyon-game1` (Windows) | public 20.55.48.184, private 10.10.1.10 | agent `win-gamebox-1` |
| Game box 2 `halcyon-game2` (Windows) | public 20.83.58.255, private 10.20.1.10 | agent `win-gamebox-2` |

The game boxes are only reachable from the hub: `ssh ubuntu@<hub>`, then `ssh halcyon@<box private ip>`.
On each box: `C:\Halcyon\HalcyonA2.dll` (the DLL the agent injects), `C:\Halcyon\server.config.psd1` (its own copy
of the launch config), `C:\Halcyon\logs\agent.log` (look for `injected pid N`).

## Access

Each person gets their own access, so it can be removed later without rotating everyone's:

1. Make an SSH key (`ssh-keygen -t ed25519`) and send the owner the **public** half (`~/.ssh/id_ed25519.pub`).
2. The owner adds it to `~ubuntu/.ssh/authorized_keys` on the hub and adds your IP to the hub's Azure NSG
   (`hub-nsg`, resource group `halcyon-rg`). SSH is closed to everything else.
3. From the hub the boxes accept `ssh halcyon@10.10.1.10` / `ssh halcyon@10.20.1.10` with the hub's own key.

The hub's `:9095` admin endpoints take the dashboard admin key in `x-dashboard-key`. It is in
`/etc/halcyon/dashboard.env` on the hub (root only). Read it into a variable on the hub; never paste or log it.

## Build

From `Server/` on a Windows dev machine with VS 2022+:

```
MSBuild.exe HalcyonA2.vcxproj -p:GameBuild=50451 -p:Configuration=Release -p:Platform=x64 "-p:SolutionDir=<repo>\" -v:m
```

Output: `build/50451/x64/Release/HalcyonA2.dll`. Note its MD5 (first 8 hex): `certutil -hashfile HalcyonA2.dll MD5`.

## Deploy

1. **Upload** the DLL, the config and the script to the hub's dist folder:
   ```
   scp build/50451/x64/Release/HalcyonA2.dll deploy/azure/server.config.psd1 deploy/azure/deploy_dll.ps1 ubuntu@20.55.26.98:/opt/halcyon-dist/
   ```
2. **Dry run** on each box (downloads and checks the DLL, lists which servers it would restart, changes nothing):
   ```
   ssh ubuntu@20.55.26.98
   ssh halcyon@10.20.1.10 "powershell -NoProfile -Command iwr http://10.10.1.4:8000/deploy_dll.ps1 -OutFile C:\Halcyon\deploy_dll.ps1 -UseBasicParsing"
   ssh halcyon@10.20.1.10 "powershell -NoProfile -ExecutionPolicy Bypass -File C:\Halcyon\deploy_dll.ps1 -Md5 <8 hex> -DryRun"
   ```
   To leave busy stations running add `-Skip <deployment id>,<deployment id>` (a prefix is enough). The deployment
   id of each station is on the dashboard (station → Deployments tab).
3. **Deploy** with the owner's go-ahead: the same command without `-DryRun`. It takes about a minute per server.
   It stops if a replacement doesn't come back within 4 minutes: don't start anything else on that box, check the
   watchdog first.
4. Repeat for the other box.

## Check it worked

- The script's last lines list every server with its start time and the two SpecEditor flags; every restarted
  server must show a new start time and `SpecEdit=True Sandbox=True`.
- `C:\Halcyon\logs\agent.log` has `injected pid N` for each new process. The agent injects ~60 s after a process
  starts, so a server with no DLL loaded at +20 s is normal. **Never kill a replacement for looking DLL-less.**
- Dashboard: the stations come back online with players.

## Rollback

The script renamed the previous DLL to `C:\Halcyon\HalcyonA2-<old md5>-<time>.dll`. Upload that file as
`HalcyonA2.dll` again and run the script with its MD5.

## When a server doesn't come back

If the watchdog retired it ("spin-up failed", then "holding: never heartbeated"), `/watchdog/resume` alone will not
relaunch it, and `/spinup` refuses `-DashboardDeploymentId`. On the hub:

1. `POST http://127.0.0.1:8080/api/stations/{station_id}/spinup` (dashboard API, same key header) with `{"box":"win-gamebox-2","name":"<name>"}`
   (gives the station a new deployment id),
2. `POST http://127.0.0.1:9095/watchdog/retire` with `{"deployment_id":"<the dead one>"}`.

Don't spin anything up while another launch is in progress on the same box.
