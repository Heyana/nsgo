package base

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

// SplitDirection is the axis a Split's panes are laid out along.
type SplitDirection uint8

const (
	// SplitHorizontal puts the first pane beside the second and its
	// divider between them, dragged left and right.
	SplitHorizontal SplitDirection = iota
	// SplitVertical stacks the first pane above the second and puts its
	// divider between them, dragged up and down.
	SplitVertical
)

const (
	// DefaultSplitThickness is the divider's band across the split: the
	// room a drag can start in. The line drawn inside it is the app's.
	DefaultSplitThickness core.Points = 9
	// splitStep is how far assistive technology moves the divider at a
	// time.
	splitStep core.Points = 10
	// splitDoubleClick is how soon a second press counts as a double
	// click on the divider.
	splitDoubleClick = 400 * time.Millisecond
)

// Split is two panes sharing its length along Direction, with a divider
// whose drag resizes the first pane. It takes the panes, their limits and
// every style from the app.
//
// DefaultSize is the first pane's initial size when Size is not set, and
// what a double click restores. A core.Pct makes it a percentage that
// follows the split's length; a core.Pt makes it fixed. The zero Length
// means half in percent mode. Min and Max bound the first pane the same
// way; without a Max it is unbounded and without a Min it can close. Sizes
// a drag or an accessibility action would take past a limit stop at it.
//
// Size makes the first pane controlled: the app returns its size, as a
// percentage to follow the split or in points to stay fixed, and follows
// OnResize to move it. Without Size, the split keeps the size the user
// gives it while it lives.
//
// Places in the divider's band accept a drag and a double click, and
// assistive technology moves it by 10 points with its increment and
// decrement actions. The band asks for the horizontal or vertical resize
// cursor, and DividerStyle may ask for another. The line in the band is
// LineStyle, one point thick by default and invisible without a color.
type Split struct {
	Direction SplitDirection
	First     core.Node
	Second    core.Node

	Size        func() core.Length
	DefaultSize core.Length
	Min, Max    core.Length
	// OnResize receives the first pane's size after each drag move, a
	// double click and an accessibility action, in the unit Size returns
	// when controlled, else in DefaultSize's.
	OnResize func(size core.Length)
	Disabled func() bool

	// Thickness is the divider band's width across the split; 9 when 0.
	Thickness core.Points

	Style            core.Style
	StyleFunc        func() core.Style
	DividerStyle     core.Style
	DividerStyleFunc func() core.Style
	LineStyle        core.Style
	LineStyleFunc    func() core.Style

	// Label names the divider for assistive technology; Hint says what
	// dragging it does. TestID names the split, and the divider
	// TestID + "-divider".
	Label  string
	Hint   string
	TestID string
}

// Render implements core.Node.
func (s Split) Render(cx *core.Scope) core.Node {
	horizontal := s.Direction != SplitVertical
	thickness := s.Thickness
	if thickness <= 0 {
		thickness = DefaultSplitThickness
	}
	if thickness < 1 {
		thickness = 1
	}
	disabled := func() bool { return s.Disabled != nil && s.Disabled() }

	defaultPercent, defaultPoints, percentMode := splitDefault(s.DefaultSize)
	fraction := core.NewSignal(cx, defaultPercent/100)
	fixed := core.NewSignal(cx, defaultPoints)
	length := core.NewSignal(cx, core.Points(0))

	resolve := func(l core.Length, base, otherwise core.Points) core.Points {
		if p, ok := l.Points(); ok {
			return p
		}
		if percent, ok := l.Percent(); ok {
			return base * core.Points(percent) / 100
		}
		return otherwise
	}
	minimum := func(base core.Points) core.Points {
		return max(resolve(s.Min, base, 0), 0)
	}
	maximum := func(base core.Points) core.Points {
		if s.Max.IsAuto() {
			return max(base, 0)
		}
		hi := resolve(s.Max, base, 0)
		if hi < 0 {
			hi = 0
		}
		return min(hi, max(base, 0))
	}
	clampSize := func(size, base core.Points) core.Points {
		lo, hi := minimum(base), maximum(base)
		if hi < lo {
			lo = hi
		}
		return min(max(size, lo), hi)
	}
	// current reads what the panes are laid out at: the app's size when
	// controlled, else the one a drag or the default gave.
	current := func() core.Points {
		base := length.Get()
		if s.Size != nil {
			size := resolve(s.Size(), base, 0)
			return min(max(size, 0), max(base, 0))
		}
		if base <= 0 {
			return 0
		}
		var size core.Points
		if percentMode {
			size = base * core.Points(fraction.Get())
		} else {
			size = fixed.Get()
		}
		return clampSize(size, base)
	}
	// report tells the app a size the user chose, in the unit it reads.
	report := func(size, base core.Points) {
		if s.OnResize == nil {
			return
		}
		percent := percentMode
		if s.Size != nil {
			l := core.Untracked(cx, s.Size)
			if _, ok := l.Percent(); ok {
				percent = true
			} else if _, ok := l.Points(); ok {
				percent = false
			}
		}
		if percent && base > 0 {
			s.OnResize(core.Pct(float32(size/base) * 100))
			return
		}
		s.OnResize(core.Pt(size))
	}
	// apply gives the first pane fullSize, within its limits and the
	// split, and follows the app if it is controlled.
	apply := func(fullSize core.Points) {
		base := length.GetUntracked()
		if base <= 0 {
			return
		}
		size, before := clampSize(fullSize, base), core.Untracked(cx, current)
		if s.Size == nil {
			if percentMode {
				fraction.Set(float32(size / base))
			} else {
				fixed.Set(size)
			}
		}
		if size != before {
			report(size, base)
		}
	}
	reset := func() {
		base := length.GetUntracked()
		if base <= 0 {
			return
		}
		if s.Size == nil {
			if percentMode {
				fraction.Set(defaultPercent / 100)
			} else {
				fixed.Set(defaultPoints)
			}
		}
		// The configured default is restored even where the current room
		// clamps what shows.
		if percentMode {
			report(base*core.Points(defaultPercent)/100, base)
		} else {
			report(defaultPoints, base)
		}
	}

	var start core.Points
	pan := func(e core.PanEvent) {
		if disabled() {
			return
		}
		switch e.Phase {
		case core.GestureBegan:
			start = core.Untracked(cx, current)
		case core.GestureEnded, core.GestureCancelled:
			return
		}
		d := e.Translation.X
		if !horizontal {
			d = e.Translation.Y
		} else if cx.Env().RTL() {
			d = -d
		}
		apply(start + d)
	}
	adjust := func(name string) {
		if disabled() {
			return
		}
		step := splitStep
		switch name {
		case core.ActionIncrement:
		case core.ActionDecrement:
			step = -step
		default:
			return
		}
		apply(core.Untracked(cx, current) + step)
	}

	var clicked bool
	var stopClick func() bool
	press := func(core.PressEvent) {
		if disabled() {
			return
		}
		if clicked {
			clicked = false
			if stopClick != nil {
				stopClick()
			}
			reset()
			return
		}
		clicked = true
		stopClick = cx.AfterFunc(splitDoubleClick, func() { clicked = false })
	}

	firstStyle := func() core.Style {
		size := current()
		if horizontal {
			return core.Style{Width: core.Pt(size), FlexShrink: 0}
		}
		return core.Style{Height: core.Pt(size), FlexShrink: 0}
	}
	secondStyle := core.Style{FlexGrow: 1, FlexShrink: 1}
	if horizontal {
		secondStyle.MinWidth = core.Pt(0)
	} else {
		secondStyle.MinHeight = core.Pt(0)
	}
	dividerStyle := func() core.Style {
		size := current()
		var style core.Style
		if horizontal {
			style = core.Style{Position: core.Absolute, Left: core.Pt(size - thickness/2),
				Top: core.Pt(0), Bottom: core.Pt(0), Width: core.Pt(thickness), Cursor: core.CursorColResize}
		} else {
			style = core.Style{Position: core.Absolute, Top: core.Pt(size - thickness/2),
				Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(thickness), Cursor: core.CursorRowResize}
		}
		style = style.Merge(s.DividerStyle)
		if s.DividerStyleFunc != nil {
			style = style.Merge(s.DividerStyleFunc())
		}
		return style
	}
	lineStyle := func() core.Style {
		style := core.Style{Position: core.Absolute}
		if horizontal {
			style.Left = core.Pt((thickness - 1) / 2)
			style.Top, style.Bottom = core.Pt(0), core.Pt(0)
			style.Width = core.Pt(1)
		} else {
			style.Top = core.Pt((thickness - 1) / 2)
			style.Left, style.Right = core.Pt(0), core.Pt(0)
			style.Height = core.Pt(1)
		}
		style = style.Merge(s.LineStyle)
		if s.LineStyleFunc != nil {
			style = style.Merge(s.LineStyleFunc())
		}
		return style
	}

	activation := core.PanActivation{ActiveOffsetX: 2, FailOffsetY: 12}
	if !horizontal {
		activation = core.PanActivation{ActiveOffsetY: 2, FailOffsetX: 12}
	}
	dividerTestID := ""
	if s.TestID != "" {
		dividerTestID = s.TestID + "-divider"
	}
	divider := core.View{
		StyleFunc:             dividerStyle,
		Disabled:              s.Disabled,
		OnPress:               press,
		OnPan:                 pan,
		PanActivation:         activation,
		Role:                  core.RoleAdjustable,
		Label:                 s.Label,
		Hint:                  s.Hint,
		TestID:                dividerTestID,
		AccessibilityActions:  splitActions,
		OnAccessibilityAction: adjust,
		AccessibilityValue: func() core.AccessibilityValue {
			base := length.Get()
			return core.AccessibilityValue{Min: float32(minimum(base)), Max: float32(maximum(base)),
				Now: float32(current())}
		},
		Children: []core.Node{core.View{StyleFunc: lineStyle}},
	}

	style := func() core.Style {
		out := core.Style{Direction: core.Row}
		if !horizontal {
			out.Direction = core.Column
		}
		if s.StyleFunc != nil {
			out = out.Merge(s.StyleFunc())
		}
		return out
	}
	return core.View{
		Style:     s.Style,
		StyleFunc: style,
		TestID:    s.TestID,
		OnLayout: func(b core.Bounds) {
			size := b.Size.Width
			if !horizontal {
				size = b.Size.Height
			}
			if size != length.GetUntracked() {
				length.Set(size)
			}
		},
		Children: []core.Node{
			core.View{StyleFunc: firstStyle, Children: []core.Node{s.First}},
			core.View{Style: secondStyle, Children: []core.Node{s.Second}},
			divider,
		},
	}
}

// splitDefault reads DefaultSize into its percentage and its points, and
// whether it is a percentage.
func splitDefault(l core.Length) (percent float32, points core.Points, isPercent bool) {
	if p, ok := l.Points(); ok {
		return 0, max(p, 0), false
	}
	if pct, ok := l.Percent(); ok {
		return min(max(pct, 0), 100), 0, true
	}
	return 50, 0, true
}

// splitActions are what assistive technology moves a divider with.
var splitActions = []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}}
