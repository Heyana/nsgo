package main

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func requireDesktop(t *testing.T) {
	t.Helper()
	if target := nsgotest.Target(); target != "macos" && target != "windows" && target != "linux" {
		t.Skip("requires native app windows")
	}
}

type nativeSurfaceCounters struct{ hovered, left, context, pressed int }

func startNativeSurface(t *testing.T) (*nsgotest.App, *core.Signal[string], *nativeSurfaceCounters) {
	t.Helper()
	requireDesktop(t)
	var value *core.Signal[string]
	counts := &nativeSurfaceCounters{}
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node {
			return core.View{TestID: "form", Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.Text{TestID: "title", Content: "Main"},
				core.Input{TestID: "field", DefaultValue: "main", Style: core.Style{Height: core.Pt(28)}},
			}}
		})
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 220, Height: 220}, func(cx *core.Scope) core.Node {
			value = core.NewSignal(cx, "start")
			return core.View{TestID: "form", Style: core.Style{FlexGrow: 1, Gap: 4}, Children: []core.Node{
				core.Text{TestID: "title", Content: "Inspector", Style: core.Style{Height: core.Pt(20)}},
				core.Input{TestID: "field", Value: value.Get, OnChange: value.Set, Style: core.Style{Height: core.Pt(28)}},
				core.View{TestID: "action", Role: core.RoleButton, Label: "Apply", Style: core.Style{Height: core.Pt(28)},
					OnPress: func(core.PressEvent) { counts.pressed++ }, OnMouseEnter: func(core.MouseEvent) { counts.hovered++ },
					OnMouseLeave: func(core.MouseEvent) { counts.left++ }, OnContextMenu: func(core.ContextMenuEvent) { counts.context++ },
					Children: []core.Node{core.Text{Content: "Apply"}}},
				core.Text{TestID: "status", ContentFunc: func() string { return "Inspector " + value.Get() }, Style: core.Style{Height: core.Pt(20)}},
			}}
		})
	}, nsgotest.Size(320, 220))
	return app, value, counts
}

func TestNativeSurfaceLocalSteps(t *testing.T) {
	app, value, counts := startNativeSurface(t)
	main, inspector := app.Surface("index:0"), app.Surface("title:Inspector")
	if !inspector.Exists("text:Inspector") || inspector.Exists("text:Main") || !slices.Contains(inspector.Texts(), "Inspector") || !inspector.Enabled("id:action") {
		t.Fatal("native queries crossed windows")
	}
	inspector.Hover("id:action")
	inspector.HoverOut()
	inspector.RightClick("id:action")
	if counts.hovered == 0 || counts.left == 0 || counts.context != 1 {
		t.Fatalf("native pointer routes: enter=%d leave=%d context=%d", counts.hovered, counts.left, counts.context)
	}
	mainSize := main.Find("id:form").Frame.Size
	inspector.Resize(260, 180)
	if inspector.Find("id:form").Frame.Size != (core.Size{Width: 260, Height: 180}) || main.Find("id:form").Frame.Size != mainSize {
		t.Fatal("native resize crossed windows")
	}
	queued := make(chan struct{})
	go func() { inspector.Post(func() { value.Set("queued") }); close(queued) }()
	<-queued
	inspector.WaitFor(`text:"Inspector queued"`, 2*time.Second)
	inspector.Post(func() { value.Set("done") })
	inspector.WaitForGone(`text:"Inspector queued"`, 2*time.Second)
	inspector.WaitUntil(2*time.Second, func() bool { return inspector.Find("id:status").Text == "Inspector done" })
	if main.Find("id:field").Text != "main" {
		t.Fatal("native queued post changed the main window")
	}
	if len(inspector.CapturePNG()) == 0 {
		t.Fatal("native surface capture is empty")
	}
}

func TestNativeSurfaceEditingAndFocus(t *testing.T) {
	app, _, counts := startNativeSurface(t)
	main, inspector := app.Surface("index:0"), app.Surface("title:Inspector")
	inspector.Clear("id:field")
	inspector.Type("id:field", "settings")
	inspector.Focus("id:field")
	if !inspector.Find("id:field").Focused || inspector.Find("id:field").Text != "settings" || main.Find("id:field").Text != "main" {
		t.Fatal("native editing or focus crossed windows")
	}
	inspector.Focus("id:action")
	inspector.Key("Return")
	if counts.pressed != 1 {
		t.Fatalf("native keyboard activation = %d", counts.pressed)
	}
}

func TestNativeSurfacePostsAndNestedLifetimes(t *testing.T) {
	requireDesktop(t)
	var value *core.Signal[string]
	var parent *desktop.Window
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "Main"} })
		parent = desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Parent", Width: 220, Height: 120}, func(cx *core.Scope) core.Node {
			value = core.NewSignal(cx, "start")
			return core.Text{TestID: "value", ContentFunc: value.Get}
		})
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Child", Parent: parent, Width: 180, Height: 100}, func(*core.Scope) core.Node {
			return core.Text{Content: "Child"}
		})
	}, nsgotest.Size(320, 220))
	parentSurface, child := app.Surface("title:Parent"), app.Surface("title:Child")
	child.Post(func() { value.Set("queued from child") })
	child.Settle()
	if parentSurface.Find("id:value").Text != "queued from child" {
		t.Fatal("native child's post was lost")
	}
	app.Post(parent.Close)
	app.Settle()
	if surfaces := app.Surfaces(); len(surfaces) != 1 || !surfaces[0].Exists("text:Main") {
		t.Fatal("native nested windows survived parent close")
	}
}
