package nsgotest

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/automation"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// fixture is a counter, a field echoing its text, a disabled button, a menu, a Dock menu and an alert.
func fixture(answered *int) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		count := core.NewSignal(cx, 0)
		name := core.NewSignal(cx, "")
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Reset", OnSelect: func() { count.Set(0) }},
			{Label: "Never", Disabled: true, OnSelect: func() {}},
		}}})
		cx.SetDockMenu([]core.MenuItem{
			{Label: "New Window", OnSelect: func() { count.Set(count.Get() + 1) }},
			{Label: "Later", Disabled: true, OnSelect: func() {}},
		})
		return core.View{Children: []core.Node{
			core.View{Role: core.RoleButton, TestID: "add", OnPress: func(core.PressEvent) { count.Set(count.Get() + 1) },
				Children: []core.Node{core.Text{Content: "Add"}}},
			core.Text{ContentFunc: func() string { return "Count: " + strconv.Itoa(count.Get()) }, TestID: "count"},
			core.Input{TestID: "name", Placeholder: "Name", OnChange: func(s string) { name.Set(s) }},
			core.Text{ContentFunc: func() string { return "Hello, " + name.Get() }},
			core.View{Role: core.RoleButton, TestID: "off", Disabled: func() bool { return true }, OnPress: func(core.PressEvent) {},
				Children: []core.Node{core.Text{Content: "Off"}}},
			core.View{Role: core.RoleButton, TestID: "ask", OnPress: func(core.PressEvent) {
				cx.ShowAlert(core.AlertConfig{Title: "Sure?", Actions: []core.AlertAction{{Label: "Yes"}, {Label: "No", Role: core.AlertCancel}}},
					func(i int) { *answered = i })
			}, Children: []core.Node{core.Text{Content: "Ask"}}},
		}}
	}
}

// serving starts the fixture headless and an agent answering for it as window 1.
func serving(t *testing.T) (*agent, *App, *int) {
	answered := new(int)
	*answered = -2
	app := Start(t, fixture(answered), PendingAlerts())
	g := newAgent()
	t.Cleanup(g.close)
	g.serve(1, app, "Fixture")
	return g, app, answered
}

// call has g answer method with params, failing the test on a transport error, and returns the answer.
func call(t *testing.T, g *agent, method string, params any) automation.Answer {
	t.Helper()
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(automation.Request{ID: 9, Method: method, Params: data})
	var a automation.Answer
	if err := json.Unmarshal(g.Do(req), &a); err != nil {
		t.Fatal(err)
	}
	if a.ID != 9 {
		t.Fatalf("answered request %d", a.ID)
	}
	return a
}

// result has g answer and decodes the result into out, failing on an error.
func result(t *testing.T, g *agent, method string, params, out any) {
	t.Helper()
	a := call(t, g, method, params)
	if a.Error != nil {
		t.Fatalf("%s: %v", method, a.Error)
	}
	if out != nil {
		if err := json.Unmarshal(a.Result, out); err != nil {
			t.Fatal(err)
		}
	}
}

// code returns the error code of an answer that must fail.
func code(t *testing.T, g *agent, method string, params any) automation.Code {
	t.Helper()
	a := call(t, g, method, params)
	if a.Error == nil {
		t.Fatalf("%s %+v succeeded", method, params)
	}
	return a.Error.Code
}

func TestAgentSnapshotsAndFinds(t *testing.T) {
	g, _, _ := serving(t)
	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 1 || ws.Windows[0].ID != 1 || ws.Windows[0].Title != "Fixture" || !ws.Windows[0].Default {
		t.Fatalf("windows %+v", ws)
	}
	var snap automation.SnapshotResult
	result(t, g, automation.Snapshot, automation.SnapshotParams{}, &snap)
	for _, want := range []string{`- button "Add" [view:`, `] id=add`, `- text "Count: 0"`, `- textbox "Name"`, `id=off disabled`} {
		if !strings.Contains(snap.Text, want) {
			t.Errorf("the snapshot lacks %q:\n%s", want, snap.Text)
		}
	}
	if strings.Contains(snap.Text, `- text "Add"`) {
		t.Errorf("the snapshot repeats a button's words:\n%s", snap.Text)
	}
	var all automation.SnapshotResult
	result(t, g, automation.Snapshot, automation.SnapshotParams{All: true}, &all)
	if !strings.Contains(all.Text, "- view [view:") || !strings.Contains(all.Text, `- text "Add"`) {
		t.Errorf("all lists containers and every text:\n%s", all.Text)
	}
	var found automation.FindResult
	result(t, g, automation.Find, automation.FindParams{Selector: "role:button"}, &found)
	if len(found.Elements) != 3 || found.Elements[0].Name != "Add" || found.Elements[0].Bounds == nil {
		t.Fatalf("found %+v", found)
	}
	// A ref names the view alone.
	var one automation.FindResult
	result(t, g, automation.Find, automation.FindParams{Selector: found.Elements[0].Ref}, &one)
	if len(one.Elements) != 1 || one.Elements[0].TestID != "add" {
		t.Fatalf("found by ref %+v", one)
	}
}

// Under nsgo dev -devtools, -highlight or -layout, DevTools' overlay covers the app; the agent lists and finds the app's views alone, as the page's Elements does.
func TestAgentLeavesDevToolsOverlayOut(t *testing.T) {
	answered := new(int)
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.Group{fixture(answered)(cx), core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}}}
	}, PendingAlerts())
	if !app.Exists("id:nsgo-layout-readout") {
		t.Fatalf("the layout audit shows no readout:\n%s", app.Dump())
	}
	g := newAgent()
	t.Cleanup(g.close)
	g.serve(1, app, "Fixture")
	var snap automation.SnapshotResult
	result(t, g, automation.Snapshot, automation.SnapshotParams{All: true}, &snap)
	if strings.Contains(snap.Text, "nsgo-") {
		t.Errorf("the snapshot lists DevTools' overlay:\n%s", snap.Text)
	}
	if !strings.Contains(snap.Text, "id=add") {
		t.Errorf("the snapshot lacks the app:\n%s", snap.Text)
	}
	if c := code(t, g, automation.Find, automation.FindParams{Selector: "id:nsgo-layout-readout"}); c != automation.NoMatch {
		t.Errorf("finding the audit's readout = %v, want no match", c)
	}
	if c := code(t, g, automation.Press, automation.PressParams{Selector: "id:nsgo-layout-readout"}); c != automation.NoMatch {
		t.Errorf("pressing the audit's readout = %v, want no match", c)
	}
	var found automation.FindResult
	result(t, g, automation.Find, automation.FindParams{Selector: "id:add"}, &found)
	if len(found.Elements) != 1 {
		t.Fatalf("found %+v", found)
	}
}

func TestAgentTakesSteps(t *testing.T) {
	g, app, answered := serving(t)
	var r automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Selector: "id:add"}, &r)
	if !r.Settled || !app.Exists("text:Count: 1") {
		t.Fatalf("press: %+v\n%s", r, app.Dump())
	}
	var found automation.FindResult
	result(t, g, automation.Find, automation.FindParams{Selector: "id:add"}, &found)
	result(t, g, automation.Press, automation.PressParams{Selector: found.Elements[0].Ref, Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Count: 2"`) {
		t.Fatalf("press by ref, then the snapshot:\n%s", r.Snapshot)
	}
	result(t, g, automation.Type, automation.TypeParams{Selector: "id:name", Text: "Ada"}, nil)
	if !app.Exists(`text:"Hello, Ada"`) {
		t.Fatalf("type:\n%s", app.Dump())
	}
	result(t, g, automation.Type, automation.TypeParams{Selector: "id:name", Clear: true, Text: "Bo"}, nil)
	if !app.Exists(`text:"Hello, Bo"`) {
		t.Fatalf("clear and type:\n%s", app.Dump())
	}
	result(t, g, automation.Key, automation.KeyParams{Selector: "id:name", Key: "Backspace"}, nil)
	if !app.Exists(`text:"Hello, B"`) {
		t.Fatalf("key:\n%s", app.Dump())
	}
	result(t, g, automation.Menu, automation.MenuParams{Path: []string{"Actions", "Reset"}}, nil)
	if !app.Exists("text:Count: 0") {
		t.Fatalf("menu:\n%s", app.Dump())
	}
	result(t, g, automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Title != "Sure?" || strings.Join(alerts.Alerts[0].Buttons, ",") != "Yes,No" {
		t.Fatalf("alerts %+v", alerts)
	}
	yes := 0
	result(t, g, automation.Alert, automation.AlertParams{Answer: &yes}, &alerts)
	if len(alerts.Alerts) != 0 || *answered != 0 {
		t.Fatalf("answered %d, still showing %+v", *answered, alerts)
	}
	result(t, g, automation.Wait, automation.WaitParams{Selector: "text:Count: 0"}, nil)
	result(t, g, automation.Wait, automation.WaitParams{Selector: "text:Count: 9", Gone: true}, nil)
}

func TestAgentFailures(t *testing.T) {
	g, _, _ := serving(t)
	for _, c := range []struct {
		method string
		params any
		want   automation.Code
	}{
		{automation.Press, automation.PressParams{Selector: "label:/(/"}, automation.InvalidRequest},
		{automation.Press, automation.PressParams{Selector: "id:none"}, automation.NoMatch},
		{automation.Press, automation.PressParams{Selector: "id:off"}, automation.Failed},
		{automation.Press, automation.PressParams{Selector: "id:add", Mode: "twice"}, automation.InvalidRequest},
		{automation.Press, automation.PressParams{Window: 7, Selector: "id:add"}, automation.NoWindow},
		{automation.Menu, automation.MenuParams{Path: []string{"Actions", "Gone"}}, automation.NoMatch},
		{automation.Menu, automation.MenuParams{Path: []string{"Actions", "Never"}}, automation.Failed},
		{automation.Menu, automation.MenuParams{Dock: true, Path: []string{"Reset"}}, automation.NoMatch},
		{automation.Menu, automation.MenuParams{Dock: true, Path: []string{"Later"}}, automation.Failed},
		{automation.Menu, automation.MenuParams{Dock: true, Popup: true, Path: []string{"New Window"}}, automation.InvalidRequest},
		{automation.Wait, automation.WaitParams{Selector: "text:Count: 9", TimeoutMS: 50}, automation.Timeout},
		{automation.Screenshot, automation.ScreenshotParams{}, automation.Unsupported},
		{automation.Status, nil, automation.UnknownMethod},
		{"fly", nil, automation.UnknownMethod},
	} {
		if got := code(t, g, c.method, c.params); got != c.want {
			t.Errorf("%s %+v: %s, want %s", c.method, c.params, got, c.want)
		}
	}
	var a automation.Answer
	if err := json.Unmarshal(g.Do([]byte("{")), &a); err != nil || a.Error == nil || a.Error.Code != automation.InvalidRequest {
		t.Errorf("bad JSON answered %+v", a)
	}
}

// fakeSurface is a surface as DevTools' registry hands it the agent, which follows it.
type fakeSurface struct {
	devtools.Surface
	graph  int
	id     int
	follow devtools.Follower
}

func (s *fakeSurface) ID() int { return s.id }
func (s *fakeSurface) Graph() int {
	if s.graph == 0 {
		return 1
	}
	return s.graph
}
func (s *fakeSurface) Follow(f devtools.Follower) { s.follow = f }

// register has g follow a surface numbered id on graph 1, as core registers each one in a running app.
func register(g *agent, id int) *fakeSurface {
	return registerGraph(g, 1, id)
}

// registerGraph has g follow a surface numbered id on graph.
func registerGraph(g *agent, graph, id int) *fakeSurface {
	s := &fakeSurface{graph: graph, id: id}
	g.Registered(s, true)
	return s
}

// opens has the app's own surface report that it opened window 1.
func opens(own *fakeSurface) {
	opensWindow(own, 1, "Main")
}

// opensWindow has own report that it opened window id with title.
func opensWindow(own *fakeSurface, id uint32, title string) {
	own.follow.Batch(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemOpenSurface, Surface: &driver.SurfaceConfig{ID: id, Title: title}}}})
}

// fakeLive is a running app's window backed by a headless tree, which reports to the agent as core does in a running app.
type fakeLive struct {
	noSurface
	surface *fakeSurface
	tree    *core.Tree
	pngs    int
	sizes   []driver.Size
}

func (f *fakeLive) commit() {
	if b := f.tree.Commit(); b != nil {
		f.surface.follow.Batch(b)
	}
}

func (f *fakeLive) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	f.tree.PressAt(driver.Event{Page: at, Count: 1, Modifiers: mods})
	f.commit()
	return nil
}

func (f *fakeLive) Screenshot() ([]byte, error) {
	f.pngs++
	// A 1 by 1 PNG.
	return []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82"), nil
}

func TestAgentFollowsALiveSurface(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
	t.Cleanup(root.Dispose)
	tree := core.NewTree(root, core.ApproxTextEngine{})
	tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
	answered := new(int)
	tree.Mount(fixture(answered))
	own := register(g, 0)
	live := &fakeLive{surface: register(g, 1), tree: tree}
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if graph == 1 && id == 1 {
			return live
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })
	opens(own)
	live.commit()
	// Batches the agent was sent are taken on its own while no request runs.
	deadline := time.Now().Add(2 * time.Second)
	for {
		g.mu.Lock()
		f := g.surfaces[surfaceKey{graph: 1, id: 1}]
		g.mu.Unlock()
		f.app.target.mu.Lock()
		left := len(f.app.target.updates)
		f.app.target.mu.Unlock()
		if left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the agent left batches untaken while idle")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 1 || ws.Windows[0].Title != "Main" || ws.Windows[0].Width != 300 {
		t.Fatalf("windows %+v", ws)
	}
	var r automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Selector: "id:add", Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Count: 1"`) {
		t.Fatalf("a live press:\n%s", r.Snapshot)
	}
	var shot automation.ScreenshotResult
	result(t, g, automation.Screenshot, automation.ScreenshotParams{}, &shot)
	if shot.Width != 1 || live.pngs != 1 {
		t.Fatalf("screenshot %dx%d", shot.Width, shot.Height)
	}
	// An alert the user answers on the platform leaves the model.
	result(t, g, automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 {
		t.Fatalf("alerts %+v", alerts)
	}
	live.surface.follow.Event(driver.Event{Kind: driver.EventAlert, Count: alerts.Alerts[0].ID, Index: 0})
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 0 {
		t.Fatalf("an answered alert still shows: %+v", alerts)
	}
	g.Unregistered(live.surface)
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 0 {
		t.Fatalf("a window gone is listed: %+v", ws)
	}
}

// TestAgentFollowsTheRegistry has core register multiple running graphs' surfaces with the agent, as nsgo dev -automation's builds do, and checks that it follows every graph and tells their surfaces apart even though each graph numbers its windows from 1.
func TestAgentFollowsTheRegistry(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	devtools.Enabled, devtools.Automation = true, g
	t.Cleanup(func() { devtools.Enabled, devtools.Automation = false, nil })
	start := func(title, text string) int {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild, Windows: &core.WindowOptions{Width: 300, Height: 200}})
		t.Cleanup(root.Dispose)
		app := core.NewTree(root, core.ApproxTextEngine{})
		// Linux lays out the app-level surface at a nonzero size even when hidden.
		app.Resize(driver.Size{Width: 480, Height: 640}, driver.Geometry{})
		app.Mount(core.AppRoot(func(cx *core.Scope) {
			cx.OpenWindow(core.WindowOptions{Title: title, Width: 300, Height: 200}, func(cx *core.Scope) core.Node { return core.Text{Content: text} })
		}))
		for _, tree := range append([]*core.Tree{app}, app.Surfaces()...) {
			tree.Commit()
		}
		return app.Graph()
	}
	g1 := start("First", "one")
	g2 := start("Second", "two")
	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 {
		t.Fatalf("windows %+v, want 2", ws)
	}
	if ws.Windows[0].Title != "First" || ws.Windows[0].Graph != g1 || ws.Windows[0].Default ||
		ws.Windows[1].Title != "Second" || ws.Windows[1].Graph != g2 || !ws.Windows[1].Default ||
		ws.Windows[0].ID == 0 || ws.Windows[1].ID == 0 || ws.Windows[0].ID == ws.Windows[1].ID {
		t.Fatalf("windows %+v (graphs %d, %d)", ws, g1, g2)
	}
	var snap automation.SnapshotResult
	// Default targets the newest window (Second).
	result(t, g, automation.Snapshot, automation.SnapshotParams{}, &snap)
	if snap.Window != ws.Windows[1].ID || !strings.Contains(snap.Text, `"two"`) || strings.Contains(snap.Text, `"one"`) {
		t.Fatalf("default snapshot (window %d):\n%s", snap.Window, snap.Text)
	}
	// Naming the first graph's window inspects it alone.
	result(t, g, automation.Snapshot, automation.SnapshotParams{Window: ws.Windows[0].ID}, &snap)
	if snap.Window != ws.Windows[0].ID || !strings.Contains(snap.Text, `"one"`) || strings.Contains(snap.Text, `"two"`) {
		t.Fatalf("first graph snapshot (window %d):\n%s", snap.Window, snap.Text)
	}
}

// TestAgentWaitsForTheFirstWindow asks a starting app, which connects before its window opens, for its windows.
func TestAgentWaitsForTheFirstWindow(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	own := register(g, 0)
	opened := make(chan struct{})
	finished, disposed := make(chan struct{}), make(chan struct{})
	defer func() { close(finished); <-disposed }()
	go func() {
		defer close(disposed)
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		defer root.Dispose()
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
		tree.Mount(fixture(new(int)))
		time.Sleep(300 * time.Millisecond)
		live := &fakeLive{surface: register(g, 1), tree: tree}
		opens(own)
		live.commit()
		close(opened)
		<-finished
	}()
	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	<-opened
	if len(ws.Windows) != 1 || ws.Windows[0].Title != "Main" || ws.Windows[0].Width != 300 {
		t.Fatalf("the first request listed %+v", ws)
	}
}

// chooser is a live surface that records the menu items it chose and the alerts it answered.
type chooser struct {
	*fakeLive
	chose, answered, docked []uint32
}

func (c *chooser) ChooseMenu(id uint32) error {
	c.chose = append(c.chose, id)
	return nil
}

func (c *chooser) Answer(id uint32, index int) error {
	c.answered = append(c.answered, id)
	return nil
}

func (c *chooser) ChooseDockMenu(id uint32) error {
	c.docked = append(c.docked, id)
	return nil
}

// TestAgentChoosesDockMenu chooses in the app icon's menu of nsgotest's own app, which its model holds whichever window set it.
func TestAgentChoosesDockMenu(t *testing.T) {
	g, app, _ := serving(t)
	result(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"New Window"}}, nil)
	app.Find("text:Count: 1")
}

// TestAgentChoosesTheAppsDockMenu follows the app's own surface and a window, whose items are numbered alike: the Dock menu is the one a surface set last, chosen through that surface whichever window the request names, and none once that surface goes, as the platform drops it.
func TestAgentChoosesTheAppsDockMenu(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	trees := map[uint32]*chooser{}
	var library func()
	for _, id := range []uint32{0, 1} {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
		tree.Mount(func(cx *core.Scope) core.Node {
			if id == 0 {
				cx.SetDockMenu([]core.MenuItem{{Label: "New Window", OnSelect: func() {}}})
				return core.Text{Content: "app"}
			}
			shown := core.NewSignal(cx, false)
			library = func() { shown.Set(true) }
			return core.Show{When: shown.Get, Then: func(cx *core.Scope) core.Node {
				cx.SetDockMenu([]core.MenuItem{{Label: "Next Library", OnSelect: func() {}}})
				return core.Text{Content: "library"}
			}, Else: func(*core.Scope) core.Node { return core.Text{Content: "window"} }}
		})
		trees[id] = &chooser{fakeLive: &fakeLive{surface: register(g, int(id)), tree: tree}}
	}
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if graph == 1 {
			if c := trees[id]; c != nil {
				return c
			}
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })
	opens(trees[0].surface)
	trees[0].commit()
	trees[1].commit()

	// The app's function set it, so the app's own surface chooses it, though the request names the window.
	result(t, g, automation.Menu, automation.MenuParams{Window: 1, Dock: true, Path: []string{"New Window"}}, nil)
	if len(trees[0].docked) != 1 || len(trees[1].docked) != 0 {
		t.Fatalf("the app chose %v and the window %v", trees[0].docked, trees[1].docked)
	}
	// The window's replaces it.
	library()
	trees[1].commit()
	if got := code(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"New Window"}}); got != automation.NoMatch {
		t.Errorf("the replaced Dock menu answered %s", got)
	}
	result(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"Next Library"}}, nil)
	if len(trees[0].docked) != 1 || len(trees[1].docked) != 1 || trees[0].docked[0] != trees[1].docked[0] {
		t.Fatalf("the app chose %v and the window %v", trees[0].docked, trees[1].docked)
	}
	// The window goes, and its menu with it.
	g.Unregistered(trees[1].surface)
	if got := code(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"Next Library"}}); got != automation.NoMatch {
		t.Errorf("the Dock menu of a window gone answered %s", got)
	}
}

// TestAgentTellsSurfacesApart follows the app's own surface and a window, whose menu items and alerts are numbered alike, as each tree numbers its own.
func TestAgentTellsSurfacesApart(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	trees := map[uint32]*chooser{}
	for _, id := range []uint32{0, 1} {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
		menu := []core.MenuItem{{Label: "Actions", Items: []core.MenuItem{{Label: "Reset", OnSelect: func() {}}}}}
		if id == 1 {
			menu = []core.MenuItem{{Label: "Edit", Items: []core.MenuItem{{Label: "Delete", OnSelect: func() {}}}}}
		}
		tree.Mount(func(cx *core.Scope) core.Node {
			cx.SetApplicationMenu(menu)
			cx.ShowAlert(core.AlertConfig{Title: "Sure?", Actions: []core.AlertAction{{Label: "Yes"}}}, func(int) {})
			return core.Text{Content: "surface " + strconv.Itoa(int(id))}
		})
		trees[id] = &chooser{fakeLive: &fakeLive{surface: register(g, int(id)), tree: tree}}
	}
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if graph == 1 {
			if c := trees[id]; c != nil {
				return c
			}
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })
	opens(trees[0].surface)
	trees[0].commit()
	trees[1].commit()

	// The app's own item is chosen by the app's own surface, even from the window, whose own item has its ID.
	result(t, g, automation.Menu, automation.MenuParams{Window: 1, Path: []string{"Actions", "Reset"}}, nil)
	result(t, g, automation.Menu, automation.MenuParams{Window: 1, Path: []string{"Edit", "Delete"}}, nil)
	if len(trees[0].chose) != 1 || len(trees[1].chose) != 1 || trees[0].chose[0] != trees[1].chose[0] {
		t.Fatalf("the app chose %v and the window %v", trees[0].chose, trees[1].chose)
	}

	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 2 || alerts.Alerts[0].Window != 0 || alerts.Alerts[1].Window != 1 || alerts.Alerts[0].ID != alerts.Alerts[1].ID {
		t.Fatalf("alerts %+v", alerts)
	}
	own := uint32(0)
	result(t, g, automation.Alert, automation.AlertParams{Window: &own}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window != 0 {
		t.Fatalf("the app's own alerts %+v", alerts)
	}
	yes, id := 0, alerts.Alerts[0].ID
	if got := code(t, g, automation.Alert, automation.AlertParams{ID: id, Answer: &yes}); got != automation.InvalidRequest {
		t.Errorf("an alert ID two windows show answered %s", got)
	}
	window := uint32(1)
	result(t, g, automation.Alert, automation.AlertParams{Window: &window, ID: id, Answer: &yes}, &alerts)
	if len(trees[1].answered) != 1 || len(trees[0].answered) != 0 || len(alerts.Alerts) != 0 {
		t.Fatalf("answered the app's %v and the window's %v, then %+v", trees[0].answered, trees[1].answered, alerts)
	}
}

func TestAgentLosesAModelRatherThanTheApp(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
	t.Cleanup(root.Dispose)
	tree := core.NewTree(root, core.ApproxTextEngine{})
	tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
	tree.Mount(fixture(new(int)))
	own := register(g, 0)
	live := &fakeLive{surface: register(g, 1), tree: tree}
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if graph == 1 && id == 1 {
			return live
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })
	opens(own)
	live.commit()
	result(t, g, automation.Snapshot, automation.SnapshotParams{Window: 1}, nil)
	// A frame for a view the model never saw.
	live.surface.follow.Batch(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemFrame, ID: 1 << 30}}})
	for _, c := range []struct {
		method string
		params any
	}{
		{automation.Screenshot, automation.ScreenshotParams{Window: 1}},
		{automation.Press, automation.PressParams{Window: 1, Selector: "id:add"}},
	} {
		a := call(t, g, c.method, c.params)
		if a.Error == nil || a.Error.Code != automation.Failed || !strings.Contains(a.Error.Message, "stopped following") {
			t.Errorf("%s on a lost model: %+v", c.method, a.Error)
		}
	}
	g.mu.Lock()
	f := g.surfaces[surfaceKey{graph: 1, id: 1}]
	g.mu.Unlock()
	live.surface.follow.Batch(&driver.MountBatch{Root: 1})
	live.surface.follow.Event(driver.Event{Kind: driver.EventFocus, ID: 1})
	f.app.target.mu.Lock()
	queued := len(f.app.target.updates)
	f.app.target.mu.Unlock()
	if queued != 0 {
		t.Errorf("a lost model still queues %d batches and events", queued)
	}
}
func TestAgentFollowsAppAndScenes(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
	t.Cleanup(root.Dispose)
	makeScene := func(id int, width, height driver.Points) *fakeLive {
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: width, Height: height}, driver.Geometry{})
		tree.Mount(fixture(new(int)))
		return &fakeLive{surface: register(g, id), tree: tree}
	}
	first := makeScene(0, 390, 844)
	second := makeScene(1, 500, 700)
	testhost.RegisterLive(func(_ int, id uint32) testhost.Surface {
		switch id {
		case 0:
			return first
		case 1:
			return second
		default:
			return nil
		}
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })
	b := first.tree.Commit()
	first.surface.follow.Batch(b)
	first.surface.follow.Batch(&driver.MountBatch{Root: b.Root, Items: []driver.MountItem{{Kind: driver.ItemOpenSurface, Surface: &driver.SurfaceConfig{ID: 1, Kind: driver.SurfaceScene, Scene: 1}}}})
	second.commit()

	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 || ws.Windows[0].ID <= 0 || ws.Windows[0].Kind != "app" || ws.Windows[1].ID <= 0 || ws.Windows[1].ID == ws.Windows[0].ID || ws.Windows[1].Kind != "scene" || !ws.Windows[1].Default {
		t.Fatalf("windows %+v", ws)
	}
	var r automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Window: ws.Windows[0].ID, Selector: "id:add", Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Count: 1"`) {
		t.Fatalf("first scene press:\n%s", r.Snapshot)
	}
	result(t, g, automation.Press, automation.PressParams{Window: ws.Windows[1].ID, Selector: "id:add", Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Count: 1"`) {
		t.Fatalf("second scene press:\n%s", r.Snapshot)
	}
}

// deviceLive models a physical Apple device live surface without simulator HID:
// presses dispatch via accessibility activate on views that declare ActionActivate,
// or via the tree's hit testing for EventPress and EventLongPress, and alerts
// dispatch EventAlert directly. All tree work runs on the graph's owner goroutine.
type deviceLive struct {
	*fakeLive
	model         *App
	work          chan func()
	axActivations int
	longPresses   int
}

func (d *deviceLive) onTree(fn func()) {
	done := make(chan struct{})
	d.work <- func() {
		defer close(done)
		fn()
	}
	<-done
}

func (d *deviceLive) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	var err error
	d.onTree(func() {
		if hit := d.tree.HitTest(at); hit != 0 && d.model != nil {
			for v := d.model.views[hit]; v != nil; v = v.Parent {
				if hold == 0 && mods == (driver.Modifiers{}) && v.Props.HasAction(driver.ActionActivate) && v.Events.Has(driver.EventAccessibilityAction) {
					d.axActivations++
					d.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: v.ID, Text: driver.ActionActivate})
					d.commit()
					return
				}
				if hold > 0 && v.Events.Has(driver.EventLongPress) {
					d.longPresses++
					d.tree.Dispatch(driver.Event{Kind: driver.EventLongPress, ID: v.ID, Page: at, Modifiers: mods})
					d.commit()
					return
				}
			}
		}
		err = d.fakeLive.Press(at, hold, mods)
	})
	return err
}

func (d *deviceLive) Answer(id uint32, index int) error {
	d.onTree(func() {
		ev := driver.Event{Kind: driver.EventAlert, Count: id, Index: index}
		d.tree.Dispatch(ev)
		d.surface.follow.Event(ev)
		d.commit()
	})
	return nil
}

func TestAgentDrivesDevicePressAndAccessibilityActions(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	work := make(chan func())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for fn := range work {
			fn()
		}
	}()
	t.Cleanup(func() {
		close(work)
		<-stopped
	})
	live := &deviceLive{fakeLive: &fakeLive{surface: register(g, 0)}, work: work}
	g.mu.Lock()
	live.model = g.surfaces[surfaceKey{graph: 1, id: 0}].app
	g.mu.Unlock()
	var dispose func()
	live.onTree(func() {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		dispose = root.Dispose
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: 390, Height: 844}, driver.Geometry{})
		tree.Mount(func(cx *core.Scope) core.Node {
			taps := core.NewSignal(cx, 0)
			ax := core.NewSignal(cx, 0)
			held := core.NewSignal(cx, 0)
			ans := core.NewSignal(cx, -2)
			return core.View{Style: core.Style{FlexGrow: 1, Gap: 8}, Children: []core.Node{
				core.View{Role: core.RoleButton, TestID: "tap", OnPress: func(core.PressEvent) { taps.Set(taps.Get() + 1) },
					OnLongPress: func(core.PressEvent) { held.Set(held.Get() + 1) },
					Children:    []core.Node{core.Text{Content: "Tap"}}},
				core.View{Role: core.RoleButton, TestID: "ax",
					OnPress:              func(core.PressEvent) {},
					AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionActivate}},
					OnAccessibilityAction: func(name string) {
						if name == core.ActionActivate {
							ax.Set(ax.Get() + 1)
						}
					},
					Children: []core.Node{core.Text{Content: "Activate"}}},
				core.View{Role: core.RoleButton, TestID: "ask", OnPress: func(core.PressEvent) {
					cx.ShowAlert(core.AlertConfig{Title: "Proceed?", Actions: []core.AlertAction{{Label: "OK"}, {Label: "Cancel", Role: core.AlertCancel}}},
						func(i int) { ans.Set(i) })
				}, Children: []core.Node{core.Text{Content: "Ask"}}},
				core.Text{ContentFunc: func() string { return "Taps: " + strconv.Itoa(taps.Get()) }},
				core.Text{ContentFunc: func() string { return "AX: " + strconv.Itoa(ax.Get()) }},
				core.Text{ContentFunc: func() string { return "Held: " + strconv.Itoa(held.Get()) }},
				core.Text{ContentFunc: func() string { return "Ans: " + strconv.Itoa(ans.Get()) }},
			}}
		})
		live.tree = tree
		live.commit()
	})
	t.Cleanup(func() { live.onTree(dispose) })
	testhost.RegisterLive(func(_ int, id uint32) testhost.Surface {
		if id == 0 {
			return live
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })

	var r automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Selector: "id:tap", Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Taps: 1"`) {
		t.Fatalf("device hit-tested tap:\n%s", r.Snapshot)
	}
	result(t, g, automation.Press, automation.PressParams{Selector: "id:ax", Snapshot: true}, &r)
	if live.axActivations != 1 || !strings.Contains(r.Snapshot, `"AX: 1"`) {
		t.Fatalf("device accessibility activate (count=%d):\n%s", live.axActivations, r.Snapshot)
	}
	result(t, g, automation.Press, automation.PressParams{Selector: "id:tap", Mode: "long", Snapshot: true}, &r)
	if live.longPresses != 1 || !strings.Contains(r.Snapshot, `"Held: 1"`) {
		t.Fatalf("device long press (count=%d):\n%s", live.longPresses, r.Snapshot)
	}
	result(t, g, automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 {
		t.Fatalf("alerts = %+v", alerts)
	}
	okIdx := 0
	result(t, g, automation.Alert, automation.AlertParams{ID: alerts.Alerts[0].ID, Answer: &okIdx}, &alerts)
	var snap automation.SnapshotResult
	result(t, g, automation.Snapshot, automation.SnapshotParams{}, &snap)
	if len(alerts.Alerts) != 0 || !strings.Contains(snap.Text, `"Ans: 0"`) {
		t.Fatalf("device alert answer (alerts=%+v):\n%s", alerts, snap.Text)
	}
}

// embeddedEngine runs a real core.Root and core.Tree on its own goroutine, like an embedded nsgo.NewView's engine, registering through devtools.Enabled and answering live testhost.Surface steps on that goroutine.
type embeddedEngine struct {
	noSurface
	graph   int
	width   int
	pngs    int
	post    chan func()
	stop    chan struct{}
	exited  chan struct{}
	focused driver.NodeID
	text    string
	edit    uint32
	alerts  map[uint32]bool
	mu      sync.Mutex
	root    *core.Scope
	tree    *core.Tree
	closeMu sync.Mutex
	closed  bool
}

func startEmbeddedEngine(t *testing.T, label string, shotWidth int, answered *int, readyText *core.Signal[string]) *embeddedEngine {
	t.Helper()
	e := &embeddedEngine{
		width:  shotWidth,
		post:   make(chan func(), 64),
		stop:   make(chan struct{}),
		exited: make(chan struct{}),
		alerts: map[uint32]bool{},
	}
	started := make(chan int, 1)
	go func() {
		defer close(e.exited)
		root := core.NewRoot(core.RootOptions{
			CheckCommits: !automationBuild,
			Post: func(fn func()) {
				select {
				case e.post <- fn:
				case <-e.stop:
				}
			},
		})
		defer root.Dispose()
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: 320, Height: 220}, driver.Geometry{})
		tree.Mount(func(cx *core.Scope) core.Node {
			extra := core.NewSignal(cx, "")
			if readyText != nil {
				*readyText = *extra
			}
			return core.View{Children: []core.Node{
				core.Text{Content: "Engine: " + label, TestID: "engine"},
				fixture(answered)(cx),
			}}
		})
		e.root, e.tree = root, tree
		e.commit()
		started <- tree.Graph()
		for {
			select {
			case fn := <-e.post:
				fn()
				e.commit()
			case <-e.stop:
				return
			}
		}
	}()
	e.graph = <-started
	t.Cleanup(e.close)
	return e
}

func (e *embeddedEngine) run(fn func()) {
	done := make(chan struct{})
	select {
	case e.post <- func() {
		defer close(done)
		fn()
	}:
	case <-e.exited:
		return
	}
	select {
	case <-done:
	case <-e.exited:
	}
}

func (e *embeddedEngine) commit() {
	b := e.tree.Commit()
	if b == nil {
		return
	}
	e.mu.Lock()
	for _, it := range b.Items {
		if it.Kind == driver.ItemAlert && it.Alert != nil {
			e.alerts[it.Alert.ID] = true
		}
	}
	e.mu.Unlock()
}

func (e *embeddedEngine) close() {
	e.closeMu.Lock()
	if e.closed {
		e.closeMu.Unlock()
		return
	}
	e.closed = true
	close(e.stop)
	e.closeMu.Unlock()
	<-e.exited
}

func (e *embeddedEngine) Sync() { e.run(func() {}) }

func (e *embeddedEngine) Press(at driver.Point, _ time.Duration, mods driver.Modifiers) error {
	e.run(func() {
		if hit := e.tree.HitTest(at); hit != 0 {
			if e.focused != hit {
				if e.focused != 0 {
					e.tree.Dispatch(driver.Event{Kind: driver.EventBlur, ID: e.focused})
				}
				e.focused = hit
				e.tree.Dispatch(driver.Event{Kind: driver.EventFocus, ID: hit})
			}
		}
		e.tree.PressAt(driver.Event{Page: at, Count: 1, Modifiers: mods})
	})
	return nil
}

func (e *embeddedEngine) Type(s string) error {
	e.run(func() {
		if e.focused == 0 {
			return
		}
		e.edit++
		e.text += s
		e.tree.Dispatch(driver.Event{Kind: driver.EventChangeText, ID: e.focused, Text: e.text, Count: e.edit})
	})
	return nil
}

func (e *embeddedEngine) Key(key string) error {
	e.run(func() {
		if e.focused == 0 {
			return
		}
		if strings.EqualFold(key, "Backspace") && len(e.text) > 0 {
			e.edit++
			e.text = e.text[:len(e.text)-1]
			e.tree.Dispatch(driver.Event{Kind: driver.EventChangeText, ID: e.focused, Text: e.text, Count: e.edit})
		}
	})
	return nil
}

func (e *embeddedEngine) Answer(id uint32, index int) error {
	e.run(func() {
		e.mu.Lock()
		delete(e.alerts, id)
		e.mu.Unlock()
		e.tree.Dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: index})
	})
	return nil
}

func (e *embeddedEngine) Screenshot() ([]byte, error) {
	e.mu.Lock()
	e.pngs++
	w := e.width
	e.mu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, w, 1))
	img.Set(0, 0, color.RGBA{R: uint8(w * 20), A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// TestAgentFollowsSeveralEmbeddedEngines runs three concurrent embedded engines (each surface 0 in its own graph, as a host calling nsgo.NewView has) and checks that the agent lists, inspects, drives, screenshots and cleanly detaches each one without colliding surface IDs or leaking events across engines.
func TestAgentFollowsSeveralEmbeddedEngines(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	devtools.Enabled, devtools.Automation = true, g
	t.Cleanup(func() { devtools.Enabled, devtools.Automation = false, nil })

	var liveMu sync.Mutex
	live := map[int]*embeddedEngine{}
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if id != 0 {
			return nil
		}
		liveMu.Lock()
		defer liveMu.Unlock()
		if e := live[graph]; e != nil {
			return e
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })

	ans1, ans2, ans3 := new(int), new(int), new(int)
	*ans1, *ans2, *ans3 = -2, -2, -2
	e1 := startEmbeddedEngine(t, "Alpha", 2, ans1, nil)
	e2 := startEmbeddedEngine(t, "Beta", 4, ans2, nil)
	e3 := startEmbeddedEngine(t, "Gamma", 6, ans3, nil)
	liveMu.Lock()
	live[e1.graph], live[e2.graph], live[e3.graph] = e1, e2, e3
	liveMu.Unlock()

	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 3 {
		t.Fatalf("windows %+v, want 3 embedded views", ws)
	}
	byGraph := map[int]automation.Window{}
	seenIDs := map[uint32]bool{}
	for i, w := range ws.Windows {
		if w.ID == 0 || seenIDs[w.ID] {
			t.Fatalf("window %d has non-unique or zero ID: %+v", i, ws.Windows)
		}
		seenIDs[w.ID] = true
		if w.Kind != "app" || w.Width != 320 || w.Height != 220 {
			t.Fatalf("window %d: %+v", i, w)
		}
		if wantDefault := i == len(ws.Windows)-1; w.Default != wantDefault {
			t.Fatalf("window %d Default=%v, want %v (%+v)", i, w.Default, wantDefault, ws.Windows)
		}
		byGraph[w.Graph] = w
	}
	w1, w2, w3 := byGraph[e1.graph].ID, byGraph[e2.graph].ID, byGraph[e3.graph].ID
	if w1 == 0 || w2 == 0 || w3 == 0 {
		t.Fatalf("missing graph in windows: %+v", ws.Windows)
	}

	// Default snapshot (window omitted / 0) hits the Default window, never colliding with ID 0.
	var snap automation.SnapshotResult
	result(t, g, automation.Snapshot, automation.SnapshotParams{}, &snap)
	if snap.Window != ws.Windows[2].ID {
		t.Fatalf("default snapshot picked window %d, want %d", snap.Window, ws.Windows[2].ID)
	}
	// Each embedded engine's snapshot shows only its own tree.
	for _, tc := range []struct {
		win  uint32
		want string
		not  string
	}{
		{w1, `"Engine: Alpha"`, `"Engine: Beta"`},
		{w2, `"Engine: Beta"`, `"Engine: Gamma"`},
		{w3, `"Engine: Gamma"`, `"Engine: Alpha"`},
	} {
		result(t, g, automation.Snapshot, automation.SnapshotParams{Window: tc.win}, &snap)
		if snap.Window != tc.win || !strings.Contains(snap.Text, tc.want) || strings.Contains(snap.Text, tc.not) {
			t.Fatalf("snapshot window %d:\n%s", tc.win, snap.Text)
		}
	}

	// Driving steps on Engine 1 and Engine 3 independently does not interfere with Engine 2 or each other.
	var step automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Window: w1, Selector: "id:add", Snapshot: true}, &step)
	if !strings.Contains(step.Snapshot, `"Count: 1"`) {
		t.Fatalf("engine 1 after press:\n%s", step.Snapshot)
	}
	result(t, g, automation.Press, automation.PressParams{Window: w3, Selector: "id:add"}, nil)
	result(t, g, automation.Press, automation.PressParams{Window: w3, Selector: "id:add", Snapshot: true}, &step)
	if !strings.Contains(step.Snapshot, `"Count: 2"`) {
		t.Fatalf("engine 3 after two presses:\n%s", step.Snapshot)
	}
	result(t, g, automation.Snapshot, automation.SnapshotParams{Window: w2}, &snap)
	if !strings.Contains(snap.Text, `"Count: 0"`) {
		t.Fatalf("engine 2 changed when engines 1 and 3 were pressed:\n%s", snap.Text)
	}

	// Typing and key presses isolate per engine.
	result(t, g, automation.Type, automation.TypeParams{Window: w1, Selector: "id:name", Text: "Ada"}, nil)
	result(t, g, automation.Type, automation.TypeParams{Window: w2, Selector: "id:name", Text: "Grace"}, nil)
	result(t, g, automation.Key, automation.KeyParams{Window: w2, Selector: "id:name", Key: "Backspace", Snapshot: true}, &step)
	if !strings.Contains(step.Snapshot, `"Hello, Grac"`) {
		t.Fatalf("engine 2 after type+backspace:\n%s", step.Snapshot)
	}
	result(t, g, automation.Snapshot, automation.SnapshotParams{Window: w1}, &snap)
	if !strings.Contains(snap.Text, `"Hello, Ada"`) {
		t.Fatalf("engine 1 text affected by engine 2:\n%s", snap.Text)
	}

	// Screenshots route to the exact engine's live surface.
	for _, tc := range []struct {
		win   uint32
		width int
		eng   *embeddedEngine
	}{
		{w1, 2, e1},
		{w2, 4, e2},
		{w3, 6, e3},
	} {
		var shot automation.ScreenshotResult
		result(t, g, automation.Screenshot, automation.ScreenshotParams{Window: tc.win}, &shot)
		if shot.Width != tc.width || tc.eng.pngs != 1 {
			t.Fatalf("window %d screenshot width=%d pngs=%d, want %d and 1", tc.win, shot.Width, tc.eng.pngs, tc.width)
		}
	}

	// Simultaneous alerts on Engine 1 and Engine 2 share alert ID 1 but have distinct automation window IDs.
	result(t, g, automation.Press, automation.PressParams{Window: w1, Selector: "id:ask"}, nil)
	result(t, g, automation.Press, automation.PressParams{Window: w2, Selector: "id:ask"}, nil)
	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 2 || alerts.Alerts[0].ID != alerts.Alerts[1].ID || alerts.Alerts[0].Window == alerts.Alerts[1].Window {
		t.Fatalf("alerts across engines: %+v", alerts)
	}
	yes := 0
	if got := code(t, g, automation.Alert, automation.AlertParams{ID: alerts.Alerts[0].ID, Answer: &yes}); got != automation.InvalidRequest {
		t.Fatalf("ambiguous alert across engines answered %s, want invalid_request", got)
	}
	// Answer Engine 1's alert via the automation API; Engine 2's stays open.
	result(t, g, automation.Alert, automation.AlertParams{Window: &w1, ID: alerts.Alerts[0].ID, Answer: &yes}, &alerts)
	if *ans1 != 0 || *ans2 != -2 || len(alerts.Alerts) != 0 {
		t.Fatalf("after answering engine 1: ans1=%d ans2=%d remaining=%+v", *ans1, *ans2, alerts)
	}
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window != w2 {
		t.Fatalf("engine 2's alert should still show: %+v", alerts)
	}
	// Platform-dispatched EventAlert on Engine 2 clears only Engine 2's alert.
	e2.Answer(alerts.Alerts[0].ID, 1)
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 0 || *ans2 != 1 {
		t.Fatalf("after engine 2 platform alert answer: ans2=%d alerts=%+v", *ans2, alerts)
	}

	// Clean detach/shutdown of Engine 2 leaves Engines 1 and 3 running with their original IDs intact.
	e2.close()
	liveMu.Lock()
	delete(live, e2.graph)
	liveMu.Unlock()
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 || ws.Windows[0].ID != w1 || ws.Windows[1].ID != w3 || !ws.Windows[1].Default {
		t.Fatalf("after closing engine 2, windows=%+v, want [%d, %d]", ws.Windows, w1, w3)
	}
	if got := code(t, g, automation.Snapshot, automation.SnapshotParams{Window: w2}); got != automation.NoWindow {
		t.Fatalf("closed engine 2 snapshot = %s, want no_window", got)
	}
	result(t, g, automation.Press, automation.PressParams{Window: w1, Selector: "id:add", Snapshot: true}, &step)
	if !strings.Contains(step.Snapshot, `"Count: 2"`) {
		t.Fatalf("engine 1 after engine 2 shutdown:\n%s", step.Snapshot)
	}

	// Closing Engine 3 (the current default) shifts Default to Engine 1.
	e3.close()
	liveMu.Lock()
	delete(live, e3.graph)
	liveMu.Unlock()
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 1 || ws.Windows[0].ID != w1 || !ws.Windows[0].Default {
		t.Fatalf("after closing engine 3, windows=%+v, want [%d default]", ws.Windows, w1)
	}
	result(t, g, automation.Snapshot, automation.SnapshotParams{}, &snap)
	if snap.Window != w1 || !strings.Contains(snap.Text, `"Engine: Alpha"`) {
		t.Fatalf("default snapshot after engine 3 closed: window=%d\n%s", snap.Window, snap.Text)
	}

	// Starting a fourth engine allocates a fresh monotonic ID without reusing closed IDs.
	ans4 := new(int)
	*ans4 = -2
	e4 := startEmbeddedEngine(t, "Delta", 8, ans4, nil)
	liveMu.Lock()
	live[e4.graph] = e4
	liveMu.Unlock()
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 || ws.Windows[0].ID != w1 || ws.Windows[1].Graph != e4.graph || !ws.Windows[1].Default {
		t.Fatalf("after starting engine 4, windows=%+v", ws.Windows)
	}
	w4 := ws.Windows[1].ID
	if w4 == w1 || w4 == w2 || w4 == w3 {
		t.Fatalf("engine 4 reused an earlier window ID %d (previous: %d, %d, %d)", w4, w1, w2, w3)
	}
	result(t, g, automation.Wait, automation.WaitParams{Window: w4, Selector: `text:"Engine: Delta"`}, nil)
}

// TestAgentIsolatesSeveralEnginesMenusAndAlerts runs two windowing graphs (each with its own windowless surface 0 and opened window 1) and checks that menus fall back only to their own graph's surface 0 and alerts on all four surfaces are addressed independently.
func TestAgentIsolatesSeveralEnginesMenusAndAlerts(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)

	type key struct {
		graph int
		id    uint32
	}
	trees := map[key]*chooser{}
	makeSurface := func(graph int, id uint32, menuLabel, itemLabel, alertTitle string) *chooser {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, core.ApproxTextEngine{})
		if id != 0 {
			tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
		}
		tree.Mount(func(cx *core.Scope) core.Node {
			cx.SetApplicationMenu([]core.MenuItem{{Label: menuLabel, Items: []core.MenuItem{{Label: itemLabel, OnSelect: func() {}}}}})
			cx.ShowAlert(core.AlertConfig{Title: alertTitle, Actions: []core.AlertAction{{Label: "OK"}}}, func(int) {})
			return core.Text{Content: alertTitle}
		})
		c := &chooser{fakeLive: &fakeLive{surface: registerGraph(g, graph, int(id)), tree: tree}}
		trees[key{graph, id}] = c
		return c
	}

	g1Own := makeSurface(1, 0, "App1", "Reset1", "G1 Own Alert")
	g1Win := makeSurface(1, 1, "Win1", "Edit1", "G1 Win Alert")
	g2Own := makeSurface(2, 0, "App2", "Reset2", "G2 Own Alert")
	g2Win := makeSurface(2, 1, "Win2", "Edit2", "G2 Win Alert")

	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if c := trees[key{graph, id}]; c != nil {
			return c
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })

	opensWindow(g1Own.surface, 1, "Graph 1")
	opensWindow(g2Own.surface, 1, "Graph 2")
	g1Own.commit()
	g1Win.commit()
	g2Own.commit()
	g2Win.commit()

	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 || ws.Windows[0].Graph != 1 || ws.Windows[1].Graph != 2 || ws.Windows[0].ID == ws.Windows[1].ID {
		t.Fatalf("windows %+v", ws)
	}
	w1, w2 := ws.Windows[0].ID, ws.Windows[1].ID

	// Each window falls back to its own graph's surface 0 menu and never sees the other graph's menu.
	result(t, g, automation.Menu, automation.MenuParams{Window: w1, Path: []string{"App1", "Reset1"}}, nil)
	result(t, g, automation.Menu, automation.MenuParams{Window: w1, Path: []string{"Win1", "Edit1"}}, nil)
	if got := code(t, g, automation.Menu, automation.MenuParams{Window: w1, Path: []string{"App2", "Reset2"}}); got != automation.NoMatch {
		t.Fatalf("graph 1 window saw graph 2's own menu: %s", got)
	}
	result(t, g, automation.Menu, automation.MenuParams{Window: w2, Path: []string{"App2", "Reset2"}}, nil)
	result(t, g, automation.Menu, automation.MenuParams{Window: w2, Path: []string{"Win2", "Edit2"}}, nil)
	if got := code(t, g, automation.Menu, automation.MenuParams{Window: w2, Path: []string{"App1", "Reset1"}}); got != automation.NoMatch {
		t.Fatalf("graph 2 window saw graph 1's own menu: %s", got)
	}
	if len(g1Own.chose) != 1 || len(g1Win.chose) != 1 || len(g2Own.chose) != 1 || len(g2Win.chose) != 1 {
		t.Fatalf("menu routing mismatch: g1Own=%v g1Win=%v g2Own=%v g2Win=%v", g1Own.chose, g1Win.chose, g2Own.chose, g2Win.chose)
	}

	// All four surfaces' alerts are listed with four distinct automation window IDs (graph 1's own keeping 0).
	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 4 {
		t.Fatalf("alerts %+v, want 4", alerts)
	}
	alertWin := map[string]uint32{}
	seenWin := map[uint32]bool{}
	for _, a := range alerts.Alerts {
		if seenWin[a.Window] {
			t.Fatalf("duplicate alert window ID %d in %+v", a.Window, alerts.Alerts)
		}
		seenWin[a.Window] = true
		alertWin[a.Title] = a.Window
	}
	if alertWin["G1 Own Alert"] != 0 || alertWin["G1 Win Alert"] != w1 || alertWin["G2 Win Alert"] != w2 || alertWin["G2 Own Alert"] == 0 {
		t.Fatalf("unexpected alert window IDs: %+v (w1=%d, w2=%d)", alertWin, w1, w2)
	}
	// Answering graph 2's own alert routes specifically to (graph 2, surface 0).
	g2OwnWin := alertWin["G2 Own Alert"]
	okIdx := 0
	result(t, g, automation.Alert, automation.AlertParams{Window: &g2OwnWin, Answer: &okIdx}, &alerts)
	if len(g2Own.answered) != 1 || len(g1Own.answered) != 0 || len(g1Win.answered) != 0 || len(g2Win.answered) != 0 {
		t.Fatalf("alert answer routing mismatch: g1Own=%v g1Win=%v g2Own=%v g2Win=%v", g1Own.answered, g1Win.answered, g2Own.answered, g2Win.answered)
	}
	if len(alerts.Alerts) != 0 {
		t.Fatalf("filtered alert result still has %+v", alerts)
	}
}

// TestAgentLosesOneEngineModelWithoutAffectingOthers checks that a corrupt batch on one engine leaves only that engine's model lost while another engine keeps answering snapshots, presses and screenshots.
func TestAgentLosesOneEngineModelWithoutAffectingOthers(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	makeLive := func(graph int) *fakeLive {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, core.ApproxTextEngine{})
		tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
		tree.Mount(fixture(new(int)))
		return &fakeLive{surface: registerGraph(g, graph, 0), tree: tree}
	}
	live1 := makeLive(1)
	live2 := makeLive(2)
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if id != 0 {
			return nil
		}
		switch graph {
		case 1:
			return live1
		case 2:
			return live2
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })
	live1.commit()
	live2.commit()

	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 {
		t.Fatalf("windows %+v", ws)
	}
	w1, w2 := ws.Windows[0].ID, ws.Windows[1].ID

	// Corrupt engine 1's model.
	live1.surface.follow.Batch(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemFrame, ID: 1 << 30}}})
	if got := code(t, g, automation.Press, automation.PressParams{Window: w1, Selector: "id:add"}); got != automation.Failed {
		t.Fatalf("lost engine 1 press = %s, want failed", got)
	}
	// Engine 2 stays healthy.
	var step automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Window: w2, Selector: "id:add", Snapshot: true}, &step)
	if !strings.Contains(step.Snapshot, `"Count: 1"`) {
		t.Fatalf("engine 2 after engine 1 lost:\n%s", step.Snapshot)
	}
	var shot automation.ScreenshotResult
	result(t, g, automation.Screenshot, automation.ScreenshotParams{Window: w2}, &shot)
	if shot.Width != 1 || live2.pngs != 1 {
		t.Fatalf("engine 2 screenshot %dx%d pngs=%d", shot.Width, shot.Height, live2.pngs)
	}
}

// TestAgentDockMenuAfterLastWindowCloses checks that a Dock menu set on an app's own surface can still be chosen after its last window closes, and disappears once that own surface unregisters.
func TestAgentDockMenuAfterLastWindowCloses(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	root0 := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
	t.Cleanup(root0.Dispose)
	tree0 := core.NewTree(root0, core.ApproxTextEngine{})
	tree0.Mount(func(cx *core.Scope) core.Node {
		cx.SetDockMenu([]core.MenuItem{{Label: "New Window", OnSelect: func() {}}})
		return core.View{}
	})
	own := &chooser{fakeLive: &fakeLive{surface: registerGraph(g, 1, 0), tree: tree0}}

	root1 := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
	t.Cleanup(root1.Dispose)
	tree1 := core.NewTree(root1, core.ApproxTextEngine{})
	tree1.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
	tree1.Mount(func(cx *core.Scope) core.Node { return core.Text{Content: "main"} })
	win := &chooser{fakeLive: &fakeLive{surface: registerGraph(g, 1, 1), tree: tree1}}

	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if graph != 1 {
			return nil
		}
		if id == 0 {
			return own
		}
		if id == 1 {
			return win
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })

	opens(own.surface)
	own.commit()
	win.commit()
	// Show the window once so awaitWindow does not wait after it closes.
	result(t, g, automation.Windows, nil, nil)
	g.Unregistered(win.surface)

	result(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"New Window"}}, nil)
	if len(own.docked) != 1 {
		t.Fatalf("own docked = %v, want 1", own.docked)
	}
	g.Unregistered(own.surface)
	if got := code(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"New Window"}}); got != automation.NoMatch {
		t.Fatalf("dock menu after own surface unregistered = %s, want no_match", got)
	}
}

// TestAgentMixedWindowingAndEmbeddedEngines combines a windowing graph (own surface, window, panel and tray icon), an embedded engine (surface 0 with views) and a secondary windowless own surface whose model is lost after showing an alert, checking window listing, kinds, isolation and cross-engine Dock menu handoff.
func TestAgentMixedWindowingAndEmbeddedEngines(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)

	type key struct {
		graph int
		id    uint32
	}
	choosers := map[key]*chooser{}
	makeTree := func(graph int, id uint32, sized bool, body func(*core.Scope) core.Node) *chooser {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, core.ApproxTextEngine{})
		if sized {
			tree.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
		}
		tree.Mount(body)
		c := &chooser{fakeLive: &fakeLive{surface: registerGraph(g, graph, int(id)), tree: tree}}
		choosers[key{graph, id}] = c
		return c
	}

	// Graph 1: windowing app with own surface, a window, a panel and a tray icon view.
	g1Own := makeTree(1, 0, false, func(cx *core.Scope) core.Node {
		cx.SetDockMenu([]core.MenuItem{{Label: "Graph 1 Dock", OnSelect: func() {}}})
		return core.View{}
	})
	g1Win := makeTree(1, 1, true, func(*core.Scope) core.Node { return core.Text{Content: "G1 Window"} })
	g1Panel := makeTree(1, 2, true, func(*core.Scope) core.Node { return core.Text{Content: "G1 Panel"} })
	g1Tray := makeTree(1, 3, true, func(*core.Scope) core.Node { return core.Text{Content: "G1 Tray"} })

	// Graph 2: embedded view (surface 0 laid out directly, with its own Dock menu replacing graph 1's).
	g2Emb := makeTree(2, 0, true, func(cx *core.Scope) core.Node {
		cx.SetDockMenu([]core.MenuItem{{Label: "Graph 2 Dock", OnSelect: func() {}}})
		return core.Text{Content: "G2 Embedded"}
	})

	// Graph 3: windowless secondary own surface showing an alert, with no windows opened yet.
	g3Own := makeTree(3, 0, false, func(cx *core.Scope) core.Node {
		cx.ShowAlert(core.AlertConfig{Title: "G3 Own Alert", Actions: []core.AlertAction{{Label: "OK"}}}, func(int) {})
		return core.View{}
	})

	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		if c := choosers[key{graph, id}]; c != nil {
			return c
		}
		return nil
	})
	t.Cleanup(func() { testhost.RegisterLive(nil) })

	g1Own.surface.follow.Batch(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemOpenSurface, Surface: &driver.SurfaceConfig{ID: 1, Kind: driver.SurfaceWindow, Title: "Main"}},
		{Kind: driver.ItemOpenSurface, Surface: &driver.SurfaceConfig{ID: 2, Kind: driver.SurfacePanel, Title: "Palette"}},
		{Kind: driver.ItemOpenSurface, Surface: &driver.SurfaceConfig{ID: 3, Kind: driver.SurfaceTrayIcon}},
	}})
	g1Own.commit()
	g1Win.commit()
	g1Panel.commit()
	g1Tray.commit()
	g2Emb.commit()
	g3Own.commit()

	// Query alerts so graph 3's windowless own surface gets a positive own window ID, then lose its model.
	var alerts automation.AlertResult
	result(t, g, automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window == 0 {
		t.Fatalf("graph 3 own alert: %+v", alerts)
	}
	g3Own.surface.follow.Batch(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemFrame, ID: 1 << 30}}})

	// windows() lists only the window, panel and embedded view (not own surfaces, tray icon or lost own surface).
	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 3 {
		t.Fatalf("windows = %+v, want 3 (window, panel, embedded app)", ws.Windows)
	}
	if ws.Windows[0].Graph != 1 || ws.Windows[0].Kind != "window" || ws.Windows[0].Title != "Main" {
		t.Fatalf("window 0 = %+v", ws.Windows[0])
	}
	if ws.Windows[1].Graph != 1 || ws.Windows[1].Kind != "panel" || ws.Windows[1].Title != "Palette" {
		t.Fatalf("window 1 = %+v", ws.Windows[1])
	}
	if ws.Windows[2].Graph != 2 || ws.Windows[2].Kind != "app" || !ws.Windows[2].Default {
		t.Fatalf("window 2 = %+v", ws.Windows[2])
	}

	// Graph 2 set the Dock menu after graph 1, so Dock menu chooses through graph 2 even when naming graph 1's window.
	result(t, g, automation.Menu, automation.MenuParams{Window: ws.Windows[0].ID, Dock: true, Path: []string{"Graph 2 Dock"}}, nil)
	if len(g2Emb.docked) != 1 || len(g1Own.docked) != 0 {
		t.Fatalf("docked g1=%v g2=%v", g1Own.docked, g2Emb.docked)
	}
	// Closing graph 2 clears the Dock menu and shifts default back to graph 1's panel.
	g.Unregistered(g2Emb.surface)
	if got := code(t, g, automation.Menu, automation.MenuParams{Dock: true, Path: []string{"Graph 2 Dock"}}); got != automation.NoMatch {
		t.Fatalf("dock menu after graph 2 closed = %s, want no_match", got)
	}
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 2 || ws.Windows[1].Kind != "panel" || !ws.Windows[1].Default {
		t.Fatalf("windows after closing embedded engine = %+v", ws.Windows)
	}
}
