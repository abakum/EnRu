chcp 65001
set p=
set v=0\
set t=Z
set c=#0000FF
call :a
set t=Я
set c=#FF0000
call :a

set v=1\
md %v%
set c=blue
set t=En
set x=(w-text_w)/2
set o=1
call :b
set c=red
set t=Ru
set o=2
call :b
call :aa
pause

set v=2\
md %v%
set c=blue
set t=Z
set x=1
set o=1
call :b
set c=red
set t=Я
set x=w-text_w
set o=2
call :b
call :bb
pause

set v=3\
md %v%
set c=blue
set t=R
set x=0
set o=1
call :b
set c=red
set t=Я
set x=w-text_w
set o=2
call :b
call :cc
pause

set v=4\
md %v%
set c=red
set t=Z
set x=1
set p=:boxborderw=9:box=1:boxcolor=blue
set o=1
call :b
set c=blue
set t=Я
set x=w-text_w
set p=:boxborderw=5:box=1:boxcolor=red
set o=2
call :b
call :bb
pause

set v=5\
md %v%
set c=red
set t=R
set x=0
set p=:boxborderw=6:box=1:boxcolor=blue
set o=1
call :b
set c=blue
set t=Я
set x=w-text_w
set p=:boxborderw=6:box=1:boxcolor=red
set o=2
call :b
call :cc

goto :EOF

:a
echo https://redketchup.io/icon-editor arial 22 bold %t% #FFFFFF on %c%
goto :EOF

:b
ffmpeg -f lavfi -i color=%c%:size=32x32 -frames:v 1 -filter_complex drawtext=text=%t%:font=impact:fontcolor=white:fontsize=32:x=%x%:y=(h-text_h)/2:shadowx=3:shadowy=2%p% -y %v%%o%.ico
goto :EOF


:aa
ffmpeg -i %v%1.ico -i %v%2.ico -filter_complex [0]crop=iw/2-1:ih:0:0[l];[1]crop=iw/2+1:ih:0:0[r];[l][r]hstack -y %v%0.ico
goto :EOF

:bb
ffmpeg -i %v%1.ico -i %v%2.ico -filter_complex [0]crop=iw/2-1:ih:0:0[l];[1]crop=iw/2+1:ih:iw/2-1:0[r];[l][r]hstack -y %v%0.ico
goto :EOF

:cc
ffmpeg -i %v%1.ico -i %v%2.ico -filter_complex [0]crop=iw/2:ih:0:0[l];[1]crop=iw/2:ih:iw/2:0[r];[l][r]hstack -y %v%0.ico
goto :EOF

