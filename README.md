# EnRu
Another global keyboard layout switcher by clicking the left or right Ctrl key<br>
Еще один глобальный переключатель раскладки клавиатуры щелчком левой или правой клавиши Ctrl

## Credits - благодарности:
- Rudi De Vos, Sam Liarfo, Ludovic Bocquet - for [UltraVNC](https://uvnc.com/downloads/ultravnc.html)
- Joe DF - for [AutoHotkey](https://github.com/AutoHotkey/AutoHotkey/releases)
- GA, ТС - for [ffmpeg](https://ffmpeg.org/download.html)
- Microsoft - for [VS Code](https://code.visualstudio.com/Download)

## Install - установка:
- `git clone https://github.com/abakum/EnRu.git`
- [download](https://github.com/AutoHotkey/AutoHotkey/releases) & install AutoHotkey
- run `EnRu.ahk` or `a2e.bat`

## Usage - использование:
- Press and release the left `Ctrl` key to switch the keyboard layout to `en_US`.<br>
Нажми и отпусти левую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `en_US.`
- Press and release the right `Ctrl` key to switch the keyboard layout to `ru_Ru`, but if the [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) window is active, the local keyboard layout will be `en_US`.<br>
Нажми и отпусти правую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `ru_Ru`, но если активно окно с [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) то раскладка локальной клавиатуры будет `en_US`

## Story of creation - история создания:

- Previously, whichever VNC-server I used, I used [RealVNC's](https://www.realvnc.com/en/connect/download/viewer/) VNC-viewer. It would send `Alt-TAB` to the VNC-server and allow me to switch keyboard layouts if the same keyboard layout switcher was set on both VNC computers.<br>
Раньше какой бы VNC-сервер я не использовал пользовался VNC-вьювером от [RealVNC](https://www.realvnc.com/en/connect/download/viewer/)
Он передавал VNC-серверу `Alt-TAB` и позволял переключать раскладку клавиатур когда на обеих компьютерах с VNC стоял один и тот же переключатель раскладок клавиатур.

 - But in the project [ngrokVNC](http://github.com/abakum/ngrokVNC) I had to use the encryption plugin and [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html), although it sent `Alt-TAB` to the VNC-server when `ScrollLock` was turned on, but it was necessary to have `en_US` layout on the VNC-viewer and `ru_Ru` layout on the VNC-server to transmit Cyrillic characters.<br>
Но в проекте [ngrokVNC](http://github.com/abakum/ngrokVNC) потребовалось использовать плагин шифрования а [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html), хоть при включении `ScrollLock` и передавал `Alt-TAB` на VNC-сервер, но для передачи кириллицы нужно было чтоб на VNC-вьювере была расскладка `en_US` а на VNC-сервере `ru_Ru`