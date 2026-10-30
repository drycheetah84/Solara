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
  [switch]$SkipLibBuild
)
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path

$java      = Join-Path $Jdk "bin\java.exe"
$keytool   = Join-Path $Jdk "bin\keytool.exe"
$apktool   = Join-Path $Tool "Apktool\apktool_2.6.1.jar"
$apksigner = Join-Path $Tool "Resources\apksigner.jar"
$zipalign  = Join-Path $Tool "Resources\zipalign.exe"

# 1) build the lib (unless told to reuse the existing one)
if (-not $SkipLibBuild) {
  Write-Host "[1/6] building libmidnight.so"
  & (Join-Path $here "build.bat") | Out-Null
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
  Set-Content -Path $ga -Value $txt -Encoding utf8 -NoNewline
} else { Write-Host "    already injected, skipping" }

# 4) build apk
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$unsigned = Join-Path $OutDir "A2-midnight-50451-unsigned.apk"
$aligned  = Join-Path $OutDir "A2-midnight-50451-aligned.apk"
$signed   = Join-Path $OutDir "A2-midnight-50451.apk"
$ks       = Join-Path $OutDir "midnight.keystore"   # NOTE: debug key; not committed
Write-Host "[4/6] apktool b"
& $java -jar $apktool b "$Decompiled" -o "$unsigned" --use-aapt2 | Out-Null
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

Write-Host ""
Write-Host "DONE -> $signed"
