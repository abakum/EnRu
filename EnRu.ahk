/*
git clone https://github.com/abakum/EnRu.git

Another global keyboard layout switcher by clicking the left or right Ctrl key<br>
Еще один глобальный переключатель раскладки клавиатуры щелчком левой или правой клавиши Ctrl

- Press and release the left `Ctrl` key to switch the keyboard layout to `en_US`.<br>
Нажми и отпусти левую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `en_US.`
- Press and release the right `Ctrl` key to switch the keyboard layout to `ru_Ru`, but if the [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) window is active, the local keyboard layout will be `en_US`.<br>
Нажми и отпусти правую клавишу `Ctrl` чтоб переключить раскладку клавиатуры на `ru_Ru`, но если активно окно с [UltraVNC\vncviewer](https://uvnc.com/docs/uvnc-viewer/71-viewer-gui.html) то раскладка локальной клавиатуры будет `en_US`
git push origin --tags
*/
;@Ahk2Exe-Let ProductVersion=v0.6.3-lw

#Requires AutoHotkey v2.0
#SingleInstance
;@Ahk2Exe-SetMainIcon 0.ico
;@Ahk2Exe-AddResource 1.ico, 160
;@Ahk2Exe-AddResource 2.ico, 161
;@Ahk2Exe-SetName EnRu
;@Ahk2Exe-SetCopyright Abakum
;@Ahk2Exe-SetProductVersion %U_ProductVersion%
;@Ahk2Exe-SetDescription Changing the input language by clicking the left or right `Ctrl`
; @Ahk2Exe-SetLanguage 0x0419
; @Ahk2Exe-SetDescription Смена языка ввода по клику левого или правого `Ctrl`
;@Ahk2Exe-PostExec "%A_ComSpec%" /c "cd /d %A_ScriptDir%&git tag %U_ProductVersion%"
;@Ahk2Exe-Cont  , , , , 1

;https://learn.microsoft.com/en-us/windows/win32/winmsg/wm-inputlangchangerequest
WM_INPUTLANGCHANGEREQUEST:=0x0050
;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getwindow
GW_HWNDPREV:=3
;"c:\Program Files\uvnc bvba\UltraVNC\vncviewer.exe" 
uvnc:="VNCMDI_Window"


Frequency:=523
Period:=2 ;set negative to stop polling
lastHKL:=0

EnRu:=[] ;["us","ru"]
KLID:=[] ;["00000409", "00000419"]
HKL:=[] ;[0x04090409, 0x04190419]
Loop Reg, "HKCU\Keyboard Layout\Preload"{
 KLID.Push RegRead("HKCU\Keyboard Layout\Preload", A_Index)
 EnRu.Push RegRead("HKLM\SYSTEM\CurrentControlSet\Control\Keyboard Layout\DosKeybCodes" , KLID[A_Index])
 HKL.Push DllCall("LoadKeyboardLayout", "Str", KLID[A_Index], "UInt", 0)
}

TrayIcon
SetTimer () => TrayIcon(), Abs(Period)*1000

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

;switch lang by index ItemPos
Lang(ItemPos){
 Item EnRu[ItemPos], ItemPos, A_TrayMenu
}

;switch lang by menu
;set lastHKL as HKL[ItemPos]
Item(ItemName, ItemPos, MyMenu) {
 hwnd:=DllCall("GetForegroundWindow", "Ptr")
 if hwnd=0
  return
 class:=WinGetClass(hwnd)

 if class==uvnc{
  SoundBeep Frequency*ItemPos
  ItemPos:=1
 }
 else
  ToolTip ItemName
  
 loop{
  ;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-postmessagea
  try{
   PostMessage WM_INPUTLANGCHANGEREQUEST, , HKL[ItemPos], , hwnd
   sleep 7
  ;  SendMessage WM_INPUTLANGCHANGEREQUEST, , HKL[ItemPos], , hwnd
  } catch {
    ToolTip "PostMessage WM_INPUTLANGCHANGEREQUEST, , " HKL[ItemPos] ", , " hwnd
  }
  if GetKeyboardLayout()=HKL[ItemPos]{
   global lastHKL:=HKL[ItemPos]
   icon ItemPos
   return
  }
  hwnd:=DllCall("GetWindow", "Ptr", hwnd, "UInt", GW_HWNDPREV, "Ptr")
  if hwnd=0
    return
  if class=="Shell_TrayWnd"{
   DllCall("SetForegroundWindow", "Ptr", hwnd)
   class:=WinGetClass(hwnd)
  }
 }
}

;get lang as HKL
GetKeyboardLayout() {
 ;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getforegroundwindow
 hwnd:=DllCall("GetForegroundWindow", "Ptr")
 if hwnd && WinGetClass(hwnd)=="ConsoleWindowClass"
  hwnd:=DllCall("GetWindow", "Ptr", hwnd, "UInt", GW_HWNDPREV, "Ptr")
 ;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getwindowthreadprocessid
 if hwnd=0
  tid:=0
 else
  tid:=DllCall("GetWindowThreadProcessId", "Ptr", hwnd, "Ptr", 0, "Ptr")
 ;https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getkeyboardlayout
 return DllCall("GetKeyboardLayout", "UInt", tid, "UInt")
}

;change icon in tray by ItemPos
icon(ItemPos){
  if ItemPos=0
    A_IconTip:=""
  else{
   A_IconTip:=EnRu[ItemPos]
   if !WinActive("ahk_class " uvnc)
    SoundBeep Frequency*ItemPos
  }
 ;@Ahk2Exe-IgnoreBegin
  TraySetIcon ItemPos ".ico"
 ;@Ahk2Exe-IgnoreEnd
 /*@Ahk2Exe-Keep
  TraySetIcon A_ScriptName, -(159+ItemPos)
 */
}

;hide ToolTip
;change lang to US in window of UltraVNC-viewer
;change icon in tray accordingly GetKeyboardLayout 
;set lastHKL as GetKeyboardLayout
TrayIcon(){
 ToolTip
 if Period<0 && lastHKL>0{
  if lastHKL!=HKL[1] && WinActive("ahk_class " uvnc)
   Lang 1
  return
 }
 
 curHKL:=GetKeyboardLayout()
 if curHKL!=HKL[1] && WinActive("ahk_class " uvnc){ 
  Lang 1
  return
 }
 if lastHKL=curHKL
  return
 global lastHKL:=curHKL
 For i, v in HKL{
  if v=curHKL{
   icon i
   return
  }
 }
 icon 0
}
