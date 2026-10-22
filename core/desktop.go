package core

// Desktop windows with the platform's native chrome. A custom title bar
// calls SetTitleBar once, marks its background with View.WindowDrag and
// keeps clear of the edges Env.TitleBar reports. Platforms without
// desktop windows ignore all of it.

// TitleBarStyle is whether the system draws the window's title bar.
type TitleBarStyle uint8

const (
	// TitleBarDefault is the system's title bar, above the app.
	TitleBarDefault TitleBarStyle = iota
	// TitleBarHidden lets the app fill the window up to its top edge and
	// draw its own title bar. The system's window controls stay, in the
	// area Env.TitleBar reports.
	TitleBarHidden
)

// WindowDrag marks a view as a handle that moves the desktop window, or
// keeps a view inside such a handle clickable. The nearest view that
// sets it decides; inputs and views with press or gesture handlers
// nearer than it stay clickable either way.
type WindowDrag uint8

const (
	// WindowDragInherit follows the views around it.
	WindowDragInherit WindowDrag = iota
	// WindowDragOn drags the window, and a double click zooms it as the
	// system's title bar does.
	WindowDragOn
	// WindowDragOff keeps presses on the view for the app.
	WindowDragOff
)

// WindowAction is what an ItemWindow asks of the window.
type WindowAction uint8

const (
	// WindowSetTitleBar shows or hides the title bar (TitleBar, Height).
	WindowSetTitleBar WindowAction = iota + 1
	WindowMinimize
	// WindowToggleMaximize maximizes the window, or restores it.
	WindowToggleMaximize
	WindowToggleFullScreen
	// WindowClose closes the window as its close button does.
	WindowClose
)

// WindowCommand is an ItemWindow's request.
type WindowCommand struct {
	Action   WindowAction
	TitleBar TitleBarStyle
	// Height is the custom title bar's height, in which the platform
	// centers its window controls; 0 is the platform's.
	Height Points
}

func (cx *Scope) window(c WindowCommand) {
	if t := cx.rt.tree; t != nil {
		t.commands = append(t.commands, MountItem{Kind: ItemWindow, Window: &c})
		t.changed = true
	}
}

// SetTitleBar hides the system's title bar so the app can draw its own
// height points tall (0: the platform's height), or restores it.
// Env.TitleBar then reports the area the window controls keep.
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

// MenuRole is a standard menu item the platform carries out, labelling it
// and giving it its usual shortcut when the item has none of its own. The
// app is not told when it is chosen.
type MenuRole uint8

const (
	MenuRoleNone MenuRole = iota
	// MenuAbout, MenuHide, MenuHideOthers and MenuShowAll are macOS's.
	MenuAbout
	MenuHide
	MenuHideOthers
	MenuShowAll
	MenuQuit
	// The edit roles act on the focused text field.
	MenuUndo
	MenuRedo
	MenuCut
	MenuCopy
	MenuPaste
	MenuSelectAll
	MenuMinimize
	MenuZoom
	MenuToggleFullScreen
	MenuClose
)

var menuRoleLabels = [...]string{"", "About", "Hide", "Hide Others", "Show All", "Quit", "Undo", "Redo",
	"Cut", "Copy", "Paste", "Select All", "Minimize", "Zoom", "Toggle Full Screen", "Close"}

// Label returns the label of an item with role r and no label of its own.
func (r MenuRole) Label() string {
	if int(r) < len(menuRoleLabels) {
		return menuRoleLabels[r]
	}
	return ""
}

// Shortcut returns the shortcut of an item with role r and no shortcut of
// its own, in OnKey's form, on a Mac (mac) or elsewhere; "" for none.
func (r MenuRole) Shortcut(mac bool) string {
	pick := func(onMac, other string) string {
		if mac {
			return onMac
		}
		return other
	}
	switch r {
	case MenuHide:
		return pick("meta+h", "")
	case MenuHideOthers:
		return pick("alt+meta+h", "")
	case MenuQuit:
		return pick("meta+q", "alt+f4")
	case MenuUndo:
		return "mod+z"
	case MenuRedo:
		return pick("shift+meta+z", "ctrl+y")
	case MenuCut:
		return "mod+x"
	case MenuCopy:
		return "mod+c"
	case MenuPaste:
		return "mod+v"
	case MenuSelectAll:
		return "mod+a"
	case MenuMinimize:
		return pick("meta+m", "")
	case MenuToggleFullScreen:
		return pick("ctrl+meta+f", "f11")
	case MenuClose:
		return "mod+w"
	}
	return ""
}

// EditsText reports whether r acts on the focused text field, whose own
// shortcut it then is.
func (r MenuRole) EditsText() bool { return r >= MenuUndo && r <= MenuSelectAll }

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

// MenuEntry is a MenuItem as an ItemMenu carries it: the item's handler
// replaced by an ID the platform reports with EventMenu (0: none), its
// shortcut normalized and its role's label filled in. A role item without
// a shortcut takes its role's, which the platform fills in.
type MenuEntry struct {
	ID        uint32
	Label     string
	Shortcut  string
	Role      MenuRole
	Disabled  bool
	Checkable bool
	Checked   bool
	Separator bool
	Items     []MenuEntry
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
				t.commands = append(t.commands, MountItem{Kind: ItemMenu})
				t.changed = true
			}
		})
	}
	t.commands = append(t.commands, MountItem{Kind: ItemMenu, Menu: entries})
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
			e.Shortcut = NormalizeKey(e.Shortcut)
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
