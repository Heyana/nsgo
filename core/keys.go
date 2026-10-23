package core

import (
	"slices"
	"strings"
)

// Keyboard shortcuts: the surface's table of bindings goes to the
// platform, which matches each key press on the main thread, consumes
// the presses that match and reports them with EventKey.

type keyBinding struct {
	key   string
	owner *Scope
	fn    func()
}

// OnKey runs fn when key is pressed, while the current owner lives. key
// names modifiers and a key joined by "+": "mod+s" (Command on Apple
// platforms, Control elsewhere), "ctrl+shift+z", "alt+left", "escape",
// "enter", "f5", "a". The newest binding of a key wins.
func (cx *Scope) OnKey(key string, fn func()) {
	t := cx.rt.tree
	if t == nil {
		return
	}
	b := &keyBinding{key: NormalizeKey(key), owner: cx.current(), fn: fn}
	t.keys = append(t.keys, b)
	t.keysChanged = true
	t.changed = true
	b.owner.addCleanup(func() {
		t.keys = slices.DeleteFunc(t.keys, func(k *keyBinding) bool { return k == b })
		t.keysChanged = true
		t.changed = true
	})
}

// NormalizeKey returns key lowercased with its modifiers in the order
// mod, ctrl, alt, shift, meta; "cmd" and "command" are meta, "option" is
// alt and "control" is ctrl.
func NormalizeKey(key string) string {
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(key, " ", "")), "+")
	name := parts[len(parts)-1]
	alias := map[string]string{"cmd": "meta", "command": "meta", "option": "alt", "control": "ctrl"}
	var given []string
	for _, m := range parts[:len(parts)-1] {
		if a, ok := alias[m]; ok {
			m = a
		}
		given = append(given, m)
	}
	var mods []string
	for _, m := range []string{"mod", "ctrl", "alt", "shift", "meta"} {
		if slices.Contains(given, m) {
			mods = append(mods, m)
		}
	}
	return strings.Join(append(mods, name), "+")
}

func (t *Tree) keymap() []string {
	var out []string
	for _, b := range t.keys {
		if !slices.Contains(out, b.key) {
			out = append(out, b.key)
		}
	}
	return out
}

func (t *Tree) pressKey(key string) {
	key = NormalizeKey(key)
	for i := len(t.keys) - 1; i >= 0; i-- {
		if b := t.keys[i]; b.key == key && !b.owner.disposed {
			t.within(b.owner, b.fn)
			return
		}
	}
}

// SetTitle sets the window's title on desktops and the page's on the web.
func (cx *Scope) SetTitle(title string) {
	if t := cx.rt.tree; t != nil {
		t.commands = append(t.commands, MountItem{Kind: ItemTitle, Value: title})
		t.changed = true
	}
}
