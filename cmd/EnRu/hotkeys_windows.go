package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	hotkeyDLL      = windows.NewLazySystemDLL("imm32.dll")
	immGetHotkey   = hotkeyDLL.NewProc("ImmGetHotKey")
	immSetHotkey   = hotkeyDLL.NewProc("ImmSetHotKey")
	layoutList     = user32.NewProc("GetKeyboardLayoutList")
	activateLayout = user32.NewProc("ActivateKeyboardLayout")
	layoutName     = user32.NewProc("GetKeyboardLayoutNameW")
	getLocaleInfo  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetLocaleInfoW")
)

func profileLayouts(limit int) (firstLanguage string, tips []string, err error) {
	root, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\International\User Profile`, registry.READ)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось открыть языковой профиль Windows: %w", err)
	}
	defer root.Close()
	size, kind, err := root.GetValue("Languages", nil)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось прочитать Languages: %w", err)
	}
	if kind != registry.MULTI_SZ || size < 2 {
		return "", nil, errors.New("Languages не содержит корректный REG_MULTI_SZ")
	}
	data := make([]byte, size)
	n, kind, err := root.GetValue("Languages", data)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось прочитать Languages: %w", err)
	}
	if kind != registry.MULTI_SZ || n < 2 || n > len(data) {
		return "", nil, errors.New("Languages не содержит корректный REG_MULTI_SZ")
	}
	languages, err := decodeProfileLanguages(data[:n])
	if err != nil {
		return "", nil, err
	}
	if len(languages) == 0 {
		return "", nil, errors.New("список Languages в профиле Windows пуст")
	}
	firstLanguage = languages[0]
	seenLanguages, seenTips := make(map[string]bool), make(map[string]bool)
	for _, lang := range languages {
		folded := strings.ToLower(lang)
		if seenLanguages[folded] {
			return firstLanguage, nil, fmt.Errorf("повторный язык %q в профиле Windows", lang)
		}
		seenLanguages[folded] = true
		remaining := 0
		if limit > 0 {
			remaining = limit - len(tips)
		}
		entries, err := languageLayouts(root, lang, remaining)
		if err != nil {
			return firstLanguage, nil, fmt.Errorf("не удалось прочитать раскладки для %s: %w", lang, err)
		}
		for _, tip := range entries {
			if seenTips[tip] {
				return firstLanguage, nil, fmt.Errorf("повторный метод ввода %q в профиле Windows", tip)
			}
			seenTips[tip] = true
			tips = append(tips, tip)
		}
		if limit > 0 && len(tips) == limit {
			break
		}
	}
	if len(tips) < 2 {
		return firstLanguage, nil, fmt.Errorf("нужны как минимум две раскладки клавиатуры; в профиле Windows найдено %d (языки: %q; методы ввода: %q)", len(tips), languages, tips)
	}
	return firstLanguage, tips, nil
}

func decodeProfileLanguages(data []byte) ([]string, error) {
	if len(data)%2 != 0 {
		return nil, errors.New("Languages содержит неполные данные UTF-16")
	}
	// Сохраняем последний тег даже без завершающего дополнительного NUL.
	var languages []string
	var tag []uint16
	for len(data) != 0 {
		unit := binary.LittleEndian.Uint16(data)
		data = data[2:]
		if unit != 0 {
			tag = append(tag, unit)
		} else if len(tag) != 0 {
			languages = append(languages, windows.UTF16ToString(tag))
			tag = tag[:0]
		} else {
			for _, b := range data {
				if b != 0 {
					return nil, errors.New("Languages содержит неожиданный пустой элемент")
				}
			}
			break
		}
	}
	if len(tag) != 0 {
		languages = append(languages, windows.UTF16ToString(tag))
	}
	return languages, nil
}

func languageLayouts(root registry.Key, lang string, limit int) ([]string, error) {
	k, err := registry.OpenKey(root, lang, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	positions := make(map[uint64][]string)
	count := 0
	for _, name := range names {
		if !strings.Contains(name, ":") {
			continue
		}
		pos, kind, err := k.GetIntegerValue(name)
		if err != nil || kind != registry.DWORD || pos == 0 {
			return nil, fmt.Errorf("не удалось определить позицию метода ввода %q", name)
		}
		positions[pos] = append(positions[pos], name)
		count++
	}
	if count == 0 {
		return nil, errors.New("нет распознаваемых раскладок клавиатуры")
	}
	if limit > 0 {
		count = min(count, limit)
	}
	ordered := make([]string, count)
	for i := range ordered {
		names := positions[uint64(i+1)]
		if len(names) != 1 {
			return nil, fmt.Errorf("позиция раскладки %d отсутствует или неоднозначна", i+1)
		}
		name := names[0]
		if len(name) != 13 || name[4] != ':' {
			return nil, fmt.Errorf("неподдерживаемый метод ввода %q", name)
		}
		if _, err := strconv.ParseUint(name[:4]+name[5:], 16, 48); err != nil || name[5] == 'e' || name[5] == 'E' {
			// У старых IME, в отличие от обычных раскладок, KLID начинается с E.
			return nil, fmt.Errorf("неподдерживаемый метод ввода %q", name)
		}
		ordered[i] = strings.ToLower(name)
	}
	return ordered, nil
}

func loadedLayouts(tips []string) (_ []uint32, _ map[uint32]uintptr, err error) {
	n, _, _ := layoutList.Call(0, 0)
	if n == 0 || n > 65536 {
		return nil, nil, errors.New("не удалось получить список загруженных раскладок")
	}
	layouts := make([]uintptr, int(n)+1)
	if count, _, _ := layoutList.Call(uintptr(len(layouts)), uintptr(unsafe.Pointer(&layouts[0]))); count != n {
		return nil, nil, errors.New("список загруженных раскладок изменился при чтении")
	}
	// GetKeyboardLayoutNameW читает активную раскладку текущего OS-потока.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var previous uintptr
	defer func() {
		if previous != 0 {
			if ok, _, callErr := activateLayout.Call(previous, 0); ok == 0 {
				err = errors.Join(err, fmt.Errorf("не удалось восстановить раскладку текущего потока: %v", callErr))
			} else if active, _, _ := procGetKeyboardLayout.Call(0); active != previous {
				err = errors.Join(err, errors.New("не удалось подтвердить восстановление раскладки текущего потока"))
			}
		}
	}()
	byTIP := make(map[string]uintptr)
	for _, hkl := range layouts[:n] {
		if hkl <= 1 {
			continue
		}
		prefix := fmt.Sprintf("%04x:", uint16(hkl))
		if !slices.ContainsFunc(tips, func(tip string) bool { return strings.HasPrefix(tip, prefix) }) {
			continue
		}
		old, _, callErr := activateLayout.Call(hkl, 0)
		if old == 0 {
			return nil, nil, fmt.Errorf("не удалось активировать HKL 0x%X для проверки: %v", hkl, callErr)
		}
		if previous == 0 {
			previous = old
		}
		if active, _, _ := procGetKeyboardLayout.Call(0); active != hkl {
			return nil, nil, fmt.Errorf("не удалось подтвердить активную раскладку HKL 0x%X", hkl)
		}
		var name [9]uint16
		if ok, _, _ := layoutName.Call(uintptr(unsafe.Pointer(&name[0]))); ok == 0 {
			return nil, nil, fmt.Errorf("не удалось определить имя загруженной раскладки HKL 0x%X", hkl)
		}
		tip := prefix + strings.ToLower(windows.UTF16ToString(name[:]))
		if !slices.Contains(tips, tip) {
			continue
		}
		if old := byTIP[tip]; old != 0 && old != hkl {
			return nil, nil, fmt.Errorf("несколько загруженных раскладок соответствуют %s", tip)
		}
		byTIP[tip] = hkl
	}
	targets := make([]uint32, 0, len(tips))
	handles := make(map[uint32]uintptr, len(tips))
	for _, tip := range tips {
		hkl := byTIP[tip]
		if hkl == 0 {
			return nil, nil, fmt.Errorf("нет загруженной раскладки, соответствующей %s", tip)
		}
		if handles[uint32(hkl)] != 0 {
			return nil, nil, fmt.Errorf("повторная цель раскладки для %s", tip)
		}
		targets = append(targets, uint32(hkl))
		handles[uint32(hkl)] = hkl
	}
	return targets, handles, nil
}

func savedHotkeys() (map[uint32]hotkey, error) {
	bindings := make(map[uint32]hotkey)
	root, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Input Method\Hot Keys`, registry.READ)
	if errors.Is(err, registry.ErrNotExist) {
		return bindings, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		slot, err := strconv.ParseUint(name, 16, 32)
		if err != nil || len(name) != 8 {
			return nil, fmt.Errorf("неизвестный слот сочетания %q", name)
		}
		k, err := registry.OpenKey(root, name, registry.QUERY_VALUE)
		if err != nil {
			return nil, err
		}
		var b hotkey
		fields := [3]*uint32{&b.modifiers, &b.key, &b.target}
		for i, field := range [...]string{"Key Modifiers", "Virtual Key", "Target IME"} {
			data, _, err := k.GetBinaryValue(field)
			if err != nil || len(data) != 4 {
				k.Close()
				return nil, fmt.Errorf("некорректное значение сохранённого сочетания %s/%s", name, field)
			}
			*fields[i] = binary.LittleEndian.Uint32(data)
		}
		k.Close()
		if directSlot(uint32(slot)) && (b.key == 0 || b.target == 0) {
			return nil, fmt.Errorf("некорректное прямое сочетание в слоте %s", name)
		}
		bindings[uint32(slot)] = b
	}
	return bindings, nil
}

func languageLabel(langID uint16) string {
	const localizedDisplayName = 0x2 // LOCALE_SLANGUAGE
	var name [128]uint16
	n, _, _ := getLocaleInfo.Call(uintptr(langID), localizedDisplayName, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if n <= 1 || n > uintptr(len(name)) {
		return ""
	}
	return " (" + windows.UTF16ToString(name[:n]) + ")"
}

func printSavedDirectHotkeys(stage string) {
	fmt.Printf("%s: сохранённые прямые сочетания раскладок\n", stage)
	saved, err := savedHotkeys()
	if err != nil {
		fmt.Printf("  Не удалось прочитать сохранённые сочетания: %v\n", err)
		return
	}
	count := 0
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		b, ok := saved[slot]
		if !ok {
			continue
		}
		key := fmt.Sprintf("модификаторы 0x%04X, виртуальная клавиша 0x%02X", b.modifiers, b.key)
		if b.modifiers == ctrlDigits && b.key >= '0' && b.key <= '9' {
			key = fmt.Sprintf("Ctrl+%c", b.key)
		}
		fmt.Printf("  Слот 0x%03X: %s -> HKL 0x%08X%s\n", slot, key, b.target, languageLabel(uint16(b.target)))
		count++
	}
	if count == 0 {
		fmt.Println("  Нет")
	}
}

func readHotkeys() (map[uint32]hotkey, map[uint32]uintptr, error) {
	saved, err := savedHotkeys()
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось прочитать сохранённые сочетания: %w", err)
	}
	handles := make(map[uint32]uintptr)
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		var b hotkey
		var hkl uintptr
		ok, _, _ := immGetHotkey.Call(uintptr(slot), uintptr(unsafe.Pointer(&b.modifiers)), uintptr(unsafe.Pointer(&b.key)), uintptr(unsafe.Pointer(&hkl)))
		entry, exists := saved[slot]
		b.target = uint32(hkl)
		if (ok != 0) != exists || (exists && b != entry) {
			return nil, nil, fmt.Errorf("активное и сохранённое сочетания различаются в слоте 0x%X", slot)
		}
		if exists {
			if old := handles[b.target]; old != 0 && old != hkl {
				return nil, nil, fmt.Errorf("неоднозначный HKL для цели 0x%08X", b.target)
			}
			handles[b.target] = hkl
		}
	}
	return saved, handles, nil
}

func prepareHotkeys(tips []string, targets []uint32, loaded map[uint32]uintptr) (func() error, error) {
	install := targets != nil
	if install && len(tips) != len(targets) {
		return nil, errors.New("число методов ввода не совпадает с числом целей раскладок")
	}
	if err := immGetHotkey.Find(); err != nil {
		return nil, fmt.Errorf("недоступна функция ImmGetHotKey: %w", err)
	}
	if err := immSetHotkey.Find(); err != nil {
		return nil, fmt.Errorf("недоступна функция ImmSetHotKey: %w", err)
	}
	before, handles, err := readHotkeys()
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		hkl := loaded[target]
		if hkl == 0 || uint32(hkl) != target {
			return nil, fmt.Errorf("нет подходящего загруженного HKL для цели 0x%08X", target)
		}
		if old := handles[target]; old != 0 && old != hkl {
			return nil, fmt.Errorf("HKL активного сочетания и загруженной раскладки различаются для цели 0x%08X", target)
		}
		handles[target] = hkl
	}
	want, err := planHotkeys(before, targets)
	if err != nil {
		return nil, err
	}
	tips = slices.Clone(tips)
	return func() error {
		read := func() (map[uint32]hotkey, error) {
			current, live, err := readHotkeys()
			if err != nil {
				return nil, err
			}
			for target, hkl := range live {
				if hkl != handles[target] {
					return nil, fmt.Errorf("изменился HKL сочетания для цели 0x%08X", target)
				}
			}
			return current, nil
		}
		current, err := read()
		if err != nil {
			return fmt.Errorf("изменения не применены: %w", err)
		}
		if !maps.Equal(current, before) {
			return errors.New("сочетания изменились после предварительной проверки; изменения не применены")
		}
		set := func(slot uint32, b hotkey) error {
			hkl := handles[b.target]
			if b.key != 0 && hkl == 0 {
				return fmt.Errorf("нет HKL для цели 0x%08X", b.target)
			}
			ok, _, callErr := immSetHotkey.Call(uintptr(slot), uintptr(b.modifiers), uintptr(b.key), hkl)
			if ok == 0 {
				return fmt.Errorf("вызов ImmSetHotKey для слота 0x%X завершился неудачно (ошибка Windows: %v)", slot, callErr)
			}
			return nil
		}
		if err := updateHotkeys(before, want, set, read); err != nil {
			return err
		}
		if !install {
			fmt.Printf("Снято прямых сочетаний: %d. Очистка подтверждена в текущем сеансе и в сохранённых настройках.\n", len(before)-len(want))
			return nil
		}
		for i, tip := range tips {
			if layoutDigit(i) == 0 {
				fmt.Printf("%d. %s: без прямого сочетания (доступно только 10 сочетаний)\n", i+1, tip)
			}
		}
		fmt.Println("Прямые сочетания проверены в текущем сеансе и в сохранённых настройках.")
		return nil
	}, nil
}
