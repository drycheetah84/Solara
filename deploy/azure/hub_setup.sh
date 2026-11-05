#!/bin/bash
# Solara hub (Azure): recreate the AWS hub layout. Idempotent.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
GAME_IPS="10.10.1.10"

echo "== swap"
if ! swapon --show | grep -q /swapfile; then
  sudo fallocate -l 4G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile >/dev/null && sudo swapon /swapfile
  echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
fi

echo "== packages"
if ! command -v node >/dev/null; then
  curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash - >/dev/null
fi
if ! command -v cloudflared >/dev/null; then
  sudo mkdir -p --mode=0755 /usr/share/keyrings
  curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
  echo 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main' | sudo tee /etc/apt/sources.list.d/cloudflared.list >/dev/null
fi
sudo apt-get update -qq
sudo apt-get install -y -qq nodejs cloudflared python3-winrm sqlite3 >/dev/null
node --version; cloudflared --version | head -1

echo "== files"
rm -rf /tmp/hub && mkdir -p /tmp/hub && tar -xzf /tmp/hub.tgz -C /tmp/hub
sudo mkdir -p /opt/solara /opt/dashboard /opt/halcyon-dist /etc/halcyon /etc/cloudflared
sudo chown -R ubuntu:ubuntu /opt/solara /opt/dashboard /opt/halcyon-dist
if [ ! -f /opt/solara/production.solara.db ]; then
  cp -r /tmp/hub/solara/. /opt/solara/
else
  echo "   (database already present -- not overwritten; binary/config refreshed)"
  cp /tmp/hub/solara/bin/solara /opt/solara/bin/solara
  cp -r /tmp/hub/solara/config/. /opt/solara/config/
fi
chmod 755 /opt/solara/bin/solara
cp -r /tmp/hub/web/dashboard/. /opt/dashboard/

echo "== secrets env"
{ echo "DASHBOARD_ADMIN_KEY=$(tr -d '\r\n' < /opt/solara/config/generated-dashboard_admin_key.txt)"
  echo "DASHBOARD_SESSION_SECRET=$(tr -d '\r\n' < /opt/solara/config/generated-dashboard_session_secret.txt)"; } | sudo tee /etc/halcyon/dashboard.env >/dev/null
sudo chown root:ubuntu /etc/halcyon/dashboard.env && sudo chmod 640 /etc/halcyon/dashboard.env

echo "== solara service"
sudo tee /etc/systemd/system/solara.service >/dev/null <<EOF
[Unit]
Description=Solara (Halcyon backend, Go)
After=network-online.target
Wants=network-online.target

[Service]
User=ubuntu
WorkingDirectory=/opt/solara
ExecStart=/opt/solara/bin/solara serve
Environment=MOTHERSHIP_INSECURE=1
Environment=SOLARA_AGENT_ALLOWED_IPS=${GAME_IPS}
AmbientCapabilities=CAP_NET_BIND_SERVICE
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
sudo mkdir -p /etc/systemd/system/solara.service.d
sudo tee /etc/systemd/system/solara.service.d/dashboard.conf >/dev/null <<EOF
[Service]
EnvironmentFile=/etc/halcyon/dashboard.env
Environment=DASHBOARD_BIND_ADDRESS=127.0.0.1
EOF
sudo tee /etc/systemd/system/solara.service.d/board.conf >/dev/null <<EOF
[Service]
Environment=BOARD_PUBLIC_BASE=https://station.halcy0n.net
EOF

echo "== dashboard build"
cd /opt/dashboard
npm ci --no-audit --no-fund --loglevel=error >/dev/null
NEXT_TELEMETRY_DISABLED=1 npx next build 2>&1 | grep -E "Compiled|rror" | head -5
rm -rf .next/standalone/.next/static .next/standalone/public
cp -r .next/static .next/standalone/.next/ && cp -r public .next/standalone/
sudo tee /etc/systemd/system/halcyon-dashboard.service >/dev/null <<EOF
[Unit]
Description=Halcyon admin dashboard (Next.js, behind dash.halcy0n.net)
After=network-online.target solara.service

[Service]
User=ubuntu
WorkingDirectory=/opt/dashboard/.next/standalone
EnvironmentFile=/etc/halcyon/dashboard.env
Environment=NODE_ENV=production
Environment=NEXT_TELEMETRY_DISABLED=1
Environment=HOSTNAME=127.0.0.1
Environment=PORT=3000
Environment=BACKEND_HOST=127.0.0.1
ExecStart=/usr/bin/node server.js
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/dashboard/.next/standalone/.next

[Install]
WantedBy=multi-user.target
EOF

echo "== DLL bundle server (private network only)"
sudo tee /etc/systemd/system/halcyon-dist.service >/dev/null <<EOF
[Unit]
Description=Halcyon game-box bundle (HalcyonA2.dll, agent) on the private network
After=network-online.target

[Service]
User=ubuntu
WorkingDirectory=/opt/halcyon-dist
ExecStart=/usr/bin/python3 -m http.server 8000 --bind 10.10.1.4
Restart=always

[Install]
WantedBy=multi-user.target
EOF

echo "== cloudflared"
sudo cp /tmp/hub/cf/95306d5c-bfed-4553-bc71-9b9966f4cf63.json /etc/cloudflared/
sudo cp /tmp/hub/cf/config.yml /etc/cloudflared/config.yml
sudo chmod 600 /etc/cloudflared/95306d5c-bfed-4553-bc71-9b9966f4cf63.json
if [ ! -f /etc/systemd/system/cloudflared.service ]; then sudo cloudflared service install >/dev/null 2>&1 || true; fi

echo "== nightly DB backup (local, 14 days)"
sudo tee /etc/cron.d/solara-backup >/dev/null <<'EOF'
17 4 * * * ubuntu mkdir -p /opt/solara/backups && cp /opt/solara/production.solara.db /opt/solara/backups/production.solara.db.$(date +\%Y\%m\%d) && cp /opt/solara/production.solara.db.meta /opt/solara/backups/production.solara.db.meta.$(date +\%Y\%m\%d) && find /opt/solara/backups -mtime +14 -delete
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now solara halcyon-dashboard halcyon-dist cloudflared >/dev/null 2>&1
sudo systemctl restart solara halcyon-dashboard halcyon-dist cloudflared
sleep 8
systemctl is-active solara halcyon-dashboard halcyon-dist cloudflared
sudo ss -ltnp | grep -E ':(90|78|50|80|8080|3000|8000|9095|9100) ' | awk '{print $4}' | sort | tr '\n' ' '; echo
