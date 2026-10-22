//go:build windows

package windows

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var (
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	setClipboardData = user32.NewProc("SetClipboardData")
	getKeyState      = user32.NewProc("GetKeyState")
	mapVirtualKeyW   = user32.NewProc("MapVirtualKeyW")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalFree       = kernel32.NewProc("GlobalFree")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	lstrlenW         = kernel32.NewProc("lstrlenW")
	rtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
	mapvkVKToChar  = 2
	vkShift        = 0x10
	vkControl      = 0x11
	vkMenu         = 0x12
	vkLWin         = 0x5B
	vkRWin         = 0x5C
	wmContextMenu  = 0x007B
	contextFromKey = ^uintptr(0) // lParam -1: Shift+F10 or the menu key
)

// setClipboard puts text on the clipboard.
func (w *window) setClipboard(text string) {
	if r, _, _ := openClipboard.Call(w.hwnd); r == 0 {
		return
	}
	defer closeClipboard.Call()
	emptyClipboard.Call()
	units := utf16.Encode([]rune(text + "\x00"))
	size := uintptr(len(units) * 2)
	h, _, _ := globalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return
	}
	p, _, _ := globalLock.Call(h)
	if p == 0 {
		globalFree.Call(h)
		return
	}
	rtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&units[0])), size)
	globalUnlock.Call(h)
	if r, _, _ := setClipboardData.Call(cfUnicodeText, h); r == 0 {
		globalFree.Call(h) // the clipboard owns it only on success
	}
}

// clipboard returns the clipboard's text, "" when it holds none.
func (w *window) clipboard() string {
	if r, _, _ := openClipboard.Call(w.hwnd); r == 0 {
		return ""
	}
	defer closeClipboard.Call()
	h, _, _ := getClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := globalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer globalUnlock.Call(h)
	n, _, _ := lstrlenW.Call(p)
	if n == 0 {
		return ""
	}
	units := make([]uint16, n)
	rtlMoveMemory.Call(uintptr(unsafe.Pointer(&units[0])), p, n*2)
	return string(utf16.Decode(units))
}

// setKeymap replaces the keyboard shortcuts.
func (w *window) setKeymap(keys []string) {
	w.keys = map[string]bool{}
	for _, k := range keys {
		w.keys[k] = true
	}
}

// shortcut reports a key press matching a shortcut, by its name with
// "mod" for Control or with "ctrl" itself, and whether it matched.
func (w *window) shortcut(vk uintptr) bool {
	if len(w.keys) == 0 {
		return false
	}
	name := keyName(vk)
	if name == "" {
		return false
	}
	ctrl, alt, shift := keyDown(vkControl), keyDown(vkMenu), keyDown(vkShift)
	meta := keyDown(vkLWin) || keyDown(vkRWin)
	for _, control := range []string{"mod", "ctrl"} {
		var parts []string
		if ctrl {
			parts = append(parts, control)
		}
		if alt {
			parts = append(parts, "alt")
		}
		if shift {
			parts = append(parts, "shift")
		}
		if meta {
			parts = append(parts, "meta")
		}
		if key := strings.Join(append(parts, name), "+"); w.keys[key] {
			w.engine.Dispatch(core.Event{Kind: core.EventKey, Text: key})
			return true
		}
	}
	return false
}

func keyDown(vk uintptr) bool {
	r, _, _ := getKeyState.Call(vk)
	return int16(r) < 0
}

var keyNames = map[uintptr]string{
	0x08: "backspace", 0x09: "tab", 0x0D: "enter", 0x1B: "escape", 0x20: "space",
	0x21: "pageup", 0x22: "pagedown", 0x23: "end", 0x24: "home",
	0x25: "left", 0x26: "up", 0x27: "right", 0x28: "down", 0x2E: "delete",
}

// keyName names a virtual key as core.NormalizeKey does, or "" for the
// modifiers and keys it has no name for.
func keyName(vk uintptr) string {
	switch {
	case keyNames[vk] != "":
		return keyNames[vk]
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return strings.ToLower(string(rune(vk)))
	case vk >= 0x60 && vk <= 0x69: // the keypad's digits
		return string(rune('0' + vk - 0x60))
	case vk >= 0x70 && vk <= 0x87:
		return "f" + strconv.Itoa(int(vk-0x6F))
	}
	// Punctuation, as the keyboard layout types it unshifted.
	c, _, _ := mapVirtualKeyW.Call(vk, mapvkVKToChar)
	if r := rune(c & 0xffff); c&0x80000000 == 0 && unicode.IsPrint(r) && r != ' ' {
		return string(unicode.ToLower(r))
	}
	return ""
}

// hit returns the view under p, in the coordinates of v's parent's
// content, that matches, as the engine's hit test finds the view a press
// goes to: the topmost, passing through views that do not match.
func hit(v *view, p core.Point, match func(*view) bool) *view {
	if v.style.Hidden || !v.frame.Contains(p) {
		return nil
	}
	local := core.Point{X: p.X - v.frame.Origin.X + v.offset.X, Y: p.Y - v.frame.Origin.Y + v.offset.Y}
	for i := len(v.children) - 1; i >= 0; i-- {
		if h := hit(v.children[i], local, match); h != nil {
			return h
		}
	}
	if match(v) {
		return v
	}
	return nil
}

// press reports a click at p: on text with links, with the offset of
// the character under it; elsewhere the engine finds the view.
func (w *window) press(at core.Point) {
	if root := w.view(w.root); root != nil {
		v := hit(root, at, func(v *view) bool { return v.events.Has(core.EventPress) || v.kind == core.KindInput })
		if l, ok := textOf(v); ok {
			b, _ := w.surfaceBounds(v)
			x := w.px(at.X - b.Origin.X - v.content.Origin.X)
			y := w.px(at.Y - b.Origin.Y - v.content.Origin.Y)
			w.engine.Dispatch(core.Event{Kind: core.EventPress, ID: v.id, Index: l.indexAt(x, y)})
			return
		}
	}
	w.engine.PressAt(at)
}

func textOf(v *view) (*spanLayout, bool) {
	if v == nil || v.kind != core.KindText {
		return nil, false
	}
	l, ok := v.text.(*spanLayout)
	return l, ok
}

// contextMenu reports a secondary click at p to the view under it that
// asks for it, at p in the view's coordinates.
func (w *window) contextMenu(at core.Point) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	if v := hit(root, at, func(v *view) bool { return v.events.Has(core.EventContextMenu) }); v != nil {
		b, _ := w.surfaceBounds(v)
		w.engine.Dispatch(core.Event{Kind: core.EventContextMenu, ID: v.id,
			Point: core.Point{X: at.X - b.Origin.X, Y: at.Y - b.Origin.Y}})
	}
}
