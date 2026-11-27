package main

import "time"

const (
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	vkLeftCtrl   = 0xA2
	vkRightCtrl  = 0xA3
)

type ctrlDetector struct {
	held, candidate uint8
	last            uint8
	lastTime        time.Time
}

func (d *ctrlDetector) event(message uintptr, key uint32, now time.Time, releaseOnly bool) (int, string) {
	var bit uint8
	var side string
	switch key {
	case vkLeftCtrl:
		bit, side = 1, "левый"
	case vkRightCtrl:
		bit, side = 2, "правый"
	}
	switch message {
	case wmKeyDown, wmSysKeyDown:
		if bit == 0 {
			if d.candidate != 0 {
				d.candidate = 0
				return -1, "Ctrl: тап отменён другой клавишей"
			}
			return -1, ""
		}
		if d.held&bit == 0 {
			d.candidate = 0
			if d.held == 0 && message == wmKeyDown {
				d.candidate = bit
			}
			d.held |= bit
		}
		if message == wmSysKeyDown {
			return -1, side + " Ctrl: WM_SYSKEYDOWN"
		}
		return -1, side + " Ctrl: WM_KEYDOWN"
	case wmKeyUp, wmSysKeyUp:
		if bit == 0 {
			return -1, ""
		}
		d.held &^= bit
		standalone := d.candidate == bit
		d.candidate = 0
		if message == wmSysKeyUp {
			return -1, side + " Ctrl: WM_SYSKEYUP, пропуск"
		}
		if !releaseOnly && !standalone {
			return -1, side + " Ctrl: WM_KEYUP, не одиночный тап"
		}
		if d.last == bit && now.Sub(d.lastTime) < debounceMs*time.Millisecond {
			return -1, side + " Ctrl: WM_KEYUP, слишком частый повтор"
		}
		d.last, d.lastTime = bit, now
		return int(bit) - 1, side + " Ctrl: WM_KEYUP, принято"
	default:
		return -1, ""
	}
}
