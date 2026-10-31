@echo off
cd /d "%~dp0"
call "C:\Program Files\Microsoft Visual Studio\18\Community\VC\Auxiliary\Build\vcvars64.bat" >nul 2>&1
set CC=cl /nologo /c /O2 /MT /EHsc /GS- /std:c++20 /bigobj /Zm400 /I"C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\HalcyonA2\ThirdParty\MinHook\include" /I"C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui" /I"C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\backends" /DIMGUI_DISABLE_OBSOLETE_FUNCTIONS
ml64 /nologo /c /Fo thunks.obj thunks.asm || exit /b 1
%CC% dllmain.cpp se_render.cpp se_ui.cpp se_game.cpp se_sdk_glue.cpp se_mcp_net.cpp || exit /b 1
%CC% "C:\Users\drych\ProjectDrift\Solara\Server\gamesdk\50451\SDK\Basic.cpp" "C:\Users\drych\ProjectDrift\Solara\Server\gamesdk\50451\SDK\CoreUObject_functions.cpp" || exit /b 1
%CC% "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\imgui.cpp" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\imgui_draw.cpp" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\imgui_tables.cpp" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\imgui_widgets.cpp" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\backends\imgui_impl_dx12.cpp" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditor\ThirdParty\imgui\backends\imgui_impl_win32.cpp" || exit /b 1
cl /nologo /c /O2 /MT "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\HalcyonA2\ThirdParty\MinHook\src\buffer.c" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\HalcyonA2\ThirdParty\MinHook\src\hook.c" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\HalcyonA2\ThirdParty\MinHook\src\trampoline.c" "C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\HalcyonA2\ThirdParty\MinHook\src\hde\hde64.c" || exit /b 1
cl /nologo /LD /Fe:dsound.dll *.obj /link /DEF:exports.def kernel32.lib user32.lib d3d12.lib dxgi.lib psapi.lib ws2_32.lib /MAP:dsound.map || exit /b 1
echo BUILD_OK
