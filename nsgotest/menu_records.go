package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// MenuItem records an application, Dock or context menu item as the test sees it, including the resolved label, icon and shortcut.
type MenuItem struct {
	Label     string
	Icon      core.MenuIcon
	Shortcut  string
	Role      core.MenuRole
	Disabled  bool
	Checkable bool
	Checked   bool
	Radio     bool
	Separator bool
	NotInText bool
	// HasHandler reports whether the app supplied an OnSelect handler.
	HasHandler bool
	Items      []MenuItem
}

func menuItems(entries []driver.MenuEntry) []MenuItem {
	if entries == nil {
		return nil
	}
	items := make([]MenuItem, len(entries))
	for i, e := range entries {
		items[i] = MenuItem{
			Label: e.Label, Icon: e.Icon.Clone(), Shortcut: e.Shortcut, Role: e.Role, Disabled: e.Disabled,
			Checkable: e.Checkable, Checked: e.Checked, Radio: e.Radio, Separator: e.Separator,
			NotInText: e.NotInText, HasHandler: e.ID != 0, Items: menuItems(e.Items),
		}
	}
	return items
}

func (a *App) clearPopupMenu() {
	a.popupMenu, a.PopupMenu, a.popupID, a.popupTree = nil, nil, 0, nil
}
