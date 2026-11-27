package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type layoutPair [2]struct {
	tip string
	hkl uintptr
}

func main() {
	args := os.Args[1:]
	err := runWindows(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if len(args) == 0 || (args[0] != "console" && args[0] != "--background" && args[0] != "--startup") {
		pauseConsole()
	}
	if err != nil {
		os.Exit(1)
	}
}

func runWindows(args []string) error {
	var command string
	if len(args) != 0 {
		command = args[0]
	}
	if command == "--background" {
		if len(args) != 5 {
			return errors.New("не переданы две раскладки для фонового запуска")
		}
		var pair layoutPair
		for i := range pair {
			pair[i].tip = args[1+i*2]
			hkl, err := strconv.ParseUint(args[2+i*2], 16, strconv.IntSize)
			if err != nil || hkl <= 1 || pair[i].tip == "" {
				return errors.New("недопустимая цель фонового переключения")
			}
			pair[i].hkl = uintptr(hkl)
		}
		if pair[0].hkl == pair[1].hkl {
			return errors.New("цели фонового переключения совпадают")
		}
		return startConsole(pair, false)
	}

	exe = resolveExe()
	if exe == "" {
		return errors.New("не удалось определить путь к EnRu")
	}
	printVersionInfo()
	if command == "stop" {
		return stopTask()
	}
	if command == "install" || command == "uninstall" {
		printSavedDirectHotkeys("До")
		defer printSavedDirectHotkeys("После")
		if err := checkShellUser(); err != nil {
			return fmt.Errorf("не удалось подтвердить профиль интерактивного пользователя: %w", err)
		}
	}
	if command == "uninstall" {
		fmt.Println("Будут сняты все прямые сочетания выбора раскладки, в том числе используемые c1c2.")
		apply, err := prepareHotkeys(nil, nil, nil)
		if err != nil {
			printManualHotkeys(false)
			return err
		}
		if err := stopTask(); err != nil {
			return fmt.Errorf("остановка EnRu не завершена; ярлык и сочетания не менялись: %w", err)
		}
		if err := removeAutostart(); err != nil {
			return fmt.Errorf("EnRu остановлен, сочетания не менялись: %w", err)
		}
		if err := apply(); err != nil {
			printManualHotkeys(false)
			return fmt.Errorf("EnRu остановлен и ярлык удалён, но очистка сочетаний не завершена: %w", err)
		}
		return nil
	}

	limit := 2
	if command == "install" {
		limit = 0
	}
	first, tips, err := profileLayouts(limit)
	if command == "install" && first != "" && !isEnglish(first) {
		// Open the dialog after the setup attempt so it cannot change its input snapshot.
		defer suggestEnglishFirst(first)
	}
	if err != nil {
		if command == "install" {
			printManualHotkeys(true)
		}
		return fmt.Errorf("не удалось прочитать порядок раскладок: %w", err)
	}
	targets, handles, err := loadedLayouts(tips)
	if err != nil {
		if command == "install" {
			printManualHotkeys(true)
		}
		return fmt.Errorf("не удалось определить цели переключения: %w", err)
	}
	var pair layoutPair
	for i := range pair {
		pair[i].tip, pair[i].hkl = tips[i], handles[targets[i]]
	}
	if command == "install" {
		apply, err := prepareHotkeys(tips, targets, handles)
		if err == nil {
			err = apply()
		}
		if err != nil {
			printManualHotkeys(true)
			return fmt.Errorf("не удалось настроить сочетания: %w", err)
		}
		if err := installAutostart(); err != nil {
			return fmt.Errorf("сочетания уже настроены, но ярлык не создан: %w", err)
		}
	}
	if err := stopTask(); err != nil {
		if command == "install" {
			return fmt.Errorf("сочетания настроены и ярлык создан, но перезапуск не завершён: %w", err)
		}
		return err
	}
	if command == "console" {
		return startConsole(pair, true)
	}
	if err := startBackground(pair); err != nil {
		if command == "install" {
			return fmt.Errorf("сочетания настроены и ярлык создан, но EnRu не запущен: %w", err)
		}
		return err
	}
	if command == "" {
		fmt.Println("Команды: install, start, uninstall, stop, console")
	}
	fmt.Println("EnRu запущен в фоновом режиме")
	printLayoutPair(pair)
	return nil
}

func printLayoutPair(pair layoutPair) {
	fmt.Printf("Левый Ctrl → первая раскладка: %s (HKL 0x%08X)\n", pair[0].tip, uint32(pair[0].hkl))
	fmt.Printf("Правый Ctrl → вторая раскладка: %s (HKL 0x%08X)\n", pair[1].tip, uint32(pair[1].hkl))
}

func isEnglish(tag string) bool {
	tag = strings.ToLower(tag)
	return tag == "en" || strings.HasPrefix(tag, "en-")
}

func suggestEnglishFirst(first string) {
	fmt.Printf("Первый язык: %s. Сочетания рассчитываются по текущему порядку раскладок.\n", first)
	fmt.Println("В окне «Язык» выделите английский и нажимайте «Вверх», пока он не станет первым. Если английского нет, нажмите «Добавить язык». Затем повторите EnRu install.")
	dir, err := windows.GetSystemDirectory()
	if err == nil {
		cmd := exec.Command(filepath.Join(dir, "rundll32.exe"), "shell32.dll,Control_RunDLL", "input.dll")
		background(cmd)
		err = cmd.Start()
		if err == nil {
			_ = cmd.Process.Release()
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Не удалось открыть окно «Язык»: %v\n", err)
		fmt.Println("Откройте вручную: rundll32.exe shell32.dll,Control_RunDLL input.dll")
	}
}

func printManualHotkeys(install bool) {
	fmt.Println("Ручная настройка: rundll32.exe shell32.dll,Control_RunDLL input.dll,,{C07337D3-DB2C-4D0B-9A93-B722A6C106E2}{HOTKEYS}")
	fmt.Println("На вкладке «Переключение клавиатуры» выберите нужную раскладку и нажмите «Сменить сочетание клавиш». Не меняйте строку «Между языками ввода».")
	if install {
		fmt.Println("Назначьте раскладкам по порядку Ctrl+1…Ctrl+9 и Ctrl+0, начиная с 11-й снимите прямые сочетания. Нажмите «Применить».")
	} else {
		fmt.Println("Для каждой раскладки снимите флажок «Использовать сочетание клавиш». Нажмите «Применить».")
	}
}

func checkShellUser() error {
	hwnd, _, _ := user32.NewProc("GetShellWindow").Call()
	if hwnd == 0 {
		return errors.New("окно оболочки Windows не найдено")
	}
	var pid uint32
	if tid, _, err := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid))); tid == 0 || pid == 0 {
		return fmt.Errorf("не удалось определить процесс оболочки: %v", err)
	}
	var shellSession, ownSession uint32
	if err := windows.ProcessIdToSessionId(pid, &shellSession); err != nil {
		return err
	}
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &ownSession); err != nil {
		return err
	}
	if shellSession != ownSession {
		return errors.New("EnRu и оболочка работают в разных сеансах")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	shell, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	own, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !shell.User.Sid.Equals(own.User.Sid) {
		return fmt.Errorf("EnRu запущен под другой учётной записью (SID EnRu: %s, оболочки: %s)", own.User.Sid, shell.User.Sid)
	}
	return nil
}

func pauseConsole() {
	input, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	var mode uint32
	if err != nil || windows.GetConsoleMode(input, &mode) != nil {
		return
	}
	readInput := kernel32.NewProc("ReadConsoleInputW")
	var record struct { // Win32 INPUT_RECORD.
		eventType uint16
		_         uint16
		keyDown   uint32
		_         [12]byte
	}
	var read uint32
	fmt.Print("Нажмите любую клавишу для продолжения . . .")
	for {
		ok, _, _ := readInput.Call(uintptr(input), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&read)))
		if ok == 0 || (read == 1 && record.eventType == 1 && record.keyDown != 0) {
			break
		}
	}
	fmt.Println()
}
