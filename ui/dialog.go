package ui

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

// Dialog is a modal surface over a dimmed app and a sheet from the bottom
// on touch screens: a title, a description, the app's children and its
// Footer of actions. On desktops it pops in and fades out
// does, centered across and with two thirds of the spare height below it.
type Dialog struct {
	Open         func() bool
	OnOpenChange func(open bool, reason base.Reason)
	// OnClose follows it being gone, once its motion ended.
	OnClose func()
	// OnConfirm is the default action Return runs before it closes with ReasonSelect, as base.Dialog's.
	OnConfirm       func()
	Title           string
	TitleFunc       func() string
	Description     string
	DescriptionFunc func() string
	// Leading and Trailing sit at the start and end of the title row, such as a sheet's Cancel and Done. While either is set, the title is centered between two sides of one width, where it wraps rather than run under them.
	Leading  []core.Node
	Trailing []core.Node
	// TitleStyle overrides the title's typography on every presentation.
	TitleStyle     core.Style
	TitleStyleFunc func() core.Style
	Children       []core.Node
	// Footer holds the actions, at the end of a row on desktops, the
	// last first in a stack across a sheet.
	Footer                 []core.Node
	NoCloseOnEscape        bool
	NoCloseOnBackdropPress bool
	// Inset is the least distance from the window's edges on desktops, 20
	// when 0; a title bar the app draws in takes it further. The strip of
	// that title bar keeps moving the window under the backdrop.
	Inset core.Points
	// Bare leaves out the surface's padding, gap and width, for content that fills it, sizes it and is clipped inside its rim to the corners Style and StyleFunc give it.
	Bare bool
	// NoGrabber hides the touch sheet's default drag handle.
	NoGrabber bool
	// ContentStyle and ContentStyleFunc override the touch sheet's padding and gap.
	ContentStyle     core.Style
	ContentStyleFunc func() core.Style
	// StartingStyle and EndingStyle override the touch sheet's entry and exit styles. Include a Transition for custom timing.
	StartingStyle core.Style
	EndingStyle   core.Style
	// Label names the dialog for assistive technology, Title when it and LabelFunc are not given, which reads Description after it. Role makes it an alert dialog, one asking for an answer, as a confirmation.
	Label     string
	LabelFunc func() string
	Role      core.Role
	TestID    string
	// Style merges over the surface's.
	Style core.Style
	// StyleFunc merges over Style and follows the signals it reads.
	StyleFunc func() core.Style
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	label := d.Label
	labelFunc := d.LabelFunc
	if label == "" && labelFunc == nil {
		label = d.Title
		if d.TitleFunc != nil {
			labelFunc = d.TitleFunc
		}
	}
	var body []core.Node
	if d.Title != "" || d.TitleFunc != nil || d.Description != "" || d.DescriptionFunc != nil {
		var head []core.Node
		if d.Title != "" || d.TitleFunc != nil {
			title := core.Text{Content: d.Title, ContentFunc: d.TitleFunc, Role: core.RoleHeader, Style: TypeTitle.Merge(d.TitleStyle), StyleFunc: func() core.Style {
				s := Colored(colors.Text)()
				if d.TitleStyleFunc != nil {
					s = s.Merge(d.TitleStyleFunc())
				}
				return s
			}}
			if d.Leading != nil || d.Trailing != nil {
				// Two sides that grow alike keep the title in the middle; the title shrinks and wraps between them.
				side := func(id string, justify core.Justify, children []core.Node) core.Node {
					return core.View{TestID: id, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: justify, FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.AutoMinimum, Gap: 8}, Children: children}
				}
				title.Style = core.Style{FlexShrink: 1, TextAlign: core.TextAlignCenter}.Merge(title.Style)
				head = append(head, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, MinHeight: core.Pt(32), Gap: 8}, Children: []core.Node{
					side("dialog-title-leading", core.JustifyStart, d.Leading), title, side("dialog-title-trailing", core.JustifyEnd, d.Trailing),
				}})
			} else {
				head = append(head, title)
			}
		}
		if d.Description != "" || d.DescriptionFunc != nil {
			head = append(head, core.Text{Content: d.Description, ContentFunc: d.DescriptionFunc, Style: TypeBody, StyleFunc: Colored(colors.Muted)})
		}
		body = append(body, core.View{Style: core.Style{Gap: 4}, Children: head})
	}
	body = append(body, d.Children...)
	if Touch(cx) {
		if d.Footer != nil {
			body = append(body, core.View{Style: core.Style{Direction: core.ColumnReverse, Gap: 8, Margin: core.Edges{Top: 8}},
				Children: d.Footer})
		}
		children := body
		if !d.Bare {
			children = sheetContent(cx, core.PlaceBottom, body, d.NoGrabber, d.ContentStyle, d.ContentStyleFunc)
		}
		return base.Sheet{Open: d.Open, OnOpenChange: d.OnOpenChange, OnClose: d.OnClose, OnConfirm: d.OnConfirm, NoCloseOnEscape: d.NoCloseOnEscape,
			Role:                   d.Role,
			NoCloseOnBackdropPress: d.NoCloseOnBackdropPress, CloseOnSwipe: !d.NoCloseOnBackdropPress, StartingStyle: d.StartingStyle, EndingStyle: d.EndingStyle,
			BackdropStyle: scrim, PopupStyleFunc: func() core.Style { return d.surfaceStyle(sheetPanel(cx, core.PlaceBottom)) },
			Children: children, Label: label, LabelFunc: labelFunc, AccessibilityDescription: d.Description, AccessibilityDescriptionFunc: d.DescriptionFunc, TestID: d.TestID}
	}
	if d.Footer != nil {
		body = append(body, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyEnd,
			Gap: 8, Margin: core.Edges{Top: 8}}, Children: d.Footer})
	}
	popup := core.Style{Width: core.Pt(440), Padding: core.All(20), Gap: 12, Transition: dialogPop}
	if d.Bare {
		// Keep the clip separate until every Apple content kind preserves its outer decoration (#1362).
		popup = core.Style{Transition: dialogPop}
		body = []core.Node{core.View{Style: core.Style{Overflow: core.Hidden, FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
			StyleFunc: func() core.Style { return bareClip(d.surfaceStyle(dialogShape)) }, Children: body}}
	}
	spacer := func(grow float32) core.View {
		return core.View{Style: core.Style{FlexGrow: grow, MinHeight: core.Pt(0)}, PointerEvents: core.PointerEventsNone}
	}
	return base.Dialog{Open: d.Open, OnOpenChange: d.OnOpenChange, OnClose: d.OnClose, OnConfirm: d.OnConfirm, Animation: core.ModalNone, NoCloseOnEscape: d.NoCloseOnEscape,
		Role: d.Role, Label: label, LabelFunc: labelFunc, AccessibilityDescription: d.Description, AccessibilityDescriptionFunc: d.DescriptionFunc,
		NoCloseOnBackdropPress: d.NoCloseOnBackdropPress, BackdropStyle: core.Style{Background: Black(0.18), Transition: dialogFade},
		BackdropStartingStyle: core.Style{Opacity: core.Alpha(0)}, BackdropEndingStyle: core.Style{Opacity: core.Alpha(0)},
		PopupStyle: popup, StartingStyle: dialogStarting, EndingStyle: dialogEnding,
		PopupStyleFunc: func() core.Style {
			window, inset := cx.Env().Window(), dialogInset(cx, d.Inset)
			finish := FinOf(cx).Dialog()
			return d.surfaceStyle(finish.Over(dialogShape.Merge(core.Style{Background: Alpha(finish.Background, 0.96), Color: colors.Text(),
				MaxWidth: core.Pt(max(0, window.Width-2*inset)), MaxHeight: core.Pt(max(0, window.Height-2*inset)), FlexShrink: 0})))
		},
		LayerStyle: core.Style{JustifyContent: core.JustifyStart}, Before: []core.Node{spacer(1)}, After: []core.Node{spacer(2), dialogDragStrip(cx)},
		Children: body, TestID: d.TestID}
}

// surfaceStyle applies the app's overrides inside the panel's style binding, shared by the centered dialog and the sheet.
func (d Dialog) surfaceStyle(style core.Style) core.Style {
	style = style.Merge(d.Style)
	if d.StyleFunc != nil {
		style = style.Merge(d.StyleFunc())
	}
	return style
}

// RadiusDialog rounds a dialog on desktops.
const RadiusDialog core.Points = 16

// dialogShape is a desktop dialog's corners and rim, before the app's Style.
var dialogShape = core.Style{CornerRadius: RadiusDialog, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1}

// bareClip rounds a bare dialog's clip to the inside of the surface's rim, style being the surface's: each corner's radius less the wider of the borders it joins, curved as the surface's.
func bareClip(style core.Style) core.Style {
	var rim core.Edges
	switch {
	case style.BorderStyle == core.BorderNone || style.BorderStyle == core.BorderHidden:
	case style.Borders != (core.Edges{}):
		rim = style.Borders
	default:
		rim = core.All(style.BorderWidth)
	}
	r := style.CornerRadii
	if r == (core.Corners{}) {
		r = core.Corners{TopLeft: style.CornerRadius, TopRight: style.CornerRadius, BottomRight: style.CornerRadius, BottomLeft: style.CornerRadius}
	}
	inside := func(radius, a, b core.Points) core.Points { return max(0, radius-max(a, b)) }
	r = core.Corners{TopLeft: inside(r.TopLeft, rim.Top, rim.Left), TopRight: inside(r.TopRight, rim.Top, rim.Right),
		BottomRight: inside(r.BottomRight, rim.Bottom, rim.Right), BottomLeft: inside(r.BottomLeft, rim.Bottom, rim.Left)}
	clip := core.Style{BorderCurve: style.BorderCurve}
	if r.Uniform() {
		clip.CornerRadius = r.TopLeft
	} else {
		clip.CornerRadii, clip.PhysicalBorders.Corners = r, style.PhysicalBorders.Corners
	}
	return clip
}

// A dialog's layers move so: the dim fades, and the surface pops
// in and fades out where it is. Under reduced motion they come and go at
// once.
var (
	dialogFade = core.Transition{Duration: 180 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionOpacity}
	dialogPop  = core.Transition{Duration: 180 * time.Millisecond, Easing: core.EaseOut,
		Properties: core.TransitionOpacity | core.TransitionTransform}
	dialogStarting = core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.96, TranslateY: 8}}
	dialogEnding   = core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.98}}
)

// dialogInset is how far a dialog stays from the window's edges: inset, 20
// when 0, at least the title bar the app draws in.
func dialogInset(cx *core.Scope, inset core.Points) core.Points {
	if inset <= 0 {
		inset = 20
	}
	return max(inset, desktop.TitleBar(cx).Top)
}

// dialogDragStrip keeps the title bar the app draws in moving the window
// under a dialog's backdrop.
func dialogDragStrip(cx *core.Scope) core.Node {
	return core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn}, StyleFunc: func() core.Style {
		return core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(desktop.TitleBar(cx).Top)}
	}}
}
