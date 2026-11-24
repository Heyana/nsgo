package nsgotest

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/automation"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// mainRan is set once Main or MainFunc hands the main thread to the platform, and opened once a test opened its app there: a process shows one.
var mainRan, opened bool

// Main runs the package's tests, from TestMain:
//
//	func TestMain(m *testing.M) { nsgotest.Main(m) }
//
// It is MainFunc(m.Run), and like it never returns.
func Main(m *testing.M) {
	MainFunc(m.Run)
}

// MainFunc runs the package's tests through run, from TestMain, and ends the process with the code run returns. run calls m.Run, with whatever setup and teardown the package needs around it, deferred calls included:
//
//	func TestMain(m *testing.M) {
//		nsgotest.MainFunc(func() int {
//			setup()
//			defer teardown()
//			return m.Run()
//		})
//	}
//
// Headless it is os.Exit(run()). Under `nsgo test -target` it gives the main thread to the platform, as macOS, iOS and Windows need their windows' loop there, and runs run beside it; the process ends only once run has returned, so its teardown completes first. A target runs each test in a process of its own, so there run, and its setup and teardown, run once per test.
func MainFunc(run func() int) {
	if h := testhost.Current(); onTarget() && h != nil {
		mainRan = true
		h.Main(run)
		return
	}
	os.Exit(run())
}

func init() { testhost.PostToTest = func(app any, fn func()) { app.(*App).postTest(fn) } }

// onTarget reports whether `nsgo test -target` runs the tests.
func onTarget() bool { return os.Getenv("NSGO_TEST_TARGET") != "" }

// Target is the platform `nsgo test -target` runs the tests on, such as
// "macos" or "ios_simulator", or "" headless.
func Target() string { return os.Getenv("NSGO_TEST_TARGET") }

// target drives an app shown on a platform: the model follows the
// batches the app's engine sends and the events the platform reports,
// and steps act with the platform's input where the model says a view
// is.
type target struct {
	a       *App
	surface testhost.Surface
	// source is the first native surface, which owns the engine's barrier.
	source testhost.Surface
	// owner is the target that follows all of this app's surfaces.
	owner *target
	// live is set for a running app's surface the automation follows (automation.go), whose model also drops the alerts the user answers.
	live bool
	// quiet reports whether the last settle came to quiet before settleWait.
	quiet bool
	// syncing is the one outstanding engine barrier, retained when a settle times out so a blocked engine never accumulates waiters.
	syncing chan struct{}

	mu sync.Mutex
	// updates preserves arrival order: a native edit must precede the controlled value the app commits in response to it.
	updates []targetUpdate
	// lost says why a live model stopped following its surface: a batch it could not apply. It queues nothing after.
	lost string
	// transitionsEnding is the engine's TransitionEnds, which it sets as the app starts; mu guards it.
	transitionsEnding func() bool
	// appScope is the scope the platform renders the app under, on the engine's graph, which App.Post posts to; a.root is the test's own graph, for storage and the model's callbacks. The engine's goroutine sets it, which may be after Open returns, as on Windows and Android.
	appScope *core.Scope
	// windowed is set where the host has the app open its windows itself (testhost.WindowHost): the app's own surface, 0, then shows no view, and the model follows the main window, numbered main once its opening came, 0 before and once it closed. The own surface's batches apply aside, for what they ask of the platform and the application menu, which ownMenu keeps. mu guards main.
	windowed bool
	main     uint32
	ownMenu  []driver.MenuEntry
	// held are the keys KeyDown holds on the surface, as the test named them, by the name keyName gives them, so a KeyUp of a key not held fails the step on every platform, as it does headless, and the test's end lets them come up (releaseHeld).
	held map[string]string
	// heldCodes are the physical positions KeyCodeDown holds, with the key name each was pressed with, so KeyCodeUp of a position not held fails and cleanup releases them.
	heldCodes map[string]string
	// heldButtons are the mouse buttons MouseDown holds on the surface, with the last point pressed, so MouseUp of a button not held fails and cleanup releases them.
	heldButtons map[driver.MouseButton]driver.Point
}

// targetUpdate is a batch or an event reported by the named surface.
type targetUpdate struct {
	surface uint32
	batch   *driver.MountBatch
	aside   bool
	main    bool
	closed  bool
	event   driver.Event
}

// observe queues the next batch of a surface, including the app's own
// surface, which puts its menu and app-wide services in the model.
func (t *target) observe(surface uint32, b *driver.MountBatch) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lost != "" || !t.windowed && surface != 0 {
		return
	}
	u := targetUpdate{surface: surface, batch: b, aside: t.windowed && surface == 0}
	if u.aside {
		for _, it := range b.Items {
			if it.Kind == driver.ItemOpenSurface && it.Surface != nil && it.Surface.Main {
				t.main = it.Surface.ID
			}
		}
	} else {
		u.main = !t.windowed || surface == t.main
		u.closed = destroys(b)
		if u.closed && u.main {
			t.main = 0
		}
	}
	t.updates = append(t.updates, u)
}

// destroys reports whether b is the last batch of an opened surface, which the engine sends as its tree goes.
func destroys(b *driver.MountBatch) bool {
	return slices.ContainsFunc(b.Items, func(it driver.MountItem) bool {
		return it.Kind == driver.ItemWindow && it.Window != nil && it.Window.Action == driver.WindowDestroy
	})
}

// surfaceEvent queues edits and input reports alongside the surface's batches.
func (t *target) surfaceEvent(surface uint32, ev driver.Event) {
	switch ev.Kind {
	case driver.EventScroll, driver.EventFocus, driver.EventBlur, driver.EventChangeText, driver.EventSelectionChange:
	case driver.EventAlert:
		if !t.live {
			return
		}
	default:
		return
	}
	t.mu.Lock()
	if t.lost == "" && (t.live || t.windowed == (surface != 0)) {
		t.updates = append(t.updates, targetUpdate{surface: surface, event: ev})
	}
	t.mu.Unlock()
}

// event is used by the live automation model, which follows one surface.
func (t *target) event(ev driver.Event) { t.surfaceEvent(0, ev) }

// lostWhy says why a live model stopped following its surface, "" while it follows.
func (t *target) lostWhy() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lost
}

// applyLive applies b to a running app's model. One the model cannot apply leaves it lost, dropping what is queued, and fails the step taking it, rather than ending the app or leaving a model half applied to answer later steps.
func (t *target) applyLive(b *driver.MountBatch) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		why := fmt.Sprint(v)
		if f, ok := v.(liveFailure); ok {
			why = f.msg
		}
		why, _, _ = strings.Cut(strings.TrimPrefix(why, "nsgotest: "), "\n")
		t.mu.Lock()
		t.lost, t.updates = why, nil
		t.mu.Unlock()
		log.Printf("nsgo: the automation stopped following a surface: %s", why)
		fail(automation.Failed, "the automation stopped following the window: "+why)
	}()
	t.a.apply(b)
}

// quietFor is how long nothing must change for a step to be over, and
// settleWait how long a step waits for that.
const (
	quietFor   = 100 * time.Millisecond
	settleWait = 5 * time.Second
)

// startTarget opens the app on the platform: a root view, view, or an app's function, fn.
func (a *App) startTarget(view func(cx *core.Scope) core.Node, fn func(app *core.Scope)) {
	a.t.Helper()
	h := testhost.Current()
	switch {
	case h == nil:
		a.t.Fatal("nsgotest: no test host is linked: test the app's main package, which imports github.com/nsgo-dev/nsgo, with nsgo test -target")
	case !mainRan:
		a.t.Fatal("nsgotest: a platform needs the main thread: add func TestMain(m *testing.M) { nsgotest.Main(m) }")
	case opened && !h.Several():
		a.t.Skip("nsgotest: the test opens a second app, which only a desktop shows beside the first")
	}
	opened = true
	if a.store == nil {
		a.store = core.NewStore(nil)
	}
	t := &target{a: a}
	if w, ok := h.(testhost.WindowHost); ok && w.OpensWindows(fn != nil) {
		t.windowed = true
		if view != nil {
			// A root view is the main view of a function of its own, as the backend's Run shows it.
			main := view
			fn = func(cx *core.Scope) { cx.SetMainView(main) }
		}
	}
	app := view
	if fn != nil {
		app = core.AppRoot(fn)
	}
	a.target = t
	// The test's own graph shares the app's store, as another window
	// would, for Storage.
	a.root = core.NewRoot(core.RootOptions{
		Post: func(fn func()) {
			a.mu.Lock()
			a.posts = append(a.posts, fn)
			a.mu.Unlock()
		},
		Clock: a.Clock, Storage: a.store, Secrets: a.Secrets,
	})
	a.t.Cleanup(func() {
		// A native context menu tracks outside the test's graph and its window stays open, so one left open would take the next test's first input. Cancel it before disposing the graph, even if its close event has not reached the model yet.
		if t.surface != nil && a.popupID != 0 {
			_ = t.surface.ChoosePopup(0)
		}
		t.releaseHeld()
		a.root.Dispose()
	})
	a.seed()
	a.anim = driver.NewAnimator(func(driver.Event) {})
	s, err := h.Open(func(cx *core.Scope) core.Node {
		t.mu.Lock()
		t.appScope = cx
		t.mu.Unlock()
		return app(cx)
	}, a.targetOptions(t))
	if err != nil {
		a.t.Fatalf("nsgotest: the platform cannot open the app: %v", err)
	}
	t.source, t.surface = s, s
	// The first batch can take longer than a step's quiet time to come,
	// as on Windows, where the window opens first; with windows, the main window's first, which comes once the platform opened it, unless the app's own surface opened none.
	for deadline := time.Now().Add(settleWait); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		t.mu.Lock()
		came := slices.ContainsFunc(t.updates, func(u targetUpdate) bool { return u.batch != nil && (!u.aside || t.main == 0) })
		t.mu.Unlock()
		if came {
			break
		}
	}
	t.settle()
}

func (a *App) targetOptions(t *target) testhost.Options {
	// A nil clock is the system's; a nil *ManualClock would not be.
	var clock core.Clock = a.Clock
	if a.measure {
		clock = nil
	}
	size := a.size
	if t.windowed && !a.sized {
		// The main window opens at the desktop's default size, as the app's does.
		size = driver.Size{}
	}
	// The test sets PickImages after Start, so the answer reads it as each pick comes, on the test's goroutine; without one the user cancels, as headless.
	pickImages := func(r driver.PickRequest, done func([]driver.PickedFile)) {
		a.postTest(func() {
			var files []driver.PickedFile
			if a.PickImages != nil {
				files = a.PickImages(imagePick(r))
			}
			done(files)
		})
	}
	// The test sets TakePhoto after Start, so the answer reads it as each request comes, on the test's goroutine; without one the user cancels, as headless.
	takePhoto := func(r driver.PickRequest, done func(*driver.PickedFile, error)) {
		a.postTest(func() {
			pick := imagePick(r)
			a.Photos = append(a.Photos, pick)
			if a.TakePhoto == nil {
				done(nil, nil)
				return
			}
			photo, err := a.TakePhoto(pick)
			done(photo, err)
		})
	}
	// ScanCode too, as TakePhoto.
	scanCode := func(r driver.ScanRequest, done func(*driver.ScannedCode, error)) {
		a.postTest(func() {
			var code *driver.ScannedCode
			var err error
			if a.ScanCode != nil {
				code, err = a.ScanCode(codeScan(r))
			}
			done(code, err)
		})
	}
	return testhost.Options{
		CheckCommits: !automationBuild && !a.measure,
		Updater:      a.updater,
		Size:         size, Windows: t.windowed, Environment: a.env, Storage: a.store, Secrets: a.Secrets, Host: a.host, Clock: clock,
		FPS:        os.Getenv("NSGO_TEST_FPS") != "",
		Observe:    t.observe,
		Event:      t.surfaceEvent,
		PickImages: pickImages,
		TakePhoto:  takePhoto,
		ScanCode:   scanCode,
		DiskSpace:  a.diskSpace,
		UserDirs:   a.targetUserDirs(),
		TransitionEnds: func(pending func() bool) {
			t.mu.Lock()
			t.transitionsEnding = pending
			t.mu.Unlock()
		},
	}
}

// drain applies what came since the last call, reporting whether
// anything did.
// drainEvents is drain, reporting only whether events came.
func (t *target) drainEvents() bool {
	t.mu.Lock()
	events := slices.ContainsFunc(t.updates, func(u targetUpdate) bool { return u.batch == nil })
	t.mu.Unlock()
	t.drain()
	return events
}

func (t *target) drain() bool {
	t.mu.Lock()
	updates := t.updates
	t.updates = nil
	t.mu.Unlock()
	for _, u := range updates {
		if b := u.batch; b != nil {
			t.a.traceBatch(b, 0)
			switch {
			case u.aside:
				t.applyOwn(b)
			case t.live:
				t.applyLive(b)
			default:
				t.applySurface(u)
			}
			continue
		}
		a := t.a
		if t.windowed {
			s := a.surfaceModels[u.surface]
			if s == nil {
				continue
			}
			a = s.model
		}
		a.applyPlatformEvent(u.event)
	}
	return len(updates) > 0
}

func (a *App) applyPlatformEvent(ev driver.Event) {
	if ev.Kind == driver.EventAlert {
		a.alerts = slices.DeleteFunc(a.alerts, func(c driver.AlertConfig) bool { return c.ID == ev.Count })
		return
	}
	v := a.views[ev.ID]
	if v == nil {
		return
	}
	switch ev.Kind {
	case driver.EventScroll:
		v.Offset, v.Applied = ev.Point, ev.Count
		if ev.Value != 0 {
			v.ZoomScale = ev.Value
		}
	case driver.EventFocus, driver.EventBlur:
		v.Focused = ev.Kind == driver.EventFocus
	case driver.EventChangeText:
		a.edits[ev.ID] = max(a.edits[ev.ID], ev.Count)
		v.Text = ev.Text
	case driver.EventSelectionChange:
		// A field's; a selection area's carries its text instead.
		if v.Kind == KindInput {
			v.Selection = driver.TextRange{Start: ev.Index, End: int(ev.Count)}
		}
	}
}

func (t *target) applySurface(u targetUpdate) {
	a := t.a
	if u.closed {
		if u.main {
			a.apply(u.batch)
		}
		a.closeSurface(u.surface)
		if u.main {
			t.mainClosed()
		}
		return
	}
	first := a.surfaceModels[u.surface] == nil
	s := a.openSurface(a.surfaceConfig(u.surface), nil, u.main || !t.windowed)
	s.updateTitle(u.batch)
	if s.model == a {
		if t.windowed && t.source != nil && first {
			// Native operations on App still act on its main window.
			t.surface = t.windowSurface(u.surface)
		}
		a.apply(u.batch)
		return
	}
	if s.model.target == nil {
		s.model.target = &target{a: s.model, owner: t, surface: t.windowSurface(u.surface)}
	}
	a.applyAside(u.batch)
	s.model.applySurface(u.batch)
}

func (t *target) windowSurface(id uint32) testhost.Surface {
	if windows, ok := t.source.(testhost.WindowSurfaces); ok {
		return windows.WindowSurface(id)
	}
	return t.source
}

// applyOwn applies a batch of the app's own surface aside: its application menu, which shows while the main window sets none, goes to ownMenu.
func (t *target) applyOwn(b *driver.MountBatch) {
	aside := *b
	aside.Items = slices.DeleteFunc(slices.Clone(b.Items), func(it driver.MountItem) bool {
		if it.Kind == driver.ItemMenu {
			t.ownMenu = it.Menu
			return true
		}
		return false
	})
	t.a.applyAside(&aside)
	t.a.showMenu()
}

// mainClosed forgets what the main window that closed left in the model: its views, keyboard shortcuts and menu, as nothing is left there for steps to reach.
func (t *target) mainClosed() {
	a := t.a
	t.surface = t.source
	a.views, a.rootID, a.Keymap, a.keyBindings = map[driver.NodeID]*View{}, 0, nil, nil
	a.SystemGestures = core.SystemGestureOptions{}
	delete(a.menus, nil)
	a.showMenu()
}

// sync waits for the engine's pending commit within the step's deadline.
func (t *target) sync(deadline time.Time) bool {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return false
	}
	if t.syncing == nil {
		done := make(chan struct{})
		t.syncing = done
		source := t.source
		if source == nil {
			source = t.surface
		}
		go func() { source.Sync(); close(done) }()
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-t.syncing:
		t.syncing = nil
		return true
	case <-timer.C:
		return false
	}
}

// settle waits for the engine to commit, then for no batch or finite animation for quietFor, settleWait at most, applying what came.
func (t *target) settle() {
	t.settleUntil(time.Now().Add(settleWait))
}

func (t *target) settleUntil(deadline time.Time) {
	if t.owner != nil {
		t.owner.settleUntil(deadline)
		t.quiet = t.owner.quiet
		return
	}
	t.runPosts()
	if !t.sync(deadline) {
		t.drain()
		t.quiet = false
		return
	}
	if t.a.measure {
		// An app measured may commit as long as it runs, as a ticking
		// table does, and asking the platform what animates walks its
		// views: a step waits quietFor for what it set off to land and
		// for the platform's events to stop, then only for what it moved
		// to rest, looking seldom, so the readings are the app's.
		quiet := time.Now()
		for {
			t.runPosts()
			if t.drainEvents() {
				quiet = time.Now()
			}
			if time.Now().After(deadline) {
				t.quiet = false
				return
			}
			if time.Since(quiet) >= quietFor && !t.animating() {
				if !t.drainEvents() {
					t.quiet = time.Now().Before(deadline)
					return
				}
				quiet = time.Now()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	quiet := time.Now()
	for {
		// Posts handed to the engine are work in flight, as a batch is.
		posted := t.runPosts()
		if t.drain() || posted || t.animating() {
			quiet = time.Now()
		}
		if time.Now().After(deadline) {
			t.quiet = false
			return
		}
		if time.Since(quiet) >= quietFor {
			// The engine can be in a slow commit while no batch arrives. Check it again before declaring the app quiet, then take the batches it sent before completing the barrier.
			if !t.sync(deadline) {
				t.drain()
				t.quiet = false
				return
			}
			// Check the platform before draining: it can report events as it applies the engine's last commit. Those events and any new posts need another turn.
			if t.animating() || t.drain() || t.pending() {
				quiet = time.Now()
				continue
			}
			t.quiet = time.Now().Before(deadline)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (t *target) animating() bool {
	if t.surface != nil && t.surface.Animating() {
		return true
	}
	for _, s := range t.a.surfaceModels {
		if other := s.model.target; other != nil && other != t && other.surface != nil && other.surface.Animating() {
			return true
		}
	}
	return false
}

func (t *target) closeWindow() {
	t.a.t.Helper()
	if s, ok := t.surface.(testhost.CloseWindow); ok {
		t.check(s.RequestClose())
		return
	}
	t.a.t.Fatalf("nsgotest: this surface has no native window to close")
}

// runPosts runs what the test's own graph posted and hands App.Post's functions to the engine, reporting whether there were any.
func (t *target) runPosts() bool {
	t.a.mu.Lock()
	posts := t.a.posts
	t.a.posts = nil
	t.a.mu.Unlock()
	for _, fn := range posts {
		fn()
	}
	return len(posts) > 0
}

// postApp hands fn, which App.Post queued, to the engine's goroutine, to run under the app's scope before the engine's next barrier completes. It runs on the test goroutine, in runPosts.
func (t *target) postApp(fn func()) {
	t.mu.Lock()
	cx := t.appScope
	t.mu.Unlock()
	if cx == nil {
		t.a.t.Fatal("nsgotest: App.Post: the platform has not rendered the app yet")
	}
	cx.Post(fn)
}

// pending reports whether posts wait for the next runPosts.
func (t *target) pending() bool {
	t.a.mu.Lock()
	defer t.a.mu.Unlock()
	return len(t.a.posts) > 0
}

func (t *target) check(err error) {
	t.a.t.Helper()
	if err != nil {
		t.a.t.Fatalf("nsgotest: %s: %v", t.a.stepName, err)
	}
}

// focusField clicks the field v, as a user focuses one. With the run's opt-in the platform first activates the app (macOS's -activate); a click that leaves the field without the focus then fails the step with the platform's reason, an inactive app on macOS, rather than leave a later step to find no focused field.
func (t *target) focusField(v *View) {
	t.prepareKeyboard()
	t.press(t.reveal(v), 0, driver.Modifiers{})
	t.settle()
	if s, ok := t.surface.(testhost.KeyboardSurface); ok && !v.Focused {
		t.check(s.FocusRefusal())
	}
}

// prepareKeyboard lets the platform activate the app for the keyboard when the run opted in to that.
func (t *target) prepareKeyboard() {
	if s, ok := t.surface.(testhost.KeyboardSurface); ok {
		t.check(s.PrepareKeyboard())
	}
}

// reveal scrolls v into view, as a user does before acting on it: each
// scroll view around it that hides its middle moves it to the middle, a
// gesture at a time. It returns where v's middle is then.
func (t *target) reveal(v *View) driver.Point {
	t.a.t.Helper()
	for range 30 {
		_, at := v.center()
		var hider *View
		var box driver.Bounds
		for p := v.Parent; p != nil; p = p.Parent {
			if p.Kind == KindScroll && !p.Scroll.Behavior.NoScroll {
				b, _ := t.a.revealViewport(v, p, at)
				if b.Size.Width <= 0 || b.Size.Height <= 0 {
					continue
				}
				outsideX := at.X < b.Origin.X || at.X >= b.Origin.X+b.Size.Width
				outsideY := at.Y < b.Origin.Y || at.Y >= b.Origin.Y+b.Size.Height
				if (p.Scroll.Horizontal || p.Scroll.Zooms()) && outsideX || (!p.Scroll.Horizontal || p.Scroll.Zooms()) && outsideY {
					hider, box = p, b
					break
				}
			}
		}
		if hider == nil {
			return at
		}
		mid := driver.Point{X: box.Origin.X + box.Size.Width/2, Y: box.Origin.Y + box.Size.Height/2}
		d := driver.Point{X: at.X - mid.X, Y: at.Y - mid.Y}
		limit := func(v, size driver.Points) driver.Points { return min(max(v, -size*0.8), size*0.8) }
		if hider.Scroll.Zooms() {
			d = driver.Point{X: limit(d.X, box.Size.Width), Y: limit(d.Y, box.Size.Height)}
		} else if hider.Scroll.Horizontal {
			d = driver.Point{X: limit(d.X, box.Size.Width)}
		} else {
			d = driver.Point{Y: limit(d.Y, box.Size.Height)}
		}
		t.check(t.surface.Scroll(mid, d))
		t.settle()
	}
	_, at := v.center()
	return at
}

func (t *target) storage() core.Storage { return t.a.root.Storage() }

// menu takes a menu step with the platform's own menus: a platform that
// shows none skips the test.
func (t *target) menu(step func(testhost.Surface) error) {
	t.a.t.Helper()
	if err := step(t.surface); errors.Is(err, testhost.ErrNoMenu) {
		t.a.t.Skipf("nsgotest: %s: %v", t.a.stepName, err)
	} else {
		t.check(err)
	}
	t.settle()
}

// takeFrames settles, then, while a view of the app waits for its style transition to end, moves the test's clock a frame at a time, settleLimit at most, and settles again, as a headless step takes frames: the platform runs the transition in real time, the core times its OnTransitionEnd on the clock.
func (t *target) takeFrames() {
	if t.owner != nil {
		t.owner.takeFrames()
		return
	}
	t.settle()
	if t.a.measure {
		// The app runs on the system's clock.
		return
	}
	t.mu.Lock()
	ending := t.transitionsEnding
	t.mu.Unlock()
	if ending == nil || !ending() {
		return
	}
	limit := t.a.Clock.Now().Add(settleLimit)
	for t.a.Clock.Now().Before(limit) {
		t.a.Clock.Advance(driver.FrameInterval)
		t.surface.Sync()
		t.drain()
		if !ending() {
			break
		}
	}
	t.settle()
}

// advance moves the test's clock, which the app's timers run on, by d:
// a frame at a time, settling between, when run, else at once. The
// platform's own animations run in real time.
func (t *target) advance(d time.Duration, run bool) {
	if t.owner != nil {
		t.owner.advance(d, run)
		return
	}
	if t.a.measure {
		time.Sleep(d)
		t.runPosts()
		t.drain()
		return
	}
	if !run {
		t.a.Clock.Jump(d)
		t.settle()
		return
	}
	for d > 0 {
		step := min(d, driver.FrameInterval)
		t.a.Clock.Advance(step)
		d -= step
		// What the step's timers posted runs before the next step, so
		// the timers it sets (an interval's next run) fall due in time.
		t.surface.Sync()
		t.drain()
	}
	t.settle()
}

func (t *target) setEnvironment(env driver.Environment) {
	t.check(t.surface.SetEnvironment(env))
	t.settle()
}

func (t *target) resize(size driver.Size) {
	t.check(t.surface.Resize(size))
	t.settle()
}

func (t *target) answer(alert driver.AlertConfig, index int) {
	t.check(t.surface.Answer(alert.ID, index))
	t.settle()
}

func (t *target) press(at driver.Point, hold time.Duration, mods driver.Modifiers) {
	t.check(t.surface.Press(at, hold, mods))
}

// pressWhile holds a press for hold, running during once the finger is
// down.
func (t *target) pressWhile(at driver.Point, hold time.Duration, during func()) {
	done := make(chan error, 1)
	go func() { done <- t.surface.Press(at, hold, driver.Modifiers{}) }()
	time.Sleep(hold / 2)
	during()
	t.check(<-done)
}

func (t *target) rightClick(at driver.Point) { t.check(t.surface.RightClick(at)) }
func (t *target) hover(at driver.Point)      { t.check(t.surface.Hover(at)) }
func (t *target) back()                      { t.check(t.surface.Back()) }

func (t *target) typeText(v *View, text string) { t.check(t.surface.Type(text)) }
func (t *target) compose(text string)           { t.ime(t.surface.Compose(text)) }
func (t *target) commit(text string)            { t.ime(t.surface.Commit(text)) }

// keyedSelection reports whether selectRange can move a field's selection on target, through a keyboard's editing keys: the desktops' and the web's.
func keyedSelection(target string) bool {
	return slices.Contains([]string{"macos", "windows", "linux", "web"}, target)
}

// selectRange moves the focused native field's UTF-16 selection through a host
// selection API when available. Desktops and the web use select-all and editing
// arrows; those arrows step by grapheme while this fallback counts code points.
func (t *target) selectRange(v *View, start, end int) {
	text := v.Text
	start = min(max(start, 0), driver.UTF16Len(text))
	end = min(max(end, 0), driver.UTF16Len(text))
	if start > end {
		start, end = end, start
	}
	if native, ok := t.surface.(testhost.TextSelectionSurface); ok {
		selection, err := native.SelectRange(driver.TextRange{Start: start, End: end})
		t.check(err)
		// Read back the native range even when no application callback is
		// subscribed. Later callbacks or controlled updates win as we settle.
		v.Selection = selection
		t.settle()
		return
	}
	if !keyedSelection(Target()) {
		t.a.t.Skipf("nsgotest: %s: the platform cannot move a native text selection", t.a.stepName)
	}
	t.check(t.surface.Key("mod+a"))
	t.check(t.surface.Key("left"))
	from, to := driver.UTF16Offset(text, start), driver.UTF16Offset(text, end)
	for range utf8.RuneCountInString(text[:from]) {
		t.check(t.surface.Key("right"))
	}
	for range utf8.RuneCountInString(text[from:to]) {
		t.check(t.surface.Key("shift+right"))
	}
	t.settle()
}

// ime checks a composition step: a machine without an input method skips
// the test.
func (t *target) ime(err error) {
	t.a.t.Helper()
	if errors.Is(err, testhost.ErrNoInputMethod) {
		t.a.t.Skipf("nsgotest: %s: %v", t.a.stepName, err)
	}
	t.check(err)
}

// clear selects the field's text and deletes it, as a keyboard does.
func (t *target) clear(v *View) {
	t.check(t.surface.Key("mod+a"))
	t.check(t.surface.Key("Backspace"))
}

func (t *target) keys(v *View, key string) { t.check(t.surface.Key(key)) }

// hold presses key and holds it, or lets it come up, where the surface can hold a key (testhost.KeyHoldSurface); elsewhere the test skips. A key not held fails its KeyUp.
func (t *target) hold(key string, down bool) {
	t.a.t.Helper()
	s, ok := t.surface.(testhost.KeyHoldSurface)
	if !ok {
		t.a.t.Skipf("nsgotest: %s: the platform cannot hold a key", t.a.stepName)
	}
	name := t.a.keyName(driver.NormalizeKey(key))
	if !down {
		if _, ok := t.held[name]; !ok {
			t.a.t.Fatalf("nsgotest: %s: %q is not held; KeyDown holds a key", t.a.stepName, key)
		}
		delete(t.held, name)
		t.check(s.KeyUp(key))
		return
	}
	if t.held == nil {
		t.held = map[string]string{}
	}
	t.held[name] = key
	t.check(s.KeyDown(key))
}

// releaseHeld lets every key KeyDown or KeyCodeDown and mouse button MouseDown still holds come up as the test ends, every other key, by name or by position, before any modifier, so neither the platform nor, on the web, the page keeps one held into the next app the test process opens, such as a later subtest's, and each key comes up with the modifiers it went down with. A key or button a window that has closed held is gone with it; such an up's error is ignored.
func (t *target) releaseHeld() {
	if s, ok := t.surface.(testhost.MouseButtonSurface); ok {
		for b := driver.MouseButtonLeft; b <= driver.MouseButtonForward; b++ {
			if at, held := t.heldButtons[b]; held {
				_ = s.MouseUp(at, b, driver.Modifiers{})
			}
		}
	}
	t.heldButtons = nil
	names := slices.Sorted(maps.Keys(t.held))
	codes := slices.Sorted(maps.Keys(t.heldCodes))
	byName, _ := t.surface.(testhost.KeyHoldSurface)
	byCode, _ := t.surface.(testhost.KeyCodeSurface)
	for _, modifiers := range []bool{false, true} {
		if byName != nil {
			for _, name := range names {
				if modifierKey(name) == modifiers {
					_ = byName.KeyUp(t.held[name])
				}
			}
		}
		if byCode != nil {
			for _, code := range codes {
				if modifierKey(t.heldCodes[code]) == modifiers {
					_ = byCode.KeyCodeUp(code, t.heldCodes[code])
				}
			}
		}
	}
	t.held = nil
	t.heldCodes = nil
}

// holdCode presses the physical position code, with the key name the US layout puts there for a host whose layout names no key there, and holds it, or lets it come up, where the surface presses positions (testhost.KeyCodeSurface); elsewhere the test skips.
func (t *target) holdCode(code, name string, down bool) {
	t.a.t.Helper()
	s, ok := t.surface.(testhost.KeyCodeSurface)
	if !ok {
		t.a.t.Skipf("nsgotest: %s: the platform cannot press a physical key position", t.a.stepName)
	}
	if !down {
		if _, ok := t.heldCodes[code]; !ok {
			t.a.t.Fatalf("nsgotest: %s: %q is not held. KeyCodeDown holds a position", t.a.stepName, code)
		}
		delete(t.heldCodes, code)
		t.check(s.KeyCodeUp(code, name))
		return
	}
	if t.heldCodes == nil {
		t.heldCodes = map[string]string{}
	}
	t.heldCodes[code] = name
	t.check(s.KeyCodeDown(code, name))
}

// mouseButton presses or releases button at surface point at on the target where it implements testhost.MouseButtonSurface.
func (t *target) mouseButton(at driver.Point, button driver.MouseButton, mods driver.Modifiers, down bool) {
	t.a.t.Helper()
	s, ok := t.surface.(testhost.MouseButtonSurface)
	if !ok {
		t.a.t.Skipf("nsgotest: %s: the platform cannot hold a mouse button", t.a.stepName)
	}
	if !down {
		if _, held := t.heldButtons[button]; !held {
			t.a.t.Fatalf("nsgotest: %s: %s button is not held; MouseDown holds a button", t.a.stepName, button)
		}
		delete(t.heldButtons, button)
		t.check(s.MouseUp(at, button, mods))
		return
	}
	if t.heldButtons == nil {
		t.heldButtons = map[driver.MouseButton]driver.Point{}
	}
	t.heldButtons[button] = at
	t.check(s.MouseDown(at, button, mods))
}

// blurWindow simulates window focus loss on the target where it implements testhost.BlurWindowSurface.
func (t *target) blurWindow() {
	t.a.t.Helper()
	s, ok := t.surface.(testhost.BlurWindowSurface)
	if !ok {
		t.a.t.Skipf("nsgotest: %s: the platform cannot blur its window in a test", t.a.stepName)
	}
	t.held = nil
	t.heldButtons = nil
	t.check(s.BlurWindow())
}

func (t *target) drag(start driver.Point, path []driver.Point) {
	t.check(t.surface.Drag(append([]driver.Point{start}, path...)))
}

// scroll drags v's content by d along its axis, starting from its middle.
func (t *target) scroll(v *View, d driver.Points) {
	_, at := v.center()
	delta := driver.Point{Y: d}
	if v.Scroll.Horizontal {
		delta = driver.Point{X: d}
	}
	t.check(t.surface.Scroll(at, delta))
}

// CapturePNG captures the app as the platform shows it and returns its PNG. On macOS, iOS and tvOS its pixels are sRGB. Headless runs skip the test because they have no native pixels.
func (a *App) CapturePNG() []byte {
	a.t.Helper()
	if a.target == nil {
		a.t.Skip("nsgotest: no pixels headless; run under nsgo test -target")
	}
	png, err := a.target.screenshotPNG()
	if err != nil {
		a.t.Fatalf("nsgotest: capture: %v", err)
	}
	return append([]byte(nil), png...)
}

// Capture captures the app as the platform shows it and decodes its PNG. On macOS, iOS and tvOS its pixels are sRGB, regardless of the display's color space. Headless runs skip the test because they have no native pixels.
func (a *App) Capture() image.Image {
	a.t.Helper()
	pngData := a.CapturePNG()
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		a.t.Fatalf("nsgotest: decode capture: %v", err)
	}
	return img
}

// Screenshot captures the app as the platform shows it. name is a relative
// path under the directory `nsgo test -target` saves screenshots to; its
// extension is optional and .png is added when absent. Headless runs have no
// pixels and keep the historical log-only behavior.
func (a *App) Screenshot(name string) {
	a.t.Helper()
	if name == "" || filepath.Clean(name) == "." || filepath.IsAbs(name) || pathHasParent(name) {
		a.t.Fatalf("nsgotest: screenshot name must be a relative file path without ..: %q", name)
	}
	if a.target == nil {
		a.t.Logf("nsgotest: no screenshot %q headless; nsgo test -target takes it", name)
		return
	}
	png := a.CapturePNG()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(name))
	// The tool saves it: the app may run on a phone, or in a browser. A device's log keeps lines of a few kilobytes, so it goes in chunks, each with the PNG's length and CRC-32, which the tool checks the joined chunks against: a log that drops or cuts lines, as HarmonyOS's hilog does when it throttles an app, fails the test instead of saving a corrupt image.
	data := base64.StdEncoding.EncodeToString(png)
	sum := fmt.Sprintf("%d:%08x", len(png), crc32.ChecksumIEEE(png))
	const chunk = 3000
	n := (len(data) + chunk - 1) / chunk
	for i := range n {
		fmt.Printf("nsgo-screenshot ~%s %d/%d %s %s\n", encoded, i+1, n, sum, data[i*chunk:min((i+1)*chunk, len(data))])
	}
}

// screenshotPNG settles and captures the app as the platform shows it.
func (t *target) screenshotPNG() ([]byte, error) {
	t.settle()
	return t.surface.Screenshot()
}

func pathHasParent(name string) bool {
	for _, part := range strings.FieldsFunc(filepath.ToSlash(name), func(r rune) bool { return r == '/' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

// targetUserDirs are the folders a target test pins, nil for the target to make its own.
func (a *App) targetUserDirs() map[core.StandardDir]string {
	if !a.userDirsPinned {
		return nil
	}
	if a.userDirs == nil {
		return map[core.StandardDir]string{}
	}
	return a.userDirs
}
