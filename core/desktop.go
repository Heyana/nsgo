package core

import (
	"path/filepath"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Desktop windows with the platform's native chrome and their menus: the
// core's part of package desktop, which apps use. Platforms without
// desktop windows ignore all of it.

func (cx *Scope) window(c driver.WindowCommand) {
	if t := cx.tree(); t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemWindow, Window: &c})
		t.changed = true
	}
}

// SetTitleBar is the core's part of desktop.SetTitleBar, which apps use.
func (cx *Scope) SetTitleBar(style TitleBarStyle, height Points) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetTitleBar, TitleBar: style, Height: min(max(height, 0), 200)})
}

// MinimizeWindow is the core's part of desktop.MinimizeWindow, which apps use.
func (cx *Scope) MinimizeWindow() { cx.window(driver.WindowCommand{Action: driver.WindowMinimize}) }

// ToggleMaximize is the core's part of desktop.ToggleMaximize, which apps use.
func (cx *Scope) ToggleMaximize() {
	cx.window(driver.WindowCommand{Action: driver.WindowToggleMaximize})
}

// ToggleFullScreen is the core's part of desktop.ToggleFullScreen, which apps use.
func (cx *Scope) ToggleFullScreen() {
	cx.window(driver.WindowCommand{Action: driver.WindowToggleFullScreen})
}

// SetFullScreen is the core's part of desktop.SetFullScreen, which apps use.
func (cx *Scope) SetFullScreen(on bool, displayID ...string) {
	c := driver.WindowCommand{Action: driver.WindowSetFullScreen, On: on}
	if on && len(displayID) != 0 {
		c.DisplayID = displayID[0]
	}
	cx.window(c)
}

// CloseWindow is the core's part of desktop.CloseWindow, which apps use.
func (cx *Scope) CloseWindow() { cx.window(driver.WindowCommand{Action: driver.WindowClose}) }

// SetWindowFrame is the core's part of desktop.SetWindowFrame, which apps use.
func (cx *Scope) SetWindowFrame(f WindowFrame) {
	f.Width, f.Height = max(f.Width, 0), max(f.Height, 0)
	if w := cx.CurrentWindow(); w != nil && w.edgeDocked {
		if f.Width == 0 && f.Height == 0 {
			return
		}
		size := cx.Env().Window()
		if f.Width == 0 {
			f.Width = size.Width
		}
		if f.Height == 0 {
			f.Height = size.Height
		}
		cx.window(driver.WindowCommand{Action: driver.WindowFitSize, Frame: WindowFrame{Width: f.Width, Height: f.Height, Animated: f.Animated}})
		return
	}
	cx.window(driver.WindowCommand{Action: driver.WindowSetFrame, Frame: f})
}

// SetWindowLevel is the core's part of desktop.SetWindowLevel, which apps use.
func (cx *Scope) SetWindowLevel(level WindowLevel) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetLevel, Level: level})
}

// SetWindowWorkspaces is the core's part of desktop.SetWindowWorkspaces, which apps use.
func (cx *Scope) SetWindowWorkspaces(w WindowWorkspaces) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetWorkspaces, Workspaces: w})
}

// SetWindowPointerEvents is the core's part of desktop.SetWindowPointerEvents, which apps use.
func (cx *Scope) SetWindowPointerEvents(p WindowPointerEvents) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetPointerEvents, PointerEvents: cx.windowPointer(p)})
}

// windowPointer is p as code using cx may set it: a window that lets the pointer through to other apps, as an overlay does, takes AccessOverlay, as NSH's windows capability's overlay grants it; without it the window takes the pointer.
func (cx *Scope) windowPointer(p WindowPointerEvents) WindowPointerEvents {
	if p == WindowPointerAuto || p > WindowPointerPainted || cx.denied(serviceOverlay, "pointerEvents") {
		return WindowPointerAuto
	}
	return p
}

// SetWindowConstraints is the core's part of desktop.SetWindowConstraints, which apps use.
func (cx *Scope) SetWindowConstraints(c WindowConstraints) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetConstraints, Constraints: c.Normalized()})
}

// OrderFront is the core's part of desktop.OrderFront, which apps use.
func (cx *Scope) OrderFront() { cx.window(driver.WindowCommand{Action: driver.WindowOrderFront}) }

// CursorPoint is the core's part of desktop.CursorPoint, which apps use
// to read the pointer in desktop screen coordinates.
func (cx *Scope) CursorPoint(done func(Point, bool)) {
	if done == nil {
		return
	}
	t := cx.rt.tree
	if t != nil && !t.reported.Simulated && !cx.denied(serviceCursorPoint, "") {
		switch t.reported.OS {
		case OSMacOS, OSWindows, OSLinux:
			t.request(cx, driver.MountItem{Kind: driver.ItemCursorPoint}, func(ev driver.Event) {
				done(ev.Point, ev.Finished)
			})
			return
		}
	}
	done(Point{}, false)
}

// FrontmostApp is the core's part of desktop.FrontmostApp, which apps use: the application in front, read tracked, zero where the platform does not follow it (CanReadFrontmostApp) or the current owner's Access refuses AccessFrontmostApp.
func (cx *Scope) FrontmostApp() FrontmostAppInfo {
	if cx.denied(serviceFrontmostApp, "") {
		return FrontmostAppInfo{}
	}
	if env := cx.Env(); env.canReadFrontmost.Get() {
		return env.frontmost.Get()
	}
	return FrontmostAppInfo{}
}

// CanReadFrontmostApp is the core's part of desktop.CanReadFrontmostApp, which apps use: whether FrontmostApp follows the application in front, read tracked, false where the platform does not or the current owner's Access refuses AccessFrontmostApp. Asking logs no refusal.
func (cx *Scope) CanReadFrontmostApp() bool {
	if cx.gate(serviceFrontmostApp, "") != nil {
		return false
	}
	return cx.Env().canReadFrontmost.Get()
}

// SetMenuBar is the core's part of desktop.SetMenuBar, which apps use.
func (cx *Scope) SetMenuBar(mode MenuBarMode) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetMenuBar, MenuBar: mode})
}

// SetWindowControls is the core's part of desktop.SetWindowControls, which apps use.
func (cx *Scope) SetWindowControls(c WindowControls) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetControls, Controls: c})
}

// SetWindowMaterial is the core's part of desktop.SetWindowMaterial, which apps use.
func (cx *Scope) SetWindowMaterial(m WindowMaterial) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetMaterial, Material: m})
}

// SetWindowTransparent is the core's part of desktop.SetWindowTransparent, which apps use.
func (cx *Scope) SetWindowTransparent(on bool) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetTransparent, On: on})
}

// SetWindowShadow is the core's part of desktop.SetWindowShadow, which apps use.
func (cx *Scope) SetWindowShadow(on bool) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetShadow, On: on})
}

// SetSkipTaskbar is the core's part of desktop.SetSkipTaskbar, which apps use.
func (cx *Scope) SetSkipTaskbar(on bool) {
	cx.window(driver.WindowCommand{Action: driver.WindowSetSkipTaskbar, On: on})
}

// MenuItem is one item of a menu: an action, a submenu when it has
// Items, or a line between groups when Separator. Apps name it
// desktop.MenuItem.
type MenuItem struct {
	Label string
	// Icon is an optional icon shown beside the item in native menus on macOS, Windows and Linux.
	Icon MenuIcon
	// Shortcut is in OnKey's form, such as "mod+n".
	Shortcut  string
	Role      MenuRole
	Disabled  bool
	Checkable bool
	Checked   bool
	// Radio makes the item checkable and shows it as one of a group of choices, a radio on Windows and Linux; the app still says which item is Checked.
	Radio     bool
	Separator bool
	Items     []MenuItem
	// When says whether the shortcut chooses the item while a text input
	// or a web view has the keyboard focus.
	When MenuWhen
	// OnSelect runs when the item is chosen, by a click or its shortcut. An editing Role (MenuUndo through MenuSelectAll, MenuPasteAndMatchStyle and MenuDelete) goes to the focused field or web view first, and Copy and Select All to selected text, so OnSelect runs when none of them takes it: Select All selecting the app's own items, as Finder's does. For every other Role, OnSelect replaces the standard role action, so an item with MenuQuit or MenuMinimize and OnSelect runs the callback without quitting or minimizing.
	OnSelect func()
}

// MenuWhen says when a menu item's shortcut chooses it while a text
// input or a web view has the keyboard focus.
type MenuWhen uint8

const (
	// MenuAlways chooses it there too.
	MenuAlways MenuWhen = iota
	// MenuAuto leaves the input the keys it uses itself (EditsText).
	MenuAuto
	// MenuNotEditing never chooses it there.
	MenuNotEditing
)

type menuAction struct {
	owner *Scope
	fn    func()
}

// SetApplicationMenu is the core's part of desktop.SetApplicationMenu, which apps use.
func (cx *Scope) SetApplicationMenu(items []MenuItem) {
	t := cx.tree()
	if t == nil {
		return
	}
	t.menuGen++
	gen := t.menuGen
	t.menuActions = map[uint32]menuAction{}
	owner := cx.current()
	var entries []driver.MenuEntry
	if items != nil {
		entries = t.menuEntries(items, owner, t.menuActions)
		owner.addCleanup(func() {
			if t.menuGen == gen {
				t.menuActions, t.menu = nil, nil
				t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemMenu})
				t.menuSent()
			}
		})
	}
	// Bindings that yield to the menu (KeyOptions.YieldsToMenu) come and go with its shortcuts.
	t.menu = entries
	t.menuSent()
	// It replaces a menu not sent yet, such as the default one the
	// cleanup of the effect setting it again asked for, which would show
	// for a moment.
	t.commands = slices.DeleteFunc(t.commands, func(it driver.MountItem) bool { return it.Kind == driver.ItemMenu })
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemMenu, Menu: entries})
	t.changed = true
}

// menuSent follows the application menu the tree sent: bindings that yield to the menu (KeyOptions.YieldsToMenu) come and go with its shortcuts, in every surface on macOS, whose one menu bar shows the menu a surface sent last.
func (t *Tree) menuSent() {
	t.rt.menuOrder++
	t.menuOrder = t.rt.menuOrder
	t.rt.menuBar = t
	t.keysChanged = true
	t.changed = true
	if t.root.Env().hostUntracked() != OSMacOS {
		return
	}
	for _, o := range append([]*Tree{t.rt.tree}, t.rt.surfaces...) {
		if o != nil && !o.closed {
			o.keysChanged = true
			o.changed = true
		}
	}
}

// menuClosed restores the last menu sent by a surface that is still open. Closing the current menu owner must also make every tree recompute keys that yield to the menu, as menuSent does.
func (t *Tree) menuClosed() {
	if t.rt.menuBar != t {
		return
	}
	var bar *Tree
	for _, o := range append([]*Tree{t.rt.tree}, t.rt.surfaces...) {
		if o != nil && !o.closed && (bar == nil || o.menuOrder > bar.menuOrder) {
			bar = o
		}
	}
	t.rt.menuBar = bar
	for _, o := range append([]*Tree{t.rt.tree}, t.rt.surfaces...) {
		if o != nil && !o.closed {
			o.keysChanged = true
			o.changed = true
		}
	}
}

// dockMenu is the app icon's menu (SetDockMenu). It belongs to the app rather than a surface, as the platform has one, so it is the one a surface set last, and its tasks, named by their labels, reach it from any surface.
type dockMenu struct {
	tree    *Tree
	gen     uint32
	entries []driver.MenuEntry
	// actions are nil while no menu is set.
	actions map[uint32]menuAction
	// pending are the labels of tasks chosen before the app set a menu, as the one a Windows Jump List started the app with; set is whether it has set one.
	pending []string
	set     bool
}

// SetDockMenu is the core's part of desktop.SetDockMenu, which apps use. The menu belongs to the current owner, and replaces the one any surface set before.
func (cx *Scope) SetDockMenu(items []MenuItem) {
	t := cx.tree()
	if t == nil {
		return
	}
	d := &cx.rt.dock
	// The engine commits the app surface before its windows, regardless of which scope set the menu last. Drop an earlier surface's pending menu before changing ownership, so it cannot mount after the replacement.
	if previous := d.tree; previous != nil && previous != t {
		previous.commands = slices.DeleteFunc(previous.commands, func(it driver.MountItem) bool { return it.Kind == driver.ItemDockMenu })
	}
	d.gen++
	gen := d.gen
	d.tree, d.entries, d.actions = t, nil, nil
	owner := cx.current()
	if items != nil {
		d.set = true
		d.actions = map[uint32]menuAction{}
		d.entries = t.menuEntries(items, owner, d.actions)
		owner.addCleanup(func() {
			if d.gen == gen {
				d.entries, d.actions = nil, nil
				t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemDockMenu})
				t.changed = true
			}
		})
	}
	// It replaces a menu not sent yet, as SetApplicationMenu's does.
	t.commands = slices.DeleteFunc(t.commands, func(it driver.MountItem) bool { return it.Kind == driver.ItemDockMenu })
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemDockMenu, Menu: d.entries})
	t.changed = true
	if pending := d.pending; d.actions != nil && len(pending) > 0 {
		d.pending = nil
		// Not while the caller builds its menu: the item may well change what it lists.
		owner.Post(func() {
			for _, label := range pending {
				t.chooseDockTask(label)
			}
		})
	}
}

// AddRecentFile is the core's part of desktop.AddRecentFile, which apps use. A relative path is taken from the working directory.
func (cx *Scope) AddRecentFile(path string) {
	t := cx.tree()
	if t == nil || path == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemRecentFile, Value: path})
	t.changed = true
}

// ClearRecentFiles is the core's part of desktop.ClearRecentFiles, which apps use.
func (cx *Scope) ClearRecentFiles() {
	if t := cx.tree(); t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemRecentFile, On: true})
		t.changed = true
	}
}

// popupMenu is the context menu PopupMenu shows: one at a time.
type popupMenu struct {
	id      uint32
	actions map[uint32]menuAction
	owner   *Scope
	closed  func()
}

// PopupMenu is the core's part of desktop.PopupMenu, which apps use. A confined owner shows one only as Scope.Confine allows, else closed runs at once.
func (cx *Scope) PopupMenu(items []MenuItem, at Point, closed func()) {
	t := cx.tree()
	if t == nil {
		return
	}
	owner := cx.current()
	if os := cx.Env().hostUntracked(); os != OSMacOS && os != OSWindows && os != OSLinux ||
		!t.confinedMay(owner, refusedPresent, askedToPresent) {
		if closed != nil {
			t.within(owner, closed)
		}
		return
	}
	t.popupClosed(t.popup.id, false)
	t.alertID++
	// The menu before it can no longer report a choice.
	t.popup = popupMenu{id: t.alertID, actions: map[uint32]menuAction{}, owner: owner, closed: closed}
	entries := t.menuEntries(items, owner, t.popup.actions)
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemPopupMenu, Menu: entries, Point: at,
		Count: t.popup.id})
	t.changed = true
}

// popupClosed runs the closed callback of context menu id once it closed: by the user when the platform reports it, else as another menu replaces it. Its items stay choosable until another menu replaces it, so a platform may report the item chosen after the menu closed.
func (t *Tree) popupClosed(id uint32, user bool) {
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
		if user {
			// The user closed it, choosing or not (mayMoveFocus); a menu replacing it is not the user's input on its owner's views.
			t.activated = owner
		}
		t.within(owner, p.closed)
	}
}

func (t *Tree) menuEntries(items []MenuItem, owner *Scope, actions map[uint32]menuAction) []driver.MenuEntry {
	out := make([]driver.MenuEntry, 0, len(items))
	for _, it := range items {
		icon := it.Icon.Clone()
		if !icon.Source.Empty() {
			t.refuseImageOwner(owner, &icon.Source)
		}
		e := driver.MenuEntry{Label: it.Label, Icon: icon, Shortcut: it.Shortcut, Role: it.Role, Disabled: it.Disabled,
			Checkable: it.Checkable || it.Checked || it.Radio, Checked: it.Checked, Radio: it.Radio, Separator: it.Separator}
		if e.Label == "" {
			e.Label = it.Role.Label()
		}
		if e.Shortcut != "" {
			e.Shortcut = driver.NormalizeKey(e.Shortcut)
		}
		shortcut := e.Shortcut
		if shortcut == "" && it.Role != MenuRoleNone {
			shortcut = driver.NormalizeKey(it.Role.Shortcut(false))
		}
		e.NotInText = it.When == MenuNotEditing || it.When == MenuAuto && shortcut != "" && EditsText(shortcut)
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
		// Choosing it is the user's input on its owner's views (mayMoveFocus).
		t.activated = a.owner
		t.within(a.owner, a.fn)
	}
}

// chooseDockMenu runs the app icon menu's item ev names: by its ID (Count) on the surface that set the menu, as macOS's Dock menu reports it, or, for a task the platform keeps apart from the app, as a Windows Jump List's, by its label (Text) on any surface.
func (t *Tree) chooseDockMenu(ev driver.Event) {
	if ev.Text != "" {
		t.chooseDockTask(ev.Text)
		return
	}
	d := &t.rt.dock
	if d.tree != t {
		return
	}
	if a, ok := d.actions[ev.Count]; ok && !a.owner.disposed {
		d.tree.activated = a.owner
		d.tree.within(a.owner, a.fn)
	}
}

// chooseDockTask runs the first enabled top-level action labeled label of the app icon's menu, or keeps the label for the first menu the app sets when it has set none yet. A label the menu lacks, or one chosen after the menu went, as from a Jump List Windows kept, does nothing.
func (t *Tree) chooseDockTask(label string) {
	d := &t.rt.dock
	if d.actions == nil {
		if !d.set {
			d.pending = append(d.pending, label)
		}
		return
	}
	for _, e := range d.entries {
		if e.Label != label || e.ID == 0 || e.Disabled || len(e.Items) > 0 {
			continue
		}
		if a, ok := d.actions[e.ID]; ok && !a.owner.disposed {
			d.tree.activated = a.owner
			d.tree.within(a.owner, a.fn)
		}
		return
	}
}
