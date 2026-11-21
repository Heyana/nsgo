//go:build windows

package windows

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	// getClipboardSequenceNumber grows on every copy, the change count a poller reads before its content.
	getClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	// enumClipboardFormats walks the formats the clipboard offers; each registered one has a name.
	enumClipboardFormats    = user32.NewProc("EnumClipboardFormats")
	getClipboardFormatNameW = user32.NewProc("GetClipboardFormatNameW")
	setClipboardData           = user32.NewProc("SetClipboardData")
	getKeyState                = user32.NewProc("GetKeyState")
	mapVirtualKeyW             = user32.NewProc("MapVirtualKeyW")
	globalAlloc                = kernel32.NewProc("GlobalAlloc")
	globalFree                 = kernel32.NewProc("GlobalFree")
	globalLock                 = kernel32.NewProc("GlobalLock")
	globalUnlock               = kernel32.NewProc("GlobalUnlock")
	lstrlenW                   = kernel32.NewProc("lstrlenW")
	rtlMoveMemory              = kernel32.NewProc("RtlMoveMemory")
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
	vkLButton      = 0x01
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

// editing reports whether a text field or a page has the keyboard focus.
func (w *window) editing() bool {
	if w.focus != nil {
		return true
	}
	for wv := range w.webs {
		if wv.focused {
			return true
		}
	}
	return false
}

// setKeymap replaces the keyboard shortcuts.
func (w *window) setKeymap(keys []driver.KeyBinding) { w.keys = keys }

// keyDown offers a key going down to the focused view that hears keys (View.OnKeyDown), before the keymap and the view's own keys, and reports whether it consumed the key. A modifier alone is reported but never consumed: no binding takes one, and the window follows the modifiers through its messages.
func (w *window) keyDown(vk, lParam uintptr) bool {
	v := w.focusedView()
	if w.engine == nil || v == nil || v.deleted || v.field != nil || !v.events.Has(driver.EventKeyDown) {
		return false
	}
	name, modifier := keyName(vk), false
	if name == "" {
		modifier = true
		switch vk {
		case vkShift:
			name = "shift"
		case vkControl:
			name = "ctrl"
		case vkMenu:
			name = "alt"
		case vkLWin, vkRWin:
			name = "meta"
		default:
			return false
		}
	}
	consumed := w.engine.DispatchKeyDown(driver.Event{Kind: driver.EventKeyDown, ID: v.id, Text: name, Modifiers: modifiers(), Repeat: repeated(lParam)})
	return consumed && !modifier
}

// shortcut reports a key press matching a shortcut, by its name with
// "mod" for Control or with "ctrl" itself, and whether it was consumed:
// the app's own first, as driver.FindKey decides for a focused field and a
// held key (lParam's repeat bit), then the application menu's.
func (w *window) shortcut(vk, lParam uintptr) bool {
	if len(w.keys) == 0 && len(w.chrome.keys) == 0 {
		return false
	}
	name := keyName(vk)
	if name == "" {
		return false
	}
	ctrl, alt, shift := keyDown(vkControl), keyDown(vkMenu), keyDown(vkShift)
	meta := keyDown(vkLWin) || keyDown(vkRWin)
	var names []string
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
		names = append(names, strings.Join(append(parts, name), "+"))
	}
	if id, consumed := driver.FindKey(w.keys, names, w.focusKeyUse(), repeated(lParam)); consumed {
		if id != 0 {
			w.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
		}
		return true
	}
	for _, key := range names {
		if w.menuShortcut(key) {
			return true
		}
	}
	return false
}

func keyDown(vk uintptr) bool {
	r, _, _ := getKeyState.Call(vk)
	return int16(r) < 0
}

// modifiers returns the modifier keys held as the current message was
// sent.
func modifiers() driver.Modifiers {
	return driver.Modifiers{Control: keyDown(vkControl), Alt: keyDown(vkMenu), Shift: keyDown(vkShift),
		Platform: keyDown(vkLWin) || keyDown(vkRWin)}
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
func hit(v *view, p driver.Point, match func(*view) bool) *view {
	if v.style.Hidden {
		return nil
	}
	self, children := v.reach(p)
	if children {
		local := v.local(p)
		local.X += v.offset.X
		local.Y += v.offset.Y
		for i := len(v.children) - 1; i >= 0; i-- {
			if h := hit(v.children[i], local, match); h != nil {
				return h
			}
		}
	}
	if self && match(v) {
		return v
	}
	return nil
}

// reach says what p, in the coordinates of v's parent's content, reaches
// of v: v itself and its children, as core.Reach decides.
func (v *view) reach(p driver.Point) (self, children bool) {
	local := v.local(p)
	if math.IsInf(float64(local.X), 0) || math.IsInf(float64(local.Y), 0) {
		return false, false
	}
	return driver.ReachView(frameOf(v).Size, &v.props, v.overflow, v.transformedKids, local)
}

// under reports whether p reaches v or its children.
func (v *view) under(p driver.Point) bool {
	self, children := v.reach(p)
	return !v.style.Hidden && (self || children)
}

// press reports a click at p, with the modifiers and count of the
// button's press: on text with links, with the offset of the character
// under it; elsewhere the engine finds the view.
func (w *window) press(at driver.Point) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	v := hit(root, at, func(v *view) bool { return v.events.Has(driver.EventPress) || v.kind == driver.KindInput })
	if v == nil {
		return
	}

	ev := w.down
	ev.Kind, ev.ID, ev.Point, ev.Page = driver.EventPress, v.id, v.surfacePoint(at), at
	if l, ok := textOf(v); ok {
		ev.Index = l.indexAt(float32(ev.Point.X-v.content.Origin.X), float32(ev.Point.Y-v.content.Origin.Y))
	}
	w.engine.Dispatch(ev)
}

func textOf(v *view) (*textLayout, bool) {
	if v == nil || v.kind != driver.KindText {
		return nil, false
	}
	l, ok := v.text.(*textLayout)
	return l, ok
}

// contextMenu reports a secondary click at p to the view under it that
// asks for it, at p in the view's coordinates.
func (w *window) contextMenu(at driver.Point) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	if v := hit(root, at, func(v *view) bool { return v.events.Has(driver.EventContextMenu) }); v != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventContextMenu, ID: v.id,
			Point: v.surfacePoint(at), Page: at, Modifiers: modifiers()})
	}
}
