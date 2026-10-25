// Package base holds NSGo's built-in unstyled components: controls,
// disclosure, progress, popups, menus, dialogs, sheets, navigation, safe
// areas, resizable panels, a dock and the keyboard. They own behavior and
// accessibility and take every visual choice from the app.
package base

import "github.com/oneo-me/nsgo/core"

// Button is a pressable container with the button role. Its look is the
// style and children the app gives it, its states included.
type Button struct {
	OnPress     func(core.PressEvent)
	OnLongPress func(core.PressEvent)
	// OnPressDown and OnPressUp follow a press going down on the button
	// and ending, released, moved away or cancelled, for the app's own
	// press feedback.
	OnPressDown func(core.PressEvent)
	OnPressUp   func(core.PressEvent)
	// Disabled stops presses and is reported to assistive technology.
	Disabled func() bool
	// Selected marks a toggled trigger or the current item: assistive
	// technology says so, and Style.Selected applies.
	Selected func() bool
	// Expanded makes the button expandable, as a disclosure, and expanded
	// while it returns true.
	Expanded func() bool
	// Role is what it is for assistive technology; RoleButton when 0.
	Role      core.Role
	Style     core.Style
	StyleFunc func() core.Style
	Label     string
	// Hint says what pressing the button does, for assistive technology.
	Hint     string
	TestID   string
	Children []core.Node
}

// Render implements core.Node.
func (b Button) Render(*core.Scope) core.Node {
	role := b.Role
	if role == core.RoleNone {
		role = core.RoleButton
	}
	return core.View{
		Style:       b.Style,
		StyleFunc:   b.StyleFunc,
		Children:    b.Children,
		OnPress:     b.OnPress,
		OnLongPress: b.OnLongPress,
		OnPressDown: b.OnPressDown,
		OnPressUp:   b.OnPressUp,
		Disabled:    b.Disabled,
		Selected:    b.Selected,
		Expanded:    b.Expanded,
		Role:        role,
		Label:       b.Label,
		Hint:        b.Hint,
		TestID:      b.TestID,
	}
}
