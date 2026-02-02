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
	WM_INPUTLANGCHANGE        = 0x0051
	VK_LCONTROL               = 0xA2
	VK_RCONTROL               = 0xA3

	KEYEVENTF_KEYUP = 0x0002

	GW_HWNDPREV         = 3
	GW_HWNDPARENT       = 4
	EnRu                = "EnRu"
	Description         = "EnRu Keyboard Layout Switcher"
	En                  = "00000409"
	Ru                  = "00000419"
	debounceMs    int64 = 150  // в миллисекундах
	FreqRu              = 523  // C5 (До)
	FreqEn              = 1046 // C6 (До на октаву выше)
	BeepDuration        = 100 * time.Millisecond
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
	procFindWindow               = user32.NewProc("FindWindowW")

	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procBeep = kernel32.NewProc("Beep")

	hook              uintptr
	exe               string
	err               error
	lastProcessedKey  uint32
	lastProcessedTime int64
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
	result, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)

	if nCode >= 0 && wParam == WM_KEYUP {
		kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))

		if kb.VkCode == VK_LCONTROL || kb.VkCode == VK_RCONTROL {
			now := time.Now().UnixNano() / 1e6 // ms
			last := lastProcessedTime

			if kb.VkCode == lastProcessedKey && (now-last) < debounceMs {
				return result // Уже обрабатывали недавно
			}

			// Обновляем время
			lastProcessedKey = kb.VkCode
			lastProcessedTime = now

			// Запускаем обработку
			go func(vkCode uint32) {
				switch vkCode {
				case VK_LCONTROL:
					playBeep(FreqEn)
					switchLanguage(En)
				case VK_RCONTROL:
					playBeep(FreqRu)
					switchLanguage(Ru)
				}
			}(kb.VkCode)
		}
	}
	return result
}

func switchLanguage(layoutID string) {
	// Загружаем раскладку
	layoutPtr, _ := windows.UTF16PtrFromString(layoutID)
	hkl, _, err := procLoadKeyboardLayout.Call(
		uintptr(unsafe.Pointer(layoutPtr)),
		0,
	)

	if hkl == 0 {
		fmt.Printf("LoadKeyboardLayout %s: %v\n", layoutID, err)
		return
	}

	hwnd, _, err := procGetForegroundWindow.Call()
	if hwnd == 0 {
		fmt.Printf("GetForegroundWindow: %v\n", err)
		return
	}

	class := getWindowClass(hwnd)
	once := true
	// Итерируем через цепочку окон
	for hwnd != 0 {
		if class == "OpusApp" && once { // Word великий и ужасный
			once = false
			fmt.Printf("Не переключаем %s на: %s\n", class, layoutID)
			if progman := findProgman(); progman != 0 {
				procSetForegroundWindow.Call(progman)
				defer procSetForegroundWindow.Call(hwnd)
				hwnd = progman
				class = "Progman"
				continue
			} else {
				return
			}
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
	for _, p := range processes {
		if p == nil || p.Pid() == pid {
			continue
		}
		if strings.EqualFold(p.Executable(), filepath.Base(exe)) {
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

func findProgman() uintptr {
	className, _ := windows.UTF16PtrFromString("Progman")
	hwnd, _, _ := procFindWindow.Call(
		uintptr(unsafe.Pointer(className)),
		0,
	)
	return hwnd
}
