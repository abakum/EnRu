//go:build linux

package main

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	version "github.com/abakum/version/lib"
	"github.com/mitchellh/go-ps"
)

var _ = version.Ver

//go:generate go run github.com/abakum/version

//go:embed VERSION
var VERSION string

const (
	EnRu         = "EnRu"
	Description  = "EnRu Keyboard Layout Switcher (Linux/X11)"
	debounceMs   = 150  // в миллисекундах
	FreqRu       = 523  // C5 (До)
	FreqEn       = 1046 // C6 (До на октаву выше)
	BeepDuration = 100  // в миллисекундах

	// X11 GrabMode
	GrabModeSync  = 1
	GrabModeAsync = 2

	// X11 Keycodes для Ctrl (X11 keycode = physical scan code + 8)
	KeyCodeLCtrl = 0x25 + 8 // 37
	KeyCodeRCtrl = 0x69 + 8 // 109

	// X11 event masks
	KeyPressMask   = 1 << 0
	KeyReleaseMask = 1 << 1
)

var (
	exe               string
	err               error
	lastProcessedKey  byte
	lastProcessedTime int64
)

func checkEnvironment() {
	// Проверка наличия GUI (X11 дисплей)
	display := os.Getenv("DISPLAY")
	if display == "" {
		fmt.Fprintln(os.Stderr, "Ошибка: Нет X11 дисплея (нет GUI). Убедитесь, что переменная DISPLAY установлена.")
		os.Exit(1)
	}

	// Проверка что не Wayland
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	if sessionType == "wayland" {
		fmt.Fprintln(os.Stderr, "Ошибка: Wayland не поддерживается. Глобальный перехват клавиатуры невозможен в Wayland по соображениям безопасности.")
		fmt.Fprintln(os.Stderr, "Переключитесь на X11-сессию (обычно при логине можно выбрать «Xorg» или «Ubuntu on Xorg»).")
		os.Exit(1)
	}
}

// generateWAV генерирует WAV-данные для тона заданной частоты и длительности
func generateWAV(freq uint, durationMs int) []byte {
	sampleRate := uint32(44100)
	numSamples := sampleRate * uint32(durationMs) / 1000
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := uint16(numChannels) * bitsPerSample / 8
	dataSize := numSamples * uint32(blockAlign)
	fileSize := uint32(36) + dataSize

	buf := make([]byte, 44+dataSize)

	// RIFF header
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], fileSize)
	copy(buf[8:12], "WAVE")

	// fmt chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], numChannels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	binary.LittleEndian.PutUint16(buf[32:34], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)

	// data chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)

	// Генерация синусоиды
	amplitude := float64(32767 * 0.3) // 30% громкости
	for i := uint32(0); i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := int16(amplitude * sin(2*3.14159265358979323846*float64(freq)*t))
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(sample))
	}

	return buf
}

func sin(x float64) float64 {
	// Простая аппроксимация синуса (достаточно для beep)
	pi := 3.14159265358979323846
	x = x - float64(int(x/(2*pi)))*2*pi
	if x < 0 {
		x = -x
	}
	if x > pi {
		x = x - pi
		return -x * (1.2732395447351627 - 0.40528473456935109*x)
	}
	return x * (1.2732395447351627 - 0.40528473456935109*x)
}

func playBeep(freq uint) {
	wavPath, err := generateWAVFile(freq, BeepDuration)
	if err != nil {
		fmt.Print("\a")
		return
	}
	defer os.Remove(wavPath)

	// Пытаемся paplay (PulseAudio/PipeWire)
	if _, err := exec.LookPath("paplay"); err == nil {
		cmd := exec.Command("paplay", wavPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	// Пытаемся aplay (ALSA)
	if _, err := exec.LookPath("aplay"); err == nil {
		cmd := exec.Command("aplay", "-q", wavPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	// Запасной вариант: терминальный bell
	fmt.Print("\a")
}

// generateWAVFile создаёт временный WAV файл для воспроизведения
func generateWAVFile(freq uint, durationMs int) (string, error) {
	data := generateWAV(freq, durationMs)
	tmpFile, err := os.CreateTemp("", "enru-beep-*.wav")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	_, err = tmpFile.Write(data)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	return tmpFile.Name(), nil
}

func switchLayout(group uint8) error {
	var layout string
	switch group {
	case 0:
		layout = "us"
	case 1:
		layout = "ru"
	default:
		return fmt.Errorf("неизвестная группа раскладки: %d", group)
	}
	cmd := exec.Command("setxkbmap", "-layout", layout)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setxkbmap %s: %v", layout, err)
	}
	return nil
}

func keyEventLoop(X *xgb.Conn) error {
	setup := xproto.Setup(X)
	screen := setup.DefaultScreen(X)
	root := screen.Root

	// Грабим Ctrl клавиши
	// GrabModeSync позволяет нам увидеть событие и затем решить, пропустить его дальше
	err := xproto.GrabKey(X, true, root, 
		xproto.ModMaskAny, xproto.Keycode(KeyCodeLCtrl), 
		xproto.GrabModeSync, xproto.GrabModeAsync).Check()
	if err != nil {
		return fmt.Errorf("GrabKey LCtrl: %v", err)
	}

	err = xproto.GrabKey(X, true, root,
		xproto.ModMaskAny, xproto.Keycode(KeyCodeRCtrl),
		xproto.GrabModeSync, xproto.GrabModeAsync).Check()
	if err != nil {
		return fmt.Errorf("GrabKey RCtrl: %v", err)
	}

	X.Sync()

	for {
		ev, err := X.WaitForEvent()
		if err != nil {
			return fmt.Errorf("WaitForEvent: %v", err)
		}
		if ev == nil {
			continue
		}

		switch keyEv := ev.(type) {
		case xproto.KeyReleaseEvent:
			// Разрешаем событию пройти дальше к приложению
			xproto.AllowEvents(X, xproto.AllowReplayPointer, xproto.Timestamp(keyEv.Time))
			X.Sync()

			now := time.Now().UnixNano() / 1e6 // ms
			keyCode := byte(keyEv.Detail)

			if keyCode == lastProcessedKey && (now-lastProcessedTime) < int64(debounceMs) {
				continue // Уже обрабатывали недавно
			}

			lastProcessedKey = keyCode
			lastProcessedTime = now

			switch keyCode {
			case KeyCodeLCtrl:
				playBeep(FreqEn)
				if err := switchLayout(0); err != nil {
					fmt.Printf("Переключение на English: %v\n", err)
				} else {
					fmt.Println("Переключено на English")
				}
			case KeyCodeRCtrl:
				playBeep(FreqRu)
				if err := switchLayout(1); err != nil {
					fmt.Printf("Переключение на Русский: %v\n", err)
				} else {
					fmt.Println("Переключено на Русский")
				}
			}

		case xproto.KeyPressEvent:
			// Просто пропускаем KeyPress
			xproto.AllowEvents(X, xproto.AllowReplayPointer, xproto.Timestamp(keyEv.Time))
			X.Sync()
		}
	}
}

func cleanup(X *xgb.Conn, root xproto.Window) {
	if X == nil {
		return
	}
	_ = xproto.UngrabKey(X, xproto.Keycode(KeyCodeLCtrl), root, xproto.ModMaskAny)
	_ = xproto.UngrabKey(X, xproto.Keycode(KeyCodeRCtrl), root, xproto.ModMaskAny)
	X.Close()
}

func stopTask() error {
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

func setupAutostart() error {
	autostartDir := filepath.Join(os.Getenv("HOME"), ".config", "autostart")
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию автозагрузки: %v", err)
	}

	desktopPath := filepath.Join(autostartDir, "EnRu.desktop")
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=EnRu
Comment=%s
Exec=%s start
Terminal=false
Hidden=false
`, Description, exe)

	if err := os.WriteFile(desktopPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("не удалось создать .desktop файл: %v", err)
	}

	fmt.Printf("Автозагрузка установлена: %s\n", desktopPath)
	return nil
}

func removeAutostart() error {
	autostartDir := filepath.Join(os.Getenv("HOME"), ".config", "autostart")
	desktopPath := filepath.Join(autostartDir, "EnRu.desktop")

	err := os.Remove(desktopPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("не удалось удалить .desktop файл: %v", err)
	}

	fmt.Printf("Автозагрузка убрана: %s\n", desktopPath)
	return nil
}

func forkBackground() error {
	cmd := exec.Command(exe, "console")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	// Отсоединяем stdin/stdout/stderr
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить в фоне: %v", err)
	}
	return nil
}

func main() {
	checkEnvironment()

	exe, err = os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		if lp, err := exec.LookPath(os.Args[0]); err == nil {
			exe = lp
		} else if abs, err := filepath.Abs(os.Args[0]); err == nil {
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
	fmt.Println(Description, exe, VERSION, s, runtime.GOOS+"/"+runtime.GOARCH)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := setupAutostart(); err != nil {
				fmt.Printf("Не удалось установить автозагрузку: %v\n", err)
			}
			fallthrough
		case "start":
			stopTask()
			if err := forkBackground(); err != nil {
				fmt.Printf("Ошибка запуска в фоне: %v\n", err)
			} else {
				fmt.Println("Запущена в фоновом режиме")
				fmt.Println("Левый Ctrl -> Английский")
				fmt.Println("Правый Ctrl -> Русский")
			}
			return
		case "uninstall":
			if err := removeAutostart(); err != nil {
				fmt.Printf("Не удалось убрать автозагрузку: %v\n", err)
			}
			fallthrough
		case "stop":
			if err := stopTask(); err != nil {
				fmt.Printf("Не удалось остановить: %v\n", err)
			}
			return
		case "console":
			stopTask()
			runConsole()
			return
		}
	}

	fmt.Println("Используй команды: install, start, uninstall, stop, console")
	fmt.Println("По команде start остановится и запустится в фоновом режиме")
	fmt.Println("По команде install установится в автозагрузку ~/.config/autostart/ остановится и запустится в фоновом режиме")
	fmt.Println("По команде uninstall уберётся из автозагрузки и остановится")
	fmt.Println("Без команды или с неправильной командой остановится и запустится в фоновом режиме")
	fmt.Println("По команде console запустится в консоли для отладки")

	stopTask()
	if err := forkBackground(); err != nil {
		fmt.Printf("Ошибка запуска в фоне: %v\n", err)
		return
	}

	fmt.Println("Запущена в фоновом режиме")
	fmt.Println("Левый Ctrl -> Английский")
	fmt.Println("Правый Ctrl -> Русский")
}

func runConsole() {
	X, err := xgb.NewConn()
	if err != nil {
		fmt.Printf("Не удалось подключиться к X11: %v\n", err)
		os.Exit(1)
	}
	defer X.Close()

	setup := xproto.Setup(X)
	screen := setup.DefaultScreen(X)
	root := screen.Root

	// Обработка сигналов для корректного завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nПолучен сигнал завершения...")
		cleanup(X, root)
		os.Exit(0)
	}()

	fmt.Println("Запущена в консоли (X11)")
	fmt.Println("Левый Ctrl -> Английский")
	fmt.Println("Правый Ctrl -> Русский")
	fmt.Println("Для остановки нажмите Ctrl+C")

	if err := keyEventLoop(X); err != nil {
		fmt.Printf("Ошибка в цикле обработки: %v\n", err)
	}
}
