/*
git clone https://github.com/abakum/EnRu.git

Another global keyboard layout switcher by clicking the left or right Ctrl key<br>
Еще один глобальный переключатель раскладки клавиатуры щелчком левой или правой клавиши Ctrl

- Press and release the left `Ctrl` key to switch the keyboard layout to `en_US`.<br>
Нажми и отпусти левую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `en_US.`
- Press and release the right `Ctrl` key to switch the keyboard layout to `ru_Ru`, but if the [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) window is active, the local keyboard layout will be `en_US`.<br>
Нажми и отпусти правую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `ru_Ru`, но если активно окно с [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) то раскладка локальной клавиатуры будет `en_US`

// git tag v0.1.1-lw
// git push origin --tags
*/

#Requires AutoHotkey v2.0
#SingleInstance
;@Ahk2Exe-AddResource 1.ico, 1
;@Ahk2Exe-AddResource 2.ico, 2
;@Ahk2Exe-SetMainIcon EnRu.ico

hkl:=[0x4090409, 0x4190419]
WM_InputLangChangeRequest:=0x0050
HWND_BroadCast:=0xFFFF
Frequency:=523
Timer:=-500
Item "En", 1, A_TrayMenu

~LControl up::{
 if "LControl"=A_PriorKey
  Item "En", 1, A_TrayMenu
}

~RControl up::{
 if "RControl"=A_PriorKey
  Item "Ru", 2, A_TrayMenu
}

A_TrayMenu.Insert "1&"
A_TrayMenu.Insert "1&", "Ru", Item
A_TrayMenu.Insert "1&", "En", Item

Item(ItemName, ItemPos, MyMenu) {
 sb:=Frequency*ItemPos
 ;"c:\Program Files\uvnc bvba\UltraVNC\vncviewer.exe" 
 if WinActive("ahk_class VNCMDI_Window") { 
  ItemName:="En"
  ItemPos:=1
 }
 ToolTip ItemName
 PostMessage WM_InputLangChangeRequest,,hkl[ItemPos],, HWND_BroadCast
 ;@Ahk2Exe-IgnoreBegin
  TraySetIcon ItemPos ".ico"
 ;@Ahk2Exe-IgnoreEnd
 /*@Ahk2Exe-Keep
  TraySetIcon A_ScriptName, -ItemPos
 */
 A_IconTip:=ItemName
 SoundBeep sb
 SetTimer () => ToolTip(), Timer
}
