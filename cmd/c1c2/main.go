//go:build windows

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	version "github.com/abakum/version/lib"
	"github.com/jxeng/shortcut"
	ps "github.com/mitchellh/go-ps"
	"golang.org/x/sys/windows"
)

var _ = version.Ver

//go:generate go run github.com/abakum/version

//go:embed VERSION
var VERSION string

const description = "c1c2 Keyboard Layout Switcher"
const hotkeyDialog = "input.dll,,{C07337D3-DB2C-4D0B-9A93-B722A6C106E2}{HOTKEYS}"

func main() {
	args := os.Args[1:]
	err := run(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if len(args) > 0 && (args[0] == "install" || args[0] == "uninstall") {
			printManualSetup(args[0] == "install")
		}
	}
	if len(args) == 0 || (args[0] != "console" && args[0] != "--background" && args[0] != "--startup") {
		pauseConsole()
	}
	if err != nil {
		os.Exit(1)
	}
}

func pauseConsole() {
	input, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	var mode uint32
	if err != nil || windows.GetConsoleMode(input, &mode) != nil {
		return
	}
	readInput := windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")
	var record struct { // Win32 INPUT_RECORD, with only KEY_EVENT fields used.
		eventType uint16
		_         uint16
		keyDown   uint32
		_         [12]byte
	}
	var read uint32
	fmt.Print("Press Any Key to continue . . .")
	for {
		ok, _, _ := readInput.Call(uintptr(input), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&read)))
		if ok == 0 || (read == 1 && record.eventType == 1 && record.keyDown != 0) {
			break
		}
	}
	fmt.Println()
}

func run(args []string) error {
	var command string
	if len(args) > 0 {
		command = args[0]
	}
	if command == "--background" {
		return runSwitcher(false)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	major, minor, build := windows.RtlGetNtVersionNumbers()
	fmt.Printf("%s %s %s Built with %s, Windows %d.%d.%d on %s\n",
		description, exe, strings.TrimSpace(VERSION), runtime.Version(), major, minor, build, runtime.GOARCH)

	var applyHotkeySettings func() error
	if command == "install" || command == "uninstall" {
		printSavedDirectHotkeys("Before")
		defer printSavedDirectHotkeys("After")
		applyHotkeySettings, err = prepareHotkeys(command == "install")
		if err != nil {
			return fmt.Errorf("cannot prepare layout shortcuts: %w", err)
		}
	}
	if command == "install" {
		if err := applyHotkeySettings(); err != nil {
			return fmt.Errorf("cannot install layout shortcuts: %w", err)
		}
		if err := configureAutostart(exe, true); err != nil {
			return fmt.Errorf("layout shortcuts are already applied, but Startup setup failed: %w", err)
		}
	}
	if err := stopTask(filepath.Base(exe)); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) && !windows.GetCurrentProcessToken().IsElevated() {
			return fmt.Errorf("cannot stop the existing instance; run c1c2 as administrator: %w", err)
		}
		return err
	}
	switch command {
	case "uninstall":
		if err := configureAutostart(exe, false); err != nil {
			return err
		}
		if err := applyHotkeySettings(); err != nil {
			return fmt.Errorf("c1c2 stopped and Startup shortcut removed, but shortcut cleanup failed: %w", err)
		}
		return openHotkeySettings("Uninstall")
	case "stop":
		return nil
	case "console":
		return runSwitcher(true)
	}

	if command != "install" {
		fmt.Println("Commands: install, start, uninstall, stop, console")
		fmt.Println("install assigns layout shortcuts Ctrl+1 through Ctrl+9, then Ctrl+0, adds a Startup shortcut, and starts c1c2.")
		fmt.Println("uninstall stops c1c2 and removes the Startup shortcut and all direct layout shortcuts, including manually assigned ones.")
		fmt.Println("start (or no command) starts c1c2 in the background; stop ends it; console runs it in the foreground.")
	}
	cmd := exec.Command(exe, "--background")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, NoInheritHandles: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start in the background: %w", err)
	}
	defer cmd.Process.Release()
	fmt.Println("Started in the background.")
	if command == "install" {
		return openHotkeySettings("Install")
	}
	return nil
}

func openHotkeySettings(operation string) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("%s completed, but cannot locate Advanced Key Settings launcher: %w", operation, err)
	}
	cmd := exec.Command(filepath.Join(dir, "rundll32.exe"), "shell32.dll,Control_RunDLL", hotkeyDialog)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, NoInheritHandles: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s completed, but cannot open Advanced Key Settings: %w", operation, err)
	}
	_ = cmd.Process.Release()
	return nil
}

func stopTask(name string) error {
	processes, err := ps.Processes()
	if err != nil {
		return fmt.Errorf("cannot list processes: %w", err)
	}
	found := false
	for _, p := range processes {
		if p == nil || p.Pid() == os.Getpid() || !strings.EqualFold(p.Executable(), name) {
			continue
		}
		proc, err := os.FindProcess(p.Pid())
		if err != nil {
			return fmt.Errorf("cannot find process %d: %w", p.Pid(), err)
		}
		err = proc.Kill()
		proc.Release()
		if err != nil {
			return fmt.Errorf("cannot stop process %d: %w", p.Pid(), err)
		}
		found = true
		fmt.Printf("Stopped process %d.\n", p.Pid())
	}
	if !found {
		fmt.Println("No other c1c2 instance is running.")
	}
	return nil
}

func configureAutostart(exe string, install bool) error {
	dir := os.Getenv("APPDATA")
	if dir == "" {
		return fmt.Errorf("cannot locate the Startup folder: APPDATA is not set")
	}
	path := filepath.Join(dir, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "c1c2.lnk")
	if install {
		sc := shortcut.Shortcut{
			ShortcutPath: path,
			Target:       exe,
			Arguments:    "--startup",
			Description:  description,
			IconLocation: exe,
			WindowStyle:  "7",
		}
		if err := shortcut.Create(sc); err != nil {
			return fmt.Errorf("cannot create Startup shortcut: %w", err)
		}
		fmt.Printf("Installed Startup shortcut: %s\n", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove Startup shortcut: %w", err)
	}
	fmt.Printf("Startup shortcut removed: %s\n", path)
	return nil
}

func printManualSetup(install bool) {
	fmt.Println("To change layout shortcuts manually, open Advanced Key Settings from Command Prompt:")
	fmt.Println("  rundll32.exe shell32.dll,Control_RunDLL " + hotkeyDialog)
	if !install {
		fmt.Println("For each assigned layout: Change Key Sequence > clear Enable Key Sequence > OK. Skip 'Between input languages'. Then click Apply.")
		return
	}
	fmt.Println("Select an input language > Change Key Sequence > Enable Key Sequence > assign Ctrl+1 > OK.")
	fmt.Println("Repeat for another input language with Ctrl+2, then click Apply. Skip 'Between input languages'.")
}
