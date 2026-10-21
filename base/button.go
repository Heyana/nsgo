// Package base holds unstyled base components: they own behavior and
// accessibility and take every visual choice from the app.
package base

import "github.com/oneo-me/nsgo/core"

// Button is a pressable container with the button role. Its look is the
// style and children the app gives it.
type Button struct {
	OnPress     func()
	OnLongPress func()
	// Disabled stops presses and is reported to assistive technology.
	Disabled  func() bool
	Style     core.Style
	StyleFunc func() core.Style
	Label     string
	TestID    string
	Children  []core.Node
}

// Render implements core.Node.
func (b Button) Render(*core.Scope) core.Node {
	return core.View{
		Style:       b.Style,
		StyleFunc:   b.StyleFunc,
		Children:    b.Children,
		OnPress:     b.OnPress,
		OnLongPress: b.OnLongPress,
		Disabled:    b.Disabled,
		Role:        core.RoleButton,
		Label:       b.Label,
		TestID:      b.TestID,
	}
}
