taskKill /IM EnRu.exe /IM AutoHotkey64.exe /IM AutoHotkey32.exe /F
cd /d c:\Program Files\AutoHotkey
Compiler\Ahk2Exe.exe  /in %~dp0EnRu.ahk /bin v2\AutoHotkey32.exe
cd /d %~dp0
timeOut /t 1 /noBreak
start EnRu.exe