package tsx

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"
	"github.com/nsgo-dev/nsgo/tsx/pkg"

	"github.com/nsgo-dev/nsgo/core"
)

// View runs the app of a package a pkg.Trust verified where it is placed in a Go app, sandboxed: the app's own screens delivered anew, or a plugin. The package's capabilities hold it, narrowed by Grant; it keeps a partition of the app's storage of its app ID's, and its host module reaches Host alone. A package that throws as it loads, or stops at a quota, panics, as any component does; place it under a core.Catch to show something else. Export names a component the package's entry module exports, which it shows in place of the root the entry mounts, with Props, so one package provides several views, each with state of its own; it runs within the same sandbox, capabilities and limits. The web cannot isolate a package from the page, so there it panics with ErrUnisolated unless Sandbox.WebUnisolated.
//
//	p, err := trust.Verify(data) // off the engine's goroutine
//	...
//	core.Catch{Try: tsx.View{Package: p}.Render, Fallback: failed}
type View struct {
	Package *pkg.Package
	// Export names a component the package's entry module exports, which the view renders in place of the root the entry mounts, whose mount then neither renders nor runs its component; empty keeps that root.
	Export string
	// Props are Export's props object, as JSON encodes them.
	Props map[string]any
	// Sandbox's Capabilities are the package's and its Storage its app
	// ID when unset.
	Sandbox
}

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	if v.Package == nil {
		panic("tsx: a View needs a package")
	}
	return Sandboxed(v.Package.App.ID, v.Package.Source(), v.Sandbox.of(v.Package), ViewOptions{Export: v.Export, Props: v.Props})(cx)
}

// of is s for package p.
func (s Sandbox) of(p *pkg.Package) Sandbox {
	if s.Capabilities == nil {
		s.Capabilities = &p.Capabilities
	}
	if s.Storage == "" {
		s.Storage = p.App.ID
	}
	return s
}

// Plugin is a package that gives the Go app functions rather than a view:
// what its entry module exports, which Call calls. It runs sandboxed as
// View does, under the scope that loaded it, until Close or that scope's
// end.
//
//	// search.tsx
//	export async function search(query: string) { ... }
//
//	plugin, err := tsx.Load(cx, p, tsx.Sandbox{})
//	plugin.Call("search", []any{"query"}, func(result any, err error) { ... })
type Plugin struct {
	r       *runtime
	cx      *core.Scope
	exports *js.Object
	closing bool // guards reentrant Close on the engine's goroutine
}

// ErrNoExport is the failure of a call of a name the plugin does not
// export as a function.
var ErrNoExport = errors.New("tsx: the plugin exports no such function")

// ErrNoViewExport is the failure of a View.Export name the package does not export.
var ErrNoViewExport = errors.New("tsx: the view exports no such component")

// ErrViewExportNotComponent is the failure of a View.Export the package exports as something other than a function.
var ErrViewExportNotComponent = errors.New("tsx: the view export is not a component")

// ErrNoTree is what Load reports when its scope has no view tree.
var ErrNoTree = errors.New("tsx: Load needs a scope attached to a view tree")

// Load runs package p as a plugin under cx, within s as View runs it, and returns it once its entry module ran, or a nil Plugin and what that threw, which goes to no Catch above cx and not to the app's report, or ErrNoTree when cx is not attached to a view tree, or ErrUnisolated on the web as View panics with it. It runs synchronously on the caller's goroutine. Call it on the engine's goroutine.
func Load(cx *core.Scope, p *pkg.Package, s Sandbox) (plugin *Plugin, err error) {
	if cx == nil || !cx.HasTree() {
		return nil, ErrNoTree
	}
	s = s.of(p)
	s.plugin = true
	// Refused before the scope runs it, which would send the panic to a Catch rather than to the caller.
	if err := s.isolation(js.Properties().SharesPageAuthority); err != nil {
		return nil, err
	}
	inner := cx.Child()
	var r *runtime
	inner.Run(func() {
		// Recovered here, inside the scope's run: past it, core would send what the entry module threw to a Catch above cx or to the app's report, and Load would return nothing of it. Disposed before the run ends, so nothing the module set up runs after it failed.
		defer func() {
			if x := recover(); x != nil {
				inner.Dispose()
				if e, ok := x.(error); ok {
					err = e
				} else {
					err = fmt.Errorf("tsx: the plugin failed as it loaded: %v", x)
				}
			}
		}()
		// Scope cleanups run in reverse order. Let the plugin run its
		// own cleanups before recording a normal close.
		inner.Cleanup(func() {
			if plugin != nil {
				plugin.dispose()
			}
		})
		r = s.start(inner, p.App.ID, p.Source())
	})
	if err != nil {
		return nil, err
	}
	plugin = &Plugin{r: r, cx: inner}
	plugin.exports, _ = r.vm.Get(exportsGlobal).(*js.Object)
	r.onPluginStop = s.OnStop
	return plugin, nil
}

// exportsGlobal is where a bundle leaves its entry module's exports
// (bundler.Compile).
const exportsGlobal = "__nsgo_exports"

// Exports are the names of the functions the plugin exports.
func (p *Plugin) Exports() []string {
	if p.exports == nil {
		return nil
	}
	var out []string
	for _, k := range p.exports.Keys() {
		if isFunction(p.exports.Get(k)) {
			out = append(out, k)
		}
	}
	return out
}

// Call calls the plugin's export name with JSON arguments and passes its result to done on the engine's goroutine, awaiting a returned promise. The error is what the export threw or rejected with (a *core.Error when it carries a code), ErrNoExport, ErrBudget for a cut slice or expired total budget, or why the plugin stopped or closed. Call it on the engine's goroutine.
func (p *Plugin) Call(name string, args []any, done func(result any, err error)) {
	r := p.r
	if err := r.stoppedErr(); err != nil || p.cx.Disposed() {
		done(nil, cmp.Or(err, ErrPluginClosed))
		return
	}
	var v js.Value
	if p.exports != nil {
		v = p.exports.Get(name)
	}
	fn, ok := js.AssertFunction(v)
	if !ok {
		done(nil, fmt.Errorf("%w: %q", ErrNoExport, name))
		return
	}
	if r.calls == nil {
		r.calls = map[*deadline]struct{}{}
	}
	dl := newDeadline(r.limits.CallTotal)
	dl.done = done
	r.calls[dl] = struct{}{}
	r.watch(dl)
	owner := dl
	if !r.deadlines && (!js.Properties().CanInterrupt || r.limits.Call <= 0) {
		owner = nil
	}
	p.cx.Run(func() { r.under(owner, func() { p.call(fn, args, dl) }) })
}

func (p *Plugin) call(fn js.Callable, args []any, dl *deadline) {
	r := p.r
	defer func() {
		if x := recover(); x != nil {
			var callErr error
			if ex, ok := x.(*js.Exception); ok {
				callErr = r.rejection(ex.Value())
			} else if e, ok := x.(error); ok {
				callErr = e
			} else {
				callErr = fmt.Errorf("%v", x)
			}
			if r.depth == 0 && errors.Is(callErr, ErrBudget) && r.stoppedErr() == nil {
				r.expire(dl)
			} else {
				r.finish(dl, nil, callErr)
			}
		}
	}()
	in := make([]js.Value, len(args))
	for i, a := range args {
		in[i] = r.vm.ToValue(a)
	}
	v, err, _ := r.invoke(fn, js.Undefined(), in...)
	if err := r.stoppedErr(); err != nil {
		panic(err)
	}
	if dl.expired {
		return
	}
	if err != nil {
		panic(r.failure(err))
	}
	if _, ok := v.Export().(*js.Promise); !ok {
		r.finish(dl, exportJSON(v), nil)
		return
	}
	then, _ := js.AssertFunction(v.(*js.Object).Get("then"))
	onResolve := r.function(func(call js.FunctionCall) js.Value {
		r.finish(dl, exportJSON(arg(call, 0)), nil)
		return js.Undefined()
	})
	onReject := r.function(func(call js.FunctionCall) js.Value {
		r.finish(dl, nil, r.rejection(arg(call, 0)))
		return js.Undefined()
	})
	if _, err, _ := r.invoke(then, v, onResolve, onReject); err != nil {
		panic(r.failure(err))
	}
}

// Interrupt stops the plugin for err, ErrInterrupted when nil, from any goroutine: the JS running is cut and none runs after, its waiting timers and promise continuations are dropped, and the calls under way and every Call after fail with err.
func (p *Plugin) Interrupt(err error) {
	if p == nil || p.r == nil {
		return
	}
	if err == nil {
		err = ErrInterrupted
	}
	p.r.interrupt(err)
}

// Stopped reports whether the plugin has stopped. It is safe to call from
// any goroutine.
func (p *Plugin) Stopped() bool { return p.Err() != nil }

// Err returns the plugin's first stop reason, or nil while it runs.
// Closing the plugin or disposing its owner normally gives ErrPluginClosed.
// Err is safe to call from any goroutine.
func (p *Plugin) Err() error {
	if p == nil || p.r == nil {
		return nil
	}
	return p.r.stoppedErr()
}

// Close runs the plugin's cleanups and fails waiting calls with its stop
// reason. Call it on the engine's goroutine.
func (p *Plugin) Close() {
	if p == nil || p.r == nil || p.closing {
		return
	}
	p.closing = true
	defer func() { p.closing = false }()
	p.cx.Dispose()
	// Posted stop work dies with the plugin's scope.
	p.r.finishPluginStop(p.Err())
}

func (p *Plugin) dispose() {
	p.r.markStopped(ErrPluginClosed)
	p.r.stopTimers()
	if p.closing || p.r.onPluginStop == nil {
		return
	}
	// The owner disposed the plugin. An explicit Close completes this
	// work after Dispose returns.
	p.closing = true
	defer func() { p.closing = false }()
	p.r.finishPluginStop(p.Err())
}

// ErrPluginClosed is the stop reason when a plugin closes normally.
var ErrPluginClosed = errors.New("tsx: the plugin was closed")

// rejection is what JS threw as a Go error: its message, with its code.
func (r *runtime) rejection(v js.Value) error {
	o, ok := v.(*js.Object)
	if !ok {
		return fmt.Errorf("%v", exportJSON(v))
	}
	msg := o.Get("message")
	if !defined(msg) {
		return fmt.Errorf("%v", v.String())
	}
	if code := o.Get("code"); defined(code) {
		return &core.Error{Code: code.String(), Message: msg.String()}
	}
	return errors.New(msg.String())
}
