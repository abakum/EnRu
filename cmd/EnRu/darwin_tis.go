//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Carbon -framework CoreFoundation
#include <Carbon/Carbon.h>
#include <dlfcn.h>
#include <stdlib.h>

typedef OSStatus (*enru_set_input_source_fn)(TISInputSourceRef);

// enru_resolve_setter находит приватный символ TISetCurrentKeyboardLayoutInputSource.
// Так как мы линкуем Carbon (HIToolbox), символ уже присутствует в адресном
// пространстве процесса и доступен через dlsym(RTLD_DEFAULT, ...).
// Дополнительно пробуем явно загрузить HIToolbox.
static enru_set_input_source_fn enru_resolve_setter(void) {
    enru_set_input_source_fn fn = (enru_set_input_source_fn)dlsym(RTLD_DEFAULT, "TISetCurrentKeyboardLayoutInputSource");
    if (!fn) {
        void *h = dlopen("/System/Library/Frameworks/Carbon.framework/Frameworks/HIToolbox.framework/HIToolbox", RTLD_LAZY);
        if (h) {
            fn = (enru_set_input_source_fn)dlsym(h, "TISetCurrentKeyboardLayoutInputSource");
        }
    }
    return fn;
}

// enru_select_input_source ищет среди включённых Input Source источник с заданным ID
// (например "com.apple.keylayout.US") и делает его текущим.
// Возвращает: 1 — найден и установлен; 0 — источник не найден;
// -1 — приватный setter недоступен; -2 — setter вернул ошибку.
static int enru_select_input_source(const char *sourceID) {
    enru_set_input_source_fn setter = enru_resolve_setter();
    if (!setter) {
        return -1;
    }

    // includeAllInstalled=false: только источники, добавленные пользователем
    CFArrayRef list = TISCreateInputSourceList(NULL, false);
    if (!list) {
        return 0;
    }

    CFStringRef target = CFStringCreateWithCString(kCFAllocatorDefault, sourceID, kCFStringEncodingUTF8);
    int result = 0;
    if (target) {
        CFIndex count = CFArrayGetCount(list);
        for (CFIndex i = 0; i < count; i++) {
            TISInputSourceRef src = (TISInputSourceRef)CFArrayGetValueAtIndex(list, i);
            if (!src) {
                continue;
            }
            CFStringRef idRef = (CFStringRef)TISGetInputSourceProperty(src, kTISPropertyInputSourceID);
            if (!idRef) {
                continue;
            }
            if (CFStringCompare(idRef, target, 0) == kCFCompareEqualTo) {
                OSStatus st = setter(src);
                result = (st == 0) ? 1 : -2;
                break;
            }
        }
        CFRelease(target);
    }
    CFRelease(list);
    return result;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// switchLayout переключает активный Input Source на macOS.
//   - group 0 → com.apple.keylayout.US (Английский)
//   - group 1 → com.apple.keylayout.Russian (Русский)
//
// Источник должен быть предварительно добавлен пользователем в
// System Settings → Keyboard → Input Sources.
func switchLayout(group uint) error {
	var sourceID string
	switch group {
	case 0:
		sourceID = "com.apple.keylayout.US"
	case 1:
		sourceID = "com.apple.keylayout.Russian"
	default:
		sourceID = "com.apple.keylayout.US"
	}

	cID := C.CString(sourceID)
	defer C.free(unsafe.Pointer(cID))

	switch C.enru_select_input_source(cID) {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("input source %s не найден: добавьте раскладку в System Settings → Keyboard → Input Sources", sourceID)
	case -1:
		return fmt.Errorf("TISetCurrentKeyboardLayoutInputSource недоступен в этой macOS")
	default:
		return fmt.Errorf("не удалось переключить на %s (setter вернул ошибку)", sourceID)
	}
}
