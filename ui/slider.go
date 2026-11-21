package ui

import (
	"math"
	"strconv"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Slider's geometry: a 4-point track under a white capsule knob 20 × 16,
// which grows by a third while held.
const (
	sliderTrack core.Points = 4
	sliderKnobW core.Points = 20
	sliderKnobH core.Points = 16
	// sliderTicks is the most ticks a stepped slider shows.
	sliderTicks = 60
)

// Slider is base.Slider in ui's look: a white capsule knob on a thin
// track that stays inside its ends, or with High two that pick a range
// from Value up to High. Step above 0 snaps the values to steps from Min
// and marks them under the track, up to 60 of them; at 0 the keys move a
// hundredth of the range.
type Slider struct {
	// Axis and Inverted are base.Slider's: a vertical slider has min at
	// the bottom, or inverted at the top, and takes its height from Style.
	Axis     base.Axis
	Inverted bool
	// ThumbRef and StartThumbRef are the end and start thumbs' refs.
	ThumbRef, StartThumbRef *core.Ref
	LabelledBy              []*core.Ref
	Invalid                 func() bool
	Value                   func() float64
	OnChange                func(value float64)
	// High and OnHighChange make it a range, Value its low end.
	High         func() float64
	OnHighChange func(value float64)
	// Min and Max are the range, 0 to 100 when Max is not above Min.
	Min, Max float64
	Step     float64
	// MinFunc, MaxFunc and StepFunc follow changing bounds and steps in place, taking precedence over Min, Max and Step. Explicit bounds may be equal, for a range whose maximum is not known yet.
	MinFunc, MaxFunc, StepFunc func() float64
	Disabled                   func() bool
	// Label names it for assistive technology; a range's knobs add
	// LowLabel and HighLabel after it, such as "minimum" and "maximum".
	Label                       string
	LabelFunc                   func() string
	LowLabel, HighLabel         string
	LowLabelFunc, HighLabelFunc func() string
	// ValueText is what assistive technology reads as a value, the
	// number when nil.
	ValueText func(value float64) string
	// TestID names the slider; its knobs add "-thumb", or "-low" and
	// "-high" in a range.
	TestID string
	Style  core.Style
}

// bounds are the slider's range.
func (s Slider) bounds() (lo, hi float64) {
	lo, hi = s.Min, s.Max
	if s.MaxFunc == nil && !(hi > lo) {
		lo, hi = 0, 100
	}
	if s.MinFunc != nil {
		lo = s.MinFunc()
	}
	if s.MaxFunc != nil {
		hi = s.MaxFunc()
	}
	return lo, hi
}

func (s Slider) step() float64 {
	if s.StepFunc != nil {
		return s.StepFunc()
	}
	return s.Step
}

// Render implements core.Node.
func (s Slider) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	ranged := s.High != nil
	read := func(fn func() float64) float64 {
		if fn == nil {
			lo, _ := s.bounds()
			return lo
		}
		return fn()
	}
	value := func() base.SliderValue {
		if ranged {
			return base.RangeValue(read(s.Value), read(s.High))
		}
		return base.SingleValue(read(s.Value))
	}
	change := func(v base.SliderValue) {
		now := core.Untracked(cx, value)
		switch {
		case !ranged && s.OnChange != nil:
			s.OnChange(v.End)
		case ranged && v.Start != now.Start && s.OnChange != nil:
			s.OnChange(v.Start)
		case ranged && v.End != now.End && s.OnHighChange != nil:
			s.OnHighChange(v.End)
		}
	}
	const height = sliderKnobH + 4
	vertical := s.Axis == base.Vertical
	sizeStyle := func() core.Style {
		tall := core.Points(height)
		if step := s.step(); step > 0 && !math.IsInf(step, 0) {
			tall += 8
		}
		if vertical {
			return core.Style{Width: core.Pt(tall)}.Merge(s.Style)
		}
		return core.Style{Height: core.Pt(tall)}.Merge(s.Style)
	}
	marks := core.Dynamic(func(*core.Scope) core.Node {
		lo, hi := s.bounds()
		step := s.step()
		var marks []core.Node
		// Check before converting to int: non-finite bounds and tiny steps must not overflow the number of marks.
		n := math.Floor((hi - lo) / step)
		if step > 0 && !math.IsInf(step, 0) && hi > lo && n >= 0 && n <= sliderTicks {
			count := int(n)
			for i := 0; i <= count; i++ {
				f := float32(float64(i) * step / (hi - lo))
				mark := core.Style{Position: core.Absolute, Width: core.Pt(2), Height: core.Pt(2), CornerRadius: 1}
				if vertical {
					if !s.Inverted {
						f = 1 - f
					}
					mark.Left, mark.Top, mark.Margin = core.Pt(sliderTrack/2+10), core.Pct(f*100), core.Edges{Top: -1}
				} else {
					if s.Inverted {
						f = 1 - f
					}
					mark.Top, mark.Left, mark.Margin = core.Pt(sliderTrack/2+10), core.Pct(f*100), core.Edges{Left: -1}
				}
				marks = append(marks, core.View{PointerEvents: core.PointerEventsNone, Style: mark, StyleFunc: Filled(colors.Track)})
			}
		}
		return core.Group(marks)
	})
	label, labelFunc := s.Label, s.LabelFunc
	if labelFunc != nil {
		label = ""
	}
	var startLabel string
	var startLabelFunc func() string
	thumb, startThumb := "", ""
	if ranged {
		label, labelFunc = sliderThumbLabel(s.Label, s.LabelFunc, s.HighLabel, s.HighLabelFunc)
		startLabel, startLabelFunc = sliderThumbLabel(s.Label, s.LabelFunc, s.LowLabel, s.LowLabelFunc)
	}
	if s.TestID != "" {
		thumb = s.TestID + "-thumb"
		if ranged {
			thumb, startThumb = s.TestID+"-high", s.TestID+"-low"
		}
	}
	valueText := s.ValueText
	if valueText == nil {
		valueText = func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	}
	// A vertical slider turns the track and the knob a quarter.
	box := core.Style{MinWidth: core.Pt(80)}
	track := core.Style{Height: core.Pct(100)}
	rail := core.Style{Height: core.Pt(sliderTrack), Margin: core.Edges{Top: (height - sliderTrack) / 2, Left: sliderKnobW / 2, Right: sliderKnobW / 2}}
	knobW, knobH := sliderKnobW, sliderKnobH
	if vertical {
		box = core.Style{MinHeight: core.Pt(80)}
		track = core.Style{FlexGrow: 1, Direction: core.Row}
		rail = core.Style{Width: core.Pt(sliderTrack), Margin: core.Edges{Left: (height - sliderTrack) / 2, Top: sliderKnobW / 2, Bottom: sliderKnobW / 2}}
		knobW, knobH = sliderKnobH, sliderKnobW
	}
	rail.CornerRadius = sliderTrack / 2
	return base.Slider{Value: value, OnChange: change, Axis: s.Axis, Inverted: s.Inverted,
		Min: func() float64 { lo, _ := s.bounds(); return lo }, Max: func() float64 { _, hi := s.bounds(); return hi }, Step: s.step, Disabled: s.Disabled,
		ThumbRef: s.ThumbRef, StartThumbRef: s.StartThumbRef, LabelledBy: s.LabelledBy, Invalid: s.Invalid, Label: label, LabelFunc: labelFunc, StartLabel: startLabel, StartLabelFunc: startLabelFunc, ValueText: valueText, TestID: s.TestID, ThumbTestID: thumb, StartThumbTestID: startThumb, Marks: []core.Node{marks},
		Style:              box.Merge(core.Style{Disabled: Dimmed()}).Merge(s.Style),
		StyleFunc:          sizeStyle,
		TrackStyle:         track,
		IndicatorStyle:     rail,
		IndicatorStyleFunc: Filled(colors.Line),
		RangeStyle:         core.Style{CornerRadius: sliderTrack / 2},
		RangeStyleFunc:     Filled(colors.AccentFill),
		ThumbStyle: core.Style{Width: core.Pt(knobW), Height: core.Pt(knobH), CornerRadius: sliderKnobH / 2,
			Transition: core.Transition{Duration: quick.Duration, Easing: core.EaseOut, Properties: core.TransitionTransform}},
		ThumbStyleFunc: func() core.Style {
			return FinOf(cx).Knob().Over(core.Style{Background: colors.Knob(), BorderWidth: 1, BorderColor: colors.Border(), FocusVisible: FocusRing(cx, 2),
				Active: &core.Style{Transform: core.Transform{Scale: 1.3, TranslateXPercent: -50, TranslateYPercent: -50}, Shadows: []core.BoxShadow{Shadow(0, 6, 14, -3, Black(0.18))}}})
		}}
}

// sliderThumbLabel combines a range's name and each thumb's suffix.
func sliderThumbLabel(label string, readLabel func() string, suffix string, readSuffix func() string) (string, func() string) {
	if readLabel == nil && readSuffix == nil {
		return label + " " + suffix, nil
	}
	return "", func() string {
		return textValue(label, readLabel) + " " + textValue(suffix, readSuffix)
	}
}
