//go:build js && wasm

package web

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// keyNames are the normalized names of the keys whose name is not their
// character.
var keyNames = map[string]string{
	"Enter": "enter", "Escape": "escape", "Tab": "tab", " ": "space", "Backspace": "backspace",
	"Delete": "delete", "ArrowUp": "up", "ArrowDown": "down", "ArrowLeft": "left", "ArrowRight": "right",
	"Home": "home", "End": "end", "PageUp": "pageup", "PageDown": "pagedown",
}

// apple reports whether "mod" is Command here.
var apple = func() bool {
	nav := js.Global().Get("navigator")
	platform := nav.Get("platform")
	if d := nav.Get("userAgentData"); d.Truthy() && d.Get("platform").Truthy() {
		platform = d.Get("platform")
	}
	p := platform.String()
	return strings.Contains(p, "Mac") || strings.Contains(p, "iPhone") ||
		strings.Contains(p, "iPad") || strings.Contains(p, "macOS")
}()

// setKeymap takes the keymap's bindings and whether the app asked for every key or mouse button held (MountItem.HeldKeys, HeldMouseButtons).
func (s *surface) setKeymap(keys []driver.KeyBinding, held, heldMouse bool) {
	started := held && !s.heldKeys
	s.keys, s.heldKeys, s.heldMouse = keys, held, heldMouse
	if started {
		s.readKeyboardLayout()
	}
}

func (s *surface) readKeyboardLayout() {
	if !s.heldKeys {
		return
	}
	keyboard := js.Global().Get("navigator").Get("keyboard")
	if !keyboard.Truthy() || keyboard.Get("getLayoutMap").Type() != js.TypeFunction {
		return
	}
	promise := keyboard.Call("getLayoutMap")
	go func() {
		layout, err := await(promise)
		if err != nil || !layout.Truthy() {
			return
		}
		names := map[string]string{}
		each := js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) >= 2 {
				name := strings.ToLower(args[0].String())
				if name == " " {
					name = "space"
				}
				names[args[1].String()] = name
			}
			return nil
		})
		layout.Call("forEach", each)
		each.Release()
		s.keyCodeNames = names
		if s.engine != nil {
			s.updateEnv()
		}
	}()
}

// focusKeyUse returns the keys the focused element uses itself, which the keymap's bindings that yield them leave it (driver.FindKey): a field's, a select's or an editable element's editing keys, a pressable view's Return and Space, a slider's or a tab bar's arrows.
func (s *surface) focusKeyUse() driver.KeyUse {
	el := s.doc.Get("activeElement")
	if editing(el) {
		return driver.KeyUseText
	}
	v := s.viewOf(el)
	if v == nil {
		return 0
	}
	return driver.FocusKeyUse(v.kind, v.props.Role, v.pressable())
}

// keyName returns the normalized name of e's key without modifiers, or
// "" for a modifier. Letters and digits follow the layout, unless it
// types none, such as Option on a Mac or a Cyrillic layout; then the
// physical key names them.
func keyName(e js.Value) string {
	key, code := e.Get("key").String(), e.Get("code").String()
	if len(key) == 1 && (key[0] >= 'a' && key[0] <= 'z' || key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9') {
		return strings.ToLower(key)
	}
	switch {
	case len(code) == 4 && strings.HasPrefix(code, "Key"):
		return strings.ToLower(code[3:])
	case len(code) == 6 && strings.HasPrefix(code, "Digit"):
		return code[5:]
	}
	if n, ok := keyNames[key]; ok {
		return n
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(key, "F")); err == nil && key[0] == 'F' && n >= 1 && n <= 24 {
		return "f" + strconv.Itoa(n)
	}
	switch key {
	case "Shift", "Control", "Alt", "AltGraph", "Meta", "CapsLock", "Dead", "Process", "Unidentified":
		return ""
	}
	return strings.ToLower(key)
}

// heldKey is a key down that focusKeyDown reported: the view it went down on, or 0 for none, and the name its up reports again.
type heldKey struct {
	id         driver.NodeID
	name, code string
}

// keyCode returns the physical key of e, which pairs a key's up with its down: its code, or its key where the browser gives no code.
func keyCode(e js.Value) string {
	if code := e.Get("code").String(); code != "" {
		return code
	}
	return e.Get("key").String()
}

// keyModifiers returns the modifier keys e says are held.
func keyModifiers(e js.Value) driver.Modifiers {
	return driver.Modifiers{Control: e.Get("ctrlKey").Bool(), Alt: e.Get("altKey").Bool(), Shift: e.Get("shiftKey").Bool(), Platform: e.Get("metaKey").Bool()}
}

// focusKeyDown offers a key going down to the focused view that hears keys (View.OnKeyDown), before the keymap and the view's own keys, and reports whether it consumed the key, which then goes no further in the page. A modifier alone is reported but never consumed, as no binding takes one. Where no such view has the focus and the app asked for every key held (MountItem.HeldKeys), it reports the down with ID 0 and consumes nothing. Either way it records the physical key with the view and the name, which focusKeyUp reports when the key comes up, also for a down the browser calls a repeat, as a key held before the page heard it. It leaves out keys typed while a text input has the focus or an input method composes.
func (s *surface) focusKeyDown(e js.Value) bool {
	el := s.doc.Get("activeElement")
	v := s.viewOf(el)
	if e.Get("isComposing").Bool() || editing(el) || v != nil && v.kind == driver.KindInput {
		return false
	}
	hears := v != nil && v.events.Has(driver.EventKeyDown)
	if !hears && !s.heldKeys {
		return false
	}
	name, modifier := keyName(e), true
	switch e.Get("key").String() {
	case "Shift":
		name = "shift"
	case "Control":
		name = "ctrl"
	case "Alt":
		name = "alt"
	case "Meta", "OS":
		name = "meta"
	default:
		modifier = false
	}
	if name == "" {
		return false
	}
	var id driver.NodeID
	if hears {
		id = v.id
	}
	mods, repeat, physical := keyModifiers(e), e.Get("repeat").Bool(), e.Get("code").String()
	code := keyCode(e)
	if physical == "Unidentified" {
		physical = ""
	}
	if held, ok := s.keysHeld[code]; !ok || !repeat {
		if ok {
			// A key that goes down again without repeating lost its up, which is reported first.
			s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: held.code, Modifiers: mods})
		}
		if s.keysHeld == nil {
			s.keysHeld = map[string]heldKey{}
		}
		s.keysHeld[code] = heldKey{id: id, name: name, code: physical}
	}
	ev := driver.Event{Kind: driver.EventKeyDown, ID: id, Text: name, KeyCode: physical, Modifiers: mods, Repeat: repeat}
	if !hears {
		// No view can consume it, so it goes without the engine's answer, as a binding's press does: with neither an ID nor a Count.
		s.engine.Dispatch(ev)
		return false
	}
	if !s.engine.DispatchKeyDown(ev) || modifier {
		return false
	}
	e.Call("preventDefault")
	e.Call("stopPropagation")
	return true
}

// focusKeyUp reports a key coming up whose down focusKeyDown recorded, to the view it went down on wherever the focus has gone since, with the name its down reported. Downs and ups pair by the physical key, as Shift changes the character: "!" may go down and "1" come up. On a Mac, Meta coming up also reports every key other than a modifier still held, as the browser there sends no up for such a key that comes up while Command is held, while it still sends the up of Shift, Control, Option and the other Command.
func (s *surface) focusKeyUp(e js.Value) {
	mods, code := keyModifiers(e), keyCode(e)
	if held, ok := s.keysHeld[code]; ok {
		delete(s.keysHeld, code)
		s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: held.code, Modifiers: mods})
	}
	switch e.Get("key").String() {
	case "Meta", "OS":
		if apple {
			s.releaseKeys(mods, false)
		}
	}
}

// releaseKeys reports the keys focusKeyDown recorded coming up, with mods held, and forgets them, as when the page loses the focus and hears no more ups. With modifiers false it leaves the modifier keys recorded, whose own ups the page still hears.
func (s *surface) releaseKeys(mods driver.Modifiers, modifiers bool) {
	for _, code := range slices.Sorted(maps.Keys(s.keysHeld)) {
		held := s.keysHeld[code]
		if !modifiers && modifierName(held.name) {
			continue
		}
		delete(s.keysHeld, code)
		s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: held.code, Modifiers: mods})
	}
}

// modifierName reports whether name is the one focusKeyDown gives a modifier key.
func modifierName(name string) bool {
	switch name {
	case "shift", "ctrl", "alt", "meta":
		return true
	}
	return false
}

// shortcut consumes a key press that a binding of the keymap names,
// with "mod" or with the literal modifier, and reports it, as
// driver.FindKey decides for the focused element and a held key.
func (s *surface) shortcut(e js.Value) {
	if len(s.keys) == 0 || len(s.alerts) > 0 || e.Get("isComposing").Bool() {
		return
	}
	name := keyName(e)
	if name == "" {
		return
	}
	ctrl, alt, shift, meta := e.Get("ctrlKey").Bool(), e.Get("altKey").Bool(), e.Get("shiftKey").Bool(), e.Get("metaKey").Bool()
	join := func(mod, ctrl, alt, shift, meta bool) string {
		var b strings.Builder
		for _, m := range []struct {
			on   bool
			name string
		}{{mod, "mod"}, {ctrl, "ctrl"}, {alt, "alt"}, {shift, "shift"}, {meta, "meta"}} {
			if m.on {
				b.WriteString(m.name + "+")
			}
		}
		return b.String() + name
	}
	names := []string{join(false, ctrl, alt, shift, meta)}
	switch {
	case apple && meta:
		names = append([]string{join(true, ctrl, alt, shift, false)}, names...)
	case !apple && ctrl:
		names = append([]string{join(true, false, alt, shift, meta)}, names...)
	}
	id, consumed := driver.FindKey(s.keys, names, s.focusKeyUse(), e.Get("repeat").Bool())
	if !consumed {
		return
	}
	e.Call("preventDefault")
	e.Call("stopPropagation")
	if id != 0 {
		s.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
	}
}
