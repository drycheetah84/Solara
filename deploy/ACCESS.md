# Deploy access (CREDENTIALS -- private repo only)

Owner-approved 2026-11-08 so the dev team can deploy. Steps are in [DEPLOY_GAME_SERVERS.md](../docs/DEPLOY_GAME_SERVERS.md).
If this repo ever goes public or someone leaves the team, rotate everything below.

**SSH is still IP-allowlisted:** your public IP must be added to the Azure NSG `hub-nsg` (resource group `halcyon-rg`) by the owner, or ssh times out.

## Hub (Ubuntu) -- `ssh -i solara_azure ubuntu@20.55.26.98`

Save the key below as `~/.ssh/solara_azure` (`chmod 600`).

```
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDyrzOY0dxCwxLZOw/TZKmcfC0Nh5meEpALlRjUq+qlNQAAAJg2ytSxNsrU
sQAAAAtzc2gtZWQyNTUxOQAAACDyrzOY0dxCwxLZOw/TZKmcfC0Nh5meEpALlRjUq+qlNQ
AAAEBdvwC9WIqLhpVLI8N8DGYJ+GEiwcJMtHwMUMfT2xwBGvKvM5jR3ELDEtk7D9NkqZx8
LQ2HmZ4SkAuVGNSr6qU1AAAAEHNvbGFyYS1henVyZS1odWIBAgMEBQ==
-----END OPENSSH PRIVATE KEY-----
```

## Game boxes (Windows) -- reach them from the hub: `ssh halcyon@10.10.1.10` / `ssh halcyon@10.20.1.10`

Local admin (RDP from an allowlisted IP: box 1 20.55.48.184, box 2 20.83.58.255):

- user: `halcyon`
- password: `Hx!3wwKawQ9pqYVuW7puzI19a`

## Dashboard admin key

On the hub in `/etc/halcyon/dashboard.env` (`DASHBOARD_ADMIN_KEY`); read it there with sudo for the `:9095` deploy endpoints.
