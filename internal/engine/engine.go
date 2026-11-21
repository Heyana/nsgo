// Package engine runs an app on a goroutine of its own and hands the
// platform one MountBatch per commit.
package engine

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SharedMounter is a Platform that applies each batch within Mount, on
// the engine goroutine, as the web does: its batches are the tree's own
// items, not copies (core.Tree.CommitShared).
type SharedMounter interface {
	Platform
	MountsAtOnce()
}

// ApproximateTextSize estimates text geometry if a platform cannot measure it.
func ApproximateTextSize(t *driver.AttributedText, width driver.Points) driver.Size {
	return (core.ApproxTextEngine{}).Prepare(t).Measure(width)
}

// Platform is what a backend gives the engine.
type Platform interface {
	driver.TextEngine
	// Mount receives each commit on the engine goroutine. The platform
	// applies it on its main thread, in one hop, without waiting.
	Mount(batch *driver.MountBatch)
	// RequestFrame asks for the next display frame. The platform calls
	// Frame when that frame begins, once for each request, and only then.
	// It is called from the engine goroutine, so it must not block. A
	// platform with no display frame leaves it empty, and the app's frame
	// callbacks wait.
	RequestFrame()
	// SubmitGPU submits an off-commit GPUFrame directly to the platform's
	// render thread without triggering a UI view-tree commit (docs/architecture/gpu.md).
	// Called on the engine goroutine; must not block.
	SubmitGPU(frame driver.GPUFrame)
}

// observed is a platform whose batches a test host sees first. With a
// clipboard of the test's, the app's clipboard items go to it instead:
// reads are answered there, and the platform never sees them.
type observed struct {
	Platform
	surface   uint32
	observe   func(uint32, *driver.MountBatch)
	diskSpace *driver.DiskSpaceResult
	clipboard *driver.MemoryClipboard
	takePhoto func(driver.PickRequest, func(*driver.PickedFile, error))
	scanCode  func(driver.ScanRequest, func(*driver.ScannedCode, error))
	dispatch  func(driver.Event)
}

// platform is what the engine runs on in o's place: o itself, or, over a platform that registers fonts, o with its RegisterFont, which embedding the Platform interface does not promote (#313).
func (o observed) platform() Platform {
	if _, ok := o.Platform.(driver.FontRegistrar); ok {
		return observedFonts{o}
	}
	return o
}

// observedFonts is an observed platform that registers fonts.
type observedFonts struct{ observed }

var _ driver.FontRegistrar = observedFonts{}

func (o observedFonts) RegisterFont(data []byte) error {
	return o.Platform.(driver.FontRegistrar).RegisterFont(data)
}

func (o observed) Mount(b *driver.MountBatch) {
	o.observe(o.surface, b)
	if o.clipboard != nil || o.takePhoto != nil || o.scanCode != nil || o.diskSpace != nil {
		var kept []driver.MountItem
		for i := range b.Items {
			if o.takePhoto != nil && b.Items[i].Kind == driver.ItemTakePhoto && b.Items[i].Pick != nil {
				item := b.Items[i]
				o.takePhoto(*item.Pick, func(photo *driver.PickedFile, err error) {
					ev := driver.Event{Kind: driver.EventPick, Count: item.Count}
					switch {
					case err != nil:
						ev.Text = err.Error()
					case photo != nil:
						ev.Data = []driver.PickedFile{*photo}
					}
					o.dispatch(ev)
				})
				continue
			}
			if it := b.Items[i]; it.Kind == driver.ItemScanCode && o.scanCode != nil {
				o.answerScan(it)
				continue
			}
			if it := b.Items[i]; it.Kind == driver.ItemDiskSpace && o.diskSpace != nil {
				o.dispatch(driver.DiskSpaceEvent(it.Count, o.diskSpace.Space, o.diskSpace.Err))
				continue
			}
			if o.clipboard == nil {
				kept = append(kept, b.Items[i])
				continue
			}
			ev, answer, taken := o.clipboard.Take(&b.Items[i])
			switch {
			case answer:
				o.dispatch(ev)
			case !taken:
				kept = append(kept, b.Items[i])
			}
		}
		if len(kept) != len(b.Items) {
			// The test reads the batch it observed: the platform gets a
			// copy.
			c := *b
			c.Items = kept
			b = &c
		}
	}
	o.Platform.Mount(b)
}

// answerScan answers it, a code to scan, from o's scanCode instead of the platform's camera.
func (o observed) answerScan(it driver.MountItem) {
	o.scanCode(*it.Scan, func(code *driver.ScannedCode, err error) {
		ev := driver.Event{Kind: driver.EventPick, Count: it.Count}
		switch {
		case err != nil:
			ev.Text = err.Error()
		case code != nil:
			ev.Data = code
		}
		o.dispatch(ev)
	})
}

// App builds the root's content once.
type App = func(cx *core.Scope) core.Node

// Options configure an engine.
type Options struct {
	// CheckCommits enables full-layout diagnostics for this engine only. In a test or development build it also checks that only the engine's goroutine touches its graph, and it re-panics an internal fault after the fault's cleanup, so a test fails rather than only reporting it.
	CheckCommits bool
	// Size is the surface's size at start.
	Size driver.Size
	// Geometry is the fold geometry measured at Size, which the first commit is laid out with (Engine.Resize); the zero Geometry is a surface without a fold.
	Geometry driver.Geometry
	// Environment is what the platform knows at start.
	Environment driver.Environment
	// NetworkWatcher reads and follows the system route on the app's first network read.
	NetworkWatcher driver.NetworkWatcher
	// Clock runs the app's timers; the system's when nil.
	Clock core.Clock
	// Storage keeps the app's entries; in memory for the engine's life
	// when nil. Platforms pass OpenFileStorage in the data directory.
	Storage *core.Store
	// Secrets is the platform's secure store (core.Scope.Secrets); nil where it has none.
	Secrets driver.SecretStore
	// DataDir, CacheDir and LogsDir are the app's directories, made when the engine starts; "" where the target has none.
	DataDir, CacheDir, LogsDir string
	// Appearance is the appearance the app starts in, forced as
	// Scope.SetAppearance forces it; the system's by default.
	Appearance driver.Appearance
	// Host is what an embedding host gives the view the app runs in.
	Host HostOptions
	// Windows, set where the platform opens windows, are the main
	// window's options (core.RootOptions.Windows): the first surface is
	// the app's own, which shows no view, and the platform attaches each
	// surface the app opens.
	Windows *WindowOptions
	// HotKeys, set where the platform registers global shortcuts but opens no windows, has the first surface take the app's global shortcuts (core.RootOptions.HotKeys).
	HotKeys bool
	// Tray is set where the platform supports app-level tray icons without
	// opening additional windows (core.RootOptions.Tray).
	Tray bool
	// Perf collects this app's commit timings; nil measures nothing.
	Perf *PerfMonitor
	// Thread runs first on the OS thread the engine keeps, before the
	// app. The engine turns input into frames, as latency-critical as the
	// main thread: a platform whose scheduler would put it on slower
	// cores raises its priority here.
	Thread func()
	// Observe sees each batch of every surface before the platform does, and Event each event the platform reports, with the surface's number (0 for the first), as a test host follows the app; Pin turns each environment the platform reports into the app's. None may block.
	Observe func(surface uint32, b *driver.MountBatch)
	Event   func(surface uint32, ev driver.Event)
	Pin     func(driver.Environment) driver.Environment
	// Clipboard, with Observe, is the clipboard of a test's own, which
	// the app's clipboard commands use instead of the platform's.
	Clipboard *driver.MemoryClipboard
	// DiskSpace, with Observe, answers capacity requests with the test's readings.
	DiskSpace *driver.DiskSpaceResult
	// TakePhoto, with Observe, answers camera requests in a test instead of handing them to the platform.
	TakePhoto func(driver.PickRequest, func(*driver.PickedFile, error))
	// ScanCode, with Observe, answers code scans in a test instead of handing them to the platform.
	ScanCode func(driver.ScanRequest, func(*driver.ScannedCode, error))
	// LogPerf logs Perf's readings once a second while the engine runs.
	LogPerf bool
}

// Host, Tokens and WindowOptions are core's, for backends.
type (
	WindowOptions = core.WindowOptions
	Host          = core.Host
	HostCall      = core.HostCall
	Tokens        = core.Tokens
)

// NewHost returns a new channel to an embedding host (core.NewHost).
func NewHost() *Host { return core.NewHost() }

// HostOptions are what an embedding host gives a view's engine.
type HostOptions struct {
	// Channel is the channel between the host and the app
	// (core.RootOptions.Host).
	Channel *Host
	// Settings are the host's settings of the view.
	Settings *HostSettings
	// OnError hears each failure the app does not catch, after the log,
	// on the engine goroutine; it must not block.
	OnError func(error)
	// DataDir is the app's data directory, its storage's, instead of the system's for the app's ID; its logs directory goes in DataDir/logs.
	DataDir string
}

// HostSettings are an embedding host's settings of one view: kept until
// the view's engine starts, which has them from its first frame, and sent
// to it after. Only the platform's main thread uses them.
type HostSettings struct {
	Environment HostEnvironment
	Tokens      Tokens
	engine      *Engine
}

// SetEnvironment sets what the host sets over the platform's environment.
func (h *HostSettings) SetEnvironment(env HostEnvironment) {
	h.Environment = env
	if h.engine != nil {
		h.engine.SetHostEnvironment(env)
	}
}

// SetTokens sets the values the host themes the app with.
func (h *HostSettings) SetTokens(t Tokens) {
	h.Tokens = t
	if h.engine != nil {
		h.engine.SetTokens(t)
	}
}

// HostEnvironment is what an embedding host sets over the platform's
// environment for its view; zero values follow the platform's.
type HostEnvironment struct {
	// Locale is a BCP 47 tag ("de-CH") the app formats for instead of
	// the system's language; the layout direction stays the system's.
	Locale string
	// FontScale is the text size multiplier, standing in for the
	// system's.
	FontScale float32
	// Appearance light or dark stands in for the system's; an appearance
	// the app forces still wins over it.
	Appearance driver.Appearance
}

// over returns env with what h sets.
func (h HostEnvironment) over(env driver.Environment) driver.Environment {
	if h.Locale != "" {
		env.Locale = h.Locale
	}
	if h.FontScale > 0 {
		env.FontScale = h.FontScale
	}
	switch h.Appearance {
	case driver.AppearanceLight:
		env.Dark = false
	case driver.AppearanceDark:
		env.Dark = true
	}
	return env
}

// Engine is one running app: one reactive graph, one goroutine, and a
// tree per surface. An Engine value is one surface's: Start returns the
// first's, and Attach another's, which the app opened. Its methods are
// safe to call from any goroutine and never block, so a platform's main
// thread can call them.
type Engine struct {
	*runner
	surface uint32
	// modality is the input modality last reported (InputModality).
	modality atomic.Uint32
	// frameWindow is the window state last queued (SetFrameWindow).
	// The runner's mutex protects it.
	frameWindow frameWindow
}

// runner is what an app's surfaces share.
type runner struct {
	mu      sync.Mutex
	event   func(uint32, driver.Event)
	pin     func(driver.Environment) driver.Environment
	raw     driver.Environment // the first surface's last, before pinning
	host    HostEnvironment
	queue   []message
	stopped bool // protected by mu; stopped engines reject new messages
	wake    chan struct{}
	done    chan struct{}
	exited  chan struct{}
	// quitted is closed once the OnQuit handlers returned.
	quitted chan struct{}
	once    sync.Once
	// forcedQuit is set before done closes, when the system ends the app's session.
	forcedQuit bool
	// tree is the first surface's graph, which only the engine's goroutine reads: Language posts a function there.
	tree *core.Tree
	// graph numbers the engine's reactive graph within the process while devtools.Enabled (core.Tree.Graph), 0 otherwise.
	graph atomic.Int64
}

type message struct {
	// surface identifies the surface a frame, environment, resize, or
	// event came from.
	surface uint32
	frame   bool
	// frameAt is the display's time for the frame, zero where the platform gave none.
	frameAt     time.Time
	frameWindow frameWindow
	event       driver.Event
	resize      *driver.Size
	// geometry is the fold geometry measured at resize, which travels with it.
	geometry driver.Geometry
	// resized are closed once the commit after the resize is mounted.
	resized []chan struct{}
	// synced is closed once this pass's commits have been mounted.
	synced  chan struct{}
	env     *driver.Environment
	at      *driver.Event // a press to hit test
	keyDown *keyDownRequest
	tokens  *core.Tokens
	fn      func()
	fonts   bool
	// attach is the platform of an opened surface; detach ends it.
	attach Platform
	detach bool
	// hidden is closed once the OnQuit handlers ran for a hidden app.
	hidden chan struct{}
}

// keyDownRequest is a key down a platform waits on (DispatchKeyDown): done hears once whether the view consumed it, false when the engine ends first.
type keyDownRequest struct {
	event driver.Event
	done  chan bool
}

// Trace, when set, hears each commit's batch, before the platform, and
// how long the commit took, on the engine goroutine of every engine the
// process starts from then on; it must not block. nsgo dev -trace's
// builds set it, as their dev client starts.
var Trace func(b *driver.MountBatch, took time.Duration)

// Start runs app on a new goroutine, its first surface on p.
func Start(p Platform, app App, opts Options) *Engine {
	e := &Engine{runner: &runner{
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
		quitted: make(chan struct{}),
		event:   opts.Event,
		pin:     opts.Pin,
	}}
	var tokens Tokens
	if hs := opts.Host.Settings; hs != nil {
		e.host, tokens = hs.Environment, hs.Tokens
		hs.engine = e
	}
	if dir := opts.Host.DataDir; dir != "" && opts.Observe == nil {
		opts.DataDir = dir
		opts.LogsDir = filepath.Join(dir, "logs")
		opts.Storage = OpenFileStorage(dir)
	}
	e.raw = opts.Environment
	opts.Environment = e.effective(opts.Environment)
	if observe := opts.Observe; observe != nil {
		p = observed{Platform: p, surface: 0, observe: observe, clipboard: opts.Clipboard, takePhoto: opts.TakePhoto, scanCode: opts.ScanCode, diskSpace: opts.DiskSpace, dispatch: e.Dispatch}.platform()
	}
	go e.run(p, app, opts, tokens)
	if opts.LogPerf && opts.Perf != nil {
		go e.logPerf(opts.Perf)
	}
	return e
}

// logPerf logs m's readings once a second until the engine closes.
func (e *Engine) logPerf(m *PerfMonitor) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-tick.C:
			log.Print("nsgo: perf ", m.Snapshot())
		}
	}
}

// Attach attaches platform p to the surface the app opened as id
// (driver.ItemOpenSurface) and returns the surface's Engine, which p
// reports to as the first surface's platform does: its size and
// environment first. Closing it detaches p.
func (e *Engine) Attach(id uint32, p Platform) *Engine {
	e.push(message{surface: id, attach: p})
	return &Engine{runner: e.runner, surface: id}
}

// surface is an attached surface: its tree and platform.
type surface struct {
	id          uint32
	frameWindow frameWindow
	lastFocused uint64
	tree        *core.Tree
	p           Platform
	commit      func() *driver.MountBatch
}

// destroyed is the batch that tells an opened surface's platform that its
// tree is gone.
func destroyed() *driver.MountBatch {
	return &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemWindow,
		Window: &driver.WindowCommand{Action: driver.WindowDestroy}}}}
}

func (e *Engine) run(p Platform, app App, opts Options, tokens Tokens) {
	// Establish the boundary before thread setup and the initial mount too.
	surfaces := map[uint32]*surface{0: {p: p}}
	var root *core.Scope
	var pending []chan struct{}
	var taken []message
	report := func(err error) {
		log.Printf("nsgo: the app failed: %v", err)
		if opts.Host.OnError != nil {
			opts.Host.OnError(err)
		}
	}
	defer func() {
		fault := recover()
		var err error
		if fault != nil {
			err = &core.PanicError{Value: fault, Stack: debug.Stack()}
		}
		// Stop accepting work before cleanup callbacks can post any more.
		e.stop()
		for _, done := range pending {
			close(done)
		}
		for _, m := range taken {
			discardMessage(m)
		}
		if fault != nil {
			if root != nil {
				faultCleanup(root.Abort)
			}
			for _, s := range surfaces {
				faultCleanup(func() { s.p.Mount(destroyed()) })
			}
			faultCleanup(func() { report(err) })
		}
		if fault != nil && opts.CheckCommits {
			panic(fault)
		}
		close(e.exited)
	}()
	// Text engines such as Android's need the same OS thread each call.
	runtime.LockOSThread()
	if opts.Thread != nil {
		opts.Thread()
	}
	for _, dir := range []string{opts.DataDir, opts.CacheDir, opts.LogsDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("nsgo: the app's directory could not be made: %v", err)
		}
	}
	var frames frameRequests
	root = core.NewRoot(core.RootOptions{
		CheckCommits: opts.CheckCommits,
		Post:         func(fn func()) { e.push(message{fn: fn}) },
		Clock:        opts.Clock,
		Report:       report,
		// The platform owns the display frame: the engine only asks.
		RequestFrame:   func() { frames.wanted = true },
		Storage:        opts.Storage,
		Secrets:        opts.Secrets,
		NetworkWatcher: opts.NetworkWatcher,
		DataDir:        opts.DataDir,
		CacheDir:       opts.CacheDir,
		LogsDir:        opts.LogsDir,
		Host:           opts.Host.Channel,
		Windows:        opts.Windows,
		HotKeys:        opts.HotKeys,
		Tray:           opts.Tray,
	})
	tree := core.NewTree(root, p)
	e.tree = tree
	e.graph.Store(int64(tree.Graph()))
	tree.Resize(opts.Size, opts.Geometry)
	tree.SetEnvironment(opts.Environment)
	tree.SetAppearance(opts.Appearance)
	tree.SetTokens(tokens)
	perf, trace := opts.Perf, Trace
	// attach makes t's surface p's. A test host's observer may keep a
	// batch past the next commit.
	attach := func(t *core.Tree, p Platform) *surface {
		s := &surface{id: t.ID(), tree: t, p: p, commit: t.Commit}
		t.SetGPUSubmitter(p.SubmitGPU)
		if _, ok := p.(SharedMounter); ok && opts.Observe == nil {
			s.commit = t.CommitShared
		}
		if opts.Observe != nil {
			// A test's steps reach every view, as NSH's test drivers.
			t.SetViewPacing(false)
		}
		surfaces[t.ID()] = s
		return s
	}
	attach(tree, p)
	// commitOne applies what changed in s and records what it cost when a
	// monitor measures or a trace hears; start is when the wake-up that
	// led here began.
	commitOne := func(s *surface, start time.Time) {
		if perf == nil && trace == nil {
			if b := s.commit(); b != nil {
				s.p.Mount(b)
			}
			return
		}
		coreStart := time.Now()
		b := s.commit()
		core := time.Since(coreStart)
		if b != nil && trace != nil {
			if b.Causes != nil && !start.IsZero() {
				// The app's code ran from the wake-up to the commit.
				b.Causes.App = max(coreStart.Sub(start), 0)
			}
			trace(b, core)
		}
		if perf == nil {
			if b != nil {
				s.p.Mount(b)
			}
			return
		}
		if b != nil {
			app := time.Since(start) - core
			if app < 0 {
				app = 0
			}
			b.Sent = time.Now()
			perf.RecordCommit(CommitTiming{App: app, Core: core, Items: len(b.Items)})
			s.p.Mount(b)
		}
	}
	// commit commits every attached surface, the first first, and ends
	// the opened ones that closed.
	commit := func(start time.Time) {
		commitOne(surfaces[0], start)
		for _, t := range slices.Clone(tree.Surfaces()) {
			s := surfaces[t.ID()]
			switch {
			case t.Closed():
				t.Forget()
				if s != nil {
					delete(surfaces, t.ID())
					s.p.Mount(destroyed())
				}
			case s != nil:
				commitOne(s, start)
			}
		}
		if perf != nil {
			perf.RecordBusy(time.Since(start))
		}
	}
	// start is when a wake-up began, for the app's time before its commit: measured while a monitor or a trace hears it.
	var start time.Time
	if perf != nil || trace != nil {
		start = time.Now()
	}
	tree.Mount(app)
	commit(start)
	frames.request(surfaces)
	for {
		select {
		case <-e.wake:
		case <-e.done:
			if e.forcedQuit {
				tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Forced: true})
			}
			root.Quitting()
			close(e.quitted)
			root.Dispose()
			return
		}
		// A push between a wake and the take that drained its message
		// leaves a second wake and nothing to do: no pass for it.
		taken = e.take()
		if len(taken) == 0 {
			continue
		}
		if perf != nil || trace != nil {
			start = time.Now()
		}
		// Keep every resize and sync wait until this entire pass commits, including resizes for a surface that closed or has not attached yet. A fault closes them too.
		for i := range taken {
			pending = append(pending, taken[i].resized...)
			taken[i].resized = nil
			if taken[i].synced != nil {
				pending = append(pending, taken[i].synced)
			}
		}
		for i, m := range taken {
			switch {
			case m.synced != nil:
				continue
			case m.frame:
				if !frames.receive(m.surface, surfaces) {
					continue
				}
				// A display's time is on the system clock's base, which an app's own clock does not share.
				at := m.frameAt
				if opts.Clock != nil {
					at = time.Time{}
				}
				root.RunFrameAt(at)
				continue
			case m.fn != nil:
				m.fn()
				continue
			case m.hidden != nil:
				root.Hiding()
				close(m.hidden)
				taken[i].hidden = nil
				continue
			case m.tokens != nil:
				tree.SetTokens(*m.tokens)
				continue
			case m.fonts:
				tree.FontsChanged()
				for _, t := range tree.Surfaces() {
					t.FontsChanged()
				}
				continue
			case m.attach != nil:
				surfaceIndex := slices.IndexFunc(tree.Surfaces(), func(t *core.Tree) bool { return t.ID() == m.surface })
				if surfaceIndex < 0 || tree.Surfaces()[surfaceIndex].Closed() {
					// It closed before its platform attached it.
					m.attach.Mount(destroyed())
					taken[i].attach = nil
					continue
				}
				opened := m.attach
				if opts.Observe != nil {
					// A test host follows the windows the app opens too, and their clipboard commands use the test's clipboard.
					opened = observed{Platform: opened, surface: m.surface, observe: opts.Observe, clipboard: opts.Clipboard, takePhoto: opts.TakePhoto, scanCode: opts.ScanCode, diskSpace: opts.DiskSpace, dispatch: (&Engine{runner: e.runner, surface: m.surface}).Dispatch}.platform()
				}
				attach(tree.Surfaces()[surfaceIndex], opened)
				taken[i].attach = nil
				continue
			}
			s := surfaces[m.surface]
			if s == nil {
				if m.keyDown != nil {
					m.keyDown.done <- false
					taken[i].keyDown = nil
				}
				continue
			}
			t := s.tree
			switch {
			case m.detach:
				if m.surface != 0 {
					delete(surfaces, m.surface)
				}
			case m.frameWindow != frameWindowUnknown:
				frames.updateWindow(s, m.frameWindow)
			case m.resize != nil:
				// One batch, so no effect sees the new size with the old environment it came with.
				root.Batch(func() {
					t.Resize(*m.resize, m.geometry)
					if m.env != nil {
						t.SetEnvironment(*m.env)
					}
				})
			case m.env != nil:
				t.SetEnvironment(*m.env)
			case m.at != nil:
				t.PressAt(*m.at)
			case m.keyDown != nil:
				// The platform waits for the answer, not for the commit.
				m.keyDown.done <- t.KeyDown(m.keyDown.event)
				taken[i].keyDown = nil
			default:
				t.Dispatch(m.event)
			}
		}
		commit(start)
		frames.request(surfaces)
		for _, done := range pending {
			close(done)
		}
		pending = nil
		taken = nil
	}
}

// faultCleanup lets a broken cleanup or platform fail without preventing the remaining surfaces from being destroyed or masking the original fault.
func faultCleanup(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("nsgo: fault cleanup failed: %v\n%s", r, debug.Stack())
		}
	}()
	fn()
}

func discardMessage(m message) {
	for _, done := range m.resized {
		close(done)
	}
	if m.hidden != nil {
		close(m.hidden)
	}
	if m.keyDown != nil {
		m.keyDown.done <- false
	}
	if m.attach != nil {
		faultCleanup(func() { m.attach.Mount(destroyed()) })
	}
}

func (e *Engine) stop() {
	e.mu.Lock()
	e.stopped = true
	queued := e.queue
	e.queue = nil
	e.mu.Unlock()
	e.once.Do(func() { close(e.done) })
	for _, m := range queued {
		discardMessage(m)
	}
}

// push queues m and wakes the engine.
func (e *Engine) push(m message) {
	if !e.enqueue(m) {
		return
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// enqueue queues m for the engine's next pass. A mergeable event replaces one of the same kind for the same view still queued, a resize the last resize of the same surface with the geometry it carries, and a display frame the one still queued from that surface, so bursts cost one update each. A wheel's turns add up into the one queued. A surface's resize and environment still queued become one message, applied in one batch.
func (e *Engine) enqueue(m message) bool {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		discardMessage(m)
		return false
	}
	if m.frameWindow != frameWindowUnknown {
		if m.frameWindow == e.frameWindow {
			e.mu.Unlock()
			return false
		}
		e.frameWindow = m.frameWindow
	}
	merged := false
	for i := range e.queue {
		q := &e.queue[i]
		same := q.surface == m.surface
		switch {
		case m.resize != nil && q.resize != nil && same:
			// The commit at the newest size answers the waits for both.
			m.resized = append(q.resized, m.resized...)
			if m.env == nil {
				m.env = q.env
			}
			*q = m
			merged = true
		case m.resize != nil && q.env != nil && same:
			// A size and the environment queued before it describe one change, as a rotation's safe area and size: the resize takes the environment's slot and carries it, unless it brings a newer one.
			if m.env == nil {
				m.env = q.env
			}
			*q = m
			merged = true
		case m.env != nil && q.resize != nil && same:
			// The same when the environment comes after the size.
			q.env = m.env
			merged = true
		case same && m.event.Kind == driver.EventWheel && q.event.Kind == driver.EventWheel && q.event.ID == m.event.ID &&
			m.event.Phase == driver.GestureChanged && q.event.Phase == driver.GestureChanged &&
			m.event.Precise == q.event.Precise && m.event.Modifiers == q.event.Modifiers:
			// Turns add up, where other moves replace each other.
			m.event.Delta.X += q.event.Delta.X
			m.event.Delta.Y += q.event.Delta.Y
			*q = m
			merged = true
		case same && m.frame && q.frame,
			m.env != nil && q.env != nil && same,
			m.tokens != nil && q.tokens != nil,
			m.fonts && q.fonts,
			same && m.event.Mergeable() && q.event.Mergeable() && q.event.Kind == m.event.Kind && q.event.ID == m.event.ID:
			*q = m
			merged = true
		}
		if merged {
			break
		}
	}
	if !merged {
		e.queue = append(e.queue, m)
	}
	e.mu.Unlock()
	return true
}

func (e *Engine) take() []message {
	e.mu.Lock()
	defer e.mu.Unlock()
	q := e.queue
	e.queue = nil
	return q
}

// Dispatch reports a platform event.
func (e *Engine) Dispatch(ev driver.Event) {
	if e.event != nil {
		e.event(e.surface, ev)
	}
	e.push(message{surface: e.surface, event: ev})
}

// DispatchBeforeFrame reports a platform event from a backend whose frame
// clock will call Frame next, as the app waits for a display frame: the
// event waits for that call to wake the engine, which then handles it and
// runs the frame in one pass, with one commit, instead of two.
func (e *Engine) DispatchBeforeFrame(ev driver.Event) {
	if e.event != nil {
		e.event(e.surface, ev)
	}
	e.enqueue(message{surface: e.surface, event: ev})
}

// DispatchKeyDown reports EventKeyDown ev and waits for the engine to run the view's OnKeyDown, after what is queued before it, returning whether it consumed the key: the platform calls it on its main thread before matching the keymap, which a consumed key skips. It returns false once the engine has ended. It must not be called from the engine goroutine.
func (e *Engine) DispatchKeyDown(ev driver.Event) bool {
	if e.event != nil {
		e.event(e.surface, ev)
	}
	request := &keyDownRequest{event: ev, done: make(chan bool, 1)}
	e.push(message{surface: e.surface, keyDown: request})
	return <-request.done
}

// InputModality reports what the user gives the surface input with
// (driver.ModalityKeyboard or ModalityPointer), as it changes: a backend
// calls it for every key and press, before the focus moves they cause.
func (e *Engine) InputModality(m uint32) {
	if e != nil && e.modality.Swap(m) != m {
		e.Dispatch(driver.Event{Kind: driver.EventInputModality, Count: m})
	}
}

// Launch is what a launch of the app on Windows or Linux asks it to open, which a second launch hands the running copy over the one-instance claim: the files (EventOpenFiles) and the links (EventOpenLinks) its command line names.
type Launch struct {
	Files []string `json:"files,omitempty"`
	Links []string `json:"links,omitempty"`
}

// Launched returns what this process's command line asks the app to open.
func Launched() Launch { return Launch{Files: LaunchFiles(), Links: LaunchLinks()} }

// LaunchFiles returns the files the command line names, as absolute paths, also those named by file URLs, as a desktop entry's %U passes them: what Windows and Linux launch an app with to open, which their backends send it as EventOpenFiles. Other arguments are left to the app. On Windows a launch that names a link opens no files: the shell puts a link into the registered command line as it is, inside quotes it may close, so a link could otherwise name local files for the app to open.
func LaunchFiles() []string {
	if runtime.GOOS == "windows" && len(LaunchLinks()) > 0 {
		return nil
	}
	var paths []string
	for _, arg := range os.Args[1:] {
		if path, ok := fileURLPath(arg); ok {
			arg = path
		}
		if fi, err := os.Stat(arg); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if abs, err := filepath.Abs(arg); err == nil {
			paths = append(paths, abs)
		}
	}
	return paths
}

// LaunchLinks returns the links the command line names (IsLink): what Windows and Linux launch an app with to open a link of a URL scheme its package registers, which their backends send it as EventOpenLinks. The app filters them by scheme itself, so a build that registers none, as a development one, takes those it is started with too.
func LaunchLinks() []string {
	var links []string
	for _, arg := range os.Args[1:] {
		if IsLink(arg) {
			links = append(links, arg)
		}
	}
	return links
}

// IsLink reports whether arg, an argument the app was launched with, is a link: an absolute URL whose scheme is not file's and is longer than one letter, which on Windows is a drive's, and that names no file.
func IsLink(arg string) bool {
	u, err := url.Parse(arg)
	if err != nil || len(u.Scheme) < 2 || u.Scheme == "file" {
		return false
	}
	_, err = os.Stat(arg)
	return err != nil
}

// fileURLPath returns the local path a file URL names.
func fileURLPath(arg string) (string, bool) {
	u, err := url.Parse(arg)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") || u.Path == "" {
		return "", false
	}
	path := u.Path
	// file:///C:/Users/a.txt names C:/Users/a.txt.
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), true
}

// Graph numbers the engine's reactive graph within the process while devtools.Enabled, 0 otherwise.
func (e *Engine) Graph() int {
	if e == nil {
		return 0
	}
	return int(e.graph.Load())
}

// Sync returns once the engine ran what was posted to it before and handed the resulting commits to the platforms, or the engine exited. It must not be called from the engine goroutine.
func (e *Engine) Sync() {
	done := make(chan struct{})
	e.push(message{synced: done})
	select {
	case <-done:
	case <-e.exited:
	}
}

// Language returns the language the app shows on its first surface (core.Env.Language), "" for an app without catalogs. It waits for what is already queued on the engine, the app's mount included, so it sees a locale the app set as it mounted; it returns "" once the engine has ended.
func (e *Engine) Language() string {
	done := make(chan string, 1)
	e.push(message{fn: func() {
		if e.tree == nil {
			done <- ""
			return
		}
		done <- e.tree.Language()
	}})
	select {
	case lang := <-done:
		return lang
	case <-e.exited:
		return ""
	}
}

// Frame reports that the display frame the app asked for has begun. The
// engine runs the callbacks waiting for it on its goroutine, then commits.
func (e *Engine) Frame() { e.push(message{surface: e.surface, frame: true}) }

// FrameAt is Frame for a frame whose time the display gave, on time.Now's base: when it shows, or when it began where the platform tells no more (core.Scope.RunFrameAt). driver.DisplayTime brings a platform's timestamp to that base.
func (e *Engine) FrameAt(at time.Time) {
	e.push(message{surface: e.surface, frame: true, frameAt: at})
}

// SetFrameWindow reports whether this window can drive frame callbacks and
// whether it has the keyboard focus. The engine uses the window's own state
// independently of the environment the app or a test sees.
func (e *Engine) SetFrameWindow(visible, focused bool) {
	state := frameWindowHidden
	if visible {
		state = frameWindowVisible
		if focused {
			state = frameWindowFocused
		}
	}
	e.push(message{surface: e.surface, frameWindow: state})
}

// Resize sets the surface size and the fold geometry measured at it, the zero Geometry for a surface without a fold. A change of the geometry alone is a resize to the same size.
func (e *Engine) Resize(size driver.Size, g driver.Geometry) {
	e.push(message{surface: e.surface, resize: &size, geometry: g})
}

// ResizeAndWait sets the surface size and its fold geometry as Resize does and returns a channel closed once the commit laid out at it is mounted: a platform that shows a new size at once waits on it, briefly.
func (e *Engine) ResizeAndWait(size driver.Size, g driver.Geometry) <-chan struct{} {
	done := make(chan struct{})
	e.push(message{surface: e.surface, resize: &size, geometry: g, resized: []chan struct{}{done}})
	return done
}

// SetEnvironment reports what the platform knows about where the app runs; only the latest report queued is applied.
func (e *Engine) SetEnvironment(env driver.Environment) {
	env = e.reported(env)
	e.push(message{surface: e.surface, env: &env})
}

// ResizeWithEnvironment sets the surface size and geometry as Resize does and reports the environment that changed with them, as a rotation's safe area, as SetEnvironment does: the app sees both in one batch, never one without the other.
func (e *Engine) ResizeWithEnvironment(size driver.Size, g driver.Geometry, env driver.Environment) {
	env = e.reported(env)
	e.push(message{surface: e.surface, resize: &size, geometry: g, env: &env})
}

// reported records env as the platform's last report for the first surface and returns what the app sees of it.
func (e *Engine) reported(env driver.Environment) driver.Environment {
	if e.surface == 0 {
		e.mu.Lock()
		e.raw = env
		e.mu.Unlock()
	}
	return e.effective(env)
}

// effective is the environment the app sees for the platform's env: the
// host's settings over it, then a test's pin.
func (e *Engine) effective(env driver.Environment) driver.Environment {
	e.mu.Lock()
	env = e.host.over(env)
	e.mu.Unlock()
	if e.pin != nil {
		env = e.pin(env)
	}
	return env
}

// Repin reports the platform's last environment again, for a pin that
// changed.
func (e *Engine) Repin() {
	e.mu.Lock()
	env := e.raw
	e.mu.Unlock()
	e.SetEnvironment(env)
}

// SetHostEnvironment sets what an embedding host sets over the
// platform's environment, from now on.
func (e *Engine) SetHostEnvironment(h HostEnvironment) {
	e.mu.Lock()
	e.host = h
	e.mu.Unlock()
	e.Repin()
}

// SetTokens gives the app an embedding host's tokens (core.Tokens).
func (e *Engine) SetTokens(t core.Tokens) { e.push(message{tokens: &t}) }

// PressAt reports a press at ev.Page in surface points, with its
// Modifiers and click Count, for platforms that draw their own views and
// leave hit testing to the engine.
func (e *Engine) PressAt(ev driver.Event) { e.push(message{surface: e.surface, at: &ev}) }

// FontsChanged prepares the app's text and records its canvases again, for fonts the platform loaded after they were registered (driver.FontRegistrar).
func (e *Engine) FontsChanged() { e.push(message{fonts: true}) }

// Close stops the app, running its OnQuit handlers and disposing its
// graph, and waits for the engine goroutine to exit; an opened surface's Engine only detaches its
// platform. It must not be called from the engine goroutine.
func (e *Engine) Close() {
	e.close(false)
}

// CloseForForcedQuit delivers the system's quit request on the engine goroutine before OnQuit, under the same bounded wait as Close.
func (e *Engine) CloseForForcedQuit() {
	e.close(true)
}

func (e *Engine) close(forced bool) {
	if e.surface != 0 {
		e.push(message{surface: e.surface, detach: true})
		return
	}
	e.once.Do(func() {
		e.forcedQuit = forced
		close(e.done)
	})
	// The OnQuit handlers have core.QuitTimeout, and the platform waits
	// no longer for one that overstays it.
	timer := time.NewTimer(core.QuitTimeout + quitGrace)
	defer timer.Stop()
	select {
	case <-e.quitted:
		<-e.exited
	case <-e.exited:
	case <-timer.C:
		log.Print("nsgo: an OnQuit handler outlasted its time; the app ends without it")
	}
}

// Hide runs the OnQuit handlers for an app the system hides and may end
// unheard (core.Scope.Hiding), keeping them and the engine for when it
// shows the app again, and waits for them as Close does.
func (e *Engine) Hide() {
	done := make(chan struct{})
	e.push(message{hidden: done})
	timer := time.NewTimer(core.QuitTimeout + quitGrace)
	defer timer.Stop()
	select {
	case <-done:
	case <-e.exited:
	case <-timer.C:
		log.Print("nsgo: an OnQuit handler outlasted its time; the app hides without it")
	}
}

// quitGrace is how long Close waits past core.QuitTimeout for the
// engine to end once its quit handlers' context is done.
const quitGrace = 100 * time.Millisecond
