package base_test

import (
	"reflect"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Validation can change without remounting a control, taking both its accessible state and its class/style variants back to their valid form.
func TestControlsFollowInvalid(t *testing.T) {
	controls := []core.Node{
		base.Button{}, base.Checkbox{}, base.Switch{}, base.Radio{}, base.RadioGroup{},
		base.Toggle{}, base.ToggleGroup{}, base.Link{}, base.Tab{}, base.Tabs{},
		base.Slider{}, base.NumberInput{}, base.OtpInput{Length: 4},
		base.Select{Items: func() []base.SelectItem { return nil }},
		base.Combobox{Items: func() []base.SelectItem { return nil }},
		base.TimeField{}, base.DatePicker{}, base.Calendar{},
		base.ColorPicker{}, base.ColorArea{}, base.ColorSlider{}, base.ColorField{}, base.ColorEyeDropper{},
		base.List{Count: func() int { return 0 }}, base.Tree{Items: func() []base.TreeItem { return nil }},
	}
	for _, control := range controls {
		t.Run(reflect.TypeOf(control).Name(), func(t *testing.T) {
			var invalid *core.Signal[bool]
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				invalid = core.NewSignal(cx, false)
				v := reflect.New(reflect.TypeOf(control)).Elem()
				v.Set(reflect.ValueOf(control))
				for name, value := range map[string]any{
					"Invalid": invalid.Get, "TestID": "control",
					"Class": "aria-invalid:bg-[#ff0000]",
					"Style": core.Style{Width: core.Pt(160), Height: core.Pt(40), BorderWidth: 1,
						AriaInvalid: &core.Style{BorderWidth: 3}},
				} {
					field := v.FieldByName(name)
					if !field.IsValid() {
						t.Fatalf("%T has no %s", control, name)
					}
					field.Set(reflect.ValueOf(value))
				}
				return v.Interface().(core.Node)
			}, nsgotest.Environment(core.Environment{ScreenColor: true}))
			mounted := app.FindMounted("id:control").ID
			for _, bad := range []bool{false, true, false} {
				invalid.Set(bad)
				app.Settle()
				v := app.FindMounted("id:control")
				if v.ID != mounted || v.Props.Invalid != bad {
					t.Fatalf("invalid=%v: node %v, invalid %v; want node %v", bad, v.ID, v.Props.Invalid, mounted)
				}
				color, width := core.Color(0), core.Points(1)
				if bad {
					color, width = core.RGB(0xff0000), 3
				}
				if v.Style.Background != color || v.Style.BorderWidth != width {
					t.Errorf("invalid=%v: background %x, border %v; want %x, %v", bad, v.Style.Background, v.Style.BorderWidth, color, width)
				}
			}
		})
	}
}

func TestInvalidPartsStyleWithoutReportingValidation(t *testing.T) {
	var bad *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		bad = core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			base.Checkbox{TestID: "check", Invalid: bad.Get, IndicatorClass: "group aria-invalid:bg-[#ff0000]",
				Indicator: core.View{TestID: "mark", Class: "group-aria-invalid:bg-[#00ff00]"}},
			base.Switch{TestID: "switch", Invalid: bad.Get,
				TrackClass: "aria-invalid:bg-[#ff0000]", ThumbClass: "aria-invalid:bg-[#ff0000]"},
			base.Slider{TestID: "slider", Invalid: bad.Get, ThumbTestID: "thumb", StartThumbTestID: "start",
				DefaultValue: base.RangeValue(10, 30), TrackClass: "bg-[#001122] aria-invalid:bg-[#ff0000]",
				Style: core.Style{Width: core.Pt(160), Height: core.Pt(40)}},
			base.NumberInput{TestID: "number", Invalid: bad.Get, InputClass: "bg-[#001122] aria-invalid:bg-[#ff0000]"},
			base.OtpInput{TestID: "code", Length: 4, Invalid: bad.Get, CellClass: "bg-[#001122] aria-invalid:bg-[#ff0000]", GroupClass: "bg-[#001122] aria-invalid:bg-[#ff0000]"},
			base.TimeField{TestID: "time", Invalid: bad.Get, HourCycle: base.HourCycle23,
				SegmentClass: "bg-[#001122] aria-invalid:bg-[#ff0000]", SeparatorClass: "bg-[#001122] aria-invalid:bg-[#ff0000]"},
		}}
	})
	for _, invalid := range []bool{true, false} {
		bad.Set(invalid)
		app.Settle()
		red, green := core.Color(0), core.Color(0)
		if invalid {
			red, green = core.RGB(0xff0000), core.RGB(0x00ff00)
		}
		check := app.FindMounted("id:check")
		indicator := check.Children[0]
		if indicator.Props.Invalid || indicator.Style.Background != red {
			t.Errorf("checkbox indicator: invalid=%v, background=%x", indicator.Props.Invalid, indicator.Style.Background)
		}
		if v := app.FindMounted("id:mark"); v.Props.Invalid || v.Style.Background != green {
			t.Errorf("group on decorative part: invalid=%v, background=%x", v.Props.Invalid, v.Style.Background)
		}
		track := app.FindMounted("id:switch").Children[0]
		for _, part := range []*nsgotest.View{track, track.Children[0]} {
			if part.Props.Invalid || part.Style.Background != red {
				t.Errorf("switch part: invalid=%v, background=%x", part.Props.Invalid, part.Style.Background)
			}
		}
		for _, id := range []string{"thumb", "start"} {
			if v := app.FindMounted("id:" + id); v.Props.Invalid != invalid {
				t.Errorf("slider %s invalid=%v, want %v", id, v.Props.Invalid, invalid)
			}
		}
		// Every visible field part changes its look along with the real input.
		partColor := core.RGB(0x001122)
		if invalid {
			partColor = red
		}
		for id, wantParts := range map[string]int{"slider": 1, "number": 1, "code": 5, "time": 3} {
			var parts int
			var visit func(*nsgotest.View)
			visit = func(v *nsgotest.View) {
				if v.Style.Background == core.RGB(0x001122) || v.Style.Background == core.RGB(0xff0000) {
					parts++
					if v.Style.Background != partColor {
						t.Errorf("%s part background=%x, want %x", id, v.Style.Background, partColor)
					}
					if id != "number" && v.Props.Invalid {
						t.Errorf("%s decorative part reports invalid", id)
					}
				}
				for _, child := range v.Children {
					visit(child)
				}
			}
			visit(app.FindMounted("id:" + id))
			if parts != wantParts {
				t.Errorf("%s has %d styled parts, want %d", id, parts, wantParts)
			}
		}
		// The real text fields must carry the accessible state even when the visible cells/segments are only decorative.
		for _, id := range []string{"number", "code", "time"} {
			var fields int
			var visit func(*nsgotest.View)
			visit = func(v *nsgotest.View) {
				if v.Kind == nsgotest.KindInput {
					fields++
					if v.Props.Invalid != invalid {
						t.Errorf("%s field invalid=%v, want %v", id, v.Props.Invalid, invalid)
					}
				}
				for _, child := range v.Children {
					visit(child)
				}
			}
			visit(app.FindMounted("id:" + id))
			if fields != 1 {
				t.Errorf("%s has %d native fields, want 1", id, fields)
			}
		}
	}
}

// A color picker's validation reaches the actual fields and adjustable thumbs, including a channel rendered through NumberInput.
func TestColorPickerSharesInvalidWithItsParts(t *testing.T) {
	var bad *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		bad = core.NewSignal(cx, false)
		return base.ColorPicker{Invalid: bad.Get, Children: []core.Node{
			base.ColorField{TestID: "text"},
			base.ColorField{TestID: "channel", ColorSpace: base.ColorRGB, Channel: base.ChannelRed},
			base.ColorArea{TestID: "area"}, base.ColorSlider{TestID: "slider"}, base.ColorEyeDropper{TestID: "pick"},
		}}
	}, nsgotest.Environment(core.Environment{ScreenColor: true}))
	for _, invalid := range []bool{true, false} {
		bad.Set(invalid)
		app.Settle()
		for _, id := range []string{"text", "channel", "area", "slider", "pick"} {
			var controls int
			var visit func(*nsgotest.View)
			visit = func(v *nsgotest.View) {
				if v.Kind == nsgotest.KindInput || v.Props.Role == core.RoleAdjustable || v.Props.Role == core.RoleButton {
					controls++
					if v.Props.Invalid != invalid {
						t.Errorf("%s accessible part invalid=%v, want %v", id, v.Props.Invalid, invalid)
					}
				}
				for _, child := range v.Children {
					visit(child)
				}
			}
			visit(app.FindMounted("id:" + id))
			if controls == 0 {
				t.Errorf("%s exposes no control", id)
			}
		}
	}
}

// A number input's step buttons and scrub handle look invalid with its field without reporting it, in either layout.
func TestNumberInputStepsLookInvalid(t *testing.T) {
	var bad *core.Signal[bool]
	look := "bg-[#001122] aria-invalid:bg-[#ff0000]"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		bad = core.NewSignal(cx, false)
		number := func(right bool, more, less string) core.Node {
			return base.NumberInput{Invalid: bad.Get, ControlsRight: right, IncrementLabel: more, DecrementLabel: less,
				IncrementButtonClass: look, DecrementButtonClass: look, Scrub: core.Text{Content: "drag"}, ScrubTestID: "scrub-" + more, ScrubClass: look}
		}
		return core.View{Children: []core.Node{number(false, "More", "Less"), number(true, "Up", "Down")}}
	})
	for _, invalid := range []bool{true, false} {
		bad.Set(invalid)
		app.Settle()
		want := core.RGB(0x001122)
		if invalid {
			want = core.RGB(0xff0000)
		}
		for _, sel := range []string{"label:More", "label:Less", "label:Up", "label:Down", "id:scrub-More", "id:scrub-Up"} {
			if v := app.FindMounted(sel); v.Props.Invalid || v.Style.Background != want {
				t.Errorf("invalid=%v: %s reports invalid %v, background %x; want %x", invalid, sel, v.Props.Invalid, v.Style.Background, want)
			}
		}
	}
}
