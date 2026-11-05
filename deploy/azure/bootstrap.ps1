# Halcyon game box bootstrap (Azure, run once as SYSTEM/admin on the Windows game VM):
#   iwr http://10.10.1.4:8000/bootstrap.ps1 -UseBasicParsing | iex
# Pulls HalcyonA2.dll + the allocator agent from the Solara hub over the VNet, installs the VC++ runtime and 7-Zip,
# unpacks the A2 50451 build (downloaded separately to C:\Halcyon\dl\game.download), opens the game UDP ports,
# enables OpenSSH (hub key only) and registers the agent as a startup task that connects to the hub on 10.10.1.4:9100.
$ErrorActionPreference = 'Stop'
$ProgressPreference    = 'SilentlyContinue'
$hub  = '10.10.1.4'
$src  = "http://${hub}:8000"
$root = 'C:\Halcyon'
$box  = 'win-gamebox-1'
$cap  = 5

function Step($m) { Write-Host "==> $m" }

New-Item -ItemType Directory -Force -Path $root, "$root\game", "$root\logs" | Out-Null

Step 'agent files'
foreach ($f in 'HalcyonAgent.ps1', 'Inject.ps1', 'server.config.psd1', 'HalcyonA2.dll', 'admin_key.pub') {
    Invoke-WebRequest "$src/$f" -OutFile "$root\$f" -UseBasicParsing
}

Step 'VC++ 2015-2022 x64 runtime'
$vc = "$env:TEMP\vc_redist.x64.exe"
Invoke-WebRequest 'https://aka.ms/vs/17/release/vc_redist.x64.exe' -OutFile $vc -UseBasicParsing
Start-Process $vc -ArgumentList '/install', '/quiet', '/norestart' -Wait

Step '7-Zip (for the RAR game archive)'
$7z = 'C:\Program Files\7-Zip\7z.exe'
if (-not (Test-Path $7z)) {
    Invoke-WebRequest 'https://www.7-zip.org/a/7z2409-x64.exe' -OutFile "$env:TEMP\7z.exe" -UseBasicParsing
    Start-Process "$env:TEMP\7z.exe" -ArgumentList '/S' -Wait
}

Step 'firewall: game UDP 7777-7790, ssh from the VNet'
Get-NetFirewallRule -DisplayName 'Halcyon*' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule -DisplayName 'Halcyon game UDP' -Direction Inbound -Protocol UDP -LocalPort 7777-7790 -Action Allow | Out-Null

Step 'loopback :78 / :90 -> Solara (DLL default DashboardApiUrl is 127.0.0.1:78)'
foreach ($p in 78, 90) {
    cmd /c "netsh interface portproxy delete v4tov4 listenaddress=127.0.0.1 listenport=$p >nul 2>&1"
    cmd /c "netsh interface portproxy add v4tov4 listenaddress=127.0.0.1 listenport=$p connectaddress=$hub connectport=$p >nul 2>&1"
}
Set-Service iphlpsvc -StartupType Automatic; Start-Service iphlpsvc

Step 'OpenSSH server (key auth from the hub only)'
if (-not (Get-Service sshd -ErrorAction SilentlyContinue)) { Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0 | Out-Null }
$ak = 'C:\ProgramData\ssh\administrators_authorized_keys'
New-Item -ItemType Directory -Force -Path 'C:\ProgramData\ssh' | Out-Null
Copy-Item "$root\admin_key.pub" $ak -Force
icacls $ak /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F' | Out-Null
Set-Service sshd -StartupType Automatic; Start-Service sshd
New-NetFirewallRule -DisplayName 'Halcyon ssh (VNet)' -Direction Inbound -Protocol TCP -LocalPort 22 -RemoteAddress 10.10.0.0/16 -Action Allow | Out-Null
New-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -Name DefaultShell -Value 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' -PropertyType String -Force | Out-Null

Step 'checks'
foreach ($p in 78, 90, 9100, 8000) {
    $ok = (Test-NetConnection $hub -Port $p -WarningAction SilentlyContinue).TcpTestSucceeded
    Write-Host ("  hub :{0} reachable: {1}" -f $p, $ok)
}
Write-Host 'bootstrap done (game unpack + agent task are separate steps).'
