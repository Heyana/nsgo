package core

import "github.com/oneo-me/nsgo/internal/driver"

// Desktop windows with the platform's native chrome. A custom title bar
// calls SetTitleBar once, marks its background with View.WindowDrag and
// keeps clear of the edges Env.TitleBar reports. Platforms without
// desktop windows ignore all of it.

func (cx *Scope) window(c WindowCommand) {
	if t := cx.rt.tree; t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemWindow, Window: &c})
		t.changed = true
	}
}

// SetTitleBar hides the system's title bar so the app can draw its own
// height points tall (0: the platform's height), or restores it. The
// platform centers its window controls in that height. Env.TitleBar then
// reports the area they keep.
func (cx *Scope) SetTitleBar(style TitleBarStyle, height Points) {
	cx.window(WindowCommand{Action: WindowSetTitleBar, TitleBar: style, Height: min(max(height, 0), 200)})
}

// MinimizeWindow minimizes the window.
func (cx *Scope) MinimizeWindow() { cx.window(WindowCommand{Action: WindowMinimize}) }

// ToggleMaximize maximizes (zooms) the window, or restores it.
func (cx *Scope) ToggleMaximize() { cx.window(WindowCommand{Action: WindowToggleMaximize}) }

// ToggleFullScreen enters or leaves full screen.
func (cx *Scope) ToggleFullScreen() { cx.window(WindowCommand{Action: WindowToggleFullScreen}) }

// CloseWindow closes the window as its close button does; the app ends
// with it.
func (cx *Scope) CloseWindow() { cx.window(WindowCommand{Action: WindowClose}) }

// MenuItem is one item of the application menu: an action, a submenu when
// it has Items, or a line between groups when Separator. The top-level
// items are the menu bar's menus; on macOS the first is the app menu.
type MenuItem struct {
	Label string
	// Shortcut is in OnKey's form, such as "mod+n".
	Shortcut  string
	Role      MenuRole
	Disabled  bool
	Checkable bool
	Checked   bool
	Separator bool
	Items     []MenuItem
	// OnSelect runs when the item is chosen, by a click or its shortcut.
	OnSelect func()
}

type menuAction struct {
	owner *Scope
	fn    func()
}

// SetApplicationMenu replaces the application menu: macOS's menu bar, the
// window's menu bar on Windows and Linux. nil restores the platform's
// default. The items' handlers run under the current owner; when it goes,
// the default menu comes back.
func (cx *Scope) SetApplicationMenu(items []MenuItem) {
	t := cx.rt.tree
	if t == nil {
		return
	}
	t.menuGen++
	gen := t.menuGen
	t.menuActions = map[uint32]menuAction{}
	owner := cx.current()
	var entries []MenuEntry
	if items != nil {
		entries = t.menuEntries(items, owner)
		owner.addCleanup(func() {
			if t.menuGen == gen {
				t.menuActions = nil
				t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemMenu})
				t.changed = true
			}
		})
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemMenu, Menu: entries})
	t.changed = true
}

func (t *Tree) menuEntries(items []MenuItem, owner *Scope) []MenuEntry {
	out := make([]MenuEntry, 0, len(items))
	for _, it := range items {
		e := MenuEntry{Label: it.Label, Shortcut: it.Shortcut, Role: it.Role, Disabled: it.Disabled,
			Checkable: it.Checkable || it.Checked, Checked: it.Checked, Separator: it.Separator}
		if e.Label == "" {
			e.Label = it.Role.Label()
		}
		if e.Shortcut != "" {
			e.Shortcut = driver.NormalizeKey(e.Shortcut)
		}
		if len(it.Items) > 0 {
			e.Items = t.menuEntries(it.Items, owner)
		}
		if it.OnSelect != nil {
			t.menuID++
			e.ID = t.menuID
			t.menuActions[e.ID] = menuAction{owner, it.OnSelect}
		}
		out = append(out, e)
	}
	return out
}

func (t *Tree) chooseMenu(id uint32) {
	if a, ok := t.menuActions[id]; ok && !a.owner.disposed {
		t.within(a.owner, a.fn)
	}
}
