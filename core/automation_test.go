package core

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// fakeAgent is an automation agent that follows every surface registered and keeps what each reports.
type fakeAgent struct {
	registered []devtools.Surface
	gone       []devtools.Surface
	follows    map[devtools.Surface]*fakeFollower
}

// fakeFollower keeps what one surface reports.
type fakeFollower struct {
	batches []*driver.MountBatch
	events  []driver.Event
}

func (f *fakeFollower) Batch(b *driver.MountBatch) { f.batches = append(f.batches, b) }
func (f *fakeFollower) Event(ev driver.Event)      { f.events = append(f.events, ev) }

func (a *fakeAgent) Registered(s devtools.Surface, _ bool) {
	a.registered = append(a.registered, s)
	f := &fakeFollower{}
	a.follows[s] = f
	s.Follow(f)
}
func (a *fakeAgent) Unregistered(s devtools.Surface) { a.gone = append(a.gone, s) }
func (a *fakeAgent) Do([]byte) []byte                { return nil }

// of returns the surface of graph numbered id the agent heard registered.
func (a *fakeAgent) of(graph, id int) devtools.Surface {
	for _, s := range a.registered {
		if s.Graph() == graph && s.ID() == id {
			return s
		}
	}
	return nil
}

// sent reports whether s reported an item cond holds for.
func (a *fakeAgent) sent(s devtools.Surface, cond func(driver.MountItem) bool) bool {
	f := a.follows[s]
	return f != nil && slices.ContainsFunc(f.batches, func(b *driver.MountBatch) bool { return slices.ContainsFunc(b.Items, cond) })
}

// automate has DevTools register every tree made from now on with a new agent, for the test.
func automate(t *testing.T) *fakeAgent {
	a := &fakeAgent{follows: map[devtools.Surface]*fakeFollower{}}
	devtools.Enabled, devtools.Automation = true, a
	t.Cleanup(func() {
		devtools.Automation = nil
		for _, s := range a.registered {
			devtools.Unregister(s)
		}
		devtools.Enabled = false
	})
	return a
}

func textIs(want string) func(driver.MountItem) bool {
	return func(it driver.MountItem) bool { return it.Kind == driver.ItemText && it.Text.Text == want }
}

func TestAgentFollowsEverySurfaceThroughTheRegistry(t *testing.T) {
	a := automate(t)
	var brief *Window
	w := newWindows(t, func(cx *Scope) {
		cx.SetMainView(func(cx *Scope) Node { return Text{Content: "main"} })
		brief = cx.OpenWindow(WindowOptions{Title: "brief"}, func(cx *Scope) Node { return Text{Content: "brief"} })
	})
	defer w.root.Dispose()
	graph := w.app.rt.dev.graph
	// The app's own surface, which DevTools pages leave out, is registered for the agent, and reports the windows it opens.
	own := a.of(graph, 0)
	if own == nil || !a.sent(own, func(it driver.MountItem) bool { return it.Kind == driver.ItemOpenSurface }) {
		t.Fatal("the app's own surface was not registered, or did not report the windows it opened")
	}
	if n := len(devtools.Surfaces()); n != 2 || slices.Contains(devtools.Surfaces(), own) {
		t.Errorf("DevTools pages list %d surfaces, not the two windows", n)
	}
	var main, other int
	for _, s := range w.app.Surfaces() {
		if s.window == brief {
			other = int(s.ID())
		} else {
			main = int(s.ID())
		}
	}
	// Each window is registered with its own ID, before its first batch.
	if !a.sent(a.of(graph, main), textIs("main")) || !a.sent(a.of(graph, other), textIs("brief")) {
		t.Fatalf("the windows' batches were not reported under their surfaces: %v", a.registered)
	}
	first := w.app.Surfaces()[0]
	first.Dispatch(driver.Event{Kind: driver.EventFocus, ID: 1})
	if got := len(a.follows[a.of(graph, int(first.ID()))].events); got != 1 {
		t.Errorf("the window's event was reported %d times", got)
	}
	closed := a.of(graph, other)
	brief.Close()
	if !slices.Contains(a.gone, closed) {
		t.Errorf("the closed window was not unregistered")
	}
	// A second graph's surfaces share the first's IDs: their graph tells them apart.
	second := newWindows(t, func(cx *Scope) {
		cx.SetMainView(func(cx *Scope) Node { return Text{Content: "second"} })
	})
	defer second.root.Dispose()
	if g := second.app.rt.dev.graph; g == graph || a.of(g, 0) == nil || a.of(g, 0) == own {
		t.Errorf("the second graph's surfaces are not told apart from the first's")
	}
}

func TestAgentKeepsSharedBatches(t *testing.T) {
	a := automate(t)
	root := testRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0))})
	defer root.Dispose()
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 100, Height: 100}, Geometry{})
	var count *Signal[int]
	tree.Mount(func(cx *Scope) Node {
		count = NewSignal(cx, 1)
		return Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }}
	})
	first := tree.CommitShared()
	kept := slices.Clone(first.Items)
	count.Set(2)
	tree.CommitShared()
	// A platform that applies batches at once gets the tree's own items, which the agent keeps past the next commit.
	same := func(x, y driver.MountItem) bool { return x.Kind == y.Kind && x.ID == y.ID && x.Text == y.Text }
	f := a.follows[a.of(root.rt.dev.graph, 0)]
	if f == nil || !slices.EqualFunc(first.Items, kept, same) || len(f.batches) != 2 || f.batches[0] != first {
		t.Error("a reported batch changed after the next commit: its items are not copies")
	}
}
