package base_test

import (
	"slices"
	"testing"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

func TestSafeAreaAndKeyboard(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.SafeAreaView{Edges: base.Top | base.Bottom, Style: core.Style{Padding: core.All(10)}, TestID: "safe",
			Children: []core.Node{
				base.KeyboardAvoidingView{TestID: "avoid", Children: []core.Node{core.Text{Content: "x", Style: core.Style{FontSize: 17}}}},
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

func TestPopupsReportChangesAndEveryCancel(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignStart}, Children: []core.Node{
			base.Popover{TestID: "pop", Trigger: core.Text{Content: "Pop"}, DefaultOpen: true,
				NoCloseOnEscape: true, NoCloseOnPressOutside: true,
				OnOpenChange: func(open bool, r base.Reason) { log = append(log, "change "+r.String()) },
				OnCancel:     func() { log = append(log, "cancel") },
				PopupStyle:   core.Style{Width: core.Pt(20), Height: core.Pt(20)},
				Children:     []core.Node{core.Text{Content: "inside"}}},
			base.Tooltip{TestID: "tip", Trigger: core.Text{Content: "Tip"},
				OnOpenChange: func(open bool, r base.Reason) { log = append(log, "tooltip "+r.String()) }},
			base.Menu{TestID: "menu", Trigger: core.Text{Content: "Menu"}, Style: core.Style{Width: core.Pt(200)},
				PopupStyle: core.Style{Width: core.Pt(50)},
				Children:   []core.Node{base.MenuItem{Label: "Item", TestID: "item"}}},
		}}
	})
	app.Back()
	app.Hover("id:tip", false)
	// Pressing the menu's trigger presses outside the popover.
	app.Press("text:Menu")
	if want := []string{"cancel", "cancel"}; !slices.Equal(log, want) {
		t.Fatalf("log = %q, want both attempts to close cancelled and no requests that change nothing", log)
	}
	app.Find("text:inside")
	menu, popup := app.Find("id:menu"), app.Find("id:item").Parent
	if popup.Frame.Origin.X != menu.Frame.Origin.X {
		t.Errorf("menu at x %v, want at its trigger's start %v", popup.Frame.Origin.X, menu.Frame.Origin.X)
	}
}

func TestSplitResizesInPointsAndResets(t *testing.T) {
	var sizes []core.Length
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.Split{Direction: base.SplitHorizontal, TestID: "split",
			DefaultSize: core.Pt(200), Min: core.Pt(120), Max: core.Pt(300),
			OnResize: func(size core.Length) { sizes = append(sizes, size) },
			Style:    core.Style{Height: core.Pt(200)},
			First:    core.View{TestID: "first"},
			Second:   core.View{TestID: "second"}}
	}, nsgotest.Size(600, 400))
	width := func() core.Points { return app.Find("id:first").Frame.Size.Width }
	if width() != 200 {
		t.Fatalf("first pane = %v wide, want its default 200", width())
	}
	app.Pan("id:split-divider", core.Point{X: 500})
	if width() != 300 {
		t.Fatalf("first pane = %v wide, want the drag stopped at its maximum 300", width())
	}
	if v := app.Find("id:split-divider").Props.Value; v != (core.AccessibilityValue{Min: 120, Max: 300, Now: 300}) {
		t.Fatalf("divider value = %+v", v)
	}
	if c := app.Find("id:split-divider").Style.Cursor; c != core.CursorColResize {
		t.Fatalf("divider cursor = %v, want the horizontal resize cursor", c)
	}
	app.Press("id:split-divider")
	app.Press("id:split-divider")
	if width() != 200 {
		t.Fatalf("first pane = %v wide after a double click, want the default 200", width())
	}
	if len(sizes) != 2 {
		t.Fatalf("reported sizes = %v, want the drag and reset", sizes)
	}
	if p, ok := sizes[0].Points(); !ok || p != 300 {
		t.Fatalf("drag reported %v, want 300 points", sizes[0])
	}
	if p, ok := sizes[1].Points(); !ok || p != 200 {
		t.Fatalf("reset reported %v, want 200 points", sizes[1])
	}
}

func TestSplitPercentFollowsTheSplitAndVerticalStack(t *testing.T) {
	var sizes []core.Length
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.Split{Direction: base.SplitVertical, TestID: "split",
			DefaultSize: core.Pct(25), Min: core.Pt(80), Max: core.Pct(80),
			OnResize: func(size core.Length) { sizes = append(sizes, size) },
			Style:    core.Style{Width: core.Pt(300), Height: core.Pct(100)},
			First:    core.View{TestID: "first", Style: core.Style{Height: core.Pct(100)}},
			Second:   core.View{TestID: "second", Style: core.Style{Height: core.Pct(100)}}}
	}, nsgotest.Size(400, 800))
	height := func() core.Points { return app.Find("id:first").Frame.Size.Height }
	if height() != 200 {
		t.Fatalf("first pane = %v tall, want a quarter of 800", height())
	}
	if d := app.Find("id:split-divider").Frame.Size; d != (core.Size{Width: 300, Height: 9}) {
		t.Fatalf("divider = %v, want the split's width and its band", d)
	}
	if l := app.Find("id:split-divider").Children[0].Frame.Size; l != (core.Size{Width: 300, Height: 1}) {
		t.Fatalf("divider line = %v, want the split's width and one point", l)
	}
	if c := app.Find("id:split-divider").Style.Cursor; c != core.CursorRowResize {
		t.Fatalf("divider cursor = %v, want the vertical resize cursor", c)
	}
	app.Pan("id:split-divider", core.Point{Y: 100})
	if height() != 300 {
		t.Fatalf("first pane = %v tall after the drag, want 37.5%%", height())
	}
	if pct, ok := sizes[len(sizes)-1].Percent(); !ok || pct != 37.5 {
		t.Fatalf("drag reported %v, want 37.5 percent", sizes[len(sizes)-1])
	}
	app.Resize(400, 1200)
	if height() != 450 {
		t.Fatalf("first pane = %v tall after growing, want it to keep 37.5%%", height())
	}
	app.Press("id:split-divider")
	app.Press("id:split-divider")
	if height() != 300 {
		t.Fatalf("first pane = %v tall after a double click, want a quarter of 1200", height())
	}
	if pct, ok := sizes[len(sizes)-1].Percent(); !ok || pct != 25 {
		t.Fatalf("reset reported %v, want 25 percent", sizes[len(sizes)-1])
	}
}

func TestSplitDragsTowardTheDividerInRightToLeft(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.Split{Direction: base.SplitHorizontal, TestID: "split",
			DefaultSize: core.Pt(200), Max: core.Pt(300),
			Style:  core.Style{Height: core.Pt(200)},
			First:  core.View{TestID: "first", Style: core.Style{Height: core.Pct(100)}},
			Second: core.View{TestID: "second", Style: core.Style{Height: core.Pct(100)}}}
	}, nsgotest.Size(600, 400))
	app.SetEnvironment(core.Environment{RTL: true})
	app.Pan("id:split-divider", core.Point{X: -100})
	if got := app.Find("id:first").Frame.Size.Width; got != 300 {
		t.Fatalf("first pane = %v wide, want a drag toward the left to grow it in RTL", got)
	}
}
