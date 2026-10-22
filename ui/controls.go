package ui

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// Variant is the look of a button.
type Variant uint8

const (
	// Primary is the main action, the default.
	Primary Variant = iota
	// Secondary is a quiet action.
	Secondary
	// Danger is a destructive action.
	Danger
	// Ghost has no face until hovered.
	Ghost
)

// Button is a base.Button with the theme's look for its Variant. Style
// merges over that look; the content is its children, or Label as text.
type Button struct {
	OnPress     func()
	OnLongPress func()
	Disabled    func() bool
	Variant     Variant
	Style       core.Style
	Label       string
	TestID      string
	Children    []core.Node
}

// Render implements core.Node.
func (b Button) Render(cx *core.Scope) core.Node {
	return base.Button{
		OnPress: b.OnPress, OnLongPress: b.OnLongPress, Disabled: b.Disabled,
		StyleFunc: look(cx, b.Style, func(t Tokens) core.Style { return t.button(b.Variant) }),
		Label:     b.Label, TestID: b.TestID, Children: orLabel(b.Children, b.Label),
	}
}

// Checkbox is a base.Checkbox drawn as a box with a check mark, then its
// children or Label as text.
type Checkbox struct {
	Checked       func() bool
	Indeterminate func() bool
	Disabled      func() bool
	OnChange      func(checked bool)
	Style         core.Style
	Children      []core.Node
	Label         string
	TestID        string
}

// Render implements core.Node.
func (c Checkbox) Render(cx *core.Scope) core.Node {
	checked, mixed := orFalse(c.Checked), orFalse(c.Indeterminate)
	return base.Checkbox{
		Checked: c.Checked, Indeterminate: c.Indeterminate, Disabled: c.Disabled, OnChange: c.OnChange,
		StyleFunc:          look(cx, c.Style, Tokens.control),
		IndicatorStyleFunc: look(cx, core.Style{}, Tokens.checkboxIndicator),
		Indicator: core.Text{ContentFunc: func() string {
			switch {
			case mixed():
				return "−"
			case checked():
				return "✓"
			}
			return ""
		}},
		Children: orLabel(c.Children, c.Label), Label: c.Label, TestID: c.TestID,
	}
}

// Switch is a base.Switch drawn as a track and a sliding thumb, then its
// children or Label as text.
type Switch struct {
	Checked  func() bool
	Disabled func() bool
	OnChange func(checked bool)
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (s Switch) Render(cx *core.Scope) core.Node {
	return base.Switch{
		Checked: s.Checked, Disabled: s.Disabled, OnChange: s.OnChange,
		StyleFunc:      look(cx, s.Style, Tokens.control),
		TrackStyleFunc: look(cx, core.Style{}, Tokens.switchTrack),
		ThumbStyleFunc: look(cx, core.Style{}, Tokens.switchThumb),
		Children:       orLabel(s.Children, s.Label), Label: s.Label, TestID: s.TestID,
	}
}

// Radio is a base.Radio drawn as a ring, then its children or Label as
// text.
type Radio struct {
	Checked  func() bool
	Disabled func() bool
	OnChange func(checked bool)
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (r Radio) Render(cx *core.Scope) core.Node {
	ring := core.View{StyleFunc: look(cx, core.Style{}, Tokens.radioIndicator),
		Checked: orFalse(r.Checked), Disabled: r.Disabled}
	return base.Radio{
		Checked: r.Checked, Disabled: r.Disabled, OnChange: r.OnChange,
		StyleFunc: look(cx, r.Style, Tokens.control),
		Children:  append([]core.Node{ring}, orLabel(r.Children, r.Label)...),
		Label:     r.Label, TestID: r.TestID,
	}
}

// RadioGroup is a base.RadioGroup that spaces its radios.
type RadioGroup struct {
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (g RadioGroup) Render(cx *core.Scope) core.Node {
	return base.RadioGroup{StyleFunc: look(cx, g.Style, Tokens.radioGroup), Children: g.Children,
		Label: g.Label, TestID: g.TestID}
}

// Tabs is a base.Tabs on a muted track.
type Tabs struct {
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (t Tabs) Render(cx *core.Scope) core.Node {
	return base.Tabs{StyleFunc: look(cx, t.Style, Tokens.tabs), Children: t.Children, Label: t.Label, TestID: t.TestID}
}

// Tab is a base.Tab raised while selected; its content is its children or
// Label as text.
type Tab struct {
	Selected func() bool
	Disabled func() bool
	OnPress  func()
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (t Tab) Render(cx *core.Scope) core.Node {
	return base.Tab{Selected: t.Selected, Disabled: t.Disabled, OnPress: t.OnPress,
		StyleFunc: look(cx, t.Style, Tokens.tab), Children: orLabel(t.Children, t.Label),
		Label: t.Label, TestID: t.TestID}
}

// look returns a style function of the theme's look with style merged
// over it.
func look(cx *core.Scope, style core.Style, of func(Tokens) core.Style) func() core.Style {
	theme := UseTheme(cx)
	return func() core.Style { return of(theme()).Merge(style) }
}

// orLabel returns children, or label as text when there are none.
func orLabel(children []core.Node, label string) []core.Node {
	if len(children) == 0 && label != "" {
		return []core.Node{core.Text{Content: label}}
	}
	return children
}

func orFalse(fn func() bool) func() bool {
	if fn == nil {
		return func() bool { return false }
	}
	return fn
}
