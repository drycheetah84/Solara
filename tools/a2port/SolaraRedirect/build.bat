@echo off
cd /d "C:\Users\drych\ProjectDrift\SolaraRedirect"
call "C:\Program Files\Microsoft Visual Studio\18\Community\VC\Auxiliary\Build\vcvars64.bat" >nul 2>&1
cl /nologo /LD /O2 /EHsc SolaraRedirect.cpp /link /OUT:SolaraRedirect.dll
