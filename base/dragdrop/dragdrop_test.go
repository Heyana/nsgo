package dragdrop_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/dragdrop"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func start(t *testing.T, build func(*core.Scope) core.Node) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, build, nsgotest.Size(400, 300), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
}

func TestDragTransfersSelectedValuesAndPreservesSourceAndPreview(t *testing.T) {
	var got any
	var sequence []string
	var ended dragdrop.Result
	var data *core.Signal[[]int]
	app := start(t, func(cx *core.Scope) core.Node {
		data = core.NewSignalFunc(cx, []int{2, 5}, slices.Equal[[]int])
		return dragdrop.Area{Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
				dragdrop.Source{View: core.View{TestID: "source", Label: "Films", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}, Children: []core.Node{core.Text{Content: "Poster", TestID: "poster"}}}, Data: func() any { return append([]int(nil), data.Get()...) }, Preview: func(value any) core.Node { return core.Text{Content: "Two films", TestID: "preview"} }, OnStart: func(any) { sequence = append(sequence, "start") }, OnEnd: func(r dragdrop.Result) { ended = r; sequence = append(sequence, "end") }},
				dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}}, OnEnter: func(dragdrop.Event) { sequence = append(sequence, "enter") }, OnDrop: func(e dragdrop.Event) { got = e.Data; sequence = append(sequence, "drop") }, OnLeave: func(dragdrop.Event) { sequence = append(sequence, "leave") }},
			}},
		}}
	})
	source, poster := app.Find("source").ID, app.Find("poster").ID
	finger := app.Touch("source", core.Point{X: 20, Y: 20})
	finger.Move(core.Point{X: 30, Y: 20})
	if !app.Exists("preview") || app.Find("source").ID != source || app.Find("poster").ID != poster {
		t.Fatal("drag remounted source or did not show preview")
	}
	preview := app.Find("preview").ID
	app.Post(func() { data.Set([]int{9}) })
	app.Settle()
	finger.Move(core.Point{X: 140, Y: 30})
	if app.Find("preview").ID != preview {
		t.Fatal("moving the preview remounted its content")
	}
	finger.Lift()
	if !reflect.DeepEqual(got, []int{2, 5}) || !ended.Dropped || ended.Target == nil {
		t.Fatalf("drop=%v result=%+v", got, ended)
	}
	if !reflect.DeepEqual(sequence, []string{"start", "enter", "drop", "leave", "end"}) {
		t.Fatalf("callback order=%v", sequence)
	}
	if app.Exists("preview") || app.Find("source").ID != source || app.Find("poster").ID != poster {
		t.Fatal("finished drag lost source or retained preview")
	}
}

func TestDragThresholdLeavesClickAndCancellationNeverDrops(t *testing.T) {
	pressed, dropped, ended := 0, 0, 0
	app := start(t, func(*core.Scope) core.Node {
		return dragdrop.Area{Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
			dragdrop.Source{View: core.View{TestID: "source", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}, OnPress: func(core.PressEvent) { pressed++ }}, Data: func() any { return 1 }, OnEnd: func(r dragdrop.Result) {
				ended++
				if r.Dropped {
					t.Error("cancelled drag reported a drop")
				}
			}},
			dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, OnDrop: func(dragdrop.Event) { dropped++ }},
		}}}}
	})
	app.Drag("source", core.Point{X: 2})
	if pressed != 1 || ended != 0 {
		t.Fatalf("short movement pressed=%d ended=%d", pressed, ended)
	}
	for _, escape := range []bool{false, true} {
		finger := app.Touch("source", core.Point{X: 20, Y: 20})
		finger.Move(core.Point{X: 140, Y: 30})
		if escape {
			app.Key("escape")
			finger.Lift()
		} else {
			finger.Cancel()
		}
	}
	if pressed != 1 || dropped != 0 || ended != 2 {
		t.Fatalf("cancel pressed=%d dropped=%d ended=%d", pressed, dropped, ended)
	}
}

func TestLongPressActivationIsExplicit(t *testing.T) {
	drops := 0
	app := start(t, func(*core.Scope) core.Node {
		return dragdrop.Area{Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
			dragdrop.Source{Activation: dragdrop.AfterLongPress, View: core.View{TestID: "source", DelayLongPress: 200 * time.Millisecond, Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, Data: func() any { return 1 }},
			dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, OnDrop: func(dragdrop.Event) { drops++ }},
		}}}}
	})
	app.DragTo("source", "target")
	if drops != 0 {
		t.Fatal("movement started a long-press source early")
	}
	finger := app.Touch("source", core.Point{X: 20, Y: 20})
	app.Advance(250 * time.Millisecond)
	finger.Move(core.Point{X: 140, Y: 30})
	finger.Lift()
	if drops != 1 {
		t.Fatalf("long press delivered %d drops", drops)
	}
}

func TestRemovingSourceTargetOrAreaBalancesLifecycle(t *testing.T) {
	for _, removed := range []string{"source", "target", "area"} {
		t.Run(removed, func(t *testing.T) {
			var visible *core.Signal[bool]
			enters, leaves, drops, ends := 0, 0, 0, 0
			app := start(t, func(cx *core.Scope) core.Node {
				visible = core.NewSignal(cx, true)
				source := dragdrop.Source{View: core.View{TestID: "source", Label: "Film", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, Data: func() any { return 1 }, Preview: func(any) core.Node { return core.Text{Content: "Moving", TestID: "preview"} }, OnEnd: func(r dragdrop.Result) {
					ends++
					if r.Dropped {
						t.Error("removal completed a drop")
					}
				}}
				target := dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, OnEnter: func(dragdrop.Event) { enters++ }, OnLeave: func(dragdrop.Event) { leaves++ }, OnDrop: func(dragdrop.Event) { drops++ }}
				var from, to core.Node = source, target
				if removed == "source" {
					from = core.Show{When: visible.Get, Then: func(*core.Scope) core.Node { return source }}
				}
				if removed == "target" {
					to = core.Show{When: visible.Get, Then: func(*core.Scope) core.Node { return target }}
				}
				area := dragdrop.Area{Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{from, to}}}}
				if removed == "area" {
					return core.Show{When: visible.Get, Then: func(*core.Scope) core.Node { return area }}
				}
				return area
			})
			finger := app.Touch("source", core.Point{X: 20, Y: 20})
			finger.Move(core.Point{X: 140, Y: 30})
			if enters != 1 {
				t.Fatal("drag did not reach target")
			}
			app.Post(func() { visible.Set(false) })
			app.Settle()
			if leaves != 1 {
				t.Fatalf("removing %s left %d leave notifications", removed, leaves)
			}
			finger.Lift()
			if drops != 0 || ends != 1 || app.Exists("preview") {
				t.Fatalf("removing %s: drops=%d ends=%d", removed, drops, ends)
			}
		})
	}
}

func TestClosestAcceptingTargetAndStationaryDisabledTarget(t *testing.T) {
	var accept, disabled *core.Signal[bool]
	var drops []string
	app := start(t, func(cx *core.Scope) core.Node {
		accept, disabled = core.NewSignal(cx, true), core.NewSignal(cx, false)
		return dragdrop.Area{Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
			dragdrop.Source{View: core.View{TestID: "source", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, Data: func() any { return "film" }},
			dragdrop.Target{View: core.View{TestID: "outer", Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}, Children: []core.Node{
				dragdrop.Target{View: core.View{TestID: "inner", Disabled: disabled.Get, Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, Accept: func(any) bool { return accept.Get() }, OnDrop: func(dragdrop.Event) { drops = append(drops, "inner") }},
			}}, OnDrop: func(dragdrop.Event) { drops = append(drops, "outer") }},
		}}}}
	})
	app.DragTo("source", "inner")
	app.Post(func() { accept.Set(false) })
	app.Settle()
	app.DragTo("source", "inner")
	if !reflect.DeepEqual(drops, []string{"inner", "outer"}) {
		t.Fatalf("target selection=%v", drops)
	}
	finger := app.Touch("source", core.Point{X: 20, Y: 20})
	finger.Move(core.Point{X: 140, Y: 30})
	app.Post(func() { disabled.Set(true) })
	app.Settle()
	finger.Lift()
	if len(drops) != 2 {
		t.Fatal("drop passed through a newly disabled descendant")
	}
}

func TestDragCancelsNestedControlPressAndRetainsItsNormalAction(t *testing.T) {
	presses, drops := 0, 0
	app := start(t, func(*core.Scope) core.Node {
		return dragdrop.Area{Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
			dragdrop.Source{View: core.View{TestID: "source", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}, Children: []core.Node{
				core.View{TestID: "control", Role: core.RoleButton, Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}, OnPress: func(core.PressEvent) { presses++ }},
			}}, Data: func() any { return 1 }},
			dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, OnDrop: func(dragdrop.Event) { drops++ }},
		}}}}
	})
	app.Press("control")
	app.DragTo("source", "target")
	if presses != 1 || drops != 1 {
		t.Fatalf("nested control presses=%d drops=%d", presses, drops)
	}
}

func TestPreviewStaysWithSourcesModalAndInheritsItsContext(t *testing.T) {
	key := core.NewKey[string]("drag preview context", "outside")
	app := start(t, func(*core.Scope) core.Node {
		return dragdrop.Area{Children: []core.Node{core.Modal{TestID: "modal", Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
			key.Provide(cx, "inside")
			return core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
				dragdrop.Source{View: core.View{TestID: "source", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, Data: func() any { return 1 }, Preview: func(any) core.Node {
					return core.Func(func(cx *core.Scope) core.Node { return core.Text{TestID: "preview", Content: key.Use(cx)} })
				}},
				dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)}}, OnDrop: func(dragdrop.Event) {}},
			}}
		})}}}}
	})
	finger := app.Touch("source", core.Point{X: 20, Y: 20})
	finger.Move(core.Point{X: 40, Y: 30})
	if got := app.Find("id:modal >> id:preview"); got.Text != "inside" {
		t.Fatalf("preview lost source context: %q", got.Text)
	}
	finger.Cancel()
}
