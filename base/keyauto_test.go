package base_test

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestAppKeysLeaveAFocusedControlItsKeys holds an app's bindings of Space, Return and the arrows to leave a focused button its Return and Space, and a focused slider or radio group its arrows, while they fire with nothing such focused and a KeyAlways binding fires everywhere (#205). The app binds them again after each focus, so they are newer than the control's own and win without the rule.
func TestAppKeysLeaveAFocusedControlItsKeys(t *testing.T) {
	var log []string
	var rebind *core.Signal[int]
	var plan *core.Signal[string]
	var level float64
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rebind = core.NewSignal(cx, 0)
		plan = core.NewSignal(cx, "a")
		radio := func(name string) core.Node {
			return base.Radio{TestID: name, Label: name, Style: core.Style{Height: core.Pt(20)},
				Checked: func() bool { return plan.Get() == name }, OnChange: func(bool) { plan.Set(name) }}
		}
		return core.View{Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				rebind.Get()
				// Return is a form's default action, which leaves a focused button its Return (#205's comment).
				for _, k := range []string{"space", "enter", "right", "left"} {
					cx.OnKey(k, func() { log = append(log, k) })
				}
				cx.OnKeyWith("down", core.KeyOptions{When: core.KeyAlways}, func() { log = append(log, "down") })
				return nil
			}),
			base.Button{TestID: "resend", Label: "Resend", OnPress: func(core.PressEvent) { log = append(log, "pressed") },
				Style: core.Style{Height: core.Pt(20)}, Children: []core.Node{core.Text{Content: "Resend"}}},
			base.Slider{TestID: "slider", Label: "Level", OnChange: func(v base.SliderValue) { level = v.End },
				Style:          core.Style{Width: core.Pt(200)},
				TrackStyle:     core.Style{Height: core.Pt(24), JustifyContent: core.JustifyCenter},
				IndicatorStyle: core.Style{Height: core.Pt(4)},
				ThumbStyle:     core.Style{Width: core.Pt(16), Height: core.Pt(16)}},
			base.RadioGroup{Label: "Plan", Children: []core.Node{radio("a"), radio("b"), radio("c")}},
			core.Input{TestID: "field", Label: "Name", Style: core.Style{Height: core.Pt(20)}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	press := func(key string, want ...string) {
		t.Helper()
		log = nil
		app.Key(key)
		if !slices.Equal(log, want) {
			t.Errorf("%s ran %q, want %q", key, log, want)
		}
	}
	focus := func(selector string) {
		t.Helper()
		app.Focus(selector)
		rebind.Set(rebind.GetUntracked() + 1)
		app.Settle()
	}

	press("space", "space")
	press("enter", "enter")
	press("right", "right")

	focus("resend")
	press("space", "pressed")
	press("enter", "pressed")
	press("right", "right")
	press("down", "down")

	focus("label:Level")
	press("right")
	if level != 1 {
		t.Errorf("right on the focused slider moved it to %v, want 1", level)
	}
	press("down", "down")

	focus("a")
	press("right")
	if got := plan.GetUntracked(); got != "b" || !app.Find("b").Focused {
		t.Errorf("right in the focused radio group checked %q", got)
	}

	focus("field")
	press("space")
	press("down", "down")
	if got := app.Find("field").Text; got != " " {
		t.Errorf("the focused field reads %q, want the space typed", got)
	}
}

// TestAListLeavesAFocusedButtonItsKeys holds a List that a press made take the keys to leave Return and Space to a button that has the keyboard focus, as the press on a row moved no focus, while its arrows still move it (#205).
func TestAListLeavesAFocusedButtonItsKeys(t *testing.T) {
	var sel *base.Selection
	var pressed, activated int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		sel = base.NewSelection(cx, false)
		return core.View{Children: []core.Node{
			base.Button{TestID: "go", Label: "Go", OnPress: func(core.PressEvent) { pressed++ }, Style: core.Style{Height: core.Pt(20)},
				Children: []core.Node{core.Text{Content: "Go"}}},
			rowsOf(sel, func(int) { activated++ }),
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Focus("go")
	app.Report(nsgotest.Event{Kind: nsgotest.EventPress, ID: app.Find("label:Cherry").ID, Count: 1})
	if !slices.Equal(sel.Keys(), []any{"Cherry"}) || !app.Find("go").Focused {
		t.Fatalf("after the press the selection is %v and the button focused %v", sel.Keys(), app.Find("go").Focused)
	}
	app.Key("space")
	app.Key("enter")
	if pressed != 2 || activated != 0 || !slices.Equal(sel.Keys(), []any{"Cherry"}) {
		t.Errorf("Space and Return pressed the button %d times, activated %d rows and chose %v; want 2, 0 and Cherry", pressed, activated, sel.Keys())
	}
	app.Key("up")
	if !slices.Equal(sel.Keys(), []any{"Blueberry"}) {
		t.Errorf("up chose %v, want Blueberry", sel.Keys())
	}
}
