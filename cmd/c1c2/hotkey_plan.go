//go:build windows

package main

import (
	"errors"
	"fmt"
	"maps"
)

const (
	firstDirect = 0x100
	lastDirect  = 0x11f
	ctrlDigits  = 0xc002
)

type hotkey struct{ modifiers, key, target uint32 }

func directSlot(slot uint32) bool { return slot >= firstDirect && slot <= lastDirect }

func layoutDigit(index int) uint32 {
	if index >= 10 {
		return 0
	}
	return uint32('0' + (index+1)%10)
}

func planHotkeys(saved map[uint32]hotkey, targets []uint32) (map[uint32]hotkey, error) {
	want := maps.Clone(saved)
	if targets == nil { // uninstall
		for slot := range want {
			if directSlot(slot) {
				delete(want, slot)
			}
		}
		return want, nil
	}
	if len(targets) < 2 {
		return nil, fmt.Errorf("at least two keyboard layouts are required")
	}
	byTarget := make(map[uint32]uint32)
	digits := make(map[uint32]bool)
	for i, target := range targets {
		if _, exists := byTarget[target]; target == 0 || exists {
			return nil, fmt.Errorf("invalid or duplicate layout target 0x%08X", target)
		}
		digit := layoutDigit(i)
		byTarget[target] = digit
		if digit != 0 {
			digits[digit] = true
		}
	}
	kept := make(map[uint32]bool)
	for slot, entry := range saved {
		if !directSlot(slot) && digits[entry.key] && (entry.modifiers&0x400 != 0 || entry.modifiers&0xf == 2) {
			return nil, fmt.Errorf("Ctrl+%c conflicts with non-layout hotkey 0x%X", entry.key, slot)
		}
	}
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		entry, exists := saved[slot]
		if !exists {
			continue
		}
		if digit, managed := byTarget[entry.target]; managed {
			if digit != 0 && entry.key == digit && entry.modifiers == ctrlDigits && !kept[entry.target] {
				kept[entry.target] = true
			} else {
				delete(want, slot)
			}
		} else if digits[entry.key] && (entry.modifiers&0x400 != 0 || entry.modifiers&0xf == 2) {
			delete(want, slot)
		}
	}
	for _, target := range targets {
		if digit := byTarget[target]; digit != 0 && !kept[target] {
			slot := uint32(firstDirect)
			for ; slot <= lastDirect; slot++ {
				if _, used := want[slot]; !used {
					break
				}
			}
			if slot > lastDirect {
				return nil, fmt.Errorf("no free direct-switch hotkey slot")
			}
			want[slot] = hotkey{ctrlDigits, digit, target}
		}
	}
	return want, nil
}

// Clear changed slots before setting new ones, including when two digits swap.
func applyHotkeys(from, to map[uint32]hotkey, set func(uint32, hotkey) error) error {
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		if from[slot] != (hotkey{}) && from[slot] != to[slot] {
			if err := set(slot, hotkey{}); err != nil {
				return err
			}
		}
	}
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		if to[slot] != (hotkey{}) && from[slot] != to[slot] {
			if err := set(slot, to[slot]); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateHotkeys(before, want map[uint32]hotkey, set func(uint32, hotkey) error, read func() (map[uint32]hotkey, error)) error {
	err := applyHotkeys(before, want, set)
	current, readErr := read()
	if err == nil && readErr == nil && maps.Equal(current, want) {
		return nil
	}
	if err == nil {
		err = errors.New("hotkeys did not match the expected result")
	}
	if readErr != nil {
		return fmt.Errorf("%w; cannot safely roll back without a consistent snapshot: %v", err, readErr)
	}
	if maps.Equal(current, before) {
		return fmt.Errorf("%w; original settings are intact", err)
	}
	for slot, entry := range current {
		if (!directSlot(slot) && entry != before[slot]) || (entry != before[slot] && entry != want[slot]) {
			return fmt.Errorf("%w; unexpected change at slot 0x%X, rollback not attempted", err, slot)
		}
	}
	for slot, entry := range before {
		if (!directSlot(slot) || entry == want[slot]) && current[slot] != entry {
			return fmt.Errorf("%w; unrelated slot 0x%X changed, rollback not attempted", err, slot)
		}
	}
	if rollback := applyHotkeys(current, before, set); rollback != nil {
		return fmt.Errorf("%w; rollback failed: %v", err, rollback)
	}
	if restored, verifyErr := read(); verifyErr != nil || !maps.Equal(restored, before) {
		return fmt.Errorf("%w; rollback could not be verified (read error: %v)", err, verifyErr)
	}
	return fmt.Errorf("%w; original hotkeys restored", err)
}
