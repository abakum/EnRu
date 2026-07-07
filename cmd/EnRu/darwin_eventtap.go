//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

// Go callback, реализован ниже через //export
extern CGEventRef goEventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo);

// enru_start_event_tap создаёт event tap для перехвата клавиш и подключает его
// к текущему run loop. Возвращает 0 при неудаче
// (например, нет прав Accessibility или процесс не доверенный).
//
// Модификаторы (Ctrl) на macOS приходят как kCGEventFlagsChanged, а не как
// kCGEventKeyDown/kCGEventKeyUp, поэтому маска включает все три типа.
static int enru_start_event_tap(void) {
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown)
                     | CGEventMaskBit(kCGEventKeyUp)
                     | CGEventMaskBit(kCGEventFlagsChanged);
    CFMachPortRef tap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        goEventTapCallback,
        NULL);
    if (!tap) {
        return 0;
    }
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
    if (!src) {
        CFRelease(tap);
        return 0;
    }
    CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopCommonModes);
    return 1;
}

// enru_run_loop запускает текущий CFRunLoop (блокирует).
static void enru_run_loop(void) {
    CFRunLoopRun();
}

// enru_stop_loop останавливает текущий CFRunLoop.
static void enru_stop_loop(void) {
    CFRunLoopStop(CFRunLoopGetCurrent());
}
*/
import "C"

import (
	"context"
	"log"
	"sync"
	"unsafe"
)

// darwinKeyEvent — событие клавиши из CGEventTap.
type darwinKeyEvent struct {
	keycode uint32
	pressed bool // true = KeyDown, false = KeyUp
}

// darwinChan — глобальный канал для передачи событий из C-колбэка в Go.
var darwinChan chan<- darwinKeyEvent

// Состояние модификаторов: FlagsChanged приходит только при изменении состояния,
// поэтому press/release определяем переключением (toggle) по keycode.
var (
	modState   = make(map[uint32]bool)
	modStateMu sync.Mutex
)

func toggleModifier(keycode uint32) bool {
	modStateMu.Lock()
	defer modStateMu.Unlock()
	modState[keycode] = !modState[keycode]
	return modState[keycode]
}

// emitKey неблокирующе кладёт событие в канал, чтобы не задерживать run loop ввода.
func emitKey(keycode uint32, pressed bool) {
	if darwinChan == nil {
		return
	}
	select {
	case darwinChan <- darwinKeyEvent{keycode: keycode, pressed: pressed}:
	default:
	}
}

//export goEventTapCallback
func goEventTapCallback(proxy C.CGEventTapProxy, eventType C.CGEventType, event C.CGEventRef, userInfo unsafe.Pointer) C.CGEventRef {
	switch eventType {
	case C.kCGEventKeyDown:
		emitKey(uint32(C.CGEventGetIntegerValueField(event, C.kCGKeyboardEventKeycode)), true)
	case C.kCGEventKeyUp:
		emitKey(uint32(C.CGEventGetIntegerValueField(event, C.kCGKeyboardEventKeycode)), false)
	case C.kCGEventFlagsChanged:
		// Модификаторы на macOS приходят сюда. Нас интересуют только Ctrl.
		keycode := uint32(C.CGEventGetIntegerValueField(event, C.kCGKeyboardEventKeycode))
		if keycode == leftCtrlCode || keycode == rightCtrlCode {
			emitKey(keycode, toggleModifier(keycode))
		}
	}
	return event
}

// listenCGEventTap создаёт event tap и крутит run loop в этой горутине.
// При ошибке создания (нет прав Accessibility) логирует инструкцию и выходит.
func listenCGEventTap(ctx context.Context, inbox chan<- darwinKeyEvent) {
	darwinChan = inbox

	if C.enru_start_event_tap() == 0 {
		log.Println("CGEventTap: не удалось создать tap")
		log.Println("  Добавьте процесс в System Settings → Privacy & Security → Accessibility")
		log.Println("  и перезапустите программу")
		return
	}

	go func() {
		<-ctx.Done()
		C.enru_stop_loop()
	}()

	log.Println("CGEventTap: запущен")
	C.enru_run_loop()
	log.Println("CGEventTap: run loop завершился")
}
