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
// with it, unless nsgo.KeepAliveOnClose keeps it running on macOS.
func (cx *Scope) CloseWindow() { cx.window(WindowCommand{Action: WindowClose}) }

// SetWindowFrame moves and sizes the window: f's top left corner is in
// points from the main screen's top left on every platform, and fields
// it leaves out keep their values, so a window given only a new height
// keeps its top edge. macOS animates the change when f asks. Linux sets
// the size GTK keeps for the window, and leaves its position to the
// compositor.
func (cx *Scope) SetWindowFrame(f WindowFrame) {
	f.Width, f.Height = max(f.Width, 0), max(f.Height, 0)
	cx.window(WindowCommand{Action: WindowSetFrame, Frame: f})
}

// SetWindowLevel puts the window at level among the others, on macOS
// and Windows; Linux leaves stacking to the compositor.
func (cx *Scope) SetWindowLevel(level WindowLevel) {
	cx.window(WindowCommand{Action: WindowSetLevel, Level: level})
}

// OrderFront shows the window, restoring it when minimized or hidden,
// brings it to the front of its level and activates the app.
func (cx *Scope) OrderFront() { cx.window(WindowCommand{Action: WindowOrderFront}) }

// SetMenuBar says when Windows and Linux show the menu bar the window
// draws for the application menu; macOS's is the system's and always
// shows.
func (cx *Scope) SetMenuBar(mode MenuBarMode) {
	cx.window(WindowCommand{Action: WindowSetMenuBar, MenuBar: mode})
}

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
		entries = t.menuEntries(items, owner, t.menuActions)
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

// popupMenu is the context menu PopupMenu shows: one at a time.
type popupMenu struct {
	id      uint32
	actions map[uint32]menuAction
	owner   *Scope
	closed  func()
}

// PopupMenu shows items as the platform's own context menu, its top left
// corner at at, in the points of the root view, as a press's or
// OnContextMenu's PagePosition. closed, when not nil, runs once it closes, after
// the OnSelect of an item chosen. A menu shown before closes first.
// Desktops show it; elsewhere closed runs at once.
func (cx *Scope) PopupMenu(items []MenuItem, at Point, closed func()) {
	t := cx.rt.tree
	if t == nil {
		return
	}
	owner := cx.current()
	if os := cx.Env().os.GetUntracked(); os != OSMacOS && os != OSWindows && os != OSLinux {
		if closed != nil {
			t.within(owner, closed)
		}
		return
	}
	t.popupClosed(t.popup.id)
	t.alertID++
	// The menu before it can no longer report a choice.
	t.popup = popupMenu{id: t.alertID, actions: map[uint32]menuAction{}, owner: owner, closed: closed}
	entries := t.menuEntries(items, owner, t.popup.actions)
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemPopupMenu, Menu: entries, Point: at,
		Count: t.popup.id})
	t.changed = true
}

// popupClosed runs the closed callback of context menu id once it
// closed. Its items stay choosable until another menu replaces it, so a
// platform may report the item chosen after the menu closed.
func (t *Tree) popupClosed(id uint32) {
	p := t.popup
	if id == 0 || p.id != id {
		return
	}
	t.popup.id, t.popup.closed = 0, nil
	if p.closed != nil {
		owner := p.owner
		if owner.disposed {
			owner = t.root
		}
		t.within(owner, p.closed)
	}
}

func (t *Tree) menuEntries(items []MenuItem, owner *Scope, actions map[uint32]menuAction) []MenuEntry {
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
			e.Items = t.menuEntries(it.Items, owner, actions)
		}
		if it.OnSelect != nil {
			t.menuID++
			e.ID = t.menuID
			actions[e.ID] = menuAction{owner, it.OnSelect}
		}
		out = append(out, e)
	}
	return out
}

func (t *Tree) chooseMenu(id uint32) {
	a, ok := t.menuActions[id]
	if !ok {
		a, ok = t.popup.actions[id]
	}
	if ok && !a.owner.disposed {
		t.within(a.owner, a.fn)
	}
}
