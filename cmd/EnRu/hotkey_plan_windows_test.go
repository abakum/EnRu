package main

import (
	"errors"
	"maps"
	"testing"
)

func TestPlanDirectHotkeys(t *testing.T) {
	// Первые два HKL имеют один LANGID, но разные раскладки.
	targets := []uint32{0x08090409, 0x04090409, 0x04190419, 4, 5, 6, 7, 8, 9, 10, 11}
	other := hotkey{ctrlDigits, 'G', 0}
	saved := map[uint32]hotkey{0x200: other, firstDirect: {ctrlDigits, '1', 11}}
	for _, n := range []int{2, 9, 10, 11} {
		want, err := planHotkeys(saved, targets[:n])
		if err != nil {
			t.Fatal(err)
		}
		if want[0x200] != other || len(want) != min(n, 10)+1 {
			t.Fatalf("wrong plan for %d layouts: %v", n, want)
		}
		for i, target := range targets[:min(n, 10)] {
			if want[firstDirect+uint32(i)] != (hotkey{ctrlDigits, uint32("1234567890"[i]), target}) {
				t.Fatalf("wrong binding at position %d: %v", i+1, want)
			}
		}
		if again, err := planHotkeys(want, targets[:n]); err != nil || !maps.Equal(again, want) {
			t.Fatalf("plan is not idempotent: %v (%v)", again, err)
		}
	}
	if _, err := planHotkeys(saved, targets[:1]); err == nil {
		t.Fatal("accepted a single layout")
	}
	if _, err := planHotkeys(map[uint32]hotkey{0x200: {ctrlDigits, '2', 0}}, targets[:2]); err == nil {
		t.Fatal("accepted a conflict with a non-layout hotkey")
	}
	saved[firstDirect+1] = hotkey{0xc001, 'K', 12}
	if removed, err := planHotkeys(saved, nil); err != nil || !maps.Equal(removed, map[uint32]hotkey{0x200: other}) {
		t.Fatalf("uninstall failed to remove all direct hotkeys: %v (%v)", removed, err)
	}
}

func TestDirectHotkeysRollback(t *testing.T) {
	before := map[uint32]hotkey{firstDirect: {ctrlDigits, '1', 1}, firstDirect + 1: {ctrlDigits, '2', 2}}
	failure := errors.New("test failure")
	for _, tc := range []struct {
		name    string
		targets []uint32
		failAt  int
	}{
		{"перестановка", []uint32{2, 1}, 4},
		{"очистка", nil, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := planHotkeys(before, tc.targets)
			if err != nil {
				t.Fatal(err)
			}
			live := maps.Clone(before)
			calls := 0
			err = updateHotkeys(before, want, func(slot uint32, b hotkey) error {
				calls++
				if calls == tc.failAt {
					return failure
				}
				if b == (hotkey{}) {
					delete(live, slot)
				} else {
					live[slot] = b
				}
				return nil
			}, func() (map[uint32]hotkey, error) { return maps.Clone(live), nil })
			if !errors.Is(err, failure) || !maps.Equal(live, before) {
				t.Fatalf("rollback did not restore original bindings: %v (%v)", live, err)
			}
		})
	}
}
