//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	vkShift      = 0x10
	vkControl    = 0x11
	vkMenu       = 0x12
	vkLeftWin    = 0x5B
	vkRightWin   = 0x5C
	vkLeftCtrl   = 0xA2
	vkRightCtrl  = 0xA3

	inputKeyboard = 1
	keyEventKeyUp = 2
	injected      = 0x10
	inputTag      = 0x43314332
	gaRootOwner   = 3
	debounce      = 150 * time.Millisecond
	beepDuration  = 100 * time.Millisecond
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	setWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	callNextHookEx      = user32.NewProc("CallNextHookEx")
	unhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	getMessage          = user32.NewProc("GetMessageW")
	getForegroundWindow = user32.NewProc("GetForegroundWindow")
	getClassName        = user32.NewProc("GetClassNameW")
	getAncestor         = user32.NewProc("GetAncestor")
	findWindow          = user32.NewProc("FindWindowW")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	getAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	sendInput           = user32.NewProc("SendInput")
	beep                = windows.NewLazySystemDLL("kernel32.dll").NewProc("Beep")

	ctrlHeld       uint8
	standaloneCtrl uint32
	lastCtrl       uint32
	lastSwitch     time.Time
	requests       = make(chan switchRequest, 8)
	diagnostics    chan string
)

type hookEvent struct {
	VK        uint32
	_         uint32 // scan code
	Flags     uint32
	_         uint32 // time
	ExtraInfo uintptr
}

// Win32 MSG storage for GetMessageW; only its return value is used.
type message struct {
	_ uintptr
	_ uint32
	_ [2]uintptr
	_ uint32
	_ [2]int32
	_ uint32
}

type keyboardInput struct {
	Type uint32
	Key  struct {
		VK        uint16
		_         uint16 // scan code
		Flags     uint32
		_         uint32 // time
		ExtraInfo uintptr
	}
	_ [8]byte // INPUT's union is sized for MOUSEINPUT, not KEYBDINPUT.
}

type switchRequest struct {
	window uintptr
	digit  uint16
}

func windowClass(window uintptr) string {
	var class [32]uint16
	getClassName.Call(window, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	return windows.UTF16ToString(class[:])
}

func foregroundTarget() uintptr {
	window, _, _ := getForegroundWindow.Call()
	if window != 0 && windowClass(window) == "tooltips_class32" {
		// Releasing Ctrl can dismiss the tooltip and restore its owner's foreground window.
		if owner, _, _ := getAncestor.Call(window, gaRootOwner); owner != 0 {
			return owner
		}
	}
	return window
}

func isTaskbar(class string) bool {
	return class == "Shell_TrayWnd" || class == "Shell_SecondaryTrayWnd" || class == "tooltips_class32"
}

func activateDesktop(taskbar uintptr) bool {
	class, _ := windows.UTF16PtrFromString("Progman")
	desktop, _, _ := findWindow.Call(uintptr(unsafe.Pointer(class)), 0)
	if desktop == 0 || foregroundTarget() != taskbar {
		return false
	}
	if ok, _, _ := setForegroundWindow.Call(desktop); ok == 0 {
		return false
	}
	deadline := time.Now().Add(40 * time.Millisecond)
	for {
		foreground, _, _ := getForegroundWindow.Call()
		if foreground == desktop {
			return true
		}
		if (foreground != 0 && foregroundTarget() != taskbar) || !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

func traceHook(message string) {
	// Never write to the console or wait for the worker from the low-level hook.
	select {
	case diagnostics <- message:
	default:
	}
}

func keyboardHook(code int, wParam, lParam uintptr) uintptr {
	result, _, _ := callNextHookEx.Call(0, uintptr(code), wParam, lParam)
	if code != 0 {
		return result
	}
	event := (*hookEvent)(unsafe.Pointer(lParam))
	if event.Flags&injected != 0 && event.ExtraInfo == inputTag {
		return result
	}

	var bit uint8
	switch event.VK {
	case vkLeftCtrl:
		bit = 1
	case vkRightCtrl:
		bit = 2
	}
	switch wParam {
	case wmKeyDown, wmSysKeyDown:
		if bit == 0 {
			standaloneCtrl = 0
		} else if ctrlHeld&bit == 0 {
			standaloneCtrl = 0
			if ctrlHeld == 0 && wParam == wmKeyDown {
				standaloneCtrl = event.VK
			}
			ctrlHeld |= bit
		}
	case wmKeyUp, wmSysKeyUp:
		if bit == 0 {
			return result
		}
		ctrlHeld &^= bit
		standalone := standaloneCtrl == event.VK
		standaloneCtrl = 0
		if wParam == wmSysKeyUp {
			traceHook("Skipped: Ctrl was released as a system key (WM_SYSKEYUP).")
		} else if !standalone {
			traceHook("Skipped: Ctrl release was not recognized as a standalone tap.")
		}
		if standalone && wParam == wmKeyUp {
			now := time.Now()
			if lastCtrl == event.VK && now.Sub(lastSwitch) < debounce {
				traceHook("Skipped: repeated Ctrl tap within the debounce interval.")
				return result
			}
			window := foregroundTarget()
			if window != 0 {
				digit := uint16('1')
				if bit == 2 {
					digit = '2'
				}
				select {
				case requests <- switchRequest{window, digit}:
					lastCtrl, lastSwitch = event.VK, now
				default:
					traceHook("Skipped: switch request queue is full.")
				}
			} else {
				traceHook("Skipped: no foreground window at Ctrl release.")
			}
		}
	}
	return result
}

func switchLayout(req switchRequest, console bool) {
	if console {
		fmt.Printf("Standalone Ctrl tap: Ctrl+%c requested for %q (0x%X).\n", req.digit, windowClass(req.window), req.window)
	}
	var input [4]keyboardInput
	for i, key := range [...]uint16{vkLeftCtrl, req.digit, req.digit, vkLeftCtrl} {
		input[i].Type = inputKeyboard
		input[i].Key.VK = key
		input[i].Key.ExtraInfo = inputTag
		if i >= 2 {
			input[i].Key.Flags = keyEventKeyUp
		}
	}
	// Windows updates the async key state after the low-level hook returns.
	deadline := time.Now().Add(20 * time.Millisecond)
	for {
		if state, _, _ := getAsyncKeyState.Call(vkControl); state&0x8000 == 0 {
			break
		}
		if !time.Now().Before(deadline) {
			if console {
				fmt.Println("Skipped: Windows still reports Ctrl held after 20 ms.")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	for _, key := range [...]uintptr{vkShift, vkMenu, vkLeftWin, vkRightWin, uintptr(req.digit)} {
		if state, _, _ := getAsyncKeyState.Call(key); state&0x8000 != 0 {
			if console {
				fmt.Printf("Skipped: modifier or shortcut digit is held (virtual key 0x%X).\n", key)
			}
			return
		}
	}
	if foreground := foregroundTarget(); foreground != req.window {
		if console {
			fmt.Printf("Skipped: foreground changed to %q (0x%X) before sending.\n", windowClass(foreground), foreground)
		}
		return
	}
	if isTaskbar(windowClass(req.window)) && !activateDesktop(req.window) {
		fmt.Println("Skipped layout change: could not activate the Progman desktop from the taskbar.")
		return
	}
	sent, _, err := sendInput.Call(uintptr(len(input)), uintptr(unsafe.Pointer(&input[0])), unsafe.Sizeof(input[0]))
	if sent != uintptr(len(input)) {
		fmt.Fprintf(os.Stderr, "SendInput Ctrl+%c (%d/%d): %v\n", req.digit, sent, len(input), err)
		return
	}
	fmt.Printf("Sent Ctrl+%c\n", req.digit)
	freq := uintptr(1046)
	if req.digit == '2' {
		freq = 523
	}
	go beep.Call(freq, uintptr(beepDuration.Milliseconds()))
}

func runSwitcher(console bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if console {
		diagnostics = make(chan string, 16)
	}
	hook, _, err := setWindowsHookEx.Call(whKeyboardLL, windows.NewCallback(keyboardHook), 0, 0)
	if hook == 0 {
		return fmt.Errorf("SetWindowsHookEx: %w", err)
	}
	defer unhookWindowsHookEx.Call(hook)
	go func() {
		for {
			select {
			case req := <-requests:
				switchLayout(req, console)
			case message := <-diagnostics:
				fmt.Println(message)
			}
		}
	}()

	if console {
		printSetup()
		fmt.Println("Running in console mode. Press Ctrl+C to exit.")
	}
	var msg message
	for {
		ret, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return fmt.Errorf("GetMessage: %w", err)
		case 0:
			return nil
		}
	}
}
