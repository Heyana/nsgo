package ui

import (
	"cmp"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// FormRowVariant is a FormRow's look.
type FormRowVariant uint8

const (
	// FormRowDefault is a setting: the title in the text's color, and a
	// chevron at the end when it can be pressed.
	FormRowDefault FormRowVariant = iota
	// FormRowAccent is an action: the title in the accent.
	FormRowAccent
	// FormRowDanger is a destructive action, or what went wrong: the
	// title in the danger color.
	FormRowDanger
)

// FormRow is a row of a FormSection: Leading or an Icon on a tile, Title over Detail, then Value in the muted color and the Control. With OnPress it is a button; core.RoleRadio gives it base.Radio's controlled selection and keyboard behavior within a base.RadioGroup.
type FormRow struct {
	Icon func() *core.SVG
	// TileSize is the side of the icon tile in points, 24 when zero.
	TileSize core.Points
	// Leading replaces the icon. LeadingWidth is the width reserved for
	// it in points, 24 when zero, and aligns the section's divider.
	Leading       core.Node
	LeadingWidth  core.Points
	Title, Detail string
	// TitleFunc and DetailFunc update text without rebuilding the row.
	TitleFunc, DetailFunc   func() string
	TitleStyle, DetailStyle core.Style
	// Below follows the title and detail in their text column.
	Below core.Node
	// TitleMaxLines limits the title; zero leaves it unlimited.
	TitleMaxLines int
	// MinHeight overrides the row's minimum height in points; zero keeps its default.
	MinHeight core.Points
	// Role overrides the row's role. core.RoleRadio uses base.Radio behavior in a base.RadioGroup, with Checked supplying the controlled state.
	Role     core.Role
	Checked  func() bool
	Value    func() string
	Control  core.Node
	OnPress  func()
	Disabled func() bool
	Variant  FormRowVariant
	// Label names a pressable row for assistive technology, Title by
	// default.
	Label string
	// LabelFunc binds an explicit accessible name; an empty name uses Title.
	LabelFunc func() string
	TestID    string
}

// Render implements core.Node.
func (r FormRow) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	tint := colors.Text
	switch r.Variant {
	case FormRowAccent:
		tint = colors.Accent
	case FormRowDanger:
		tint = colors.Danger
	}
	var children []core.Node
	width := r.leadingWidth()
	if r.Leading != nil {
		children = append(children, core.View{Style: core.Style{Width: core.Pt(width), FlexShrink: 0,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{r.Leading}})
	} else if r.Icon != nil {
		fill := colors.AccentFill
		if r.Variant == FormRowDanger {
			fill = colors.Danger
		}
		children = append(children, core.View{Style: core.Style{Width: core.Pt(width), Height: core.Pt(width), FlexShrink: 0,
			CornerRadius: width / 4, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			StyleFunc: Filled(fill), Children: []core.Node{Icon(r.Icon, width*14/formIconTile, colors.OnAccent)}})
	}
	texts := []core.Node{core.Text{Content: r.Title, ContentFunc: r.TitleFunc, MaxLines: r.TitleMaxLines, Style: TypeBody.Merge(r.TitleStyle),
		StyleFunc: func() core.Style { return core.Style{Color: cmp.Or(r.TitleStyle.Color, tint())} }}}
	if r.Detail != "" || r.DetailFunc != nil {
		texts = append(texts, core.Show{When: func() bool { return textValue(r.Detail, r.DetailFunc) != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{Content: r.Detail, ContentFunc: r.DetailFunc,
				Style:     core.Style{FontSize: 11.5, FontWeight: Regular, LineHeight: 15}.Merge(r.DetailStyle),
				StyleFunc: func() core.Style { return core.Style{Color: cmp.Or(r.DetailStyle.Color, colors.Muted())} }}
		}})
	}
	if r.Below != nil {
		texts = append(texts, r.Below)
	}
	children = append(children, core.View{Style: core.Style{Gap: 1, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: texts})
	if r.Value != nil {
		value := Textf(r.Value, 13, Regular, colors.Muted)
		value.MaxLines, value.Style.FlexShrink, value.Style.MinWidth = 1, 1, core.Pt(0)
		children = append(children, value)
	}
	if r.Control != nil {
		children = append(children, core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{r.Control}})
	}
	if r.OnPress != nil && r.Role != core.RoleRadio && r.Variant == FormRowDefault {
		children = append(children, Icon(lucide.ChevronRight, 13, colors.Faint))
	}
	style := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: formRowGap, MinHeight: core.Pt(cmp.Or(r.MinHeight, formRowHeightFor(cx))),
		Padding: core.Symmetric(6, formRowPad)}
	label, labelFunc := textBinding(r.Label, r.LabelFunc, r.Title, r.TitleFunc)
	if r.Role == core.RoleRadio {
		// base.Radio takes a fixed Label, so a bound name is read once as the row mounts.
		if labelFunc != nil {
			label = core.Untracked(cx, labelFunc)
		}
		style.Disabled = Dimmed()
		return base.Radio{Label: label, TestID: r.TestID, Checked: r.Checked, Disabled: r.Disabled,
			OnChange: func(bool) {
				if r.OnPress != nil {
					r.OnPress()
				}
			}, Style: style, StyleFunc: func() core.Style { return core.Style{FocusVisible: FocusRing(cx, -2)} },
			Children: children}
	}
	if r.OnPress == nil {
		if r.Role == core.RoleNone {
			label, labelFunc = "", nil
		}
		return core.View{TestID: r.TestID, Role: r.Role, Label: label, LabelFunc: labelFunc, Style: style, Children: children}
	}
	style.Disabled = Dimmed()
	return base.Button{Role: r.Role, Label: label, LabelFunc: labelFunc, TestID: r.TestID, Disabled: r.Disabled, OnPress: func(core.PressEvent) { r.OnPress() },
		Style: style,
		StyleFunc: func() core.Style {
			return core.Style{Active: &core.Style{Background: colors.Hover()}, FocusVisible: FocusRing(cx, -2)}
		},
		Children: children}
}

// leadingWidth is the slot used by both the row and its hairline.
func (r FormRow) leadingWidth() core.Points {
	switch {
	case r.Leading != nil:
		return cmp.Or(r.LeadingWidth, formIconTile)
	case r.Icon != nil:
		return cmp.Or(r.TileSize, formIconTile)
	default:
		return 0
	}
}
