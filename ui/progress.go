package ui

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Progress is a bar of how far a task is, Value from 0 to 100, or with
// Indeterminate one under way, a segment sweeping across it. Axis and
// Inverted are base.Progress's: a vertical bar fills from the bottom, or
// inverted from the top, and takes its height from Style.
type Progress struct {
	Value         func() float32
	Indeterminate func() bool
	Axis          base.Axis
	Inverted      bool
	// Label names the task for assistive technology.
	Label     string
	LabelFunc func() string
	TestID    string
	Style     core.Style
}

// Render implements core.Node.
func (p Progress) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	const height core.Points = 6
	vertical := p.Axis == base.Vertical
	thick := core.Style{Height: core.Pt(height)}
	if vertical {
		thick = core.Style{Width: core.Pt(height), FlexGrow: 1}
	}
	bar := func() core.Style {
		s := core.Style{CornerRadius: RadiusFull, Background: colors.AccentFill()}
		if vertical {
			s.Width = core.Pt(height)
		} else {
			s.Height = core.Pt(height)
		}
		return s
	}
	env := cx.Env()
	// sweep moves the segment from the end the bar fills from to the other.
	sweepTo := func(s *core.Style) {
		switch {
		case vertical && p.Inverted:
			s.Transform.TranslateYPercent = 200
		case vertical:
			s.Transform.TranslateYPercent = -200
		case env.RTL() != p.Inverted:
			s.Transform.TranslateXPercent = -200
		default:
			s.Transform.TranslateXPercent = 200
		}
	}
	still := ReduceMotion(cx)
	var sweep *core.Signal[bool]
	if p.Indeterminate != nil && !still {
		sweep = core.NewSignal(cx, false)
		const period = 900 * time.Millisecond
		var tick func()
		tick = func() {
			if is(p.Indeterminate) {
				sweep.Set(!sweep.GetUntracked())
			}
			cx.AfterFunc(period, tick)
		}
		cx.AfterFunc(0, tick)
	}
	return base.Progress{Value: p.Value, Indeterminate: p.Indeterminate, Axis: p.Axis, Inverted: p.Inverted, Label: p.Label, LabelFunc: p.LabelFunc, TestID: p.TestID, Style: p.Style,
		TrackStyle:     thick.Merge(core.Style{CornerRadius: RadiusFull, BorderWidth: 0, Overflow: core.Hidden}),
		TrackStyleFunc: Filled(colors.Inset),
		IndicatorStyleFunc: func() core.Style {
			if !is(p.Indeterminate) {
				return bar().Merge(core.Style{Transition: core.Transition{Duration: 200 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionLayout}})
			}
			if still {
				full := core.Style{Width: core.Pct(100)}
				if vertical {
					full = core.Style{Height: core.Pct(100)}
				}
				return bar().Merge(full).Merge(core.Style{Animation: core.KeyframeAnimation{Keyframes: core.KeyframesPulse}})
			}
			third := core.Style{Width: core.Pct(100.0 / 3)}
			if vertical {
				third = core.Style{Height: core.Pct(100.0 / 3)}
			}
			s := bar().Merge(third).Merge(core.Style{
				Transition: core.Transition{Duration: 900 * time.Millisecond, Easing: core.EaseInOut, Properties: core.TransitionTransform}})
			if sweep.Get() {
				sweepTo(&s)
			}
			return s
		}}
}
