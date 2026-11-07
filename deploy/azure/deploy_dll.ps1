# Rolling HalcyonA2.dll deploy on ONE Windows game box. Run it ON the box with `powershell -File` (not stdin):
#   powershell -NoProfile -ExecutionPolicy Bypass -File C:\Halcyon\deploy_dll.ps1 -Md5 <first 8 hex> -DryRun
# It downloads the DLL and server.config.psd1 the hub serves (http://10.10.1.4:8000), swaps the file under the live
# path, then restarts the game servers one at a time, waiting for the agent to relaunch AND inject each before the next.
# Read docs/DEPLOY_GAME_SERVERS.md first: a restart kicks everyone on that server.
param(
    [Parameter(Mandatory = $true)][string]$Md5,      # first 8 hex of the MD5 of the DLL you uploaded; aborts on mismatch
    [string[]]$Skip = @(),                           # deployment ids to leave running (busy stations)
    [switch]$DryRun,                                 # print what would happen, change nothing
    [string]$Dist = 'http://10.10.1.4:8000'
)
$ProgressPreference = 'SilentlyContinue'
$ErrorActionPreference = 'Stop'
function T { (Get-Date).ToUniversalTime().ToString('HH:mm:ss') }
$Md5 = $Md5.ToLower()
$new = "C:\Halcyon\HalcyonA2-$Md5.dll"
$live = 'C:\Halcyon\HalcyonA2.dll'
$cfg = 'C:\Halcyon\server.config.psd1'

# 0. which servers would restart. All servers run -ServerName=Halcyon: skip by -DashboardDeploymentId, never by name.
$procs = @(Get-CimInstance Win32_Process -Filter "Name='A2-Win64-Shipping.exe'")
$restart = @(); $kept = @()
foreach ($p in $procs) {
    $dep = if ($p.CommandLine -match '-DashboardDeploymentId=(\S+)') { $Matches[1] } else { '?' }
    if ($Skip | Where-Object { $_ -and $dep -like "$_*" }) { $kept += "$($p.ProcessId)=$dep" } else { $restart += [pscustomobject]@{ Pid = $p.ProcessId; Dep = $dep } }
}
"$(T) restart: " + (($restart | ForEach-Object { "$($_.Pid)=$($_.Dep)" }) -join ', ')
"$(T) keep:    " + ($kept -join ', ')
if ($Skip.Count -gt 0 -and $kept.Count -eq 0) { "$(T) -Skip given but it matched no server -- ABORT"; exit 1 }

# 1. fetch and check the DLL before touching anything
Invoke-WebRequest "$Dist/HalcyonA2.dll" -OutFile $new -UseBasicParsing
$h = (Get-FileHash $new -Algorithm MD5).Hash.Substring(0, 8).ToLower()
if ($h -ne $Md5) { "$(T) downloaded dll md5 $h, expected $Md5 -- ABORT"; exit 1 }
if ($DryRun) { "$(T) dry run: dll ok, nothing changed"; exit 0 }

# 2. the repo server config (SpecEditor flags) -- every deploy copies it
Copy-Item $cfg "$cfg.pre-$Md5" -Force
Invoke-WebRequest "$Dist/server.config.psd1" -OutFile $cfg -UseBasicParsing
"$(T) config: " + ((Get-Content $cfg | Select-String 'DllPath').Line.Trim())

# 3. swap the file under the live path: a loaded DLL cannot be overwritten, but it can be renamed
$oldHash = (Get-FileHash $live -Algorithm MD5).Hash.Substring(0, 8).ToLower()
if ($oldHash -ne $Md5) {
    Rename-Item $live "HalcyonA2-$oldHash-$(Get-Date -Format HHmmss).dll" -Force   # this is the rollback copy
    Copy-Item $new $live -Force
}
"$(T) live path now " + (Get-FileHash $live -Algorithm MD5).Hash.Substring(0, 8)

# 4. one server at a time. The agent injects ~60 s after a process starts: never kill a replacement that looks DLL-less.
$ErrorActionPreference = 'Continue'
foreach ($s in $restart) {
    if (-not (Get-Process -Id $s.Pid -ErrorAction SilentlyContinue)) { "$(T) pid $($s.Pid) already gone"; continue }
    $before = @(Get-Process A2-Win64-Shipping -ErrorAction SilentlyContinue | ForEach-Object { $_.Id })
    "$(T) stopping $($s.Pid) ($($s.Dep))"
    Stop-Process -Id $s.Pid -Force
    $deadline = (Get-Date).AddSeconds(240)
    $newPid = $null; $done = $false
    while ((Get-Date) -lt $deadline -and -not $done) {
        Start-Sleep 5
        $fresh = @(Get-Process A2-Win64-Shipping -ErrorAction SilentlyContinue | ForEach-Object { $_.Id } | Where-Object { $before -notcontains $_ })
        if ($fresh.Count -gt 0) { $newPid = $fresh[0] }
        if ($newPid -and (Select-String -Path C:\Halcyon\logs\agent.log -Pattern "injected pid $newPid\b" -Quiet)) { $done = $true }
    }
    if ($done) { "$(T)   -> replacement $newPid injected"; Start-Sleep 20 }
    else { "$(T)   -> no injected replacement in 240s (new pid: $newPid) -- STOPPING the rollout, check the watchdog"; break }
}

# 5. verify: by process start time and command line (hashing the module path reads the new file, not the loaded image)
"$(T) servers:"
Get-CimInstance Win32_Process -Filter "Name='A2-Win64-Shipping.exe'" | ForEach-Object {
    $dep = if ($_.CommandLine -match '-DashboardDeploymentId=(\S+)') { $Matches[1] } else { '?' }
    "  pid $($_.ProcessId) dep $dep started $($_.CreationDate.ToUniversalTime().ToString('HH:mm:ss')) SpecEdit=$($_.CommandLine -match '-SpecEdit\b') Sandbox=$($_.CommandLine -match '-SpecEditSandbox')"
}
