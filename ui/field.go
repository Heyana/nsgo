package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Field is the platform's text field with base.Input's fields, in a text
// box's fill and rim that follow neither the pointer nor focus; its own
// style sets its size and padding. Its Style and then its StyleFunc merge
// over that look, so core.Transparent in Background and core.Zero in
// BorderWidth turn the box off and leave the text alone.
type Field base.Input

// Render implements core.Node.
func (f Field) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	in := base.Input(f)
	static, own := in.Style, in.StyleFunc
	in.Style = core.Style{FontSize: 13}.Merge(static)
	// The caller's style goes over the look's colors as well: the core merges a dynamic style over a static one, so a static color would otherwise lose to the look's.
	in.StyleFunc = func() core.Style {
		s := core.Style{Color: colors.Text(), PlaceholderColor: colors.Faint(), SelectionColor: colors.Accent(),
			Background: colors.Field(), BorderWidth: 1, BorderColor: colors.FieldBorder(), CornerRadius: RadiusControl, BorderCurve: core.BorderCurveContinuous}
		s = s.Merge(static)
		if own != nil {
			s = s.Merge(own())
		}
		return s
	}
	return in
}
