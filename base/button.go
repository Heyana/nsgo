// Package base holds unstyled base components: they own behavior and
// accessibility and take every visual choice from the app.
package base

import "github.com/oneo-me/nsgo/core"

// Button returns a pressable container with the button role. Its look is
// the style and children the app gives it.
func Button(onPress func(), children ...*core.Element) *core.Element {
	return core.View(children...).Role(core.RoleButton).OnPress(onPress)
}
