package base_test

import (
	"testing"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

func TestControlsReportTheNextValue(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		mixed := core.NewSignal(cx, true)
		tab := core.NewSignal(cx, 0)
		return core.View{Children: []core.Node{
			base.Checkbox{Checked: on.Get, Indeterminate: mixed.Get, Label: "Select all", TestID: "all",
				OnChange: func(v bool) { on.Set(v); mixed.Set(false) },
				IndicatorStyle: core.Style{Width: core.Pt(16), Height: core.Pt(16),
					Checked: &core.Style{Background: core.RGB(0x0969da)}},
			},
			base.Switch{Checked: on.Get, OnChange: on.Set, Label: "Wi-Fi", TestID: "wifi"},
			base.Tabs{Children: []core.Node{
				base.Tab{Selected: func() bool { return tab.Get() == 0 }, OnPress: func() { tab.Set(0) }, Label: "One", TestID: "one"},
				base.Tab{Selected: func() bool { return tab.Get() == 1 }, OnPress: func() { tab.Set(1) }, Label: "Two", TestID: "two"},
			}},
		}}
	})
	all := app.Find("role:checkbox Select all")
	if !all.Props.Indeterminate || all.Props.Checked {
		t.Fatalf("checkbox starts %+v, want mixed", all.Props)
	}
	app.Press("id:all")
	if all := app.Find("id:all"); !all.Props.Checked || all.Props.Indeterminate {
		t.Fatalf("after press %+v, want checked", all.Props)
	}
	if indicator := app.Find("id:all").Children[0]; indicator.Style.Background != core.RGB(0x0969da) {
		t.Errorf("indicator background = %x, want its checked style", indicator.Style.Background)
	}
	if !app.Find("role:switch Wi-Fi").Props.Checked {
		t.Error("the switch does not follow the shared value")
	}
	app.Press("id:wifi")
	if app.Find("id:all").Props.Checked {
		t.Error("turning the switch off did not uncheck the checkbox")
	}
	app.Press("label:Two")
	if !app.Find("id:two").Props.Selected || app.Find("id:one").Props.Selected {
		t.Error("pressing a tab did not select it")
	}
}

func TestSafeAreaAndKeyboard(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.SafeAreaView{Edges: base.Top | base.Bottom, Style: core.Style{Padding: core.All(10)}, TestID: "safe",
			Children: []core.Node{
				base.KeyboardAvoidingView{TestID: "avoid", Children: []core.Node{core.Text{Content: "x"}}},
			}}
	})
	app.SetEnvironment(core.Environment{SafeArea: core.Edges{Top: 44, Bottom: 34, Left: 5}, KeyboardHeight: 300})
	safe, avoid := app.Find("id:safe"), app.Find("id:avoid")
	if got := avoid.Frame.Origin; got != (core.Point{X: 10, Y: 54}) {
		t.Errorf("content origin = %v, want insets added to padding on top only", got)
	}
	// Text 17 points tall is 20.4, rounded up to 21.
	if got, want := safe.Frame.Size.Height, core.Points(10+44+300+21+10+34); got != want {
		t.Errorf("height = %v, want %v", got, want)
	}
}

func TestPopupsNavAndAlerts(t *testing.T) {
	var reasons []string
	var chosen string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		nav := NewNavHelper(cx)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			base.Menu{TestID: "menu", Trigger: core.Text{Content: "File"},
				OnOpenChange: func(open bool, r base.Reason) { reasons = append(reasons, r.String()) },
				Children: []core.Node{
					base.MenuItem{Label: "Open", OnSelect: func() { nav.Push("detail") }},
					base.MenuItem{Label: "Quit", OnSelect: func() {
						base.ShowAlertDialog(cx, base.AlertDialog{Title: "Quit?", Actions: []core.AlertAction{
							{Label: "Cancel", Role: core.AlertCancel}, {Label: "Quit", Role: core.AlertDestructive}}},
							func(label string) { chosen = label })
					}},
				}},
			nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, _ func() int) core.Screen {
				return core.Screen{Title: route, TestID: "page-" + route,
					Children: []core.Node{core.Text{Content: "page " + route}}}
			}),
		}}
	})
	app.Press("text:File")
	app.Press("label:Open")
	app.Find("text:page detail")
	if app.Exists("label:Open") {
		t.Fatal("choosing an item left the menu open")
	}
	app.Back()
	app.Find("text:page home")
	if app.Exists("text:page detail") {
		t.Fatal("back did not pop the page")
	}

	app.Press("text:File")
	app.Press("label:Quit")
	if alerts := app.Alerts(); len(alerts) != 1 || alerts[0].Title != "Quit?" {
		t.Fatalf("alerts = %+v", alerts)
	}
	app.Answer(1)
	if chosen != "Quit" {
		t.Fatalf("chosen = %q", chosen)
	}
	want := []string{"triggerPress", "select", "triggerPress", "select"}
	if len(reasons) != len(want) {
		t.Fatalf("reasons = %v, want %v", reasons, want)
	}
}

func NewNavHelper(cx *core.Scope) *base.Nav[string] { return base.NewNav(cx, "home") }
