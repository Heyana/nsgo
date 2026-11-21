// Package tsx runs TSX apps: bundles of what tsx/bundler emits, on the
// goja JavaScript engine, or the page's own on the web (internal/js),
// rendered by core as Go apps are. It is a port
// of NSH's nsh-js: reactive.go of reactive.rs, dom.go of dom.rs,
// protocol.go of protocol.rs and modules.go of modules.rs, so a change
// there ports here (docs/architecture/tsx.md).
//
// A bundle is a script: imports of NSGo's modules (nsgo, nsgo/base,
// nsgo:rt) are calls of require.
package tsx

import (
	"fmt"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
)

// App returns the app of a bundle, for nsgo.Run and nsgotest.Start: it
// runs the bundle, and what the bundle mounts is the app. name names the
// bundle in stacks.
func App(name, src string) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		r := newRuntime(cx)
		if _, err := r.vm.RunScript(name, src); err != nil {
			panic(err)
		}
		return r.root.elem
	}
}

// SplitApp returns the app of a split bundle (bundler.CompileSplit), as
// App does: an import() of one of its chunks calls chunk, on a goroutine
// of its own, for the chunk's script by its key, and runs it.
func SplitApp(name, src string, chunk func(key string) (string, error)) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		r := newRuntime(cx)
		r.chunk = chunk
		if _, err := r.vm.RunScript(name, src); err != nil {
			panic(err)
		}
		return r.root.elem
	}
}

// Hot returns the app of a hot bundle (bundler.CompileHot), as App
// does, and what runs a newer one in the same VM: the modules whose code
// changed run again and their components' instances render again, while
// the rest of the app keeps its state. update runs on the engine
// goroutine, as from Scope.Post.
func Hot(name, src string) (app func(cx *core.Scope) core.Node, update func(src string)) {
	var r *runtime
	app = func(cx *core.Scope) core.Node {
		r = newRuntime(cx)
		if _, err := r.vm.RunScript(name, src); err != nil {
			panic(err)
		}
		return r.root.elem
	}
	update = func(src string) {
		if r == nil {
			return
		}
		r.cx.Run(func() {
			if err, _ := r.within(r.limits.Load, func() error {
				_, err := r.vm.RunScript(name, src)
				return err
			}); err != nil {
				panic(err)
			}
		})
	}
	return app, update
}

// runtime is one VM and what NSH keeps per context: the module table,
// the graph's scope and the renderer's root.
type runtime struct {
	vm      *js.Runtime
	cx      *core.Scope
	root    *node
	modules map[string]*js.Object
	// warned holds the warnings given, each given once.
	warned map[string]bool
	timers timers
	anim   animClasses
	// assets are the data: URLs of the asset: URLs a bundle ships.
	assets map[string]string
	// blobs are the blob: URLs URL.createObjectURL made, until revoked.
	blobs   map[string]blobEntry
	blobSeq int
	// canvasClasses are Path2D and CanvasGradient.
	canvasClasses canvasClasses
	// platforms are the platform objects (ios={{…}}) that apply here.
	platforms []string
	// web is what web.js gives back; base is the URL relative URLs
	// resolve against.
	web  *js.Object
	base string
	// limits are the sandbox's; depth counts the calls into JS under way,
	// the outermost of which has the time budget.
	limits Limits
	depth  int
	// stopMu guards sandbox.stopped; calls are the pending Plugin.Call callbacks on the engine's goroutine.
	stopMu sync.Mutex
	calls  map[*deadline]struct{}
	// onPluginStop belongs to the engine's goroutine.
	onPluginStop func(error)
	// interrupted says the host stopped the runtime (interrupt), and hears so through the calls that fail: the JS it cut ends without a panic into the app's Catch.
	interrupted atomic.Bool
	// deadlines enables CallTotal. current owns the JS work running now, and sliceDeadline is the earliest total deadline in the current slice.
	deadlines     bool
	current       *deadline
	slice         *time.Timer
	sliceEnd      time.Time
	sliceDeadline *deadline
	continuations continuations
	// regexTimeout is the shortest sandbox budget, applied to regexp2
	// patterns because its matcher does not observe goja's interrupt flag.
	regexTimeout time.Duration
	// sandbox is a sandboxed runtime's state (Sandbox), nil for the
	// app's own code.
	sandbox *sandboxed
	// origin is when performance.now() and frames' times count from.
	origin time.Time
	// classes are the Go classes made in this VM; posting counts the
	// async calls converting their arguments.
	classes vmClasses
	posting int
	// chunk loads a split bundle's chunks (SplitApp).
	chunk func(key string) (string, error)
	// committing is set while a commit is asked for after the page's
	// engine called Go (engine_browser.go).
	committing bool
	// exportView says a view renders one of the entry module's exports, so mount attaches nothing and runs nothing of what it is given.
	exportView bool
	// retainPromiseJobs keeps a native plugin's unstarted work across a budget cut.
	retainPromiseJobs bool
}

// maxCallStack is the native runtime's default and maximum call depth.
const maxCallStack = 10000

// continuations restores the owner of a promise reaction and runs it untracked, even if a different call settled the promise.
type continuations struct {
	r      *runtime
	resume []continuation
}

type continuation struct {
	tracking func()
	owner    *deadline
}

func (c *continuations) Grab() any { return c.r.current }

func (c *continuations) Resumed(ctx any) {
	r := c.r
	c.resume = append(c.resume, continuation{tracking: r.cx.PauseTracking(), owner: r.current})
	if dl, ok := ctx.(*deadline); ok && dl != nil {
		r.current = dl
		if r.depth > 0 {
			r.adopt(dl)
		}
	}
}

func (c *continuations) Exited() {
	n := len(c.resume) - 1
	prev := c.resume[n]
	c.resume[n] = continuation{}
	c.resume = c.resume[:n]
	c.r.current = prev.owner
	prev.tracking()
}

func newRuntime(cx *core.Scope) *runtime {
	started.Store(true)
	r := &runtime{vm: js.New(), cx: cx, modules: map[string]*js.Object{}, warned: map[string]bool{}, origin: cx.Now()}
	r.root = &node{r: r, kind: kindRegion, elem: core.NewElement(cx, nil)}
	r.vm.SetFieldNameMapper(js.UncapFieldNameMapper())
	// Bound recursion in the VM to protect the goroutine's stack.
	r.vm.SetMaxCallStackSize(maxCallStack)
	// Date reads the app's clock, a test's manual one included.
	r.vm.SetTimeSource(cx.WallTime)
	// Promise continuations run as the outermost JS call returns, which
	// may be an effect's: what they read is not the effect's, as in NSH,
	// whose VM runs them after the effect.
	r.continuations.r = r
	r.vm.SetAsyncContextTracker(&r.continuations)
	r.attachEngine()
	r.vm.Set("require", r.require)
	r.installGlobals()
	r.installTimers()
	r.installAnimated()
	r.installCanvas()
	r.base = "asset:/"
	r.installWeb()
	if devtools.Enabled {
		scripts.add(r)
		cx.Cleanup(func() { scripts.remove(r) })
	}
	return r
}

// scripts are the runtimes DevTools ask which TSX line runs now (devtools.ScriptSite).
var scripts devScripts

type devScripts struct {
	mu   sync.Mutex
	runs []*runtime
}

func (d *devScripts) add(r *runtime) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runs = append(d.runs, r)
	devtools.ScriptSite = d.site
}

func (d *devScripts) remove(r *runtime) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runs = slices.DeleteFunc(d.runs, func(x *runtime) bool { return x == r })
}

// site is the line the runtime running JS now is at; a runtime that runs none has no stack.
func (d *devScripts) site() (string, int, bool) {
	d.mu.Lock()
	runs := slices.Clone(d.runs)
	d.mu.Unlock()
	for _, r := range runs {
		if file, line, ok := js.Caller(r.vm); ok {
			return file, line, true
		}
	}
	return "", 0, false
}

// require returns a module's exports, made on first use.
func (r *runtime) require(name string) js.Value {
	if m, ok := r.modules[name]; ok {
		return m
	}
	builtin, ok := moduleTable[name]
	more, has := registeredExports(name)
	if !ok && !has {
		panic(r.vm.NewTypeError("nsgo: no module %q", name))
	}
	if r.sandbox != nil && !r.sandbox.importsModule(name) {
		panic(r.vm.NewTypeError("nsgo: module %q is not given to this sandbox (Sandbox.Modules)", name))
	}
	m := r.vm.NewObject()
	for _, e := range builtin {
		if !r.withholds(name, e.name) {
			_ = m.Set(e.name, e.make(r))
		}
	}
	for _, e := range more {
		if r.imports(name, e.name) {
			_ = m.Set(e.name, e.make(r))
		}
	}
	r.addStatics(name, m)
	r.modules[name] = m
	return m
}

// warn logs a warning once.
func (r *runtime) warn(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if !r.warned[s] {
		r.warned[s] = true
		log.Print("nsgo: " + s)
	}
}

// call calls fn, panicking with what it throws: back into JS when JS
// called the Go code calling it, else to core, which reports it or
// shows the nearest boundary.
func (r *runtime) call(fn js.Value, args ...js.Value) js.Value {
	f, ok := js.AssertFunction(fn)
	if !ok {
		panic(r.vm.NewTypeError("nsgo: %s is not a function", fn))
	}
	v, err, cut := r.invoke(f, js.Undefined(), args...)
	if err != nil {
		if r.interrupted.Load() || cut {
			return js.Undefined()
		}
		panic(r.failure(err))
	}
	if v == nil {
		return js.Undefined()
	}
	return v
}

// invoke runs a function in a JS slice, reporting errors and whether an expired deadline already dealt with the error.
func (r *runtime) invoke(fn js.Callable, this js.Value, args ...js.Value) (v js.Value, err error, cut bool) {
	err, cut = r.within(r.limits.Call, func() error {
		v, err = fn(this, args...)
		return err
	})
	return
}

// function makes a JS function of fn.
func (r *runtime) function(fn func(call js.FunctionCall) js.Value) *js.Object {
	return r.vm.ToValue(fn).(*js.Object)
}

func isFunction(v js.Value) bool {
	_, ok := js.AssertFunction(v)
	return ok
}

// arg returns argument i, undefined when missing.
func arg(call js.FunctionCall, i int) js.Value { return call.Argument(i) }
