package core

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAnchoredOverlayFollowsVisibleScrollOffset(t *testing.T) {
	for _, tc := range []struct {
		name     string
		offset   Point
		animated bool
		want     Points
	}{
		{name: "immediate", offset: Point{Y: 20}, want: 100},
		{name: "clamped", offset: Point{X: 800, Y: 800}, want: 80},
		{name: "animated", offset: Point{Y: 40}, animated: true, want: 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var scroll, anchor Ref
			var moved *Signal[bool]
			h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
				moved = NewSignal(cx, false)
				return View{Children: []Node{
					Scroll{Ref: &scroll, Style: Style{Height: Pt(200)}, Children: []Node{
						View{StyleFunc: func() Style {
							if moved.Get() {
								return Style{Height: Pt(110)}
							}
							return Style{Height: Pt(100)}
						}},
						View{Ref: &anchor, Style: Style{Height: Pt(20)}},
						View{Style: Style{Height: Pt(120)}},
					}},
					Overlay{Anchor: &anchor, Placement: PlaceBottom, TestID: "popup",
						Style: Style{Width: Pt(80), Height: Pt(30)}},
				}}
			})
			h.commit()
			popup := h.find("popup")
			x := popup.frame.Origin.X
			scroll.ScrollTo(tc.offset, tc.animated)
			if tc.animated {
				moved.Set(true)
			}
			check := func(batch *driver.MountBatch, want Points) {
				t.Helper()
				i := slices.IndexFunc(batch.Items, func(it driver.MountItem) bool {
					return it.Kind == driver.ItemFrame && it.ID == popup.id
				})
				if i < 0 || batch.Items[i].Frame.Origin != (Point{X: x, Y: want}) {
					t.Fatalf("popup did not move to (%v, %v) in this batch; frame = %v", x, want, popup.frame)
				}
			}
			check(h.commit(), tc.want)
			if tc.animated {
				h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.n.id, Count: 1, Point: Point{Y: 6}})
				check(h.commit(), 124)
			}
		})
	}
}

func TestAnchoredOverlayFollowsListCompensationInTheSameBatch(t *testing.T) {
	var scroll, anchor Ref
	var keys *Signal[[]int]
	h := newHost(t, Size{Width: 300, Height: 500}, func(cx *Scope) Node {
		initial := make([]int, 100)
		for i := range initial {
			initial[i] = i
		}
		keys = NewSignalFunc(cx, initial, nil)
		return View{Children: []Node{
			List{Ref: &scroll, Style: Style{Height: Pt(400)}, EstimatedSize: 40,
				Count: func() int { return len(keys.Get()) }, Key: func(i int) any { return keys.Get()[i] },
				Row: func(cx *Scope, r ListRow) Node {
					return Dynamic(func(*Scope) Node {
						row := View{Style: Style{Height: Pt(40)}}
						if keys.Get()[r.Index()] == 30 {
							row.Ref = &anchor
						}
						return row
					})
				}},
			Overlay{Anchor: &anchor, Placement: PlaceBottom, TestID: "popup",
				Style: Style{Width: Pt(80), Height: Pt(30)}},
		}}
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.n.id, Point: Point{Y: 1000}})
	h.commit()
	popup := h.find("popup")
	before := popup.frame.Origin
	keys.Set(slices.Concat([]int{-5, -4, -3, -2, -1}, keys.Get()))
	b := h.commit()
	if !slices.ContainsFunc(b.Items, func(it driver.MountItem) bool {
		return it.Kind == driver.ItemScrollBy && it.ID == scroll.n.id && it.Point.Y == 200
	}) {
		t.Fatal("the prepend did not emit a 200 pt compensation")
	}
	for _, it := range slices.Backward(b.Items) {
		if it.Kind == driver.ItemFrame && it.ID == popup.id {
			if got := it.Frame.Origin; got != before {
				t.Fatalf("popup moved from %v to %v during List compensation", before, got)
			}
			break
		}
	}
}

// newOverlays runs app on a platform that opens no windows, as Android's: the first tree is the app's view, and the surfaces it opens commit on their own.
func newOverlays(t *testing.T, os OS, app func(cx *Scope) Node) *windows {
	w := &windows{t: t, items: map[uint32][]driver.MountItem{}}
	w.root = testRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0))})
	w.app = NewTree(w.root, ApproxTextEngine{})
	w.app.SetEnvironment(Environment{OS: os})
	w.app.Mount(app)
	w.commit()
	return w
}

// denyOverlays is an Access that grants everything but AccessOverlay.
type denyOverlays struct{}

func (denyOverlays) Allows(req AccessRequest) bool { return req.Permission != AccessOverlay }
func (denyOverlays) WebHosts() []string            { return nil }

func TestOverlayPermission(t *testing.T) {
	var cx *Scope
	w := newOverlays(t, OSIOS, func(c *Scope) Node { cx = c; return View{} })
	got := OverlayGranted
	cx.OverlayPermission(func(p OverlayPermission) { got = p })
	w.commit()
	if got != OverlayUnavailable || len(w.take(0, driver.ItemOverlayPermission)) != 0 {
		t.Fatalf("off Android: %v", got)
	}

	w = newOverlays(t, OSAndroid, func(c *Scope) Node { cx = c; return View{} })
	cx.RequestOverlayPermission(func(p OverlayPermission) { got = p })
	w.commit()
	asked := w.take(0, driver.ItemOverlayPermission)
	if len(asked) != 1 || !asked[0].On {
		t.Fatalf("asked %+v", asked)
	}
	w.dispatch(0, driver.Event{Kind: driver.EventOverlayPermission, Count: asked[0].Count, Index: driver.OverlayGranted})
	if got != OverlayGranted {
		t.Fatalf("answered %v", got)
	}

	var denied *Window
	w = newOverlays(t, OSAndroid, func(c *Scope) Node {
		c.ProvideAccess(denyOverlays{})
		c.OverlayPermission(func(p OverlayPermission) { got = p })
		denied = c.OpenOverlay(OverlayOptions{}, nil)
		return View{}
	})
	if got != OverlayUnavailable || denied != nil || len(w.take(0, driver.ItemOverlayPermission)) != 0 {
		t.Errorf("under an Access without overlays: %v, %v", got, denied)
	}
}

func TestOverlaySurface(t *testing.T) {
	var cx *Scope
	var overlay *Window
	w := newOverlays(t, OSAndroid, func(c *Scope) Node {
		cx = c
		overlay = c.OpenOverlay(OverlayOptions{Frame: WindowFrame{X: 12, Y: 18, Move: true, Width: 240, Height: 120},
			Focusable: true, Accessible: true}, func(*Scope) Node { return Text{Content: "floating"} })
		return View{}
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 1 || opened[0].Surface.Kind != driver.SurfaceOverlay {
		t.Fatalf("opened %+v", opened)
	}
	cfg := opened[0].Surface
	if cfg.Width != 240 || cfg.Height != 120 || cfg.Frame != (WindowFrame{X: 12, Y: 18, Move: true, Width: 240, Height: 120}) ||
		!cfg.Focusable || cfg.Passive || !cfg.Accessible || cfg.Hidden {
		t.Fatalf("config %+v", cfg)
	}
	if texts := w.texts(cfg.ID); len(texts) != 1 || texts[0] != "floating" {
		t.Fatalf("its tree shows %q", texts)
	}
	if again := cx.OpenOverlay(OverlayOptions{}, nil); again != nil {
		t.Error("a second overlay opened while one is open")
	}

	// Moving, hiding and showing are the window commands every surface takes.
	inside := w.app.Surfaces()[0]
	inside.root.SetWindowFrame(WindowFrame{X: 40, Y: 60, Move: true})
	overlay.Hide()
	overlay.Show()
	w.commit()
	var actions []driver.WindowAction
	for _, it := range w.take(cfg.ID, driver.ItemWindow) {
		actions = append(actions, it.Window.Action)
	}
	if len(actions) != 3 || actions[0] != driver.WindowSetFrame || actions[1] != driver.WindowHide || actions[2] != driver.WindowShow {
		t.Fatalf("window commands %v", actions)
	}

	overlay.Close()
	w.commit()
	if len(w.take(cfg.ID, driver.ItemWindow)) != 1 || !overlay.Closed() {
		t.Fatal("closing it sent no destroy")
	}
	if next := cx.OpenOverlay(OverlayOptions{Passive: true, Focusable: true}, nil); next == nil {
		t.Fatal("no overlay opens after the last closed")
	}
	w.commit()
	if cfg := w.take(0, driver.ItemOpenSurface)[0].Surface; !cfg.Passive || cfg.Focusable || cfg.Width != 320 || cfg.Height != 180 || cfg.Frame.Move {
		t.Errorf("a passive overlay %+v", cfg)
	}
}

func TestOverlayEnds(t *testing.T) {
	var ends []OverlayEnd
	var overlay *Window
	w := newOverlays(t, OSAndroid, func(c *Scope) Node {
		overlay = c.OpenOverlay(OverlayOptions{OnEnd: func(e OverlayEnd) { ends = append(ends, e) }}, nil)
		return View{}
	})
	id := w.take(0, driver.ItemOpenSurface)[0].Surface.ID
	w.dispatch(id, driver.Event{Kind: driver.EventWindowEnded, Index: driver.OverlayEndRevoked})
	if !overlay.Closed() || len(ends) != 1 || ends[0] != OverlayEndRevoked || ends[0].String() != "revoked" {
		t.Fatalf("revoked: closed %v, ends %v", overlay.Closed(), ends)
	}
	if len(w.take(id, driver.ItemWindow)) != 1 {
		t.Error("the platform heard no destroy")
	}
	if len(w.app.Surfaces()) != 0 {
		t.Error("the engine kept the ended tree")
	}

	// The app closing it, or its owner going, is no end of the platform's.
	ends = nil
	var owner *Scope
	var shown *Signal[bool]
	w = newOverlays(t, OSAndroid, func(c *Scope) Node {
		shown = NewSignal(c, true)
		return Show{When: shown.Get, Then: func(cx *Scope) Node {
			owner = cx
			overlay = cx.OpenOverlay(OverlayOptions{OnEnd: func(e OverlayEnd) { ends = append(ends, e) }}, nil)
			return View{}
		}}
	})
	if overlay == nil || owner == nil {
		t.Fatal("no overlay opened")
	}
	shown.Set(false)
	w.commit()
	if !overlay.Closed() || len(ends) != 0 {
		t.Fatalf("its owner went: closed %v, ends %v", overlay.Closed(), ends)
	}
}

func TestOverlayLifecycle(t *testing.T) {
	var app, inside *Env
	var overlay *Window
	w := newOverlays(t, OSAndroid, func(c *Scope) Node {
		app = c.Env()
		overlay = c.OpenOverlay(OverlayOptions{Hidden: true}, func(cx *Scope) Node {
			inside = cx.Env()
			return View{}
		})
		return View{}
	})
	id := w.take(0, driver.ItemOpenSurface)[0].Surface.ID
	if overlay.Visible() || inside == nil {
		t.Fatalf("a hidden overlay: visible %v, its tree %v", overlay.Visible(), inside)
	}

	// The overlay shows and hides through Visible, and the platform hiding it tells it too.
	overlay.Show()
	w.commit()
	if !overlay.Visible() {
		t.Fatal("shown, it is not visible")
	}
	w.dispatch(id, driver.Event{Kind: driver.EventWindowHidden})
	if overlay.Visible() {
		t.Fatal("hidden by the platform, it is still visible")
	}

	// Its tree's lifecycle is its window's, apart from the app's view's.
	tree := w.app.Surfaces()[0]
	tree.SetEnvironment(Environment{OS: OSAndroid, Lifecycle: LifecycleInactive})
	w.commit()
	if inside.Lifecycle() != LifecycleInactive || app.Lifecycle() != LifecycleActive {
		t.Fatalf("without the focus: overlay %v, app %v", inside.Lifecycle(), app.Lifecycle())
	}
	tree.SetEnvironment(Environment{OS: OSAndroid, Lifecycle: LifecycleActive})
	w.commit()
	if inside.Lifecycle() != LifecycleActive {
		t.Fatalf("with the focus: %v", inside.Lifecycle())
	}
}

func TestExplicitScopeOwnsOverlay(t *testing.T) {
	var appScope *Scope
	var shown *Signal[bool]
	var overlay *Window
	w := newOverlays(t, OSAndroid, func(app *Scope) Node {
		appScope = app
		shown = NewSignal(app, true)
		return Show{When: shown.Get, Then: func(branch *Scope) Node {
			return View{
				TestID: "open",
				Style:  Style{Width: Pt(40), Height: Pt(20)},
				OnPress: func(PressEvent) {
					overlay = app.OpenOverlay(OverlayOptions{}, func(*Scope) Node { return Text{Content: "floating"} })
				},
			}
		}}
	})
	var btn NodeID
	for _, n := range w.app.nodes {
		if n != nil && n.props.TestID == "open" {
			btn = n.id
			break
		}
	}
	w.dispatch(0, driver.Event{Kind: driver.EventPress, ID: btn})
	if overlay == nil || overlay.Closed() {
		t.Fatal("overlay did not open")
	}
	shown.Set(false)
	w.commit()
	if overlay.Closed() {
		t.Fatal("overlay opened with app scope closed when button branch disposed")
	}
	appScope.Dispose()
	w.commit()
	if !overlay.Closed() {
		t.Fatal("overlay survived app scope disposal")
	}
	if appScope.OpenOverlay(OverlayOptions{}, nil) != nil {
		t.Fatal("OpenOverlay on a disposed scope returned a handle")
	}
}

// TestScenesShowTheMainView opens the scenes a platform connects beyond the app's first, as iPadOS does for each window the user opens: each is a tree of its own showing the main view, the scenes share the app's signals, and the platform ends one as the system discards it.
func TestScenesShowTheMainView(t *testing.T) {
	var shared *Signal[string]
	var built int
	root := func(cx *Scope) Node {
		built++
		return Text{ContentFunc: func() string { return shared.Get() }}
	}
	for _, test := range []struct {
		name string
		app  func(cx *Scope) Node
	}{
		{"root view", func(cx *Scope) Node {
			if shared == nil {
				shared = NewSignal(cx, "one")
			}
			return root(cx)
		}},
		{"main view", AppRoot(func(cx *Scope) {
			shared = NewSignal(cx, "one")
			cx.SetMainView(root)
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			shared, built = nil, 0
			w := newOverlays(t, OSIOS, test.app)
			w.dispatch(0, driver.Event{Kind: driver.EventOpenScene, Count: 7})
			opened := w.take(0, driver.ItemOpenSurface)
			if len(opened) != 1 || opened[0].Surface.Kind != driver.SurfaceScene || opened[0].Surface.Scene != 7 || opened[0].Surface.ID == 0 {
				t.Fatalf("opened %+v", opened)
			}
			id := opened[0].Surface.ID
			if built != 2 {
				t.Fatalf("the main view was built %d times, want once a scene", built)
			}
			if texts := w.texts(id); len(texts) != 1 || texts[0] != "one" {
				t.Fatalf("the scene shows %q", texts)
			}
			shared.Set("two")
			w.commit()
			for _, s := range []uint32{0, id} {
				if texts := w.texts(s); len(texts) == 0 || texts[len(texts)-1] != "two" {
					t.Fatalf("surface %d shows %q after the shared signal changed", s, texts)
				}
			}
			w.dispatch(id, driver.Event{Kind: driver.EventWindowEnded})
			if len(w.take(id, driver.ItemWindow)) != 1 || len(w.app.Surfaces()) != 0 {
				t.Fatal("ending the scene destroyed no tree")
			}
			if texts := w.texts(0); len(texts) == 0 || texts[len(texts)-1] != "two" {
				t.Fatalf("the first scene shows %q after the other ended", texts)
			}
		})
	}
}

// TestScenesOpenNoWindowsOnDesktops checks that a platform that opens the app's windows opens no tree for a scene: the app opens what it shows there.
func TestScenesOpenNoWindowsOnDesktops(t *testing.T) {
	w := newWindows(t, func(cx *Scope) { cx.SetMainView(func(*Scope) Node { return View{} }) })
	before := len(w.app.Surfaces())
	w.take(0, driver.ItemOpenSurface)
	w.dispatch(0, driver.Event{Kind: driver.EventOpenScene, Count: 1})
	if len(w.app.Surfaces()) != before || len(w.take(0, driver.ItemOpenSurface)) != 0 {
		t.Fatal("a scene opened a window where the app opens its own")
	}
}
