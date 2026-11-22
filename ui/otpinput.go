package ui

import (
	"cmp"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// OtpInput is a one-time code in bordered cells, the one that takes the
// next digit ringed in the accent: base.OtpInput's one native field over
// them, with the system's code autofill.
type OtpInput struct {
	Length int
	// Value makes the code controlled; DefaultValue starts its own.
	Value        func() string
	DefaultValue string
	Masked       bool
	Disabled     func() bool
	// Groups is how many groups the cells split into.
	Groups int
	// Size is the default width and height of each cell in points: 36 on desktops and TVs, 44 on touch devices when zero.
	Size core.Points
	// CellWidth and CellHeight override Size independently.
	CellWidth, CellHeight core.Points
	// TextStyle sets the cell's typography over TypeBody.
	TextStyle core.Style
	// CornerRadius defaults to RadiusControl when zero.
	CornerRadius core.Points
	// CaretHeight defaults to the cell text's line height less two points, with a one-point minimum.
	CaretHeight          core.Points
	OnChange, OnComplete func(value string)
	// Label is what assistive technology says for the field.
	Label string
	// LabelFunc binds a changing Label.
	LabelFunc func() string
	Ref       *base.OtpInputRef
	TestID    string
	Style     core.Style
}

// Render implements core.Node.
func (o OtpInput) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	cell := o.Size
	if cell == 0 {
		cell = ButtonHeight + 8
		if Touch(cx) {
			cell = 44
		}
	}
	text := TypeBody.Merge(o.TextStyle)
	lineHeight := text.LineHeight
	switch {
	case text.LineHeightMultiple > 0:
		lineHeight = text.FontSize * core.Points(text.LineHeightMultiple)
	case text.LineHeightEm != 0:
		lineHeight = text.FontSize * core.Points(text.LineHeightEm)
	case o.TextStyle.FontSize != 0 && o.TextStyle.LineHeight == 0:
		// Keep TypeBody's line height proportional when only the font size changes.
		lineHeight = TypeBody.LineHeight * text.FontSize / TypeBody.FontSize
		text.LineHeight = lineHeight
	}
	caretHeight := cmp.Or(o.CaretHeight, max(lineHeight-2, 1))
	return base.OtpInput{Length: o.Length, Value: o.Value, DefaultValue: o.DefaultValue, Masked: o.Masked, Disabled: o.Disabled, Groups: o.Groups,
		OnChange: o.OnChange, OnComplete: o.OnComplete, Label: o.Label, LabelFunc: o.LabelFunc, Ref: o.Ref, TestID: o.TestID,
		Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignStart, Gap: 12}.Merge(o.Style),
		GroupStyle: core.Style{Direction: core.Row, Gap: 6},
		CellStyle: text.Merge(core.Style{Width: core.Pt(cmp.Or(o.CellWidth, cell)), Height: core.Pt(cmp.Or(o.CellHeight, cell)),
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: cmp.Or(o.CornerRadius, RadiusControl), BorderWidth: 1, Transition: quick, Disabled: Dimmed()}),
		CellStyleFunc: func() core.Style {
			return core.Style{Color: colors.Text(), Background: colors.Field(), BorderColor: colors.FieldBorder(),
				Focus: &core.Style{BorderColor: colors.Accent()}}
		},
		CaretStyle:     core.Style{Width: core.Pt(1.5), Height: core.Pt(caretHeight)},
		CaretStyleFunc: Filled(colors.Accent)}
}
