package tsx

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2/v2"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"github.com/nsgo-dev/nsgo/tsx/websrc"

	"github.com/nsgo-dev/nsgo/core"
)

// The sandbox (NSH's sandbox.rs and access.rs): what a bundle may use and
// reach, set by the Go code that runs it, never by JS. A bundle holds only
// the app's code, so nothing in JS is trusted to keep them. Core checks
// the capabilities where requests are made (core.Access), the runtime
// where only it makes them: fetch, the clipboard, the pickers, speech.

// Limits are what a bundle may use. Zero time budgets and quotas leave them unlimited.
type Limits struct {
	// Load limits the entry script and Call limits each slice of JS. On goja, a Plugin.Call whose slice is cut fails with ErrBudget even after it has awaited. CallTotal limits the wall time of a plugin call and the work it starts; at that deadline its pending work is dropped and a call still waiting fails with ErrBudget. The entry module's work is held to Load and standalone host events to CallTotal when CallTotal is set. Zero leaves a budget unlimited.
	Load, Call, CallTotal time.Duration
	// Stack is the call depth in frames on native targets. Values at or below zero or above 10,000 use 10,000, so a plugin's recursion throws a catchable RangeError rather than overflowing the goroutine's stack. DefaultLimits uses 1,000. The web uses the browser's stack limit.
	Stack int
	// Harden deletes eval, makes the Function constructors throw and freezes the constructors and prototypes of the realm's built-ins, including typed arrays and DataView.
	Harden bool
	// Nodes is how many elements the bundle may hold at once, each of
	// which may hold a native view; past it the bundle stops.
	Nodes int
	// Timers is how many timers and frame callbacks may wait at once, and
	// Requests how many fetches may be under way: one more fails.
	Timers, Requests int
	// Body is the most bytes a request's or a response's body may hold.
	Body int
	// Storage is the most bytes its partition of the app's storage may
	// hold (Sandbox.Storage).
	Storage int
	// Memory is the most bytes the built-ins that allocate by a size may hold for the bundle at once on goja, as ArrayBuffer, the typed arrays, String.prototype.repeat, Array.prototype.join and fill, and JSON.stringify do (memory_goja.go): each checks before it allocates, what it made is charged until it is collected, and past it, once the Go collector has run, the bundle stops. What the rest of the bundle's JS makes, its objects, closures and arrays and strings it builds, copies, concatenates or spreads, is not counted, nor anything on the web, whose page's engine keeps its own heap, so a bundle written to exhaust memory still can. Zero leaves it unlimited; DefaultLimits holds 64 MiB.
	Memory int
}

// DefaultLimits are NSH's, 10 seconds to load, 2 per call, 1000 frames, hardened, 200 000 nodes, 10 000 timers, 32 requests, 64 MiB bodies and 1 MiB of storage, and NSGo's 64 MiB of memory.
var DefaultLimits = Limits{Load: 10 * time.Second, Call: 2 * time.Second, Stack: 1000, Harden: true,
	Nodes: 200_000, Timers: 10_000, Requests: 32, Body: 64 << 20, Memory: 64 << 20, Storage: 1 << 20}

// ErrBudget is what a call cut at its time budget fails with.
var ErrBudget = errors.New("tsx: the script ran past its time budget")

// ErrInterrupted is used when a host interrupts a plugin without supplying
// its own error.
var ErrInterrupted = errors.New("tsx: the plugin was interrupted")

// regexpTimeouts are the match timeouts of the sandboxed calls running, whose shortest is regexp2's process-wide default while any runs, as a regular expression takes it when it compiles, and the default before them once none runs. The lock is held only while the default changes, never across a call: a sandboxed call can run another sandbox's on the same goroutine, as a host function calling a plugin does.
var regexpTimeouts struct {
	sync.Mutex
	running []time.Duration
	before  time.Duration
}

// limitRegexps makes d a match timeout of the regular expressions compiled until the returned func runs.
func limitRegexps(d time.Duration) (restore func()) {
	regexpTimeouts.Lock()
	defer regexpTimeouts.Unlock()
	if len(regexpTimeouts.running) == 0 {
		regexpTimeouts.before = regexp2.DefaultMatchTimeout
	}
	regexpTimeouts.running = append(regexpTimeouts.running, d)
	regexp2.DefaultMatchTimeout = slices.Min(regexpTimeouts.running)
	return func() {
		regexpTimeouts.Lock()
		defer regexpTimeouts.Unlock()
		i := slices.Index(regexpTimeouts.running, d)
		regexpTimeouts.running = slices.Delete(regexpTimeouts.running, i, i+1)
		if len(regexpTimeouts.running) == 0 {
			regexp2.DefaultMatchTimeout = regexpTimeouts.before
		} else {
			regexp2.DefaultMatchTimeout = slices.Min(regexpTimeouts.running)
		}
	}
}

// ParseLimits reads limits as nsgo dev -limits writes them: DefaultLimits with the fields spec names set, as "nodes=1000,call=500ms". Its keys are load, call, call-total, stack, nodes, timers, requests, body, storage and memory; "default" or "" is DefaultLimits.
func ParseLimits(spec string) (Limits, error) {
	l := DefaultLimits
	if spec == "" || spec == "default" {
		return l, nil
	}
	for _, kv := range strings.Split(spec, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			return l, fmt.Errorf("tsx: limit %q is not key=value", kv)
		}
		var err error
		switch k {
		case "load", "call", "call-total":
			var d time.Duration
			if d, err = time.ParseDuration(v); err == nil {
				switch k {
				case "load":
					l.Load = d
				case "call":
					l.Call = d
				default:
					l.CallTotal = d
				}
			}
		case "stack", "nodes", "timers", "requests", "body", "storage", "memory":
			var n int
			if n, err = strconv.Atoi(v); err == nil && n < 0 {
				err = errors.New("is negative")
			}
			*map[string]*int{"stack": &l.Stack, "nodes": &l.Nodes, "timers": &l.Timers, "requests": &l.Requests,
				"body": &l.Body, "storage": &l.Storage, "memory": &l.Memory}[k] = n
		default:
			return l, fmt.Errorf("tsx: no limit %q: load, call, call-total, stack, nodes, timers, requests, body, storage or memory", k)
		}
		if err != nil {
			return l, fmt.Errorf("tsx: limit %s: %v", k, err)
		}
	}
	return l, nil
}

// Sandbox is what a bundle runs within: its limits, what it may reach and
// import, the host it talks to and the storage it keeps.
type Sandbox struct {
	// plugin says Load runs it.
	plugin bool
	// exportView says a view renders one of its exports in place of what its entry module mounts.
	exportView bool
	// Limits are what it may use, DefaultLimits when nil.
	Limits *Limits
	// Capabilities are what its code may reach (a package's own); nil
	// reaches what the app's code does. Grant, when set, narrows them to
	// what it also allows: what the host gives this view.
	Capabilities, Grant *pkg.Capabilities
	// Modules are what it may import beyond SandboxModules and what is registered SandboxSafe into them: a module ("myapp"), one export ("nsgo/base.Terminal", "nsgo.setColorScheme") or static or member ("nsgo/base.Scrollbar.setTheme", "nsgo/mobile.BackHandler.exitApp"), a native component ("native:MapView"), or "*" for everything.
	Modules []string
	// Host is the channel its host module reaches, for the Go app to
	// answer its calls and send it events; nil gives it none.
	Host *core.Host
	// Storage names its partition of the app's storage and of its secrets (core.Scope.ProvideSecrets), which no one else's code sees; "" leaves it the app's.
	Storage string
	// WebUnisolated runs it on the web with the page's authority. The page's engine runs it in a realm of the page's origin, which reaches the page's own APIs (XMLHttpRequest, WebSocket, navigator, localStorage, the page's window) past Go, so there Capabilities and Grant hold only the requests the runtime and core make, and Storage's partition and a pkg.Trust kept in the page's storage are open to its code. Without it, a sandbox with Capabilities or Grant refuses to start on the web with ErrUnisolated. Other targets isolate it and ignore this.
	WebUnisolated bool
}

// ErrUnisolated is what a sandbox with Capabilities or Grant fails with on the web unless Sandbox.WebUnisolated is set: the page's engine cannot isolate a package's realm from the page, so what it declares would not hold.
var ErrUnisolated = errors.New("tsx: the web cannot isolate a sandboxed package from the page, so its capabilities would not hold; set Sandbox.WebUnisolated to run it with the page's authority")

// isolation is why s cannot start, browser saying whether the page's engine runs it. A sandbox without Capabilities or Grant reaches what the app's code does on every target, which the page's realm gives it anyway.
func (s Sandbox) isolation(browser bool) error {
	if browser && (s.Capabilities != nil || s.Grant != nil) && !s.WebUnisolated {
		return ErrUnisolated
	}
	return nil
}

// SandboxModules are the modules a sandboxed bundle imports without Sandbox.Modules naming them: NSGo's own, with what is registered into them SandboxSafe, none of the Go functions registered with RegisterFunctions, and none of their exports that act on the whole app (setColorScheme, BackHandler.exitApp).
var SandboxModules = []string{"nsgo", "nsgo:rt", "nsgo/base", "nsgo/base/plot", "nsgo/ui", "nsgo/ui/chat", "nsgo/mobile"}

// ViewOptions selects an exported component to render from a bundle. An empty Export keeps the bundle's entry-mounted root. Props are the component's props object, as JSON encodes and decodes them, so each view gets a copy of its own and the bundle reaches no Go value through them.
type ViewOptions struct {
	Export string
	Props  map[string]any
}

// Sandboxed returns the app of a bundle, as App does, run within s, in a view of its own: its frame, which fills the room its parent gives it, no more, so the bundle cannot grow over the host's views, and clips what the bundle shows, overlays included (core.Scope.Confine). In a scroll view that room is the scroll view's height: a host that wants the bundle as tall as its content puts it in a view of its own. On the web it panics with ErrUnisolated when s has Capabilities or Grant, unless s.WebUnisolated. When options has an Export, that named component, which runs within s as the rest of the bundle does, is rendered instead of the entry-mounted root; a missing one panics with ErrNoViewExport and one that is not a function with ErrViewExportNotComponent.
func Sandboxed(name, src string, s Sandbox, options ...ViewOptions) func(cx *core.Scope) core.Node {
	var view ViewOptions
	if len(options) > 0 {
		view = options[0]
	}
	s.exportView = view.Export != ""
	return func(cx *core.Scope) core.Node {
		inner := cx.Child()
		var root core.Node
		inner.Run(func() {
			r := s.start(inner, name, src)
			if view.Export != "" {
				r.renderExport(view.Export, view.Props)
			}
			root = r.root.elem
		})
		return core.View{Style: sandboxFrame, Children: []core.Node{root}}
	}
}

// renderExport mounts the entry module's export name as a component in the runtime's root, in place of what the entry mounted, with props, under a child owner so it has state of its own.
func (r *runtime) renderExport(name string, props map[string]any) {
	exports, _ := r.vm.Get(exportsGlobal).(*js.Object)
	if exports == nil || !defined(exports.Get(name)) {
		panic(fmt.Errorf("%w: %q", ErrNoViewExport, name))
	}
	component := exports.Get(name)
	if !isFunction(component) {
		panic(fmt.Errorf("%w: %q", ErrViewExportNotComponent, name))
	}
	input := r.vm.NewObject()
	if props != nil {
		// A copy, so that the bundle neither writes to the host's map, nor shares it with another view given the same one, nor reaches a Go value in it.
		b, err := json.Marshal(props)
		if err != nil {
			panic(fmt.Errorf("tsx: a view's props: %w", err))
		}
		var copied map[string]any
		if err := json.Unmarshal(b, &copied); err != nil {
			panic(fmt.Errorf("tsx: a view's props: %w", err))
		}
		input = r.vm.ToValue(copied).ToObject(r.vm)
	}
	for _, child := range slices.Clone(r.root.children) {
		r.removeChild(r.root, child)
	}
	owner := r.cx.Child()
	owner.Run(func() { r.insertValue(r.root, r.call(component, input), nil) })
}

// sandboxFrame is the style of the view a sandboxed bundle shows in, which the host's scope owns and the bundle cannot restyle.
var sandboxFrame = core.Style{FlexGrow: 1, FlexShrink: 1, Overflow: core.Hidden}

// start runs src in a new runtime under cx, panicking with what it throws as it loads, or with ErrUnisolated where it cannot be isolated.
func (s Sandbox) start(cx *core.Scope, name, src string) *runtime {
	if err := s.isolation(js.Browser); err != nil {
		panic(err)
	}
	limits := DefaultLimits
	if s.Limits != nil {
		limits = *s.Limits
	}
	if limits.Stack <= 0 || limits.Stack > maxCallStack {
		limits.Stack = maxCallStack
	}
	for _, c := range []*pkg.Capabilities{s.Capabilities, s.Grant} {
		if c != nil {
			cx.ProvideAccess(access{c})
		}
	}
	cx.ProvideHost(s.Host)
	// Its key bindings and back handlers, keymap's and BackHandler's and its Go components', fire only while the focus is in what it shows, and what it shows stays in its frame.
	cx.Confine()
	if s.Storage != "" {
		cx.ProvideStorage(s.Storage, limits.Storage)
		cx.ProvideSecrets(s.Storage)
	}
	r := newRuntime(cx)
	r.limits = limits
	if limits.Call > 0 && (limits.Load <= 0 || limits.Call < limits.Load) {
		r.regexTimeout = limits.Call
	} else {
		r.regexTimeout = limits.Load
	}
	r.sandbox = &sandboxed{caps: s.Capabilities != nil || s.Grant != nil, modules: s.Modules}
	r.exportView = s.exportView
	if limits.Stack < maxCallStack {
		r.vm.SetMaxCallStackSize(limits.Stack)
	}
	if limits.Memory > 0 {
		r.limitMemory(int64(limits.Memory))
	}
	r.limitNesting()
	if limits.Harden {
		r.harden()
	}
	if s.plugin && !js.Browser && (limits.Call > 0 || limits.CallTotal > 0) {
		r.keepPromiseJobs()
	}
	if s.plugin && limits.CallTotal > 0 {
		// What the entry module sets going as it loads runs until the load's deadline.
		r.deadlines = true
		r.current = newDeadline(cmp.Or(limits.Load, limits.CallTotal))
		r.watch(r.current)
		defer func() { r.current = nil }()
	}
	if err, _ := r.within(limits.Load, func() error {
		_, err := r.vm.RunScript(name, src)
		return err
	}); err != nil {
		panic(r.failure(err))
	}
	return r
}

// failure is what a call into JS failing with err fails with: why the
// runtime stopped when it did, whatever JS made of it.
func (r *runtime) failure(err error) error {
	if stopped := r.stoppedErr(); stopped != nil {
		return stopped
	}
	var exception *js.Exception
	if errors.As(err, &exception) && strings.HasPrefix(exception.Value().String(), "RangeError: match timeout after") {
		return ErrBudget
	}
	return err
}

// access is capabilities as core.Access.
type access struct{ c *pkg.Capabilities }

func (a access) Allows(req core.AccessRequest) bool {
	return a.c.Allows(req.Permission, req.Resource, req.Method)
}

func (a access) WebHosts() []string { return a.c.WebHosts() }

// sandboxed is a sandboxed runtime's state.
type sandboxed struct {
	// caps says capabilities hold it, so the runtime checks its own
	// requests.
	caps    bool
	modules []string
	// nodes, timers and requests are what it holds now.
	nodes, timers, requests int
	// stopped is why it stopped: no JS runs after.
	stopped error
	// memory is what the built-ins hold for it under Limits.Memory, nil without one.
	memory *memory
	// nesting is how many levels the built-ins limitNesting holds have gone down in the calls running.
	nesting int
}

// importsModule reports whether the bundle may import module: one of
// SandboxModules, or one Modules names, whole or by an export.
func (s *sandboxed) importsModule(module string) bool {
	if slices.Contains(SandboxModules, module) {
		return true
	}
	return slices.ContainsFunc(s.modules, func(m string) bool {
		return m == "*" || m == module || strings.HasPrefix(m, module+".")
	})
}

// importsExport reports whether the bundle may import an export of module, or a static, that is not SandboxSafe: one Modules names, or any of a module it names that is not one of SandboxModules.
func (s *sandboxed) importsExport(module, name string) bool {
	return slices.ContainsFunc(s.modules, func(m string) bool {
		return m == "*" || m == module+"."+name || m == module && !slices.Contains(SandboxModules, module)
	})
}

// appWide are the exports of NSGo's own modules, and their members, that act on the whole app rather than the view showing them, as Scrollbar.setTheme does: sandboxed bundles get them only as Sandbox.Modules names them. setColorScheme forces every window's appearance, and BackHandler.exitApp leaves the app.
var appWide = map[string]bool{"nsgo.setColorScheme": true, "nsgo/mobile.BackHandler.exitApp": true}

// withholds reports whether the runtime is sandboxed and may not import module's export, or member of one, name, one of appWide that Modules does not name.
func (r *runtime) withholds(module, name string) bool {
	return r.sandbox != nil && appWide[module+"."+name] && !r.sandbox.importsExport(module, name)
}

// imports reports whether the runtime may import a registered export of module, or a static of one ("TextView.parse"): all of them unless it is sandboxed, else those registered SandboxSafe and those Modules names.
func (r *runtime) imports(module, name string) bool {
	return r.sandbox == nil || isSandboxSafe(module, name) || r.sandbox.importsExport(module, name)
}

// importsNative reports whether the bundle may show the embedding app's
// native component name.
func (s *sandboxed) importsNative(name string) bool {
	return slices.Contains(s.modules, "*") || slices.Contains(s.modules, "native:"+name)
}

// checkAccess is the refusal of req, for the requests the runtime makes.
func (r *runtime) checkAccess(req core.AccessRequest) error {
	if r.sandbox == nil || !r.sandbox.caps {
		return nil
	}
	return r.cx.CheckAccess(req)
}

// quota is the failure of a quota of what used up.
func quota(what string, limit int) error {
	return &core.Error{Code: core.CodeQuotaExceeded, Resource: what,
		Message: fmt.Sprintf("tsx: the sandbox's quota of %d %s is used up", limit, what)}
}

// markStopped records err, ErrInterrupted when nil, as why the runtime stopped, and reports whether it was the first. Safe from any goroutine.
func (r *runtime) markStopped(err error) bool {
	if err == nil {
		err = ErrInterrupted
	}
	r.stopMu.Lock()
	defer r.stopMu.Unlock()
	if r.sandbox.stopped != nil {
		return false
	}
	r.sandbox.stopped = err
	return true
}

// stopTimers stops the runtime's waiting timers, on the engine's goroutine.
func (r *runtime) stopTimers() {
	for id := range r.timers.stop {
		r.stopTimer(id)
	}
}

// finish delivers a plugin call's result once, even if its promise later settles or the plugin closes. Successful calls leave their work running.
func (r *runtime) finish(dl *deadline, result any, err error) {
	delete(r.calls, dl)
	if done := dl.done; done != nil {
		dl.done = nil
		if dl.at.IsZero() {
			dl.timers = nil
		}
		done(result, err)
	}
}

// failCalls fails the plugin calls still waiting for a result after it stops.
func (r *runtime) failCalls(err error) {
	for dl := range r.calls {
		r.finish(dl, nil, err)
	}
}

// stop stops the runtime for err: the JS running is cut and none runs after, while the views stay as they are, its timers are dropped and the plugin calls waiting fail with err. What runs it panics with err into the nearest core.Catch. Call it on the engine's goroutine.
func (r *runtime) stop(err error) {
	if !r.markStopped(err) {
		if js.Browser {
			panic(r.goError(r.stoppedErr()))
		}
		return
	}
	r.stopTimers()
	if len(r.calls) > 0 {
		r.cx.Post(func() { r.failCalls(err) })
	}
	r.vm.Interrupt(err)
	if js.Browser {
		// The page's engine cannot be interrupted: a throw unwinds it.
		panic(r.goError(err))
	}
}

// interrupt stops the runtime for err as stop does, from any goroutine: goja's interrupt is safe to call concurrently, while the timers and the waiting calls belong to the engine's goroutine, which a post stops and fails.
func (r *runtime) interrupt(err error) {
	if !r.markStopped(err) {
		return
	}
	r.interrupted.Store(true)
	r.vm.Interrupt(err)
	r.cx.Post(func() {
		r.stopTimers()
		r.failCalls(r.stoppedErr())
	})
}

// stoppedErr is why the runtime stopped, nil while it runs.
func (r *runtime) stoppedErr() error {
	if r.sandbox == nil {
		return nil
	}
	r.stopMu.Lock()
	defer r.stopMu.Unlock()
	return r.sandbox.stopped
}

// newCoreElement makes a core element owned by cx, counted against the node
// quota.
func (r *runtime) newCoreElement(cx *core.Scope, props core.Node) *core.Element {
	if s := r.sandbox; s != nil && r.limits.Nodes > 0 {
		if s.nodes >= r.limits.Nodes || r.stoppedErr() != nil {
			r.stop(quota("nodes", r.limits.Nodes))
		}
		s.nodes++
		cx.Cleanup(func() { s.nodes-- })
	}
	return core.NewElement(cx, props)
}

// within runs one slice of JS. Only the outermost entry arms the slice watchdog, and cut reports whether a deadline handled the interruption.
func (r *runtime) within(budget time.Duration, fn func() error) (err error, cut bool) {
	if r.stoppedErr() != nil {
		return nil, false
	}
	if r.depth > 0 {
		return fn(), false
	}
	prev := r.current
	if prev == nil && r.deadlines {
		// A standalone host event has a deadline from its delivery.
		r.current = newDeadline(r.limits.CallTotal)
	}
	if r.passed(r.current) {
		r.current = prev
		return nil, true
	}
	r.depth++
	resumed := len(r.continuations.resume)
	if r.regexTimeout > 0 && !js.Browser {
		defer limitRegexps(r.regexTimeout)()
	}
	r.sliceDeadline = r.current
	var end time.Time
	if budget > 0 {
		end = time.Now().Add(budget)
	}
	if dl := r.current; dl != nil && !dl.at.IsZero() && (end.IsZero() || dl.at.Before(end)) {
		end = dl.at
	}
	var interrupt *sync.Once
	if !end.IsZero() {
		guard := new(sync.Once)
		interrupt = guard
		r.sliceEnd = end
		r.slice = time.AfterFunc(time.Until(end), func() {
			guard.Do(func() { r.vm.Interrupt(ErrBudget) })
		})
	}
	defer func() {
		total, owner := r.sliceDeadline, r.current
		inJob := len(r.continuations.resume) > resumed
		if interrupt != nil {
			r.slice.Stop()
			// Do not let a watchdog racing Stop interrupt the next slice.
			interrupt.Do(func() {})
			r.vm.ClearInterrupt()
			r.slice = nil
		}
		r.sliceDeadline = nil
		// Goja may unwind an interrupted reaction without calling Exited.
		for len(r.continuations.resume) > resumed {
			r.continuations.Exited()
		}
		r.depth--
		r.current = prev
		cut = r.passed(total)
		if r.retainPromiseJobs && (r.stoppedErr() != nil || r.cx.Disposed()) {
			r.vm.SetPromiseJobFilter(nil)
			return
		}
		if stop, ok := err.(*js.InterruptedError); ok && !r.interrupted.Load() && errors.Is(stop, ErrBudget) {
			if owner != nil && (owner.done != nil || inJob && r.retainPromiseJobs) {
				r.expire(owner)
				cut = true
				if inJob && r.retainPromiseJobs && owner != prev {
					// The other owner's reaction interrupted the queue
					// after this entry returned a value or threw.
					err = stop.CallError()
				}
			}
			if r.retainPromiseJobs {
				r.cx.Post(r.resumePromiseJobs)
			}
		}
	}()
	return fn(), false
}

func (r *runtime) keepPromiseJobs() {
	r.retainPromiseJobs = true
	r.vm.SetPromiseJobFilter(func(context any) bool {
		owner, _ := context.(*deadline)
		return r.stoppedErr() == nil && !r.cx.Disposed() && !r.passed(owner)
	})
	r.cx.Cleanup(func() {
		if r.depth == 0 {
			r.vm.SetPromiseJobFilter(nil)
		}
	})
}

func (r *runtime) resumePromiseJobs() {
	if r.stoppedErr() != nil || r.cx.Disposed() {
		r.vm.SetPromiseJobFilter(nil)
		return
	}
	if err, cut := r.within(r.limits.Call, r.vm.DrainPromiseJobs); err != nil && !cut {
		panic(r.failure(err))
	}
}

// harden closes what makes code from strings and freezes the shared
// prototypes.
var hardenProgram = js.MustCompile(websrc.Harden.Name, websrc.Harden.Source, true)

func (r *runtime) harden() {
	v, err := r.vm.RunProgram(hardenProgram)
	if err != nil {
		panic(err)
	}
	f, _ := js.AssertFunction(v)
	if _, err := f(js.Undefined(), r.vm.GlobalObject()); err != nil {
		panic(err)
	}
}
