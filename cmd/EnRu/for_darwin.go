//go:build darwin

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"
)

// Коды клавиш macOS (kVK_Control / kVK_RightControl из Events.h)
const (
	leftCtrlCode  uint32 = 59
	rightCtrlCode uint32 = 62

	launchAgentLabel = "github.abakum.EnRu"
	plistName        = "github.abakum.EnRu.plist"
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

func playBeep(freq uint) {
	wavPath, err := generateWAVFile(freq, int(BeepDuration.Milliseconds()))
	if err != nil {
		fmt.Print("\a")
		return
	}
	defer os.Remove(wavPath)

	if _, err := exec.LookPath("afplay"); err == nil {
		cmd := exec.Command("afplay", wavPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	fmt.Print("\a")
}

// listenKeyboards получает события из CGEventTap и переключает раскладку
// по клику (нажал-отпустил без других клавиш) левого/правого Ctrl.
func listenKeyboards(ctx context.Context, leftCode, rightCode uint32, printMode bool) {
	inbox := make(chan darwinKeyEvent, 16)

	go listenCGEventTap(ctx, inbox)

	var useGroup int
	var prevKey darwinKeyEvent
	var prevKeyTime time.Time

	for {
		var ev darwinKeyEvent
		select {
		case <-ctx.Done():
			return
		case ev = <-inbox:
		}

		// Подавляем дубликаты подряд одинаковых событий
		now := time.Now()
		if prevKey.keycode == ev.keycode && prevKey.pressed == ev.pressed && now.Sub(prevKeyTime) < debounceMs*time.Millisecond {
			continue
		}
		prevKey = ev
		prevKeyTime = now

		if printMode {
			log.Printf("code:%v pressed:%v", ev.keycode, ev.pressed)
		}

		switch {
		case ev.pressed: // key down
			useGroup = 0
			if ev.keycode == leftCode {
				useGroup = 1
			} else if ev.keycode == rightCode {
				useGroup = 2
			}
		case !ev.pressed && useGroup == 0:
			// отпускание не-Ctrl, либо нажатие Ctrl не было зарегистрировано
			continue
		case ev.keycode == leftCode && useGroup == 1: // key up левого Ctrl
			playBeep(FreqEn)
			if err := switchLayout(0); err != nil {
				log.Printf("English: %v", err)
			} else {
				log.Println("→ English")
			}
			useGroup = 0
		case ev.keycode == rightCode && useGroup == 2: // key up правого Ctrl
			playBeep(FreqRu)
			if err := switchLayout(1); err != nil {
				log.Printf("Русский: %v", err)
			} else {
				log.Println("→ Русский")
			}
			useGroup = 0
		default:
			useGroup = 0
		}
	}
}

func installAutostart() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("не удалось определить HOME: %v", err)
	}
	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию LaunchAgents: %v", err)
	}

	plistPath := filepath.Join(launchAgentsDir, plistName)
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>LimitLoadToSessionType</key>
    <string>Aqua</string>
</dict>
</plist>
`, launchAgentLabel, exe)

	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("не удалось создать plist: %v", err)
	}

	fmt.Printf("Автозагрузка установлена: %s\n", plistPath)
	fmt.Println("Примените: launchctl load " + plistPath)
	return nil
}

func removeAutostart() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("не удалось определить HOME: %v", err)
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", plistName)
	err = os.Remove(plistPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("не удалось удалить plist: %v", err)
	}
	fmt.Printf("Автозагрузка убрана: %s\n", plistPath)
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
	fmt.Printf("LEFTCTRL=%d RIGHTCTRL=%d\n", leftCtrlCode, rightCtrlCode)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nПолучен сигнал завершения...")
		os.Exit(0)
	}()

	fmt.Println("Запущена в консоли (CGEventTap)")
	fmt.Println("Левый Ctrl -> Английский")
	fmt.Println("Правый Ctrl -> Русский")
	fmt.Println("Для остановки нажмите Ctrl+C")
	fmt.Println("Требуется разрешение: System Settings → Privacy & Security → Accessibility")

	listenKeyboards(ctx, leftCtrlCode, rightCtrlCode, true)
}

func printUsage() {
	fmt.Println("Используй команды: install, start, uninstall, stop, console")
	fmt.Println("По команде start остановится и запустится в фоновом режиме")
	fmt.Println("По команде install установится в автозагрузку ~/Library/LaunchAgents/ остановится и запустится в фоновом режиме")
	fmt.Println("По команде uninstall уберётся из автозагрузки и остановится")
	fmt.Println("Без команды или с неправильной командой остановится и запустится в фоновом режиме")
	fmt.Println("По команде console запустится в консоли для отладки")
}
