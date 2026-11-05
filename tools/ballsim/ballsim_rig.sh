#!/bin/bash
# ballsim_rig.sh : LOCAL test only. Station Prime server (hardlinked copy) + the freshly built HalcyonA2.dll + N clients,
# then prints the [RBSTAT] rollback-health lines. Never touches live servers or the original game install.
set -u
NCLI=${1:-1}
SC=/c/Users/drych/HalcyonEvent/ServerCopy/A2/Binaries/Win64
C=/c/Users/drych/Downloads/33526083366990015/client50451/A2/Binaries/Win64
DLL='C:\Users\drych\ProjectDrift\Solara\build\50451\x64\Release\HalcyonA2.dll'
LOG=/c/Users/drych/AppData/Local/Temp/HalcyonA2.log
powershell -NoProfile -Command "Get-Process A2-Win64-Shipping -ErrorAction SilentlyContinue | Stop-Process -Force" 2>/dev/null
sleep 2; rm -f "$LOG"
SK=$(grep -oP "^SERVER_API_KEY=\K.*" /c/Users/drych/solara-local/config/legacy-secrets.env)
ARGS="-nullrhi -nosound -log -SpecEdit -SpecEditLocalTest -SpecEditSandbox -DashboardApiUrl=http://127.0.0.1:78 -BackendHost=127.0.0.1 -MothershipHost=127.0.0.1 -ServerName=BallsimTest -ServerApiKey=$SK"
MSYS_NO_PATHCONV=1 powershell -NoProfile -Command "Start-Process -FilePath '$(cygpath -w $SC/A2-Win64-Shipping.exe)' -WorkingDirectory '$(cygpath -w $SC)' -ArgumentList '$ARGS'"
for i in $(seq 1 40); do
  PID=$(powershell -NoProfile -Command "(Get-Process A2-Win64-Shipping -ErrorAction SilentlyContinue | Where-Object { \$_.Path -like '*ServerCopy*' } | Select-Object -First 1).Id")
  [ -n "$PID" ] && break; sleep 1
done
echo "server pid=$PID"
/c/Users/drych/ProjectDrift/Solara/bin/a2inject.exe -pid "$PID" -dll "$DLL" -watch=false 2>&1 | tail -1
for i in $(seq 1 90); do grep -qa "RBSTAT\] correction-push counter hook" "$LOG" 2>/dev/null && break; sleep 2; done
grep -aE "RBTICK\] rollback step driver hook|RBSTAT\] correction-push|RBFRAMEFIX\] missed-inputs sender" "$LOG" | head -4
# wait for the station map to be up before clients connect
for i in $(seq 1 90); do grep -qa "issued: open" "$LOG" 2>/dev/null && break; sleep 2; done
sleep 25
for n in $(seq 1 "$NCLI"); do
  MSYS_NO_PATHCONV=1 powershell -NoProfile -Command "Start-Process -FilePath '$(cygpath -w $C/A2-Win64-Shipping.exe)' -WorkingDirectory '$(cygpath -w $C)' -ArgumentList '-windowed -ResX=960 -ResY=540 -log -SpecEditConnect=127.0.0.1:7777'"
  sleep 20
done
S=$SECONDS; until [ "$(grep -ac 'player joined' "$LOG")" -ge "$NCLI" ] || [ $((SECONDS-S)) -gt 240 ]; do sleep 3; done
echo "joined: $(grep -ac 'player joined' "$LOG")"
sleep 15
grep -a "\[RBSTAT\] corr" "$LOG" | tail -8
