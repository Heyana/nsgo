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

// showMenuBar builds the application menu as a GTK menu bar over the
// surface, unless the title bar is hidden or the window is full screen.
func (w *window) showMenuBar() {
	if w.popover != 0 && w.popoverID == 0 {
		gtkPopoverPopdown(w.popover)
	}
	if w.menuBar != 0 {
		// Unparenting drops the bar's only reference.
		gtkWidgetSetVisible(w.menuBar, 0)
		gtkBoxRemove(w.box, w.menuBar)
		w.menuBar = 0
	}
	clear(w.actionRefs)
	c := &w.chrome
	if c.menu == nil || c.titleBar != driver.TitleBarDefault || c.fullScreen || w.transparent ||
		c.menuBar == driver.MenuBarAutoHide && !c.revealed {
		return
	}
	w.actionGroup = gSimpleActionGroupNew()
	menu := w.buildMenu(w.chrome.menu)
	if menu == 0 {
		return
	}
	gtkWidgetInsertActionGroup(w.win, cstr("win"), w.actionGroup)
	gObjectUnref(w.actionGroup)
	bar := gtkPopoverMenuBarNewFromModel(menu)
	gObjectUnref(menu)
	if bar == 0 {
		return
	}
	showMenuIcons(bar)
	gtkWidgetSetHalign(bar, 1) // start
	gtkBoxPrepend(w.box, bar)
	gtkWidgetSetVisible(bar, 1)
	w.menuBar = bar
}

// buildMenu returns a GMenuModel of entries.
func (w *window) buildMenu(entries []driver.MenuEntry) uintptr {
	menu := gMenuNew()
	for i := range entries {
		e := &entries[i]
		if e.Separator || !shown(e) {
			continue
		}
		if len(e.Items) > 0 {
			sub := w.buildSubmenu(e.Items)
			if sub != 0 {
				w.appendSubmenuItem(menu, menutext.Label(w.language, e, ""), e.Icon, sub)
				gObjectUnref(sub)
			}
			continue
		}
		if item := w.buildItem(e); item != 0 {
			gMenuAppendItem(menu, item)
			gObjectUnref(item)
		}
	}
	return menu
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
	if driver.MenuRoleUnsupported(e.Role, driver.OSLinux, speakAvailable()) {
		return false
	}
	if len(e.Items) == 0 {
		return !e.Role.HoldsMenu()
	}
	for i := range e.Items {
		if !e.Items[i].Separator && shown(&e.Items[i]) {
			return true
		}
	}
	return false
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

// addAction registers a menu item's action on the window and returns its
// name; "" when the item does nothing.
func (w *window) addAction(id uint32, role driver.MenuRole, disabled, checkable, checked, radio bool) string {
	w.menuCounter++
	name := fmt.Sprintf("nsgo.item-%d", w.menuCounter)
	var action uintptr
	switch {
	case radio:
		// A radio's action takes its target as a string parameter, which GTK requires to activate it, and its string state says whether the item is checked. The action sinks the floating variant and owns it.
		state := ""
		if checked {
			state = radioTarget
		}
		action = gSimpleActionNewStateful(cstr(name), cstr("s"), gVariantNewString(cstr(state)))
	case checkable:
		// The action sinks the floating variant and owns it.
		state := gVariantNewBoolean(boolInt(checked))
		action = gSimpleActionNewStateful(cstr(name), nil, state)
	default:
		action = gSimpleActionNew(cstr(name), 0)
	}
	if action == 0 {
		return ""
	}
	if disabled {
		gSimpleActionSetEnabled(action, 0)
	}
	group, refs := w.actionGroup, w.actionRefs
	if w.building.group != 0 {
		group, refs = w.building.group, w.building.refs
	}
	refs[action] = menuRef{id: id, role: role}
	gSignalConnect(action, "activate", cbMenuActivate, w.handle)
	if group != 0 {
		gActionMapAddAction(group, action)
	}
	gObjectUnref(action)
	return name
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
