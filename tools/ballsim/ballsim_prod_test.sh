#!/bin/bash
# ballsim_prod_test.sh : a LOCAL Station Prime server (hardlinked copy, never the original install) with the freshly
# built HalcyonA2.dll, registered on the PROD backend through the Cloudflare tunnel so the owner can join it from the
# headset on the home LAN. Live servers are not touched. Stop it with: Stop-Process -Name A2-Win64-Shipping.
set -u
SC=/c/Users/drych/HalcyonEvent/ServerCopy/A2/Binaries/Win64
DLL='C:\Users\drych\ProjectDrift\Solara\build\50451\x64\Release\HalcyonA2.dll'
LOG=/c/Users/drych/AppData/Local/Temp/HalcyonA2.log
LANIP=${LANIP:-192.168.1.158}
powershell -NoProfile -Command "Get-Process A2-Win64-Shipping -ErrorAction SilentlyContinue | Where-Object { \$_.Path -like '*ServerCopy*' } | Stop-Process -Force" 2>/dev/null
sleep 2; rm -f "$LOG"
SK=$(grep -oP "^SERVER_API_KEY=\K.*" /c/Users/drych/ProjectDrift/Solara/config/legacy-secrets.env | tr -d '\r"')
ARGS="-nullrhi -nosound -log -SpecEdit -SpecEditLocalTest -SpecEditSandbox -NoSpecBlock -DashboardApiUrl=https://station.halcy0n.net -BackendHost=station.halcy0n.net -BackendPort=443 -MothershipHost=mothership-prod.halcy0n.net -MothershipPort=443 -RegisterIp=$LANIP -ServerName=BallsimTest -ServerApiKey=$SK"
MSYS_NO_PATHCONV=1 powershell -NoProfile -Command "Start-Process -FilePath '$(cygpath -w $SC/A2-Win64-Shipping.exe)' -WorkingDirectory '$(cygpath -w $SC)' -ArgumentList '$ARGS'"
for i in $(seq 1 40); do
  PID=$(powershell -NoProfile -Command "(Get-Process A2-Win64-Shipping -ErrorAction SilentlyContinue | Where-Object { \$_.Path -like '*ServerCopy*' } | Select-Object -First 1).Id")
  [ -n "$PID" ] && break; sleep 1
done
echo "server pid=$PID"
/c/Users/drych/ProjectDrift/Solara/bin/a2inject.exe -pid "$PID" -dll "$DLL" -watch=false 2>&1 | tail -1
for i in $(seq 1 120); do grep -qaE "\[REGISTER\]|registered|register_server" "$LOG" 2>/dev/null && break; sleep 2; done
sleep 10
grep -aE "RBSTAT\] correction-push|REGISTER|register_server|station id|CURLREDIRECT\] http|issued: open" "$LOG" | cut -c1-220 | head -12
