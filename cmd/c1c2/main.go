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

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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

	if command == "install" || command == "uninstall" {
		if err := configureAutostart(exe, command == "install"); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if err := stopTask(filepath.Base(exe)); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) && !windows.GetCurrentProcessToken().IsElevated() {
			return fmt.Errorf("cannot stop the existing instance; run c1c2 as administrator: %w", err)
		}
		return err
	}
	switch command {
	case "uninstall", "stop":
		return nil
	case "console":
		return runSwitcher(true)
	}

	fmt.Println("Commands: install, start, uninstall, stop, console")
	fmt.Println("install adds a Startup shortcut and starts c1c2; uninstall removes it and stops c1c2.")
	fmt.Println("start (or no command) starts c1c2 in the background; stop ends it; console runs it in the foreground.")
	printSetup()
	cmd := exec.Command(exe, "--background")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, NoInheritHandles: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start in the background: %w", err)
	}
	defer cmd.Process.Release()
	fmt.Println("Started in the background.")
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
			Arguments:    "start",
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

func printSetup() {
	fmt.Println("Windows setup: open Advanced Key Settings from Command Prompt:")
	fmt.Println("  rundll32.exe shell32.dll,Control_RunDLL input.dll,,{HOTKEYS}")
	fmt.Println("Select an input language > Change Key Sequence > Enable Key Sequence > assign Ctrl+1 > OK.")
	fmt.Println("Repeat for another input language with Ctrl+2, then click Apply. Skip 'Between input languages'.")
	fmt.Println("Tap left Ctrl to send Ctrl+1 or right Ctrl to send Ctrl+2.")
}
