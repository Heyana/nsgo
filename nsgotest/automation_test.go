package nsgotest

import (
	"encoding/json"
	"strconv"
	"strings"
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

// fakeSurface is a surface of graph 1 as DevTools' registry hands it the agent, which follows it.
type fakeSurface struct {
	devtools.Surface
	id     int
	follow devtools.Follower
}

func (s *fakeSurface) ID() int                    { return s.id }
func (s *fakeSurface) Graph() int                 { return 1 }
func (s *fakeSurface) Follow(f devtools.Follower) { s.follow = f }

// register has g follow a surface numbered id, as core registers each one in a running app.
func register(g *agent, id int) *fakeSurface {
	s := &fakeSurface{id: id}
	g.Registered(s)
	return s
}

// opens has the app's own surface report that it opened window 1.
func opens(own *fakeSurface) {
	own.follow.Batch(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemOpenSurface, Surface: &driver.SurfaceConfig{ID: 1, Title: "Main"}}}})
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
	testhost.RegisterLive(func(id uint32) testhost.Surface {
		if id == 1 {
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
		f := g.surfaces[1]
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

// TestAgentFollowsTheRegistry has core register a running app's surfaces with the agent, as nsgo dev -automation's builds do, and checks that it follows the first graph's alone, as a second graph's surfaces have the same IDs.
func TestAgentFollowsTheRegistry(t *testing.T) {
	g := newAgent()
	t.Cleanup(g.close)
	devtools.Enabled, devtools.Automation = true, g
	t.Cleanup(func() { devtools.Enabled, devtools.Automation = false, nil })
	start := func(title, text string) {
		root := core.NewRoot(core.RootOptions{CheckCommits: !automationBuild, Windows: &core.WindowOptions{Width: 300, Height: 200}})
		t.Cleanup(root.Dispose)
		app := core.NewTree(root, core.ApproxTextEngine{})
		app.Mount(core.AppRoot(func(cx *core.Scope) {
			cx.OpenWindow(core.WindowOptions{Title: title, Width: 300, Height: 200}, func(cx *core.Scope) core.Node { return core.Text{Content: text} })
		}))
		for _, tree := range append([]*core.Tree{app}, app.Surfaces()...) {
			tree.Commit()
		}
	}
	start("First", "one")
	start("Second", "two")
	var ws automation.WindowsResult
	result(t, g, automation.Windows, nil, &ws)
	if len(ws.Windows) != 1 || ws.Windows[0].Title != "First" {
		t.Fatalf("windows %+v", ws)
	}
	var snap automation.SnapshotResult
	result(t, g, automation.Snapshot, automation.SnapshotParams{}, &snap)
	if !strings.Contains(snap.Text, `"one"`) || strings.Contains(snap.Text, `"two"`) {
		t.Fatalf("the first graph's window shows:\n%s", snap.Text)
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
	testhost.RegisterLive(func(id uint32) testhost.Surface {
		if c := trees[id]; c != nil {
			return c
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
	testhost.RegisterLive(func(id uint32) testhost.Surface {
		if c := trees[id]; c != nil {
			return c
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
	testhost.RegisterLive(func(id uint32) testhost.Surface {
		if id == 1 {
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
	f := g.surfaces[1]
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
	testhost.RegisterLive(func(id uint32) testhost.Surface {
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
	if len(ws.Windows) != 2 || ws.Windows[0].ID != 0 || ws.Windows[0].Kind != "app" || ws.Windows[1].ID != 1 || ws.Windows[1].Kind != "scene" || !ws.Windows[1].Default {
		t.Fatalf("windows %+v", ws)
	}
	var r automation.StepResult
	result(t, g, automation.Press, automation.PressParams{Window: 0, Selector: "id:add", Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Count: 1"`) {
		t.Fatalf("first scene press:\n%s", r.Snapshot)
	}
	result(t, g, automation.Press, automation.PressParams{Window: 1, Selector: "id:add", Snapshot: true}, &r)
	if !strings.Contains(r.Snapshot, `"Count: 1"`) {
		t.Fatalf("second scene press:\n%s", r.Snapshot)
	}
}
