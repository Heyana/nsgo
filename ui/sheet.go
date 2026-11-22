package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Sheet is a panel at the bottom over a dimmed backdrop while Open
// (base.Sheet in ui's look); a press on the backdrop, Escape or
// the back button calls OnClose.
type Sheet struct {
	Open    func() bool
	OnClose func()
	// CloseOnSwipe lets a downward drag ask to close the sheet.
	CloseOnSwipe bool
	// Label names the sheet for screen readers; LabelFunc follows the signals it reads.
	Label     string
	LabelFunc func() string
	TestID    string
	// Style merges over the sheet's surface style. A CornerRadius without CornerRadii rounds every corner in place of the default rounded top.
	Style core.Style
	// StyleFunc merges over Style and follows the signals it reads.
	StyleFunc func() core.Style
	Children  []core.Node
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	env := cx.Env()
	return base.Sheet{
		Open:         s.Open,
		Label:        s.Label,
		LabelFunc:    s.LabelFunc,
		TestID:       s.TestID,
		CloseOnSwipe: s.CloseOnSwipe,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				s.OnClose()
			}
		},
		BackdropStyle: core.Style{Background: Black(0.4)},
		PopupStyleFunc: func() core.Style {
			style := core.Style{Gap: 12, Padding: core.Edges{Top: 8, Right: 16, Bottom: env.SafeArea().Bottom + 16, Left: 16},
				CornerRadii: core.Corners{TopLeft: 16, TopRight: 16}, Background: colors.Surface()}
			style = mergeSheetSurface(style, s.Style)
			if s.StyleFunc != nil {
				style = mergeSheetSurface(style, s.StyleFunc())
			}
			return style
		},
		Children: append([]core.Node{core.View{Style: core.Style{Width: core.Pt(36), Height: core.Pt(5), CornerRadius: 3, AlignSelf: core.AlignCenter},
			StyleFunc: Filled(colors.Border)}}, s.Children...),
	}
}

// mergeSheetSurface merges o over the surface style s. The default rounds the top corners with CornerRadii, which would outrank a CornerRadius in o, so a CornerRadius without CornerRadii drops the default radii.
func mergeSheetSurface(s, o core.Style) core.Style {
	if o.CornerRadius != 0 && o.CornerRadii == (core.Corners{}) {
		s.CornerRadii = core.Corners{}
	}
	return s.Merge(o)
}
