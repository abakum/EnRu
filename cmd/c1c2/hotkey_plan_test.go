//go:build windows

package main

import (
	"errors"
	"maps"
	"strings"
	"testing"
)

func TestPlanHotkeys(t *testing.T) {
	targets := []uint32{0x08090409, 0x04090409, 0x04190419, 4, 5, 6, 7, 8, 9, 10, 11}
	other := hotkey{ctrlDigits, 'G', 0}
	saved := map[uint32]hotkey{0x200: other, 0x100: {ctrlDigits, '1', 11}}
	for _, n := range []int{2, 9, 10, 11} {
		want, err := planHotkeys(saved, targets[:n])
		if err != nil {
			t.Fatal(err)
		}
		if want[0x200] != other || len(want) != min(n, 10)+1 {
			t.Fatalf("wrong result for %d layouts: %v", n, want)
		}
		for i, target := range targets[:min(n, 10)] {
			if want[firstDirect+uint32(i)] != (hotkey{ctrlDigits, uint32("1234567890"[i]), target}) {
				t.Fatalf("wrong binding at position %d: %v", i+1, want)
			}
		}
		again, err := planHotkeys(want, targets[:n])
		if err != nil || !maps.Equal(again, want) {
			t.Fatalf("install is not idempotent: %v (%v)", again, err)
		}
		removed, err := planHotkeys(want, nil)
		if err != nil || !maps.Equal(removed, map[uint32]hotkey{0x200: other}) {
			t.Fatalf("uninstall result: %v (%v)", removed, err)
		}
	}
	if _, err := planHotkeys(saved, targets[:1]); err == nil {
		t.Fatal("accepted a single layout")
	}
}

func TestHotkeysRollback(t *testing.T) {
	before := map[uint32]hotkey{0x100: {ctrlDigits, '1', 1}, 0x101: {ctrlDigits, '2', 2}}
	for _, targets := range [][]uint32{{2, 1}, nil} {
		want, err := planHotkeys(before, targets)
		if err != nil {
			t.Fatal(err)
		}
		live := maps.Clone(before)
		calls := 0
		err = updateHotkeys(before, want, func(slot uint32, b hotkey) error {
			calls++
			if calls == 2 {
				return errors.New("test failure")
			}
			if b == (hotkey{}) {
				delete(live, slot)
			} else {
				live[slot] = b
			}
			return nil
		}, func() (map[uint32]hotkey, error) { return maps.Clone(live), nil })
		if err == nil || !strings.Contains(err.Error(), "original hotkeys restored") || !maps.Equal(live, before) {
			t.Fatalf("rollback result: %v (%v)", live, err)
		}
	}
}
