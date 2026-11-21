package ui

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// TestSlider moves a knob with a press on the track, a drag, the keys on
// desktops and assistive technology, snapped to its steps; a range's
// knobs never cross.
func TestSlider(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var value, low, high *core.Signal[float64]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			value, low, high = core.NewSignal(cx, 20.0), core.NewSignal(cx, 20.0), core.NewSignal(cx, 60.0)
			return core.View{Style: core.Style{Gap: 16, Width: core.Pt(220)}, Children: []core.Node{
				Slider{TestID: "volume", Label: "Volume", Value: value.Get, OnChange: value.Set, Step: 5},
				Slider{TestID: "price", Label: "Price", LowLabel: "minimum", HighLabel: "maximum",
					Value: low.Get, OnChange: low.Set, High: high.Get, OnHighChange: high.Set},
			}}
		})
		app.Touch(viewOf(app.Find("volume").Children[0]), core.Point{X: 110, Y: 10}).Lift()
		if value.Get() != 50 {
			t.Fatalf("a press in the middle gave %v, want 50", value.Get())
		}
		app.Drag("volume-thumb", core.Point{X: 40}, core.Point{X: 51})
		if v := value.Get(); v < 70 || v > 80 || v != float64(int(v/5)*5) {
			t.Fatalf("a drag of 51 pt across 200 gave %v, want 75 in steps of 5", v)
		}
		app.AccessibilityAction("volume-thumb", core.ActionDecrement)
		if v := app.Find("label:Volume"); value.Get() != 70 || v.Props.Role != core.RoleAdjustable {
			t.Fatalf("value = %v, role %v; want 70 from an adjustable knob", value.Get(), v.Props.Role)
		}
		if e.Idiom == core.IdiomDesktop {
			app.Key("tab")
			app.Key("end")
			app.Key("left")
			if value.Get() != 95 {
				t.Fatalf("End then Left gave %v, want 95", value.Get())
			}
		}
		// The middle is nearer the high knob, which takes the press.
		app.Touch(viewOf(app.Find("price").Children[0]), core.Point{X: 110, Y: 10}).Lift()
		if low.Get() != 20 || high.Get() != 50 {
			t.Fatalf("range = %v–%v, want 20–50", low.Get(), high.Get())
		}
		for range 40 {
			app.AccessibilityAction("label:Price minimum", core.ActionIncrement)
		}
		if low.Get() != 50 || app.Find("price-low").Props.Value.Max != 50 {
			t.Fatalf("the low knob reached %v, want it stopped at the high one's 50", low.Get())
		}
	})
}

// A duration can arrive while the slider is focused or being scrubbed. Updating it must keep the thumb and its responder mounted.
func TestSliderReactiveBoundsPreserveFocusAndDrag(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var value, maximum *core.Signal[float64]
		var label *core.Signal[string]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			value, maximum = core.NewSignal(cx, 60.0), core.NewSignal(cx, 0.0)
			label = core.NewSignal(cx, "Position")
			return Slider{TestID: "position", LabelFunc: label.Get, Value: value.Get, OnChange: value.Set,
				Max: 1000, MaxFunc: maximum.Get, Style: core.Style{Width: core.Pt(220)}}
		})
		control, thumb := app.Find("position").ID, app.Find("position-thumb").ID
		if v := app.Find("position-thumb").Props.Value; v.Max != 0 || v.Now != 0 || value.Get() != 60 {
			t.Fatalf("unknown duration: accessibility %+v, app value %v", v, value.Get())
		}
		if e.Idiom == core.IdiomDesktop {
			app.Focus("position-thumb")
		}
		app.Post(func() { maximum.Set(300) })
		app.Settle()
		if v := app.Find("position-thumb"); v.ID != thumb || app.Find("position").ID != control || v.Props.Value.Max != 300 || v.Props.Value.Now != 60 || value.Get() != 60 {
			t.Fatalf("duration replaced the control or its value: thumb %+v, app value %v", v.Props.Value, value.Get())
		}
		if e.Idiom == core.IdiomDesktop && !app.Find("position-thumb").Focused {
			t.Fatal("duration update lost the keyboard focus")
		}
		finger := app.Touch("position-thumb", core.Point{X: 10, Y: 8})
		finger.Move(core.Point{X: 30, Y: 8})
		before := value.Get()
		if before <= 60 {
			t.Fatalf("scrub did not start: %v", before)
		}
		app.Post(func() { maximum.Set(600); label.Set("Playback position") })
		app.Settle()
		if v := app.Find("position-thumb"); v.ID != thumb || v.Props.Label != "Playback position" || v.Props.Value.Max != 600 || value.Get() != before {
			t.Fatalf("bounds changed during scrub: %+v, app value %v (was %v)", v.Props.Value, value.Get(), before)
		}
		finger.Move(core.Point{X: 50, Y: 8})
		if value.Get() <= before || value.Get() > 600 {
			t.Fatalf("active scrub did not follow the new range: %v (was %v)", value.Get(), before)
		}
		finger.Lift()
		if v := app.Find("position-thumb"); v.ID != thumb || v.Props.Value.Now != float32(value.Get()) || v.Props.Value.Max != 600 {
			t.Fatalf("release lost the scrub's value: %+v, app value %v", v.Props.Value, value.Get())
		}
	})
}

func TestSliderReactiveStepsAndMinimum(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var value, minimum, maximum, step *core.Signal[float64]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			value, minimum = core.NewSignal(cx, 20.0), core.NewSignal(cx, 10.0)
			maximum, step = core.NewSignal(cx, 30.0), core.NewSignal(cx, 10.0)
			slider := Slider{TestID: "level", Value: value.Get, OnChange: value.Set,
				Min: -100, Max: 100, Step: 1, MinFunc: minimum.Get, MaxFunc: maximum.Get, StepFunc: step.Get,
				Style: core.Style{Width: core.Pt(220)}}
			fixed := slider
			fixed.TestID, fixed.Style.Height = "fixed", core.Pt(40)
			return core.Group([]core.Node{slider, fixed, Slider{TestID: "minimum-only", MinFunc: minimum.Get}})
		})
		thumb := app.Find("level-thumb").ID
		checkMarks := func(want int, height core.Points) {
			t.Helper()
			n := 0
			for _, v := range views(app.Find("level")) {
				if v.Frame.Size.Width == 2 && v.Frame.Size.Height == 2 {
					n++
				}
			}
			if n != want || app.Find("level").Frame.Size.Height != height || app.Find("fixed").Frame.Size.Height != 40 {
				t.Fatalf("ticks = %d, height = %v, custom height = %v; want %d, %v, 40", n, app.Find("level").Frame.Size.Height, app.Find("fixed").Frame.Size.Height, want, height)
			}
		}
		checkMarks(3, 28)
		app.Post(func() { minimum.Set(20); maximum.Set(80); step.Set(20) })
		app.Settle()
		checkMarks(4, 28)
		if v := app.Find("minimum-only-thumb").Props.Value; v.Min != 20 || v.Max != 100 {
			t.Fatalf("MinFunc lost the default maximum: %+v", v)
		}
		if v := app.Find("level-thumb"); v.ID != thumb || v.Props.Value.Min != 20 || v.Props.Value.Max != 80 {
			t.Fatalf("range not updated in place: %+v", v.Props.Value)
		}
		app.AccessibilityAction("level-thumb", core.ActionIncrement)
		if value.Get() != 40 {
			t.Fatalf("increment ignored the new step: %v", value.Get())
		}
		if e.Idiom == core.IdiomDesktop {
			app.Focus("level-thumb")
			app.Key("right")
			if value.Get() != 60 {
				t.Fatalf("Right ignored the new step: %v", value.Get())
			}
			app.Key("end")
			if value.Get() != 80 {
				t.Fatalf("End ignored the new maximum: %v", value.Get())
			}
			app.Key("home")
			if value.Get() != 20 {
				t.Fatalf("Home ignored the new minimum: %v", value.Get())
			}
		}
		app.Post(func() { value.Set(20); step.Set(0) })
		app.Settle()
		checkMarks(0, 20)
		app.AccessibilityAction("level-thumb", core.ActionIncrement)
		if math.Abs(value.Get()-20.6) > 1e-9 {
			t.Fatalf("continuous increment = %v, want 20.6", value.Get())
		}
		app.Post(func() { step.Set(math.Inf(1)) })
		app.Settle()
		checkMarks(0, 20)
		if app.Find("level-thumb").ID != thumb {
			t.Fatal("changing steps replaced the thumb")
		}
	})
}

// A drag holds back both thumbs' accessible values until it ends, though a range's other thumb takes its bound from the one that moves.
func TestSliderRangeDragHoldsAccessibleValues(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var low, high *core.Signal[float64]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			low, high = core.NewSignal(cx, 20.0), core.NewSignal(cx, 60.0)
			return Slider{TestID: "price", Label: "Price", LowLabel: "minimum", HighLabel: "maximum",
				Value: low.Get, OnChange: low.Set, High: high.Get, OnHighChange: high.Set, Style: core.Style{Width: core.Pt(220)}}
		})
		finger := app.Touch("price-high", core.Point{X: 10, Y: 8})
		finger.Move(core.Point{X: 40, Y: 8})
		if high.Get() <= 60 {
			t.Fatalf("drag did not move the high knob: %v", high.Get())
		}
		if lo, hi := app.Find("price-low").Props.Value, app.Find("price-high").Props.Value; lo.Max != 60 || hi.Now != 60 {
			t.Fatalf("a drag updated accessible values before its end: low %+v, high %+v", lo, hi)
		}
		finger.Lift()
		if lo, hi := app.Find("price-low").Props.Value, app.Find("price-high").Props.Value; lo.Max != float32(high.Get()) || hi.Now != float32(high.Get()) {
			t.Fatalf("release did not report the drag's value: low %+v, high %+v, value %v", lo, hi, high.Get())
		}
	})
}

// TestSliderDirections puts min where Axis and Inverted say: a press and
// the arrow keys along the track follow it.
func TestSliderDirections(t *testing.T) {
	cases := []struct {
		name     string
		axis     base.Axis
		inverted bool
		press    core.Point
		key      string
	}{
		{"bottom up", base.Vertical, false, core.Point{X: 10, Y: 60}, "up"},
		{"top down", base.Vertical, true, core.Point{X: 10, Y: 160}, "down"},
		{"end to start", base.Horizontal, true, core.Point{X: 60, Y: 10}, "left"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var value *core.Signal[float64]
			app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
				value = core.NewSignal(cx, 0.0)
				style := core.Style{Width: core.Pt(220)}
				if c.axis == base.Vertical {
					style = core.Style{Height: core.Pt(220)}
				}
				return core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
					Slider{TestID: "level", Label: "Level", Axis: c.axis, Inverted: c.inverted, Value: value.Get, OnChange: value.Set, Step: 5, Style: style}}}
			})
			// 50 pt from the max end of a 200-point rail.
			app.Touch(viewOf(app.Find("level").Children[0]), c.press).Lift()
			if value.Get() != 75 {
				t.Fatalf("a press at %v gave %v, want 75", c.press, value.Get())
			}
			app.Focus("level-thumb")
			app.Key(c.key)
			if value.Get() != 80 {
				t.Fatalf("%s gave %v, want 80", c.key, value.Get())
			}
		})
	}
}
