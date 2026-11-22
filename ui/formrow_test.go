package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

func TestFormRowTextColumnOptions(t *testing.T) {
	each(t, func(t *testing.T, env core.Environment) {
		var note *core.Signal[string]
		app, _ := start(t, env, func(cx *core.Scope) core.Node {
			note = core.NewSignal(cx, "Preparing")
			return core.View{Style: core.Style{Width: core.Pt(280)}, Children: []core.Node{
				FormRow{TestID: "row", Title: "First line\nSecond line\nThird line", TitleMaxLines: 1,
					Detail: "Details", MinHeight: 84, Leading: core.View{Style: core.Style{Width: core.Pt(24), Height: core.Pt(24)}},
					Control: core.Text{TestID: "control", Content: "Value"},
					Below:   core.Text{TestID: "below", ContentFunc: note.Get}},
				FormRow{TestID: "uncapped", Title: "First line\nSecond line\nThird line"},
			}}
		})
		row := app.Find("row")
		if row.Frame.Size.Height != 84 {
			t.Fatalf("row height = %v, want 84", row.Frame.Size.Height)
		}
		title := row.Children[1].Children[0]
		if title.MaxLines != 1 || title.Frame.Size.Height >= app.Find("uncapped").Children[0].Children[0].Frame.Size.Height {
			t.Fatalf("title was not limited to one line: max %d, height %v", title.MaxLines, title.Frame.Size.Height)
		}
		below, detail, control := app.Find("below"), app.Find("text:Details"), app.Find("control")
		if below.Bounds().Origin.X != title.Bounds().Origin.X || below.Bounds().Origin.Y < detail.Bounds().Origin.Y+detail.Frame.Size.Height || below.Bounds().Origin.X+below.Frame.Size.Width > control.Bounds().Origin.X {
			t.Fatalf("below node is outside the text column: below %v, detail %v, control %v", below.Bounds(), detail.Bounds(), control.Bounds())
		}
		id := below.ID
		app.Post(func() { note.Set("Ready") })
		app.Settle()
		if below = app.Find("below"); below.Text != "Ready" || below.ID != id {
			t.Fatal("the below slot did not update in place")
		}
	})
}

func TestFormRowRadioUsesControlledGroupBehavior(t *testing.T) {
	headless(t)
	var choice *core.Signal[string]
	presses := 0
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		choice = core.NewSignal(cx, "a")
		row := func(name string, disabled bool) core.Node {
			return FormRow{TestID: name, Title: name, Role: core.RoleRadio, Checked: func() bool { return choice.Get() == name },
				Disabled: func() bool { return disabled }, OnPress: func() { presses++; choice.Set(name) }}
		}
		return base.RadioGroup{Label: "Plan", Children: []core.Node{FormSection{Children: []core.Node{row("a", false), row("off", true), row("b", false)}}}}
	})
	a, b := app.Find("a"), app.Find("b")
	if a.Props.Role != core.RoleRadio || !a.Props.Checked || !a.Props.Selected || a.Props.Label != "a" || b.Props.Checked {
		t.Fatalf("radio state = %+v, %+v", a.Props, b.Props)
	}
	app.Press("a")
	if !app.Find("off").Props.Disabled {
		t.Fatal("disabled radio did not report its state")
	}
	if presses != 0 {
		t.Fatal("checked radio invoked its action")
	}
	app.Press("b")
	if choice.GetUntracked() != "b" || presses != 1 || a.Props.Checked || !b.Props.Checked {
		t.Fatal("press did not update the controlled choice")
	}
	app.Key("Tab")
	if !b.Focused || !b.Props.Focusable || a.Props.Focusable {
		t.Fatal("the group did not enter at its checked row")
	}
	app.Key("up")
	if choice.GetUntracked() != "a" || !a.Focused || presses != 2 {
		t.Fatal("arrow navigation did not skip the disabled row and select a")
	}
}
