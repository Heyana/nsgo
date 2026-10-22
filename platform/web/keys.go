//go:build js && wasm

package web

import (
	"strconv"
	"strings"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
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

func (s *surface) setKeymap(keys []string) {
	s.keys = make(map[string]bool, len(keys))
	for _, k := range keys {
		s.keys[k] = true
	}
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

// fieldKeys are the keys without modifiers that bindings take from a
// text field.
var fieldKeys = map[string]bool{"up": true, "down": true, "pageup": true, "pagedown": true}

// shortcut consumes a key press that a binding of the keymap names,
// with "mod" or with the literal modifier, and reports it. In a text
// field only presses with Control, Alt or Command, function keys, and
// Up, Down, Page Up and Page Down, which a one-line field barely uses,
// count, so bindings never take typing yet can move through a list
// from its search field.
func (s *surface) shortcut(e js.Value) {
	if len(s.keys) == 0 || len(s.alerts) > 0 || e.Get("isComposing").Bool() {
		return
	}
	name := keyName(e)
	if name == "" {
		return
	}
	ctrl, alt, shift, meta := e.Get("ctrlKey").Bool(), e.Get("altKey").Bool(), e.Get("shiftKey").Bool(), e.Get("metaKey").Bool()
	if editing(s.doc.Get("activeElement")) && !ctrl && !alt && !meta && !fieldKeys[name] &&
		!(len(name) > 1 && name[0] == 'f' && name[1] >= '1' && name[1] <= '9') {
		return
	}
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
	for _, k := range names {
		if s.keys[k] {
			e.Call("preventDefault")
			e.Call("stopPropagation")
			s.engine.Dispatch(core.Event{Kind: core.EventKey, Text: k})
			return
		}
	}
}
