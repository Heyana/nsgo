package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Input is the platform's text field with base.Input's fields, styled as a text box 28 pt tall with 13 pt text, or Multiline growing from three lines: its rim darker under the pointer and the accent while focused. Its Style and then its StyleFunc merge over that look, so the focus and hover rims stay while neither sets BorderColor or Hover. Its Padding replaces ui's 10 points a side, and core.Zero takes it away.
type Input base.Input

// Render implements core.Node.
func (f Input) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	in := base.Input(f)
	focused := core.NewSignal(cx, false)
	onFocus, onBlur := in.OnFocus, in.OnBlur
	in.OnFocus = func() {
		focused.Set(true)
		if onFocus != nil {
			onFocus()
		}
	}
	in.OnBlur = func() {
		focused.Set(false)
		if onBlur != nil {
			onBlur()
		}
	}
	shape := core.Style{Height: core.Pt(ButtonHeight), Padding: core.Symmetric(0, 10), FontSize: 13}
	if in.Multiline {
		// A multiline field's three-line floor is in the look below, where the caller's height can drop it: a minimum would outgrow a smaller height.
		shape = core.Style{Padding: core.Symmetric(6, 10), TextAlignVertical: core.TextAlignVerticalTop, FontSize: 13}
	}
	// The caller's style goes over the look's colors as well as its shape: the core merges a dynamic style over a static one, so a static color would otherwise lose to the look's.
	static, own := in.Style, in.StyleFunc
	in.Style = shape.Merge(static)
	in.StyleFunc = func() core.Style {
		s := core.Style{PlaceholderColor: colors.Faint(), SelectionColor: cx.Palette().Selection, CaretColor: colors.Accent(),
			Color: colors.Text(), Background: colors.Field(),
			BorderWidth: 1, BorderColor: colors.FieldBorder(), CornerRadius: RadiusControl,
			Hover: &core.Style{BorderColor: Mix(colors.FieldBorder(), colors.Text(), 0.25)}}
		if focused.Get() {
			s.BorderColor, s.Hover = colors.Accent(), nil
		}
		s = s.Merge(static)
		if own != nil {
			s = s.Merge(own())
		}
		if in.Multiline && s.Height == (core.Length{}) && s.MinHeight == (core.Length{}) {
			s.MinHeight = core.Pt(ButtonHeight + 2*TypeBody.LineHeight)
		}
		return s
	}
	return in
}
