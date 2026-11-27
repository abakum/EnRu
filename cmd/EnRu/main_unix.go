//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	exe = resolveExe()

	printVersionInfo()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := installAutostart(); err != nil {
				fmt.Printf("Не удалось установить автозагрузку: %v\n", err)
			}
			fallthrough
		case "start":
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
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			startConsole(ctx)
		}
	}

	printUsage()
	startBackground()

	fmt.Println("Запущена в фоновом режиме")
	fmt.Println("Левый Ctrl -> Английский")
	fmt.Println("Правый Ctrl -> Русский")
}
