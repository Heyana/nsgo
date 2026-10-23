package base

import (
	"math"

	"github.com/oneo-me/nsgo/core"
)

// Slider picks a number in a range by dragging along its track, with no
// look of its own. Its parts are a track holding the fill, which spans the
// track up to the value, and the thumb, centered on the value. The app
// owns the value: a drag reports each value it passes to OnChange, snapped
// to Step, and the slider shows what Value returns.
//
// A drag anywhere on the slider moves the value by the distance dragged,
// so a narrow thumb need not be hit. Assistive technology sees an
// adjustable element with Label and the value as a range, and its
// increment and decrement actions move the value by Step, or a tenth of
// the range when continuous.
type Slider struct {
	Value func() float64
	// Min and Max bound the value; 0 to 1 when Max is not above Min.
	Min, Max float64
	// Step snaps the value to multiples of itself from Min; 0 is
	// continuous.
	Step     float64
	Disabled func() bool
	OnChange func(value float64)

	// Each StyleFunc's result merges over its style. The track lays out the
	// fill and the thumb absolutely, so give it a height.
	Style          core.Style
	StyleFunc      func() core.Style
	TrackStyle     core.Style
	TrackStyleFunc func() core.Style
	FillStyle      core.Style
	FillStyleFunc  func() core.Style
	ThumbStyle     core.Style
	ThumbStyleFunc func() core.Style
	Label          string
	TestID         string
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
	// fraction is how far along the track the value is.
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
	return core.View{
		Style: s.Style, StyleFunc: s.StyleFunc, Label: s.Label, TestID: s.TestID, Disabled: s.Disabled,
		Role: core.RoleAdjustable,
		AccessibilityValue: func() core.AccessibilityValue {
			return core.AccessibilityValue{Min: float32(lo), Max: float32(hi), Now: float32(value())}
		},
		AccessibilityActions:  sliderActions,
		OnAccessibilityAction: adjust,
		OnPan:                 pan, PanActivation: core.PanActivation{ActiveOffsetX: 2, FailOffsetY: 12},
		Children: []core.Node{core.View{
			Style: s.TrackStyle, StyleFunc: s.TrackStyleFunc, Disabled: s.Disabled,
			OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
			Children: []core.Node{
				core.View{Style: s.FillStyle, Disabled: s.Disabled, StyleFunc: func() core.Style {
					fill := core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0),
						Width: core.Pct(fraction() * 100)}
					if s.FillStyleFunc != nil {
						fill = fill.Merge(s.FillStyleFunc())
					}
					return fill
				}},
				// A zero-width column at the value centers the thumb on it.
				core.View{StyleFunc: func() core.Style {
					return core.Style{Position: core.Absolute, Left: core.Pct(fraction() * 100), Top: core.Pt(0), Bottom: core.Pt(0),
						Width: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
				}, Children: []core.Node{core.View{Style: s.ThumbStyle, StyleFunc: s.ThumbStyleFunc, Disabled: s.Disabled}}},
			},
		}},
	}
}

// sliderActions are what assistive technology adjusts a slider with.
var sliderActions = []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}}
