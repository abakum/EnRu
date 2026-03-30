//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/grafov/evdev"
	"golang.org/x/sys/unix"
)

func resolveExe() string {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		if lp, err := exec.LookPath(os.Args[0]); err == nil {
			return lp
		} else if abs, err := filepath.Abs(os.Args[0]); err == nil {
			return abs
		}
		fmt.Println(err)
		os.Exit(1)
	}
	return exe
}

func printVersionInfo() {
	s := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		s = "Собрано " + info.GoVersion
	}
	fmt.Println(Description, exe, VERSION, s, runtime.GOOS+"/"+runtime.GOARCH)
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
	wavPath, err := generateWAVFile(freq, int(BeepDuration.Milliseconds()))
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

// switchMethod хранит имя метода переключения для логирования
var switchMethod string

// switchLayoutFunc кэширует рабочий метод переключения после первого успеха
var switchLayoutFunc func(uint) error

// switchLayout переключает раскладку, пробуя несколько методов:
// 1) KDE D-Bus  2) GNOME Shell  3) IBus engine  4) setxkbmap (X11)
// После первого успеха кэширует рабочий метод
func switchLayout(group uint) error {
	// Если метод уже определён — используем его
	if switchLayoutFunc != nil {
		return switchLayoutFunc(group)
	}

	// Метод 1: KDE D-Bus
	if err := switchLayoutKDE(group); err == nil {
		switchMethod = "KDE"
		switchLayoutFunc = switchLayoutKDE
		log.Printf("Метод переключения: %s", switchMethod)
		return nil
	}

	// Метод 2: GNOME Shell Eval (D-Bus)
	if err := switchLayoutGNOME(group); err == nil {
		switchMethod = "GNOME"
		switchLayoutFunc = switchLayoutGNOME
		log.Printf("Метод переключения: %s", switchMethod)
		return nil
	}

	// Метод 3: IBus engine
	if err := switchLayoutIBus(group); err == nil {
		switchMethod = "IBus"
		switchLayoutFunc = switchLayoutIBus
		log.Printf("Метод переключения: %s", switchMethod)
		return nil
	}

	// Метод 4: setxkbmap (X11)
	if err := switchLayoutSetxkbmap(group); err == nil {
		switchMethod = "setxkbmap"
		switchLayoutFunc = switchLayoutSetxkbmap
		log.Printf("Метод переключения: %s", switchMethod)
		return nil
	}

	return fmt.Errorf("все методы не сработали (KDE, GNOME, IBus, setxkbmap)")
}

// switchLayoutKDE переключает раскладку через KDE D-Bus
func switchLayoutKDE(group uint) error {
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

// switchLayoutGNOME переключает раскладку через GNOME Shell D-Bus Eval
// Работает на GNOME (X11 и Wayland), обновляет индикатор раскладок
func switchLayoutGNOME(group uint) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("GNOME D-Bus connect: %v", err)
	}
	defer conn.Close()

	// JavaScript для переключения InputSource в GNOME Shell
	js := fmt.Sprintf(
		"imports.ui.status.keyboard.getInputSourceManager().inputSources[%d].activate()",
		group)

	obj := conn.Object("org.gnome.Shell", "/org/gnome/Shell")
	call := obj.Call("org.gnome.Shell.Eval", 0, js)
	if call.Err != nil {
		return fmt.Errorf("GNOME Shell.Eval: %v", call.Err)
	}

	// Shell.Eval возвращает (success bool, result string)
	if len(call.Body) < 1 {
		return fmt.Errorf("GNOME Shell.Eval: пустой ответ")
	}
	success := call.Body[0].(bool)
	if !success {
		return fmt.Errorf("GNOME Shell.Eval: вернул false (возможно unsafe-mode выключен)")
	}
	return nil
}

// switchLayoutIBus переключает раскладку через ibus engine (внешняя команда)
func switchLayoutIBus(group uint) error {
	var engineName string
	switch group {
	case 0:
		engineName = "xkb:us::eng"
	case 1:
		engineName = "xkb:ru::rus"
	default:
		engineName = "xkb:us::eng"
	}
	cmd := exec.Command("ibus", "engine", engineName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ibus engine %s: %v", engineName, err)
	}
	return nil
}

// switchLayoutSetxkbmap переключает раскладку через setxkbmap (X11)
// Работает даже при активном IBus, т.к. меняет конфигурацию XKB на уровне X-сервера
func switchLayoutSetxkbmap(group uint) error {
	layout := "us"
	if group == 1 {
		layout = "ru"
	}
	cmd := exec.Command("setxkbmap", "-layout", layout)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setxkbmap -layout %s: %v", layout, err)
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

// canAccessEvdev проверяет, есть ли у пользователя права на чтение /dev/input/event*
func canAccessEvdev() bool {
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil || len(paths) == 0 {
		return false
	}
	f, err := os.OpenFile(paths[0], os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// inotifyEv — событие inotify, передаваемое по каналу
type inotifyEv struct {
	name string
	mask uint32
}

// watchDevices отслеживает клавиатуры через inotify (без опроса).
// Первичное сканирование + реакция на подключение/отключение устройств.
func watchDevices(ctx context.Context, inbox chan message) {
	if !canAccessEvdev() {
		log.Println("evdev: нет доступа к /dev/input/event*")
		log.Println("  Добавьте пользователя в группу input: sudo usermod -aG input $USER")
		log.Println("  Работает только XRecord (VNC/X11)")
		return
	}

	keyboards := make(map[string]*evdev.InputDevice)
	kbdLost := make(chan string, 8)

	// Первичное сканирование существующих устройств
	for devicePath, device := range getInputDevices() {
		if isKeyboard(device) {
			log.Printf("Клавиатура: %s (%s)", device.Name, devicePath)
			keyboards[devicePath] = device
			go listenEvents(ctx, devicePath, device, inbox, kbdLost)
		} else {
			device.File.Close()
		}
	}

	// inotify для отслеживания новых/удалённых устройств
	fd, err := unix.InotifyInit()
	if err != nil {
		log.Printf("evdev: inotify: %v", err)
		return
	}
	defer unix.Close(fd)

	_, err = unix.InotifyAddWatch(fd, "/dev/input",
		unix.IN_CREATE|unix.IN_DELETE|unix.IN_MOVED_TO)
	if err != nil {
		log.Printf("evdev: inotify: %v", err)
		return
	}

	// Горутина для чтения событий inotify (неблокирующая)
	inotifyCh := make(chan inotifyEv, 16)
	go func() {
		defer close(inotifyCh)
		buf := make([]byte, unix.SizeofInotifyEvent+4096)
		for {
			n, err := unix.Read(fd, buf)
			if err != nil {
				return // fd закрыт → выход
			}
			var offset uint32
			for offset < uint32(n) {
				event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
				nameLen := event.Len
				if nameLen > 0 {
					name := string(buf[offset+unix.SizeofInotifyEvent : offset+unix.SizeofInotifyEvent+nameLen])
					name = strings.TrimRight(name, "\x00")
					inotifyCh <- inotifyEv{name: name, mask: event.Mask}
				}
				offset += unix.SizeofInotifyEvent + nameLen
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return // defer закроет fd → горутина выйдет
		case name := <-kbdLost:
			delete(keyboards, name)
		case ie, ok := <-inotifyCh:
			if !ok {
				return
			}
			if !strings.HasPrefix(ie.name, "event") {
				continue
			}
			devicePath := "/dev/input/" + ie.name
			switch {
			case ie.mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0:
				time.Sleep(100 * time.Millisecond) // ждём готовности устройства
				device, err := evdev.Open(devicePath)
				if err != nil {
					continue
				}
				if isKeyboard(device) {
					if _, ok := keyboards[devicePath]; !ok {
						log.Printf("Клавиатура: %s (%s)", device.Name, devicePath)
						keyboards[devicePath] = device
						go listenEvents(ctx, devicePath, device, inbox, kbdLost)
					}
				} else {
					device.File.Close()
				}
			case ie.mask&unix.IN_DELETE != 0:
				delete(keyboards, devicePath)
			}
		}
	}
}

func listenEvents(ctx context.Context, name string, kbd *evdev.InputDevice, replyTo chan message, kbdLost chan string) {
	// При отмене контекста закрываем файл устройства → kbd.Read() вернёт ошибку
	go func() {
		<-ctx.Done()
		kbd.File.Close()
	}()

	for {
		events, err := kbd.Read()
		if err != nil || len(events) == 0 {
			select {
			case <-ctx.Done():
				// Нормальное завершение
			default:
				log.Printf("Клавиатура %s отключена", kbd.Name)
				kbdLost <- name
			}
			return
		}
		select {
		case replyTo <- message{Device: kbd, Events: events}:
		case <-ctx.Done():
			return
		}
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

func listenKeyboards(ctx context.Context, leftCode, rightCode uint16, printMode bool) {
	inbox := make(chan message, 8)

	go watchDevices(ctx, inbox)
	go listenX11Record(ctx, inbox)

	var useGroup int
	var prevKey evdev.InputEvent
	var prevKeyTime time.Time

	for {
		var msg message
		select {
		case <-ctx.Done():
			return
		case msg = <-inbox:
		}
		for _, ev := range msg.Events {
			if ev.Type != evdev.EV_KEY {
				continue
			}
			// Skip duplicates (same key+state from evdev and X11)
			now := time.Now()
			if prevKey.Code == ev.Code && prevKey.Value == ev.Value && now.Sub(prevKeyTime) < debounceMs*time.Millisecond {
				continue
			}
			prevKey = ev
			prevKeyTime = now

			if printMode {
				name := "X11"
				if msg.Device != nil {
					name = msg.Device.Name
				}
				log.Printf("%s type:%v code:%v(%s) %s", name, ev.Type, ev.Code, keyName(ev.Code), valueName(ev.Value))
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

func installAutostart() error {
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

func startBackground() {
	cmd := exec.Command(exe, "console")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		fmt.Printf("Ошибка запуска в фоне: %v\n", err)
		os.Exit(1)
	}
}

func startConsole(ctx context.Context) {
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

	listenKeyboards(ctx, leftCode, rightCode, true)
}

func printUsage() {
	fmt.Println("Используй команды: install, start, uninstall, stop, console")
	fmt.Println("По команде start остановится и запустится в фоновом режиме")
	fmt.Println("По команде install установится в автозагрузку ~/.config/autostart/ остановится и запустится в фоновом режиме")
	fmt.Println("По команде uninstall уберётся из автозагрузки и остановится")
	fmt.Println("Без команды или с неправильной командой остановится и запустится в фоновом режиме")
	fmt.Println("По команде console запустится в консоли для отладки")
}
