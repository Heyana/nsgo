package ios

import (
	"fmt"
	"math"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file builds on every system, so tests check UIKit's accessibility mapping on any host.

// headingLevel returns the UIAccessibilityTextAttributeHeadingLevel value ([1, 6]) for a levelled heading, or 0 for none.
func headingLevel(p *driver.ViewProps) int {
	if p.Role != driver.RoleHeader || p.Level <= 0 {
		return 0
	}
	return min(p.Level, 6)
}

// editorTraits adds the input's role and action traits onto the native editor's own traits.
func editorTraits(base uint64, p *driver.ViewProps, searchField, adjustable uint64) uint64 {
	if p.Role == driver.RoleSearchField {
		base |= searchField
	}
	if p.Role == driver.RoleSpinButton || p.HasAction(driver.ActionIncrement) || p.HasAction(driver.ActionDecrement) {
		base |= adjustable
	}
	return base
}

// spokenValue is the value VoiceOver reads: the value's text, else a range's percentage, then the checked, expanded, invalid and level states.
func spokenValue(p *driver.ViewProps) string {
	var parts []string
	val := p.Value
	switch {
	case val.Text != "":
		parts = append(parts, val.Text)
	case val.IsRange():
		parts = append(parts, fmt.Sprintf("%g%%", math.Round(float64((val.Now-val.Min)/(val.Max-val.Min)*100))))
	}
	return strings.Join(append(parts, spokenStates(p)...), ", ")
}

// spokenStates are the states VoiceOver reads after a value, which UIKit has no traits for.
func spokenStates(p *driver.ViewProps) []string {
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
	if p.Level > 0 && p.Role != driver.RoleHeader {
		parts = append(parts, fmt.Sprintf("level %d", p.Level))
	}
	if p.Expandable {
		if p.Expanded {
			parts = append(parts, "expanded")
		} else {
			parts = append(parts, "collapsed")
		}
	}
	// UIKit has no trait for it, so it is spoken, as mixed is.
	if p.Invalid {
		parts = append(parts, "invalid data")
	}
	return parts
}

// editorValue is what VoiceOver reads as an input's value: the editor's own, its text, then the input's value text when it differs, such as a multiple combobox's chosen items, and its states, such as a combobox's expanded state.
func editorValue(own string, p *driver.ViewProps) string {
	var parts []string
	if own != "" {
		parts = append(parts, own)
	}
	if t := p.Value.Text; t != "" && t != own {
		parts = append(parts, t)
	}
	return strings.Join(append(parts, spokenStates(p)...), ", ")
}
