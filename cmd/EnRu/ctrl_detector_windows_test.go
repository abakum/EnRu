package main

import (
	"testing"
	"time"
)

func TestCtrlDetector(t *testing.T) {
	type event struct {
		message uintptr
		key     uint32
		want    int
	}
	leftDown, leftUp := event{wmKeyDown, vkLeftCtrl, -1}, event{wmKeyUp, vkLeftCtrl, 0}
	rightDown, rightUp := event{wmKeyDown, vkRightCtrl, -1}, event{wmKeyUp, vkRightCtrl, 1}
	ignoreLeft, ignoreRight := event{wmKeyUp, vkLeftCtrl, -1}, event{wmKeyUp, vkRightCtrl, -1}
	for _, tt := range []struct {
		name    string
		release bool
		events  []event
	}{
		{"both sides", false, []event{leftDown, leftUp, rightDown, rightUp}},
		{"auto repeat", false, []event{leftDown, leftDown, leftUp}},
		{"Ctrl+C", false, []event{leftDown, {wmKeyDown, 'C', -1}, {wmKeyUp, 'C', -1}, ignoreLeft}},
		{"repeat after cancellation", false, []event{leftDown, {wmKeyDown, 'C', -1}, leftDown, ignoreLeft}},
		{"two Ctrl keys", false, []event{leftDown, rightDown, ignoreRight, ignoreLeft}},
		{"system Ctrl", false, []event{{wmSysKeyDown, vkLeftCtrl, -1}, {wmSysKeyUp, vkLeftCtrl, -1}, leftDown, leftUp}},
		{"system release cancels tap", false, []event{leftDown, {wmSysKeyUp, vkLeftCtrl, -1}, ignoreLeft}},
		{"debounce", false, []event{leftDown, leftUp, leftDown, ignoreLeft}},
		{"missing keydown", false, []event{ignoreLeft, ignoreRight}},
		{"release control build", true, []event{leftUp, rightUp}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var d ctrlDetector
			for i, ev := range tt.events {
				got, _ := d.event(ev.message, ev.key, time.Unix(1, 0).Add(time.Duration(i)*time.Millisecond), tt.release)
				if got != ev.want {
					t.Fatalf("event %d: got %d, want %d", i, got, ev.want)
				}
			}
		})
	}
}
