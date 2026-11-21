package android

import "github.com/nsgo-dev/nsgo/internal/driver"

func androidKeyCode(scan, key uint32) string {
	if scan > 0 && scan <= 0xffff-8 {
		return driver.XKBKeyCode(uint16(scan + 8)) // evdev to XKB
	}
	// Synthesized KeyEvents may lack a scan code. Use Android's named
	// keys for those cases where its virtual code identifies a position.
	switch {
	case key >= 29 && key <= 54:
		return driver.USBKeyCode(uint16(key - 29 + 4)) // KEYCODE_A ... Z
	case key >= 8 && key <= 16:
		return driver.USBKeyCode(uint16(key - 8 + 30)) // KEYCODE_1 ... 9
	case key >= 131 && key <= 142:
		return driver.USBKeyCode(uint16(key - 131 + 58)) // KEYCODE_F1 ... F12
	}
	return virtualKeyPositions[key]
}

var virtualKeyPositions = map[uint32]string{
	7: "Digit0", 19: "ArrowUp", 20: "ArrowDown", 21: "ArrowLeft", 22: "ArrowRight",
	57: "AltLeft", 58: "AltRight", 59: "ShiftLeft", 60: "ShiftRight", 61: "Tab",
	62: "Space", 66: "Enter", 67: "Backspace", 92: "PageUp", 93: "PageDown",
	111: "Escape", 112: "Delete", 113: "ControlLeft", 114: "ControlRight", 117: "MetaLeft", 118: "MetaRight",
	122: "Home", 123: "End", 124: "Insert", 160: "NumpadEnter",
}

func (s *surface) physicalKey(r *reader) {
	from, id := r.u32(), driver.NodeID(r.u32())
	scan, key := r.u32(), r.u32()
	flags := r.byte()
	name := r.str()
	target := s.target(from)
	if target == nil {
		return
	}
	kind := driver.EventKeyDown
	if flags&16 != 0 {
		kind = driver.EventKeyUp
	}
	target.Dispatch(driver.Event{Kind: kind, ID: id, Text: name, KeyCode: androidKeyCode(scan, key), Repeat: flags&32 != 0,
		Modifiers: driver.Modifiers{Control: flags&1 != 0, Alt: flags&2 != 0, Shift: flags&4 != 0, Platform: flags&8 != 0}})
}
