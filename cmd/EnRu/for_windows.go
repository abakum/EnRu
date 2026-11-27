//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/jxeng/shortcut"
	"golang.org/x/sys/windows"
)

const (
	WH_KEYBOARD_LL            = 13
	WM_INPUTLANGCHANGEREQUEST = 0x0050
	GW_HWNDPREV               = 3
	wordInputTag              = 0x456E5275
	llkhfInjected             = 0x10
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx         = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx           = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx      = user32.NewProc("UnhookWindowsHookEx")
	procPostMessage              = user32.NewProc("PostMessageW")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetMessage               = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessage          = user32.NewProc("DispatchMessageW")
	procGetWindow                = user32.NewProc("GetWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetClassName             = user32.NewProc("GetClassNameW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	procSendInput                = user32.NewProc("SendInput")
	procGetAsyncKeyState         = user32.NewProc("GetAsyncKeyState")

	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procBeep = kernel32.NewProc("Beep")

	hook        uintptr
	detector    ctrlDetector
	switchTo    chan int
	hookTrace   chan ctrlTrace
	vncTestMode string // Set only in the two diagnostic builds via -ldflags -X.
)

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type ctrlTrace struct {
	message string
	event   KBDLLHOOKSTRUCT
	wm      uintptr
}

type MSG struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type POINT struct {
	X, Y int32
}

type keyboardInput struct {
	Type uint32
	Key  struct {
		VK, Scan    uint16
		Flags, Time uint32
		ExtraInfo   uintptr
	}
	_ [8]byte // The Win32 INPUT union is sized for MOUSEINPUT.
}

// Abs возвращает абсолютный путь, приводя букву диска к нижнему регистру (только для Windows).
func Abs(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Если путь начинается с буквы диска (например, "C:\"), делаем её строчной.
	if len(absPath) > 1 && absPath[1] == ':' {
		absPath = strings.ToLower(absPath[:1]) + absPath[1:]
	}

	return absPath, nil
}

func resolveExe() string {
	exe, err := os.Executable()
	if err == nil {
		// Как в маке
		exe, err = filepath.EvalSymlinks(exe)
	}

	if err != nil {
		if lp, err := exec.LookPath(os.Args[0]); err == nil {
			return lp
		} else if abs, err := Abs(os.Args[0]); err == nil {
			return abs
		}
		return ""
	}
	return exe
}

func printVersionInfo() {
	s := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		s = "Собрано " + info.GoVersion
	}
	majorVersion, minorVersion, buildNumber := windows.RtlGetNtVersionNumbers()
	fmt.Println(Description, exe, VERSION, s, fmt.Sprintf("%d.%d.%d on %s", majorVersion, minorVersion, buildNumber, runtime.GOARCH))
}

func playBeep(freq uint) {
	procBeep.Call(uintptr(freq), uintptr(BeepDuration.Milliseconds()))
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
		return fmt.Errorf("SetWindowsHookEx WH_KEYBOARD_LL: %v", err)
	}

	hook = r
	return nil
}

func unhook() {
	if hook != 0 {
		procUnhookWindowsHookEx.Call(hook)
	}
}

func messageLoop() error {
	var msg MSG
	for {
		// Блокирующее ожидание сообщений
		ret, _, err := procGetMessage.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)

		switch int32(ret) {
		case 0:
			return nil
		case -1:
			return fmt.Errorf("GetMessageW: %w", err)
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func keyboardHook(nCode int, wParam uintptr, lParam uintptr) uintptr {
	result, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	if nCode != 0 {
		return result
	}
	kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
	if hookTrace != nil && (kb.VkCode == vkLeftCtrl || kb.VkCode == vkRightCtrl || kb.VkCode == 0x11) {
		queueCtrlTrace(ctrlTrace{event: *kb, wm: wParam})
	}
	if kb.Flags&llkhfInjected != 0 && kb.DwExtraInfo == wordInputTag {
		if kb.VkCode == vkLeftCtrl || kb.VkCode == vkRightCtrl {
			traceCtrl("Ctrl: собственное событие SendInput пропущено")
		}
		return result
	}
	index, trace := detector.event(wParam, kb.VkCode, time.Now(), vncTestMode == "release")
	traceCtrl(trace)
	if index >= 0 {
		select {
		case switchTo <- index:
			traceCtrl("Ctrl: запрос переключения принят")
		default:
			traceCtrl("Ctrl: очередь переключений заполнена")
		}
	}
	return result
}

func traceCtrl(message string) {
	if message != "" {
		queueCtrlTrace(ctrlTrace{message: message})
	}
}

func queueCtrlTrace(entry ctrlTrace) {
	select {
	case hookTrace <- entry:
	default:
	}
}

func switchLanguage(hkl uintptr, layoutID string, digit uint16) {
	hwnd, _, err := procGetForegroundWindow.Call()
	if hwnd == 0 {
		fmt.Printf("GetForegroundWindow: %v\n", err)
		return
	}

	class := getWindowClass(hwnd)
	if class == "OpusApp" {
		switchWordLayout(hwnd, hkl, digit)
		return
	}
	// Итерируем через цепочку окон
	for hwnd != 0 {
		if getWindowClass(hwnd) == "OpusApp" {
			return
		}

		procPostMessage.Call(
			hwnd,
			WM_INPUTLANGCHANGEREQUEST,
			0,
			hkl,
		)

		time.Sleep(7 * time.Millisecond)

		// Проверяем успешность
		if getKeyboardLayout(hwnd) == hkl {
			fmt.Printf("Переключили %s на: %s\n", class, layoutID)
			return
		}
		// Переходим к предыдущему окну
		hwnd, _, _ = procGetWindow.Call(hwnd, GW_HWNDPREV)

		// Для панели задач активируем следующее окно
		if class == "Shell_TrayWnd" && hwnd != 0 {
			procSetForegroundWindow.Call(hwnd)
			class = getWindowClass(hwnd)
		}
	}

	fmt.Printf("Не удалось переключиться на: %s\n", layoutID)
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

// Функция для получения раскладки конкретного окна
func getKeyboardLayout(hwnd uintptr) uintptr {
	if hwnd == 0 {
		return 0
	}

	// Для консольных окон получаем предыдущее окно
	class := getWindowClass(hwnd)
	if class == "ConsoleWindowClass" {
		hwnd, _, _ = procGetWindow.Call(hwnd, GW_HWNDPREV)
	}

	tid, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	if tid == 0 {
		return 0
	}

	layout, _, _ := procGetKeyboardLayout.Call(tid)
	return layout
}

func switchWordLayout(word, hkl uintptr, digit uint16) {
	// Do not send an editing shortcut to Word when the system binding is missing or stale.
	matched := false
	if immGetHotkey.Find() == nil {
		for slot := uintptr(firstDirect); slot <= lastDirect; slot++ {
			var modifiers, key uint32
			var target uintptr
			ok, _, _ := immGetHotkey.Call(slot, uintptr(unsafe.Pointer(&modifiers)), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&target)))
			if ok != 0 && key == uint32(digit) && modifiers == ctrlDigits && target == hkl {
				matched = true
				break
			}
		}
	}
	if !matched {
		fmt.Printf("Word: Ctrl+%c не назначено на нужную раскладку; выполните EnRu install\n", digit)
		return
	}
	for _, key := range [...]uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C, uintptr(digit)} { // Shift, Ctrl, Alt, Win, digit.
		if state, _, _ := procGetAsyncKeyState.Call(key); state&0x8000 != 0 {
			fmt.Printf("Word: Ctrl+%c не отправлено, удерживается клавиша 0x%X\n", digit, key)
			return
		}
	}
	var input [4]keyboardInput
	ctrl := uint16(vkLeftCtrl)
	if digit == '2' {
		ctrl = vkRightCtrl
	}
	for i, key := range [...]uint16{ctrl, digit, digit, ctrl} {
		input[i].Type = 1 // INPUT_KEYBOARD
		input[i].Key.VK = key
		input[i].Key.ExtraInfo = wordInputTag
		if i >= 2 {
			input[i].Key.Flags = 2 // KEYEVENTF_KEYUP
		}
		if key == vkRightCtrl {
			input[i].Key.Flags |= 1 // KEYEVENTF_EXTENDEDKEY
		}
	}
	if foreground, _, _ := procGetForegroundWindow.Call(); foreground != word {
		fmt.Println("Word: активное окно изменилось, сочетание не отправлено")
		return
	}
	sent, _, err := procSendInput.Call(uintptr(len(input)), uintptr(unsafe.Pointer(&input[0])), unsafe.Sizeof(input[0]))
	if sent != uintptr(len(input)) {
		fmt.Printf("Word: SendInput Ctrl+%c (%d/%d): %v\n", digit, sent, len(input), err)
		return
	}
	fmt.Printf("Word: отправлено системное сочетание Ctrl+%c\n", digit)
}

func installAutostart() error {
	// Получаем путь к папке автозагрузки
	startupDir, err := getStartupDir()
	if err != nil {
		return err
	}

	// Создаем ярлык
	shortcutPath := filepath.Join(startupDir, "EnRu.lnk")

	sc := shortcut.Shortcut{
		ShortcutPath: shortcutPath,
		Target:       exe,
		Arguments:    "--startup",
		Description:  Description,
		IconLocation: exe,
		WindowStyle:  "7",
	}

	err = shortcut.Create(sc)
	if err != nil {
		return fmt.Errorf("не удалось создать ярлык: %v", err)
	}

	fmt.Printf("Ярлык добавлен в папку автозапуска: %s\n", shortcutPath)
	return nil
}

func removeAutostart() error {
	startupDir, err := getStartupDir()
	if err != nil {
		return err
	}

	shortcutPath := filepath.Join(startupDir, "EnRu.lnk")
	err = os.Remove(shortcutPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("не удалось убрать ярлык из автозагрузки: %v", err)
	}

	fmt.Printf("Ярлык убран из: %s\n", shortcutPath)
	return nil
}

func getStartupDir() (string, error) {
	// Для текущего пользователя
	dir := os.Getenv("APPDATA")
	if dir == "" {
		return "", fmt.Errorf("не найдена переменная среды APPDATA")
	}
	return filepath.Join(dir, "Microsoft", "Windows", "Start Menu", "Programs", "Startup"), nil
}

func background(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:    windows.CREATE_NO_WINDOW,
		NoInheritHandles: true,
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
}

func startBackground(pair layoutPair) error {
	cmd := exec.Command(exe, "--background", pair[0].tip, fmt.Sprintf("%x", pair[0].hkl), pair[1].tip, fmt.Sprintf("%x", pair[1].hkl))
	background(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ошибка фонового запуска: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func startConsole(pair layoutPair, console bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switchTo = make(chan int, 8)
	if console && vncTestMode != "" {
		hookTrace = make(chan ctrlTrace, 64)
		fmt.Printf("Диагностика VNC: %s\n", vncTestMode)
	}
	if err := setHook(); err != nil {
		return err
	}
	defer unhook()
	if hookTrace != nil {
		go func() {
			for entry := range hookTrace {
				if entry.message != "" {
					fmt.Println(entry.message)
					continue
				}
				e := entry.event
				fmt.Printf("Ctrl raw: msg=0x%04X vk=0x%02X scan=0x%02X flags=0x%02X injected=%t extra=0x%X time=%d ms\n",
					entry.wm, e.VkCode, e.ScanCode, e.Flags, e.Flags&llkhfInjected != 0, e.DwExtraInfo, e.Time)
			}
		}()
	}
	go func() {
		for index := range switchTo {
			playBeep([2]uint{FreqEn, FreqRu}[index])
			switchLanguage(pair[index].hkl, pair[index].tip, uint16('1'+index))
		}
	}()
	if console {
		printLayoutPair(pair)
		fmt.Println("Для остановки нажмите Ctrl+C")
	}
	return messageLoop()
}
