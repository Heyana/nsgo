package ui

import (
	"math"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
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

// Checkbox is a box with a check mark, then its children or Label as
// text. HideLabel leaves the text out, keeping Label for assistive
// technology, as where a row's own text names it.
type Checkbox struct {
	Checked       func() bool
	Indeterminate func() bool
	Disabled      func() bool
	OnChange      func(checked bool)
	Style         core.Style
	Children      []core.Node
	Label         string
	HideLabel     bool
	TestID        string
}

// Render implements core.Node.
func (c Checkbox) Render(cx *core.Scope) core.Node {
	checked, mixed := orFalse(c.Checked), orFalse(c.Indeterminate)
	shows := func() bool { return checked() && !mixed() }
	return core.View{
		StyleFunc:     look(cx, c.Style, Tokens.control),
		Role:          core.RoleCheckbox,
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
			StyleFunc:     look(cx, core.Style{}, Tokens.checkboxIndicator),
			Checked:       shows,
			Indeterminate: mixed,
			Disabled:      c.Disabled,
			Children: []core.Node{core.Text{ContentFunc: func() string {
				switch {
				case mixed():
					return "−"
				case checked():
					return "✓"
				}
				return ""
			}}},
		}}, shown(c.Children, c.Label, c.HideLabel)...),
	}
}

// Switch is a track and a sliding thumb, then its children or Label as
// text. HideLabel leaves the text out, as a Checkbox's does.
type Switch struct {
	Checked   func() bool
	Disabled  func() bool
	OnChange  func(checked bool)
	Style     core.Style
	Children  []core.Node
	Label     string
	HideLabel bool
	TestID    string
}

// Render implements core.Node.
func (s Switch) Render(cx *core.Scope) core.Node {
	checked := orFalse(s.Checked)
	track := look(cx, core.Style{}, Tokens.switchTrack)
	thumb := look(cx, core.Style{}, Tokens.switchThumb)
	part := func(style func() core.Style, children ...core.Node) core.View {
		return core.View{StyleFunc: style, Checked: checked, Disabled: s.Disabled, Children: children}
	}
	return core.View{
		StyleFunc: look(cx, s.Style, Tokens.control),
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
		Children: append([]core.Node{part(track, part(thumb))}, shown(s.Children, s.Label, s.HideLabel)...),
	}
}

// Radio is a ring, then its children or Label as text.
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
	checked := orFalse(r.Checked)
	ring := core.View{StyleFunc: look(cx, core.Style{}, Tokens.radioIndicator), Checked: checked, Disabled: r.Disabled}
	return core.View{
		StyleFunc: look(cx, r.Style, Tokens.control),
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
		Children: append([]core.Node{ring}, orLabel(r.Children, r.Label)...),
	}
}

// RadioGroup is a group of radios that spaces them.
type RadioGroup struct {
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (g RadioGroup) Render(cx *core.Scope) core.Node {
	return core.View{StyleFunc: look(cx, g.Style, Tokens.radioGroup), Role: core.RoleRadioGroup,
		Label: g.Label, TestID: g.TestID, Children: g.Children}
}

// Tabs is a list of tabs on a muted track.
type Tabs struct {
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (t Tabs) Render(cx *core.Scope) core.Node {
	return core.View{StyleFunc: look(cx, t.Style, Tokens.tabs), Role: core.RoleTabList,
		Label: t.Label, TestID: t.TestID, Children: t.Children}
}

// Tab is raised while selected; its content is its children or Label as
// text.
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
	return core.View{
		StyleFunc: look(cx, t.Style, Tokens.tab),
		Role:      core.RoleTab,
		Label:     t.Label,
		TestID:    t.TestID,
		Selected:  orFalse(t.Selected),
		Disabled:  t.Disabled,
		OnPress:   orNop(t.OnPress),
		Children:  orLabel(t.Children, t.Label),
	}
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

// shown returns children, or label as text unless hidden.
func shown(children []core.Node, label string, hidden bool) []core.Node {
	if hidden {
		return children
	}
	return orLabel(children, label)
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

// IconButton is a base.Button showing only an icon, which Label names for
// assistive technology and, on desktops, in a tooltip. It has no face until
// hovered, and its icon takes the muted text color. With Menu it is a
// base.Menu's trigger instead, opening Menu's items.
type IconButton struct {
	Icon *core.SVG
	// IconFunc replaces Icon, again when what it reads changes.
	IconFunc func() *core.SVG
	OnPress  func()
	Disabled func() bool
	// Menu holds MenuItems, MenuSeparators and Submenus. Placement and
	// Align place the menu as a Menu's do.
	Menu      []core.Node
	Placement core.Placement
	Align     core.Align
	// Size is the icon's; 16 when 0. The button stays 32 points square.
	Size core.Points
	// NoTooltip leaves the tooltip out.
	NoTooltip bool
	Style     core.Style
	Label     string
	TestID    string
}

// Render implements core.Node.
func (b IconButton) Render(cx *core.Scope) core.Node {
	size := b.Size
	if size == 0 {
		size = 16
	}
	icon := core.Icon{SVG: b.Icon, SVGFunc: b.IconFunc, Style: core.Style{Width: core.Pt(size), Height: core.Pt(size)}}
	var button core.Node = base.Button{OnPress: b.OnPress, Disabled: b.Disabled,
		StyleFunc: look(cx, b.Style, Tokens.iconButton), Label: b.Label, TestID: b.TestID,
		Children: []core.Node{icon}}
	if len(b.Menu) > 0 {
		button = base.Menu{Placement: b.Placement, Align: b.Align, Offset: 4, Margin: 8,
			StyleFunc: look(cx, b.Style, Tokens.iconButton), PopupStyleFunc: look(cx, core.Style{}, Tokens.popup),
			Trigger: icon, Children: b.Menu, Label: b.Label, TestID: b.TestID}
	}
	if b.NoTooltip || b.Label == "" {
		return button
	}
	return Tooltip{Text: b.Label, Trigger: button}
}

// Input is a core.Input in the theme's field: a bordered box that marks
// focus with the ring color. Icon, when set, leads the field, as a search
// field's magnifier does.
type Input struct {
	Value        func() string
	DefaultValue string
	Placeholder  string
	Icon         *core.SVG
	Masked       bool
	Readonly     bool
	Disabled     func() bool
	KeyboardType core.KeyboardType
	AutoFocus    bool
	OnChange     func(text string)
	OnSubmit     func(text string)
	OnFocus      func()
	OnBlur       func()
	Ref          *core.Ref
	// Style merges over the field's look.
	Style  core.Style
	Label  string
	TestID string
}

// Render implements core.Node.
func (i Input) Render(cx *core.Scope) core.Node {
	input := core.Input{Value: i.Value, DefaultValue: i.DefaultValue, Placeholder: i.Placeholder, Masked: i.Masked,
		Readonly: i.Readonly, Disabled: i.Disabled, KeyboardType: i.KeyboardType, AutoFocus: i.AutoFocus,
		OnChange: i.OnChange, OnSubmit: i.OnSubmit, OnFocus: i.OnFocus, OnBlur: i.OnBlur,
		Ref: i.Ref, Label: i.Label, TestID: i.TestID}
	if i.Icon == nil {
		input.StyleFunc = look(cx, i.Style, Tokens.field)
		return input
	}
	// The icon and the input share the field's frame, which shows the
	// input's focus itself: a state style applies to its own view only.
	theme := UseTheme(cx)
	focused := core.NewSignal(cx, false)
	input.OnFocus = func() {
		focused.Set(true)
		if i.OnFocus != nil {
			i.OnFocus()
		}
	}
	input.OnBlur = func() {
		focused.Set(false)
		if i.OnBlur != nil {
			i.OnBlur()
		}
	}
	input.StyleFunc = func() core.Style {
		t := theme()
		return core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pct(100),
			Color: t.Foreground, FontSize: i.Style.FontSize}
	}
	return core.View{
		StyleFunc: func() core.Style {
			t := theme()
			s := t.field()
			s.Direction, s.AlignItems, s.Gap = core.Row, core.AlignCenter, 8
			if focused.Get() {
				s.BorderColor, s.Hover = t.Ring, nil
			}
			return s.Merge(i.Style)
		},
		Disabled: i.Disabled,
		Children: []core.Node{
			core.Icon{SVG: i.Icon, StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0, Color: theme().MutedForeground}
			}},
			input,
		},
	}
}

// SelectOption is one choice of a Select: the value it reports, the text
// it shows and an optional icon before that.
type SelectOption struct {
	Value, Label string
	Icon         *core.SVG
}

// Select is a field showing the chosen option, which opens a menu of the
// options with the chosen one checked. The app owns the value: choosing
// reports it to OnChange.
type Select struct {
	Value    func() string
	Options  []SelectOption
	OnChange func(value string)
	// Placeholder shows while no option has the value.
	Placeholder string
	// Placement and Align place the menu; below and start-aligned when 0.
	Placement core.Placement
	Align     core.Align

	// Style merges over the field's look and PopupStyle over the menu's.
	Style      core.Style
	PopupStyle core.Style
	Label      string
	TestID     string
}

// Render implements core.Node.
func (s Select) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	value := s.Value
	if value == nil {
		value = func() string { return "" }
	}
	chosen := func() (SelectOption, bool) {
		v := value()
		for _, o := range s.Options {
			if o.Value == v {
				return o, true
			}
		}
		return SelectOption{}, false
	}
	items := make([]core.Node, 0, len(s.Options))
	for _, o := range s.Options {
		on := func() bool { return value() == o.Value }
		var row []core.Node
		if o.Icon != nil {
			row = append(row, core.Icon{SVG: o.Icon, Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0}})
		}
		items = append(items, MenuItem{Label: o.Label, Checked: on, Children: append(row, core.Text{Content: o.Label}),
			OnSelect: func() {
				if s.OnChange != nil && !on() {
					s.OnChange(o.Value)
				}
			}})
	}
	place, align := s.Placement, s.Align
	if place == core.PlaceDefault {
		place = core.PlaceBottom
	}
	if align == 0 {
		align = core.AlignStart
	}
	return base.Menu{
		Placement: place, Align: align, Offset: 4, Margin: 8,
		StyleFunc: look(cx, s.Style, Tokens.selectField),
		PopupStyleFunc: look(cx, s.PopupStyle, func(t Tokens) core.Style {
			popup := t.popup()
			popup.MinWidth = core.Pt(180)
			return popup
		}),
		Trigger: core.Group{
			core.Text{MaxLines: 1,
				ContentFunc: func() string {
					if o, ok := chosen(); ok {
						return o.Label
					}
					return s.Placeholder
				},
				StyleFunc: func() core.Style {
					style := core.Style{FlexGrow: 1, FlexShrink: 1}
					if _, ok := chosen(); !ok {
						style.Color = theme().MutedForeground
					}
					return style
				}},
			core.Icon{SVG: lucide.ChevronsUpDown(), StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(14), Height: core.Pt(14), FlexShrink: 0, Color: theme().MutedForeground}
			}},
		},
		Children: items, Label: s.Label, TestID: s.TestID,
	}
}

// Slider is a thin track filled with the primary color up to a round
// thumb. The app owns the value: a drag reports each value it passes to
// OnChange, snapped to Step, and the slider shows what Value returns.
type Slider struct {
	Value    func() float64
	Min, Max float64
	Step     float64
	Disabled func() bool
	OnChange func(value float64)
	Style    core.Style
	Label    string
	TestID   string
}

// Render implements core.Node.
func (s Slider) Render(cx *core.Scope) core.Node {
	lo, hi := s.Min, s.Max
	if hi <= lo {
		lo, hi = 0, 1
	}
	value := func() float64 {
		if s.Value == nil {
			return lo
		}
		return math.Min(math.Max(s.Value(), lo), hi)
	}
	fraction := func() float32 { return float32((value() - lo) / (hi - lo)) }
	width := core.NewSignal(cx, core.Points(0))
	var start float64
	disabled := orFalse(s.Disabled)
	pan := func(e core.PanEvent) {
		w := width.GetUntracked()
		if disabled() || w <= 0 || s.OnChange == nil {
			return
		}
		switch e.Phase {
		case core.GestureBegan:
			start = core.Untracked(cx, value)
		case core.GestureEnded, core.GestureCancelled:
			return
		}
		v := start + float64(e.Translation.X/w)*(hi-lo)
		if s.Step > 0 {
			v = lo + math.Round((v-lo)/s.Step)*s.Step
		}
		v = math.Min(math.Max(v, lo), hi)
		if v != core.Untracked(cx, value) {
			s.OnChange(v)
		}
	}
	step := s.Step
	if step <= 0 {
		step = (hi - lo) / 10
	}
	adjust := func(name string) {
		if s.OnChange == nil {
			return
		}
		v := core.Untracked(cx, value)
		switch name {
		case core.ActionIncrement:
			v = math.Min(v+step, hi)
		case core.ActionDecrement:
			v = math.Max(v-step, lo)
		default:
			return
		}
		if v != core.Untracked(cx, value) {
			s.OnChange(v)
		}
	}
	track := look(cx, core.Style{}, Tokens.sliderTrack)
	fill := look(cx, core.Style{}, Tokens.sliderFill)
	thumb := look(cx, core.Style{}, Tokens.sliderThumb)
	return core.View{
		StyleFunc: look(cx, s.Style, Tokens.slider),
		Label:     s.Label,
		TestID:    s.TestID,
		Disabled:  s.Disabled,
		Role:      core.RoleAdjustable,
		AccessibilityValue: func() core.AccessibilityValue {
			return core.AccessibilityValue{Min: float32(lo), Max: float32(hi), Now: float32(value())}
		},
		AccessibilityActions:  sliderActions,
		OnAccessibilityAction: adjust,
		OnPan:                 pan,
		PanActivation:         core.PanActivation{ActiveOffsetX: 2, FailOffsetY: 12},
		Children: []core.Node{core.View{
			StyleFunc: track, Disabled: s.Disabled,
			OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
			Children: []core.Node{
				core.View{Disabled: s.Disabled, StyleFunc: func() core.Style {
					bar := core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0),
						Width: core.Pct(fraction() * 100)}
					return bar.Merge(fill())
				}},
				core.View{StyleFunc: func() core.Style {
					return core.Style{Position: core.Absolute, Left: core.Pct(fraction() * 100), Top: core.Pt(0), Bottom: core.Pt(0),
						Width: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
				}, Children: []core.Node{core.View{StyleFunc: thumb, Disabled: s.Disabled}}},
			},
		}},
	}
}

// sliderActions are what assistive technology adjusts a slider with.
var sliderActions = []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}}
