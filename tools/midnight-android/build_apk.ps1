<#
  build_apk.ps1 - patch an A2 (UE) decompiled APK with libmidnight.so and produce a
  signed, installable APK. Reproducible, headless, non-destructive.

  Pipeline: build libmidnight.so (build.bat) -> copy into lib/arm64-v8a ->
            inject System.loadLibrary("midnight") right after "Unreal" in GameActivity.smali
            (idempotent; backs up the smali once) -> apktool b -> zipalign -> apksigner.

  The DECOMPILED TREE is an apktool working copy (not the original APK); the one edited
  smali is backed up to <file>.orig so the edit is reversible. Nothing here touches the
  original .apk or the Midnight source.

  Usage (defaults target this machine's APK.Easy.Tool layout):
    pwsh -File build_apk.ps1
    pwsh -File build_apk.ps1 -Decompiled "<decompiled dir>" -OutDir "<out dir>"
#>
param(
  [string]$Decompiled = "C:\Users\drych\Downloads\APK.Easy.Tool.v1.60.Portable\1-Decompiled APKs\A2-Android-Shipping-arm64 (2)",
  [string]$OutDir     = "C:\Users\drych\Downloads\APK.Easy.Tool.v1.60.Portable\2-Recompiled APKs",
  [string]$Tool       = "C:\Users\drych\Downloads\APK.Easy.Tool.v1.60.Portable",
  [string]$Jdk        = "C:\Program Files\Java\jdk-17",
  [string]$StorePass  = "midnight123",
  [switch]$SkipLibBuild,
  # Optional explicit tool paths (default: the APK Easy Tool layout under -Tool, as before).
  [string]$Apktool    = "",
  [string]$Apksigner  = "",
  [string]$Zipalign   = "",
  [string]$OutName    = "A2-midnight-50451"
)
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path

$java      = Join-Path $Jdk "bin\java.exe"
$keytool   = Join-Path $Jdk "bin\keytool.exe"
$apktool   = if ($Apktool)   { $Apktool }   else { Join-Path $Tool "Apktool\apktool_2.6.1.jar" }
$apksigner = if ($Apksigner) { $Apksigner } else { Join-Path $Tool "Resources\apksigner.jar" }
$zipalign  = if ($Zipalign)  { $Zipalign }  else { Join-Path $Tool "Resources\zipalign.exe" }

# 1) build the lib (unless told to reuse the existing one)
if (-not $SkipLibBuild) {
  Write-Host "[1/6] building libmidnight.so"
  & (Join-Path $here "build.bat") | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "build.bat (libmidnight.so) failed" }
}
$so = Join-Path $here "libs\arm64-v8a\libmidnight.so"
if (-not (Test-Path $so)) { throw "libmidnight.so not found: $so" }

# 2) copy the lib into the APK tree
Write-Host "[2/6] copy lib -> lib/arm64-v8a"
Copy-Item $so (Join-Path $Decompiled "lib\arm64-v8a\libmidnight.so") -Force

# 3) inject the loader after the "Unreal" load (idempotent)
Write-Host "[3/6] inject System.loadLibrary(`"midnight`")"
$ga = Join-Path $Decompiled "smali\com\epicgames\unreal\GameActivity.smali"
$txt = Get-Content -Raw $ga
if ($txt -notmatch '"midnight"') {
  if (-not (Test-Path "$ga.orig")) { Copy-Item $ga "$ga.orig" }
  $anchor = @"
    const-string v0, "Unreal"

    invoke-static {v0}, Ljava/lang/System;->loadLibrary(Ljava/lang/String;)V
"@
  $inject = @"
    const-string v0, "Unreal"

    invoke-static {v0}, Ljava/lang/System;->loadLibrary(Ljava/lang/String;)V

    const-string v0, "midnight"

    invoke-static {v0}, Ljava/lang/System;->loadLibrary(Ljava/lang/String;)V
"@
  # replace only the first occurrence
  $idx = $txt.IndexOf($anchor)
  if ($idx -lt 0) { throw "anchor (Unreal loadLibrary) not found in GameActivity.smali" }
  $txt = $txt.Substring(0,$idx) + $inject + $txt.Substring($idx + $anchor.Length)
  # UTF-8 WITHOUT a BOM: Windows PowerShell 5.1's `Set-Content -Encoding utf8` writes one, and smali then fails
  # with "[1,0] required (...)+ loop did not match anything at input '?'" (pwsh 7 writes no BOM; this works in both).
  [System.IO.File]::WriteAllText($ga, $txt, (New-Object System.Text.UTF8Encoding($false)))
} else { Write-Host "    already injected, skipping" }

# 4) build apk
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$unsigned = Join-Path $OutDir "$OutName-unsigned.apk"
$aligned  = Join-Path $OutDir "$OutName-aligned.apk"
$signed   = Join-Path $OutDir "$OutName.apk"
$ks       = Join-Path $OutDir "midnight.keystore"   # NOTE: debug key; not committed
Write-Host "[4/6] apktool b"
# From here on only native tools run, each checked by exit code. Windows PowerShell 5.1 would otherwise turn their
# harmless stderr output (java warnings, apktool progress) into a terminating error under "Stop".
$ErrorActionPreference = "Continue"
# apktool 2.x needs --use-aapt2; 3.x always uses aapt2 and no longer has the flag.
$apktoolVer = (& $java -jar $apktool --version 2>$null | Select-Object -Last 1)
$aaptFlag = if ("$apktoolVer" -match '^[012]\.') { @('--use-aapt2') } else { @() }
& $java -jar $apktool b "$Decompiled" -o "$unsigned" @aaptFlag | Out-Null
if ($LASTEXITCODE -ne 0) { throw "apktool build failed" }

# 5) zipalign (extractNativeLibs=true -> plain 4-byte align, no -p needed)
Write-Host "[5/6] zipalign"
& $zipalign -f 4 "$unsigned" "$aligned"
if ($LASTEXITCODE -ne 0) { throw "zipalign failed" }

# 6) sign (debug keystore; minSdk 29 -> v2/v3)
Write-Host "[6/6] sign + verify"
if (-not (Test-Path $ks)) {
  & $keytool -genkeypair -keystore "$ks" -alias midnight -keyalg RSA -keysize 2048 `
    -validity 10000 -storepass $StorePass -keypass $StorePass -dname "CN=Midnight, O=Halcyon, C=US" | Out-Null
}
& $java -jar $apksigner sign --ks "$ks" --ks-key-alias midnight `
  --ks-pass "pass:$StorePass" --key-pass "pass:$StorePass" `
  --v2-signing-enabled true --v3-signing-enabled true --out "$signed" "$aligned"
if ($LASTEXITCODE -ne 0) { throw "apksigner sign failed" }
& $java -jar $apksigner verify --verbose "$signed"
if ($LASTEXITCODE -ne 0) { throw "apksigner verify failed" }

Write-Host ""
Write-Host "DONE -> $signed"
