package nsgotest

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ViewProps records a mounted view's identity, accessibility and interaction settings.
type ViewProps struct {
	Role core.Role
	// Level is a heading's or a tree item's level, from 1; 0 for none.
	Level int
	Label string
	// LabelledBy lists the visible elements naming this view. The model
	// owns a copy; a pointer keeps ViewProps comparable. Label holds the
	// resolved name too.
	LabelledBy *[]core.NodeID
	// LabelFor is the control this visible label activates. LabelPress
	// marks a press handler that forwards activation to that control.
	LabelFor   core.NodeID
	LabelPress bool
	// Description says more about the element, read after its name, as
	// ARIA's aria-description; Hint says what activating it does.
	Description string
	Hint        string
	Disabled    bool
	// Selected, Checked and Indeterminate (mixed) are reported to
	// assistive technology, and Expanded when Expandable.
	Selected bool
	// MultiSelectable says a RoleGrid, RoleListbox or RoleTree permits selecting more than one of its items.
	MultiSelectable bool
	// SelectionItem says the view is one item in a selectable collection; it stays true when the item is not currently selected.
	SelectionItem bool
	Checked       bool
	Indeterminate bool
	// Pressed is a toggle that is on and Busy an element still loading;
	// Invalid a field whose content the app rejected (aria-invalid).
	Pressed    bool
	Busy       bool
	Invalid    bool
	Expandable bool
	Expanded   bool
	// Value is a range's or a textual element's value.
	Value core.AccessibilityValue
	// Accessible makes the view one element, or none.
	Accessible core.Accessible
	// ElementsHidden hides the view and its subtree from assistive
	// technology.
	ElementsHidden bool
	LiveRegion     core.LiveRegion
	// Actions are the actions offered to assistive technology, reported
	// with EventAccessibilityAction. The model owns a copy; a pointer
	// keeps ViewProps comparable. Read it with ActionList.
	Actions *[]core.AccessibilityAction
	TestID  string
	// WindowDrag, PanActivation and selections are decided on the main
	// thread.
	WindowDrag    core.WindowDrag
	PanActivation core.PanActivation
	// PanScrollHandoff: the view's pan shares a drag with the scroll view
	// inside it where the drag starts, on iOS (core.View.PanScrollHandoff).
	PanScrollHandoff bool
	// SelectionArea is a view's, Selectable and SelectionColor a text's.
	SelectionArea  bool
	Selectable     core.Selectable
	SelectionColor core.Color
	// NoFocus keeps a pressable view out of the Tab stops, and Focusable
	// makes a Tab stop of a view that takes no presses.
	NoFocus, Focusable bool
	// TabAfter is a shown dialog overlay's place in the Tab order, the
	// view it follows, before that view's children: the native view
	// right before it where the app put it, as WAI-ARIA's non-modal
	// popups follow their trigger. Backends that order Tab themselves
	// take it; the toolkits ordering it by their own trees (AppKit's key
	// view loop, the browser's DOM) leave the overlay where it is.
	TabAfter core.NodeID
	// Topmost marks a Topmost overlay hoisted above modals on its host.
	Topmost bool
	// Appearance is the one its subtree forces (core.Scope.SetSubtreeAppearance),
	// for the platform's views in it; AppearanceSystem follows the window.
	Appearance core.Appearance
	// PointerEvents, HitSlop and DelayLongPress are the view's; a zero
	// DelayLongPress is the platform's 500 ms.
	PointerEvents  core.PointerEvents
	HitSlop        core.Edges
	DelayLongPress time.Duration
	// Ripple is Android's press ripple, nil for none. The model owns a copy.
	Ripple *core.Ripple
}

func viewProps(p driver.ViewProps) ViewProps {
	var labelledBy *[]core.NodeID
	if p.LabelledBy != nil {
		ids := slices.Clone(*p.LabelledBy)
		labelledBy = &ids
	}
	var actions *[]core.AccessibilityAction
	if p.Actions != nil {
		list := slices.Clone(*p.Actions)
		actions = &list
	}
	var ripple *core.Ripple
	if p.Ripple != nil {
		copy := *p.Ripple
		ripple = &copy
	}
	return ViewProps{
		Role:             p.Role,
		Level:            p.Level,
		Label:            p.Label,
		LabelledBy:       labelledBy,
		LabelFor:         p.LabelFor,
		LabelPress:       p.LabelPress,
		Description:      p.Description,
		Hint:             p.Hint,
		Disabled:         p.Disabled,
		Selected:         p.Selected,
		MultiSelectable:  p.MultiSelectable,
		SelectionItem:    p.SelectionItem,
		Checked:          p.Checked,
		Indeterminate:    p.Indeterminate,
		Pressed:          p.Pressed,
		Busy:             p.Busy,
		Invalid:          p.Invalid,
		Expandable:       p.Expandable,
		Expanded:         p.Expanded,
		Value:            p.Value,
		Accessible:       p.Accessible,
		ElementsHidden:   p.ElementsHidden,
		LiveRegion:       p.LiveRegion,
		Actions:          actions,
		TestID:           p.TestID,
		WindowDrag:       p.WindowDrag,
		PanActivation:    p.PanActivation,
		PanScrollHandoff: p.PanScrollHandoff,
		SelectionArea:    p.SelectionArea,
		Selectable:       p.Selectable,
		SelectionColor:   p.SelectionColor,
		NoFocus:          p.NoFocus,
		Focusable:        p.Focusable,
		TabAfter:         p.TabAfter,
		Topmost:          p.Topmost,
		Appearance:       p.Appearance,
		PointerEvents:    p.PointerEvents,
		HitSlop:          p.HitSlop,
		DelayLongPress:   p.DelayLongPress,
		Ripple:           ripple,
	}
}

func (p ViewProps) driverProps() driver.ViewProps {
	return driver.ViewProps{
		Role:             p.Role,
		Level:            p.Level,
		Label:            p.Label,
		LabelledBy:       p.LabelledBy,
		LabelFor:         p.LabelFor,
		LabelPress:       p.LabelPress,
		Description:      p.Description,
		Hint:             p.Hint,
		Disabled:         p.Disabled,
		Selected:         p.Selected,
		MultiSelectable:  p.MultiSelectable,
		SelectionItem:    p.SelectionItem,
		Checked:          p.Checked,
		Indeterminate:    p.Indeterminate,
		Pressed:          p.Pressed,
		Busy:             p.Busy,
		Invalid:          p.Invalid,
		Expandable:       p.Expandable,
		Expanded:         p.Expanded,
		Value:            p.Value,
		Accessible:       p.Accessible,
		ElementsHidden:   p.ElementsHidden,
		LiveRegion:       p.LiveRegion,
		Actions:          p.Actions,
		TestID:           p.TestID,
		WindowDrag:       p.WindowDrag,
		PanActivation:    p.PanActivation,
		PanScrollHandoff: p.PanScrollHandoff,
		SelectionArea:    p.SelectionArea,
		Selectable:       p.Selectable,
		SelectionColor:   p.SelectionColor,
		NoFocus:          p.NoFocus,
		Focusable:        p.Focusable,
		TabAfter:         p.TabAfter,
		Topmost:          p.Topmost,
		Appearance:       p.Appearance,
		PointerEvents:    p.PointerEvents,
		HitSlop:          p.HitSlop,
		DelayLongPress:   p.DelayLongPress,
		Ripple:           p.Ripple,
	}
}

// HelpText joins the description and hint as the platform reads them.
func (p *ViewProps) HelpText() string {
	native := p.driverProps()
	return native.HelpText()
}

// HasAction reports whether the view offers name to assistive technology.
func (p *ViewProps) HasAction(name string) bool {
	native := p.driverProps()
	return native.HasAction(name)
}

// ActionList returns the recorded accessibility actions.
func (p *ViewProps) ActionList() []core.AccessibilityAction {
	native := p.driverProps()
	return native.ActionList()
}

// FocusStop reports whether a visible, mounted non-input view takes keyboard focus.
func (p *ViewProps) FocusStop(events EventMask) bool {
	native := p.driverProps()
	return native.FocusStop(events)
}
