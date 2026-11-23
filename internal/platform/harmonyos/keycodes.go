package harmonyos

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file builds on every system so tests can check the table without the js/wasm runtime.

// ohosKeyPositions names InputKit's KeyCodes for a keyboard's keys by KeyboardEvent.code. The input service assigns a KeyCode to each hardware key, and the layout then decides the text it types, which the page sends as keyText. Keys a phone's keypad has and a keyboard lacks, such as KEYCODE_STAR, name no position.
var ohosKeyPositions = func() map[driver.NodeID]string {
	m := map[driver.NodeID]string{
		2012: "ArrowUp", 2013: "ArrowDown", 2014: "ArrowLeft", 2015: "ArrowRight",
		2043: "Comma", 2044: "Period", 2045: "AltLeft", 2046: "AltRight", 2047: "ShiftLeft", 2048: "ShiftRight",
		2049: "Tab", 2050: "Space", 2054: "Enter", 2055: "Backspace", 2056: "Backquote", 2057: "Minus", 2058: "Equal",
		2059: "BracketLeft", 2060: "BracketRight", 2061: "Backslash", 2062: "Semicolon", 2063: "Quote", 2064: "Slash",
		2067: "ContextMenu", 2068: "PageUp", 2069: "PageDown", 2070: "Escape", 2071: "Delete",
		2072: "ControlLeft", 2073: "ControlRight", 2074: "CapsLock", 2075: "ScrollLock", 2076: "MetaLeft", 2077: "MetaRight",
		2079: "PrintScreen", 2080: "Pause", 2081: "Home", 2082: "End", 2083: "Insert", 2102: "NumLock",
		2113: "NumpadDivide", 2114: "NumpadMultiply", 2115: "NumpadSubtract", 2116: "NumpadAdd", 2117: "NumpadDecimal",
		2118: "NumpadComma", 2119: "NumpadEnter", 2120: "NumpadEqual", 2121: "NumpadParenLeft", 2122: "NumpadParenRight",
	}
	for i := range 10 {
		m[driver.NodeID(2000+i)] = fmt.Sprintf("Digit%d", i)
		m[driver.NodeID(2103+i)] = fmt.Sprintf("Numpad%d", i)
	}
	for i := range 26 {
		m[driver.NodeID(2017+i)] = "Key" + string(rune('A'+i))
	}
	for i := range 12 {
		m[driver.NodeID(2090+i)] = fmt.Sprintf("F%d", i+1)
	}
	return m
}()

// keyPosition returns the physical position, named as KeyboardEvent.code, of a KeyCode the page reported, or of the Linux input code the emulator's uinput passes through in its place, which XKB numbers 8 higher. It is empty for a key with no position.
func keyPosition(code driver.NodeID) string {
	if name, ok := ohosKeyPositions[code]; ok {
		return name
	}
	if code > 0 && code < 2000 {
		return driver.XKBKeyCode(uint16(code) + 8)
	}
	return ""
}
