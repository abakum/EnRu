setlocal EnableExtensions

taskKill /im EnRu.exe /im AutoHotkey64.exe /im AutoHotkey32.exe /f

for /f "tokens=2*" %%a in ('reg.exe query "HKLM\SOFTWARE\AutoHotkey" /v "InstallDir"') do (cd /d %%b)
Compiler\Ahk2Exe.exe  /in %~dp0EnRu.ahk /bin v2\AutoHotkey32.exe
cd /d %~dp0
timeOut /t 1 /noBreak
start EnRu.exe