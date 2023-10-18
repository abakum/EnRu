;https://github.com/abakum/EnRu.git
;Another global keyboard layout switch by clicking the left or right Ctrl key
;Еще один глобальный переключатель раскладки клавиатуры щелчком левой или правой клавиши Ctrl

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
; Item "En", 1, A_TrayMenu

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
 ToolTip ItemName
 PostMessage WM_InputLangChangeRequest,,hkl[ItemPos],, HWND_BroadCast
 ;@Ahk2Exe-IgnoreBegin
  TraySetIcon ItemPos ".ico"
 ;@Ahk2Exe-IgnoreEnd
 /*@Ahk2Exe-Keep
  TraySetIcon A_ScriptName, -ItemPos
 */
 A_IconTip:=ItemName
 SoundBeep Frequency*ItemPos
 SetTimer () => ToolTip(), Timer
}
