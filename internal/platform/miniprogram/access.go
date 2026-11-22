package miniprogram

import (
	"strconv"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ariaRoles maps driver.Role to WAI-ARIA role names used by mini programs'
// aria-role attribute (matching internal/platform/web/web.go).
var ariaRoles = map[driver.Role]string{
	driver.RoleButton:           "button",
	driver.RoleLink:             "link",
	driver.RoleHeader:           "heading",
	driver.RoleImage:            "img",
	driver.RoleCheckbox:         "checkbox",
	driver.RoleSwitch:           "switch",
	driver.RoleRadio:            "radio",
	driver.RoleTab:              "tab",
	driver.RoleRadioGroup:       "radiogroup",
	driver.RoleTabList:          "tablist",
	driver.RoleMenu:             "menu",
	driver.RoleMenuItem:         "menuitem",
	driver.RoleDialog:           "dialog",
	driver.RoleAlertDialog:      "alertdialog",
	driver.RoleAlert:            "alert",
	driver.RoleAdjustable:       "slider",
	driver.RoleSpinButton:       "spinbutton",
	driver.RoleCombobox:         "combobox",
	driver.RoleProgressBar:      "progressbar",
	driver.RoleTable:            "table",
	driver.RoleGrid:             "grid",
	driver.RoleRowGroup:         "rowgroup",
	driver.RoleRow:              "row",
	driver.RoleColumnHeader:     "columnheader",
	driver.RoleCell:             "cell",
	driver.RoleListbox:          "listbox",
	driver.RoleOption:           "option",
	driver.RoleGroup:            "group",
	driver.RoleRegion:           "region",
	driver.RoleNavigation:       "navigation",
	driver.RoleTree:             "tree",
	driver.RoleTreeItem:         "treeitem",
	driver.RoleToolbar:          "toolbar",
	driver.RoleMenuItemCheckbox: "menuitemcheckbox",
	driver.RoleMenuItemRadio:    "menuitemradio",
	driver.RoleStatus:           "status",
	driver.RoleSearchField:      "searchbox",
}

var ariaLiveRegions = [...]string{
	driver.LiveRegionPolite:    "polite",
	driver.LiveRegionAssertive: "assertive",
}

// accessJSON encodes v's accessibility attributes for the render layer's ax
// field, or nil when all attributes are empty/default.
func accessJSON(kind driver.ViewKind, p *driver.ViewProps) []byte {
	role := ariaRoles[p.Role]
	grouped := p.Accessible == driver.AccessibleYes && kind != driver.KindText && kind != driver.KindInput
	if role == "" && grouped {
		role = "group"
	}
	if p.Accessible == driver.AccessibleNo && p.Role == driver.RoleNone {
		role = "presentation"
	}
	checked := ""
	switch p.Role {
	case driver.RoleCheckbox, driver.RoleSwitch, driver.RoleRadio, driver.RoleMenuItemCheckbox, driver.RoleMenuItemRadio:
		if p.Indeterminate {
			checked = "mixed"
		} else {
			checked = strconv.FormatBool(p.Checked)
		}
	default:
		if p.Checked {
			checked = "true"
		}
	}
	selected := ""
	if p.SelectionItem || p.Role == driver.RoleTab || p.Role == driver.RoleOption {
		selected = strconv.FormatBool(p.Selected)
	} else if p.Selected {
		selected = "true"
	}
	expanded := ""
	if p.Expandable {
		expanded = strconv.FormatBool(p.Expanded)
	}
	help := p.HelpText()
	live := ""
	if int(p.LiveRegion) < len(ariaLiveRegions) {
		live = ariaLiveRegions[p.LiveRegion]
	}

	var o object
	if role != "" {
		o.str("r", role)
	}
	if p.Label != "" {
		o.str("l", p.Label)
	}
	if help != "" {
		o.str("h", help)
	}
	if p.Disabled {
		o.boolean("d", true)
	}
	if p.Invalid {
		o.boolean("inv", true)
	}
	if p.ElementsHidden {
		o.boolean("hd", true)
	}
	if checked != "" {
		o.str("ck", checked)
	}
	if selected != "" {
		o.str("sl", selected)
	}
	if p.Pressed {
		o.boolean("pr", true)
	}
	if p.Busy {
		o.boolean("bs", true)
	}
	if expanded != "" {
		o.str("ex", expanded)
	}
	if p.Level > 0 {
		o.num("lv", float64(p.Level))
	}
	if live != "" {
		o.str("lr", live)
	}
	if len(o.b) == 0 {
		return nil
	}
	return o.bytes()
}
