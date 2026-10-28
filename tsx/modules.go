package tsx

import "github.com/dop251/goja"

// The modules a bundle requires (NSH's modules.rs): nsgo, the API apps
// write against, nsgo/base, and nsgo:rt, what compiled JSX calls.

// export is one export of a module and how a VM makes it.
type export struct {
	name string
	make func(r *runtime) goja.Value
}

// fn exports a function of the runtime.
func fn(name string, f func(r *runtime, call goja.FunctionCall) goja.Value) export {
	return export{name, func(r *runtime) goja.Value {
		return r.function(func(call goja.FunctionCall) goja.Value { return f(r, call) })
	}}
}

// host exports a primitive as a component.
func host(name, tag string) export {
	return export{name, func(r *runtime) goja.Value { return r.hostComponent(tag) }}
}

// script exports what a JS source evaluates to.
func script(name, src string) export {
	return export{name, func(r *runtime) goja.Value {
		v, err := r.vm.RunString(src)
		if err != nil {
			panic(err)
		}
		return v
	}}
}

func init() { moduleTable["nsgo"] = append(moduleTable["nsgo"], envExports...) }

var moduleTable = map[string][]export{
	"nsgo": {
		fn("signal", (*runtime).signal),
		fn("computed", (*runtime).computed),
		fn("effect", (*runtime).effect),
		fn("batch", (*runtime).batch),
		fn("untracked", (*runtime).untracked),
		fn("cleanup", (*runtime).cleanup),
		fn("mount", (*runtime).mount),
		fn("context", (*runtime).context),
		fn("ErrorBoundary", (*runtime).errorBoundary),
		script("split", splitSource),
		host("View", "view"),
		host("Text", "text"),
		host("Scroll", "scroll"),
		host("Image", "image"),
	},
	"nsgo/base": {
		host("Input", "input"),
	},
	"nsgo:rt": {
		fn("element", (*runtime).element),
		fn("text", (*runtime).textNode),
		fn("append", (*runtime).append),
		fn("insert", (*runtime).insert),
		fn("prop", (*runtime).prop),
		fn("bindProp", (*runtime).bindProp),
		fn("component", (*runtime).component),
		fn("cond", (*runtime).cond),
		fn("each", (*runtime).each),
		fn("ref", (*runtime).ref),
		fn("spread", (*runtime).spread),
		script("merge", mergeSource),
	},
}
