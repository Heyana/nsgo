package windows

import (
	"strings"
	"syscall"
	"unicode"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	activeKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	keyCodeVirtualKey    = user32.NewProc("MapVirtualKeyExW")
	keyCodeUnicode       = user32.NewProc("ToUnicodeEx")
)

func (w *window) keyboardCodeNames() map[string]string {
	if !w.heldKeys {
		return nil
	}
	layout, _, _ := activeKeyboardLayout.Call(0)
	if w.keyLayout == layout && w.env.KeyCodeNames != nil {
		return w.env.KeyCodeNames
	}
	w.keyLayout = layout
	names := map[string]string{}
	for _, prefix := range []uint16{0, 0xe000} {
		for code := range uint16(128) {
			scan := prefix | code
			position := driver.WinKeyCode(scan)
			if position == "" {
				continue
			}
			const scanToVirtualKey = 3
			vk, _, _ := keyCodeVirtualKey.Call(uintptr(scan), scanToVirtualKey, layout)
			var state [256]byte
			var chars [8]uint16
			const keepKeyboardState = 4
			n, _, _ := keyCodeUnicode.Call(vk, uintptr(scan), uintptr(unsafe.Pointer(&state[0])), uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)), keepKeyboardState, layout)
			if signed := int32(n); signed > 0 && signed <= int32(len(chars)) && unicode.IsPrint(rune(chars[0])) && chars[0] != ' ' {
				names[position] = strings.ToLower(syscall.UTF16ToString(chars[:signed]))
			} else if name, _ := heldName(vk); name != "" {
				names[position] = name
			}
		}
	}
	return names
}
