taskKill /IM EnRu.exe /F
taskKill /IM AutoHotkey64.exe /F
taskKill /IM AutoHotkey32.exe /F
cd /d c:\Program Files\AutoHotkey
:Compiler\Ahk2Exe.exe  /in %~dp0EnRu.ahk /bin v2\AutoHotkey32.exe /icon %~dp0EnRu.ico
Compiler\Ahk2Exe.exe  /in %~dp0EnRu.ahk /bin v2\AutoHotkey32.exe
cd /d %~dp0
timeOut /t 1 /noBreak
start EnRu.exe