package base

import "github.com/oneo-me/nsgo/core"

// Checkbox is checked, unchecked or indeterminate, with no look of its
// own. The app owns the value: a press reports the next one to OnChange,
// and the checkbox shows what Checked returns. Its parts are an indicator
// holding the app's mark, then its children; the indicator's state styles
// follow the checkbox's state.
type Checkbox struct {
	Checked       func() bool
	Indeterminate func() bool
	Disabled      func() bool
	// OnChange receives the value a press asks for: true from unchecked
	// and indeterminate, false from checked.
	OnChange func(checked bool)

	// Style is the checkbox's; its Checked, Indeterminate and Disabled
	// state styles apply in those states. Each StyleFunc's result merges
	// over its style, again when what it reads changes.
	Style              core.Style
	StyleFunc          func() core.Style
	IndicatorStyle     core.Style
	IndicatorStyleFunc func() core.Style
	// Indicator is the mark inside the indicator, which the app shows by
	// the value it owns.
	Indicator core.Node
	Children  []core.Node
	// Role is what assistive technology calls it; a checkbox by default.
	Role   core.Role
	Label  string
	TestID string
}

// Render implements core.Node.
func (c Checkbox) Render(*core.Scope) core.Node {
	checked, mixed := orFalse(c.Checked), orFalse(c.Indeterminate)
	shows := func() bool { return checked() && !mixed() }
	return core.View{
		Style:         c.Style,
		StyleFunc:     c.StyleFunc,
		Role:          orRole(c.Role, core.RoleCheckbox),
		Label:         c.Label,
		TestID:        c.TestID,
		Checked:       shows,
		Indeterminate: mixed,
		Disabled:      c.Disabled,
		OnPress: func() {
			if c.OnChange != nil {
				c.OnChange(mixed() || !checked())
			}
		},
		Children: append([]core.Node{core.View{
			Style:         c.IndicatorStyle,
			StyleFunc:     c.IndicatorStyleFunc,
			Checked:       shows,
			Indeterminate: mixed,
			Disabled:      c.Disabled,
			Children:      []core.Node{c.Indicator},
		}}, c.Children...),
	}
}

// Switch is on or off, with no look of its own: a track holding a thumb,
// then its children. The app owns the value; a press reports the next one
// to OnChange. The track's and thumb's state styles follow the switch's.
type Switch struct {
	Checked  func() bool
	Disabled func() bool
	OnChange func(checked bool)

	// Each StyleFunc's result merges over its style.
	Style          core.Style
	StyleFunc      func() core.Style
	TrackStyle     core.Style
	TrackStyleFunc func() core.Style
	ThumbStyle     core.Style
	ThumbStyleFunc func() core.Style
	Children       []core.Node
	Label          string
	TestID         string
}

// Render implements core.Node.
func (s Switch) Render(*core.Scope) core.Node {
	checked := orFalse(s.Checked)
	part := func(style core.Style, fn func() core.Style, children ...core.Node) core.View {
		return core.View{Style: style, StyleFunc: fn, Checked: checked, Disabled: s.Disabled, Children: children}
	}
	return core.View{
		Style:     s.Style,
		StyleFunc: s.StyleFunc,
		Role:      core.RoleSwitch,
		Label:     s.Label,
		TestID:    s.TestID,
		Checked:   checked,
		Disabled:  s.Disabled,
		OnPress: func() {
			if s.OnChange != nil {
				s.OnChange(!checked())
			}
		},
		Children: append([]core.Node{part(s.TrackStyle, s.TrackStyleFunc, part(s.ThumbStyle, s.ThumbStyleFunc))}, s.Children...),
	}
}

// Radio is one option of a RadioGroup, with no look of its own. A press
// on an unchecked radio reports true to OnChange; the app checks it and
// unchecks the others.
type Radio struct {
	Checked  func() bool
	Disabled func() bool
	OnChange func(checked bool)

	Style     core.Style
	StyleFunc func() core.Style
	Children  []core.Node
	Label     string
	TestID    string
}

// Render implements core.Node.
func (r Radio) Render(*core.Scope) core.Node {
	checked := orFalse(r.Checked)
	return core.View{
		Style:     r.Style,
		StyleFunc: r.StyleFunc,
		Role:      core.RoleRadio,
		Label:     r.Label,
		TestID:    r.TestID,
		Checked:   checked,
		Disabled:  r.Disabled,
		OnPress: func() {
			if r.OnChange != nil && !checked() {
				r.OnChange(true)
			}
		},
		Children: r.Children,
	}
}

// RadioGroup groups radios for assistive technology.
type RadioGroup struct {
	Style     core.Style
	StyleFunc func() core.Style
	Children  []core.Node
	Label     string
	TestID    string
}

// Render implements core.Node.
func (g RadioGroup) Render(*core.Scope) core.Node {
	return core.View{Style: g.Style, StyleFunc: g.StyleFunc, Role: core.RoleRadioGroup, Label: g.Label, TestID: g.TestID, Children: g.Children}
}

// Tab is one tab of a Tabs list, with no look of its own. OnPress
// reports a press; the app selects the tab.
type Tab struct {
	Selected func() bool
	Disabled func() bool
	OnPress  func()

	// Style's Selected state style applies while the tab is selected.
	Style     core.Style
	StyleFunc func() core.Style
	Children  []core.Node
	Label     string
	TestID    string
}

// Render implements core.Node.
func (t Tab) Render(*core.Scope) core.Node {
	return core.View{
		Style:     t.Style,
		StyleFunc: t.StyleFunc,
		Role:      core.RoleTab,
		Label:     t.Label,
		TestID:    t.TestID,
		Selected:  orFalse(t.Selected),
		Disabled:  t.Disabled,
		OnPress:   orNop(t.OnPress),
		Children:  t.Children,
	}
}

// Tabs is a list of tabs.
type Tabs struct {
	Style     core.Style
	StyleFunc func() core.Style
	Children  []core.Node
	Label     string
	TestID    string
}

// Render implements core.Node.
func (t Tabs) Render(*core.Scope) core.Node {
	return core.View{Style: t.Style, StyleFunc: t.StyleFunc, Role: core.RoleTabList, Label: t.Label, TestID: t.TestID, Children: t.Children}
}

func orFalse(fn func() bool) func() bool {
	if fn == nil {
		return func() bool { return false }
	}
	return fn
}

func orNop(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	return fn
}

func orRole(r, def core.Role) core.Role {
	if r == core.RoleNone {
		return def
	}
	return r
}
