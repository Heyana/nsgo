// Package engine runs an app on a goroutine of its own and hands the
// platform one MountBatch per commit.
package engine

import (
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/internal/driver"
)

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
}

// observed is a platform whose batches a test host sees first.
type observed struct {
	Platform
	observe func(*driver.MountBatch)
}

func (o observed) Mount(b *driver.MountBatch) {
	o.observe(b)
	o.Platform.Mount(b)
}

// App builds the root's content once.
type App = func(cx *core.Scope) core.Node

// Options configure an engine.
type Options struct {
	// Size is the surface's size at start.
	Size driver.Size
	// Environment is what the platform knows at start.
	Environment driver.Environment
	// Clock runs the app's timers; the system's when nil.
	Clock core.Clock
	// Logger receives failures the app does not catch; slog's default
	// when nil.
	Logger *slog.Logger
	// Storage keeps the app's entries; in memory for the engine's life
	// when nil. Platforms pass OpenFileStorage in the data directory.
	Storage *core.Store
	// DataDir and CacheDir are the app's directories, made when the
	// engine starts; "" where the target has none.
	DataDir, CacheDir string
	// Appearance is the appearance the app starts in, forced as
	// Scope.SetAppearance forces it; the system's by default.
	Appearance driver.Appearance
	// Host is what an embedding host gives the view the app runs in.
	Host HostOptions
	// Perf collects this app's commit timings; nil measures nothing.
	Perf *PerfMonitor
	// Thread runs first on the OS thread the engine keeps, before the
	// app. The engine turns input into frames, as latency-critical as the
	// main thread: a platform whose scheduler would put it on slower
	// cores raises its priority here.
	Thread func()
	// Observe sees each batch before the platform does, and Event each
	// event the platform reports, as a test host follows the app; Pin
	// turns each environment the platform reports into the app's. None
	// may block.
	Observe func(*driver.MountBatch)
	Event   func(driver.Event)
	Pin     func(driver.Environment) driver.Environment
}

// Host and Tokens are core's, for backends.
type (
	Host     = core.Host
	HostCall = core.HostCall
	Tokens   = core.Tokens
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
	// OnError hears each failure the app does not catch, after the
	// Logger, on the engine goroutine; it must not block.
	OnError func(error)
	// DataDir is the app's data directory, its storage's, instead of the
	// system's for the app's ID.
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

// Engine is one running app: one tree, one reactive graph, one goroutine.
// Its methods are safe to call from any goroutine and never block, so a
// platform's main thread can call them.
type Engine struct {
	mu     sync.Mutex
	event  func(driver.Event)
	pin    func(driver.Environment) driver.Environment
	raw    driver.Environment // the platform's last, before pinning
	host   HostEnvironment
	queue  []message
	wake   chan struct{}
	done   chan struct{}
	exited chan struct{}
	once   sync.Once
}

type message struct {
	frame  bool
	event  driver.Event
	resize *driver.Size
	env    *driver.Environment
	at     *driver.Event // a press to hit test
	tokens *core.Tokens
	fn     func()
}

// Start runs app on a new goroutine.
func Start(p Platform, app App, opts Options) *Engine {
	e := &Engine{
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
		exited: make(chan struct{}),
		event:  opts.Event,
		pin:    opts.Pin,
	}
	var tokens Tokens
	if hs := opts.Host.Settings; hs != nil {
		e.host, tokens = hs.Environment, hs.Tokens
		hs.engine = e
	}
	if dir := opts.Host.DataDir; dir != "" && opts.Observe == nil {
		opts.DataDir = dir
		opts.Storage = OpenFileStorage(dir)
	}
	e.raw = opts.Environment
	opts.Environment = e.effective(opts.Environment)
	if observe := opts.Observe; observe != nil {
		p = observed{p, observe}
	}
	go e.run(p, app, opts, tokens)
	return e
}

func (e *Engine) run(p Platform, app App, opts Options, tokens Tokens) {
	defer close(e.exited)
	// Text engines such as Android's need the same OS thread each call.
	runtime.LockOSThread()
	if opts.Thread != nil {
		opts.Thread()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	for _, dir := range []string{opts.DataDir, opts.CacheDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("nsgo: the app's directory could not be made", "err", err)
		}
	}
	root := core.NewRoot(core.RootOptions{
		Post:  func(fn func()) { e.push(message{fn: fn}) },
		Clock: opts.Clock,
		Report: func(err error) {
			logger.Error("nsgo: the app failed", "err", err)
			if opts.Host.OnError != nil {
				opts.Host.OnError(err)
			}
		},
		// The platform owns the display frame: the engine only asks.
		RequestFrame: p.RequestFrame,
		Storage:      opts.Storage,
		DataDir:      opts.DataDir,
		CacheDir:     opts.CacheDir,
		Host:         opts.Host.Channel,
	})
	defer root.Dispose()
	tree := core.NewTree(root, p)
	tree.Resize(opts.Size)
	tree.SetEnvironment(opts.Environment)
	tree.SetAppearance(opts.Appearance)
	tree.SetTokens(tokens)
	perf := opts.Perf
	// commit applies what changed and records what it cost when a monitor
	// measures; start is when the wake-up that led here began.
	commit := func(start time.Time) {
		if perf == nil {
			if b := tree.Commit(); b != nil {
				p.Mount(b)
			}
			return
		}
		coreStart := time.Now()
		b := tree.Commit()
		core := time.Since(coreStart)
		if b != nil {
			app := time.Since(start) - core
			if app < 0 {
				app = 0
			}
			b.Sent = time.Now()
			perf.RecordCommit(CommitTiming{App: app, Core: core, Items: len(b.Items)})
			p.Mount(b)
		}
		perf.RecordBusy(time.Since(start))
	}
	var start time.Time
	if perf != nil {
		start = time.Now()
	}
	tree.Mount(app)
	commit(start)
	for {
		select {
		case <-e.wake:
		case <-e.done:
			return
		}
		if perf != nil {
			start = time.Now()
		}
		for _, m := range e.take() {
			switch {
			case m.frame:
				root.RunFrame()
			case m.fn != nil:
				m.fn()
			case m.resize != nil:
				tree.Resize(*m.resize)
			case m.env != nil:
				tree.SetEnvironment(*m.env)
			case m.at != nil:
				tree.PressAt(*m.at)
			case m.tokens != nil:
				tree.SetTokens(*m.tokens)
			default:
				tree.Dispatch(m.event)
			}
		}
		commit(start)
	}
}

// push queues m and wakes the engine. A mergeable event replaces one of
// the same kind for the same view still queued, a resize the last resize,
// and a frame request the one still queued, so bursts cost one update
// each.
func (e *Engine) push(m message) {
	e.mu.Lock()
	merged := false
	for i := range e.queue {
		q := &e.queue[i]
		switch {
		case m.frame && q.frame,
			m.resize != nil && q.resize != nil,
			m.env != nil && q.env != nil,
			m.tokens != nil && q.tokens != nil,
			m.event.Mergeable() && q.event.Mergeable() && q.event.Kind == m.event.Kind && q.event.ID == m.event.ID:
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
	select {
	case e.wake <- struct{}{}:
	default:
	}
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
		e.event(ev)
	}
	e.push(message{event: ev})
}

// Frame reports that the display frame the app asked for has begun. The
// engine runs the callbacks waiting for it on its goroutine, then commits.
func (e *Engine) Frame() { e.push(message{frame: true}) }

// Resize sets the surface size.
func (e *Engine) Resize(size driver.Size) { e.push(message{resize: &size}) }

// SetEnvironment reports what the platform knows about where the app
// runs; only the latest report queued is applied.
func (e *Engine) SetEnvironment(env driver.Environment) {
	e.mu.Lock()
	e.raw = env
	e.mu.Unlock()
	env = e.effective(env)
	e.push(message{env: &env})
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
func (e *Engine) PressAt(ev driver.Event) { e.push(message{at: &ev}) }

// Close stops the app, disposing its graph, and waits for the engine
// goroutine to exit. It must not be called from the engine goroutine.
func (e *Engine) Close() {
	e.once.Do(func() { close(e.done) })
	<-e.exited
}
