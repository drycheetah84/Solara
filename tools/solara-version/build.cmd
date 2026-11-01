@echo off
rem Builds the A2 50451 PC-client redirect proxy two ways: build\XINPUT1_4.dll (use this one) and build\version.dll.
setlocal
cd /d "%~dp0"
call "C:\Program Files\Microsoft Visual Studio\18\Community\VC\Auxiliary\Build\vcvars64.bat" >nul 2>&1 || exit /b 1
set MH=C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\HalcyonA2\ThirdParty\MinHook
if not exist build mkdir build
cl /nologo /c /O2 /MT /Fobuild\ "%MH%\src\buffer.c" "%MH%\src\hook.c" "%MH%\src\trampoline.c" "%MH%\src\hde\hde64.c" || exit /b 1
set MHOBJ=build\buffer.obj build\hook.obj build\trampoline.obj build\hde64.obj
cl /nologo /c /O2 /MT /EHsc /std:c++20 /I"%MH%\include" /Fobuild\version.obj version.cpp || exit /b 1
link /nologo /DLL /DEF:version.def /OUT:build\version.dll build\version.obj %MHOBJ% kernel32.lib || exit /b 1
cl /nologo /c /O2 /MT /EHsc /std:c++20 /DPROXY_XINPUT /I"%MH%\include" /Fobuild\xinput.obj version.cpp || exit /b 1
link /nologo /DLL /DEF:xinput.def /OUT:build\XINPUT1_4.dll build\xinput.obj %MHOBJ% kernel32.lib || exit /b 1
echo built build\XINPUT1_4.dll and build\version.dll
