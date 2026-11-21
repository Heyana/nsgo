//go:build !js

package tsx

import (
	"math"
	"slices"
	"strconv"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// maxNesting is how many levels the built-ins that go down what a bundle gives them in Go, a Go call or more for each level, may go in a sandbox before they throw a RangeError, as a browser's do past their stack.
const maxNesting = 1000

// limitNesting replaces those built-ins, JSON.stringify, Array.prototype.join, toString, toLocaleString and flat, and JSON.parse with a reviver, with ones that throw a RangeError once maxNesting levels deep. goja counts only JS calls against the call stack's limit (Limits.Stack), so data a million levels deep, which a bundle builds in a moment, had them search the levels above for cycles at every level for hours, past the reach of the call's time budget, which interrupts only JS, or overflow the goroutine's stack, which ends the process whatever the sandbox's limits (#875); an array whose join is Array.prototype.toString did so at once. The levels a wrapper goes down count in r.sandbox.nesting while it runs, so that one a getter, toJSON, an element's toString or a reviver calls from within another has only what is left. Each wrapper is the only one around its built-in: under Limits.Memory, join, toLocaleString and stringify check what they make as well (memory_goja.go). The web keeps the browser's built-ins, which have their own stack.
func (r *runtime) limitNesting() {
	g := r.vm.GlobalObject()
	array := object(g, "Array", "prototype")
	join, _ := js.AssertFunction(array.Get("join"))
	isArray, _ := js.AssertFunction(object(g, "Array", "isArray"))
	r.wrap(array, "join", func(call js.FunctionCall, orig js.Callable) js.Value {
		r.nest()
		defer r.unnest()
		if r.sandbox.memory == nil {
			return r.callOrig(orig, call.This, call.Arguments)
		}
		o, sep := r.vm.ToObject(call.This), call.Argument(0)
		if js.IsUndefined(sep) {
			sep = r.vm.ToValue(",")
		} else {
			sep = sep.ToString()
		}
		return r.join(o, sep, js.Value.ToString, join)
	})
	r.wrap(array, "toLocaleString", func(call js.FunctionCall, orig js.Callable) js.Value {
		r.nest()
		defer r.unnest()
		if r.sandbox.memory == nil {
			return r.callOrig(orig, call.This, call.Arguments)
		}
		return r.join(r.vm.ToObject(call.This), r.vm.ToValue(","), r.localeString, join)
	})
	// toString calls the array's join, which counts a level of its own, so that an array of arrays goes half as deep through String as through join; it counts one itself, as its join may be another toString, which would call itself without end. %TypedArray%.prototype.toString is the same function, so it takes the same wrapper.
	typed := object(g, "Uint8Array", "prototype").Prototype()
	toString := array.Get("toString")
	r.wrap(array, "toString", func(call js.FunctionCall, orig js.Callable) js.Value {
		r.nest()
		defer r.unnest()
		return r.callOrig(orig, call.This, call.Arguments)
	})
	if typed.Get("toString") == toString {
		_ = typed.DefineDataProperty("toString", array.Get("toString"), js.FLAG_TRUE, js.FLAG_TRUE, js.FLAG_FALSE)
	}
	r.wrap(array, "flat", func(call js.FunctionCall, orig js.Callable) js.Value {
		return r.flat(call, orig, isArray)
	})

	json := object(g, "JSON")
	isRawJSON, _ := js.AssertFunction(json.Get("isRawJSON"))
	var project js.Callable
	if m := r.sandbox.memory; m != nil {
		project = m.project
	} else {
		project, _ = js.AssertFunction(r.run(memoryProgram).(*js.Object).Get("project"))
	}
	r.wrap(json, "stringify", func(call js.FunctionCall, orig js.Callable) js.Value {
		return r.stringify(call, orig, isArray, isRawJSON, project)
	})
	walker, _ := js.AssertFunction(r.run(reviveProgram))
	enter := r.function(func(js.FunctionCall) js.Value { r.nest(); return js.Undefined() })
	leave := r.function(func(js.FunctionCall) js.Value { r.unnest(); return js.Undefined() })
	revive, _ := js.AssertFunction(r.callOrig(walker, js.Undefined(), []js.Value{enter, leave}))
	r.wrap(json, "parse", func(call js.FunctionCall, orig js.Callable) js.Value {
		// Without a reviver goja's parse calls nothing of the bundle's and goes down no more than the text, which its decoder stops at 10,000 levels.
		reviver := call.Argument(1)
		if !isFunction(reviver) {
			return r.callOrig(orig, call.This, call.Arguments)
		}
		v := r.callOrig(orig, call.This, []js.Value{call.Argument(0)})
		// What the walk counted is uncounted here when the reviver throws.
		outer := r.sandbox.nesting
		defer func() { r.sandbox.nesting = outer }()
		return r.callOrig(revive, js.Undefined(), []js.Value{reviver, v})
	})
}

// nest counts a level a built-in goes down, throwing a RangeError past maxNesting.
func (r *runtime) nest() {
	if r.sandbox.nesting >= maxNesting {
		panic(r.tooDeep())
	}
	r.sandbox.nesting++
}

// unnest uncounts the level nest counted.
func (r *runtime) unnest() { r.sandbox.nesting-- }

// tooDeep is the RangeError a built-in throws past maxNesting, a browser's for a stack too deep.
func (r *runtime) tooDeep() *js.Object {
	return r.goError(&JSError{Name: "RangeError", Message: "Maximum call stack size exceeded"})
}

// flat is Array.prototype.flat, which goes down as many levels as its depth asks. It reads the depth once and hands it to the original as a number. Asked to go deeper than the levels left, it has the original go down those and throws if what it made still holds an array, which goes deeper.
func (r *runtime) flat(call js.FunctionCall, orig, isArray js.Callable) js.Value {
	args := slices.Clone(call.Arguments)
	depth := 1.0
	if len(args) > 0 && !js.IsUndefined(args[0]) {
		args[0] = args[0].ToNumber()
		if depth = math.Trunc(args[0].ToFloat()); math.IsNaN(depth) {
			depth = 0
		}
	}
	outer := r.sandbox.nesting
	defer func() { r.sandbox.nesting = outer }()
	left := maxNesting - outer
	if depth <= float64(left) {
		r.sandbox.nesting += int(max(depth, 0))
		return r.callOrig(orig, call.This, args)
	}
	if left <= 0 {
		panic(r.tooDeep())
	}
	if len(args) == 0 {
		args = append(args, nil)
	}
	args[0] = r.vm.ToValue(left)
	r.sandbox.nesting = maxNesting
	v := r.callOrig(orig, call.This, args)
	r.sandbox.nesting = outer
	o := r.vm.ToObject(v)
	for i, n := int64(0), r.lengthOf(o); i < n; i++ {
		if e := o.Get(strconv.FormatInt(i, 10)); e != nil && r.callOrig(isArray, js.Undefined(), []js.Value{e}).ToBoolean() {
			panic(r.tooDeep())
		}
	}
	return v
}

// reviveProgram is JSON.parse's walk of what it parsed through a reviver, as goja's own walks it: down each array's elements, as many as its length when it is reached, and each other object's own enumerable string keys, calling the reviver with each value after what is in it and deleting what it returns undefined for. goja's walks in Go, a Go call for each level, down what the reviver may have put in place of what was parsed; this one walks in JS with a stack of its own, so that it takes no more of the call stack however deep it goes, counting each level with enter and leave, nest and unnest. Its stack is set by definition, which no setter a bundle adds to Array.prototype intercepts.
var reviveProgram = js.MustCompile("revive.js", `(function (isArray, keys, define, remove, apply) {
	"use strict";
	const data = (value) => ({ __proto__: null, value, writable: true, enumerable: true, configurable: true });
	return (enter, leave) => (reviver, root) => {
		const stack = [];
		const open = (holder, name) => {
			const value = holder[name];
			const f = { __proto__: null, holder, name, value, names: null, n: 0, i: 0, open: false };
			if ((typeof value === "object" || typeof value === "function") && value !== null) {
				enter();
				f.open = true;
				if (isArray(value)) {
					const n = +value.length;
					f.n = !(n > 0) ? 0 : n > 9007199254740991 ? 9007199254740991 : n - n % 1;
				} else {
					f.names = keys(value);
					f.n = f.names.length;
				}
			}
			define(stack, stack.length, data(f));
		};
		open({ "": root }, "");
		for (;;) {
			const f = stack[stack.length - 1];
			if (f.i < f.n) {
				const key = f.names === null ? "" + f.i : f.names[f.i];
				f.i++;
				open(f.value, key);
				continue;
			}
			stack.length--;
			if (f.open) leave();
			const v = apply(reviver, f.holder, [f.name, f.value]);
			if (stack.length === 0) return v;
			if (v === undefined) remove(f.holder, f.name);
			else define(f.holder, f.name, data(v));
		}
	};
})(Array.isArray, Object.keys, Reflect.defineProperty, Reflect.deleteProperty, Reflect.apply)`, true)
