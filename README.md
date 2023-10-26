# EnRu
Another global keyboard layout switch by clicking the left or right `Ctrl` key<br>
Еще один глобальный переключатель раскладки клавиатуры щелчком левой или правой клавиши `Ctrl`

## Credits - благодарности:
- Rudi De Vos, Sam Liarfo, Ludovic Bocquet - for [UltraVNC](https://uvnc.com/downloads/ultravnc.html)![UltraVNC](uvnc.png)![winvnc](winvnc.png)
- Joe DF - for [AutoHotkey](https://github.com/AutoHotkey/AutoHotkey/releases)![ahk](ahk.png)
- GA, ТС - for [FFmpeg](https://ffmpeg.org/download.html)![FFmpeg](FFmpeg.png)
- Microsoft - for [VS Code](https://code.visualstudio.com/Download)![VS Code](VScode.png)

## Install - установка:
- `git clone https://github.com/abakum/EnRu.git` [![EnRu](0.ico)](https://github.com/abakum/EnRu.git)
- [download](https://github.com/AutoHotkey/AutoHotkey/releases) & install [AutoHotkey](https://github.com/AutoHotkey/AutoHotkey/releases)![ahk](ahk.png)
- run `EnRu.ahk` or `a2e.bat`

## Usage - использование:
- Press and release the left `Ctrl` key to switch the keyboard layout to `en_US`![en_US](1.ico).<br>
Нажми и отпусти левую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `en_US`![en_US](1.ico).
- Press and release the right `Ctrl` key to switch the keyboard layout to `ru_Ru`![ru_Ru](2.ico), but if the [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) window is active, the local keyboard layout will be `en_US`![en_US](1.ico).<br>
Нажми и отпусти правую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `ru_Ru`![ru_Ru](2.ico), но если активно окно с [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) то раскладка локальной клавиатуры будет `en_US`![en_US](1.ico).
- You can switch the keyboard layout with the mouse by right-clicking on the tray icon.<br>
Можно переключить раскладку клавиатуры мышкой через правый клик на иконке в трэе.
- If you make the window with [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) active then after 2 seconds or earlier, the layout of the local keyboard will switch to `en_US`![en_US](1.ico).<br>
Если сделать активным окно с [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) то через 2 секунды или раньше раскладка локальной клавиатуры переключится `en_US`![en_US](1.ico).
- If you switch the keyboard layout not via [![EnRu](0.ico)](https://github.com/abakum/EnRu.git) then after 2 seconds or earlier, the icon in the tray will change accordingly.<br>
Если переключить раскладку клавиатуры не через [![EnRu](0.ico)](https://github.com/abakum/EnRu.git) то через 2 секунды или раньше иконка в трэе изменится соответственно.

## Story of creation - история создания:

- Previously, whichever VNC-server![vnc](vnc.png) I used, I used only [RealVNC's](https://www.realvnc.com/en/connect/download/viewer/)![RealVNC](RealVNC.png) VNC-viewer. It would send `Alt-TAB` to the VNC-server and allow me to switch keyboard layouts if the same keyboard layout switcher was set on both VNC computers.<br>
Раньше какой бы VNC-сервер![vnc](vnc.png) я не использовал пользовался только VNC-вьювером от [RealVNC](https://www.realvnc.com/en/connect/download/viewer/)![RealVNC](RealVNC.png)
Он передавал VNC-серверу `Alt-TAB` и позволял переключать раскладку клавиатур когда на обеих компьютерах с VNC стоял один и тот же переключатель раскладок клавиатур.

 - But in the [ngrokVNC](http://github.com/abakum/ngrokVNC)![ngrokVNC](ngrokVNC.png) project I had to use the encryption plugin and [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html), which, when `ScrollLock` is enabled, can transmit `Alt-TAB` to the [UltrVNC-server](https://uvnc.com/docs/uvnc-server.html)![winvnc](winvnc.png), but it was necessary to have `en_US`![en_US](1.ico) layout on the [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) and `ru_Ru`![ru_Ru](2.ico) layout on the [![winvnc](winvnc.png)](https://uvnc.com/docs/uvnc-server.html) to transmit cyrillics.<br>
Но в проекте [ngrokVNC](http://github.com/abakum/ngrokVNC)![ngrokVNC](ngrokVNC.png) потребовалось использовать плагин шифрования и [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html), который при включении `ScrollLock` может передавать `Alt-TAB` на [VNC-сервер](https://uvnc.com/docs/uvnc-server.html)![winvnc](winvnc.png), но для передачи кириллицы нужно чтоб на [![UltraVNC](uvnc.png)](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html)  была расскладка `en_US`![en_US](1.ico) а на [![winvnc](winvnc.png)](https://uvnc.com/docs/uvnc-server.html) `ru_Ru`![ru_Ru](2.ico)
