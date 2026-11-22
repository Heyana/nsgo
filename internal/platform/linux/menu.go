//go:build linux && !android

package linux

import (
	"fmt"
	"unicode"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/menutext"
)

// roleActionBase separates a role's action data from a menu item's ID.
const roleActionBase = 1 << 20

// setMenu replaces the application menu and its shortcuts; nil restores
// GTK's own menu bar.
func (w *window) setMenu(entries []driver.MenuEntry) {
	w.chrome.menu = entries
	w.chrome.keys = map[string]menuRef{}
	w.menuKeys(entries)
	w.showMenuBar()
}

// menuPopups reports whether this window offers its application menu as a popup instead of a bar, including while a hidden-title-bar window is full screen.
func (w *window) menuPopups() bool {
	return !w.embedded && len(w.chrome.menu) != 0 && w.chrome.titleBar == driver.TitleBarHidden && !(w.primary && w.opts.Hidden)
}

// popupApplicationMenu opens the application menu at the surface's leading top corner. Request zero is internal: it has no PopupMenu close callback.
func (w *window) popupApplicationMenu(entries []driver.MenuEntry) {
	var at driver.Point
	if w.dir == 2 { // GTK_TEXT_DIR_RTL
		at.X = driver.Points(gtkWidgetGetWidth(w.surface))
	}
	w.showPopupMenu(entries, at, 0, true)
}

// restoreMenuFocus gives native menus' actions and dismissal back the editor or view that owned the keyboard before opening. A retained widget may have left the tree while the menu was open; only one still in this window returns.
func (w *window) restoreMenuFocus() {
	if !w.menuFocusPending {
		return
	}
	focus := w.menuFocus
	w.menuFocus, w.menuFocusPending = 0, false
	if focus != 0 {
		defer gObjectUnref(focus)
	}
	if w.closing {
		return
	}
	top := w.toplevel()
	var target uintptr
	for p := focus; p != 0; p = gtkWidgetGetParent(p) {
		if p == top {
			target = focus
			break
		}
	}
	if top != 0 {
		gtkWindowSetFocus(top, target)
	}
}

// menuPopupKey follows Alt alone, F10 and top-level mnemonics in a window with no bar. Other Alt chords, including the window manager's, keep their keys.
func (w *window) menuPopupKey(keyval, state uint32, pressed, repeat bool) bool {
	c := &w.chrome
	alt := keyval == gdkAltL || keyval == gdkAltR
	const other = gdkControl | gdkShift | gdkSuper | gdkMeta
	switch {
	case pressed && alt:
		if !repeat {
			c.altAlone = state&other == 0
		}
	case !pressed && alt:
		alone := c.altAlone
		c.altAlone = false
		if alone {
			w.popupApplicationMenu(c.menu)
			return true
		}
	case pressed:
		c.altAlone = false
		if keyval == gdkF10 && state&(other|gdkAlt) == 0 {
			if !repeat {
				w.popupApplicationMenu(c.menu)
			}
			return true
		}
		if state&gdkAlt == 0 || state&(gdkControl|gdkSuper|gdkMeta) != 0 {
			return false
		}
		key := unicode.ToLower(rune(keyLayoutAPI().unicode(keyval)))
		for i := range c.menu {
			e := &c.menu[i]
			if e.Separator || e.Disabled || !shown(e) || menutext.Mnemonic(menutext.Label(w.language, e, "")) != key || key == 0 {
				continue
			}
			if !repeat {
				if len(e.Items) != 0 {
					w.popupApplicationMenu(e.Items)
				} else {
					w.menuChosen(menuRef{id: e.ID, role: e.Role, notInText: e.NotInText})
				}
			}
			return true
		}
	}
	return false
}

// showMenuBar shows the application menu as a GTK menu bar over the surface, unless the title bar is hidden, the window is full screen or transparent, or an auto-hiding bar is not revealed. A bar already shown is updated in place: its GMenu models and GSimpleActionGroup stay, and only the items that changed are replaced, so setting the menu again (such as checking an item) neither flashes the bar nor closes a menu the user has open.
func (w *window) showMenuBar() {
	if w.popover != 0 && w.popoverID == 0 {
		gtkPopoverPopdown(w.popover)
	}
	c := &w.chrome
	if c.menu == nil || c.titleBar != driver.TitleBarDefault || c.fullScreen || w.transparent ||
		c.menuBar == driver.MenuBarAutoHide && !c.revealed {
		w.clearMenuBar()
		return
	}
	next := buildGTKMenuTree(c.menu, w.language, speakAvailable(), w.menuIconLook())
	if w.menuBar != 0 && w.menuModel != 0 && w.actionGroup != 0 {
		syncGTKMenuItems(gtkMenuBarOps{w}, w.menuModel, w.menuTree, next)
		w.menuTree = next
		// GTK makes rows for inserted items as the model changes; their icons need showing as the first ones did.
		showMenuIcons(w.menuBar)
		return
	}
	w.clearMenuBar()
	group := gSimpleActionGroupNew()
	if group == 0 {
		return
	}
	model := gMenuNew()
	if model == 0 {
		gObjectUnref(group)
		return
	}
	w.actionGroup, w.menuModel = group, model
	syncGTKMenuItems(gtkMenuBarOps{w}, model, nil, next)
	w.menuTree = next
	gtkWidgetInsertActionGroup(w.win, cstr("win"), group)
	bar := gtkPopoverMenuBarNewFromModel(model)
	if bar == 0 {
		w.clearMenuBar()
		return
	}
	showMenuIcons(bar)
	gtkWidgetSetHalign(bar, 1) // start
	gtkBoxPrepend(w.box, bar)
	gtkWidgetSetVisible(bar, 1)
	w.menuBar = bar
}

// menuIconLook names what a menu icon's pixels depend on besides the icon: their size in pixels and, for a template icon, the appearance it is tinted for.
func (w *window) menuIconLook() gtkMenuIconLook {
	return gtkMenuIconLook{size: w.menuIconSize(), dark: w.shownDark()}
}

// clearMenuBar removes the application menu bar and releases its GMenu models, actions and action group.
func (w *window) clearMenuBar() {
	if w.menuBar != 0 {
		// Unparenting drops the bar's only reference.
		gtkWidgetSetVisible(w.menuBar, 0)
		if w.box != 0 {
			gtkBoxRemove(w.box, w.menuBar)
		}
		w.menuBar = 0
	}
	if len(w.menuTree) > 0 {
		releaseGTKMenuTree(gtkMenuBarOps{w}, w.menuTree)
		w.menuTree = nil
	}
	if w.menuModel != 0 {
		gObjectUnref(w.menuModel)
		w.menuModel = 0
	}
	if w.actionGroup != 0 {
		if w.win != 0 {
			gtkWidgetInsertActionGroup(w.win, cstr("win"), 0)
		}
		gObjectUnref(w.actionGroup)
		w.actionGroup = 0
	}
	clear(w.actionRefs)
}

// gtkMenuBarOps applies menu bar sync operations to a window's GMenu models and action group.
type gtkMenuBarOps struct {
	w *window
}

func (gtkMenuBarOps) newMenu() uintptr { return gMenuNew() }

func (gtkMenuBarOps) unrefMenu(menu uintptr) {
	if menu != 0 {
		gObjectUnref(menu)
	}
}

func (o gtkMenuBarOps) insertSubmenu(parent uintptr, pos int32, n *gtkMenuNode) {
	if parent == 0 || n.menu == 0 {
		return
	}
	if n.icon.Empty() {
		gMenuInsertSubmenu(parent, pos, cstr(n.label), n.menu)
		return
	}
	item := gMenuItemNewSubmenu(cstr(n.label), n.menu)
	if item == 0 {
		gMenuInsertSubmenu(parent, pos, cstr(n.label), n.menu)
		return
	}
	o.w.attachMenuIcon(item, n.icon)
	gMenuInsertItem(parent, pos, item)
	gObjectUnref(item)
}

func (gtkMenuBarOps) insertSection(parent uintptr, pos int32, section uintptr) {
	if parent != 0 && section != 0 {
		gMenuInsertSection(parent, pos, nil, section)
	}
}

func (o gtkMenuBarOps) insertItem(parent uintptr, pos int32, n *gtkMenuNode) {
	if parent == 0 {
		return
	}
	var item uintptr
	if n.actionName == "" {
		item = gMenuItemNew(cstr(n.label), nil)
	} else {
		item = gMenuItemNew(cstr(n.label), cstr("win."+n.actionName))
		if item != 0 && n.actionKind == gtkMenuActionRadio {
			// A target its action's state may equal makes GTK draw a radio, checked while it does.
			gMenuItemSetAttributeValue(item, cstr("target"), gVariantNewString(cstr(radioTarget)))
		}
	}
	if item != 0 {
		o.w.attachMenuIcon(item, n.icon)
		gMenuInsertItem(parent, pos, item)
		gObjectUnref(item)
	}
}

func (gtkMenuBarOps) removeAt(parent uintptr, pos int32) {
	if parent != 0 {
		gMenuRemove(parent, pos)
	}
}

func (o gtkMenuBarOps) addAction(n *gtkMenuNode) (uintptr, string) {
	return o.w.registerMenuAction(o.w.actionGroup, o.w.actionRefs, n.id, n.role, n.actionKind, n.disabled, n.checked)
}

func (o gtkMenuBarOps) updateAction(action uintptr, old, next *gtkMenuNode) {
	if old.disabled != next.disabled {
		gSimpleActionSetEnabled(action, boolInt(!next.disabled))
	}
	if old.checked != next.checked {
		switch next.actionKind {
		case gtkMenuActionCheck:
			// The action sinks the floating variant and owns it.
			gSimpleActionSetState(action, gVariantNewBoolean(boolInt(next.checked)))
		case gtkMenuActionRadio:
			state := ""
			if next.checked {
				state = radioTarget
			}
			gSimpleActionSetState(action, gVariantNewString(cstr(state)))
		}
	}
	if old.id != next.id || old.role != next.role {
		o.w.actionRefs[action] = menuRef{id: next.id, role: next.role}
	}
}

func (o gtkMenuBarOps) removeAction(action uintptr, name string) {
	delete(o.w.actionRefs, action)
	if o.w.actionGroup != 0 && name != "" {
		gActionMapRemoveAction(o.w.actionGroup, cstr(name))
	}
}

// buildSubmenu returns a GMenuModel of a submenu's items, separated into
// sections at the Separator entries as GTK menus show groups.
func (w *window) buildSubmenu(entries []driver.MenuEntry) uintptr {
	sub := gMenuNew()
	section := gMenuNew()
	flush := func() {
		if section == 0 {
			return
		}
		gMenuAppendSection(sub, nil, section)
		gObjectUnref(section)
		section = 0
	}
	for i := range entries {
		e := &entries[i]
		if e.Separator {
			flush()
			section = gMenuNew()
			continue
		}
		if !shown(e) {
			continue
		}
		if len(e.Items) > 0 {
			child := w.buildSubmenu(e.Items)
			if child != 0 {
				w.appendSubmenuItem(section, menutext.Label(w.language, e, ""), e.Icon, child)
				gObjectUnref(child)
			}
			continue
		}
		if item := w.buildItem(e); item != 0 {
			gMenuAppendItem(section, item)
			gObjectUnref(item)
		}
	}
	flush()
	return sub
}

// shown reports whether the menu shows e: not a role GTK has no use for, nor a submenu none of whose items it shows, such as a Help holder with no items or a Speech submenu of macOS's roles.
func shown(e *driver.MenuEntry) bool {
	return linuxMenuShown(e, speakAvailable())
}

// appendSubmenuItem appends a submenu item with an optional icon to menu.
func (w *window) appendSubmenuItem(menu uintptr, label string, icon driver.MenuIcon, sub uintptr) {
	if icon.Empty() {
		gMenuAppendSubmenu(menu, cstr(label), sub)
		return
	}
	item := gMenuItemNewSubmenu(cstr(label), sub)
	if item == 0 {
		gMenuAppendSubmenu(menu, cstr(label), sub)
		return
	}
	w.attachMenuIcon(item, icon)
	gMenuAppendItem(menu, item)
	gObjectUnref(item)
}

// buildItem returns a GMenuItem of e with an action when it has one.
func (w *window) buildItem(e *driver.MenuEntry) uintptr {
	label := menutext.Label(w.language, e, "")
	action := ""
	if e.ID != 0 {
		action = w.addAction(e.ID, e.Role, e.Disabled, e.Checkable, e.Checked, e.Radio)
	} else if e.Role != driver.MenuRoleNone {
		action = w.addAction(0, e.Role, e.Disabled, false, false, false)
	}
	var item uintptr
	if action == "" {
		item = gMenuItemNew(cstr(label), nil)
	} else {
		prefix := "win."
		if w.building.prefix != "" {
			prefix = w.building.prefix
		}
		item = gMenuItemNew(cstr(label), cstr(prefix+action))
		if item != 0 && e.Radio && e.ID != 0 {
			// A target its action's state may equal makes GTK draw a radio, checked while it does.
			gMenuItemSetAttributeValue(item, cstr("target"), gVariantNewString(cstr(radioTarget)))
		}
	}
	w.attachMenuIcon(item, e.Icon)
	return item
}

// radioTarget is the target of a radio item, and its action's state while it is checked.
const radioTarget = "selected"

// addAction registers a menu item's action on the window, or on the popup being built, and returns its name; "" when the item does nothing.
func (w *window) addAction(id uint32, role driver.MenuRole, disabled, checkable, checked, radio bool) string {
	kind := gtkMenuActionPlain
	switch {
	case radio:
		kind = gtkMenuActionRadio
	case checkable:
		kind = gtkMenuActionCheck
	}
	group, refs := w.actionGroup, w.actionRefs
	if w.building.group != 0 {
		group, refs = w.building.group, w.building.refs
	}
	_, name := w.registerMenuAction(group, refs, id, role, kind, disabled, checked)
	return name
}

// registerMenuAction creates the GSimpleAction of one menu item, adds it to group and refs, and returns it and its name.
func (w *window) registerMenuAction(group uintptr, refs map[uintptr]menuRef, id uint32, role driver.MenuRole, kind gtkMenuActionKind, disabled, checked bool) (uintptr, string) {
	w.menuCounter++
	name := fmt.Sprintf("nsgo.item-%d", w.menuCounter)
	var action uintptr
	switch kind {
	case gtkMenuActionRadio:
		// A radio's action takes its target as a string parameter, which GTK requires to activate it, and its string state says whether the item is checked. The action sinks the floating variant and owns it.
		state := ""
		if checked {
			state = radioTarget
		}
		action = gSimpleActionNewStateful(cstr(name), cstr("s"), gVariantNewString(cstr(state)))
	case gtkMenuActionCheck:
		// The action sinks the floating variant and owns it.
		state := gVariantNewBoolean(boolInt(checked))
		action = gSimpleActionNewStateful(cstr(name), nil, state)
	default:
		action = gSimpleActionNew(cstr(name), 0)
	}
	if action == 0 {
		return 0, ""
	}
	if disabled {
		gSimpleActionSetEnabled(action, 0)
	}
	refs[action] = menuRef{id: id, role: role}
	gSignalConnect(action, "activate", cbMenuActivate, w.handle)
	if group != 0 {
		gActionMapAddAction(group, action)
	}
	gObjectUnref(action)
	return action, name
}

// menuActivateCallback runs the item of a chosen menu action.
func menuActivateCallback(action, parameter, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	if ref, ok := w.actionRefs[action]; ok {
		w.menuChosen(ref)
	} else if ref, ok := w.popupRefs[action]; ok {
		w.restoreMenuFocus()
		w.menuChosen(ref)
	}
}
