package core

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The reactive graph, scheduled as Solid's: signals hold values, memos
// cache what they compute, effects run again when what they read changes.
// Bindings of elements run before the app's effects. Every computation
// and every scope owns what is created while it runs, and disposing an
// owner disposes all of it.
//
// One graph belongs to one engine and runs on its goroutine, so nothing
// here locks. Other goroutines reach it with Scope.Post.

// Scope owns what is created under it: memos, effects, cleanups, child
// scopes and the views of its elements. Components receive one as cx.
//
// Go has no goroutine-local state, so cx is how code finds its graph.
// While the graph runs code, reactive primitives (Effect, Memo, Cleanup, AfterFunc, Context, Child, Key.Provide and event registrations) belong to the innermost running owner, whichever cx the code captured, so closures inside an Effect clean up when the Effect runs again; use Scope.Run to switch the running owner when calling those from another handler. Explicit-lifetime resources and surfaces (OpenWindow, OpenPanel, NewTray, OpenOverlay, NewAudioPlayer, LoadSound, NewHolder and Post) instead belong to cx itself, so passing a longer-lived scope such as the app's from an OnPress handler keeps them alive after the button's scope goes.
type Scope struct {
	rt       *runtime
	parent   *Scope
	first    *Scope // owned scopes, oldest first
	last     *Scope
	prev     *Scope
	next     *Scope
	cleanups []func()
	timerMu  sync.Mutex
	timers   map[*afterTimer]struct{}
	values   map[any]any
	catch    func(error)
	ctx      context.Context
	cancel   context.CancelFunc
	work     *workObserver
	comp     *computation // set when the scope is a computation's
	// surf is the tree of the window the scope is in, inherited by what
	// it owns; nil leaves it the graph's first.
	surf *Tree
	// park, set on a list cell's scope, holds back what runs under it while the cell is spare.
	park     *parking
	disposed bool
}

// parking holds back the computations under a list cell's scope while the cell is spare, as its index may name a row that is gone or another row's, and runs them when the cell shows a row again: its effects, and its memos read from outside it, for their readers. guard, the list's computation that frees and binds its cells, is brought up to date before anything under the scope runs, so a row's binding never runs with an index its list has not settled.
type parking struct {
	guard    *computation
	parked   bool
	deferred []*computation
}

// park holds back what runs under p's scope.
func (rt *runtime) park(p *parking) {
	if !p.parked {
		p.parked = true
		rt.parked++
	}
}

// unpark lets what p held back run, in the running batch or the next.
func (rt *runtime) unpark(p *parking) {
	if p.parked {
		p.parked = false
		rt.parked--
	}
	for _, c := range p.deferred {
		if !c.disposed && c.state != clean {
			rt.enqueue(c)
		}
	}
	clear(p.deferred)
	p.deferred = p.deferred[:0]
}

// RootOptions connect a graph to its host, an engine or a test driver.
type RootOptions struct {
	// CheckCommits compares incremental commits with a full layout and panics on changes the incremental pass missed. Tests enable it; performance measurements leave it off for their own graph. In a native test build (nsgotest or nsgocheck, not WebAssembly) it also records the goroutine that calls NewRoot as the graph's owner and panics when another goroutine writes to the graph, commits it or runs code in it; a development build (nsgodev) checks every native graph whatever this says.
	CheckCommits bool
	// Post runs fn on the graph's goroutine later. It must be safe to
	// call from any goroutine and must not block.
	Post func(fn func())
	// Clock schedules timers; the system's when nil.
	Clock Clock
	// Report receives failures no Catch handles; they are dropped when nil.
	Report func(error)
	// RequestFrame tells the host that the app waits for the next display frame: it calls Scope.RunFrame, or Scope.RunFrameAt with the display's time for the frame, when that frame begins, and is not asked again until then. It runs on the graph's goroutine, so it must not block. A host without display frames leaves it nil, and the callbacks wait.
	RequestFrame func()
	// RequestSurfaceFrame is RequestFrame for a host with independent displays. It calls Tree.RunFrameAt for the surface named by id.
	RequestSurfaceFrame func(id uint32)
	// SetFrameRate updates the host's preferred display rate for the surface, if the host can change it. It must not block.
	SetFrameRate func(id uint32, rate FrameRateRange)
	// Storage keeps the app's entries; a store in memory when nil.
	Storage *Store
	// Secrets is the platform's secure store, which Scope.Secrets keeps secrets in; nil where there is none, so Secrets fails with ErrSecretsUnavailable.
	Secrets SecretStore
	// NetworkWatcher supplies network state when the app first reads it.
	NetworkWatcher interface {
		WatchNetwork(changed func(Network)) (initial Network, stop func())
	}
	// ReadAsset reads a slash-separated resource path on a worker goroutine; nil where the host cannot read bundled files.
	ReadAsset func(context.Context, string) ([]byte, error)
	// Updater is the initial state of package updater, supplied by the platform.
	Updater UpdaterState
	// GameServices is a per-app game services backend (such as nsgotest's fake) for package gameservices; nil uses the platform backend.
	GameServices GameServices
	// DataDir, CacheDir and LogsDir are the app's directories, "" where the target has none.
	DataDir, CacheDir, LogsDir string
	// Host is the channel to the host app that embeds the app: its
	// parameters, events, handlers and URL handler. The graph attaches to
	// it until its root is disposed.
	Host *Host
	// Windows, set where the platform opens windows, are the main
	// window's options (SetMainView): the first tree is then the app's
	// own surface, which shows no view.
	Windows *WindowOptions
	// HotKeys, set where the platform registers global shortcuts but opens no windows, sends RegisterHotKey's shortcuts to the first tree, as a platform that opens windows has them sent to the app's own surface. The platform answers each with EventHotKeyState; elsewhere RegisterHotKey's handles are unsupported at once.
	HotKeys bool
	// Tray is set where the platform supports app-level tray icons without
	// opening additional windows.
	Tray bool
}

// NewRoot returns the root scope of a new graph.
func NewRoot(opts RootOptions) *Scope {
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	rt := &runtime{graphOwner: newGraphOwner(opts.CheckCommits), post: opts.Post, clock: opts.Clock, report: opts.Report, checkCommits: opts.CheckCommits,
		store: opts.Storage, secrets: opts.Secrets, readAsset: opts.ReadAsset, gameServices: opts.GameServices, dataDir: opts.DataDir, cacheDir: opts.CacheDir, logsDir: opts.LogsDir,
		requestFrame: opts.RequestFrame, requestSurfaceFrame: opts.RequestSurfaceFrame, setFrameRate: opts.SetFrameRate, windows: opts.Windows, takesHotKeys: opts.HotKeys, tray: opts.Tray, networkWatcher: opts.NetworkWatcher}
	rt.root = &Scope{rt: rt}
	if devtools.Enabled {
		rt.dev = newDevState()
	}
	rt.updater = NewSignal(rt.root, opts.Updater)
	if opts.Host != nil {
		rt.attachHost(opts.Host)
	}
	return rt.root
}

const maxRuns = 1_000_000

type runtime struct {
	graphOwner
	workObserved bool
	activeWork   *workObserver
	automation   automationToolRegistry
	aborted      bool // fault recovery must never schedule another computation
	checkCommits bool
	classCache   parseCache[classSource, *classList]
	templates    parseCache[GridTracks, *gridTemplate]
	reported     parseCache[string, bool]
	tokenSets    uint64 // the host token sets given so far, which number them

	owner    *Scope       // the owner of what running code creates
	observer *computation // the computation reads subscribe
	batch    int
	flushing bool
	render   []*computation
	user     []*computation
	// renderAt and userAt are the next in each queue flush runs.
	renderAt, userAt int
	runs             int
	post             func(func())
	clock            Clock
	report           func(error)

	// firstFrames serves the first tree and scopes with no tree. Every
	// other surface keeps its own queue and cadence.
	firstFrames         frameQueue
	frameRequested      bool   // a request to a host with one shared clock
	frameSeq            uint64 // display frames run so far, for package gamepad (FrameState)
	inFrame             bool   // set while a frame's callbacks run, for package gamepad
	requestFrame        func()
	requestSurfaceFrame func(uint32)
	setFrameRate        func(uint32, FrameRateRange)
	env                 *Env
	tree                *Tree
	root                *Scope
	store               *Store
	storage             *storageWatch
	secrets             driver.SecretStore
	readAsset           func(context.Context, string) ([]byte, error)
	gameServices        driver.GameServices
	dataDir             string
	cacheDir            string
	logsDir             string
	host                *hostApp
	networkWatcher      driver.NetworkWatcher
	updater             *Signal[UpdaterState]
	// restricted says an owner was given an Access (ProvideAccess), so
	// requests look for one.
	restricted bool
	// storageScoped says an owner was given a partition (ProvideStorage), and secretsScoped one of the secrets (ProvideSecrets).
	storageScoped bool
	secretsScoped bool
	// hostScoped says an owner was given a host (ProvideHost).
	hostScoped bool
	// parked counts the parked scopes, so a memo looks for one above it only while there are any.
	parked int
	// refusals are the services a refusal was logged of.
	refusals map[service]bool
	// speech is the speech session running, one per engine.
	speech *SpeechSession
	// speaking is the utterance being spoken, one per engine.
	speaking *utterance
	// audio is the engine's sounds and players, made with the first.
	audio *audioState
	// causes are what ran since a tree last committed, while driver.TraceCauses is set.
	causes causes
	// dev is what DevTools read, while devtools.Enabled.
	dev *devState
	// guide is the permission guide showing, one per engine.
	guide *permissionGuide
	// quits are the OnQuit handlers, oldest first.
	quits []*quitHandler
	// quitRequests are the handlers for user-initiated quit requests.
	quitRequests []*quitRequestHandler
	quitPending  bool
	// notificationPresses hear notifications pressed, and pressesPending
	// keeps those pressed before any did.
	notificationPresses []*notificationPressHandler
	pressesPending      []string
	// purchaseUpdates hear transactions the store updates, and updatesPending keeps those updated before any did.
	purchaseUpdates []*purchaseUpdateHandler
	updatesPending  []Transaction
	// taskProgress is the task progress sent last, by the tree whose ID is taskProgressFrom, once taskProgressSent. Only a repeat from that same tree sends nothing: the Dock tile and a Linux launcher entry show one progress for the whole app, which another tree may have changed since. It follows the calls' order, not the trees' commits, as those platforms keep the latest call's progress (driver.TaskProgress.Call).
	taskProgress     driver.TaskProgress
	taskProgressFrom uint32
	taskProgressSent bool
	// updateActive says an in-app update downloads or installs (UpdateActive).
	updateActive atomic.Bool
	// windows are the main window's options where the platform opens
	// windows (RootOptions.Windows); surfaces the trees of the windows,
	// panels and tray views opened, keyed those opened with a key.
	windows   *WindowOptions
	tray      bool
	surfaces  []*Tree
	surfaceID uint32
	keyed     map[string]*Window
	// globalKeys are the bindings made with KeyOptions.AllSurfaces, which every tree opened takes.
	globalKeys []*globalKey
	// menuBar is the tree that set the application menu last, whose menu macOS's menu bar shows (menuHasKey).
	menuBar   *Tree
	menuOrder uint64
	// overlay is the overlay over other apps opened last (OpenOverlay).
	overlay *Window
	// shutdown says when the app ends; main is its main window, ownMain
	// the one SetMainView opened to show mainView.
	shutdown      ShutdownMode
	main, ownMain *Window
	mainView      *Signal[func(cx *Scope) Node]
	// rootView is what the first tree mounted, which a platform's further scenes show when the app set no main view (openScene).
	rootView func(cx *Scope) Node
	reopens  []*reopenHandler
	// opens hear the files the system opens with the app, and links the app's own links (desktop.OnOpenLinks).
	opens, links openQueue[string]
	address      *appAddress
	shares       openQueue[[]driver.SharedItem]
	// authLinks claim the private redirect URIs whose callbacks arrive as open links, including sessions started in an opened surface.
	authLinks map[string]*authSession
	// dock is the app icon's menu, the one a surface set last.
	dock     dockMenu
	trays    map[uint32]*Tray
	trayID   uint32
	hotKeys  []*hotKey
	hotKeyID uint32
	// takesHotKeys is RootOptions.HotKeys.
	takesHotKeys bool
	// theme is DefaultTheme's, made on first use; subtreeAppearance
	// whether any scope has forced one.
	theme             *Theme
	subtreeAppearance bool
	// confined is set once a scope is confined (Scope.Confine), as apps without one skip finding its frames and its owners (confinement).
	confined bool
}

// tree is the tree of the surface cx is in: its window's, or the
// graph's first.
func (cx *Scope) tree() *Tree {
	if cx.surf != nil {
		return cx.surf
	}
	return cx.rt.tree
}

// current is the scope that owns what code using cx creates now.
func (cx *Scope) current() *Scope {
	if o := cx.rt.owner; o != nil {
		return o
	}
	return cx
}

func (cx *Scope) adopt(child *Scope) {
	child.rt = cx.rt
	child.parent = cx
	if child.surf == nil {
		child.surf = cx.surf
	}
	if cx.disposed {
		child.disposed = true
		return
	}
	child.prev = cx.last
	if cx.last != nil {
		cx.last.next = child
	} else {
		cx.first = child
	}
	cx.last = child
}

// Child returns a new scope owned by the current one.
func (cx *Scope) Child() *Scope {
	s := &Scope{}
	cx.current().adopt(s)
	return s
}

// Dispose disposes everything the scope owns, runs its cleanups and
// cancels its Context.
func (cx *Scope) Dispose() {
	cx.rt.check()
	if cx.disposed {
		return
	}
	rt := cx.rt
	rt.batch++
	defer func() {
		rt.batch--
		if rt.batch == 0 && !rt.flushing {
			rt.flush()
		}
	}()
	cx.disposeOwned()
	cx.disposed = true
	if rt.dev != nil {
		rt.dev.forget(cx)
	}
	if cx == rt.root {
		rt.detachHost()
	}
	if cx.comp != nil {
		cx.comp.unlink()
	}
	if p := cx.parent; p != nil {
		if cx.prev != nil {
			cx.prev.next = cx.next
		} else if p.first == cx {
			p.first = cx.next
		}
		if cx.next != nil {
			cx.next.prev = cx.prev
		} else if p.last == cx {
			p.last = cx.prev
		}
		cx.prev, cx.next = nil, nil
	}
}

// Abort implements internal/engine's fault shutdown: stop scheduling work, then dispose the graph and cancel its resources. Only a root may abort. Cleanups still run, but cannot restart effects on a partially committed tree.
func (cx *Scope) Abort() {
	cx.rt.check()
	if cx != cx.rt.root {
		panic("core: only the root scope can abort a graph")
	}
	cx.rt.aborted = true
	// Detach before cleanups, even if a damaged graph cannot finish disposal.
	cx.rt.detachHost()
	cx.Dispose()
}

// Disposed reports whether the scope has been disposed.
func (cx *Scope) Disposed() bool { return cx.disposed }

// HasTree reports whether the scope belongs to a view tree.
func (cx *Scope) HasTree() bool { return cx.tree() != nil }

func (cx *Scope) disposeOwned() {
	for c := cx.last; c != nil; {
		prev := c.prev
		c.prev, c.next = nil, nil
		c.parent = nil
		c.disposeDetached()
		c = prev
	}
	cx.first, cx.last = nil, nil
	for i := len(cx.cleanups) - 1; i >= 0; i-- {
		cx.rt.guard(cx, cx.cleanups[i])
	}
	cx.cleanups = nil
	// After the cleanups, which may arm timers of their own.
	cx.stopTimers()
	cx.values = nil
	if cx.cancel != nil {
		cx.cancel()
		cx.ctx, cx.cancel = nil, nil
	}
}

func (cx *Scope) disposeDetached() {
	cx.disposeOwned()
	cx.disposed = true
	if cx.rt != nil && cx.rt.dev != nil {
		cx.rt.dev.forget(cx)
	}
	if cx.comp != nil {
		cx.comp.unlink()
	}
}

// Cleanup registers fn to run when the current owner is disposed or runs again, last registered first, as testing.T.Cleanup does. Readers of what fn writes run once the disposal, or the owner's new run, is over, never against a half-disposed tree.
func (cx *Scope) Cleanup(fn func()) { cx.current().addCleanup(fn) }

func (o *Scope) addCleanup(fn func()) {
	if o.disposed {
		o.rt.guard(o, fn)
		return
	}
	o.cleanups = append(o.cleanups, fn)
}

// Context returns a context cancelled when the current owner is disposed
// or runs again; pass it to work the owner starts on other goroutines.
func (cx *Scope) Context() context.Context { return cx.current().context() }

func (o *Scope) context() context.Context {
	if o.ctx == nil {
		parent := context.Background()
		if o.parent != nil {
			parent = o.parent.context()
		}
		o.ctx, o.cancel = context.WithCancel(parent)
		if o.disposed {
			o.cancel()
		}
	}
	return o.ctx
}

// Post runs fn later on the graph's goroutine, under cx and in one
// batch. It is safe to call from any goroutine; fn does not run if cx is
// disposed first.
func (cx *Scope) Post(fn func()) {
	o, rt := cx, cx.rt
	if rt.post == nil {
		panic("core: Post needs a host: start the scope's graph with an engine or a test driver")
	}
	rt.post(func() {
		if !o.disposed {
			rt.run(o, fn)
		}
	})
}

// afterTimer is a timer AfterFunc armed. Its owner keeps it in a set from the moment it is armed until its call has run or it is stopped, so disposing the owner, or the owner's computation running again, stops it even after the clock has fired and its call waits in the post queue. stop may be called on any goroutine, so the set is under the owner's timerMu.
type afterTimer struct {
	owner     *Scope
	fn        func()
	clockStop func() bool
	// done is set once fn has run or the timer was stopped, whichever came first.
	done atomic.Bool
}

// fire runs on the clock's goroutine and hands the call to the graph's.
func (t *afterTimer) fire() { t.owner.Post(t.run) }

// run is on the graph's goroutine; a timer stopped since the clock fired does not call fn.
func (t *afterTimer) run() {
	t.owner.removeTimer(t)
	fn := t.fn
	t.fn = nil
	if t.done.CompareAndSwap(false, true) {
		fn()
	}
}

// stop reports whether it stopped the timer before fn ran.
func (t *afterTimer) stop() bool {
	if t.done.Swap(true) {
		return false
	}
	t.owner.removeTimer(t)
	if t.clockStop != nil {
		t.clockStop()
	}
	return true
}

func (o *Scope) addTimer(t *afterTimer) {
	o.timerMu.Lock()
	if o.timers == nil {
		o.timers = make(map[*afterTimer]struct{})
	}
	o.timers[t] = struct{}{}
	o.timerMu.Unlock()
}

func (o *Scope) removeTimer(t *afterTimer) {
	o.timerMu.Lock()
	delete(o.timers, t)
	o.timerMu.Unlock()
}

// stopTimers stops the timers the owner has pending, those whose call waits in the post queue included.
func (o *Scope) stopTimers() {
	o.timerMu.Lock()
	timers := make([]*afterTimer, 0, len(o.timers))
	for t := range o.timers {
		timers = append(timers, t)
	}
	o.timers = nil
	o.timerMu.Unlock()
	for _, t := range timers {
		t.stop()
	}
}

// AfterFunc runs fn on the graph's goroutine after d, unless the current owner is disposed first or stop is called. stop reports whether it stopped the timer before fn ran, and may be called on any goroutine. A timer that has fired or been stopped keeps nothing on its owner, so re-arming one under a long-lived scope, as a debounce or a repeating tick does, does not grow it.
func (cx *Scope) AfterFunc(d time.Duration, fn func()) (stop func() bool) {
	o := cx.current()
	t := &afterTimer{owner: o, fn: fn}
	if o.disposed {
		// As a cleanup registered on a disposed owner runs at once, the timer is stopped before it starts.
		t.done.Store(true)
		t.fn = nil
		return t.stop
	}
	// The clock fires on its own goroutine and the call is posted, so the timer is in the set and clockStop is set before run or stop can look for them.
	o.addTimer(t)
	t.clockStop = cx.rt.clock.AfterFunc(d, t.fire)
	return t.stop
}

// Untrack runs fn without subscribing the running computation to what
// it reads.
func (cx *Scope) Untrack(fn func()) {
	cx.rt.check()
	rt := cx.rt
	prev := rt.observer
	rt.observer = nil
	defer func() { rt.observer = prev }()
	fn()
}

// PauseTracking stops subscribing the running computation to what is
// read until resume is called: for code that runs within a computation
// without belonging to it, as a promise's continuation the tsx package's
// VM runs as a JS call returns.
func (cx *Scope) PauseTracking() (resume func()) {
	cx.rt.check()
	rt := cx.rt
	prev := rt.observer
	rt.observer = nil
	return func() { rt.observer = prev }
}

// Untracked returns fn's result without subscribing the running
// computation to what it reads.
func Untracked[T any](cx *Scope, fn func() T) (v T) {
	cx.Untrack(func() { v = fn() })
	return v
}

// Batch runs fn and then the effects its writes scheduled, once each.
func (cx *Scope) Batch(fn func()) {
	cx.rt.check()
	rt := cx.rt
	rt.batch++
	defer func() {
		rt.batch--
		if rt.batch == 0 && !rt.flushing {
			rt.flush()
		}
	}()
	fn()
}

// Effect runs fn after the current batch and again whenever what it read
// changes, until the current owner is disposed or stop is called.
func (cx *Scope) Effect(fn func()) (stop func()) {
	c := newComputation(cx.current(), userEffect, nil)
	c.effect = fn
	c.state = dirty
	cx.rt.enqueue(c)
	cx.rt.maybeFlush()
	return c.Dispose
}

// renderEffect runs fn now and again before the app's effects whenever
// what it read changes. Element bindings are render effects.
func (cx *Scope) renderEffect(fn func()) *computation {
	c := newComputation(cx.current(), renderEffect, nil)
	c.effect = fn
	c.run()
	return c
}

// run runs fn under owner o, untracked, in a batch, catching panics.
func (rt *runtime) run(o *Scope, fn func()) {
	rt.check()
	if rt.aborted {
		return
	}
	prevOwner, prevObs := rt.owner, rt.observer
	rt.owner, rt.observer = o, nil
	defer func() { rt.owner, rt.observer = prevOwner, prevObs }()
	rt.batch++
	end := rt.beginWork(o)
	if end != nil {
		defer func() {
			if end != nil {
				end()
			}
		}()
	}
	rt.guard(o, fn)
	if end != nil {
		end()
		end = nil
	}
	rt.batch--
	if rt.batch == 0 && !rt.flushing {
		rt.flush()
	}
}

// guard runs fn and sends a panic to the nearest Catch above o.
func (rt *runtime) guard(o *Scope, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			rt.fail(o, r)
		}
	}()
	fn()
}

// PanicError is a panic recovered from app code or an engine's fault boundary.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v\n\n%s", e.Value, e.Stack) }

// Unwrap returns the panic's value when it is an error.
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

func (rt *runtime) fail(o *Scope, r any) {
	rt.raise(o, &PanicError{Value: r, Stack: debug.Stack()})
}

// raise sends err to the nearest Catch above o, or to the host's Report without one.
func (rt *runtime) raise(o *Scope, err error) {
	for s := o; s != nil; s = s.parent {
		if s.catch != nil && !s.disposed {
			s.catch(err)
			return
		}
	}
	if rt.report != nil {
		rt.report(err)
	}
}

// Key is a context key: a value an owner provides to everything below it.
// Declare one as a package-level variable.
type Key[T any] struct {
	name string
	def  T
}

// NewKey returns a key whose Use returns def where nothing provides one.
func NewKey[T any](name string, def T) *Key[T] { return &Key[T]{name: name, def: def} }

func (k *Key[T]) String() string { return k.name }

// Provide makes v the key's value for the current owner and below.
func (k *Key[T]) Provide(cx *Scope, v T) {
	o := cx.current()
	if o.values == nil {
		o.values = map[any]any{}
	}
	o.values[k] = v
}

// Use returns the value the nearest owner provides, or the default.
func (k *Key[T]) Use(cx *Scope) T {
	for s := cx.current(); s != nil; s = s.parent {
		if v, ok := s.values[k]; ok {
			return v.(T)
		}
	}
	return k.def
}

// Computations.

type compKind uint8

const (
	memoKind compKind = iota
	renderEffect
	userEffect
)

type compState uint8

const (
	clean compState = iota
	check           // a memo it reads may have changed
	dirty           // something it reads changed
)

type source interface {
	readers() *subscribers
	refresh() // brings a memo up to date
}

type computation struct {
	Scope
	fn     func() (changed bool)
	effect func() // an effect's, which changes nothing it is read for, in place of fn
	kind   compKind
	state  compState
	queued bool
	// sources are what c read, each with c's index in the source's
	// readers, so unlinking takes constant time however many read it.
	sources []sourceLink
	subs    *subscribers // a memo's readers
}

func newComputation(owner *Scope, kind compKind, fn func() bool) *computation {
	owner.rt.check()
	c := &computation{fn: fn, kind: kind}
	c.comp = c
	owner.adopt(&c.Scope)
	if c.rt.dev != nil {
		c.rt.dev.computation(c)
	}
	return c
}

func (c *computation) unlink() {
	for _, s := range c.sources {
		s.src.readers().remove(s.at)
	}
	clear(c.sources)
	c.sources = c.sources[:0]
}

func (c *computation) mark(s compState) {
	if c.state >= s || c.disposed {
		return
	}
	wasClean := c.state == clean
	c.state = s
	if !wasClean {
		return
	}
	if c.subs != nil {
		for _, r := range c.subs.list {
			r.c.mark(check)
		}
		return
	}
	c.rt.enqueue(c)
}

// update recomputes c if a source really changed.
func (c *computation) update() {
	if c.disposed {
		return
	}
	if c.state == check {
		for _, s := range c.sources {
			s.src.refresh()
			if c.state == dirty {
				break
			}
		}
	}
	if c.state == dirty {
		// A memo in a spare cell, read from outside it, keeps its value until the cell shows a row again, and runs then for its readers.
		if c.subs != nil && c.rt.parked > 0 {
			if p := c.parent.heldBack(); p != nil {
				if !slices.Contains(p.deferred, c) {
					p.deferred = append(p.deferred, c)
				}
				return
			}
		}
		c.run()
		return
	}
	c.state = clean
}

// heldBack returns the parking of s or the nearest owner above it that is parked, or nil.
func (s *Scope) heldBack() *parking {
	for ; s != nil; s = s.parent {
		if s.park != nil && s.park.parked {
			return s.park
		}
	}
	return nil
}

// run recomputes c. Writes during the run are batched, and a write to
// something c already read makes it run again.
func (c *computation) run() {
	c.rt.check()
	if c.rt.aborted {
		return
	}
	if driver.TraceCauses {
		c.rt.noteRun(c.kind)
	}
	var began time.Time
	if c.rt.dev != nil {
		began = time.Now()
	}
	rt := c.rt
	// The batch covers the disposal too, so a cleanup's write runs nothing before c has run again.
	rt.batch++
	end := c.Scope.BeginWork()
	if end != nil {
		defer func() {
			if end != nil {
				end()
			}
		}()
	}
	c.disposeOwned()
	c.unlink()
	c.state = clean
	prevOwner, prevObs := rt.owner, rt.observer
	rt.owner, rt.observer = &c.Scope, c
	changed := false
	rt.guard(&c.Scope, func() {
		if c.effect != nil {
			c.effect()
		} else {
			changed = c.fn()
		}
	})
	if end != nil {
		end()
		end = nil
	}
	// Its own time: what its writes run next, in maybeFlush, is theirs.
	if rt.dev != nil {
		rt.dev.ran(c, time.Since(began))
	}
	rt.batch--
	rt.owner, rt.observer = prevOwner, prevObs
	if changed && c.subs != nil {
		for _, r := range c.subs.list {
			r.c.mark(dirty)
		}
	}
	rt.maybeFlush()
}

func (rt *runtime) enqueue(c *computation) {
	if rt.aborted {
		return
	}
	if c.queued {
		return
	}
	c.queued = true
	if c.kind == userEffect {
		rt.user = append(rt.user, c)
	} else {
		rt.render = append(rt.render, c)
	}
}

func (rt *runtime) maybeFlush() {
	if rt.batch == 0 && !rt.flushing {
		rt.flush()
	}
}

func (rt *runtime) flush() {
	if rt.aborted {
		return
	}
	rt.flushing = true
	defer func() { rt.flushing = false }()
	for {
		var c *computation
		// Taken by index, so the queues keep what they grew to.
		switch {
		case rt.renderAt < len(rt.render):
			c = rt.render[rt.renderAt]
			rt.render[rt.renderAt] = nil
			rt.renderAt++
		case rt.userAt < len(rt.user):
			c = rt.user[rt.userAt]
			rt.user[rt.userAt] = nil
			rt.userAt++
		default:
			rt.render, rt.user, rt.renderAt, rt.userAt = rt.render[:0], rt.user[:0], 0, 0
			rt.runs = 0
			return
		}
		c.queued = false
		if rt.runs++; rt.runs > maxRuns {
			rt.render, rt.user, rt.renderAt, rt.userAt = nil, nil, 0, 0
			rt.runs = 0
			rt.fail(&c.Scope, fmt.Errorf("core: effects ran %d times without settling", maxRuns))
			return
		}
		c.runTop()
	}
}

// runTop brings c's stale owners up to date first, outermost first, so a
// branch about to be disposed does not run.
func (c *computation) runTop() {
	slow := false
	for s := c.parent; s != nil && !slow; s = s.parent {
		slow = s.comp != nil && s.comp.state != clean || s.park != nil && (s.park.parked || s.park.guard != nil && s.park.guard.state != clean)
	}
	if !slow {
		c.update()
		return
	}
	// Outermost first: stale owners run before what they own, and a list's guard before its cells. A computation in a cell the guard leaves spare waits with the cell.
	var path []*Scope
	for s := c.parent; s != nil; s = s.parent {
		if s.comp != nil && s.comp.state != clean || s.park != nil {
			path = append(path, s)
		}
	}
	for i := len(path) - 1; i >= 0; i-- {
		s := path[i]
		if p := s.park; p != nil {
			if p.guard != nil && p.guard.state != clean {
				p.guard.update()
			}
			if p.parked {
				if !c.disposed && c.state != clean {
					p.deferred = append(p.deferred, c)
				}
				return
			}
		}
		if s.comp != nil && s.comp.state != clean {
			s.comp.update()
		}
	}
	c.update()
}

// subscribers are a source's readers, each with the source's index in
// the reader's sources.
type subscribers struct {
	list []readerLink
}

// sourceLink is a source a computation read and its index in the
// source's readers; readerLink a computation reading a source and the
// source's index in the computation's sources. One slice of pairs a side
// keeps the graph to half the objects of a slice of each.
type (
	sourceLink struct {
		src source
		at  int
	}
	readerLink struct {
		c  *computation
		at int
	}
)

// remove takes out the reader at i, moving the last one into its place.
func (s *subscribers) remove(i int) {
	last := len(s.list) - 1
	if i != last {
		moved := s.list[last]
		s.list[i] = moved
		moved.c.sources[moved.at].at = i
	}
	s.list[last] = readerLink{}
	s.list = s.list[:last]
}

func (rt *runtime) track(src source, subs *subscribers) {
	c := rt.observer
	// A disposed computation never runs again, so nothing would take out a link it made (#997).
	if c == nil || c.disposed {
		return
	}
	for _, s := range c.sources {
		if s.src == src {
			return
		}
	}
	c.sources = append(c.sources, sourceLink{src, len(subs.list)})
	subs.list = append(subs.list, readerLink{c, len(c.sources) - 1})
}

// Signal is a value computations subscribe to by reading it.
type Signal[T any] struct {
	rt    *runtime
	value T
	equal func(a, b T) bool
	subs  subscribers
}

// NewSignal returns a signal holding v. Setting an equal value does
// nothing.
func NewSignal[T comparable](cx *Scope, v T) *Signal[T] {
	cx.rt.check()
	s := &Signal[T]{rt: cx.rt, value: v, equal: func(a, b T) bool { return a == b }}
	if cx.rt.dev != nil {
		devSignal(cx.devOwner(), s)
	}
	return s
}

// NewSignalFunc returns a signal holding v that compares values with
// equal; with a nil equal every Set notifies, as slices and maps need.
func NewSignalFunc[T any](cx *Scope, v T, equal func(a, b T) bool) *Signal[T] {
	cx.rt.check()
	s := &Signal[T]{rt: cx.rt, value: v, equal: equal}
	if cx.rt.dev != nil {
		devSignal(cx.devOwner(), s)
	}
	return s
}

// Get returns the value and subscribes the running computation to it.
func (s *Signal[T]) Get() T {
	s.rt.track(s, &s.subs)
	return s.value
}

// GetUntracked returns the value without subscribing.
func (s *Signal[T]) GetUntracked() T { return s.value }

// Set stores v and schedules the computations that read the signal.
func (s *Signal[T]) Set(v T) {
	s.rt.check()
	if s.equal != nil && s.equal(s.value, v) {
		return
	}
	s.value = v
	rt := s.rt
	if driver.TraceCauses {
		rt.noteWrite()
	}
	rt.batch++
	for _, r := range s.subs.list {
		r.c.mark(dirty)
	}
	rt.batch--
	rt.maybeFlush()
}

// Update sets the value fn returns for the current one.
func (s *Signal[T]) Update(fn func(T) T) { s.rt.check(); s.Set(fn(s.value)) }

func (s *Signal[T]) readers() *subscribers { return &s.subs }
func (s *Signal[T]) refresh()              {}

// Memo caches a computed value and notifies its readers only when the
// value changes.
type Memo[T any] struct {
	c     *computation
	value T
	equal func(a, b T) bool
}

// NewMemo returns a memo of fn, computed now and again when what fn read
// changes.
func NewMemo[T comparable](cx *Scope, fn func() T) *Memo[T] {
	return newMemo(cx, fn, func(a, b T) bool { return a == b })
}

// NewMemoFunc returns a memo that compares values with equal; with a nil
// equal every computation notifies.
func NewMemoFunc[T any](cx *Scope, fn func() T, equal func(a, b T) bool) *Memo[T] {
	return newMemo(cx, fn, equal)
}

func newMemo[T any](cx *Scope, fn func() T, equal func(a, b T) bool) *Memo[T] {
	m := &Memo[T]{equal: equal}
	first := true
	m.c = newComputation(cx.current(), memoKind, func() bool {
		v := fn()
		if !first && m.equal != nil && m.equal(m.value, v) {
			return false
		}
		first = false
		m.value = v
		return true
	})
	m.c.subs = &subscribers{}
	if d := m.c.rt.dev; d != nil {
		d.memo(m.c, func() string { return fmt.Sprint(any(m.value)) })
	}
	m.c.run()
	return m
}

// Get returns the value, recomputing it first if needed, and subscribes
// the running computation to it.
func (m *Memo[T]) Get() T {
	m.refresh()
	m.c.rt.track(m, m.c.subs)
	return m.value
}

// GetUntracked returns the up-to-date value without subscribing.
func (m *Memo[T]) GetUntracked() T {
	m.refresh()
	return m.value
}

func (m *Memo[T]) readers() *subscribers { return m.c.subs }
func (m *Memo[T]) refresh() {
	if m.c.state != clean {
		m.c.update()
	}
}
