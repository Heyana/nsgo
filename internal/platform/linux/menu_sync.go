package linux

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/menutext"
)

// gtkMenuActionKind is the GSimpleAction signature a leaf menu item uses. A GSimpleAction's parameter type and state type are fixed at creation, so only items of the same kind can be updated in place; a kind change replaces the action and its GMenuItem.
type gtkMenuActionKind uint8

const (
	gtkMenuActionNone gtkMenuActionKind = iota
	gtkMenuActionPlain
	gtkMenuActionCheck
	gtkMenuActionRadio
)

// gtkMenuNode is one normalized item in a GTK GMenu model, kept alongside the live GtkPopoverMenuBar so subsequent SetApplicationMenu calls mutate GMenu and GSimpleAction objects in place instead of rebuilding the menu bar widget.
type gtkMenuNode struct {
	label      string
	submenu    bool
	sections   []gtkMenuSection // populated when submenu == true
	menu       uintptr          // owned GMenu handle for submenu
	actionKind gtkMenuActionKind
	id         uint32
	role       driver.MenuRole
	disabled   bool
	checked    bool
	action     uintptr // GSimpleAction handle (kept alive by actionGroup)
	actionName string  // name inside actionGroup (without "win." prefix)
	icon       driver.MenuIcon
	// iconKey identifies the icon's pixels: the icon and, for a template one, the appearance it is tinted for, and the size.
	iconKey string
}

// gtkMenuIconLook is what a menu icon's pixels depend on besides the icon itself.
type gtkMenuIconLook struct {
	size int
	dark bool
}

func (l gtkMenuIconLook) key(icon driver.MenuIcon) string {
	if icon.Empty() {
		return ""
	}
	return fmt.Sprintf("%s|%d|%v", icon.Key(), l.size, l.dark && icon.IsTemplate())
}

// sameRow reports whether n's GMenuItem shows what o's does, so the item can stay in its menu.
func (n *gtkMenuNode) sameRow(o *gtkMenuNode) bool {
	return n.label == o.label && n.iconKey == o.iconKey
}

// gtkMenuSection is one separator-delimited GMenu section inside a submenu.
type gtkMenuSection struct {
	menu  uintptr // owned GMenu handle for section
	items []gtkMenuNode
}

// gtkMenuOps abstracts GIO GMenu and GSimpleActionGroup mutations so in-place menu diffing can be tested on any host without GTK.
type gtkMenuOps interface {
	newMenu() uintptr
	unrefMenu(menu uintptr)
	insertSubmenu(parent uintptr, pos int32, n *gtkMenuNode)
	insertSection(parent uintptr, pos int32, section uintptr)
	insertItem(parent uintptr, pos int32, n *gtkMenuNode)
	removeAt(parent uintptr, pos int32)
	addAction(n *gtkMenuNode) (action uintptr, name string)
	updateAction(action uintptr, old, next *gtkMenuNode)
	removeAction(action uintptr, name string)
}

// linuxMenuShown reports whether a Linux GTK menu shows e: not a macOS-only role, nor a submenu none of whose items it shows, such as an empty Help holder or a Speech submenu where speech is unavailable.
func linuxMenuShown(e *driver.MenuEntry, speak bool) bool {
	if driver.MenuRoleUnsupported(e.Role, driver.OSLinux, speak) {
		return false
	}
	if len(e.Items) == 0 {
		return !e.Role.HoldsMenu()
	}
	for i := range e.Items {
		if !e.Items[i].Separator && linuxMenuShown(&e.Items[i], speak) {
			return true
		}
	}
	return false
}

// buildGTKMenuTree normalizes top-level entries for a GTK menu bar model.
func buildGTKMenuTree(entries []driver.MenuEntry, language string, speak bool, look gtkMenuIconLook) []gtkMenuNode {
	var out []gtkMenuNode
	for i := range entries {
		e := &entries[i]
		if e.Separator || !linuxMenuShown(e, speak) {
			continue
		}
		if len(e.Items) > 0 {
			sections := buildGTKMenuSections(e.Items, language, speak, look)
			if len(sections) == 0 {
				continue
			}
			out = append(out, gtkMenuNode{
				label:    menutext.Label(language, e, ""),
				submenu:  true,
				sections: sections,
				icon:     e.Icon,
				iconKey:  look.key(e.Icon),
			})
			continue
		}
		out = append(out, buildGTKMenuLeaf(e, language, look))
	}
	return out
}

// buildGTKMenuSections splits a submenu's entries on separators into non-empty GTK GMenu sections.
func buildGTKMenuSections(entries []driver.MenuEntry, language string, speak bool, look gtkMenuIconLook) []gtkMenuSection {
	var sections []gtkMenuSection
	var cur []gtkMenuNode
	flush := func() {
		if len(cur) > 0 {
			sections = append(sections, gtkMenuSection{items: cur})
			cur = nil
		}
	}
	for i := range entries {
		e := &entries[i]
		if e.Separator {
			flush()
			continue
		}
		if !linuxMenuShown(e, speak) {
			continue
		}
		if len(e.Items) > 0 {
			sub := buildGTKMenuSections(e.Items, language, speak, look)
			if len(sub) == 0 {
				continue
			}
			cur = append(cur, gtkMenuNode{
				label:    menutext.Label(language, e, ""),
				submenu:  true,
				sections: sub,
				icon:     e.Icon,
				iconKey:  look.key(e.Icon),
			})
			continue
		}
		cur = append(cur, buildGTKMenuLeaf(e, language, look))
	}
	flush()
	return sections
}

func buildGTKMenuLeaf(e *driver.MenuEntry, language string, look gtkMenuIconLook) gtkMenuNode {
	n := gtkMenuNode{
		label:    menutext.Label(language, e, ""),
		disabled: e.Disabled,
		icon:     e.Icon,
		iconKey:  look.key(e.Icon),
	}
	switch {
	case e.ID != 0:
		n.id, n.role = e.ID, e.Role
		switch {
		case e.Radio:
			n.actionKind = gtkMenuActionRadio
			n.checked = e.Checked
		case e.Checkable:
			n.actionKind = gtkMenuActionCheck
			n.checked = e.Checked
		default:
			n.actionKind = gtkMenuActionPlain
		}
	case e.Role != driver.MenuRoleNone:
		n.role = e.Role
		n.actionKind = gtkMenuActionPlain
	default:
		n.actionKind = gtkMenuActionNone
	}
	return n
}

// syncGTKMenuItems updates parent's items in place from old to next. When a leaf item keeps its label and action kind, only its GSimpleAction's enabled, state and callback mapping change—parent's GMenu is not touched at all.
func syncGTKMenuItems(ops gtkMenuOps, parent uintptr, old, next []gtkMenuNode) {
	common := min(len(old), len(next))
	pos := int32(0)
	for i := range common {
		o, n := &old[i], &next[i]
		switch {
		case o.submenu && n.submenu:
			n.menu = o.menu
			syncGTKMenuSections(ops, n.menu, o.sections, n.sections)
			if !n.sameRow(o) {
				ops.removeAt(parent, pos)
				ops.insertSubmenu(parent, pos, n)
			}
			pos++
		case !o.submenu && !n.submenu:
			if o.actionKind == n.actionKind {
				n.action, n.actionName = o.action, o.actionName
				if n.action != 0 && (o.disabled != n.disabled || o.checked != n.checked || o.id != n.id || o.role != n.role) {
					ops.updateAction(n.action, o, n)
				}
				if !n.sameRow(o) {
					ops.removeAt(parent, pos)
					ops.insertItem(parent, pos, n)
				}
			} else {
				if o.action != 0 {
					ops.removeAction(o.action, o.actionName)
				}
				if n.actionKind != gtkMenuActionNone {
					n.action, n.actionName = ops.addAction(n)
				}
				ops.removeAt(parent, pos)
				ops.insertItem(parent, pos, n)
			}
			pos++
		default:
			releaseGTKMenuNode(ops, o)
			ops.removeAt(parent, pos)
			if insertGTKMenuNode(ops, parent, pos, n) {
				pos++
			}
		}
	}
	for i := len(old) - 1; i >= len(next); i-- {
		releaseGTKMenuNode(ops, &old[i])
		ops.removeAt(parent, pos+int32(i-len(next)))
	}
	for i := len(old); i < len(next); i++ {
		if insertGTKMenuNode(ops, parent, pos, &next[i]) {
			pos++
		}
	}
}

func syncGTKMenuSections(ops gtkMenuOps, parent uintptr, old, next []gtkMenuSection) {
	common := min(len(old), len(next))
	pos := int32(0)
	for i := range common {
		next[i].menu = old[i].menu
		syncGTKMenuItems(ops, next[i].menu, old[i].items, next[i].items)
		pos++
	}
	for i := len(old) - 1; i >= len(next); i-- {
		releaseGTKMenuSection(ops, &old[i])
		ops.removeAt(parent, pos+int32(i-len(next)))
	}
	for i := len(old); i < len(next); i++ {
		sec := ops.newMenu()
		if sec == 0 {
			continue
		}
		next[i].menu = sec
		syncGTKMenuItems(ops, sec, nil, next[i].items)
		ops.insertSection(parent, pos, sec)
		pos++
	}
}

func insertGTKMenuNode(ops gtkMenuOps, parent uintptr, pos int32, n *gtkMenuNode) bool {
	if n.submenu {
		sub := ops.newMenu()
		if sub == 0 {
			return false
		}
		n.menu = sub
		syncGTKMenuSections(ops, sub, nil, n.sections)
		ops.insertSubmenu(parent, pos, n)
		return true
	}
	if n.actionKind != gtkMenuActionNone {
		n.action, n.actionName = ops.addAction(n)
	}
	ops.insertItem(parent, pos, n)
	return true
}

func releaseGTKMenuTree(ops gtkMenuOps, items []gtkMenuNode) {
	for i := range items {
		releaseGTKMenuNode(ops, &items[i])
	}
}

func releaseGTKMenuNode(ops gtkMenuOps, n *gtkMenuNode) {
	if n.submenu {
		for i := range n.sections {
			releaseGTKMenuSection(ops, &n.sections[i])
		}
		if n.menu != 0 {
			ops.unrefMenu(n.menu)
			n.menu = 0
		}
		return
	}
	if n.action != 0 {
		ops.removeAction(n.action, n.actionName)
		n.action, n.actionName = 0, ""
	}
}

func releaseGTKMenuSection(ops gtkMenuOps, sec *gtkMenuSection) {
	releaseGTKMenuTree(ops, sec.items)
	if sec.menu != 0 {
		ops.unrefMenu(sec.menu)
		sec.menu = 0
	}
}

// firstShow tracks a newly created window waiting for its first mount batch before presenting itself on screen, matching macOS's surface.showFirst.
type firstShow struct {
	pending bool
}

func (f *firstShow) arm(hidden bool) { f.pending = !hidden }

func (f *firstShow) cancel() { f.pending = false }

func (f *firstShow) command(action driver.WindowAction) {
	switch action {
	case driver.WindowShow, driver.WindowOrderFront, driver.WindowHide,
		driver.WindowClose, driver.WindowDestroy:
		f.pending = false
	}
}

func (f *firstShow) consume(closed bool) bool {
	if !f.pending {
		return false
	}
	f.pending = false
	return !closed
}
