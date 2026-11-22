// Package base holds NSGo's built-in unstyled components: controls,
// disclosure, progress, popups, menus, dialogs, sheets, navigation, safe
// areas, resizable panels, a dock and the keyboard. They own behavior and
// accessibility and take every visual choice from the app.
package base

import "github.com/nsgo-dev/nsgo/core"

// Button is a pressable container with the button role. Its look is the
// style and children the app gives it, its states included.
type Button struct {
	// Invalid is core.View.Invalid, for accessibility and aria-invalid styles.
	Invalid    func() bool
	Ref        *core.Ref
	LabelledBy []*core.Ref
	// OnPress is the button's action. Without it the button still takes presses and is a keyboard stop, as a button with no action yet; for a look inside a view that takes the presses itself, such as a Popover's Trigger, use a plain core.View.
	OnPress     func(core.PressEvent)
	OnLongPress func(core.PressEvent)
	// OnPressDown and OnPressUp follow a press going down on the button
	// and ending, released, moved away or cancelled, for the app's own
	// press feedback.
	OnPressDown func(core.PressEvent)
	OnPressUp   func(core.PressEvent)
	// Disabled stops presses and is reported to assistive technology.
	Disabled func() bool
	// Busy says the button's action is still loading.
	Busy func() bool
	// Selected marks a toggled trigger or the current item: assistive
	// technology says so, and Style.Selected applies.
	Selected func() bool
	// Expanded makes the button expandable, as a disclosure, and expanded
	// while it returns true.
	Expanded func() bool
	// Role is what it is for assistive technology; RoleButton when 0.
	Role core.Role
	// Class is Tailwind CSS classes under Style, as core.View's, as each
	// part's class is under its style.
	Class     string
	Style     core.Style
	StyleFunc func() core.Style
	Label     string
	LabelFunc func() string
	// Hint says what pressing the button does, for assistive technology.
	Hint     string
	HintFunc func() string
	// Tooltip shows above the button once the pointer rests over it on
	// desktops, as core.View's.
	Tooltip core.Node
	TestID  string
	// HitSlop is how far outside the button presses count.
	HitSlop core.Edges
	// NoFocus keeps it out of the Tab stops, for a button of a row the
	// keyboard reaches another way.
	NoFocus bool
	// AutoFocus moves the keyboard focus here once it mounts on desktops, the web, Android and iOS/tvOS, unless NoFocus or Disabled keeps it out of the keyboard stops.
	AutoFocus bool
	// AccessibilityElementsHidden hides it from assistive technology while
	// it returns true, as a leaf's chevron.
	AccessibilityElementsHidden func() bool
	// OnLayout is its view's, as a core.View's.
	OnLayout func(core.Bounds)
	// Desktop holds the props only desktops have, as a core.View's.
	Desktop  core.DesktopViewProps
	Children []core.Node
}

// Render implements core.Node.
func (b Button) Render(cx *core.Scope) core.Node {
	// A button remains a press target and keyboard stop without an action; its look must not decide its behavior.
	press := b.OnPress
	if press == nil {
		press = func(core.PressEvent) {}
	}
	role := b.Role
	if role == core.RoleNone {
		role = core.RoleButton
	}
	ref := b.Ref
	if ref == nil && b.AutoFocus && !b.NoFocus {
		ref = &core.Ref{}
	}
	view := barItem(cx, core.View{
		Ref:                         ref,
		Class:                       b.Class,
		Style:                       b.Style,
		StyleFunc:                   b.StyleFunc,
		Children:                    b.Children,
		OnPress:                     press,
		OnLongPress:                 b.OnLongPress,
		OnPressDown:                 b.OnPressDown,
		OnPressUp:                   b.OnPressUp,
		Disabled:                    b.Disabled,
		Busy:                        b.Busy,
		Invalid:                     b.Invalid,
		Selected:                    b.Selected,
		Expanded:                    b.Expanded,
		Role:                        role,
		Label:                       b.Label,
		LabelledBy:                  b.LabelledBy,
		LabelFunc:                   b.LabelFunc,
		Desktop:                     b.Desktop,
		Tooltip:                     b.Tooltip,
		Hint:                        b.Hint,
		HintFunc:                    b.HintFunc,
		HitSlop:                     b.HitSlop,
		NoFocus:                     b.NoFocus,
		AccessibilityElementsHidden: b.AccessibilityElementsHidden,
		TestID:                      b.TestID, OnLayout: b.OnLayout,
	}, nil)
	if b.AutoFocus && !b.NoFocus && ref != nil {
		cx.Post(func() {
			if view.Disabled == nil || !view.Disabled() {
				ref.Focus()
			}
		})
	}
	return view
}
