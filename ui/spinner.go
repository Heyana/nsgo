package ui

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// SpinnerVariant is a Spinner's look.
type SpinnerVariant uint8

const (
	// SpinnerRing is an indeterminate ProgressRing. It is the default.
	SpinnerRing SpinnerVariant = iota
	// SpinnerSpokes is twelve fading spokes that step around in the muted text's color, or Tint.
	SpinnerSpokes
)

// Spinner is work of unknown length. Label names it for assistive
// technology; Size is its diameter, 32 pt when 0. Under reduced motion
// it stands still.
type Spinner struct {
	Variant SpinnerVariant
	Size    core.Points
	// LineWidth is the ring's track and arc width. When 0, ProgressRing's default is used. It has no effect on SpinnerSpokes.
	LineWidth core.Points
	// Tint colors it, as ProgressRing's: the ring the accent when nil, the spokes the muted text.
	Tint  func() core.Color
	Label string
	// LabelFunc binds a changing Label.
	LabelFunc func() string
	TestID    string
	Style     core.Style
}

// Render implements core.Node.
func (s Spinner) Render(cx *core.Scope) core.Node {
	if s.Variant != SpinnerSpokes {
		return ProgressRing{Indeterminate: func() bool { return true }, Size: s.Size, LineWidth: s.LineWidth, Tint: s.Tint, Label: s.Label, LabelFunc: s.LabelFunc, TestID: s.TestID, Style: s.Style}
	}
	size := s.Size
	if size <= 0 {
		size = ringSize
	}
	turn := core.NewAnimatedValue(cx, 0)
	if !ReduceMotion(cx) {
		// A spoke's turn at a time, as the spokes fade along.
		cx.StartAnimation(core.ValueKeyframes(turn, core.KeyframeTiming{Duration: 900 * time.Millisecond, Iterations: core.Forever,
			Easing: core.StepsEasing(12, core.JumpEnd)}, core.Stops(0, 360)...), nil)
	}
	tint := s.Tint
	if tint == nil {
		tint = ColorsOf(cx).Muted
	}
	draw := func(d *core.Drawing, sz core.Size) {
		dia := float32(min(sz.Width, sz.Height))
		if dia <= 0 {
			return
		}
		color := tint()
		scale := dia / 100
		cx, cy := float32(sz.Width)/2, float32(sz.Height)/2
		r0, r1 := 24*scale, 44*scale
		d.SetLineWidth(9 * scale).SetLineCap(core.CapRound)
		// Twelve spokes fading behind the one at twelve o'clock, as MyGo's indicator draws them.
		for i := range 12 {
			age := (12 - i) % 12
			angle := float64(i) * math.Pi / 6
			sin, cos := float32(math.Sin(angle)), float32(math.Cos(angle))
			opacity := 1 - float32(age)/12*0.85
			d.BeginPath().MoveTo(cx+r0*sin, cy-r0*cos).LineTo(cx+r1*sin, cy-r1*cos).
				SetStrokeColor(Alpha(color, opacity)).Stroke()
		}
	}
	return base.Progress{TestID: s.TestID, Label: s.Label, LabelFunc: s.LabelFunc, Indeterminate: func() bool { return true }, TrackStyle: core.Style{Display: core.DisplayNone},
		Style:     core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0}.Merge(s.Style),
		StyleFunc: func() core.Style { return core.Style{Animated: &core.AnimatedStyle{Rotate: turn}} },
		Children:  []core.Node{core.Canvas{Draw: draw, Style: core.Style{FlexGrow: 1}}}}
}
