package core

import "strings"

// AccessibilityValue is a range's or a textual element's value
// (accessibilityValue). The zero value is no value.
type AccessibilityValue struct {
	// Min, Max and Now are a range, when Max is above Min: a slider's or
	// a progress bar's.
	Min, Max, Now float32
	// Text is read instead of Now, or alone without a range.
	Text string
}

// IsRange reports whether v is a range.
func (v AccessibilityValue) IsRange() bool { return v.Max > v.Min }

// AccessibilityAction is an action assistive technology offers on an
// element; OnAccessibilityAction receives its Name.
type AccessibilityAction struct {
	// Name is a standard action (ActionActivate and the others), which
	// the screen reader performs with its own gesture or key, or a custom
	// action's name.
	Name string
	// Label is what a custom action is called in the screen reader's
	// actions menu; Name when empty.
	Label string
}

// Standard accessibility actions.
const (
	// ActionActivate replaces the press a screen reader's double tap or
	// VO-Space makes.
	ActionActivate = "activate"
	// ActionIncrement and ActionDecrement adjust an adjustable element:
	// a swipe up or down on iOS, TalkBack's volume keys, VO-Up and Down.
	ActionIncrement = "increment"
	ActionDecrement = "decrement"
	// ActionLongPress replaces a long press (Android).
	ActionLongPress = "longpress"
	// ActionExpand and ActionCollapse change an expandable element
	// (Android).
	ActionExpand   = "expand"
	ActionCollapse = "collapse"
	// ActionEscape is VoiceOver's two-finger scrub on iOS, AXCancel on
	// macOS and Escape on the web.
	ActionEscape = "escape"
	// ActionMagicTap is VoiceOver's two-finger double tap (iOS).
	ActionMagicTap = "magicTap"
)

// IsStandardAction reports whether name is one of the standard actions,
// which platforms map to their own gestures rather than list as custom
// actions.
func IsStandardAction(name string) bool {
	switch name {
	case ActionActivate, ActionIncrement, ActionDecrement, ActionLongPress,
		ActionExpand, ActionCollapse, ActionEscape, ActionMagicTap:
		return true
	}
	return false
}

// Accessible says whether a view is one element for assistive technology.
type Accessible uint8

const (
	// AccessibleAuto leaves it to the platform: views with a role, a
	// label or a press handler are elements.
	AccessibleAuto Accessible = iota
	// AccessibleYes makes the view one element: its descendants are not
	// exposed separately, and without a label it reads their text.
	AccessibleYes
	// AccessibleNo makes the view no element; its descendants still are.
	AccessibleNo
)

// Importance is importantForAccessibility, Android's name for Accessible
// and AccessibilityElementsHidden in one prop.
type Importance uint8

const (
	// ImportantAuto leaves it to the view's other props.
	ImportantAuto Importance = iota
	// ImportantYes is Accessible.
	ImportantYes
	// ImportantNo makes the view no element; its descendants still are.
	ImportantNo
	// ImportantNoHideDescendants hides the view and its subtree.
	ImportantNoHideDescendants
)

// LiveRegion says how screen readers announce changes of a view's label
// and text (accessibilityLiveRegion). Android and the web announce them
// natively; macOS and iOS announce a changed label or text themselves.
type LiveRegion uint8

const (
	LiveRegionNone LiveRegion = iota
	// LiveRegionPolite is announced when the user is idle.
	LiveRegionPolite
	// LiveRegionAssertive interrupts what is being read.
	LiveRegionAssertive
)

// HasAction reports whether p offers the action named name.
func (p *ViewProps) HasAction(name string) bool {
	for _, a := range p.ActionList() {
		if a.Name == name {
			return true
		}
	}
	return false
}

// ActionList returns p's actions.
func (p *ViewProps) ActionList() []AccessibilityAction {
	if p.Actions == nil {
		return nil
	}
	return *p.Actions
}

// SetAccessibilityFocus moves the screen reader's focus to the element,
// as after a change the user should hear about first.
func (r *Ref) SetAccessibilityFocus() {
	if r.n != nil && !r.n.deleted {
		r.t.command(ItemAccessibilityFocus, r.n)
	}
}

// a11y is what an element's accessibility props are bound from.
type a11y struct {
	labelFunc  func() string
	value      func() AccessibilityValue
	expanded   func() bool
	hidden     func() bool
	actions    []AccessibilityAction
	accessible bool
	important  Importance
}

// bindAccessibility sets n's accessibility props from a and binds those
// that change.
func (t *Tree) bindAccessibility(cx *Scope, n *node, a a11y) {
	p := &n.props
	switch {
	case a.important == ImportantNo || a.important == ImportantNoHideDescendants:
		p.Accessible = AccessibleNo
	case a.accessible || a.important == ImportantYes:
		p.Accessible = AccessibleYes
	}
	if len(a.actions) > 0 {
		p.Actions = &a.actions
	}
	if a.labelFunc != nil {
		cx.renderEffect(func() {
			p.Label = a.labelFunc()
			t.touch(n)
		})
	}
	if a.value != nil {
		cx.renderEffect(func() {
			p.Value = a.value()
			t.touch(n)
		})
	}
	if a.expanded != nil {
		p.Expandable = true
		cx.renderEffect(func() {
			p.Expanded = a.expanded()
			t.touch(n)
		})
	}
	always := a.important == ImportantNoHideDescendants
	p.ElementsHidden = always
	if a.hidden != nil && !always {
		cx.renderEffect(func() {
			p.ElementsHidden = a.hidden()
			t.touch(n)
		})
	}
}

// sentProps returns the props n sends: a group without a label of its
// own reads the text inside it.
func (n *node) sentProps() ViewProps {
	p := n.props
	if p.Accessible == AccessibleYes && p.Label == "" && n.text == nil {
		var b strings.Builder
		n.appendText(&b)
		p.Label = b.String()
	}
	return p
}

// appendText appends the text of the texts inside n that assistive
// technology would read, separated by spaces.
func (n *node) appendText(b *strings.Builder) {
	for _, c := range n.children {
		if c.deleted || c.style.Display == DisplayNone || c.props.ElementsHidden {
			continue
		}
		var s string
		switch {
		case c.props.Label != "":
			s = c.props.Label
		case c.text != nil:
			s = strings.ReplaceAll(c.text.text.Text, string(AttachmentRune), "")
		}
		if s = strings.TrimSpace(s); s != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(s)
			continue
		}
		c.appendText(b)
	}
}
