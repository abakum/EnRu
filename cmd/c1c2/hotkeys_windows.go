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
	currentLayout  = user32.NewProc("GetKeyboardLayout")
	layoutName     = user32.NewProc("GetKeyboardLayoutNameW")
	getLocaleInfo  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetLocaleInfoW")
)

func profileLayouts() ([]string, error) {
	root, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\International\User Profile`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	size, _, err := root.GetValue("Languages", nil)
	if err != nil {
		return nil, err
	}
	data := make([]byte, size)
	n, kind, err := root.GetValue("Languages", data)
	if err != nil {
		return nil, err
	}
	if kind != registry.MULTI_SZ || n > len(data) {
		return nil, errors.New("Languages is not a valid REG_MULTI_SZ value")
	}
	languages, err := decodeProfileLanguages(data[:n])
	if err != nil {
		return nil, err
	}
	var tips []string
	for _, lang := range languages {
		entries, err := languageLayouts(root, lang)
		if err != nil {
			return nil, fmt.Errorf("read layouts for %s: %w", lang, err)
		}
		tips = append(tips, entries...)
	}
	if len(tips) < 2 {
		sid := "unknown"
		if user, err := windows.GetCurrentProcessToken().GetTokenUser(); err == nil {
			sid = user.User.Sid.String()
		}
		return nil, fmt.Errorf("at least two keyboard layouts are required; this Windows profile has %d (SID: %s; languages: %q; input methods: %q). Run install as the account whose layouts you want to configure", len(tips), sid, languages, tips)
	}
	return tips, nil
}

func decodeProfileLanguages(data []byte) ([]string, error) {
	if len(data)%2 != 0 {
		return nil, errors.New("Languages contains incomplete UTF-16 data")
	}
	// Preserve the last language even if REG_MULTI_SZ is missing its extra NUL.
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
					return nil, errors.New("Languages contains an unexpected empty entry")
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

func languageLayouts(root registry.Key, lang string) ([]string, error) {
	k, err := registry.OpenKey(root, lang, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	positions := make(map[uint64]string)
	for _, name := range names {
		if !strings.Contains(name, ":") {
			continue
		}
		if len(name) != 13 || name[4] != ':' {
			return nil, fmt.Errorf("unsupported input method %q", name)
		}
		if _, err := strconv.ParseUint(name[:4]+name[5:], 16, 48); err != nil {
			return nil, fmt.Errorf("unsupported input method %q", name)
		}
		pos, kind, err := k.GetIntegerValue(name)
		if err != nil || kind != registry.DWORD || pos == 0 || positions[pos] != "" {
			return nil, fmt.Errorf("ambiguous position for input method %q", name)
		}
		positions[pos] = strings.ToLower(name)
	}
	if len(positions) == 0 {
		return nil, errors.New("no identifiable keyboard layouts")
	}
	ordered := make([]string, len(positions))
	for i := range ordered {
		ordered[i] = positions[uint64(i+1)]
		if ordered[i] == "" {
			return nil, errors.New("layout positions are not consecutive")
		}
	}
	return ordered, nil
}

func loadedLayouts(tips []string) (_ []uint32, _ map[uint32]uintptr, err error) {
	n, _, _ := layoutList.Call(0, 0)
	if n == 0 || n > 65536 {
		return nil, nil, errors.New("cannot enumerate loaded keyboard layouts")
	}
	layouts := make([]uintptr, int(n)+1)
	if count, _, _ := layoutList.Call(uintptr(len(layouts)), uintptr(unsafe.Pointer(&layouts[0]))); count != n {
		return nil, nil, errors.New("loaded keyboard layouts changed during enumeration")
	}
	// GetKeyboardLayoutNameW identifies the active layout of this OS thread.
	// Do not load layouts, change their order, or activate them process-wide.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var previous uintptr
	defer func() {
		if previous != 0 {
			if ok, _, callErr := activateLayout.Call(previous, 0); ok == 0 {
				err = errors.Join(err, fmt.Errorf("cannot restore the install thread's keyboard layout: %v", callErr))
			}
		}
	}()
	byTIP := make(map[string]uintptr)
	for _, hkl := range layouts[:n] {
		if hkl <= 1 {
			continue
		}
		old, _, _ := activateLayout.Call(hkl, 0)
		if old == 0 {
			continue
		}
		if previous == 0 {
			previous = old
		}
		if active, _, _ := currentLayout.Call(0); active != hkl {
			return nil, nil, fmt.Errorf("cannot verify active HKL 0x%X", hkl)
		}
		var name [9]uint16
		if ok, _, _ := layoutName.Call(uintptr(unsafe.Pointer(&name[0]))); ok == 0 {
			return nil, nil, fmt.Errorf("cannot identify loaded HKL 0x%X", hkl)
		}
		tip := fmt.Sprintf("%04x:%s", uint16(hkl), strings.ToLower(windows.UTF16ToString(name[:])))
		if !slices.Contains(tips, tip) {
			continue
		}
		if old := byTIP[tip]; old != 0 && old != hkl {
			return nil, nil, fmt.Errorf("multiple loaded handles match %s", tip)
		}
		byTIP[tip] = hkl
	}
	targets := make([]uint32, 0, len(tips))
	handles := make(map[uint32]uintptr)
	for _, tip := range tips {
		hkl := byTIP[tip]
		if hkl == 0 {
			return nil, nil, fmt.Errorf("no loaded keyboard layout matches %s", tip)
		}
		if handles[uint32(hkl)] != 0 {
			return nil, nil, fmt.Errorf("duplicate layout target for %s", tip)
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
			return nil, fmt.Errorf("unrecognized hotkey slot %q", name)
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
				return nil, fmt.Errorf("invalid saved hotkey %s/%s", name, field)
			}
			*fields[i] = binary.LittleEndian.Uint32(data)
		}
		k.Close()
		if directSlot(uint32(slot)) && (b.key == 0 || b.target == 0) {
			return nil, fmt.Errorf("invalid direct hotkey in slot %s", name)
		}
		bindings[uint32(slot)] = b
	}
	return bindings, nil
}

func languageLabel(langID uint16) string {
	const englishDisplayName = 0x72 // LOCALE_SENGLISHDISPLAYNAME
	var name [128]uint16
	n, _, _ := getLocaleInfo.Call(uintptr(langID), englishDisplayName, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if n <= 1 || n > uintptr(len(name)) {
		return ""
	}
	return " (" + windows.UTF16ToString(name[:n]) + ")"
}

func printSavedDirectHotkeys(stage string) {
	fmt.Printf("%s: saved direct layout shortcuts\n", stage)
	saved, err := savedHotkeys()
	if err != nil {
		fmt.Printf("  Could not read saved shortcuts: %v\n", err)
		return
	}
	count := 0
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		b, ok := saved[slot]
		if !ok {
			continue
		}
		key := fmt.Sprintf("modifiers=0x%04X, virtual key=0x%02X", b.modifiers, b.key)
		if b.modifiers == ctrlDigits && b.key >= '0' && b.key <= '9' {
			key = fmt.Sprintf("Ctrl+%c", b.key)
		}
		fmt.Printf("  Slot 0x%03X: %s -> HKL 0x%08X%s\n", slot, key, b.target, languageLabel(uint16(b.target)))
		count++
	}
	if count == 0 {
		fmt.Println("  None")
	}
}

func readHotkeys() (map[uint32]hotkey, map[uint32]uintptr, error) {
	saved, err := savedHotkeys()
	if err != nil {
		return nil, nil, err
	}
	handles := make(map[uint32]uintptr)
	for slot := uint32(firstDirect); slot <= lastDirect; slot++ {
		var b hotkey
		var hkl uintptr
		ok, _, _ := immGetHotkey.Call(uintptr(slot), uintptr(unsafe.Pointer(&b.modifiers)), uintptr(unsafe.Pointer(&b.key)), uintptr(unsafe.Pointer(&hkl)))
		entry, exists := saved[slot]
		b.target = uint32(hkl)
		if (ok != 0) != exists || (exists && b != entry) {
			return nil, nil, fmt.Errorf("active and saved hotkeys disagree at slot 0x%X", slot)
		}
		if exists {
			if old := handles[b.target]; old != 0 && old != hkl {
				return nil, nil, fmt.Errorf("ambiguous handle for target 0x%08X", b.target)
			}
			handles[b.target] = hkl
		}
	}
	return saved, handles, nil
}

func prepareHotkeys(install bool) (func() error, error) {
	if err := immGetHotkey.Find(); err != nil {
		return nil, err
	}
	if err := immSetHotkey.Find(); err != nil {
		return nil, err
	}
	before, handles, err := readHotkeys()
	if err != nil {
		return nil, err
	}
	var tips []string
	var targets []uint32
	if install {
		tips, err = profileLayouts()
		if err != nil {
			return nil, err
		}
		var loaded map[uint32]uintptr
		targets, loaded, err = loadedLayouts(tips)
		if err != nil {
			return nil, err
		}
		for target, hkl := range loaded {
			if old := handles[target]; old != 0 && old != hkl {
				return nil, fmt.Errorf("active hotkey and loaded layout disagree for target 0x%08X", target)
			}
			handles[target] = hkl
		}
	}
	want, err := planHotkeys(before, targets)
	if err != nil {
		return nil, err
	}
	return func() error {
		current, live, err := readHotkeys()
		if err != nil {
			return err
		}
		if !maps.Equal(current, before) {
			return errors.New("hotkeys changed since preflight; no changes applied")
		}
		for target, hkl := range live {
			if hkl != handles[target] {
				return fmt.Errorf("hotkey handle changed for target 0x%08X; no changes applied", target)
			}
		}
		set := func(slot uint32, b hotkey) error {
			hkl := handles[b.target]
			if b.key != 0 && hkl == 0 {
				return fmt.Errorf("no handle for target 0x%08X", b.target)
			}
			ok, _, callErr := immSetHotkey.Call(uintptr(slot), uintptr(b.modifiers), uintptr(b.key), hkl)
			if ok == 0 {
				return fmt.Errorf("ImmSetHotKey failed for slot 0x%X (last error: %v)", slot, callErr)
			}
			return nil
		}
		if err := updateHotkeys(before, want, set, func() (map[uint32]hotkey, error) {
			current, _, err := readHotkeys()
			return current, err
		}); err != nil {
			return err
		}
		if !install {
			fmt.Printf("Removed %d direct layout shortcuts from the active session and saved settings.\n", len(before)-len(want))
			return nil
		}
		for i, tip := range tips {
			if layoutDigit(i) == 0 {
				label := languageLabel(uint16(targets[i]))
				fmt.Printf("%d. %s%s: no direct shortcut (10-shortcut limit)\n", i+1, tip, label)
			}
		}
		fmt.Println("Direct shortcuts verified in the active session and saved settings.")
		return nil
	}, nil
}
