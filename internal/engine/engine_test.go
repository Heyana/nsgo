package engine

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// recorder is a platform that keeps the batches it is handed.
type recorder struct {
	core.ApproxTextEngine
	mu        sync.Mutex
	batches   []*driver.MountBatch
	gpuFrames []driver.GPUFrame
	mounted   chan struct{}
	// frames receives a value for each display frame the app asks for.
	frames chan struct{}
}

func (r *recorder) Mount(b *driver.MountBatch) {
	r.mu.Lock()
	r.batches = append(r.batches, b)
	r.mu.Unlock()
	select {
	case r.mounted <- struct{}{}:
	default:
	}
}

// RequestFrame implements Platform: the app waits for a display frame.
func (r *recorder) SubmitGPU(frame driver.GPUFrame) {
	r.mu.Lock()
	r.gpuFrames = append(r.gpuFrames, frame)
	r.mu.Unlock()
}

func (r *recorder) RequestFrame() {
	if r.frames == nil {
		return
	}
	select {
	case r.frames <- struct{}{}:
	default:
	}
}

// texts returns the latest text of every text view.
func (r *recorder) texts() map[driver.NodeID]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[driver.NodeID]string{}
	for _, b := range r.batches {
		for _, it := range b.Items {
			if it.Kind == driver.ItemText {
				out[it.ID] = it.Text.Text
			}
		}
	}
	return out
}

func TestDispatchNeverBlocksAndMerges(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	var presses int
	var button driver.NodeID
	e := Start(p, func(cx *core.Scope) core.Node {
		count := core.NewSignal(cx, 0)
		return core.View{Children: []core.Node{
			core.View{OnPress: func(core.PressEvent) { presses++; count.Set(presses) }, TestID: "button"},
			core.Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }},
		}}
	}, Options{Size: driver.Size{Width: 100, Height: 100}})
	defer e.Close()
	<-p.mounted
	p.mu.Lock()
	for _, b := range p.batches {
		for _, it := range b.Items {
			if it.Kind == driver.ItemViewProps && it.ViewProps.TestID == "button" {
				button = it.ID
			}
		}
	}
	p.mu.Unlock()

	// Far more than any fixed buffer, from a goroutine standing in for the
	// main thread: none of these may block.
	done := make(chan struct{})
	go func() {
		for range 5000 {
			e.Dispatch(driver.Event{Kind: driver.EventPress, ID: button})
			e.Resize(driver.Size{Width: 100, Height: 100}, driver.Geometry{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Dispatch blocked")
	}
	deadline := time.After(5 * time.Second)
	for {
		found := false
		for _, s := range p.texts() {
			found = found || s == "5000"
		}
		if found {
			return
		}
		select {
		case <-p.mounted:
		case <-deadline:
			t.Fatalf("texts = %v, want 5000 presses counted", p.texts())
		}
	}
}

func TestLanguageReadsTheMountedApp(t *testing.T) {
	core.SetLanguageMatch(func(locale string) string {
		if locale == "zh-CN" {
			return "zh-Hans"
		}
		return "en"
	})
	defer core.SetLanguageMatch(nil)
	p := &recorder{mounted: make(chan struct{}, 1)}
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.SetLocale("zh-CN")
		return core.Text{Content: "text"}
	}, Options{Size: driver.Size{Width: 100, Height: 100}, Environment: driver.Environment{Locale: "en-US"}})
	// Asked as the platform starts, before the first commit arrives: Language waits for the mount.
	if got := e.Language(); got != "zh-Hans" {
		t.Fatalf("Language() = %q, want zh-Hans", got)
	}
	e.Close()
	got := make(chan string, 1)
	go func() { got <- e.Language() }()
	select {
	case lang := <-got:
		if lang != "" {
			t.Fatalf("Language() after Close = %q, want \"\"", lang)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Language() after Close did not return")
	}
}

func TestCloseRunsTheQuitHandlersFirst(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	var order []string
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.OnQuit(func(ctx context.Context) { order = append(order, "first") })
		cx.OnQuit(func(ctx context.Context) {
			// Work of its own, waited for while the time lasts.
			saved := make(chan struct{})
			go func() { time.Sleep(10 * time.Millisecond); close(saved) }()
			select {
			case <-saved:
				order = append(order, "saved")
			case <-ctx.Done():
			}
		})
		cx.Cleanup(func() { order = append(order, "disposed") })
		return core.View{}
	}, Options{})
	<-p.mounted
	e.Close()
	if !slices.Equal(order, []string{"first", "saved", "disposed"}) {
		t.Errorf("quitting ran %v", order)
	}
}

func TestHideRunsTheQuitHandlersAndKeepsTheApp(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	runs := 0
	disposed := false
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.OnQuit(func(context.Context) { runs++ })
		cx.Cleanup(func() { disposed = true })
		return core.View{}
	}, Options{})
	<-p.mounted
	e.Hide()
	e.Hide()
	if runs != 2 || disposed {
		t.Errorf("two hides ran the handler %d times, the app disposed: %v", runs, disposed)
	}
	e.Close()
	if runs != 3 || !disposed {
		t.Errorf("closing after hides ran the handler %d times, the app disposed: %v", runs, disposed)
	}
}

func TestCloseWaitsNoLongerThanTheQuitTimeout(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	release := make(chan struct{})
	defer close(release)
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.OnQuit(func(context.Context) { <-release })
		return core.View{}
	}, Options{})
	<-p.mounted
	start := time.Now()
	e.Close()
	if took := time.Since(start); took > core.QuitTimeout+time.Second {
		t.Errorf("Close waited %v for a handler that never returns", took)
	}
}

func TestCloseDisposesTheApp(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	cleaned := make(chan struct{})
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.Cleanup(func() { close(cleaned) })
		return core.View{}
	}, Options{})
	<-p.mounted
	e.Close()
	select {
	case <-cleaned:
	default:
		t.Fatal("Close did not dispose the app")
	}
}

// The app's frame callbacks run when the platform reports the frame it
// was asked for, and a callback that waits again asks for another.
func TestFramesFollowThePlatform(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1), frames: make(chan struct{}, 4)}
	seen := make(chan time.Time, 4)
	e := Start(p, func(cx *core.Scope) core.Node {
		var next func(at time.Time)
		next = func(at time.Time) {
			seen <- at
			cx.RequestAnimationFrame(next)
		}
		cx.RequestAnimationFrame(next)
		return core.View{}
	}, Options{})
	defer e.Close()
	<-p.mounted

	select {
	case <-p.frames:
	case <-time.After(5 * time.Second):
		t.Fatal("the app asked for no display frame")
	}
	e.Frame()
	select {
	case at := <-seen:
		if at.IsZero() {
			t.Fatal("the frame had no time")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the frame did not run the app's callback")
	}
	select {
	case <-p.frames:
	case <-time.After(5 * time.Second):
		t.Fatal("the callback waiting for the next frame did not ask for it")
	}
}

// A frame the display timed runs the app's callbacks at the display's time on the system clock, and at the app's own clock's time under one of its own, whose base the display's time does not share.
func TestFrameAtGivesTheDisplaysTime(t *testing.T) {
	start := func(clock core.Clock) (*Engine, *recorder, chan time.Time) {
		p := &recorder{mounted: make(chan struct{}, 1), frames: make(chan struct{}, 4)}
		seen := make(chan time.Time, 4)
		e := Start(p, func(cx *core.Scope) core.Node {
			cx.RequestAnimationFrame(func(at time.Time) { seen <- at })
			return core.View{}
		}, Options{Clock: clock})
		<-p.mounted
		select {
		case <-p.frames:
		case <-time.After(5 * time.Second):
			t.Fatal("the app asked for no display frame")
		}
		return e, p, seen
	}
	wait := func(seen chan time.Time) time.Time {
		select {
		case at := <-seen:
			return at
		case <-time.After(5 * time.Second):
			t.Fatal("the frame did not run the app's callback")
		}
		return time.Time{}
	}

	e, _, seen := start(nil)
	shown := time.Now().Add(time.Hour)
	e.FrameAt(shown)
	if at := wait(seen); !at.Equal(shown) {
		t.Errorf("the frame's time = %v, want the display's %v", at, shown)
	}
	e.Close()

	clock := core.NewManualClock(time.Unix(100, 0))
	e, _, seen = start(clock)
	defer e.Close()
	e.FrameAt(shown)
	if at := wait(seen); !at.Equal(clock.Now()) {
		t.Errorf("under the app's clock the frame's time = %v, want the clock's %v", at, clock.Now())
	}
}

// An event a backend reports before the frame it is stepping waits for
// that frame: the engine handles the event, runs the frame and commits
// once.
func TestAnEventBeforeAFrameSharesItsPass(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1), frames: make(chan struct{}, 4)}
	var order []string
	e := Start(p, func(cx *core.Scope) core.Node {
		pressed, framed := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		cx.RequestAnimationFrame(func(time.Time) {
			order = append(order, "frame")
			framed.Set(1)
		})
		return core.View{Children: []core.Node{
			core.View{OnPress: func(core.PressEvent) {
				order = append(order, "press")
				pressed.Set(1)
			}, TestID: "button"},
			core.Text{ContentFunc: func() string { return strconv.Itoa(pressed.Get()) + strconv.Itoa(framed.Get()) }},
		}}
	}, Options{Size: driver.Size{Width: 100, Height: 100}})
	defer e.Close()
	<-p.mounted
	// Sync waits for the passes begun so far to commit.
	settle := func() int {
		e.Sync()
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.batches)
	}
	before := settle()
	var button driver.NodeID
	p.mu.Lock()
	for _, b := range p.batches {
		for _, it := range b.Items {
			if it.Kind == driver.ItemViewProps && it.ViewProps.TestID == "button" {
				button = it.ID
			}
		}
	}
	p.mu.Unlock()

	e.DispatchBeforeFrame(driver.Event{Kind: driver.EventPress, ID: button})
	e.Frame()
	commits := settle() - before
	if !slices.Equal(order, []string{"press", "frame"}) || commits != 1 {
		t.Errorf("the engine ran %v in %d commits, want the press, then the frame, in one", order, commits)
	}
}

// TestATestsClipboardIsItsOwn sets and reads the clipboard of an app a
// test observes: its own memory answers, and the platform sees nothing.
func TestATestsClipboardIsItsOwn(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	read := make(chan string, 1)
	var opts Options
	opts.UseTest(testhostOptions(func(uint32, *driver.MountBatch) {}), &TestPin{})
	opts.Size = driver.Size{Width: 100, Height: 100}
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.SetClipboard("secret")
		cx.Post(func() { cx.ReadClipboard(func(s string) { read <- s }) })
		return core.View{}
	}, opts)
	defer e.Close()
	select {
	case s := <-read:
		if s != "secret" {
			t.Errorf("read %q, want the text set", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the read was not answered")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, b := range p.batches {
		for _, it := range b.Items {
			if it.Kind == driver.ItemSetClipboard || it.Kind == driver.ItemReadClipboard {
				t.Errorf("the platform got %v", it.Kind)
			}
		}
	}
}

// fontRecorder is a recorder that registers fonts, handing each font's data to fonts and answering with err.
type fontRecorder struct {
	recorder
	fonts chan []byte
	err   error
}

func (r *fontRecorder) RegisterFont(data []byte) error {
	r.fonts <- slices.Clone(data)
	return r.err
}

// TestATestsAppRegistersFonts registers a font from an app a test observes: the platform under the test host takes it and answers (#313).
func TestATestsAppRegistersFonts(t *testing.T) {
	p := &fontRecorder{recorder: recorder{mounted: make(chan struct{}, 1)}, fonts: make(chan []byte, 1), err: errors.New("the platform's answer")}
	answered := make(chan error, 1)
	var opts Options
	opts.UseTest(testhostOptions(func(uint32, *driver.MountBatch) {}), &TestPin{})
	opts.Size = driver.Size{Width: 100, Height: 100}
	e := Start(p, func(cx *core.Scope) core.Node {
		answered <- cx.RegisterFont([]byte("font"))
		return core.View{}
	}, opts)
	defer e.Close()
	select {
	case data := <-p.fonts:
		if string(data) != "font" {
			t.Errorf("the platform registered %q, want the app's font", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the platform registered no font")
	}
	if err := <-answered; err != p.err {
		t.Errorf("RegisterFont returned %v, want the platform's answer", err)
	}
}

// TestAnObservedPlatformRegistersOnlyWhatItCan keeps a platform that registers no fonts from seeming to under a test host, where Scope.RegisterFont would prepare every text again for nothing.
func TestAnObservedPlatformRegistersOnlyWhatItCan(t *testing.T) {
	if _, ok := (observed{Platform: &recorder{}}).platform().(driver.FontRegistrar); ok {
		t.Error("a platform without fonts registers them when observed")
	}
	if _, ok := (observed{Platform: &fontRecorder{}}).platform().(driver.FontRegistrar); !ok {
		t.Error("a platform with fonts stops registering them when observed")
	}
}

// Trace hears each commit's batch before the platform does.
func TestTraceHearsEachCommit(t *testing.T) {
	type heard struct {
		items   int
		mounted bool
	}
	p := &recorder{mounted: make(chan struct{}, 1)}
	traced := make(chan heard, 4)
	Trace = func(b *driver.MountBatch, took time.Duration) {
		p.mu.Lock()
		mounted := slices.Contains(p.batches, b)
		p.mu.Unlock()
		traced <- heard{len(b.Items), mounted}
	}
	defer func() { Trace = nil }()
	e := Start(p, func(cx *core.Scope) core.Node { return core.Text{Content: "hi"} }, Options{})
	defer e.Close()
	<-p.mounted
	if h := <-traced; h.items == 0 || h.mounted {
		t.Fatalf("the trace heard %+v", h)
	}
}

// items returns the items of kind the recorder was handed.
func (r *recorder) items(kind driver.ItemKind) []driver.MountItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []driver.MountItem
	for _, b := range r.batches {
		for _, it := range b.Items {
			if it.Kind == kind {
				out = append(out, it)
			}
		}
	}
	return out
}

// wait waits until cond holds of r.
func (r *recorder) wait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		select {
		case <-r.mounted:
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestAttachOpenedSurfaces(t *testing.T) {
	app := &recorder{mounted: make(chan struct{}, 1)}
	var count *core.Signal[int]
	e := Start(app, core.AppRoot(func(cx *core.Scope) {
		count = core.NewSignal(cx, 1)
		cx.SetMainView(func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }}
		})
	}), Options{Windows: &core.WindowOptions{Width: 300, Height: 200}})
	defer e.Close()
	app.wait(t, "the main window to open", func() bool { return len(app.items(driver.ItemOpenSurface)) == 1 })
	cfg := app.items(driver.ItemOpenSurface)[0].Surface
	if cfg.ID != 1 || cfg.Width != 300 {
		t.Fatalf("opened %+v", cfg)
	}
	win := &recorder{mounted: make(chan struct{}, 1)}
	we := e.Attach(cfg.ID, win)
	we.Resize(driver.Size{Width: 300, Height: 200}, driver.Geometry{})
	win.wait(t, "the window's view", func() bool { return slices.Contains(slices.Collect(values(win.texts())), "1") })
	done := make(chan struct{})
	e.push(message{fn: func() { count.Set(2); close(done) }})
	<-done
	win.wait(t, "a shared signal to reach the window", func() bool {
		return slices.Contains(slices.Collect(values(win.texts())), "2")
	})
	we.Dispatch(driver.Event{Kind: driver.EventWindowClose})
	win.wait(t, "the window to be destroyed", func() bool {
		for _, it := range win.items(driver.ItemWindow) {
			if it.Window.Action == driver.WindowDestroy {
				return true
			}
		}
		return false
	})
	app.wait(t, "the app to quit with its last window", func() bool { return len(app.items(driver.ItemQuit)) == 1 })
}

// values returns the values of m.
func values(m map[driver.NodeID]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

// TestWheelTurnsAddUp merges a view's queued wheel turns by adding their
// deltas, where other moves replace each other, and keeps a touchpad's
// apart from a wheel's.
func TestWheelTurnsAddUp(t *testing.T) {
	e := &Engine{runner: &runner{}}
	turn := func(dy driver.Points, precise bool) {
		e.push(message{event: driver.Event{Kind: driver.EventWheel, ID: 7, Phase: driver.GestureChanged, Delta: driver.Point{Y: dy}, Precise: precise}})
	}
	turn(10, false)
	turn(15, false)
	turn(4, true)
	q := e.take()
	if len(q) != 2 || q[0].event.Delta.Y != 25 || q[1].event.Delta.Y != 4 {
		t.Fatalf("queued %+v", q)
	}
}

// A traced commit's causes say how long the app's code ran before it, while traces record them.
func TestTraceHearsTheAppsTimeBeforeACommit(t *testing.T) {
	driver.TraceCauses = true
	defer func() { driver.TraceCauses = false }()
	p := &recorder{mounted: make(chan struct{}, 1)}
	traced := make(chan *driver.Causes, 4)
	Trace = func(b *driver.MountBatch, took time.Duration) { traced <- b.Causes }
	defer func() { Trace = nil }()
	e := Start(p, func(cx *core.Scope) core.Node {
		// The app's code takes a moment before the first commit.
		time.Sleep(5 * time.Millisecond)
		return core.Text{Content: "hi"}
	}, Options{})
	defer e.Close()
	c := <-traced
	// Bounded above too: a wake-up's start left unset reads as a time past any real one.
	if c == nil || c.App < 5*time.Millisecond || c.App > time.Second || c.Layout <= 0 {
		t.Fatalf("causes = %+v, want the app's 5 ms and some layout", c)
	}
}

func TestResizeMergesWithItsGeometry(t *testing.T) {
	// A fold opening while a resize waits: the newer resize takes the older one's slot with the geometry measured at its own size, and carries an environment pushed between them, so the tree never applies the older geometry at the newer size.
	e := &Engine{runner: &runner{wake: make(chan struct{}, 1)}}
	folded := driver.Size{Width: 400, Height: 600}
	open := driver.Size{Width: 800, Height: 600}
	hinge := driver.Geometry{Obstructions: []driver.Obstruction{{Frame: driver.Bounds{Origin: driver.Point{X: 390}, Size: driver.Size{Width: 20, Height: 600}},
		Occluding: true, Separating: true, State: driver.HingeOpen}}}
	e.Resize(folded, driver.Geometry{})
	e.SetEnvironment(driver.Environment{Dark: true})
	done := e.ResizeAndWait(open, hinge)
	q := e.take()
	if len(q) != 1 || q[0].resize == nil || q[0].env == nil {
		t.Fatalf("queue %+v, want the merged resize and environment", q)
	}
	if *q[0].resize != open || !q[0].geometry.Equal(hinge) || len(q[0].resized) != 1 {
		t.Fatalf("merged resize at %v with %+v, want %v with the hinge", *q[0].resize, q[0].geometry, open)
	}

	// Applied in one batch, as the engine applies a message, what reads the size and the geometry sees them together.
	root := core.NewRoot(core.RootOptions{Post: func(func()) {}, Clock: core.NewManualClock(time.Unix(0, 0))})
	defer root.Dispose()
	tree := core.NewTree(root, core.ApproxTextEngine{})
	tree.Resize(folded, driver.Geometry{})
	type seen struct {
		size    driver.Size
		regions int
	}
	var runs []seen
	tree.Mount(func(cx *core.Scope) core.Node {
		cx.Effect(func() { runs = append(runs, seen{cx.Env().Window(), len(cx.Env().Geometry().Regions)}) })
		return core.View{}
	})
	root.Batch(func() {
		tree.Resize(*q[0].resize, q[0].geometry)
		tree.SetEnvironment(*q[0].env)
	})
	if !slices.Equal(runs, []seen{{folded, 1}, {open, 2}}) {
		t.Errorf("runs %+v, want the folded size with one region, then the open one with two", runs)
	}
	select {
	case <-done:
		t.Error("the wait ended before a commit")
	default:
	}
}

func TestEnvironmentMergesWithFollowingResize(t *testing.T) {
	// Android reports a rotation's safe area before the size it goes with: the two still queued become one message, as do a resize and the environment after it, and a resize that brings its own environment keeps it over an older one queued.
	wide := driver.Environment{SafeArea: driver.Edges{Top: 28, Right: 48, Left: 48}}
	landscape := driver.Size{Width: 846, Height: 411}
	e := &Engine{runner: &runner{wake: make(chan struct{}, 1)}}
	e.SetEnvironment(wide)
	done := e.ResizeAndWait(landscape, driver.Geometry{})
	q := e.take()
	if len(q) != 1 || q[0].resize == nil || q[0].env == nil || *q[0].resize != landscape || !q[0].env.Equal(wide) || len(q[0].resized) != 1 {
		t.Fatalf("queue %+v, want one resize to %v carrying %+v", q, landscape, wide)
	}
	select {
	case <-done:
		t.Error("the wait ended before a commit")
	default:
	}

	e.Resize(landscape, driver.Geometry{})
	e.SetEnvironment(wide)
	if q := e.take(); len(q) != 1 || q[0].resize == nil || q[0].env == nil || !q[0].env.Equal(wide) {
		t.Fatalf("queue %+v, want the resize carrying the environment after it", q)
	}

	e.SetEnvironment(driver.Environment{Dark: true})
	e.ResizeWithEnvironment(landscape, driver.Geometry{}, wide)
	if q := e.take(); len(q) != 1 || q[0].resize == nil || q[0].env == nil || !q[0].env.Equal(wide) {
		t.Fatalf("queue %+v, want the resize with its own environment, not the older one", q)
	}
}

func TestRotationShowsItsSafeAreaWithItsSize(t *testing.T) {
	// A rotation reaches the engine as a safe area and a size: queued together, or reported as one, no effect sees one orientation's safe area with the other's size.
	portrait, landscape := driver.Size{Width: 411, Height: 846}, driver.Size{Width: 846, Height: 411}
	tall := driver.Environment{SafeArea: driver.Edges{Top: 28}}
	wide := driver.Environment{SafeArea: driver.Edges{Top: 28, Right: 48, Left: 48}}
	type seen struct {
		size driver.Size
		safe driver.Edges
	}
	var mu sync.Mutex
	var runs []seen
	p := &recorder{mounted: make(chan struct{}, 1)}
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.Effect(func() {
			s := seen{cx.Env().Window(), cx.Env().SafeArea()}
			mu.Lock()
			runs = append(runs, s)
			mu.Unlock()
		})
		return core.View{}
	}, Options{Size: portrait, Environment: tall})
	defer e.Close()
	<-p.mounted

	// The engine is busy as Android sends the safe area, then the size.
	hold, held := make(chan struct{}), make(chan struct{})
	e.push(message{fn: func() { close(held); <-hold }})
	<-held
	e.SetEnvironment(wide)
	done := e.ResizeAndWait(landscape, driver.Geometry{})
	close(hold)
	<-done
	// Back, as one report.
	e.ResizeWithEnvironment(portrait, driver.Geometry{}, tall)
	e.Sync()

	mu.Lock()
	defer mu.Unlock()
	want := []seen{{portrait, tall.SafeArea}, {landscape, wide.SafeArea}, {portrait, tall.SafeArea}}
	if !slices.Equal(runs, want) {
		t.Errorf("runs %+v, want %+v", runs, want)
	}
}

func TestDisplaySwitchShowsItsEnvironmentWithItsSize(t *testing.T) {
	// iOS on the iPhone Duo: closing moves the app from the inner display, its status bar on top, to the outer one, its vertical bar trailing. The backend reports each layout's size, geometry and environment as one, the vertical bar's edge following in a layout of its own at the same size, so no effect sees the outer display's safe area at the inner display's size.
	inner, outer := driver.Size{Width: 669, Height: 951}, driver.Size{Width: 466, Height: 678}
	top := driver.Environment{SafeArea: driver.Edges{Top: 82, Bottom: 34}}
	bar := driver.Environment{SafeArea: driver.Edges{Right: 84, Bottom: 34}}
	trailing := bar
	trailing.VerticalBarEdge = driver.VerticalBarTrailing
	closed := driver.Geometry{State: driver.HingeClosed, HasAngle: true}
	type seen struct {
		size driver.Size
		safe driver.Edges
		edge driver.VerticalBarEdge
		g    driver.HingeState
	}
	var mu sync.Mutex
	var runs []seen
	p := &recorder{mounted: make(chan struct{}, 1)}
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.Effect(func() {
			s := seen{cx.Env().Window(), cx.Env().SafeArea(), cx.Env().VerticalBarEdge(), cx.Env().Geometry().State}
			mu.Lock()
			runs = append(runs, s)
			mu.Unlock()
		})
		return core.View{}
	}, Options{Size: inner, Environment: top, Geometry: driver.Geometry{State: driver.HingeOpen}})
	defer e.Close()
	<-p.mounted

	e.ResizeWithEnvironment(outer, closed, bar)
	e.Sync()
	e.ResizeWithEnvironment(outer, closed, trailing)
	e.Sync()

	mu.Lock()
	defer mu.Unlock()
	want := []seen{{inner, top.SafeArea, driver.VerticalBarUnspecified, driver.HingeOpen}, {outer, bar.SafeArea, driver.VerticalBarUnspecified, driver.HingeClosed}, {outer, bar.SafeArea, driver.VerticalBarTrailing, driver.HingeClosed}}
	if !slices.Equal(runs, want) {
		t.Errorf("runs %+v, want %+v", runs, want)
	}
}

func TestStartLaysOutWithItsGeometry(t *testing.T) {
	// An app started on a half-open foldable: its first commit has the two regions the start's size came with, not one replaced by them in the next.
	p := &recorder{mounted: make(chan struct{}, 1)}
	size := driver.Size{Width: 800, Height: 600}
	fold := driver.Geometry{Obstructions: []driver.Obstruction{{Frame: driver.Bounds{Origin: driver.Point{X: 400}, Size: driver.Size{Height: 600}},
		Separating: true, State: driver.HingeHalfOpen}}}
	var regions []int
	e := Start(p, func(cx *core.Scope) core.Node {
		regions = append(regions, len(cx.Env().Geometry().Regions))
		return core.View{}
	}, Options{Size: size, Geometry: fold})
	defer e.Close()
	<-p.mounted
	if !slices.Equal(regions, []int{2}) {
		t.Errorf("the app built with %v regions, want 2", regions)
	}
}

// machineKeychain fails a test that reaches it, as the platform's own secure store would be the machine's.
type machineKeychain struct{ t *testing.T }

func (k machineKeychain) ReadSecret(driver.SecretItem) (string, bool, error) {
	k.t.Error("a test read the machine's keychain")
	return "", false, nil
}

func (k machineKeychain) WriteSecret(driver.SecretItem, string) error {
	k.t.Error("a test wrote the machine's keychain")
	return nil
}

func (k machineKeychain) DeleteSecret(driver.SecretItem) error {
	k.t.Error("a test deleted from the machine's keychain")
	return nil
}

// TestATestsSecretsAreItsOwn keeps an app a test opens on a platform away from the platform's secure store: what the app keeps goes to the test's store, in memory.
func TestATestsSecretsAreItsOwn(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	read := make(chan string, 1)
	mem := &driver.MemorySecrets{}
	opts := Options{Secrets: machineKeychain{t}}
	o := testhostOptions(func(uint32, *driver.MountBatch) {})
	o.Secrets = mem
	opts.UseTest(o, &TestPin{})
	opts.Size = driver.Size{Width: 100, Height: 100}
	e := Start(p, func(cx *core.Scope) core.Node {
		s := cx.Secrets()
		if err := s.Set("token", "kept"); err != nil {
			t.Error(err)
		}
		v, _, _ := s.Get("token")
		read <- v
		return core.View{}
	}, opts)
	defer e.Close()
	select {
	case v := <-read:
		if v != "kept" || len(mem.Items()) != 1 {
			t.Errorf("read %q; the test's store holds %v", v, mem.Items())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the app never ran")
	}
	// Without a store of the test's, one in memory still takes the platform's place.
	opts = Options{Secrets: machineKeychain{t}}
	opts.UseTest(testhostOptions(func(uint32, *driver.MountBatch) {}), &TestPin{})
	if _, ok := opts.Secrets.(*driver.MemorySecrets); !ok {
		t.Errorf("a test keeps its secrets in %T", opts.Secrets)
	}
}

type rateRecorder struct {
	*recorder
	rates []driver.FrameRateRange
}

func (p *rateRecorder) SetPreferredFrameRate(rate driver.FrameRateRange) {
	p.rates = append(p.rates, rate)
}

func TestFrameRequestsStayOnTheirSurface(t *testing.T) {
	app := &recorder{mounted: make(chan struct{}, 1)}
	seen := make(chan int, 3)
	e := Start(app, func(cx *core.Scope) core.Node {
		cx.RequestAnimationFrame(func(time.Time) { seen <- 0 })
		for _, hz := range []int{30, 60} {
			cx.OpenWindow(core.WindowOptions{}, func(w *core.Scope) core.Node {
				w.SetPreferredFrameRate(core.FrameRateRange{Preferred: float32(hz)})
				w.RequestAnimationFrame(func(time.Time) { seen <- hz })
				return core.View{}
			})
		}
		return nil
	}, Options{Windows: &core.WindowOptions{}})
	defer e.Close()
	e.Sync()
	var ids []uint32
	for _, b := range app.batches {
		for _, item := range b.Items {
			if item.Kind == driver.ItemOpenSurface {
				ids = append(ids, item.Surface.ID)
			}
		}
	}
	if len(ids) != 2 {
		t.Fatalf("the app opened %d surfaces, want 2", len(ids))
	}
	first := &rateRecorder{recorder: &recorder{frames: make(chan struct{}, 4)}}
	second := &rateRecorder{recorder: &recorder{frames: make(chan struct{}, 4)}}
	firstEngine := e.Attach(ids[0], first)
	secondEngine := e.Attach(ids[1], second)
	// Desktop backends report their windows' state, so a tick from a window that does not drive the app's tree cannot satisfy it.
	firstEngine.SetFrameWindow(true, false)
	secondEngine.SetFrameWindow(true, false)
	e.SetFrameWindow(false, false)
	e.Sync()
	for _, tc := range []struct {
		p  *rateRecorder
		hz float32
	}{{first, 30}, {second, 60}} {
		want := driver.FrameRateRange{Minimum: tc.hz, Maximum: tc.hz, Preferred: tc.hz}
		if len(tc.p.rates) != 1 || tc.p.rates[0] != want {
			t.Errorf("surface %g Hz received settings %v, want %v", tc.hz, tc.p.rates, want)
		}
		// The first window also drives the app's tree, which may ask it again.
		if len(tc.p.frames) == 0 {
			t.Errorf("surface %g Hz received no frame request", tc.hz)
		}
	}
	secondEngine.FrameAt(time.Now())
	e.Sync()
	if len(seen) != 1 || <-seen != 60 {
		t.Fatal("the second surface's frame did not reach it alone")
	}
	firstEngine.FrameAt(time.Now())
	e.Sync()
	if len(seen) != 2 || <-seen != 30 || <-seen != 0 {
		t.Fatal("the first surface's frame did not reach it and the app")
	}
}
