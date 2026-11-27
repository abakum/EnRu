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
	want := make(map[uint32]hotkey, len(saved))
	maps.Copy(want, saved)
	if targets == nil {
		for slot := range want {
			if directSlot(slot) {
				delete(want, slot)
			}
		}
		return want, nil
	}
	if len(targets) < 2 {
		return nil, errors.New("нужны как минимум две раскладки клавиатуры")
	}
	byTarget := make(map[uint32]uint32)
	digits := make(map[uint32]bool)
	for i, target := range targets {
		if _, exists := byTarget[target]; target == 0 || exists {
			return nil, fmt.Errorf("недопустимая или повторная цель раскладки 0x%08X", target)
		}
		digit := layoutDigit(i)
		byTarget[target] = digit
		if digit != 0 {
			digits[digit] = true
		}
	}
	conflicts := func(entry hotkey) bool {
		return digits[entry.key] && (entry.modifiers&0x400 != 0 || entry.modifiers&0xf == 2)
	}
	for slot, entry := range saved {
		if !directSlot(slot) && conflicts(entry) {
			return nil, fmt.Errorf("Ctrl+%c конфликтует со сторонним сочетанием в слоте 0x%X", entry.key, slot)
		}
	}
	kept := make(map[uint32]bool)
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
		} else if conflicts(entry) {
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
				return nil, errors.New("нет свободного слота для прямого переключения раскладки")
			}
			want[slot] = hotkey{ctrlDigits, digit, target}
		}
	}
	return want, nil
}

// Сначала освобождаем изменяемые слоты, в том числе при обмене цифрами.
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
	attempted := make(map[uint32]hotkey)
	err := applyHotkeys(before, want, func(slot uint32, b hotkey) error {
		attempted[slot] = b
		return set(slot, b)
	})
	current, readErr := read()
	if err == nil && readErr == nil && maps.Equal(current, want) {
		return nil
	}
	if err == nil {
		err = errors.New("сочетания не совпали с ожидаемым результатом")
	}
	if readErr != nil {
		return fmt.Errorf("%w; безопасный откат невозможен: %v", err, readErr)
	}
	if maps.Equal(current, before) {
		return fmt.Errorf("%w; исходные настройки не изменились", err)
	}
	for slot, entry := range current {
		if last, tried := attempted[slot]; entry != before[slot] && (!tried || entry != last) {
			return fmt.Errorf("%w; неожидаемое изменение в слоте 0x%X, откат не выполнялся", err, slot)
		}
	}
	for slot, entry := range before {
		if _, tried := attempted[slot]; !tried && current[slot] != entry {
			return fmt.Errorf("%w; посторонний слот 0x%X изменился, откат не выполнялся", err, slot)
		}
	}
	if rollback := applyHotkeys(current, before, set); rollback != nil {
		return fmt.Errorf("%w; ошибка отката: %v", err, rollback)
	}
	restored, verifyErr := read()
	if verifyErr != nil {
		return fmt.Errorf("%w; не удалось проверить откат: %v", err, verifyErr)
	}
	if !maps.Equal(restored, before) {
		return fmt.Errorf("%w; после отката сочетания не совпали с исходными", err)
	}
	return fmt.Errorf("%w; исходные сочетания восстановлены", err)
}
