package nsgotest

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"maps"
	"slices"
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

// The automation's agent. `nsgo dev -automation` links it into the running app (automation_on.go), where it is DevTools' agent (devtools.Automation): DevTools' registry hands it every surface core registers across every graph in the process, and it follows each (devtools.Surface.Follow) for its batches and events, so each surface has a model as a test's app on a target does, and it answers package internal/automation's requests with the steps tests take, acting through the backend's live surfaces (testhost.Live).

// surfaceKey identifies one surface within the process: each graph numbers its own surfaces from 0.
type surfaceKey struct {
	graph int
	id    uint32
}

// agent follows every running engine's surfaces in the process and answers requests about them.
type agent struct {
	tb   *liveTB
	stop chan struct{}
	tick sync.Once

	mu       sync.Mutex
	surfaces map[surfaceKey]*followed
	byWindow map[uint32]*followed
	// ownZero is the first windowless app-own surface, which keeps window ID 0 for its alerts as a single-engine app does; another graph's own surface gets a window ID of its own.
	ownZero *followed
	// nextID numbers listed windows, embedded views and secondary own surfaces monotonically from 1, so closed IDs are never reused and no listed window collides with 0 (the default window).
	nextID uint32
	// nextSeq orders surfaces oldest-first across every graph.
	nextSeq uint64
	// dock is the surface that set the app icon's menu last, as the batches came: the platform shows that surface's menu alone, and drops it as the surface goes. Nil while none did, as for nsgotest's own apps, whose model has the app's.
	dock *followed

	// work is held while a request runs or the models take what came, so one goroutine at a time reads and changes them.
	work sync.Mutex
	// shown is set once a request found a window shown, or waited firstWindow for one; g.work guards it.
	shown bool
}

// followed is a surface the agent follows: its model, and what the app opened it as. It hears the surface as its devtools.Follower.
type followed struct {
	g   *agent
	key surfaceKey
	// winID is the automation window ID exposed to tools, valid once hasWinID is set.
	winID    uint32
	hasWinID bool
	// own is set when winID was assigned for a windowless app-own surface (alerts), not a listed view.
	own bool
	// seq is when the agent first heard of this surface, for deterministic oldest-first ordering across graphs.
	seq uint64
	app *App
	// listed reports whether the initial surface displays views of its own.
	listed bool
	// opened is set for a window, a panel or a tray icon's view the app opened, of kind and title.
	opened bool
	kind   driver.SurfaceKind
	title  string
	// opensSurfaces is set once this surface opens child surfaces (ItemOpenSurface): it is an app's own windowless surface, not a view shown in windows().
	opensSurfaces bool
	// closed is set once the surface went.
	closed bool
}

// lost says why f's model stopped following its surface (target.applyLive), "" while it follows.
func (f *followed) lost() string {
	if f.app.target == nil {
		return ""
	}
	return f.app.target.lostWhy()
}

// drainInterval is how often the models take what came while no request runs, maxWait the longest a wait waits, well within the time nsgo dev gives the app to answer (cmd/nsgo's answerFor), as the app answers one request at a time, and firstWindow how long the requests that reach a starting app wait for its first window, as the app connects before it opens one.
const (
	drainInterval = 250 * time.Millisecond
	maxWait       = time.Minute
	firstWindow   = 10 * time.Second
)

func newAgent() *agent {
	return &agent{
		tb:       &liveTB{},
		stop:     make(chan struct{}),
		surfaces: map[surfaceKey]*followed{},
		byWindow: map[uint32]*followed{},
	}
}

// newLiveApp returns a model of a running app's surface, which the agent feeds: target mode with no surface of a test's, alerts left to whoever answers them, and no clock of its own but the system's.
func newLiveApp(tb testing.TB) *App {
	a := &App{
		t:               tb,
		Clock:           core.NewManualClock(time.Now()),
		views:           map[driver.NodeID]*View{},
		edits:           map[driver.NodeID]uint32{},
		anim:            driver.NewAnimator(func(driver.Event) {}),
		pendingAlerts:   true,
		stepName:        "automation",
		ImageCacheLimit: core.DefaultImageCacheLimit,
	}
	a.target = &target{a: a, live: true, surface: noSurface{}}
	return a
}

// follow returns key's followed surface, made as the surface registers or its graph's own surface opens it. The caller holds g.mu.
func (g *agent) follow(key surfaceKey) *followed {
	f := g.surfaces[key]
	if f == nil || f.closed {
		if f != nil {
			g.forget(key, f)
		}
		g.nextSeq++
		f = &followed{g: g, key: key, seq: g.nextSeq, app: newLiveApp(g.tb)}
		g.surfaces[key] = f
	}
	return f
}

// forget removes f from the agent's tables. The caller holds g.mu.
func (g *agent) forget(key surfaceKey, f *followed) {
	delete(g.surfaces, key)
	if f.hasWinID && g.byWindow[f.winID] == f {
		delete(g.byWindow, f.winID)
	}
	if g.ownZero == f {
		g.ownZero = nil
	}
	if g.dock == f {
		g.dock = nil
	}
}

// allocWindowID returns the next unused positive automation window ID. The caller holds g.mu.
func (g *agent) allocWindowID() uint32 {
	for {
		g.nextID++
		if g.nextID == 0 {
			g.nextID = 1
		}
		if g.byWindow[g.nextID] == nil {
			return g.nextID
		}
	}
}

// windowID returns f's automation window ID, assigning one when it has none. own is true only for a windowless app-own surface: the first keeps 0 for its alerts, while every listed window and every other graph's own surface gets a unique positive ID. The caller holds g.mu.
func (g *agent) windowID(f *followed, own bool) uint32 {
	if f.hasWinID {
		if !own && f.winID == 0 {
			// A surface that was assigned 0 before it had a size and later became a listed view moves to a positive ID so it never collides with 0 (the default window).
			if g.byWindow[0] == f {
				delete(g.byWindow, 0)
			}
			if g.ownZero == f {
				g.ownZero = nil
			}
			f.hasWinID, f.own = false, false
		} else {
			if !own {
				f.own = false
			}
			return f.winID
		}
	}
	if own && f.key.id == 0 && g.ownZero == nil {
		f.winID, f.hasWinID, f.own = 0, true, true
		g.ownZero = f
		g.byWindow[0] = f
		return 0
	}
	id := g.allocWindowID()
	f.winID, f.hasWinID, f.own = id, true, own
	g.byWindow[id] = f
	return id
}

// serve has the agent answer for a, as window id: for tests of the agent on nsgotest's own apps, whose steps then fail the request rather than the test.
func (g *agent) serve(id uint32, a *App, title string) {
	a.t = g.tb
	g.mu.Lock()
	g.nextSeq++
	key := surfaceKey{graph: 0, id: id}
	f := &followed{g: g, key: key, winID: id, hasWinID: true, seq: g.nextSeq, app: a, opened: true, kind: driver.SurfaceWindow, title: title}
	g.surfaces[key] = f
	g.byWindow[id] = f
	if id > g.nextID {
		g.nextID = id
	}
	g.mu.Unlock()
}

// close stops the agent's drains.
func (g *agent) close() { close(g.stop) }

// Registered implements devtools.Agent: the agent follows every surface of every graph in the process, from before the surface's first batch.
func (g *agent) Registered(s devtools.Surface, listed bool) {
	key := surfaceKey{graph: s.Graph(), id: uint32(s.ID())}
	g.mu.Lock()
	f := g.follow(key)
	f.listed = listed
	g.mu.Unlock()
	s.Follow(f)
}

// Unregistered implements devtools.Agent: the next drain forgets the surface.
func (g *agent) Unregistered(s devtools.Surface) {
	key := surfaceKey{graph: s.Graph(), id: uint32(s.ID())}
	g.mu.Lock()
	if f := g.surfaces[key]; f != nil {
		f.closed = true
	}
	g.mu.Unlock()
}

// Batch implements devtools.Follower.
func (f *followed) Batch(b *driver.MountBatch) {
	g := f.g
	g.tick.Do(func() { go g.drainEvery(drainInterval) })
	g.mu.Lock()
	closed := f.closed
	if !closed {
		for _, it := range b.Items {
			if it.Kind == driver.ItemOpenSurface && it.Surface != nil {
				if it.Surface.Kind != driver.SurfaceScene {
					f.opensSurfaces = true
					if f.key.id == 0 && !f.hasWinID && g.ownZero == nil {
						f.winID, f.hasWinID, f.own = 0, true, true
						g.ownZero = f
						g.byWindow[0] = f
					}
				}
				o := g.follow(surfaceKey{graph: f.key.graph, id: it.Surface.ID})
				o.opened, o.kind, o.title = true, it.Surface.Kind, it.Surface.Title
			}
			if it.Kind == driver.ItemDockMenu {
				g.dock = f
			}
		}
	}
	g.mu.Unlock()
	if !closed && f.app.target != nil {
		// Each model follows its one surface, as a target's first.
		f.app.target.observe(0, b)
	}
}

// Event implements devtools.Follower.
func (f *followed) Event(ev driver.Event) {
	g := f.g
	g.mu.Lock()
	closed := f.closed
	g.mu.Unlock()
	if !closed && f.app.target != nil {
		f.app.target.event(ev)
	}
}

// drainEvery has the models take what came every d, so what the app sends while no request runs never piles up.
func (g *agent) drainEvery(d time.Duration) {
	tick := time.NewTicker(d)
	defer tick.Stop()
	for {
		select {
		case <-g.stop:
			return
		case <-tick.C:
			g.work.Lock()
			g.drainAll()
			g.work.Unlock()
		}
	}
}

// drainAll has every live model take what came, and forgets the surfaces destroyed. The caller holds g.work.
func (g *agent) drainAll() {
	g.mu.Lock()
	var live []*followed
	for key, f := range g.surfaces {
		if f.closed {
			g.forget(key, f)
			continue
		}
		if f.app.target != nil && f.lost() == "" {
			live = append(live, f)
		}
	}
	g.mu.Unlock()
	for _, f := range live {
		g.drain(f)
	}
}

// drain has f's model take what came. A batch the model cannot apply leaves it lost rather than ending the app; the error says so.
func (g *agent) drain(f *followed) *automation.Error {
	return g.tb.try(func() { f.app.target.drain() })
}

// Do implements devtools.Agent.
func (g *agent) Do(request []byte) []byte {
	var req automation.Request
	var answer automation.Answer
	if err := json.Unmarshal(request, &req); err != nil {
		answer = automation.Reply(0, nil, automation.Errorf(automation.InvalidRequest, "%v", err))
	} else {
		g.work.Lock()
		result, err := g.do(req)
		g.work.Unlock()
		answer = automation.Reply(req.ID, result, err)
	}
	data, _ := json.Marshal(answer)
	return data
}

func (g *agent) do(req automation.Request) (any, *automation.Error) {
	params := automation.Params[req.Method]
	switch req.Method {
	case automation.Status, automation.Logs, automation.Reload:
		params = nil
	}
	if params == nil {
		return nil, automation.Errorf(automation.UnknownMethod, "the app answers no method %q", req.Method)
	}
	p := params()
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, p); err != nil {
			return nil, automation.Errorf(automation.InvalidRequest, "%s's params: %v", req.Method, err)
		}
	}
	g.awaitWindow()
	switch p := p.(type) {
	case *automation.WindowsParams:
		return automation.WindowsResult{Windows: g.windows()}, nil
	case *automation.SnapshotParams:
		id, f, err := g.window(p.Window)
		if err != nil {
			return nil, err
		}
		return automation.SnapshotResult{Window: id, Text: outline(f.app, p.All, p.Bounds)}, nil
	case *automation.FindParams:
		return g.find(p)
	case *automation.PressParams:
		var step func(a *App)
		switch p.Mode {
		case "", "tap":
			step = func(a *App) {
				if p.At != nil {
					a.PressAt(p.Selector, driver.Point{X: driver.Points(p.At.X), Y: driver.Points(p.At.Y)})
				} else {
					a.Press(p.Selector)
				}
			}
		case "long":
			step = func(a *App) { a.LongPress(p.Selector) }
		case "right":
			step = func(a *App) { a.RightClick(p.Selector) }
		default:
			return nil, automation.Errorf(automation.InvalidRequest, "no press mode %q: tap, long or right", p.Mode)
		}
		return g.step(p.Window, p.Selector, p.Snapshot, step)
	case *automation.TypeParams:
		return g.step(p.Window, p.Selector, p.Snapshot, func(a *App) {
			if p.Clear {
				a.Clear(p.Selector)
			}
			if p.Text != "" {
				a.Type(p.Selector, p.Text)
			}
			if p.Submit {
				a.Submit(p.Selector)
			}
			if !p.Clear && p.Text == "" && !p.Submit {
				a.Focus(p.Selector)
			}
		})
	case *automation.KeyParams:
		if p.Key == "" {
			return nil, automation.Errorf(automation.InvalidRequest, "key names no key")
		}
		return g.step(p.Window, p.Selector, p.Snapshot, func(a *App) {
			if p.Selector != "" {
				a.KeyPress(p.Selector, p.Key)
			} else {
				a.Key(p.Key)
			}
		})
	case *automation.ScrollParams:
		if p.By == 0 && p.To == nil {
			return nil, automation.Errorf(automation.InvalidRequest, "scroll needs by or to")
		}
		return g.step(p.Window, p.Selector, p.Snapshot, func(a *App) {
			by := driver.Points(p.By)
			if p.To != nil {
				v := scrollViewOf(a, p.Selector)
				at := v.Offset.Y
				if v.Scroll.Horizontal {
					at = v.Offset.X
				}
				by = driver.Points(*p.To) - at
			}
			a.Scroll(p.Selector, by)
		})
	case *automation.DragParams:
		if len(p.Path) == 0 {
			return nil, automation.Errorf(automation.InvalidRequest, "drag needs a path")
		}
		path := make([]driver.Point, len(p.Path))
		for i, q := range p.Path {
			path[i] = driver.Point{X: driver.Points(q.X), Y: driver.Points(q.Y)}
		}
		return g.step(p.Window, p.Selector, p.Snapshot, func(a *App) {
			if p.At != nil {
				a.DragAt(p.Selector, driver.Point{X: driver.Points(p.At.X), Y: driver.Points(p.At.Y)}, path...)
			} else {
				a.Drag(p.Selector, path...)
			}
		})
	case *automation.HoverParams:
		return g.step(p.Window, p.Selector, p.Snapshot, func(a *App) { a.Hover(p.Selector) })
	case *automation.MenuParams:
		return g.menu(p)
	case *automation.AlertParams:
		return g.alert(p)
	case *automation.WaitParams:
		return g.wait(p)
	case *automation.ScreenshotParams:
		return g.screenshot(p)
	}
	return nil, automation.Errorf(automation.UnknownMethod, "the app answers no method %q", req.Method)
}

// windowKind returns how windows lists f ("window", "panel", "app", or "" for a windowless own surface or tray icon) and its root's size. The caller holds g.work and g.mu.
func (f *followed) windowKind() (string, driver.Size) {
	var size driver.Size
	if root := f.app.views[f.app.rootID]; root != nil {
		size = root.Frame.Size
	}
	switch {
	case f.opened && f.kind == driver.SurfaceWindow:
		return "window", size
	case f.opened && f.kind == driver.SurfacePanel:
		return "panel", size
	case f.opened && f.kind == driver.SurfaceScene:
		return "scene", size
	case !f.opened && f.listed && !f.opensSurfaces && (size.Width > 0 && size.Height > 0 || f.hasWinID && !f.own && f.lost() != ""):
		// Where the platform opens no windows of the app's, or in an embedded view (NewView), the surface shows views directly; keep a lost model listed so requests to it report why it stopped following instead of no_window.
		return "app", size
	default:
		return "", size
	}
}

// windows lists the windows, panels and embedded views the process shows across every graph, oldest first; the newest is the default.
func (g *agent) windows() []automation.Window {
	g.mu.Lock()
	defer g.mu.Unlock()
	list := slices.Collect(maps.Values(g.surfaces))
	slices.SortFunc(list, func(a, b *followed) int { return cmp.Compare(a.seq, b.seq) })
	out := []automation.Window{}
	for _, f := range list {
		if f.closed {
			continue
		}
		kind, size := f.windowKind()
		if kind == "" {
			continue
		}
		id := g.windowID(f, false)
		title := f.app.Title
		if title == "" {
			title = f.title
		}
		out = append(out, automation.Window{ID: id, Graph: f.key.graph, Title: title, Kind: kind, Width: float64(size.Width), Height: float64(size.Height)})
	}
	if len(out) > 0 {
		out[len(out)-1].Default = true
	}
	return out
}

// awaitWindow has the models take what came and, until the app first shows a window laid out, waits up to firstWindow for one: a starting app answers requests before its window opens, which would otherwise list no window. An app that never shows one costs the first request alone the wait. The caller holds g.work.
func (g *agent) awaitWindow() {
	deadline := time.Now().Add(firstWindow)
	for {
		g.drainAll()
		if g.shown {
			return
		}
		if slices.ContainsFunc(g.windows(), func(w automation.Window) bool { return w.Width > 0 && w.Height > 0 }) || time.Now().After(deadline) {
			g.shown = true
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// window returns the window id names, the default for 0.
func (g *agent) window(id uint32) (uint32, *followed, *automation.Error) {
	ws := g.windows()
	if len(ws) == 0 {
		return 0, nil, automation.Errorf(automation.NoWindow, "the app shows no window")
	}
	at := len(ws) - 1
	if id != 0 {
		at = slices.IndexFunc(ws, func(w automation.Window) bool { return w.ID == id })
		if at < 0 {
			return 0, nil, automation.Errorf(automation.NoWindow, "no window %d: windows lists the app's", id)
		}
	}
	id = ws[at].ID
	g.mu.Lock()
	f := g.byWindow[id]
	closed := f == nil || f.closed
	g.mu.Unlock()
	if closed {
		return 0, nil, automation.Errorf(automation.NoWindow, "no window %d: windows lists the app's", id)
	}
	if why := f.lost(); why != "" {
		return 0, nil, automation.Errorf(automation.Failed, "the automation stopped following window %d: %s", id, why)
	}
	g.prepare(f)
	return id, f, nil
}

// prepare gives f's model the backend's live surface to act on, or one that shows nothing.
func (g *agent) prepare(f *followed) {
	if f.app.target == nil {
		return
	}
	s := testhost.Live(f.key.graph, f.key.id)
	if s == nil {
		s = noSurface{}
	}
	f.app.target.surface = s
}

// match checks selector before a step: it reads, and a view shown matches it.
func match(a *App, selector string) *automation.Error {
	if _, err := parseSelector(selector); err != nil {
		return automation.Errorf(automation.InvalidRequest, "%v", err)
	}
	if !slices.ContainsFunc(a.Query(selector), func(v *View) bool { return !devtoolsOwn(v) }) {
		return automation.Errorf(automation.NoMatch, "no view shown matches %q", selector)
	}
	return nil
}

// step takes a step in window, on what selector names when given, and answers how the app came to rest.
func (g *agent) step(window uint32, selector string, snapshot bool, step func(a *App)) (any, *automation.Error) {
	_, f, err := g.window(window)
	if err != nil {
		return nil, err
	}
	a := f.app
	if selector != "" {
		if err := match(a, selector); err != nil {
			return nil, err
		}
	}
	if err := g.tb.try(func() { step(a) }); err != nil {
		return nil, err
	}
	r := automation.StepResult{Settled: a.target == nil || a.target.quiet}
	if snapshot {
		r.Snapshot = outline(a, false, false)
	}
	return r, nil
}

func (g *agent) find(p *automation.FindParams) (any, *automation.Error) {
	_, f, err := g.window(p.Window)
	if err != nil {
		return nil, err
	}
	if _, perr := parseSelector(p.Selector); perr != nil {
		return nil, automation.Errorf(automation.InvalidRequest, "%v", perr)
	}
	var views []*View
	if p.Mounted {
		views = f.app.QueryMounted(p.Selector)
	} else {
		views = f.app.Query(p.Selector)
	}
	if len(views) == 0 {
		return nil, automation.Errorf(automation.NoMatch, "no view matches %q", p.Selector)
	}
	r := automation.FindResult{Elements: []automation.Element{}}
	for _, v := range views {
		if !devtoolsOwn(v) {
			r.Elements = append(r.Elements, element(v, true))
		}
	}
	if len(r.Elements) == 0 {
		return nil, automation.Errorf(automation.NoMatch, "no view matches %q", p.Selector)
	}
	return r, nil
}

// menu chooses an item of the application menu, the window's or its graph's own surface's (on macOS set from the app's function), of the context menu showing, or of the app icon's menu, the process's whichever surface set it.
func (g *agent) menu(p *automation.MenuParams) (any, *automation.Error) {
	if len(p.Path) == 0 {
		return nil, automation.Errorf(automation.InvalidRequest, "menu needs a path")
	}
	if p.Popup && p.Dock {
		return nil, automation.Errorf(automation.InvalidRequest, "popup and dock name different menus")
	}
	var id uint32
	var f *followed
	if p.Dock && p.Window == 0 && !p.Snapshot {
		// On macOS the Dock menu can still be chosen after every window closes.
		if ws := g.windows(); len(ws) > 0 {
			var err *automation.Error
			id, f, err = g.window(0)
			if err != nil {
				return nil, err
			}
		}
	} else {
		var err *automation.Error
		id, f, err = g.window(p.Window)
		if err != nil {
			return nil, err
		}
	}
	if p.Popup {
		return g.step(id, "", p.Snapshot, func(a *App) { a.ChoosePopup(p.Path...) })
	}
	owner := f
	var item *driver.MenuEntry
	var why string
	if p.Dock {
		// The process has one Dock menu, the one a surface set last, whichever window the request names.
		g.mu.Lock()
		dock := g.dock
		if dock != nil {
			owner = dock
		} else if f != nil && f.app.target != nil {
			// A live app whose Dock menu was never set, or whose owner closed.
			owner = nil
		}
		gone := owner == nil || owner.closed
		g.mu.Unlock()
		if !gone {
			if lost := owner.lost(); lost != "" {
				return nil, automation.Errorf(automation.Failed, "the automation stopped following surface %d, which set the Dock menu: %s", owner.key.id, lost)
			}
		}
		if gone || len(owner.app.dockMenu) == 0 {
			return nil, automation.Errorf(automation.NoMatch, "the app shows no Dock menu")
		}
		item, why = menuItemAt(owner.app.dockMenu, p.Path)
	} else {
		item, why = menuItemAt(f.app.menu, p.Path)
		if item == nil {
			g.mu.Lock()
			own := g.surfaces[surfaceKey{graph: f.key.graph, id: 0}]
			ownClosed := own == nil || own.closed
			g.mu.Unlock()
			if !ownClosed && own != f {
				if it, w := menuItemAt(own.app.menu, p.Path); it != nil || len(f.app.menu) == 0 {
					item, why, owner = it, w, own
				}
			}
		}
	}
	if item == nil {
		return nil, automation.Errorf(automation.NoMatch, "%s", why)
	}
	if item.Disabled {
		return nil, automation.Errorf(automation.Failed, "the menu item %q is disabled", p.Path)
	}
	if owner.app.target == nil {
		// A model of nsgotest's own runs the menu as a test does.
		choose := owner.app.ChooseMenu
		if p.Dock {
			choose = owner.app.ChooseDockMenu
		}
		if err := g.tb.try(func() { choose(p.Path...) }); err != nil {
			return nil, err
		}
	} else {
		// Each surface numbers its menu's items itself, so the surface whose menu has the item chooses it.
		s := testhost.Live(owner.key.graph, owner.key.id)
		if s == nil {
			return nil, automation.Errorf(automation.Unsupported, "no surface shows the menu")
		}
		choose := s.ChooseMenu
		if p.Dock {
			choose = s.ChooseDockMenu
		}
		if err := choose(item.ID); errors.Is(err, testhost.ErrNoMenu) {
			return nil, automation.Errorf(automation.Unsupported, "%v", err)
		} else if err != nil {
			return nil, automation.Errorf(automation.Failed, "%v", err)
		}
		g.prepare(owner)
		if err := g.tb.try(func() {
			owner.app.target.settle()
			if f != nil && f != owner {
				f.app.target.settle()
			}
		}); err != nil {
			return nil, err
		}
	}
	r := automation.StepResult{Settled: f == nil || f.app.target == nil || f.app.target.quiet}
	if p.Snapshot && f != nil {
		r.Snapshot = outline(f.app, false, false)
	}
	return r, nil
}

// menuItemAt returns the item at path in items, or why none is.
func menuItemAt(items []driver.MenuEntry, path []string) (*driver.MenuEntry, string) {
	for i, label := range path {
		var found *driver.MenuEntry
		for k := range items {
			if items[k].Label == label && !items[k].Separator {
				found = &items[k]
			}
		}
		if found == nil {
			return nil, fmt.Sprintf("no menu item %q", path[:i+1])
		}
		if i == len(path)-1 {
			if found.ID == 0 {
				return nil, fmt.Sprintf("the menu item %q is a submenu", path)
			}
			return found, ""
		}
		items = found.Items
	}
	return nil, "no menu item"
}

// shownAlert is an alert showing, the automation window ID of the surface showing it, and where it is among its model's alerts.
type shownAlert struct {
	window uint32
	f      *followed
	alert  driver.AlertConfig
	index  int
}

// alerts lists the alerts showing across every graph, oldest surface first: window's alone when it names one.
func (g *agent) alerts(window *uint32) []shownAlert {
	g.mu.Lock()
	defer g.mu.Unlock()
	list := slices.Collect(maps.Values(g.surfaces))
	slices.SortFunc(list, func(a, b *followed) int { return cmp.Compare(a.seq, b.seq) })
	var out []shownAlert
	for _, f := range list {
		if f.closed || len(f.app.alerts) == 0 {
			continue
		}
		kind, _ := f.windowKind()
		winID := g.windowID(f, kind == "")
		if window != nil && winID != *window {
			continue
		}
		for i, a := range f.app.alerts {
			out = append(out, shownAlert{winID, f, a, i})
		}
	}
	return out
}

func (g *agent) alert(p *automation.AlertParams) (any, *automation.Error) {
	list := func() automation.AlertResult {
		r := automation.AlertResult{Alerts: []automation.AlertInfo{}}
		for _, s := range g.alerts(p.Window) {
			info := automation.AlertInfo{ID: s.alert.ID, Window: s.window, Title: s.alert.Title, Message: s.alert.Message, Buttons: []string{}}
			for _, b := range s.alert.Actions {
				info.Buttons = append(info.Buttons, b.Label)
			}
			r.Alerts = append(r.Alerts, info)
		}
		return r
	}
	if p.Answer == nil {
		return list(), nil
	}
	shown := g.alerts(p.Window)
	if p.ID != 0 {
		// Each surface numbers its alerts itself.
		shown = slices.DeleteFunc(shown, func(s shownAlert) bool { return s.alert.ID != p.ID })
		if len(shown) > 1 {
			return nil, automation.Errorf(automation.InvalidRequest, "several windows show an alert %d: name its window", p.ID)
		}
	}
	if len(shown) == 0 {
		return nil, automation.Errorf(automation.NoMatch, "no alert %d shows", p.ID)
	}
	s, index := shown[0], *p.Answer
	if index < -1 || index >= len(s.alert.Actions) {
		return nil, automation.Errorf(automation.InvalidRequest, "the alert has no button %d: %d buttons", index, len(s.alert.Actions))
	}
	a := s.f.app
	if a.target == nil {
		// By its place, as a model's surfaces number their alerts alike, answered to the surface that showed it.
		// As a step, so what the answer starts, as a sheet's exit, runs its frames.
		if err := g.tb.try(func() { a.answerAt(s.index, index); a.done() }); err != nil {
			return nil, err
		}
		return list(), nil
	}
	live := testhost.Live(s.f.key.graph, s.f.key.id)
	if live == nil {
		return nil, automation.Errorf(automation.Unsupported, "no surface shows the alert")
	}
	if err := live.Answer(s.alert.ID, index); err != nil {
		return nil, automation.Errorf(automation.Failed, "%v", err)
	}
	a.alerts = slices.DeleteFunc(a.alerts, func(c driver.AlertConfig) bool { return c.ID == s.alert.ID })
	g.prepare(s.f)
	if err := g.tb.try(a.target.settle); err != nil {
		return nil, err
	}
	return list(), nil
}

func (g *agent) wait(p *automation.WaitParams) (any, *automation.Error) {
	_, f, err := g.window(p.Window)
	if err != nil {
		return nil, err
	}
	if _, perr := parseSelector(p.Selector); perr != nil {
		return nil, automation.Errorf(automation.InvalidRequest, "%v", perr)
	}
	timeout := min(time.Duration(p.TimeoutMS)*time.Millisecond, maxWait)
	if timeout <= 0 {
		timeout = settleWait
	}
	a := f.app
	deadline := time.Now().Add(timeout)
	for {
		if a.target != nil {
			if err := g.drain(f); err != nil {
				return nil, err
			}
		} else if err := g.tb.try(a.Settle); err != nil {
			return nil, err
		}
		if n := len(a.Query(p.Selector)); n > 0 != p.Gone {
			return automation.StepResult{Settled: true}, nil
		}
		if time.Now().After(deadline) {
			if p.Gone {
				return nil, automation.Errorf(automation.Timeout, "%q still matches after %v", p.Selector, timeout)
			}
			return nil, automation.Errorf(automation.Timeout, "nothing matches %q after %v", p.Selector, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (g *agent) screenshot(p *automation.ScreenshotParams) (any, *automation.Error) {
	_, f, err := g.window(p.Window)
	if err != nil {
		return nil, err
	}
	if f.app.target == nil {
		return nil, automation.Errorf(automation.Unsupported, "a headless app has no pixels")
	}
	var data []byte
	var serr error
	if err := g.tb.try(func() { data, serr = f.app.target.screenshotPNG() }); err != nil {
		return nil, err
	}
	if serr != nil {
		return nil, automation.Errorf(automation.Failed, "%v", serr)
	}
	cfg, derr := png.DecodeConfig(bytes.NewReader(data))
	if derr != nil {
		return nil, automation.Errorf(automation.Failed, "the screenshot is no PNG: %v", derr)
	}
	return automation.ScreenshotResult{PNG: data, Width: cfg.Width, Height: cfg.Height}, nil
}

// scrollViewOf is the scroll view Scroll scrolls for selector.
func scrollViewOf(a *App, selector string) *View {
	isScroll := func(v *View) bool { return v.Kind == KindScroll }
	return a.pick(selector, "scroll view", func(m *View) (*View, bool) {
		t := m.upTo(isScroll)
		if t == nil {
			t = m.downTo(isScroll)
		}
		return t, t != nil && !t.Scroll.Behavior.NoScroll
	})
}

// devtoolsOwn reports whether v is one of DevTools' own views, on their overlay over the app under nsgo dev -devtools, -highlight or -layout, which the agent leaves out.
func devtoolsOwn(v *View) bool {
	for ; v != nil; v = v.Parent {
		if v.Props.TestID == devtools.OverlayTestID {
			return true
		}
	}
	return false
}

// outline returns what a shows, an element a line, indented by the elements around it: the named views (a role, a label, a test ID, a text or a field) or with all every view shown.
func outline(a *App, all, bounds bool) string {
	shown := map[*View]bool{}
	var roots []*View
	for _, v := range a.shown() {
		shown[v] = true
		if v.Parent == nil {
			roots = append(roots, v)
		}
	}
	var b strings.Builder
	var walk func(v *View, depth int, named bool)
	walk = func(v *View, depth int, named bool) {
		if v.Style.Hidden || v.Props.TestID == devtools.OverlayTestID {
			return
		}
		// A text whose words name the element around it, a button's, says nothing more.
		if shown[v] && (all || listed(v) && !(named && v.Kind == KindText && (v.Props.Role == driver.RoleNone || v.Props.Role == driver.RoleText) && v.Props.TestID == "")) {
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(line(element(v, bounds)))
			b.WriteByte('\n')
			depth++
			named = v.Props.Role != driver.RoleNone && v.Props.Label == "" && v.Kind != KindText
		}
		for _, c := range v.Children {
			walk(c, depth, named)
		}
	}
	for _, r := range roots {
		walk(r, 0, false)
	}
	return b.String()
}

// listed reports whether a snapshot lists v when it lists the named views alone.
func listed(v *View) bool {
	switch {
	case v.Props.Role != driver.RoleNone, v.Props.TestID != "", v.Props.Label != "", v.Kind == KindInput:
		return true
	case v.Kind == KindText:
		return strings.TrimSpace(driver.VisibleText(v.Text, v.Spans)) != ""
	}
	return false
}

// element describes v as a user's tools see it.
func element(v *View, bounds bool) automation.Element {
	e := automation.Element{Ref: fmt.Sprintf("view:%d", v.ID), Role: roleName(v.Props.Role), Name: v.Props.Label, TestID: v.Props.TestID}
	if e.Role == "" {
		e.Role = map[ViewKind]string{KindText: "text", KindInput: "textbox", KindScroll: "scroll",
			KindImage: "image", KindModal: "modal", KindCanvas: "canvas", KindWeb: "web",
			KindVideo: "video", KindNative: "native", KindShader: "shader", KindGPU: "gpu"}[v.Kind]
		if e.Role == "" {
			e.Role = "view"
		}
	}
	switch {
	case v.Kind == KindText:
		e.Text = driver.VisibleText(v.Text, v.Spans)
	case v.Kind == KindInput:
		e.Text = v.Text
		if e.Name == "" {
			e.Name = v.Input.Placeholder
		}
	case e.Name == "" && v.Props.Role != driver.RoleNone:
		e.Name = strings.TrimSpace(v.textWithin())
	}
	if val := v.Props.Value; val.Text != "" {
		e.Value = val.Text
	} else if val.Max > val.Min && !val.NoNow {
		e.Value = fmt.Sprint(val.Now)
	}
	p := v.Props
	for _, s := range []struct {
		on   bool
		name string
	}{
		{v.Focused, "focused"}, {v.disabled(nil), "disabled"}, {p.Selected, "selected"}, {p.Checked, "checked"},
		{p.Indeterminate, "mixed"}, {p.Pressed, "pressed"}, {p.Expandable && p.Expanded, "expanded"},
		{p.Expandable && !p.Expanded, "collapsed"}, {p.Busy, "busy"}, {p.Invalid, "invalid"}, {v.Hovered, "hovered"},
	} {
		if s.on {
			e.States = append(e.States, s.name)
		}
	}
	if bounds {
		f := v.surfaceFrame()
		e.Bounds = &[4]float64{float64(f.Origin.X), float64(f.Origin.Y), float64(f.Size.Width), float64(f.Size.Height)}
	}
	return e
}

// line is e as a snapshot's line: `- button "Add" [view:12] id=add focused`.
func line(e automation.Element) string {
	var b strings.Builder
	b.WriteString("- " + e.Role)
	if e.Name != "" {
		fmt.Fprintf(&b, " %q", e.Name)
	}
	if e.Text != "" && e.Text != e.Name {
		if e.Role == "text" {
			fmt.Fprintf(&b, " %q", e.Text)
		} else {
			fmt.Fprintf(&b, " text=%q", e.Text)
		}
	}
	b.WriteString(" [" + e.Ref + "]")
	if e.TestID != "" {
		b.WriteString(" id=" + e.TestID)
	}
	if e.Value != "" {
		fmt.Fprintf(&b, " value=%q", e.Value)
	}
	for _, s := range e.States {
		b.WriteString(" " + s)
	}
	if r := e.Bounds; r != nil {
		fmt.Fprintf(&b, " @%g,%g %gx%g", r[0], r[1], r[2], r[3])
	}
	return b.String()
}

// noSurface is the live surface of a model whose surface shows no window, as a tray icon's.
type noSurface struct{}

var errNoWindow = errors.New("the surface shows no window to act on")

func (noSurface) Press(driver.Point, time.Duration, driver.Modifiers) error { return errNoWindow }
func (noSurface) RightClick(driver.Point) error                             { return errNoWindow }
func (noSurface) Drag([]driver.Point) error                                 { return errNoWindow }
func (noSurface) Scroll(driver.Point, driver.Point) error                   { return errNoWindow }
func (noSurface) Hover(driver.Point) error                                  { return errNoWindow }
func (noSurface) Type(string) error                                         { return errNoWindow }
func (noSurface) Compose(string) error                                      { return errNoWindow }
func (noSurface) Commit(string) error                                       { return errNoWindow }
func (noSurface) Key(string) error                                          { return errNoWindow }
func (noSurface) Back() error                                               { return errNoWindow }
func (noSurface) Answer(uint32, int) error                                  { return errNoWindow }
func (noSurface) ChooseMenu(uint32) error                                   { return testhost.ErrNoMenu }
func (noSurface) ChooseDockMenu(uint32) error                               { return testhost.ErrNoMenu }
func (noSurface) ChoosePopup(uint32) error                                  { return testhost.ErrNoMenu }
func (noSurface) Screenshot() ([]byte, error)                               { return nil, errNoWindow }
func (noSurface) Animating() bool                                           { return false }
func (noSurface) Resize(driver.Size) error                                  { return errNoWindow }
func (noSurface) SetEnvironment(driver.Environment) error                   { return errNoWindow }
func (noSurface) Sync()                                                     {}

// liveTB is the testing.TB the agent's models run steps with: what fails a step panics with a liveFailure, which try turns into the request's error, as tsxtest's appTB does for a TSX test's steps.
type liveTB struct{ testing.TB }

// liveFailure is why a step could not be taken.
type liveFailure struct {
	code automation.Code
	msg  string
}

func (*liveTB) Helper()             {}
func (*liveTB) Name() string        { return "automation" }
func (*liveTB) Cleanup(func())      {}
func (*liveTB) Log(...any)          {}
func (*liveTB) Logf(string, ...any) {}
func (*liveTB) Failed() bool        { return false }
func (*liveTB) Skipped() bool       { return false }
func (*liveTB) Fatalf(format string, args ...any) {
	fail(automation.Failed, fmt.Sprintf(format, args...))
}
func (*liveTB) Fatal(args ...any) { fail(automation.Failed, fmt.Sprint(args...)) }
func (*liveTB) Errorf(format string, args ...any) {
	fail(automation.Failed, fmt.Sprintf(format, args...))
}
func (*liveTB) Error(args ...any) { fail(automation.Failed, fmt.Sprint(args...)) }
func (*liveTB) FailNow()          { fail(automation.Failed, "the step failed") }
func (*liveTB) Skipf(format string, args ...any) {
	fail(automation.Unsupported, fmt.Sprintf(format, args...))
}
func (*liveTB) Skip(args ...any) { fail(automation.Unsupported, fmt.Sprint(args...)) }
func (*liveTB) SkipNow()         { fail(automation.Unsupported, "the platform cannot take the step") }

func fail(code automation.Code, msg string) { panic(liveFailure{code, msg}) }

// try takes a step, returning why it could not be: the first line of its failure, without the model's dump. A step that panics otherwise fails, rather than ending the app it acts on.
func (*liveTB) try(step func()) (err *automation.Error) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		f, ok := v.(liveFailure)
		if !ok {
			f = liveFailure{automation.Failed, fmt.Sprintf("the step panicked: %v", v)}
		}
		msg, _, _ := strings.Cut(strings.TrimPrefix(f.msg, "nsgotest: "), "\n")
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		err = &automation.Error{Code: f.code, Message: msg}
	}()
	step()
	return nil
}
