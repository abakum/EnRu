//go:build linux

package main

/*
#cgo LDFLAGS: -lX11 -lXtst
#include <X11/Xlib.h>
#include <X11/extensions/record.h>

// Go callback, implemented in Go below
extern void xrecordKeyCallback(int keycode, int pressed);

// C callback for XRecord, calls Go callback
static void xrecord_intercept_cb(XPointer closure, XRecordInterceptData *data) {
	if (data->category == XRecordFromServer) {
		unsigned char *ptr = data->data;
		int type = ptr[0] & 0x7F;
		int keycode = ptr[1];
		if (type == KeyPress || type == KeyRelease) {
			xrecordKeyCallback(keycode, type == KeyPress ? 1 : 0);
		}
	}
	XRecordFreeData(data);
}

// Wrapper to create record context (handles XRecordAllClients internally)
static XRecordContext create_xrecord_context(Display *ctrl, XRecordRange *range) {
	XRecordClientSpec client = XRecordAllClients;
	return XRecordCreateContext(ctrl, 0, &client, 1, &range, 1);
}

// Wrapper to enable record context (blocking call)
static int enable_xrecord(Display *data, XRecordContext ctx) {
	return XRecordEnableContext(data, ctx, xrecord_intercept_cb, NULL);
}
*/
import "C"

import (
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/grafov/evdev"
)

// Глобальный канал для передачи событий из C-колбэка в Go
var x11Chan chan<- message

//export xrecordKeyCallback
func xrecordKeyCallback(keycode C.int, pressed C.int) {
	if x11Chan == nil {
		return
	}
	// X keycode = evdev code + 8
	evdevCode := uint16(keycode - 8)
	var val int32
	if pressed != 0 {
		val = 1
	}
	x11Chan <- message{
		Events: []evdev.InputEvent{
			{
				Type:  evdev.EV_KEY,
				Code:  evdevCode,
				Value: val,
			},
		},
	}
}

// listenX11Record запускает XRecord для перехвата X11-событий клавиатуры (включая VNC/XTest).
// Блокируется, работает в отдельной горутине.
func listenX11Record(inbox chan<- message) {
	if os.Getenv("DISPLAY") == "" {
		log.Println("X11: DISPLAY не задан, XRecord не запущен")
		return
	}

	// Два соединения: control (создание контекста) и data (чтение событий)
	ctrlDisplay := C.XOpenDisplay(nil)
	if ctrlDisplay == nil {
		log.Println("X11: не удалось открыть control display")
		return
	}
	defer C.XCloseDisplay(ctrlDisplay)

	dataDisplay := C.XOpenDisplay(nil)
	if dataDisplay == nil {
		log.Println("X11: не удалось открыть data display")
		return
	}
	defer C.XCloseDisplay(dataDisplay)

	// Проверяем поддержку XRecord
	var major, minor C.int
	if C.XRecordQueryVersion(ctrlDisplay, &major, &minor) == 0 {
		log.Println("X11: XRecord extension не поддерживается")
		return
	}

	// Создаём диапазон перехватываемых событий — только клавиатура
	recordRange := C.XRecordAllocRange()
	if recordRange == nil {
		log.Println("X11: не удалось создать XRecordRange")
		return
	}
	defer C.XFree(unsafe.Pointer(recordRange))
	recordRange.device_events.first = C.KeyPress
	recordRange.device_events.last = C.KeyRelease

	// Создаём контекст записи — перехват от всех клиентов
	ctx := C.create_xrecord_context(ctrlDisplay, recordRange)
	if ctx == 0 {
		log.Println("X11: не удалось создать XRecordContext")
		return
	}
	defer C.XRecordFreeContext(ctrlDisplay, ctx)

	// Важно: сбрасываем буфер control-соединения, чтобы сервер
	// успел зарегистрировать контекст до XRecordEnableContext
	C.XSync(ctrlDisplay, 0)

	x11Chan = inbox
	log.Printf("X11: XRecord запущен (version %d.%d)", int(major), int(minor))

	ret := C.enable_xrecord(dataDisplay, ctx)
	if ret == 0 {
		log.Println("X11: XRecord завершился с ошибкой, перезапуск через 5 сек...")
		time.Sleep(5 * time.Second)
		go listenX11Record(inbox)
	} else {
		log.Println("X11: XRecord завершился")
	}
}
