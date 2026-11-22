package nsgotest

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type surfaceStepState struct {
	value                                        *core.Signal[string]
	disabled                                     *core.Signal[bool]
	presses, entered, left, context, dragEntered int
	drops                                        []string
}

func surfaceStepForm(name string, state *surfaceStepState) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		state.value, state.disabled = core.NewSignal(cx, "start"), core.NewSignal(cx, false)
		return core.View{TestID: "form", Style: core.Style{FlexGrow: 1, Gap: 4}, Children: []core.Node{
			core.Text{TestID: "title", Content: name, Style: core.Style{Height: core.Pt(20)}},
			core.Input{TestID: "field", Value: state.value.Get, OnChange: state.value.Set, Style: core.Style{Height: core.Pt(28)}},
			core.View{TestID: "action", Role: core.RoleButton, Label: "Apply", Disabled: state.disabled.Get,
				Style:        core.Style{Height: core.Pt(28)},
				OnPress:      func(core.PressEvent) { state.presses++; state.disabled.Set(true) },
				OnMouseEnter: func(core.MouseEvent) { state.entered++ }, OnMouseLeave: func(core.MouseEvent) { state.left++ },
				OnContextMenu: func(core.ContextMenuEvent) { state.context++ }, Children: []core.Node{core.Text{Content: "Apply"}}},
			core.Text{TestID: "status", ContentFunc: func() string { return name + " " + state.value.Get() }, Style: core.Style{Height: core.Pt(20)}},
			core.View{TestID: "drop", Style: core.Style{Height: core.Pt(24)},
				OnFileDragEnter: func() { state.dragEntered++ }, OnFileDrop: func(uris []string) { state.drops = slices.Clone(uris) }},
		}}
	}
}

func startSurfaceSteps(t testing.TB, os core.OS, nested bool) (*App, *surfaceStepState, *surfaceStepState, *desktop.Window) {
	mainState, inspectorState := &surfaceStepState{}, &surfaceStepState{}
	var inspector *desktop.Window
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(surfaceStepForm("Main", mainState))
		inspector = desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 220, Height: 220}, surfaceStepForm("Inspector", inspectorState))
		if nested {
			desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Child", Width: 180, Height: 160, Parent: inspector}, surfaceStepForm("Child", &surfaceStepState{}))
		}
	}, Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop}), Size(320, 220))
	return app, mainState, inspectorState, inspector
}

func TestSurfaceLocalStepsAcrossWindows(t *testing.T) {
	platforms := []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux}
	if target := Target(); target != "" {
		switch target {
		case "macos":
			platforms = []core.OS{core.OSMacOS}
		case "windows":
			platforms = []core.OS{core.OSWindows}
		case "linux":
			platforms = []core.OS{core.OSLinux}
		default:
			t.Skip("requires native app windows")
		}
	}
	for _, os := range platforms {
		t.Run(map[core.OS]string{core.OSMacOS: "macos", core.OSWindows: "windows", core.OSLinux: "linux"}[os], func(t *testing.T) {
			app, mainState, inspectorState, _ := startSurfaceSteps(t, os, false)
			main, inspector := app.Surface("index:0"), app.Surface("title:Inspector")
			if !inspector.Exists("text:Inspector") || inspector.Exists("text:Main") || !slices.Contains(inspector.Texts(), "Inspector") {
				t.Fatal("surface queries crossed windows")
			}
			if len(inspector.QueryMounted("id:field")) != 1 || !inspector.Enabled("id:action") {
				t.Fatal("surface mounted or enabled query lost its view")
			}
			inspector.Hover("id:action")
			inspector.HoverOut()
			inspector.RightClick("id:action")
			if inspectorState.entered == 0 || inspectorState.left == 0 || inspectorState.context != 1 || mainState.entered != 0 || mainState.context != 0 {
				t.Fatalf("pointer routes: main=%+v inspector=%+v", mainState, inspectorState)
			}
			inspector.Clear("id:field")
			inspector.Type("id:field", "settings")
			inspector.Focus("id:field")
			if !inspector.Find("id:field").Focused || inspector.Find("id:field").Text != "settings" || main.Find("id:field").Text != "start" {
				t.Fatal("editing or focus crossed surfaces")
			}
			inspector.Focus("id:action")
			inspector.Key("Return")
			if inspectorState.presses != 1 || mainState.presses != 0 || inspector.Enabled("id:action") || !main.Enabled("id:action") {
				t.Fatal("keyboard activation or disabled state crossed surfaces")
			}
			mainSize := main.Find("id:form").Frame.Size
			inspector.Resize(260, 180)
			if got := inspector.Find("id:form").Frame.Size; got != (core.Size{Width: 260, Height: 180}) || main.Find("id:form").Frame.Size != mainSize {
				t.Fatalf("resize changed the wrong surface: inspector=%v main=%v", got, main.Find("id:form").Frame.Size)
			}
			posted := make(chan struct{})
			go func() { inspector.Post(func() { inspectorState.value.Set("queued") }); close(posted) }()
			<-posted
			inspector.WaitFor(`text:"Inspector queued"`, time.Second)
			if main.Find("id:status").Text != "Main start" {
				t.Fatal("queued post changed the main window")
			}
			inspector.Post(func() { inspectorState.value.Set("done") })
			inspector.WaitForGone(`text:"Inspector queued"`, time.Second)
			inspector.WaitUntil(time.Second, func() bool { return inspector.Find("id:status").Text == "Inspector done" })
		})
	}
}

func TestSurfaceFileDropsAndModelPostsStayLocal(t *testing.T) {
	if Target() != "" {
		t.Skip("headless platform reports")
	}
	app, mainState, inspectorState, _ := startSurfaceSteps(t, core.OSMacOS, false)
	inspector := app.Surface("title:Inspector")
	inspector.DropFiles("id:drop", "file:///settings.txt")
	if inspectorState.dragEntered != 1 || !slices.Equal(inspectorState.drops, []string{"file:///settings.txt"}) || mainState.dragEntered != 0 || len(mainState.drops) != 0 {
		t.Fatal("file drops crossed surfaces")
	}
	// The partial App model also schedules through its owner, never a private undrained queue.
	inspector.model.Post(func() { inspectorState.value.Set("model post") })
	inspector.Settle()
	if inspector.Find("id:field").Text != "model post" || app.Find("id:field").Text != "start" {
		t.Fatal("model post was lost or routed to the main window")
	}
}

func TestSurfacePostsAndNestedWindowLifetimes(t *testing.T) {
	if target := Target(); target != "" && target != "macos" && target != "windows" && target != "linux" {
		t.Skip("requires native app windows")
	}
	app, _, state, window := startSurfaceSteps(t, core.OSMacOS, true)
	inspector, child := app.Surface("title:Inspector"), app.Surface("title:Child")
	calls := 0
	child.Post(func() { calls++; state.value.Set("before close") })
	inspector.Settle()
	if calls != 1 || inspector.Find("id:field").Text != "before close" {
		t.Fatal("child's queued post did not use the shared graph")
	}
	app.Post(window.Close)
	app.Settle()
	if len(app.Surfaces()) != 1 || !inspector.closed || !child.closed || app.Find("id:field").Text != "start" {
		t.Fatal("closing a parent left its child open or affected the main window")
	}
}

func TestSurfaceClosedQueriesAndPostsNameTheirWindow(t *testing.T) {
	if Target() != "" {
		t.Skip("checks failure diagnostics in the headless model")
	}
	for _, queuedClose := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "queued close"}[queuedClose], func(t *testing.T) {
			ran := false
			message := fails(t, func(tb testing.TB) {
				app, _, _, window := startSurfaceSteps(tb, core.OSMacOS, true)
				child := app.Surface("title:Child")
				if queuedClose {
					app.Post(window.Close)
				} else {
					app.Surface("title:Inspector").Close()
				}
				child.Post(func() { ran = true })
				app.Settle()
			})
			if ran || !strings.Contains(message, `surface "Child" is closed`) {
				t.Fatalf("closed post ran=%v, message=%q", ran, message)
			}
		})
	}
	message := fails(t, func(tb testing.TB) {
		app, _, _, _ := startSurfaceSteps(tb, core.OSMacOS, false)
		window := app.Surface("title:Inspector")
		window.Close()
		window.Exists("id:field")
	})
	if !strings.Contains(message, `surface "Inspector" is closed`) {
		t.Fatalf("closed query = %q", message)
	}
}

func TestSurfacePostsObserveEarlierNativeClosures(t *testing.T) {
	for _, staleBarrier := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh barrier", true: "earlier barrier"}[staleBarrier], func(t *testing.T) {
			tg, _, _ := engineFixture(t)
			tg.windowed = true
			child := tg.a.openSurface(SurfaceConfig{ID: 7, Title: "Child"}, nil, false)
			child.model.target = &target{a: child.model, owner: tg, surface: noSurface{}}
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			source := tg.surface
			tg.surface = settlingSurface{sync: func() { unblock(); source.Sync() }}
			if staleBarrier {
				// A timed-out earlier step can leave a completed barrier that predates the close. It must be consumed before a fresh one.
				tg.syncing = make(chan struct{})
				close(tg.syncing)
			}
			var ran atomic.Bool
			message := fails(t, func(tb testing.TB) {
				original := tg.a.t
				tg.a.t = tb
				defer func() { tg.a.t = original }()
				tg.a.Post(func() {
					<-release
					tg.mu.Lock()
					tg.updates = append(tg.updates, targetUpdate{surface: 7, batch: &driver.MountBatch{}, closed: true})
					tg.mu.Unlock()
				})
				child.Post(func() { ran.Store(true) })
				tg.runPosts()
				// If the post missed the closure, finish both barriers so the assertion also observes any incorrectly dispatched callback.
				deadline := time.Now().Add(time.Second)
				tg.sync(deadline)
				tg.sync(deadline)
			})
			if ran.Load() || !strings.Contains(message, `surface "Child" is closed`) {
				t.Fatalf("closed native post ran=%v, message=%q", ran.Load(), message)
			}
		})
	}
}
