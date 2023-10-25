/*
git clone https://github.com/abakum/EnRu.git

Another global keyboard layout switcher by clicking the left or right Ctrl key<br>
Еще один глобальный переключатель раскладки клавиатуры щелчком левой или правой клавиши Ctrl

- Press and release the left `Ctrl` key to switch the keyboard layout to `en_US`.<br>
Нажми и отпусти левую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `en_US.`
- Press and release the right `Ctrl` key to switch the keyboard layout to `ru_Ru`, but if the [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) window is active, the local keyboard layout will be `en_US`.<br>
Нажми и отпусти правую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `ru_Ru`, но если активно окно с [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) то раскладка локальной клавиатуры будет `en_US`

git tag v0.2.1-lw
git push origin --tags
*/

#Requires AutoHotkey v2.0
#SingleInstance
;@Ahk2Exe-SetMainIcon EnRu.ico
;@Ahk2Exe-AddResource 1.ico, 160
;@Ahk2Exe-AddResource 2.ico, 161
;@Ahk2Exe-SetName EnRu
;@Ahk2Exe-SetCopyright Abakum
;@Ahk2Exe-SetProductVersion v0.1.4-lw
;@Ahk2Exe-SetDescription Changing the input language by clicking the left or right `Ctrl`
; @Ahk2Exe-SetLanguage 0x0419
; @Ahk2Exe-SetDescription Смена языка ввода по клику левого или правого `Ctrl`

;https://learn.microsoft.com/en-us/windows/win32/winmsg/wm-inputlangchangerequest
WM_INPUTLANGCHANGEREQUEST:=0x0050
KLID:=["00000409", "00000419"]
EnRu:=["En","Ru"]
HKL:=[0x04090409, 0x04190419]
;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-loadkeyboardlayouta
KLF_ACTIVATE:=1
;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-postmessagea
HWND_BROADCAST:=0xFFFF
Frequency:=523
Period:=2000
lastHKL:=0

TrayIcon
SetTimer () => TrayIcon(), Period

~LControl up::{
 if "LControl"=A_PriorKey
  Lang 1
}

~RControl up::{
 if "RControl"=A_PriorKey
  Lang 2
}

A_TrayMenu.Insert "1&"
A_TrayMenu.Insert "1&", EnRu[2], Item
A_TrayMenu.Insert "1&", EnRu[1], Item

Lang(ItemPos){
 Item EnRu[ItemPos], ItemPos, A_TrayMenu
}

Item(ItemName, ItemPos, MyMenu) {
 sb:=Frequency*ItemPos
 ;"c:\Program Files\uvnc bvba\UltraVNC\vncviewer.exe" 
 if WinActive("ahk_class VNCMDI_Window") { 
  ItemPos:=1
 } else {
  ToolTip ItemName
 }
 lastHKL:= DllCall("LoadKeyboardLayout", "Str", KLID[ItemPos], "uint", KLF_ACTIVATE)
 PostMessage WM_INPUTLANGCHANGEREQUEST, , lastHKL, , HWND_BROADCAST
 ico ItemPos
 SoundBeep sb
}

GetCurrentKeyboardLayout() {
 hwnd:=DllCall("GetForegroundWindow")
 if hwnd==0
  return 0
 return DllCall("GetKeyboardLayout", "UInt", DllCall("GetWindowThreadProcessId", "UInt", hwnd, "UInt", 0), "UInt")
}

ico(ItemPos){
 A_IconTip:=EnRu[ItemPos]
 ;@Ahk2Exe-IgnoreBegin
  TraySetIcon ItemPos ".ico"
 ;@Ahk2Exe-IgnoreEnd
 /*@Ahk2Exe-Keep
  TraySetIcon A_ScriptName, -(159+ItemPos)
 */
}

TrayIcon(){
 ToolTip
 curHKL:=GetCurrentKeyboardLayout()
 if lastHKL==curHKL
  return
 global lastHKL:=curHKL
 For i, v in HKL{
  if v==curHKL{
   ico i
   return
  }
 }
}