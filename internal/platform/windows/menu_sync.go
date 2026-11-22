package windows

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/menutext"
)

// The MENUITEMINFOW fType and fState flags a menu item's state sets. The flags every item of a menu shares, such as MFT_RIGHTORDER, come from window.menuTypeFlags.
const (
	mfString     = 0x0000
	mfRadioCheck = 0x0200
	mfSeparator  = 0x0800
	// mfRightOrder lays a menu out right to left: its items' text, and the bar's and its submenus' order.
	mfRightOrder = 0x2000

	mfsGrayed  = 0x0003
	mfsChecked = 0x0008
)

// menuRef is a menu item's ID and role.
type menuRef struct {
	id        uint32
	role      driver.MenuRole
	notInText bool
}

type winMenuKind uint8

const (
	winMenuCommand winMenuKind = iota
	winMenuSubmenu
	winMenuSeparator
)

// winMenuItem is one item of a Win32 HMENU, kept beside the menu bar so that setting the menu again updates the items that changed with SetMenuItemInfoW, InsertMenuItemW and DeleteMenu instead of rebuilding the bar.
type winMenuItem struct {
	kind      winMenuKind
	label     string
	fType     uint32
	fState    uint32
	commandID uint32
	children  []winMenuItem
	handle    uintptr // the HMENU of a submenu, kept across updates
	icon      driver.MenuIcon
	// iconKey identifies the icon's pixels: the icon, its size and, for a template icon, the appearance it is tinted for.
	iconKey string
	bitmap  uintptr // the HBITMAP attached to the item, owned by it
}

// winMenuIconLook is what a menu icon's pixels depend on besides the icon itself.
type winMenuIconLook struct {
	size int
	dark bool
}

func (l winMenuIconLook) key(icon driver.MenuIcon) string {
	if icon.Empty() {
		return ""
	}
	return fmt.Sprintf("%s|%d|%v", icon.Key(), l.size, l.dark && icon.IsTemplate())
}

// winMenuDiff records which MENUITEMINFOW fields differ between two items of the same kind.
type winMenuDiff struct {
	fTypeChanged  bool
	fStateChanged bool
	idChanged     bool
	labelChanged  bool
	iconChanged   bool
}

func (d winMenuDiff) any() bool {
	return d.fTypeChanged || d.fStateChanged || d.idChanged || d.labelChanged || d.iconChanged
}

func diffWinMenuItem(old, next *winMenuItem) winMenuDiff {
	if next.kind == winMenuSeparator {
		return winMenuDiff{}
	}
	return winMenuDiff{
		fTypeChanged:  old.fType != next.fType,
		fStateChanged: old.fState != next.fState,
		idChanged:     next.kind == winMenuCommand && old.commandID != next.commandID,
		labelChanged:  old.label != next.label,
		iconChanged:   old.iconKey != next.iconKey,
	}
}

// winMenuOps applies HMENU and HBITMAP changes, so that menu diffing can be tested without Windows.
type winMenuOps interface {
	createPopup() uintptr
	// insertItem inserts it at pos with its type, state, label, command or submenu, and bitmap.
	insertItem(menu uintptr, pos uint32, it *winMenuItem)
	// updateItem sets the fields diff names of the item at pos to next's.
	updateItem(menu uintptr, pos uint32, next *winMenuItem, diff winMenuDiff)
	// deleteItem deletes the item at pos, destroying its submenu.
	deleteItem(menu uintptr, pos uint32)
	iconBitmap(icon driver.MenuIcon) uintptr
	freeBitmap(bitmap uintptr)
}

// buildWinMenuTree returns the items of entries as appendMenuTo always laid them out: each command's menuRef is appended to refs and numbered by its index there plus one; the roles Windows leaves out, menu holders with no items and submenus left with no items are left out; and a separator goes between items only. typeFlags are the MFT_* flags every item shares (window.menuTypeFlags).
func buildWinMenuTree(entries []driver.MenuEntry, language string, typeFlags uint32, speak bool, look winMenuIconLook, refs *[]menuRef) []winMenuItem {
	var out []winMenuItem
	separate := false
	for i := range entries {
		e := &entries[i]
		switch {
		case e.Separator:
			separate = len(out) > 0
			continue
		case driver.MenuRoleUnsupported(e.Role, driver.OSWindows, speak), e.Role.HoldsMenu() && len(e.Items) == 0:
			continue
		}
		it := winMenuItem{
			label:   strings.ReplaceAll(menutext.Label(language, e, ""), "&", "&&"),
			fType:   typeFlags,
			icon:    e.Icon,
			iconKey: look.key(e.Icon),
		}
		if e.Disabled {
			it.fState |= mfsGrayed
		}
		if len(e.Items) > 0 {
			it.kind = winMenuSubmenu
			it.children = buildWinMenuTree(e.Items, language, typeFlags, speak, look, refs)
			if len(it.children) == 0 {
				continue
			}
		} else {
			it.kind = winMenuCommand
			if k := driver.MenuShortcutKey(*e, false); k != "" {
				it.label += "\t" + shortcutLabel(k)
			}
			if e.Checked {
				it.fState |= mfsChecked
			}
			if e.Radio {
				it.fType |= mfRadioCheck
			}
			*refs = append(*refs, menuRef{e.ID, e.Role, e.NotInText})
			it.commandID = uint32(len(*refs))
		}
		if separate {
			out = append(out, winMenuItem{kind: winMenuSeparator, fType: mfSeparator})
			separate = false
		}
		out = append(out, it)
	}
	return out
}

// syncWinMenu updates menu in place from old to next, keeping the items and submenu handles of the same kind and setting only the fields that changed. It reports whether an item of menu itself changed, which for the menu bar says DrawMenuBar is needed.
func syncWinMenu(ops winMenuOps, menu uintptr, old, next []winMenuItem) bool {
	changed := false
	common := min(len(old), len(next))
	pos := uint32(0)
	for i := range common {
		o, n := &old[i], &next[i]
		if o.kind != n.kind {
			ops.deleteItem(menu, pos)
			releaseWinMenuItem(ops, o)
			changed = true
			if insertWinMenuItem(ops, menu, pos, n) {
				pos++
			}
			continue
		}
		d := diffWinMenuItem(o, n)
		n.bitmap = o.bitmap
		if d.iconChanged {
			n.bitmap = 0
			if !n.icon.Empty() {
				n.bitmap = ops.iconBitmap(n.icon)
			}
		}
		if d.any() {
			ops.updateItem(menu, pos, n, d)
			changed = true
		}
		if d.iconChanged && o.bitmap != 0 {
			ops.freeBitmap(o.bitmap)
		}
		if n.kind == winMenuSubmenu {
			n.handle = o.handle
			syncWinMenu(ops, n.handle, o.children, n.children)
		}
		pos++
	}
	for i := len(old) - 1; i >= len(next); i-- {
		ops.deleteItem(menu, pos+uint32(i-len(next)))
		releaseWinMenuItem(ops, &old[i])
		changed = true
	}
	for i := len(old); i < len(next); i++ {
		if insertWinMenuItem(ops, menu, pos, &next[i]) {
			pos++
			changed = true
		}
	}
	return changed
}

func insertWinMenuItem(ops winMenuOps, menu uintptr, pos uint32, it *winMenuItem) bool {
	if it.kind == winMenuSubmenu {
		sub := ops.createPopup()
		if sub == 0 {
			return false
		}
		it.handle = sub
		syncWinMenu(ops, sub, nil, it.children)
	}
	if it.kind != winMenuSeparator && !it.icon.Empty() {
		it.bitmap = ops.iconBitmap(it.icon)
	}
	ops.insertItem(menu, pos, it)
	return true
}

// releaseWinMenuTree frees the bitmaps items and their submenus own. Destroying or deleting the menus is the caller's.
func releaseWinMenuTree(ops winMenuOps, items []winMenuItem) {
	for i := range items {
		releaseWinMenuItem(ops, &items[i])
	}
}

func releaseWinMenuItem(ops winMenuOps, it *winMenuItem) {
	if it.bitmap != 0 {
		ops.freeBitmap(it.bitmap)
		it.bitmap = 0
	}
	releaseWinMenuTree(ops, it.children)
}

// shortcutLabel shows a normalized shortcut as Windows' menus do, such as "Ctrl+Shift+N".
func shortcutLabel(key string) string {
	parts := strings.Split(key, "+")
	for i, part := range parts {
		switch part {
		case "mod", "ctrl":
			parts[i] = "Ctrl"
		case "alt":
			parts[i] = "Alt"
		case "shift":
			parts[i] = "Shift"
		case "meta":
			parts[i] = "Win"
		case "pageup":
			parts[i] = "PgUp"
		case "pagedown":
			parts[i] = "PgDn"
		case "delete":
			parts[i] = "Del"
		case "escape":
			parts[i] = "Esc"
		default:
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "+")
}

// firstShow tracks a newly opened window waiting for its first mount batch before it shows, as macOS's surface.showFirst does.
type firstShow struct {
	pending bool
}

func (f *firstShow) arm(hidden bool) { f.pending = !hidden }

func (f *firstShow) cancel() { f.pending = false }

// command cancels the pending show when the app shows, hides, closes or destroys the window itself first.
func (f *firstShow) command(action driver.WindowAction) {
	switch action {
	case driver.WindowShow, driver.WindowOrderFront, driver.WindowHide,
		driver.WindowClose, driver.WindowDestroy:
		f.pending = false
	}
}

// consume reports whether this batch should show the window, once.
func (f *firstShow) consume(closed bool) bool {
	if !f.pending {
		return false
	}
	f.pending = false
	return !closed
}
