# Cloudflare Tunnel — Solara under `*.halcy0n.net`

Exposes the Solara backend services through one named Cloudflare Tunnel (`halcyon`),
one hostname per service, with Cloudflare edge TLS (no origin certs needed).

- **Tunnel:** `halcyon`  id `95306d5c-bfed-4553-bc71-9b9966f4cf63`
- **Config (ingress):** `deploy/cloudflare/halcyon.yml` (copy of `~/.cloudflared/halcyon.yml`)
- **Credentials:** `~/.cloudflared/95306d5c-…json` — **secret, NOT in git.** Recreate with
  `cloudflared tunnel create halcyon` if lost (it deletes/recreates the tunnel id).

## Hostnames → service

| Hostname | Solara service | Local port | Game domain it fronts |
|---|---|---|---|
| `mothership.halcy0n.net` | Mothership (login/auth) | 90 | `*.aa-mothership.com` |
| `station.halcy0n.net` | A2StationDb (stations/roles/quests/deployments) | 78 | `api.oriondrift.net` |
| `eos.halcy0n.net` | EOS gateway (server list) | 50 | EOS / `*.epicgames.com` |
| `eosws.halcy0n.net` | EOS websocket | 80 | EOS WS |
| `dash.halcy0n.net` | Ares web dashboard (admin) | 8080 | — (protect w/ CF Access) |

Private, **not** tunneled: HalcyonSocket orchestrator `:9095`/`:9100`.

### Two things the tunnel cannot/should not do
- **Game traffic (`7777-7787/UDP`)** cannot go through a Cloudflare Tunnel (no arbitrary
  UDP). The game servers still need a public `IP:port`.
- **`api.halcy0n.net`** already has an A record → `157.173.194.216` (a different box), so it
  was left alone; `station.halcy0n.net` is used for `:78`. Repoint `api.*` to this tunnel
  (dashboard or API) if you want `api` as the canonical station name.

## Bring it up

```bash
# 1. backend (binds :50 :78 :80 :90 :8080 + private :9095/:9100)
bin/solara.exe serve

# 2. tunnel (halcy0n.net already authorized by ~/.cloudflared/cert.pem)
cloudflared --config ~/.cloudflared/halcyon.yml tunnel run halcyon
```

First-time only (already done for this account): `cloudflared tunnel create halcyon`,
then `cloudflared tunnel route dns halcyon <hostname>` for each hostname above.

### Run them as services (survive logout/reboot)
```bash
cloudflared --config ~/.cloudflared/halcyon.yml service install   # Windows service
# solara: wrap bin/solara.exe with NSSM or a scheduled task set to run at startup
```

## Verify
```bash
curl -s -o /dev/null -w "%{http_code}\n" https://mothership.halcy0n.net/   # 200 = origin reached
```
A `404`/`500` from these hosts is the **origin** (Solara) answering — still proves routing.
A Cloudflare `1033`/`530` error page means the tunnel/origin is down.

## Point the game at these
Update the redirect (Midnight `opts.h` / the in-memory `DashboardApiUrl` config patch /
Fiddler rules) so the game's Epic/Mothership/Orion hosts resolve to the matching
`*.halcy0n.net` hostname over HTTPS:443 instead of `http://<ip>:<port>`.
