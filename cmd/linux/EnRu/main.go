//go:build linux

package main

import (
	_ "embed"
	"encoding/binary"
	"fmt"
ь	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/grafov/evdev"
	version "github.com/abakum/version/lib"
	"github.com/mitchellh/go-ps"
)

var _ = version.Ver

//go:generate go run github.com/abakum/version

//go:embed VERSION
var VERSION string

const (
	EnRu         = "EnRu"
	Description  = "EnRu Keyboard Layout Switcher (Linux)"
	FreqRu       = 523  // C5 (До)
	FreqEn       = 1046 // C6 (До на октаву выше)
	BeepDuration = 100  // миллисекунды
	scanPeriod   = 4 * time.Second
)

var (
	exe string
	err error
)

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

	if _, err := exec.LookPath("paplay"); err == nil {
		cmd := exec.Command("paplay", wavPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	if _, err := exec.LookPath("aplay"); err == nil {
		cmd := exec.Command("aplay", "-q", wavPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	fmt.Print("\a")
}

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

// switchLayout переключает раскладку через KDE D-Bus
func switchLayout(group uint) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("D-Bus connect: %v", err)
	}
	defer conn.Close()

	obj := conn.Object("org.kde.keyboard", "/Layouts")
	call := obj.Call("org.kde.KeyboardLayouts.setLayout", 0, group)
	if call.Err != nil {
		return fmt.Errorf("D-Bus setLayout(%d): %v", group, call.Err)
	}
	return nil
}

// getKeyCode возвращает evdev код клавиши по имени
func getKeyCode(keysym string) (uint16, error) {
	for evdevKeyCode, evdevKeySym := range evdev.KEY {
		if evdevKeySym == "KEY_"+keysym {
			return uint16(evdevKeyCode), nil
		}
	}
	return 0, fmt.Errorf("keycode for KEY_%s not found", keysym)
}

// message объединяет устройство и события для передачи по каналу
type message struct {
	Device *evdev.InputDevice
	Events []evdev.InputEvent
}

func getInputDevices() map[string]*evdev.InputDevice {
	inputDevices := make(map[string]*evdev.InputDevice)
	devicePaths, err := evdev.ListInputDevicePaths("/dev/input/event*")
	if err == nil && len(devicePaths) > 0 {
		for _, devicePath := range devicePaths {
			device, err := evdev.Open(devicePath)
			if err != nil {
				continue
			}
			inputDevices[devicePath] = device
		}
	}
	return inputDevices
}

func isKeyboard(device *evdev.InputDevice) bool {
	caps, ok := device.Capabilities["EV_KEY"]
	if !ok {
		return false
	}
	// Клавиатура должна иметь KEY_LEFTCTRL (29) и KEY_SPACE (57)
	_, hasCtrl := caps[29]
	_, hasSpace := caps[57]
	return hasCtrl && hasSpace
}

func scanDevices(inbox chan message, scanOnce bool) {
	keyboards := make(map[string]*evdev.InputDevice)
	kbdLost := make(chan string, 8)

	for {
		select {
		case name := <-kbdLost:
			delete(keyboards, name)
		default:
			for devicePath, device := range getInputDevices() {
				if isKeyboard(device) {
					if _, ok := keyboards[devicePath]; !ok {
						log.Printf("Клавиатура: %s (%s)", device.Name, devicePath)
						keyboards[devicePath] = device
						go listenEvents(devicePath, device, inbox, kbdLost)
					}
				} else {
					device.File.Close()
				}
			}
			if scanOnce {
				return
			}
			time.Sleep(scanPeriod)
		}
	}
}

func listenEvents(name string, kbd *evdev.InputDevice, replyTo chan message, kbdLost chan string) {
	for {
		events, err := kbd.Read()
		if err != nil || len(events) == 0 {
			log.Printf("Клавиатура %s отключена", kbd.Name)
			kbdLost <- name
			return
		}
		replyTo <- message{Device: kbd, Events: events}
	}
}

func keyName(code uint16) string {
	if name, ok := evdev.KEY[int(code)]; ok {
		return strings.TrimPrefix(name, "KEY_")
	}
	return fmt.Sprintf("KEY_%d", code)
}

func valueName(v int32) string {
	switch v {
	case 0:
		return "released"
	case 1:
		return "pressed"
	case 2:
		return "hold"
	default:
		return fmt.Sprintf("undefined(%d)", v)
	}
}

func listenKeyboards(leftCode, rightCode uint16, printMode bool) {
	inbox := make(chan message, 8)

	go scanDevices(inbox, false)

	var useGroup int
	var prevKey evdev.InputEvent

	for msg := range inbox {
		for _, ev := range msg.Events {
			if ev.Type != evdev.EV_KEY {
				continue
			}
			// Skip duplicates
			if prevKey.Code == ev.Code && prevKey.Value == ev.Value {
				continue
			}
			prevKey = ev

			if printMode {
				log.Printf("%s type:%v code:%v(%s) %s", msg.Device.Name, ev.Type, ev.Code, keyName(ev.Code), valueName(ev.Value))
			}

			switch ev.Value {
			case 1: // key down
				useGroup = 0
				if ev.Code == leftCode {
					useGroup = 1
				} else if ev.Code == rightCode {
					useGroup = 2
				}
			case 0: // key up
				if useGroup == 0 {
					break
				}
				if ev.Code == leftCode && useGroup == 1 {
					playBeep(FreqEn)
					if err := switchLayout(0); err != nil {
						log.Printf("English: %v", err)
					} else {
						log.Println("→ English")
					}
				} else if ev.Code == rightCode && useGroup == 2 {
					playBeep(FreqRu)
					if err := switchLayout(1); err != nil {
						log.Printf("Русский: %v", err)
					} else {
						log.Println("→ Русский")
					}
				}
				useGroup = 0
			}
		}
	}
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
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить в фоне: %v", err)
	}
	return nil
}

func runConsole() {
	// Определяем коды клавиш
	leftCode, err := getKeyCode("LEFTCTRL")
	if err != nil {
		log.Fatalf("Не найден код LEFTCTRL: %v", err)
	}
	rightCode, err := getKeyCode("RIGHTCTRL")
	if err != nil {
		log.Fatalf("Не найден код RIGHTCTRL: %v", err)
	}

	fmt.Printf("LEFTCTRL=%d RIGHTCTRL=%d\n", leftCode, rightCode)

	// Обработка сигналов для корректного завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nПолучен сигнал завершения...")
		os.Exit(0)
	}()

	fmt.Println("Запущена в консоли (evdev)")
	fmt.Println("Левый Ctrl -> Английский")
	fmt.Println("Правый Ctrl -> Русский")
	fmt.Println("Для остановки нажмите Ctrl+C")

	listenKeyboards(leftCode, rightCode, true)
}

func main() {
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