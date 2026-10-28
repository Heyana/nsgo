// Package tsx runs TSX apps: bundles of what tsx/compiler emits, on the
// goja JavaScript engine, rendered by core as Go apps are. It is a port
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

	"github.com/dop251/goja"

	"github.com/oneo-me/nsgo/core"
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

// runtime is one VM and what NSH keeps per context: the module table,
// the graph's scope and the renderer's root.
type runtime struct {
	vm      *goja.Runtime
	cx      *core.Scope
	root    *node
	modules map[string]*goja.Object
	// warned holds the warnings given, each given once.
	warned map[string]bool
	timers timers
}

func newRuntime(cx *core.Scope) *runtime {
	r := &runtime{vm: goja.New(), cx: cx, modules: map[string]*goja.Object{}, warned: map[string]bool{}}
	r.root = &node{r: r, kind: kindRegion, elem: core.NewElement(cx, nil)}
	r.vm.Set("require", r.require)
	r.installGlobals()
	r.installTimers()
	return r
}

// require returns a module's exports, made on first use.
func (r *runtime) require(name string) goja.Value {
	if m, ok := r.modules[name]; ok {
		return m
	}
	exports, ok := moduleTable[name]
	if !ok {
		panic(r.vm.NewTypeError("nsgo: no module %q", name))
	}
	m := r.vm.NewObject()
	for _, e := range exports {
		_ = m.Set(e.name, e.make(r))
	}
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
func (r *runtime) call(fn goja.Value, args ...goja.Value) goja.Value {
	f, ok := goja.AssertFunction(fn)
	if !ok {
		panic(r.vm.NewTypeError("nsgo: %s is not a function", fn))
	}
	v, err := f(goja.Undefined(), args...)
	if err != nil {
		panic(err)
	}
	return v
}

// function makes a JS function of fn.
func (r *runtime) function(fn func(call goja.FunctionCall) goja.Value) *goja.Object {
	return r.vm.ToValue(fn).(*goja.Object)
}

func isFunction(v goja.Value) bool {
	_, ok := goja.AssertFunction(v)
	return ok
}

// arg returns argument i, undefined when missing.
func arg(call goja.FunctionCall, i int) goja.Value { return call.Argument(i) }
