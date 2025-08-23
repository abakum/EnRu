package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	WH_KEYBOARD_LL            = 13
	WM_KEYUP                  = 0x0101
	WM_INPUTLANGCHANGEREQUEST = 0x0050
	VK_LCONTROL               = 0xA2
	VK_RCONTROL               = 0xA3
	GW_HWNDPREV               = 3
)

var (
	user32                       = windows.NewLazyDLL("user32.dll")
	procSetWindowsHookEx         = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx           = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx      = user32.NewProc("UnhookWindowsHookEx")
	procPostMessage              = user32.NewProc("PostMessageW")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetMessage               = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessage          = user32.NewProc("DispatchMessageW")
	procLoadKeyboardLayout       = user32.NewProc("LoadKeyboardLayoutW")
	procGetWindow                = user32.NewProc("GetWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetClassName             = user32.NewProc("GetClassNameW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")

	hook uintptr
)

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type MSG struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type POINT struct {
	X, Y int32
}

func setHook() error {
	callback := windows.NewCallback(keyboardHook)
	r, _, err := procSetWindowsHookEx.Call(
		WH_KEYBOARD_LL,
		callback,
		0,
		0,
	)

	if r == 0 {
		return fmt.Errorf("SetWindowsHookEx failed: %v", err)
	}

	hook = r
	return nil
}

func unhook() {
	if hook != 0 {
		procUnhookWindowsHookEx.Call(hook)
	}
}

func messageLoop() {
	var msg MSG
	for {
		// Блокирующее ожидание сообщений
		ret, _, _ := procGetMessage.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)

		if ret == 0 { // WM_QUIT
			break
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func keyboardHook(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && wParam == WM_KEYUP {
		kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))

		switch kb.VkCode {
		case VK_LCONTROL:
			switchLanguage("00000409") // English
		case VK_RCONTROL:
			switchLanguage("00000419") // Russian
		}
	}

	// Всегда передаем событие дальше
	result, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return result
}

func switchLanguage(layoutID string) {
	// Загружаем раскладку
	layoutPtr, _ := windows.UTF16PtrFromString(layoutID)
	hkl, _, _ := procLoadKeyboardLayout.Call(
		uintptr(unsafe.Pointer(layoutPtr)),
		0,
	)

	if hkl == 0 {
		return
	}

	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return
	}

	class := getWindowClass(hwnd)

	// Итерируем через цепочку окон
	for hwnd != 0 {
		if getKeyboardLayout() == hkl {
			return
		}
		// Отправляем сообщение
		procPostMessage.Call(
			hwnd,
			WM_INPUTLANGCHANGEREQUEST,
			0,
			hkl,
		)

		time.Sleep(7 * time.Millisecond)

		// Проверяем успешность
		if getKeyboardLayout() == hkl {
			// fmt.Printf("Successfully switched to %s\n", layoutID)
			return
		}

		// Переходим к предыдущему окну
		hwnd, _, _ = procGetWindow.Call(hwnd, GW_HWNDPREV)

		// Для панели задач активируем следующее окно
		if class == "Shell_TrayWnd" {
			procSetForegroundWindow.Call(hwnd)
			class = getWindowClass(hwnd)
		}
	}

	// fmt.Printf("Failed to switch to %s\n", layoutID)
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := setupTaskScheduler(); err != nil {
				fmt.Printf("Install failed: %v\n", err)
			}
		case "uninstall":
			if err := removeTaskScheduler(); err != nil {
				fmt.Printf("Uninstall failed: %v\n", err)
			}
		case "start":
			if err := startTask(); err != nil {
				fmt.Printf("Start failed: %v\n", err)
			}
		case "stop":
			if err := stopTask(); err != nil {
				fmt.Printf("Stop failed: %v\n", err)
			}
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
		}
		return
	}

	// Автозагрузка через планировщик задач
	fmt.Println("EnRu Switcher starting in console mode...")
	fmt.Println("Left Ctrl -> English")
	fmt.Println("Right Ctrl -> Russian")

	if err := setHook(); err != nil {
		log.Printf("Hook failed: %v", err)
		return
	}
	defer unhook()

	messageLoop()
}

func setupTaskScheduler() error {
	exePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return err
	}

	// Удаляем существующую задачу
	exec.Command("schtasks", "/Delete", "/TN", "EnRuSwitcher", "/F").Run()

	// Создаем новую задачу
	cmd := exec.Command("schtasks", "/Create", "/TN", "EnRuSwitcher",
		"/TR", fmt.Sprintf(`"%s"`, exePath),
		"/SC", "ONLOGON",
		"/IT",
		"/F")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks failed: %v, output: %s", err, string(output))
	}

	fmt.Println("Task scheduler setup completed successfully")
	return nil
}

func removeTaskScheduler() error {
	cmd := exec.Command("schtasks", "/Delete", "/TN", "EnRuSwitcher", "/F")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("schtasks delete failed: %v, output: %s", err, string(output))
	}
	fmt.Println("Task scheduler removed successfully")
	return nil
}

func startTask() error {
	cmd := exec.Command("schtasks", "/Run", "/TN", "EnRuSwitcher")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks run failed: %v, output: %s", err, string(output))
	}
	fmt.Println("Task started successfully")
	return nil
}

func stopTask() error {
	// Находим процесс и завершаем его
	cmd := exec.Command("taskkill", "/IM", "EnRuSwitcher.exe", "/F")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("taskkill failed: %v, output: %s", err, string(output))
	}
	fmt.Println("Process stopped successfully")
	return nil
}

func getWindowClass(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}

	className := make([]uint16, 256)

	length, _, _ := procGetClassName.Call(
		hwnd,
		uintptr(unsafe.Pointer(&className[0])),
		uintptr(len(className)),
	)

	if length > 0 {
		return windows.UTF16ToString(className)
	}
	return ""
}

func getKeyboardLayout() uintptr {
	// Получаем активное окно
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return 0
	}

	// Для консольных окон получаем предыдущее окно
	class := getWindowClass(hwnd)
	if class == "ConsoleWindowClass" {
		hwnd, _, _ = procGetWindow.Call(hwnd, 3) // GW_HWNDPREV
	}

	// Получаем thread и layout
	var tid uintptr
	if hwnd == 0 {
		tid = 0
	} else {
		tid, _, _ = procGetWindowThreadProcessId.Call(hwnd, 0)
	}

	layout, _, _ := procGetKeyboardLayout.Call(tid)
	return layout
}
