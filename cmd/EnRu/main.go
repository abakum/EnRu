package main

import (
	_ "embed"
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

	version "github.com/abakum/version/lib"
	"github.com/jxeng/shortcut"
	"github.com/mitchellh/go-ps"
	"golang.org/x/sys/windows"
)

var _ = version.Ver

//go:generate go run github.com/abakum/version

//go:embed VERSION
var VERSION string

const (
	WH_KEYBOARD_LL            = 13
	WM_KEYUP                  = 0x0101
	WM_INPUTLANGCHANGEREQUEST = 0x0050
	VK_LCONTROL               = 0xA2
	VK_RCONTROL               = 0xA3
	GW_HWNDPREV               = 3
	EnRu                      = "EnRu"
	Description               = "EnRu Keyboard Layout Switcher"
	En                        = "00000409"
	Ru                        = "00000419"
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
	exe  string
	err  error
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
			switchLanguage(En)
		case VK_RCONTROL:
			switchLanguage(Ru)
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
		fmt.Printf("Не удалось загрузить: %s\n", layoutID)
		return
	}

	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		fmt.Printf("Не удалось получить активное окно")
		return
	}

	class := getWindowClass(hwnd)

	// Итерируем через цепочку окон
	for hwnd != 0 {
		if getKeyboardLayout() == hkl {
			// Уже на
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
			fmt.Printf("Переключились на: %s\n", layoutID)
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

	fmt.Printf("Не удалось переключиться на: %s\n", layoutID)
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

func main() {
	exe, err = os.Executable()
	if err == nil {
		// Как в маке
		exe, err = filepath.EvalSymlinks(exe)
	}

	if err != nil {
		if lp, err := exec.LookPath(os.Args[0]); err == nil {
			exe = lp
		} else if abs, err := Abs(os.Args[0]); err == nil {
			exe = abs
		} else {
			fmt.Println(err)
			return
		}
	}

	s := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		s = "Собрано " + info.GoVersion
	}
	majorVersion, minorVersion, buildNumber := windows.RtlGetNtVersionNumbers()
	fmt.Println(Description, exe, VERSION, s, fmt.Sprintf("%d.%d.%d on %s", majorVersion, minorVersion, buildNumber, runtime.GOARCH))
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := setupStartupShortcut(); err != nil {
				fmt.Printf("Не удалось установить ярлык в папку автозагрузки: %v\n", err)
			}
			fallthrough
		case "start":
		case "uninstall":
			if err := removeStartupShortcut(); err != nil {
				fmt.Printf("Не удалось убрать ярлык из папки автозагрузки: %v\n", err)
			}
			fallthrough
		case "stop":
			if err := stopTask(); err != nil {
				fmt.Printf("Не удалось остановить: %v\n", err)
			}
			return
		case "console":
			stopTask()
			if err := setHook(); err != nil {
				fmt.Println(err)
				return
			}
			defer unhook()
			fmt.Println("Запущена в фоновом режиме")
			fmt.Println("Левый Ctrl -> Английский")
			fmt.Println("Правый Ctrl -> Русский")
			fmt.Println("Для остановки нажмите Ctrl+C")
			messageLoop()
		}
	}
	fmt.Println("Используй команды: install, start, uninstall, stop, console")
	fmt.Println("По команде start остановится и запустится в фоновом режиме")
	fmt.Println("По команде install установится в автозагрузку shell:startup остановится и запустится в фоновом режиме")
	fmt.Println("По команде uninstall уберётся из автозагрузки shell:startup и остановится")
	fmt.Println("Без команды или с неправильной командой остановится и запустится в фоновом режиме")
	fmt.Println("По команде console запустится в консоле для отладки")

	cmd := exec.Command(exe, "console")
	background(cmd)

	if err := cmd.Start(); err != nil {
		fmt.Printf("Ошибка запуска: %v\n", err)
		return
	}

	fmt.Println("Запущена в фоновом режиме")
	fmt.Println("Левый Ctrl -> Английский")
	fmt.Println("Правый Ctrl -> Русский")
}

func stopTask() error {
	// Ищем процесс по имени
	processes, err := ps.Processes()
	if err != nil {
		return fmt.Errorf("не удалось получить процессы: %v", err)
	}

	found := false
	pid := os.Getpid()
	exe := filepath.Base(os.Args[0])
	for _, p := range processes {
		if p == nil || p.Pid() == pid {
			continue
		}
		if strings.EqualFold(p.Executable(), exe) {
			found = true
			// Находим фоновый процесс
			proc, err := os.FindProcess(p.Pid())
			if err != nil {
				return fmt.Errorf("не удалось найти фоновый процесс %d: %v", p.Pid(), err)
			}

			err = proc.Kill()
			if err != nil {
				return fmt.Errorf("не удалось остановить фоновый процесс %d: %v", p.Pid(), err)
			}

			fmt.Printf("Фоновый процесс %d остановлен\n", p.Pid())
			time.Sleep(100 * time.Millisecond)
		}
	}

	if !found {
		fmt.Println("Фоновый процесс не запущен")
	}

	return nil
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
		hwnd, _, _ = procGetWindow.Call(hwnd, GW_HWNDPREV)
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

func setupStartupShortcut() error {
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
		Arguments:    "start",
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

func getStartupDir() (string, error) {
	// Для текущего пользователя
	dir := os.Getenv("APPDATA")
	if dir == "" {
		return "", fmt.Errorf("не найдена переменная среды APPDATA")
	}
	return filepath.Join(dir, "Microsoft", "Windows", "Start Menu", "Programs", "Startup"), nil
}

// Функция удаления ярлыка из автозагрузки
func removeStartupShortcut() error {
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
