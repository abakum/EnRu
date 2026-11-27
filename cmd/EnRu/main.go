package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	version "github.com/abakum/version/lib"
	"github.com/mitchellh/go-ps"
)

var _ = version.Ver

//go:generate go run github.com/abakum/version

//go:embed VERSION
var VERSION string

const (
	EnRu         = "EnRu"
	FreqRu       = 523  // C5 (До)
	FreqEn       = 1046 // C6 (До на октаву выше)
	BeepDuration = 100 * time.Millisecond
	Description  = "EnRu Keyboard Layout Switcher"
	debounceMs   = 150
)

var (
	exe string
	err error
)

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
