@echo off
chcp 65001
setlocal

set v=cmd\EnRu\winres
md %v% 2>nul

echo Creating 4 RЯ combined PNG files...

call :create_combined 16 1
call :create_combined 20 2
call :create_combined 32 3
call :create_combined 40 4
call :create_combined 48 5
call :create_combined 64 6
call :create_combined 96 7
call :create_combined 256 8

echo Done! Created files:
dir %v%\*.png /b

goto :EOF

:create_combined
set size=%1
set shadow=shadowx=%2:shadowy=%2:
set /a half=%size%/2

:ffmpeg -f lavfi -i color=red:size=%half%x%size% -f lavfi -i color=blue:size=%half%x%size% -filter_complex "[0]format=rgba,drawtext=text=R:font=impact:fontcolor=green:fontsize=%size%:x=0:y=(h-text_h)/2:%shadow%boxborderw=%size%/8:box=1:boxcolor=blue,chromakey=green:0.1:0.0[left];[1]format=rgba,drawtext=text=Я:font=impact:fontcolor=white:fontsize=%size%:x=w-text_w:y=(h-text_h)/2:%shadow%boxborderw=%size%/8:box=1:boxcolor=red[right];[left][right]hstack" -frames:v 1 -y %v%\%size%x%size%.png
ffmpeg -f lavfi -i color=red:size=%half%x%size% -f lavfi -i color=blue:size=%half%x%size% -filter_complex "[0]drawtext=text=R:font=impact:fontcolor=white:fontsize=%size%:x=0:y=(h-text_h)/2:%shadow%boxborderw=%size%/8:box=1:boxcolor=blue[left];[1]drawtext=text=Я:font=impact:fontcolor=white:fontsize=%size%:x=w-text_w:y=(h-text_h)/2:%shadow%boxborderw=%size%/8:box=1:boxcolor=red[right];[left][right]hstack" -frames:v 1 -y %v%\%size%x%size%.png

goto :EOF