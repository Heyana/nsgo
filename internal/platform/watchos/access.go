package watchos

import (
	"fmt"
	"math"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Accessibility flags for opAccess (Host.swift's Node).
const (
	accessElement uint8 = 1 << iota
	accessHidden
	accessDisabled
	accessModal
)

// Accessibility traits for opAccess (Host.swift's Node.swiftTraits).
const (
	traitButton uint32 = 1 << iota
	traitLink
	traitHeader
	traitImage
	traitStaticText
	traitSelected
	traitSearchField
	traitUpdatesFrequently
	traitTabBar
)

// watchAccess is what the SwiftUI host exposes to VoiceOver for one node.
type watchAccess struct {
	flags  uint8
	traits uint32
	label  string
	hint   string
	value  string
}

// watchAccessOf maps a view's kind, props, events and plain text to the
// accessibility state Host.swift applies to its SwiftUI node.
func watchAccessOf(kind driver.ViewKind, p *driver.ViewProps, events driver.EventMask, plainText string, inputDisabled bool) watchAccess {
	var a watchAccess
	if p.ElementsHidden {
		a.flags |= accessHidden
	}
	if p.Disabled || inputDisabled {
		a.flags |= accessDisabled
	}
	if p.Role == driver.RoleDialog || p.Role == driver.RoleAlertDialog {
		a.flags |= accessModal
	}
	a.label = p.Label
	if a.label == "" && kind == driver.KindText {
		a.label = plainText
	}
	a.hint = p.HelpText()
	a.value = watchSpokenValue(p)

	role := p.Role
	if role == driver.RoleStatus && kind == driver.KindText {
		role = driver.RoleText
	}
	switch role {
	case driver.RoleRadioGroup, driver.RoleTabList, driver.RoleMenu, driver.RoleDialog, driver.RoleAlertDialog, driver.RoleAlert, driver.RoleStatus,
		driver.RoleTable, driver.RoleGrid, driver.RoleRowGroup, driver.RoleRow, driver.RoleListbox, driver.RoleGroup,
		driver.RoleRegion, driver.RoleNavigation, driver.RoleTree, driver.RoleToolbar:
		if p.Role == driver.RoleTabList {
			a.traits |= traitTabBar
		}
		return a
	}

	pressable := !p.LabelPress && (events.Has(driver.EventPress) || events.Has(driver.EventLongPress))
	switch role {
	case driver.RoleNone:
		if pressable {
			a.traits |= traitButton
		} else if kind == driver.KindText {
			a.traits |= traitStaticText
		}
	case driver.RoleButton, driver.RoleCheckbox, driver.RoleSwitch, driver.RoleRadio, driver.RoleTab, driver.RoleMenuItem,
		driver.RoleMenuItemCheckbox, driver.RoleMenuItemRadio, driver.RoleOption, driver.RoleTreeItem, driver.RoleCombobox:
		a.traits |= traitButton
	case driver.RoleLink:
		a.traits |= traitLink
	case driver.RoleHeader, driver.RoleColumnHeader:
		a.traits |= traitHeader
	case driver.RoleImage:
		a.traits |= traitImage
	case driver.RoleText:
		a.traits |= traitStaticText
	case driver.RoleProgressBar:
		a.traits |= traitUpdatesFrequently
	case driver.RoleSearchField:
		a.traits |= traitSearchField
	}
	if p.Selected || p.Pressed {
		a.traits |= traitSelected
	}
	if p.Busy {
		a.traits |= traitUpdatesFrequently
	}

	element := p.Role != driver.RoleNone || kind == driver.KindText || pressable || a.label != "" || p.Accessible == driver.AccessibleYes
	if p.Accessible == driver.AccessibleNo || p.Role == driver.RoleTextField || kind == driver.KindInput {
		element = false
	}
	if element {
		a.flags |= accessElement
	}
	return a
}

// watchSpokenValue is the value VoiceOver reads: the value's text or range
// percentage followed by states SwiftUI has no dedicated trait for.
func watchSpokenValue(p *driver.ViewProps) string {
	var parts []string
	val := p.Value
	switch {
	case val.Text != "":
		parts = append(parts, val.Text)
	case val.IsRange():
		parts = append(parts, fmt.Sprintf("%g%%", math.Round(float64((val.Now-val.Min)/(val.Max-val.Min)*100))))
	}
	return strings.Join(append(parts, watchSpokenStates(p)...), ", ")
}

func watchSpokenStates(p *driver.ViewProps) []string {
	var parts []string
	switch p.Role {
	case driver.RoleCheckbox, driver.RoleSwitch, driver.RoleRadio, driver.RoleMenuItemCheckbox, driver.RoleMenuItemRadio:
		switch {
		case p.Indeterminate:
			parts = append(parts, "mixed")
		case p.Checked || p.Selected:
			parts = append(parts, "1")
		default:
			parts = append(parts, "0")
		}
	}
	if p.Expandable {
		if p.Expanded {
			parts = append(parts, "expanded")
		} else {
			parts = append(parts, "collapsed")
		}
	}
	if p.Invalid {
		parts = append(parts, "invalid data")
	}
	return parts
}

func (w *writer) access(id driver.NodeID, a watchAccess) {
	w.op(opAccess, id)
	w.byte(a.flags)
	w.u32(a.traits)
	w.str(a.label)
	w.str(a.hint)
	w.str(a.value)
}
