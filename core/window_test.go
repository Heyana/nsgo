package core

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// windows runs an app on a platform that opens windows, as the engine
// does: the first tree is the app's own, and each surface opened commits
// on its own.
type windows struct {
	t    *testing.T
	root *Scope
	app  *Tree
	// items are the items each surface was sent, by ID.
	items map[uint32][]driver.MountItem
}

func newWindows(t *testing.T, app func(cx *Scope)) *windows {
	return newWindowsWith(t, WindowOptions{Width: 400, Height: 300}, app)
}

// newWindowsWith runs app as newWindows does, its main window opening with main.
func newWindowsWith(t *testing.T, main WindowOptions, app func(cx *Scope)) *windows {
	w := &windows{t: t, items: map[uint32][]driver.MountItem{}}
	w.root = testRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0)),
		Windows: &main})
	w.app = NewTree(w.root, ApproxTextEngine{})
	w.app.Mount(AppRoot(app))
	w.commit()
	return w
}

// commit commits every surface, as the engine does, keeping their items.
func (w *windows) commit() {
	trees := append([]*Tree{w.app}, w.app.Surfaces()...)
	for _, t := range trees {
		if t.Closed() {
			w.items[t.ID()] = append(w.items[t.ID()], driver.MountItem{Kind: driver.ItemWindow,
				Window: &driver.WindowCommand{Action: driver.WindowDestroy}})
			t.Forget()
			continue
		}
		if b := t.Commit(); b != nil {
			w.items[t.ID()] = append(w.items[t.ID()], b.Items...)
		}
	}
}

// take returns the items surface id was sent of kind, and forgets them.
func (w *windows) take(id uint32, kind driver.ItemKind) []driver.MountItem {
	var out, kept []driver.MountItem
	for _, it := range w.items[id] {
		if it.Kind == kind {
			out = append(out, it)
		} else {
			kept = append(kept, it)
		}
	}
	w.items[id] = kept
	return out
}

// texts returns the texts surface id shows.
func (w *windows) texts(id uint32) []string {
	var out []string
	for _, it := range w.items[id] {
		if it.Kind == driver.ItemText {
			out = append(out, it.Text.Text)
		}
	}
	return out
}

func (w *windows) dispatch(id uint32, ev driver.Event) {
	t := w.app
	if id != 0 {
		i := slices.IndexFunc(w.app.Surfaces(), func(t *Tree) bool { return t.ID() == id })
		if i < 0 {
			w.t.Fatalf("no surface %d", id)
		}
		t = w.app.Surfaces()[i]
	}
	t.Dispatch(ev)
	w.commit()
}

func TestWindowsShareSignals(t *testing.T) {
	var count *Signal[int]
	var settings *Window
	w := newWindows(t, func(cx *Scope) {
		count = NewSignal(cx, 1)
		label := func(cx *Scope) Node {
			return Text{ContentFunc: func() string { return "count " + string(rune('0'+count.Get())) }}
		}
		cx.SetMainView(label)
		settings = cx.OpenWindow(WindowOptions{Title: "Settings", Width: 200, Height: 100}, label)
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 2 || opened[0].Surface.Width != 400 || opened[1].Surface.Title != "Settings" ||
		opened[1].Surface.Width != 200 {
		t.Fatalf("opened %+v", opened)
	}
	if w.texts(1)[0] != "count 1" || w.texts(2)[0] != "count 1" {
		t.Fatalf("texts %q %q", w.texts(1), w.texts(2))
	}
	if w.app.Surfaces()[1].Size() != (Size{Width: 200, Height: 100}) {
		t.Error("a window does not start at its size")
	}
	w.items = map[uint32][]driver.MountItem{}
	count.Set(2)
	w.commit()
	if w.texts(1)[0] != "count 2" || w.texts(2)[0] != "count 2" {
		t.Errorf("after a change: %q %q", w.texts(1), w.texts(2))
	}
	if settings.CurrentWindowOf() != settings {
		t.Error("a window's content is not in it")
	}
}

func TestChildWindowClosesWithParent(t *testing.T) {
	var parent, child, other *Window
	var childClosing bool
	w := newWindows(t, func(cx *Scope) {
		parent = cx.OpenWindow(WindowOptions{}, nil)
		child = cx.Child().OpenWindow(WindowOptions{Parent: parent}, func(cx *Scope) Node {
			cx.CurrentWindow().OnClosing(cx, func(*WindowClosing) { childClosing = true })
			return Text{Content: "Child"}
		})
		other = cx.OpenWindow(WindowOptions{}, nil)
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 3 || opened[1].Surface.Parent != opened[0].Surface.ID || opened[2].Surface.Parent != 0 {
		t.Fatalf("unexpected surface relationships: %+v", opened)
	}
	w.dispatch(opened[0].Surface.ID, driver.Event{Kind: driver.EventWindowClose})
	if !parent.Closed() || !child.Closed() || childClosing || other.Closed() {
		t.Fatalf("closed parent %v, child %v, other %v; asked child %v", parent.Closed(), child.Closed(), other.Closed(), childClosing)
	}
	if destroyed := w.take(opened[1].Surface.ID, driver.ItemWindow); len(destroyed) != 1 || destroyed[0].Window.Action != driver.WindowDestroy {
		t.Fatalf("child's commands: %+v", destroyed)
	}
	orphan := w.root.OpenWindow(WindowOptions{Parent: parent}, nil)
	w.commit()
	if got := w.take(0, driver.ItemOpenSurface); orphan.Closed() || len(got) != 1 || got[0].Surface.Parent != 0 {
		t.Fatalf("opening with a closed parent produced %+v", got)
	}
}

// A handler's decision reads back through Canceled, as the close button's request goes on to close the window or not.
func TestWindowClosingReportsCanceled(t *testing.T) {
	var seen []bool
	cancel := true
	w := newWindows(t, func(cx *Scope) {
		cx.SetMainView(func(cx *Scope) Node {
			win := cx.CurrentWindow()
			win.OnClosing(cx, func(c *WindowClosing) {
				seen = append(seen, c.Canceled())
				if cancel {
					c.Cancel()
				}
				seen = append(seen, c.Canceled())
			})
			return Text{Content: "main"}
		})
	})
	main := w.root.MainWindow()
	w.dispatch(1, driver.Event{Kind: driver.EventWindowClose})
	if len(seen) != 2 || seen[0] || !seen[1] || main.Closed() {
		t.Fatalf("a canceling handler saw %v, closed %v", seen, main.Closed())
	}
	cancel, seen = false, nil
	w.dispatch(1, driver.Event{Kind: driver.EventWindowClose})
	if len(seen) != 2 || seen[0] || seen[1] || !main.Closed() {
		t.Fatalf("a passing handler saw %v, closed %v", seen, main.Closed())
	}
	if (*WindowClosing)(nil).Canceled() {
		t.Fatal("a nil request reports canceled")
	}
}

// CurrentWindowOf returns the window w's content finds as its own.
func (w *Window) CurrentWindowOf() *Window { return w.scope.CurrentWindow() }

func TestWindowCloseEndsTheAppByMode(t *testing.T) {
	for _, tc := range []struct {
		mode  ShutdownMode
		close uint32
		quits bool
	}{
		{ShutdownOnLastWindowClose, 2, false},
		{ShutdownOnMainWindowClose, 2, false},
		{ShutdownOnMainWindowClose, 1, true},
		{ShutdownOnExplicit, 1, false},
	} {
		w := newWindows(t, func(cx *Scope) {
			cx.SetShutdownMode(tc.mode)
			cx.SetMainView(func(cx *Scope) Node { return Text{Content: "main"} })
			cx.OpenWindow(WindowOptions{}, func(cx *Scope) Node { return Text{Content: "other"} })
		})
		w.dispatch(tc.close, driver.Event{Kind: driver.EventWindowClose})
		if d := w.take(tc.close, driver.ItemWindow); len(d) != 1 || d[0].Window.Action != driver.WindowDestroy {
			t.Errorf("mode %d closing %d: %+v", tc.mode, tc.close, d)
		}
		if quits := len(w.take(0, driver.ItemQuit)) > 0; quits != tc.quits {
			t.Errorf("mode %d closing %d: quits %v", tc.mode, tc.close, quits)
		}
	}
	w := newWindows(t, func(cx *Scope) {
		cx.SetMainView(func(cx *Scope) Node { return Text{Content: "main"} })
	})
	w.dispatch(1, driver.Event{Kind: driver.EventWindowClose})
	if len(w.take(0, driver.ItemQuit)) != 1 {
		t.Error("the last window's close does not end the app")
	}
}

func TestWindowClosingCancels(t *testing.T) {
	var main *Window
	w := newWindows(t, func(cx *Scope) {
		cx.SetMainView(func(cx *Scope) Node {
			win := cx.CurrentWindow()
			win.OnClosing(cx, func(c *WindowClosing) {
				c.Cancel()
				win.Hide()
			})
			return Text{Content: "main"}
		})
		main = cx.MainWindow()
	})
	w.dispatch(1, driver.Event{Kind: driver.EventWindowClose})
	cmds := w.take(1, driver.ItemWindow)
	if len(cmds) != 1 || cmds[0].Window.Action != driver.WindowHide || main.Closed() || main.Visible() {
		t.Fatalf("a canceled close: %+v closed %v", cmds, main.Closed())
	}
	if len(w.take(0, driver.ItemQuit)) != 0 {
		t.Error("a canceled close ended the app")
	}
	w.dispatch(0, driver.Event{Kind: driver.EventReopen})
	if cmds := w.take(1, driver.ItemWindow); len(cmds) != 1 || cmds[0].Window.Action != driver.WindowOrderFront ||
		!main.Visible() {
		t.Errorf("reopening does not show the main window: %+v", cmds)
	}
}

// TestWindowOutlivesTheBatchThatOpenedIt: an app opens its windows from
// code the engine runs inside a batch (a click that opens an editor window),
// and that batch ends the moment the content it built is rebuilt. A window
// that belonged to the batch would vanish with it, so it belongs to the
// scope OpenWindow is called on instead.
func TestWindowOutlivesTheBatchThatOpenedIt(t *testing.T) {
	var cx0 *Scope
	w := newWindows(t, func(cx *Scope) {
		cx0 = cx
		cx.SetShutdownMode(ShutdownOnExplicit)
	})
	// A batch owner, as a build has while it runs code the app wrote.
	batch := cx0.Child()
	cx0.rt.owner = batch
	win := cx0.OpenWindow(WindowOptions{Key: "editor"}, nil)
	cx0.rt.owner = nil
	w.commit()
	if win == nil || win.Closed() {
		t.Fatal("the window did not open")
	}
	batch.Dispose()
	w.commit()
	if win.Closed() {
		t.Error("the window closed with the batch that opened it, which is what makes a password window go away when a field is clicked")
	}
	// It still closes with the scope it was opened on.
	cx0.Dispose()
	w.commit()
	if !win.Closed() {
		t.Error("the window outlived the scope it was opened on")
	}
}

func TestWindowKeyAndOwner(t *testing.T) {
	var cx0 *Scope
	var owner *Scope
	w := newWindows(t, func(cx *Scope) {
		cx0 = cx
		cx.SetShutdownMode(ShutdownOnExplicit)
	})
	a := cx0.OpenWindow(WindowOptions{Key: "settings"}, nil)
	b := cx0.OpenWindow(WindowOptions{Key: "settings"}, nil)
	if a != b {
		t.Error("a key opened a second window")
	}
	owner = cx0.Child()
	c := owner.OpenWindow(WindowOptions{}, nil)
	w.commit()
	if len(w.take(0, driver.ItemOpenSurface)) != 2 {
		t.Error("windows opened")
	}
	owner.Dispose()
	w.commit()
	if !c.Closed() || len(w.take(c.t.ID(), driver.ItemWindow)) == 0 {
		t.Error("a window outlived its owner")
	}
	if len(w.app.Surfaces()) != 1 {
		t.Errorf("surfaces left: %d", len(w.app.Surfaces()))
	}
	// Methods of a closed or nil window do nothing.
	c.Show()
	var none *Window
	none.Focus()
	if none.Visible() || !none.Closed() {
		t.Error("a nil window shows")
	}
}

func TestTrayAndPanel(t *testing.T) {
	var chosen, clicked int
	var panel *Window
	w := newWindows(t, func(cx *Scope) {
		cx.SetShutdownMode(ShutdownOnExplicit)
		panel = cx.OpenPanel(PanelOptions{HideOnBlur: true}, func(cx *Scope) Node { return Text{Content: "panel"} })
		cx.NewTray(TrayOptions{Icon: []byte{1}, Template: true, Panel: panel,
			Menu: func() []MenuItem { return []MenuItem{{Label: "Quit", OnSelect: func() { chosen++ }}} }})
		cx.NewTray(TrayOptions{View: func(cx *Scope) Node { return Text{Content: "3"} }, OnClick: func() { clicked++ }})
	})
	trays := w.take(0, driver.ItemTray)
	if len(trays) != 2 || !trays[0].Tray.Template || trays[1].Tray.View != 2 {
		t.Fatalf("trays %+v %+v", trays[0].Tray, trays[1].Tray)
	}
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 2 || opened[0].Surface.Kind != driver.SurfacePanel || !opened[0].Surface.Hidden ||
		!opened[0].Surface.HideOnBlur || opened[1].Surface.Kind != driver.SurfaceTrayIcon || opened[1].Surface.Tray != 2 {
		t.Fatalf("opened %+v %+v", opened[0].Surface, opened[1].Surface)
	}
	if w.texts(2)[0] != "3" {
		t.Error("a tray's view does not show")
	}
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: 1})
	if cmds := w.take(1, driver.ItemWindow); len(cmds) != 1 || cmds[0].Window.Action != driver.WindowShow ||
		cmds[0].Window.Anchor != 1 || !panel.Visible() {
		t.Fatalf("a primary click does not show the panel under the icon: %+v", cmds)
	}
	w.dispatch(1, driver.Event{Kind: driver.EventWindowHidden})
	if panel.Visible() {
		t.Error("a hidden panel shows")
	}
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: 1, Index: 1})
	menus := w.take(0, driver.ItemTrayMenu)
	if len(menus) != 1 || menus[0].Tray.ID != 1 || menus[0].Menu[0].Label != "Quit" {
		t.Fatalf("menus %+v", menus)
	}
	w.dispatch(0, driver.Event{Kind: driver.EventMenu, Count: menus[0].Menu[0].ID})
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: 2})
	if chosen != 1 || clicked != 1 {
		t.Errorf("chosen %d clicked %d", chosen, clicked)
	}
}

func TestPanelPlacement(t *testing.T) {
	var panel, settings *Window
	var owner *Scope
	w := newWindows(t, func(cx *Scope) {
		owner = cx
		cx.SetShutdownMode(ShutdownOnExplicit)
		panel = cx.OpenPanel(PanelOptions{NoActivate: true}, func(cx *Scope) Node { return Text{Content: "panel"} })
		cx.OpenPanel(PanelOptions{}, func(cx *Scope) Node { return Text{Content: "plain"} })
		settings = cx.OpenWindow(WindowOptions{Title: "Settings"}, func(cx *Scope) Node { return Text{Content: "settings"} })
		cx.NewTray(TrayOptions{Icon: []byte{1}, Panel: panel})
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 3 || !opened[0].Surface.NoActivate || opened[1].Surface.NoActivate {
		t.Fatalf("NoActivate does not reach the platform: %+v", opened)
	}
	last := func(want driver.WindowAction) driver.WindowCommand {
		t.Helper()
		w.commit()
		cmds := w.take(panel.t.id, driver.ItemWindow)
		if len(cmds) != 1 || cmds[0].Window.Action != want {
			t.Fatalf("commands %+v, want one of action %d", cmds, want)
		}
		return *cmds[0].Window
	}
	var failures []error
	panel.OnPlacementError(owner, func(err error) { failures = append(failures, err) })

	// MoveTo before a show only keeps the placement, which the next show uses.
	caret := Bounds{Origin: Point{X: 100, Y: 200}, Size: Size{Width: 1, Height: 18}}
	if err := panel.MoveTo(caret, PlaceTop); err != nil {
		t.Fatal(err)
	}
	if c := last(driver.WindowSetAnchor); c.AnchorBounds != caret || c.Placement != driver.WindowPlaceTop || panel.Visible() {
		t.Fatalf("MoveTo sent %+v, visible %v", c, panel.Visible())
	}
	panel.Show()
	if c := last(driver.WindowShow); c.AnchorBounds != caret || c.Placement != driver.WindowPlaceTop || !panel.Visible() {
		t.Fatalf("Show after MoveTo sent %+v", c)
	}
	// A point is an anchor too, and PlaceDefault places below it.
	pointer := Bounds{Origin: Point{X: 300, Y: 40}}
	if err := panel.ShowAt(pointer, PlaceDefault); err != nil {
		t.Fatal(err)
	}
	if c := last(driver.WindowShow); c.AnchorBounds != pointer || c.Placement != driver.WindowPlaceBottom {
		t.Fatalf("ShowAt a point sent %+v", c)
	}
	if err := panel.MoveTo(caret, PlaceRight); err != nil {
		t.Fatal(err)
	}
	if c := last(driver.WindowSetAnchor); c.AnchorBounds != caret || c.Placement != driver.WindowPlaceRight {
		t.Fatalf("MoveTo sent %+v", c)
	}
	// Hiding and toggling keep the placement for the platform's own show.
	panel.Toggle()
	last(driver.WindowHide)
	panel.Toggle()
	last(driver.WindowOrderFront)
	panel.Hide()
	last(driver.WindowHide)
	panel.Show()
	if c := last(driver.WindowShow); c.AnchorBounds != caret || c.Placement != driver.WindowPlaceRight {
		t.Fatalf("Show after a toggle sent %+v", c)
	}
	// A tray icon shows the panel under itself, forgetting the placement.
	panel.Hide()
	last(driver.WindowHide)
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: 1})
	if c := last(driver.WindowShow); c.Anchor != 1 || c.Placement != driver.WindowPlaceDefault {
		t.Fatalf("a tray click sent %+v", c)
	}
	panel.Hide()
	last(driver.WindowHide)
	panel.Show()
	if c := last(driver.WindowShow); c.Placement != driver.WindowPlaceDefault || c.AnchorBounds != (Bounds{}) {
		t.Fatalf("Show after the tray's sent %+v", c)
	}

	// A platform that cannot place the panel leaves it hidden and says so.
	if err := panel.ShowAt(caret, PlaceBottom); err != nil {
		t.Fatal(err)
	}
	last(driver.WindowShow)
	w.dispatch(panel.t.id, driver.Event{Kind: driver.EventWindowPlacementFailed, Text: "unsupported"})
	if len(failures) != 1 || failures[0].Error() != "unsupported" || ErrorCode(failures[0]) != CodeUnavailable || panel.Visible() {
		t.Fatalf("placement failures %v, visible %v", failures, panel.Visible())
	}

	// Only panels are placed.
	for _, err := range []error{settings.ShowAt(caret, PlaceBottom), settings.MoveTo(caret, PlaceBottom)} {
		if ErrorCode(err) != CodeUnavailable {
			t.Errorf("placing a window answers %v", err)
		}
	}
	w.commit()
	if cmds := w.take(settings.t.id, driver.ItemWindow); len(cmds) != 0 {
		t.Errorf("placing a window sent %+v", cmds)
	}
	var none *Window
	if err := none.ShowAt(caret, PlaceBottom); err != nil {
		t.Errorf("a nil window answers %v", err)
	}
	none.OnPlacementError(owner, func(error) {})

	// A handler goes with its owner.
	child := owner.Child()
	heard := 0
	panel.OnPlacementError(child, func(error) { heard++ })
	child.Dispose()
	w.dispatch(panel.t.id, driver.Event{Kind: driver.EventWindowPlacementFailed, Text: "again"})
	if heard != 0 || len(failures) != 2 {
		t.Errorf("a disposed owner's handler heard %d failures; the app's heard %d", heard, len(failures))
	}
}

func TestPanelFitsContent(t *testing.T) {
	var panel *Window
	var height *Signal[Points]
	var seen Size
	w := newWindows(t, func(cx *Scope) {
		cx.SetShutdownMode(ShutdownOnExplicit)
		height = NewSignal(cx, Points(100))
		panel = cx.OpenPanel(PanelOptions{Width: 380, Height: 520, FitContent: true}, func(cx *Scope) Node {
			cx.Effect(func() { seen = cx.Env().Window() })
			return View{StyleFunc: func() Style { return Style{Width: Pt(300), Height: Pt(height.Get())} }}
		})
		// Shown as it opens: the size goes ahead of the show.
		panel.Show()
	})
	id := panel.t.id
	fits := func() []driver.WindowCommand {
		t.Helper()
		w.commit()
		var out []driver.WindowCommand
		for _, it := range w.take(id, driver.ItemWindow) {
			out = append(out, *it.Window)
		}
		return out
	}
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 1 || opened[0].Surface.Width != 380 || opened[0].Surface.Height != 520 {
		t.Fatalf("a fitted panel opens at its largest size: %+v", opened)
	}
	cmds := fits()
	if len(cmds) != 2 || cmds[0].Action != driver.WindowFitSize || cmds[1].Action != driver.WindowShow {
		t.Fatalf("the first commit sent %+v, want the size and then the show", cmds)
	}
	want := Size{Width: 300, Height: 100}
	if got := (Size{Width: cmds[0].Frame.Width, Height: cmds[0].Frame.Height}); got != want || seen != want || panel.t.Size() != want {
		t.Fatalf("fitted %v, the content sees %v and the tree has %v; want %v", got, seen, panel.t.Size(), want)
	}
	// The platform attaching at the size it opened with hears the fitted one again, and once it has it, nothing.
	panel.t.Resize(Size{Width: 380, Height: 520}, Geometry{})
	if cmds := fits(); len(cmds) != 1 || cmds[0].Frame.Height != 100 || seen != want {
		t.Fatalf("a stale size from the platform sent %+v, the content sees %v", cmds, seen)
	}
	panel.t.Resize(want, Geometry{})
	if cmds := fits(); len(cmds) != 0 {
		t.Fatalf("the platform's own size sent %+v", cmds)
	}
	// It follows the content as it grows and shrinks, up to its largest size, at which the content fills it.
	for _, c := range []struct{ set, want Points }{{240, 240}, {60, 60}, {900, 520}} {
		height.Set(c.set)
		cmds := fits()
		if len(cmds) != 1 || cmds[0].Action != driver.WindowFitSize || cmds[0].Frame.Height != c.want || seen.Height != c.want {
			t.Fatalf("a content %v tall sent %+v, the content sees %v; want %v", c.set, cmds, seen, c.want)
		}
	}
	if f := panel.t.view.frame; f.Size != (Size{Width: 300, Height: 520}) {
		t.Errorf("the root of a full panel is %v", f)
	}
	// A commit that lays the panel out more than once, as a list filling its rows has it, sends only the last size: the platform applies them in order.
	height.Set(200)
	panel.t.timeLayout()
	height.Set(150)
	if cmds := fits(); len(cmds) != 1 || cmds[0].Frame.Height != 150 || seen.Height != 150 {
		t.Fatalf("a content laid out at 200 and then 150 in one commit sent %+v, the content sees %v", cmds, seen)
	}
}

func TestTransparentPanel(t *testing.T) {
	var clear, plain *Window
	var shown []bool
	w := newWindows(t, func(cx *Scope) {
		cx.SetShutdownMode(ShutdownOnExplicit)
		clear = cx.OpenPanel(PanelOptions{Transparent: true}, func(cx *Scope) Node {
			cx.Effect(func() { shown = append(shown, cx.Env().WindowTransparent()) })
			return Text{Content: "card"}
		})
		plain = cx.OpenPanel(PanelOptions{}, func(cx *Scope) Node { return Text{Content: "plain"} })
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 2 || !opened[0].Surface.Transparent || opened[1].Surface.Transparent {
		t.Fatalf("Transparent does not reach the platform: %+v %+v", opened[0].Surface, opened[1].Surface)
	}
	if plain.t.root.Env().WindowTransparent() {
		t.Error("a panel without Transparent reports it")
	}
	// A platform that could not clear the panel says so.
	clear.t.SetEnvironment(Environment{})
	w.commit()
	if len(shown) != 2 || !shown[0] || shown[1] {
		t.Fatalf("the panel saw transparency %v, want true then false", shown)
	}
	// A panel of the platform's size sends none of its own.
	for _, p := range []*Window{clear, plain} {
		for _, it := range w.take(p.t.id, driver.ItemWindow) {
			if it.Window.Action == driver.WindowFitSize {
				t.Errorf("a panel without FitContent sent %+v", it.Window)
			}
		}
	}
}

// A window opened Transparent reads WindowTransparent from its first frame, as the main window opened so does, until its platform reports otherwise; it is the window's state, apart from CanMakeWindowsTransparent, whether it could be.
func TestTransparentWindowReportsIt(t *testing.T) {
	var pet, plain *Window
	var shown []bool
	w := newWindows(t, func(cx *Scope) {
		cx.SetShutdownMode(ShutdownOnExplicit)
		pet = cx.OpenWindow(WindowOptions{Transparent: true}, func(cx *Scope) Node {
			cx.Effect(func() { shown = append(shown, cx.Env().WindowTransparent()) })
			return View{}
		})
		plain = cx.OpenWindow(WindowOptions{}, func(cx *Scope) Node { return View{} })
	})
	if plain.t.root.Env().WindowTransparent() {
		t.Error("a window without Transparent reports it")
	}
	// The platform framing it again (SetWindowTransparent) says so, and transparent again, while it can be either way.
	pet.t.SetEnvironment(Environment{CanMakeWindowsTransparent: true})
	w.commit()
	pet.t.SetEnvironment(Environment{CanMakeWindowsTransparent: true, WindowTransparent: true})
	w.commit()
	if !slices.Equal(shown, []bool{true, false, true}) || !pet.t.root.Env().CanMakeWindowsTransparent() {
		t.Fatalf("the window saw transparency %v", shown)
	}

	var mainShown []bool
	main := newWindowsWith(t, WindowOptions{Width: 400, Height: 300, Transparent: true}, func(app *Scope) {
		app.SetMainView(func(cx *Scope) Node {
			cx.Effect(func() { mainShown = append(mainShown, cx.Env().WindowTransparent()) })
			return View{}
		})
	})
	main.commit()
	if !slices.Equal(mainShown, []bool{true}) {
		t.Fatalf("the main window opened transparent saw %v", mainShown)
	}
}

func TestHotKeysDockAndLogin(t *testing.T) {
	var cx0, owner *Scope
	fired := 0
	var login []bool
	w := newWindows(t, func(cx *Scope) {
		cx0 = cx
		cx.SetDockVisible(false)
		cx.SetLaunchAtLogin(true)
		cx.LaunchAtLogin(func(on bool) { login = append(login, on) })
	})
	// Outside a running owner, what a scope makes is its own.
	owner = cx0.Child()
	owner.RegisterHotKey("Alt+Space", func() { fired++ })
	w.commit()
	keys := w.take(0, driver.ItemHotKeys)
	if len(keys) != 1 || keys[0].Keys[0].Key != "alt+space" || keys[0].Keys[0].ID == 0 {
		t.Fatalf("keys %+v", keys)
	}
	w.dispatch(0, driver.Event{Kind: driver.EventHotKey, Count: keys[0].Keys[0].ID})
	if fired != 1 {
		t.Error("a hot key did not fire")
	}
	if d := w.take(0, driver.ItemDock); len(d) != 1 || d[0].On {
		t.Errorf("dock %+v", d)
	}
	if s := w.take(0, driver.ItemSetLaunchAtLogin); len(s) != 1 || !s[0].On {
		t.Errorf("launch at login %+v", s)
	}
	asks := w.take(0, driver.ItemLaunchAtLogin)
	w.dispatch(0, driver.Event{Kind: driver.EventLaunchAtLogin, Count: asks[0].Count, Finished: true})
	if !slices.Equal(login, []bool{true}) {
		t.Errorf("login %v", login)
	}
	owner.Dispose()
	w.commit()
	if keys := w.take(0, driver.ItemHotKeys); len(keys) != 1 || len(keys[0].Keys) != 0 {
		t.Errorf("a hot key outlived its owner: %+v", keys)
	}
}

func TestMainViewWithoutWindows(t *testing.T) {
	var cx0 *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, AppRoot(func(cx *Scope) {
		cx0 = cx
		cx.SetMainView(func(cx *Scope) Node { return Text{Content: "one", TestID: "one"} })
		if cx.OpenWindow(WindowOptions{}, nil) != nil || cx.NewTray(TrayOptions{}) != nil {
			t.Error("a platform without windows opened one")
		}
	}))
	h.commit()
	if !h.has("one") {
		t.Fatal("the main view does not show")
	}
	cx0.SetMainView(func(cx *Scope) Node { return Text{Content: "two", TestID: "two"} })
	h.commit()
	if h.has("one") || !h.has("two") {
		t.Error("a later main view does not replace it")
	}
}

// TestHotKeysWithoutWindows: a platform that opens no windows takes global shortcuts on its one surface once it says it registers them, as Linux does.
func TestHotKeysWithoutWindows(t *testing.T) {
	for _, takes := range []bool{false, true} {
		fired := 0
		w := &windows{t: t, items: map[uint32][]driver.MountItem{}}
		w.root = testRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0)), HotKeys: takes})
		w.app = NewTree(w.root, ApproxTextEngine{})
		w.app.Resize(Size{Width: 100, Height: 100}, Geometry{})
		w.app.Mount(AppRoot(func(cx *Scope) {
			cx.SetMainView(func(cx *Scope) Node { return Text{Content: "main"} })
			cx.RegisterHotKey("Ctrl+Alt+K", func() { fired++ })
			if cx.OpenWindow(WindowOptions{}, nil) != nil {
				t.Error("a platform without windows opened one")
			}
		}))
		w.commit()
		keys := w.take(0, driver.ItemHotKeys)
		if !takes {
			if len(keys) != 0 {
				t.Errorf("a platform that takes no shortcuts was sent %+v", keys)
			}
			continue
		}
		if len(keys) != 1 || len(keys[0].Keys) != 1 || keys[0].Keys[0].Key != "ctrl+alt+k" {
			t.Fatalf("keys %+v", keys)
		}
		w.dispatch(0, driver.Event{Kind: driver.EventHotKey, Count: keys[0].Keys[0].ID})
		if fired != 1 {
			t.Error("a hot key did not fire")
		}
		if !slices.Contains(w.texts(0), "main") {
			t.Errorf("the main view does not show beside the shortcut: %v", w.texts(0))
		}
	}
}

// TestHotKeyRegistrationState: a shortcut's handle is pending until the platform answers its ID, follows each answer, a computation reading it included, goes pending again while a change to the set registers it anew, and ignores answers for IDs no longer registered and states out of range; where the platform has no global shortcuts it is unsupported at once.
func TestHotKeyRegistrationState(t *testing.T) {
	var first, second *HotKey
	var scope *Scope
	var seen []HotKeyState
	w := newWindows(t, func(cx *Scope) {
		first = cx.RegisterHotKey("Ctrl+Alt+K", nil)
		scope = cx.Child()
		cx.Effect(func() { seen = append(seen, first.State()) })
	})
	keys := w.take(0, driver.ItemHotKeys)
	if len(keys) != 1 || len(keys[0].Keys) != 1 {
		t.Fatalf("keys %+v", keys)
	}
	id := keys[0].Keys[0].ID
	answer := func(id uint32, state HotKeyState) {
		t.Helper()
		w.dispatch(0, driver.Event{Kind: driver.EventHotKeyState, Count: id, Index: int(state)})
	}
	if got := first.State(); got != HotKeyPending {
		t.Fatalf("before an answer the state is %v, want pending", got)
	}
	answer(id, HotKeyRegistered)
	if got := first.State(); got != HotKeyRegistered {
		t.Fatalf("after registered the state is %v", got)
	}
	w.dispatch(0, driver.Event{Kind: driver.EventHotKeyState, Count: id, Index: 9})
	if got := first.State(); got != HotKeyRegistered {
		t.Errorf("a state out of range changed it to %v", got)
	}

	// Another shortcut sends the whole set again, which the platform registers anew.
	scope.Run(func() { second = scope.RegisterHotKey("Ctrl+Alt+J", nil) })
	w.commit()
	keys = w.take(0, driver.ItemHotKeys)
	if len(keys) != 1 || len(keys[0].Keys) != 2 {
		t.Fatalf("keys after the second shortcut %+v", keys)
	}
	if got := first.State(); got != HotKeyPending {
		t.Errorf("while the set registers anew the first shortcut is %v, want pending", got)
	}
	answer(id, HotKeyRegistered)
	answer(keys[0].Keys[1].ID, HotKeyTaken)
	if got := second.State(); got != HotKeyTaken {
		t.Errorf("the second shortcut is %v, want taken", got)
	}
	if want := []HotKeyState{HotKeyPending, HotKeyRegistered, HotKeyPending, HotKeyRegistered}; !slices.Equal(seen, want) {
		t.Errorf("a computation reading the state saw %v, want %v", seen, want)
	}

	// Its owner gone, an answer for its ID changes nothing.
	gone := keys[0].Keys[1].ID
	scope.Dispose()
	w.commit()
	if keys = w.take(0, driver.ItemHotKeys); len(keys) != 1 || len(keys[0].Keys) != 1 {
		t.Fatalf("keys after the drop %+v", keys)
	}
	answer(id, HotKeyRegistered)
	answer(gone, HotKeyRegistered)
	if got := second.State(); got != HotKeyTaken {
		t.Errorf("an answer for a dropped shortcut changed its handle to %v", got)
	}

	root := NewRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0))})
	if got := root.RegisterHotKey("Ctrl+Alt+K", nil).State(); got != HotKeyUnsupported {
		t.Errorf("without global shortcuts the state is %v, want unsupported", got)
	}
	if got := (*HotKey)(nil).State(); got != HotKeyUnsupported {
		t.Errorf("a nil handle's state is %v, want unsupported", got)
	}
}

func TestTrayWithoutWindows(t *testing.T) {
	root := testRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0)), Tray: true})
	tree := NewTree(root, ApproxTextEngine{})
	var tray *Tray
	tree.Mount(AppRoot(func(cx *Scope) {
		tray = cx.NewTray(TrayOptions{Tooltip: "tray"})
	}))
	b := tree.Commit()
	if tray == nil {
		t.Fatal("tray capability returned nil")
	}
	if b == nil || !slices.ContainsFunc(b.Items, func(it driver.MountItem) bool {
		return it.Kind == driver.ItemTray && it.Tray != nil && it.Tray.Tooltip == "tray"
	}) {
		t.Fatalf("tray mount batch: %+v", b)
	}
	root.Dispose()
}

func TestWindowConstraints(t *testing.T) {
	var cx *Scope
	w := newWindows(t, func(app *Scope) {
		app.OpenWindow(WindowOptions{Width: 1000, Height: 100, Constraints: WindowConstraints{
			MinWidth: 300, MinHeight: 200, MaxWidth: 100, MaxHeight: 800, NoResize: true}},
			func(c *Scope) Node { cx = c; return View{} })
	})
	cfg := w.take(0, driver.ItemOpenSurface)[0].Surface
	want := WindowConstraints{MinWidth: 300, MinHeight: 200, MaxWidth: 300, MaxHeight: 800, NoResize: true, NoMaximize: true}
	if cfg.Constraints != want {
		t.Fatalf("a window opens with %+v; want %+v", cfg.Constraints, want)
	}
	if cfg.Width != 300 || cfg.Height != 200 {
		t.Fatalf("a window opens at %v by %v; want its sizes kept to, at 300 by 200", cfg.Width, cfg.Height)
	}
	cx.SetWindowConstraints(WindowConstraints{MinWidth: -1, MaxHeight: 500, NoMinimize: true})
	w.commit()
	sent := w.take(cfg.ID, driver.ItemWindow)
	if len(sent) != 1 || sent[0].Window.Action != driver.WindowSetConstraints ||
		sent[0].Window.Constraints != (WindowConstraints{MaxHeight: 500, NoMinimize: true}) {
		t.Fatalf("the window was sent %+v", sent)
	}
}

// A window opens on the workspaces its options name, and SetWindowWorkspaces sends the new ones apart from the level, so a later level keeps them.
func TestWindowWorkspaces(t *testing.T) {
	var cx *Scope
	w := newWindows(t, func(app *Scope) {
		app.OpenWindow(WindowOptions{Workspaces: WindowWorkspacesAll}, func(c *Scope) Node { cx = c; return View{} })
		app.OpenWindow(WindowOptions{}, func(*Scope) Node { return View{} })
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 2 || opened[0].Surface.Workspaces != WindowWorkspacesAll ||
		opened[1].Surface.Workspaces != WindowWorkspacesCurrent {
		t.Fatalf("windows opened with %+v", opened)
	}
	cx.SetWindowWorkspaces(WindowWorkspacesCurrent)
	cx.SetWindowLevel(WindowLevelFloating)
	w.commit()
	var got []WindowCommand
	for _, it := range w.take(opened[0].Surface.ID, driver.ItemWindow) {
		got = append(got, *it.Window)
	}
	want := []WindowCommand{{Action: WindowSetWorkspaces, Workspaces: WindowWorkspacesCurrent},
		{Action: WindowSetLevel, Level: WindowLevelFloating}}
	if !slices.Equal(got, want) {
		t.Fatalf("the window was sent %+v; want %+v", got, want)
	}
}

func TestWindowConstraintsClamp(t *testing.T) {
	c := WindowConstraints{MinWidth: 300, MaxHeight: 500}.Normalized()
	for _, tc := range []struct{ in, want Size }{
		{Size{Width: 100, Height: 100}, Size{Width: 300, Height: 100}},
		{Size{Width: 4000, Height: 4000}, Size{Width: 4000, Height: 500}},
	} {
		if got := c.Clamp(tc.in); got != tc.want {
			t.Errorf("Clamp(%v) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

// A window restored at a frame opens there at its size, and reports the
// frame the platform gives it, not the app's own.
func TestWindowFrame(t *testing.T) {
	saved := WindowFrame{X: 40, Y: 50, Move: true, Width: 900, Height: 700}
	var inside *Scope
	w := newWindows(t, func(cx *Scope) {
		cx.OpenWindow(WindowOptions{Frame: saved, Constraints: WindowConstraints{MaxWidth: 800}}, func(cx *Scope) Node {
			inside = cx
			return View{}
		})
		cx.OpenWindow(WindowOptions{Frame: WindowFrame{Width: 900, Height: 700}}, func(cx *Scope) Node { return View{} })
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 2 {
		t.Fatalf("opened %d surfaces", len(opened))
	}
	s := opened[0].Surface
	if s.Frame != saved || s.Width != 800 || s.Height != 700 {
		t.Fatalf("opened %+v", s)
	}
	if s := opened[1].Surface; s.Frame != (WindowFrame{}) || s.Width != 400 || s.Height != 300 {
		t.Fatalf("a frame that does not move opened %+v", s)
	}
	env := inside.Env()
	if env.WindowFrame() != (WindowFrame{}) || env.FullScreen() {
		t.Fatal("a window starts with a frame the platform has not reported")
	}
	moved := WindowFrame{X: 60, Y: 70, Move: true, Width: 800, Height: 700}
	w.app.Surfaces()[0].SetEnvironment(driver.Environment{FullScreen: true, WindowFrame: moved})
	if env.WindowFrame() != moved || !env.FullScreen() {
		t.Fatalf("reported frame %+v", env.WindowFrame())
	}
}

// A window opens transparent, without a shadow and off the taskbar as its options ask, the main window as the app's do, and changes them later.
func TestWindowLook(t *testing.T) {
	var cx *Scope
	w := newWindows(t, func(app *Scope) {
		app.OpenWindow(WindowOptions{Transparent: true, NoShadow: true, SkipTaskbar: true}, func(c *Scope) Node {
			cx = c
			return View{}
		})
		app.OpenWindow(WindowOptions{}, func(*Scope) Node { return View{} })
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 2 {
		t.Fatalf("opened %d surfaces", len(opened))
	}
	if s := opened[0].Surface; !s.Transparent || !s.NoShadow || !s.SkipTaskbar {
		t.Fatalf("a transparent window opened %+v", s)
	}
	if s := opened[1].Surface; s.Transparent || s.NoShadow || s.SkipTaskbar {
		t.Fatalf("a plain window opened %+v", s)
	}
	cx.SetWindowTransparent(false)
	cx.SetWindowShadow(true)
	cx.SetSkipTaskbar(false)
	w.commit()
	var sent []WindowCommand
	for _, it := range w.take(opened[0].Surface.ID, driver.ItemWindow) {
		sent = append(sent, *it.Window)
	}
	want := []WindowCommand{{Action: driver.WindowSetTransparent}, {Action: driver.WindowSetShadow, On: true}, {Action: driver.WindowSetSkipTaskbar}}
	if !slices.Equal(sent, want) {
		t.Fatalf("the window was sent %+v; want %+v", sent, want)
	}

	main := newWindowsWith(t, WindowOptions{Width: 400, Height: 300, Transparent: true, SkipTaskbar: true}, func(app *Scope) {
		app.SetMainView(func(*Scope) Node { return View{} })
	})
	if s := main.take(0, driver.ItemOpenSurface)[0].Surface; !s.Transparent || s.NoShadow || !s.SkipTaskbar {
		t.Fatalf("the main window opened %+v", s)
	}
}

func TestOpenLinksWaitForAHandlerApartFromFiles(t *testing.T) {
	var posted []func()
	root := testRoot(RootOptions{Post: func(fn func()) { posted = append(posted, fn) }, Clock: NewManualClock(time.Unix(0, 0))})
	tree := NewTree(root, ApproxTextEngine{})
	var links, files [][]string
	handle := NewSignal(root, false)
	tree.Mount(func(cx *Scope) Node {
		return Show{When: handle.Get, Then: func(cx *Scope) Node {
			cx.OnOpenLinks(func(l []string) { links = append(links, l) })
			cx.OnOpenFiles(func(p []string) { files = append(files, p) })
			return nil
		}}
	})
	tree.Commit()
	tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"umapp://connect?a=1"}})
	handle.Set(true)
	tree.Commit()
	for len(posted) > 0 {
		fn := posted[0]
		posted = posted[1:]
		fn()
	}
	tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"umapp://b"}})
	tree.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: []string{"/a.flow"}})
	if want := [][]string{{"umapp://connect?a=1"}, {"umapp://b"}}; !slices.EqualFunc(links, want, slices.Equal) {
		t.Errorf("links %q, want %q", links, want)
	}
	if want := [][]string{{"/a.flow"}}; !slices.EqualFunc(files, want, slices.Equal) {
		t.Errorf("files %q, want %q", files, want)
	}
	handle.Set(false)
	tree.Commit()
	tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"umapp://c"}})
	if len(links) != 2 {
		t.Errorf("a disposed handler heard %q", links[2:])
	}
}

func TestOpenFilesWaitForAHandler(t *testing.T) {
	var posted []func()
	root := testRoot(RootOptions{Post: func(fn func()) { posted = append(posted, fn) }, Clock: NewManualClock(time.Unix(0, 0))})
	tree := NewTree(root, ApproxTextEngine{})
	var opened [][]string
	handle := NewSignal(root, false)
	tree.Mount(func(cx *Scope) Node {
		return Show{When: handle.Get, Then: func(cx *Scope) Node {
			cx.OnOpenFiles(func(paths []string) { opened = append(opened, paths) })
			return nil
		}}
	})
	tree.Commit()
	open := func(paths ...string) {
		tree.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: paths})
	}
	open("/a.flow")
	open("/b.flow")
	handle.Set(true)
	tree.Commit()
	for len(posted) > 0 {
		fn := posted[0]
		posted = posted[1:]
		fn()
	}
	open("/c.flow")
	if want := [][]string{{"/a.flow", "/b.flow"}, {"/c.flow"}}; !slices.EqualFunc(opened, want, slices.Equal) {
		t.Errorf("opened %q, want %q", opened, want)
	}
	handle.Set(false)
	tree.Commit()
	open("/d.flow")
	if len(opened) != 2 {
		t.Errorf("a disposed handler heard %q", opened[2:])
	}
}

// A window or a panel opens taking the pointer where the app asked, and SetWindowPointerEvents changes it; code whose Access does not grant overlays keeps the pointer.
func TestWindowPointerEvents(t *testing.T) {
	var inside, held *Scope
	w := newWindows(t, func(cx *Scope) {
		cx.OpenWindow(WindowOptions{PointerEvents: WindowPointerPainted}, func(cx *Scope) Node { inside = cx; return View{} })
		cx.OpenPanel(PanelOptions{PointerEvents: WindowPointerNone}, func(cx *Scope) Node { return View{} })
		denied := cx.Child()
		denied.Run(func() {
			denied.ProvideAccess(denyOverlays{})
			denied.OpenWindow(WindowOptions{PointerEvents: WindowPointerNone}, func(cx *Scope) Node { held = cx; return View{} })
		})
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 3 {
		t.Fatalf("opened %d surfaces", len(opened))
	}
	for i, want := range []WindowPointerEvents{WindowPointerPainted, WindowPointerNone, WindowPointerAuto} {
		if got := opened[i].Surface.PointerEvents; got != want {
			t.Errorf("surface %d opens with pointer events %d; want %d", i, got, want)
		}
	}
	window, heldWindow := opened[0].Surface.ID, opened[2].Surface.ID
	for _, p := range []WindowPointerEvents{WindowPointerNone, WindowPointerAuto, WindowPointerPainted} {
		inside.SetWindowPointerEvents(p)
		held.SetWindowPointerEvents(p)
		w.commit()
		if sent := w.take(window, driver.ItemWindow); len(sent) != 1 || sent[0].Window.Action != driver.WindowSetPointerEvents ||
			sent[0].Window.PointerEvents != p {
			t.Fatalf("setting %d sent %+v", p, windowCommands(sent))
		}
		if sent := w.take(heldWindow, driver.ItemWindow); len(sent) != 1 || sent[0].Window.PointerEvents != WindowPointerAuto {
			t.Fatalf("code held from overlays set %d: %+v", p, windowCommands(sent))
		}
	}
}

// windowCommands are the window commands of items, for a failure's message.
func windowCommands(items []driver.MountItem) []driver.WindowCommand {
	var out []driver.WindowCommand
	for _, it := range items {
		if it.Window != nil {
			out = append(out, *it.Window)
		}
	}
	return out
}

// Env reports whether the window can be transparent and skip the taskbar as its environment says, and a scope reading them follows a change.
func TestWindowLookCapabilities(t *testing.T) {
	var transparent, taskbar []bool
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.Effect(func() { transparent = append(transparent, cx.Env().CanMakeWindowsTransparent()) })
		cx.Effect(func() { taskbar = append(taskbar, cx.Env().CanSkipTaskbar()) })
		return View{}
	})
	h.commit()
	h.tree.SetEnvironment(Environment{CanMakeWindowsTransparent: true})
	h.commit()
	h.tree.SetEnvironment(Environment{CanMakeWindowsTransparent: true, CanSkipTaskbar: true})
	h.commit()
	if !slices.Equal(transparent, []bool{false, true}) || !slices.Equal(taskbar, []bool{false, true}) {
		t.Fatalf("transparent = %v, taskbar = %v", transparent, taskbar)
	}
}
