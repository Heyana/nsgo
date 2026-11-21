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
	"github.com/nsgo-dev/nsgo/internal/platform/viewtree"
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
	vkLShift       = 0xA0
	vkRShift       = 0xA1
	scanLeftShift  = 0x2A
	scanRightShift = 0x36
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

// setKeymap replaces the keyboard shortcuts, and whether every key or mouse button going down and up on the surface is reported (driver.MountItem.HeldKeys, HeldMouseButtons).
func (w *window) setKeymap(keys []driver.KeyBinding, held, heldMouse bool) {
	w.keys, w.heldKeys, w.heldMouse = keys, held, heldMouse
}

// heldMouseButton is a mouse button whose down was reported: the view it went to (nil/0 for none) and its click count.
type heldMouseButton struct {
	id     driver.NodeID
	v      *view
	clicks uint32
}

// buttonClicks tracks consecutive clicks of one mouse button.
type buttonClicks struct {
	time  uint32
	at    point
	count uint32
}

// countButtonClicks counts consecutive clicks of btn against the system double-click time and distance.
func (w *window) countButtonClicks(btn driver.MouseButton, lParam uintptr) uint32 {
	t, _, _ := getMessageTime.Call()
	maxMs, _, _ := getDoubleClickTime.Call()
	cx, _, _ := getSystemMetrics.Call(smCxClick)
	cy, _, _ := getSystemMetrics.Call(smCyClick)
	p := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
	if w.mouseClicks == nil {
		w.mouseClicks = map[driver.MouseButton]buttonClicks{}
	}
	c := w.mouseClicks[btn]
	near := abs32(p.X-c.at.X) <= int32(cx)/2 && abs32(p.Y-c.at.Y) <= int32(cy)/2
	if near && c.count > 0 && uint32(t)-c.time <= uint32(maxMs) {
		c.count++
	} else {
		c.count = 1
	}
	c.time, c.at = uint32(t), p
	w.mouseClicks[btn] = c
	return c.count
}

// mouseViewAt returns the innermost view at client point at that listens for mouse button events.
func (w *window) mouseViewAt(at driver.Point) *view {
	root := w.view(w.root)
	if root == nil {
		return nil
	}
	under, _ := viewsAt(root, at, func(v *view) bool { return v.events.Has(driver.EventMouseDown) }, nil)
	if len(under) == 0 {
		return nil
	}
	return under[len(under)-1]
}

// mouseButtonDown reports btn going down at client point at, returning whether a view heard it.
func (w *window) mouseButtonDown(btn driver.MouseButton, at driver.Point, clicks uint32) bool {
	if btn == driver.MouseButtonNone || w.engine == nil {
		return false
	}
	v := w.mouseViewAt(at)
	if v == nil && !w.heldMouse {
		return false
	}
	mods := modifiers()
	if clicks == 0 {
		clicks = 1
	}
	if prev, ok := w.mouseHeld[btn]; ok {
		delete(w.mouseHeld, btn)
		w.engine.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: prev.id, Page: at, Modifiers: mods, Button: btn})
	}
	if w.mouseHeld == nil {
		w.mouseHeld = map[driver.MouseButton]heldMouseButton{}
	}
	if v != nil {
		w.mouseHeld[btn] = heldMouseButton{id: v.id, v: v, clicks: clicks}
		w.engine.Dispatch(driver.Event{
			Kind:      driver.EventMouseDown,
			ID:        v.id,
			Point:     v.surfacePoint(at),
			Page:      at,
			Modifiers: mods,
			Button:    btn,
			Count:     clicks,
		})
		return true
	}
	w.mouseHeld[btn] = heldMouseButton{clicks: clicks}
	w.engine.Dispatch(driver.Event{
		Kind:      driver.EventMouseDown,
		Page:      at,
		Modifiers: mods,
		Button:    btn,
		Count:     clicks,
	})
	return false
}

// mouseButtonUp reports btn coming up at client point at to the view its down went to (or the view currently under the pointer).
func (w *window) mouseButtonUp(btn driver.MouseButton, at driver.Point) {
	if btn == driver.MouseButtonNone || w.engine == nil {
		return
	}
	mods := modifiers()
	if h, ok := w.mouseHeld[btn]; ok {
		delete(w.mouseHeld, btn)
		ev := driver.Event{
			Kind:      driver.EventMouseUp,
			ID:        h.id,
			Page:      at,
			Modifiers: mods,
			Button:    btn,
			Count:     h.clicks,
		}
		if h.v != nil && !h.v.deleted {
			ev.Point = h.v.surfacePoint(at)
		}
		w.engine.Dispatch(ev)
		return
	}
	if v := w.mouseViewAt(at); v != nil {
		w.engine.Dispatch(driver.Event{
			Kind:      driver.EventMouseUp,
			ID:        v.id,
			Point:     v.surfacePoint(at),
			Page:      at,
			Modifiers: mods,
			Button:    btn,
			Count:     1,
		})
	}
}

// releaseMouseButtons reports every held mouse button coming up with zero modifiers and clicks when the window loses focus.
func (w *window) releaseMouseButtons() {
	if w.engine == nil {
		return
	}
	for b := driver.MouseButtonLeft; b <= driver.MouseButtonForward; b++ {
		if h, ok := w.mouseHeld[b]; ok {
			delete(w.mouseHeld, b)
			w.engine.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: h.id, Button: b})
		}
	}
}

// heldKey is a key the window reported going down, by its physical key (physicalKey), until it reports it coming up: the view the down went to, or 0, and the name the down had.
type heldKey struct {
	id   driver.NodeID
	name string
}

// physicalKey identifies the key a key message's lParam names, so a key's up finds its down whatever the layout or the modifiers make of it since: the scan code and the extended bit (bits 16-24), which tell the left Shift from the right and the keypad's Enter from the main one. Input that carries no scan code is identified by its virtual key, outside the scan codes' range.
func physicalKey(vk, lParam uintptr) uint32 {
	if code := uint32(lParam>>16) & 0x1ff; code&0xff != 0 {
		return code
	}
	return 0x10000 | uint32(vk&0xffff)
}

// heldName names a key as keyDown reports it, a modifier alone as "shift", "ctrl", "alt" or "meta", and says whether it is a modifier; "" for a key with no name, such as the IME's VK_PROCESSKEY while it composes.
func heldName(vk uintptr) (name string, modifier bool) {
	if name := keyName(vk); name != "" {
		return name, false
	}
	switch vk {
	case vkShift:
		return "shift", true
	case vkControl:
		return "ctrl", true
	case vkMenu:
		return "alt", true
	case vkLWin, vkRWin:
		return "meta", true
	}
	return "", false
}

// keyDown offers a key going down to the focused view that hears keys (View.OnKeyDown), before the keymap and the view's own keys, and reports whether it consumed the key. A modifier alone is reported but never consumed: no binding takes one, and the window follows the modifiers through its messages. Where no view that hears keys has the focus and the surface asked for held keys, the down is reported asynchronously with ID 0 and consumes nothing. Either way the window records the key with the view and the name it reported, the first time the key is seen going down, even as an auto-repeat (a key held as the window got the keyboard), so its up reaches the same view (keyUp); a recorded key going down again without repeating lost its up, which it reports first. Keys typed while a text field or a page has the focus are neither reported nor recorded; an input method composing sends VK_PROCESSKEY, which has no name.
func (w *window) keyDown(vk, lParam uintptr) bool {
	if w.escapePointer(vk) {
		return true
	}
	if w.engine == nil {
		return false
	}
	w.releaseShifts()
	if w.editing() {
		return false
	}
	name, modifier := heldName(vk)
	if name == "" {
		return false
	}
	v := w.focusedView()
	hears := v != nil && !v.deleted && v.field == nil && v.events.Has(driver.EventKeyDown)
	if !hears && !w.heldKeys {
		return false
	}
	ev := driver.Event{Kind: driver.EventKeyDown, Text: name, Modifiers: modifiers(), Repeat: repeated(lParam)}
	if hears {
		ev.ID = v.id
	}
	code := physicalKey(vk, lParam)
	if held, recorded := w.keysHeld[code]; !recorded || !ev.Repeat {
		if recorded {
			// A key that goes down again without repeating lost its up, which is reported first, so the view hears one up for each down.
			w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, Modifiers: ev.Modifiers})
		}
		if w.keysHeld == nil {
			w.keysHeld = map[uint32]heldKey{}
		}
		w.keysHeld[code] = heldKey{id: ev.ID, name: name}
	}
	if !hears {
		// An auto-repeat changes nothing the app can read without a view to hear it.
		if !ev.Repeat {
			w.engine.Dispatch(ev)
		}
		return false
	}
	consumed := w.engine.DispatchKeyDown(ev)
	return consumed && !modifier
}

// keyUp reports a key the window recorded going down coming up (driver.EventKeyUp), to the view its down went to wherever the focus is now, with the modifiers still held, which GetKeyState no longer counts the released key among. An up the window recorded no down for (a key pressed in a field, or before the window had the keyboard) is not reported.
func (w *window) keyUp(vk, lParam uintptr) {
	if w.engine == nil {
		return
	}
	w.releaseShifts()
	code := physicalKey(vk, lParam)
	k, ok := w.keysHeld[code]
	if !ok {
		return
	}
	delete(w.keysHeld, code)
	w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: k.id, Text: k.name, Modifiers: modifiers()})
}

// releaseShifts reports each Shift the window holds whose side GetKeyState shows up. With both Shifts down, Windows sends no WM_KEYUP for the first one released, so the window checks on every key message.
func (w *window) releaseShifts() {
	for code, k := range w.keysHeld {
		var side uintptr
		switch code {
		case scanLeftShift:
			side = vkLShift
		case scanRightShift:
			side = vkRShift
		default:
			continue
		}
		if !keyDown(side) {
			delete(w.keysHeld, code)
			w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: k.id, Text: k.name, Modifiers: modifiers()})
		}
	}
}

// releaseKeys reports every key the window holds coming up, with no modifiers, as the window loses the keyboard and with it the keys' ups.
func (w *window) releaseKeys() {
	if w.engine == nil {
		clear(w.keysHeld)
		return
	}
	for code, k := range w.keysHeld {
		delete(w.keysHeld, code)
		w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: k.id, Text: k.name})
	}
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

func hit(v *view, p driver.Point, match func(*view) bool) *view {
	return viewtree.Hit(v, p, match)
}

func (v *view) PointerVisibility() (hidden, transparent bool) {
	return v.style.Hidden, v.style.Opacity <= 0
}

func (v *view) PointerChildren(local driver.Point) (driver.Point, []*view) {
	return driver.Point{X: local.X + v.offset.X, Y: local.Y + v.offset.Y}, v.children
}

func (v *view) ScrollbarContains(local driver.Point) bool {
	return v.kind == driver.KindScroll && !v.scroll.HideIndicator && !v.scroll.Behavior.NoScroll && v.bar().Contains(local)
}

func (v *view) PointerReach(p driver.Point) (local driver.Point, self, children bool) {
	local = v.local(p)
	if math.IsInf(float64(local.X), 0) || math.IsInf(float64(local.Y), 0) {
		return local, false, false
	}
	size := frameOf(v).Size
	self, children = driver.ReachView(size, &v.props, v.overflow, v.transformedKids, local)
	if self && v.image.HitTestAlpha > 0 && v.window.pointerMode == driver.WindowPointerPainted {
		self = v.window.imageHit(v, size, local)
	}
	return local, self, children
}

// reach reports which parts of v the point reaches, in its parent's content coordinates.
func (v *view) reach(p driver.Point) (self, children bool) {
	_, self, children = v.PointerReach(p)
	return self, children
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
