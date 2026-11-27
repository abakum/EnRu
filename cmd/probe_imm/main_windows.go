package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	firstSlot = 0x100
	lastSlot  = 0x11f
	ctrlMask  = 0xc002
	hotKeys   = `Control Panel\Input Method\Hot Keys`
)

type binding struct {
	Modifiers, Key, Target uint32
}

type backup struct {
	SID      string
	Slot     uint32
	TestKey  uint32
	Snapshot map[uint32]binding
}

var (
	getLayouts = windows.NewLazySystemDLL("user32.dll").NewProc("GetKeyboardLayoutList")
	imm32      = windows.NewLazySystemDLL("imm32.dll")
	get        = imm32.NewProc("ImmGetHotKey")
	set        = imm32.NewProc("ImmSetHotKey")
)

func active() (map[uint32]binding, map[uint32]uintptr) {
	bindings := make(map[uint32]binding)
	handles := make(map[uint32]uintptr)
	for slot := uint32(firstSlot); slot <= lastSlot; slot++ {
		var mod, key uint32
		var hkl uintptr
		if ok, _, _ := get.Call(uintptr(slot), uintptr(unsafe.Pointer(&mod)), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&hkl))); ok != 0 {
			bindings[slot] = binding{mod, key, uint32(hkl)}
			handles[slot] = hkl
		}
	}
	return bindings, handles
}

func persisted() (map[uint32]binding, error) {
	root, err := registry.OpenKey(registry.CURRENT_USER, hotKeys, registry.READ)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	bindings := make(map[uint32]binding)
	for _, name := range names {
		slot, err := strconv.ParseUint(name, 16, 32)
		if err != nil || len(name) != 8 {
			return nil, fmt.Errorf("unrecognized hotkey slot %q", name)
		}
		if _, exists := bindings[uint32(slot)]; exists {
			return nil, fmt.Errorf("duplicate hotkey slot 0x%X", slot)
		}
		key, err := registry.OpenKey(root, name, registry.QUERY_VALUE)
		if err != nil {
			return nil, err
		}
		var b binding
		for i, field := range [...]string{"Key Modifiers", "Virtual Key", "Target IME"} {
			data, _, err := key.GetBinaryValue(field)
			if err != nil || len(data) != 4 {
				key.Close()
				return nil, fmt.Errorf("invalid registry value %s/%s: %v (length %d)", name, field, err, len(data))
			}
			*([3]*uint32{&b.Modifiers, &b.Key, &b.Target}[i]) = binary.LittleEndian.Uint32(data)
		}
		key.Close()
		bindings[uint32(slot)] = b
	}
	return bindings, nil
}

func direct(all map[uint32]binding) map[uint32]binding {
	bindings := make(map[uint32]binding)
	for slot, b := range all {
		if slot >= firstSlot && slot <= lastSlot {
			bindings[slot] = b
		}
	}
	return bindings
}

func printSlots(title string, bindings map[uint32]binding) {
	fmt.Println(title)
	for slot := uint32(firstSlot); slot <= lastSlot; slot++ {
		if b, ok := bindings[slot]; ok {
			fmt.Printf("  0x%03X: modifiers=0x%04X key=0x%02X HKL=0x%08X\n", slot, b.Modifiers, b.Key, b.Target)
		}
	}
}

func writeBackup(path string, b backup) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(b)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func validate(b backup, sid string) error {
	original := b.Snapshot[b.Slot]
	if b.SID != sid || b.Slot < firstSlot || b.Slot > lastSlot || original.Modifiers != ctrlMask ||
		original.Target == 0 || original.Key < '0' || original.Key > '9' ||
		b.TestKey < '0' || b.TestKey > '9' || b.TestKey == original.Key {
		return errors.New("backup does not describe a valid experiment for this Windows account")
	}
	for slot, item := range b.Snapshot {
		if item.Key == b.TestKey || (slot != b.Slot && item.Key == original.Key) {
			return fmt.Errorf("possible digit conflict in slot 0x%X", slot)
		}
	}
	return nil
}

func change(slot uint32, b binding, hkl uintptr, want map[uint32]binding) error {
	if b != (binding{}) && (hkl == 0 || uint32(hkl) != b.Target) {
		return errors.New("current HKL does not identify the original target")
	}
	ok, _, callErr := set.Call(uintptr(slot), uintptr(b.Modifiers), uintptr(b.Key), hkl)
	if ok == 0 {
		return fmt.Errorf("ImmSetHotKey returned FALSE (last error: %v); rescan before making further changes", callErr)
	}
	current, _ := active()
	saved, err := persisted()
	if err != nil {
		return fmt.Errorf("ImmSetHotKey returned TRUE, but persisted settings could not be read: %w", err)
	}
	activeMatch, savedMatch := maps.Equal(current, direct(want)), maps.Equal(saved, want)
	fmt.Printf("ImmSetHotKey returned TRUE. Active slots match: %t. Saved entries match: %t.\n", activeMatch, savedMatch)
	if !activeMatch || !savedMatch {
		return errors.New("settings did not match the expected result; keep the backup and inspect the Windows dialog")
	}
	return nil
}

func loadedHandle(target uint32) (uintptr, error) {
	if err := getLayouts.Find(); err != nil {
		return 0, err
	}
	n, _, _ := getLayouts.Call(0, 0)
	if n == 0 || n > 65536 {
		return 0, errors.New("could not determine the number of loaded layouts")
	}
	// One extra entry lets us detect a list that grew between calls.
	layouts := make([]uintptr, int(n)+1)
	count, _, _ := getLayouts.Call(uintptr(len(layouts)), uintptr(unsafe.Pointer(&layouts[0])))
	if count != n {
		return 0, errors.New("could not obtain a stable list of loaded layouts")
	}
	var match uintptr
	for _, hkl := range layouts[:count] {
		if hkl == 0 || uint32(hkl) != target {
			continue
		}
		if match != 0 && match != hkl {
			return 0, errors.New("backup target matches multiple loaded handles")
		}
		match = hkl
	}
	if match == 0 {
		return 0, fmt.Errorf("backup target 0x%08X is not loaded; use Advanced Key Settings", target)
	}
	return match, nil
}

func run(args []string) error {
	if len(args) == 0 || !(len(args) == 1 && (args[0] == "scan" || args[0] == "clear" || args[0] == "restore") || len(args) == 3 && args[0] == "assign") {
		return errors.New("usage: probe_imm.exe scan | assign <slot e.g. 0x100> <digit> | clear | restore")
	}
	if err := get.Find(); err != nil {
		return err
	}
	if args[0] != "scan" {
		if err := set.Find(); err != nil {
			return err
		}
	} else if err := set.Find(); err != nil {
		fmt.Printf("ImmSetHotKey unavailable: %v\n", err)
	}
	current, handles := active()
	saved, err := persisted()
	if err != nil {
		return fmt.Errorf("read saved hotkeys: %w", err)
	}
	if args[0] == "scan" {
		printSlots("Read through ImmGetHotKey:", current)
		printSlots("Saved Windows settings:", direct(saved))
		if !maps.Equal(current, direct(saved)) {
			return errors.New("active and saved slots disagree; FALSE from ImmGetHotKey does not establish that a slot is empty")
		}
		return nil
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sid := user.User.Sid.String()
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "probe_imm_backup.json")
	if !filepath.IsAbs(path) {
		return errors.New("expected an absolute user configuration directory")
	}
	fmt.Printf("Backup: %s\n", path)

	if args[0] == "assign" {
		slot, err := strconv.ParseUint(args[1], 0, 32)
		if err != nil || len(args[2]) != 1 {
			return errors.New("expected a direct slot and one digit")
		}
		b := backup{sid, uint32(slot), uint32(args[2][0]), saved}
		if err := validate(b, sid); err != nil {
			return err
		}
		if !maps.Equal(current, direct(saved)) {
			return errors.New("active and saved slots disagree; nothing changed")
		}
		if err := writeBackup(path, b); err != nil {
			return fmt.Errorf("could not create exclusive backup: %w; nothing changed", err)
		}
		test := saved[b.Slot]
		test.Key = b.TestKey
		want := maps.Clone(saved)
		want[b.Slot] = test
		return change(b.Slot, test, handles[b.Slot], want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var b backup
	if err := json.Unmarshal(data, &b); err != nil {
		return err
	}
	if err := validate(b, sid); err != nil {
		return err
	}
	original := b.Snapshot[b.Slot]
	test := original
	test.Key = b.TestKey
	wantTest := maps.Clone(b.Snapshot)
	wantTest[b.Slot] = test
	wantCleared := maps.Clone(b.Snapshot)
	delete(wantCleared, b.Slot)
	originalMatches := maps.Equal(saved, b.Snapshot) && maps.Equal(current, direct(b.Snapshot))
	if args[0] == "clear" {
		if !originalMatches {
			return errors.New("clear requires the original settings from the backup; nothing changed")
		}
		hkl, err := loadedHandle(original.Target)
		if err != nil {
			return err
		}
		if hkl != handles[b.Slot] {
			return errors.New("original slot does not match the loaded layout; nothing changed")
		}
		return change(b.Slot, binding{}, 0, wantCleared)
	}
	if originalMatches {
		fmt.Println("Original settings already in place. Backup retained.")
		return nil
	}
	if maps.Equal(saved, wantCleared) && maps.Equal(current, direct(wantCleared)) {
		hkl, err := loadedHandle(original.Target)
		if err != nil {
			return err
		}
		return change(b.Slot, original, hkl, b.Snapshot)
	}
	if (!maps.Equal(saved, wantTest) && !maps.Equal(saved, b.Snapshot)) || !maps.Equal(current, direct(wantTest)) {
		return errors.New("settings no longer match the experiment; nothing changed. Check Advanced Key Settings")
	}
	// Use the live handle from the verified test binding, never a handle from disk.
	return change(b.Slot, original, handles[b.Slot], b.Snapshot)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
