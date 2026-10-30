@echo off
setlocal

rem ndk-build only needs the NDK. Honor explicit paths, then look in the SDK.
set "NDK_PATH="
if defined ANDROID_NDK_HOME if exist "%ANDROID_NDK_HOME%\ndk-build.cmd" set "NDK_PATH=%ANDROID_NDK_HOME%"
if not defined NDK_PATH if defined ANDROID_NDK_ROOT if exist "%ANDROID_NDK_ROOT%\ndk-build.cmd" set "NDK_PATH=%ANDROID_NDK_ROOT%"
if not defined ANDROID_HOME if defined ANDROID_SDK_ROOT set "ANDROID_HOME=%ANDROID_SDK_ROOT%"
if not defined NDK_PATH if defined ANDROID_HOME if exist "%ANDROID_HOME%\..\NDK\ndk-build.cmd" set "NDK_PATH=%ANDROID_HOME%\..\NDK"
if not defined NDK_PATH if defined ANDROID_HOME if exist "%ANDROID_HOME%\ndk-bundle\ndk-build.cmd" set "NDK_PATH=%ANDROID_HOME%\ndk-bundle"
if not defined NDK_PATH if defined ANDROID_HOME (
    for /d %%D in ("%ANDROID_HOME%\ndk\*") do if exist "%%D\ndk-build.cmd" set "NDK_PATH=%%D"
)

rem Unity Hub installs its SDK and NDK side by side, outside SDK\ndk.
if not defined NDK_PATH (
    for /d %%U in ("%ProgramFiles%\Unity\Hub\Editor\*") do if not defined NDK_PATH if exist "%%U\Editor\Data\PlaybackEngines\AndroidPlayer\NDK\ndk-build.cmd" (
        set "NDK_PATH=%%U\Editor\Data\PlaybackEngines\AndroidPlayer\NDK"
        if not defined ANDROID_HOME set "ANDROID_HOME=%%U\Editor\Data\PlaybackEngines\AndroidPlayer\SDK"
    )
)

if not defined NDK_PATH (
    echo Android NDK not found. Set ANDROID_NDK_HOME to the folder containing ndk-build.cmd.
    exit /b 1
)

rem NDK makefiles require a path without spaces; Windows short paths handle Unity's Program Files location.
for %%N in ("%NDK_PATH%") do set "NDK_PATH=%%~sN"
pushd "%~dp0"
if errorlevel 1 exit /b 1
if not exist "build" mkdir "build"
echo Using NDK: %NDK_PATH%

call "%NDK_PATH%\ndk-build.cmd" ^
  "NDK_PROJECT_PATH=." ^
  "APP_BUILD_SCRIPT=Android.mk" ^
  "NDK_APPLICATION_MK=Application.mk" ^
  "NDK_OUT=build" %*
set "BUILD_EXIT=%ERRORLEVEL%"
popd

if not "%BUILD_EXIT%"=="0" (
    echo Build failed with exit code %BUILD_EXIT%.
    exit /b %BUILD_EXIT%
)
echo Build complete: %~dp0libs\arm64-v8a\libmidnight.so
exit /b 0
