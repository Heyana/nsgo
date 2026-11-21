package driver

import (
	"slices"
	"strings"
)

// HotKeyState is the platform's answer to a global shortcut registration.
type HotKeyState uint8

const (
	// HotKeyPending says the platform has not answered the latest registration.
	HotKeyPending HotKeyState = iota
	// HotKeyRegistered says the shortcut is active.
	HotKeyRegistered
	// HotKeyTaken says another registration owns the combination.
	HotKeyTaken
	// HotKeyUnsupported says the key or the current session cannot provide a global shortcut.
	HotKeyUnsupported
)

func (s HotKeyState) String() string {
	switch s {
	case HotKeyPending:
		return "pending"
	case HotKeyRegistered:
		return "registered"
	case HotKeyTaken:
		return "taken"
	case HotKeyUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

// KeyBinding is one binding of the keymap the platform matches key
// presses against, on the main thread.
type KeyBinding struct {
	// Key is normalized, as NormalizeKey writes it.
	Key string
	// InText fires it while a text input or a web view has the keyboard
	// focus.
	InText bool
	// Repeat fires it again on the auto-repeats of a held key; else they
	// are consumed and fire nothing.
	Repeat bool
	// Yields are the focused controls it gives its key to, a subset of what KeyUsedBy says for the key: it takes no press while such a control has the keyboard focus, which an older binding of the key or the control then takes.
	Yields KeyUse
	// ID names the binding: an in-app binding's (ItemKeymap), which EventKey reports when the platform matches it, or a global shortcut's (ItemHotKeys), which EventHotKey reports. The engine never gives an in-app ID to another binding, so a press matched by a keymap that has since changed runs the binding it matched or none.
	ID uint32
	// Title and Group are localized names for keyboard shortcut discovery.
	Title, Group string
	// Priority asks UIKit to deliver an eligible command before text or
	// focus movement, as a binding with an explicit When takes the key.
	Priority bool
}

// KeyCommands returns the titled bindings that currently win their key's
// physical press. names gives the platform's spellings for the same press.
func KeyCommands(keymap []KeyBinding, focus KeyUse, names func(string) []string) []KeyBinding {
	var commands []KeyBinding
	for _, b := range keymap {
		if b.Title != "" {
			if id, _ := FindKey(keymap, names(b.Key), focus, false); id == b.ID {
				commands = append(commands, b)
			}
		}
	}
	return commands
}

// KeyUse is a set of the keys a focused view uses itself, by the kind of view.
type KeyUse uint8

const (
	// KeyUsePress is Return and Space, which press a focused pressable view.
	KeyUsePress KeyUse = 1 << iota
	// KeyUseAdjust is the arrows, Page Up, Page Down, Home and End, which move a focused slider or spin button.
	KeyUseAdjust
	// KeyUseSelect is the arrows, Page Up, Page Down, Home and End, which move the selection or the focus in a focused tab bar, radio group, list, menu or tree.
	KeyUseSelect
	// KeyUseText is the keys a focused text input or web view edits with; InText says whether a binding fires over it.
	KeyUseText
)

// KeyUsedBy returns the focused views that use key, a normalized key, themselves: Return and Space a pressable view, the arrows and the paging keys a slider or a selection. Modified keys are none's.
func KeyUsedBy(key string) KeyUse {
	switch key {
	case "enter", "space":
		return KeyUsePress
	case "up", "down", "left", "right", "home", "end", "pageup", "pagedown":
		return KeyUseAdjust | KeyUseSelect
	}
	return 0
}

// FocusKeyUse returns the keys a focused view uses itself, by its kind, its role and whether it takes presses: a text input or web view its editing keys, a pressable view Return and Space, a slider or spin button its adjusting keys, and a tab bar, radio group, list, menu or tree and their items their moving keys. Each backend's focused view and nsgotest's are read the same way.
func FocusKeyUse(kind ViewKind, role Role, pressable bool) KeyUse {
	var use KeyUse
	if kind == KindInput || kind == KindWeb {
		use |= KeyUseText
	}
	if pressable {
		use |= KeyUsePress
	}
	switch role {
	case RoleAdjustable, RoleSpinButton:
		use |= KeyUseAdjust
	case RoleTab, RoleTabList, RoleRadio, RoleRadioGroup, RoleListbox, RoleOption, RoleMenu, RoleMenuItem,
		RoleMenuItemCheckbox, RoleMenuItemRadio, RoleTree, RoleTreeItem:
		use |= KeyUseSelect
	}
	return use
}

// NormalizeKey returns key lowercased with its modifiers in the order mod, ctrl, alt, shift, meta; "cmd" and "command" are meta, "option" is alt, "control" is ctrl and the key "return" is "enter".
func NormalizeKey(key string) string {
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(key, " ", "")), "+")
	name := parts[len(parts)-1]
	if name == "return" {
		name = "enter"
	}
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

// FiresOver reports whether the binding takes a press while the view with the keyboard focus uses the keys focus says: not from a text input or web view unless InText, nor from a control whose keys it Yields.
func (b KeyBinding) FiresOver(focus KeyUse) bool {
	return (focus&KeyUseText == 0 || b.InText) && focus&b.Yields == 0
}

// FindKey says what a key press does with keymap: names are the press's normalized names (with "mod" and with the literal modifier), focus the keys the view with the keyboard focus uses itself (FocusKeyUse, KeyUseText alone for a backend that knows only whether a text input or web view has it), and repeat whether it is a held key's auto-repeat. A binding takes no press from a focused text input or web view unless InText, nor from a focused control whose keys it Yields. It returns the ID of the binding that fires, or 0, and whether the press is consumed: a binding fires, or a held key's repeat is taken by a binding that fires once.
func FindKey(keymap []KeyBinding, names []string, focus KeyUse, repeat bool) (id uint32, consumed bool) {
	for i := len(keymap) - 1; i >= 0; i-- {
		b := keymap[i]
		if !slices.Contains(names, b.Key) || !b.FiresOver(focus) {
			continue
		}
		if repeat && !b.Repeat {
			return 0, true
		}
		return b.ID, true
	}
	return 0, false
}

// MenuShortcut finds the enabled item of entries or of their enabled submenus that takes a shortcut of names where the focus is, a role's own where it has none, as the platforms read them: on a Mac every item's, elsewhere all but the editing roles', whose keys the focused field handles itself, and on Windows and Linux none whose role their menus leave out (MenuRoleUnsupported, with speak saying whether the platform can speak). On a Mac the first such item in menu order takes the key, as AppKit's menu does; Windows and Linux try the press's names in order and take the first item registered for that spelling. A disabled item takes nothing, and on a Mac it keeps the key from the items after it too, as AppKit stops at the first key equivalent that matches. While editing (a text input or web view has the focus), a NotInText item takes nothing and keeps its key from the items after it, since each desktop stops at it: macOS disables it during the key press, and Windows and Linux keep the first item of each shortcut. Linux offers no key pressed without Control, Alt or a logo key to the menu while editing.
func MenuShortcut(entries []MenuEntry, names []string, os OS, editing, speak bool) (MenuEntry, bool) {
	if os == OSLinux && editing && !slices.ContainsFunc(names, commandKey) {
		return MenuEntry{}, false
	}
	if os != OSMacOS {
		return menuShortcutByName(entries, names, os, editing, speak)
	}
	e, ok, _ := menuShortcut(entries, names, editing)
	return e, ok
}

// menuShortcutByName follows Windows and Linux, which register the first enabled item of each spelling and then try the press's spellings in order.
func menuShortcutByName(entries []MenuEntry, names []string, os OS, editing, speak bool) (MenuEntry, bool) {
	items := map[string]MenuEntry{}
	kept := map[string]bool{}
	var visit func([]MenuEntry)
	visit = func(entries []MenuEntry) {
		for _, e := range entries {
			switch {
			case e.Separator, e.Disabled, MenuRoleUnsupported(e.Role, os, speak):
			case len(e.Items) > 0:
				visit(e.Items)
			case e.Role.EditsText():
			default:
				shortcut := MenuShortcutKey(e, false)
				if shortcut == "" || kept[shortcut] {
					continue
				}
				if editing && e.NotInText {
					kept[shortcut] = true
					continue
				}
				if _, taken := items[shortcut]; !taken {
					items[shortcut] = e
				}
			}
		}
	}
	visit(entries)
	for _, name := range names {
		if e, ok := items[name]; ok {
			return e, true
		}
	}
	return MenuEntry{}, false
}

// MenuRoleUnsupported reports whether os's menus leave out an item of role, which then takes no key: Windows and Linux leave out macOS's own roles (About, Hide, Hide Others, Show All, Services and Bring All to Front), and the Speech items where nothing speaks (speak, Env.CanSpeak).
func MenuRoleUnsupported(role MenuRole, os OS, speak bool) bool {
	return os != OSMacOS && (role == MenuAbout || role == MenuHide || role == MenuHideOthers || role == MenuShowAll ||
		role == MenuServices || role == MenuBringAllToFront ||
		(role == MenuStartSpeaking || role == MenuStopSpeaking) && !speak)
}

// MenuShortcutKey returns an item's shortcut, or its role's when it has none, normalized as key bindings are.
func MenuShortcutKey(e MenuEntry, mac bool) string {
	if e.Shortcut != "" {
		return e.Shortcut
	}
	return NormalizeKey(e.Role.Shortcut(mac))
}

// commandKey reports whether the normalized key is pressed with Control, Alt or a logo key, Shift not counting.
func commandKey(key string) bool {
	parts := strings.Split(key, "+")
	return slices.ContainsFunc(parts[:len(parts)-1], func(m string) bool { return m == "mod" || m == "ctrl" || m == "alt" || m == "meta" })
}

// menuShortcut is MenuShortcut on a Mac, also reporting whether an item ended the search: AppKit stops at the first key equivalent that matches, a disabled item's or, while editing, a NotInText one's.
func menuShortcut(entries []MenuEntry, names []string, editing bool) (found MenuEntry, ok, stopped bool) {
	for _, e := range entries {
		shortcut := MenuShortcutKey(e, true)
		has := shortcut != "" && slices.Contains(names, shortcut)
		switch {
		case e.Separator:
		case len(e.Items) > 0:
			if e.Disabled {
				continue
			}
			if found, ok, stopped := menuShortcut(e.Items, names, editing); ok || stopped {
				return found, ok, stopped
			}
		case e.Disabled, e.NotInText && editing:
			if has {
				return MenuEntry{}, false, true
			}
		case has:
			return e, true, false
		}
	}
	return MenuEntry{}, false, false
}
