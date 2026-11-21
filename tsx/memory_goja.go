//go:build !js

package tsx

import (
	"fmt"
	"math"
	"reflect"
	goruntime "runtime"
	"slices"
	"strconv"
	"sync"
	"unicode/utf16"
	"weak"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// A sandbox's memory budget (Limits.Memory). goja allocates from Go's heap, which the host and every other runtime share, so a sample of that heap cannot tell what one bundle holds, and one allocation can take more than the system has before any sample sees it. Instead the built-ins that allocate by a size the bundle gives them, a length, a count or a length's worth of elements, are replaced by ones that check the budget before they call the original, so that what would not fit is never allocated, and charge what the original made until the Go collector frees it. What the original reads its size from, a length, an argument list or the parts of a string it joins, is read once, here, and handed to it as read, as a value, a copy or a proxy that answers the length, so that a getter, a proxy or a valueOf cannot have it allocate more than was checked. The bundle's other allocations, its objects, closures, the arrays it builds, copies, concatenates, flattens or spreads, the strings it concatenates or replaces into and the copies the web APIs make, are not counted: goja gives no hook for them. Those made in JS cost the bundle time as they grow, which Limits.Call bounds per call, but the built-ins that copy what the bundle holds do so at the speed of memory, so a bundle that doubles an array or a string a few dozen times, or flattens or concatenates many references to one, still takes as much as it likes: Memory stops runaway allocations of the built-ins it holds, not a bundle written to exhaust memory.

// memory is what the built-ins hold for a sandbox.
type memory struct {
	limit int64
	mu    sync.Mutex
	used  int64
	// held is what each charged allocation holds, by a weak pointer to it: the first byte of a buffer's or a string's memory, or an array's object. A cleanup on it releases what it holds, or collect once the collector found it unreachable.
	held map[any]holding
	// joining are the arrays Array.prototype.join is joining, outermost first, as goja's own join keeps them, so that an array in itself joins as "".
	joining []*js.Object
	// pin, iterate and project are memoryProgram's, which the wrappers hand the originals in place of what the bundle gave them.
	pin, iterate, project js.Callable
}

const (
	// valueSize is what a value in an array or a list of arguments takes: a Go interface.
	valueSize = 16
	// unitSize is the most a string's UTF-16 code unit takes: goja keeps an ASCII string in a byte each.
	unitSize = 2
	// numberLen is the most UTF-16 code units a number takes as a string, a locale's grouping included.
	numberLen = 32
	// heldMin is the least an allocation is charged from. Smaller ones cost no more than the objects goja makes uncounted, and Go may batch tiny ones with others so that they are never freed alone.
	heldMin = 1 << 10
)

// holding is what an allocation is charged, and whether the collector has yet to find it unreachable.
type holding struct {
	n    int64
	live func() bool
}

// limitMemory holds the built-ins that allocate by size to limit bytes, before the bundle runs and hardening freezes them.
func (r *runtime) limitMemory(limit int64) {
	m := &memory{limit: limit, held: map[any]holding{}}
	r.sandbox.memory = m
	g := r.vm.GlobalObject()
	helpers := r.run(memoryProgram).(*js.Object)
	m.pin, _ = js.AssertFunction(helpers.Get("pin"))
	m.iterate, _ = js.AssertFunction(helpers.Get("iterate"))
	m.project, _ = js.AssertFunction(helpers.Get("project"))
	r.wrapConstructor("ArrayBuffer", 0)
	for _, name := range []string{"Uint8Array", "Uint8ClampedArray", "Int8Array", "Uint16Array", "Int16Array", "Uint32Array", "Int32Array", "Float16Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array"} {
		r.wrapConstructor(name, g.Get(name).(*js.Object).Get("BYTES_PER_ELEMENT").ToInteger())
	}

	buffer := object(g, "ArrayBuffer", "prototype")
	r.wrap(buffer, "slice", func(call js.FunctionCall, orig js.Callable) js.Value {
		r.checkMemory(int64(len(r.bytesOf(call.This))))
		return r.holdBytes(r.callOrig(orig, call.This, call.Arguments))
	})
	transfer := func(call js.FunctionCall, orig js.Callable) js.Value {
		args := slices.Clone(call.Arguments)
		grows := false
		if len(args) > 0 && !js.IsUndefined(args[0]) {
			args[0] = args[0].ToNumber()
			n := lengthOf(args[0])
			if grows = n > int64(len(r.bytesOf(call.This))); grows {
				r.checkMemory(n)
			}
		}
		v := r.callOrig(orig, call.This, args)
		if grows {
			r.holdBytes(v)
		}
		return v
	}
	r.wrap(buffer, "transfer", transfer)
	r.wrap(buffer, "transferToFixedLength", transfer)
	typed := object(g, "Uint8Array", "prototype").Prototype()
	for _, name := range []string{"slice", "map", "filter", "toReversed", "toSorted", "with"} {
		r.wrap(typed, name, func(call js.FunctionCall, orig js.Callable) js.Value {
			r.checkMemory(int64(len(r.bytesOf(call.This))))
			return r.holdBytes(r.callOrig(orig, call.This, call.Arguments))
		})
	}
	// A typed array's join and toLocaleString write each number in at most numberLen units.
	for _, name := range []string{"join", "toLocaleString"} {
		r.wrap(typed, name, func(call js.FunctionCall, orig js.Callable) js.Value {
			args := slices.Clone(call.Arguments)
			sepLen := int64(1)
			if name == "join" && len(args) > 0 && !js.IsUndefined(args[0]) {
				args[0] = args[0].ToString()
				sepLen = stringLen(args[0])
			}
			r.checkMemory(mul(r.elementsOf(call.This), numberLen+sepLen, unitSize))
			return r.holdString(r.callOrig(orig, call.This, args))
		})
	}

	str := object(g, "String", "prototype")
	r.wrap(str, "repeat", func(call js.FunctionCall, orig js.Callable) js.Value {
		if js.IsUndefined(call.This) || js.IsNull(call.This) {
			return r.callOrig(orig, call.This, call.Arguments)
		}
		s, count := call.This.ToString(), call.Argument(0).ToNumber()
		if f := count.ToFloat(); f < 0 || math.IsInf(f, 0) {
			// Out of range: the original throws its RangeError.
			return r.callOrig(orig, s, []js.Value{count})
		}
		r.checkMemory(mul(stringLen(s), lengthOf(count), unitSize))
		return r.holdString(r.callOrig(orig, s, []js.Value{count}))
	})
	pad := func(call js.FunctionCall, orig js.Callable) js.Value {
		if js.IsUndefined(call.This) || js.IsNull(call.This) {
			return r.callOrig(orig, call.This, call.Arguments)
		}
		s, n := call.This.ToString(), call.Argument(0).ToNumber()
		args := []js.Value{n}
		filled := true
		if fill := call.Argument(1); !js.IsUndefined(fill) {
			fill = fill.ToString()
			args = append(args, fill)
			filled = stringLen(fill) > 0
		}
		if !filled || lengthOf(n) <= stringLen(s) {
			return r.callOrig(orig, s, args)
		}
		r.checkMemory(mul(lengthOf(n), unitSize))
		return r.holdString(r.callOrig(orig, s, args))
	}
	r.wrap(str, "padStart", pad)
	r.wrap(str, "padEnd", pad)
	r.wrap(object(g, "String"), "raw", func(call js.FunctionCall, orig js.Callable) js.Value {
		return r.raw(call, orig)
	})

	// Array.prototype.join and toLocaleString, and JSON.stringify, check what they make as they make it in the wrappers limitNesting puts around them, which hold how deep they go as well.
	array := object(g, "Array", "prototype")
	// fill, sort and the copies read the length of what is not an array once more, so they are given it pinned to what was checked.
	r.wrap(array, "fill", func(call js.FunctionCall, orig js.Callable) js.Value {
		o := r.vm.ToObject(call.This)
		this, n := r.pinned(o, false)
		start, end := call.Argument(1).ToNumber(), call.Argument(2)
		if !js.IsUndefined(end) {
			end = end.ToNumber()
		}
		from, to := relIndex(start, n, 0), relIndex(end, n, n)
		if to > from {
			r.checkMemory(mul(to-from, valueSize))
		}
		r.callOrig(orig, this, []js.Value{call.Argument(0), start, end})
		hold(r, o, mul(to, valueSize))
		return o
	})
	// sort copies the elements of what is not an array into a list first.
	r.wrap(array, "sort", func(call js.FunctionCall, orig js.Callable) js.Value {
		o := r.vm.ToObject(call.This)
		this, n := r.pinned(o, false)
		r.checkMemory(mul(n, valueSize))
		r.callOrig(orig, this, call.Arguments)
		return o
	})
	// What makes an array as long as this one, or one longer by its arguments.
	for _, name := range []string{"toSorted", "toReversed", "toSpliced", "with"} {
		r.wrap(array, name, func(call js.FunctionCall, orig js.Callable) js.Value {
			this, n := r.pinned(r.vm.ToObject(call.This), false)
			r.checkMemory(mul(n+int64(len(call.Arguments)), valueSize))
			return r.holdArray(r.callOrig(orig, this, call.Arguments))
		})
	}
	r.wrap(object(g, "Array"), "from", func(call js.FunctionCall, orig js.Callable) js.Value {
		args := slices.Clone(call.Arguments)
		if o, ok := call.Argument(0).(*js.Object); ok {
			var n int64
			args[0], n = r.source(o)
			r.checkMemory(mul(n, valueSize))
		} else if s, ok := call.Argument(0).(js.String); ok {
			// A string iterates by code points, no more of them than its units.
			r.checkMemory(mul(int64(s.Length()), valueSize))
		}
		return r.holdArray(r.callOrig(orig, call.This, args))
	})

	// What makes a list of arguments of an array-like's length. The list is copied here, reading its length and elements once as the original would, so that the original makes it of what was checked.
	arguments := func(i int) func(call js.FunctionCall, orig js.Callable) js.Value {
		return func(call js.FunctionCall, orig js.Callable) js.Value {
			args := slices.Clone(call.Arguments)
			if o, ok := call.Argument(i).(*js.Object); ok {
				args[i] = r.argumentList(o)
			}
			return r.callOrig(orig, call.This, args)
		}
	}
	r.wrap(object(g, "Function", "prototype"), "apply", arguments(1))
	r.wrap(object(g, "Reflect"), "apply", arguments(2))
	r.wrap(object(g, "Reflect"), "construct", arguments(1))
}

// object is the object at path from o.
func object(o *js.Object, path ...string) *js.Object {
	for _, name := range path {
		o = o.Get(name).(*js.Object)
	}
	return o
}

// adoptProgram replaces the global constructor name with a wrapper that constructs through construct, taking the original's statics, prototype and place in the prototype chain, so that instanceof, subclasses, species and the statics find it. The wrapper is JS, as only JS tells a call from new (goja's Go constructors see neither new.target nor new).
var adoptProgram = js.MustCompile("memory.js", `(function (g, name, construct) {
	"use strict";
	const original = g[name], apply = Reflect.apply;
	const wrapper = function (...args) {
		return new.target === undefined ? apply(original, this, args) : construct(new.target, args);
	};
	for (const key of Reflect.ownKeys(original)) {
		if (key !== "prototype") Object.defineProperty(wrapper, key, Object.getOwnPropertyDescriptor(original, key));
	}
	Object.defineProperty(wrapper, "prototype", { value: original.prototype, writable: false });
	Object.defineProperty(original.prototype, "constructor", { value: wrapper });
	Object.setPrototypeOf(wrapper, Object.getPrototypeOf(original));
	Object.defineProperty(g, name, { value: wrapper });
})`, true)

// wrapConstructor replaces the global constructor name, of a typed array of elements of size bytes or, size 0, of ArrayBuffer, with one that checks what it would allocate first and charges what it made.
func (r *runtime) wrapConstructor(name string, size int64) {
	construct, _ := js.AssertConstructor(r.vm.Get(name))
	checked := r.function(func(call js.FunctionCall) js.Value {
		newTarget := call.Argument(0).(*js.Object)
		list := call.Argument(1).(*js.Object)
		args := make([]js.Value, r.lengthOf(list))
		for i := range args {
			args[i] = list.Get(strconv.Itoa(i))
		}
		allocates := true
		if len(args) > 0 {
			switch a, ok := args[0].(*js.Object); {
			case size == 0:
				// ArrayBuffer's length, read once.
				args[0] = args[0].ToNumber()
				r.checkMemory(lengthOf(args[0]))
			case ok:
				switch {
				case a.ExportType() == arrayBufferType:
					// A view of a buffer allocates nothing.
					allocates = false
				case isTyped(a):
					// Another typed array, as long as it is, whatever its length property says.
					r.checkMemory(mul(r.elementsOf(a), size))
				default:
					// An array-like as long as its length, or an iterable, as many as it yields, charged once made.
					var n int64
					args[0], n = r.source(a)
					r.checkMemory(mul(n, size))
				}
			default:
				args[0] = args[0].ToNumber()
				r.checkMemory(mul(lengthOf(args[0]), size))
			}
		}
		o, err := construct(newTarget, args...)
		if err != nil {
			panic(err)
		}
		if allocates {
			r.holdBytes(o)
		}
		return o
	})
	adopt, _ := js.AssertFunction(r.run(adoptProgram))
	if _, err := adopt(js.Undefined(), r.vm.GlobalObject(), r.vm.ToValue(name), checked); err != nil {
		panic(err)
	}
}

var arrayBufferType = reflect.TypeFor[js.ArrayBuffer]()

// run runs a program of the runtime's own.
func (r *runtime) run(p *js.Program) js.Value {
	v, err := r.vm.RunProgram(p)
	if err != nil {
		panic(err)
	}
	return v
}

// wrap replaces o's method name with fn, which calls the original, keeping its name and length.
func (r *runtime) wrap(o *js.Object, name string, fn func(call js.FunctionCall, orig js.Callable) js.Value) {
	v, _ := o.Get(name).(*js.Object)
	orig, ok := js.AssertFunction(v)
	if !ok {
		return
	}
	f := r.function(func(call js.FunctionCall) js.Value { return fn(call, orig) })
	for _, key := range []string{"name", "length"} {
		_ = f.DefineDataProperty(key, v.Get(key), js.FLAG_FALSE, js.FLAG_TRUE, js.FLAG_FALSE)
	}
	_ = o.DefineDataProperty(name, f, js.FLAG_TRUE, js.FLAG_TRUE, js.FLAG_FALSE)
}

// callOrig calls a wrapped built-in, throwing what it throws.
func (r *runtime) callOrig(orig js.Callable, this js.Value, args []js.Value) js.Value {
	v, err := orig(this, args...)
	if err != nil {
		panic(err)
	}
	return v
}

// join joins the elements of o, an array or an array-like, with sep through orig, the original Array.prototype.join, as Array.prototype.join does when part is ToString and toLocaleString when it calls each element's: it makes each a string first, so that what they add up to is checked before the original allocates the whole. An array within itself joins as "", as in goja's own.
func (r *runtime) join(o *js.Object, sep js.Value, part func(e js.Value) js.Value, orig js.Callable) js.Value {
	m := r.sandbox.memory
	if slices.Contains(m.joining, o) {
		return r.vm.ToValue("")
	}
	m.joining = append(m.joining, o)
	defer func() { m.joining = m.joining[:len(m.joining)-1] }()
	n := r.lengthOf(o)
	total := sum(mul(n, valueSize), mul(n, stringLen(sep), unitSize))
	r.checkMemory(total)
	parts := make([]any, n)
	for i := range parts {
		e := o.Get(strconv.Itoa(i))
		if e == nil || js.IsUndefined(e) || js.IsNull(e) {
			parts[i] = ""
			continue
		}
		s := part(e)
		parts[i] = s
		total = sum(total, mul(stringLen(s), unitSize))
		r.checkMemory(total)
	}
	return r.holdString(r.callOrig(orig, r.vm.NewArray(parts...), []js.Value{sep}))
}

// localeString is e.toLocaleString() as a string, as Array.prototype.toLocaleString makes each element.
func (r *runtime) localeString(e js.Value) js.Value {
	f, ok := js.AssertFunction(r.vm.ToObject(e).Get("toLocaleString"))
	if !ok {
		panic(r.vm.NewTypeError("Property 'toLocaleString' of object %s is not a function", e))
	}
	return r.callOrig(f, e, nil).ToString()
}

// raw is String.raw, which reads the template's parts and the substitutions between them once, as the original would, checking the strings they make as they add up, and hands the original those strings to join.
func (r *runtime) raw(call js.FunctionCall, orig js.Callable) js.Value {
	raw := r.vm.ToObject(call.Argument(0)).Get("raw")
	if raw == nil {
		raw = js.Undefined()
	}
	o := r.vm.ToObject(raw)
	n := r.lengthOf(o)
	r.checkMemory(mul(n, valueSize))
	var subs []js.Value
	if len(call.Arguments) > 1 {
		subs = call.Arguments[1:]
	}
	parts := make([]any, 0, n)
	args := []js.Value{nil}
	var total int64
	for i := int64(0); i < n; i++ {
		part := o.Get(strconv.FormatInt(i, 10))
		if part == nil {
			part = js.Undefined()
		}
		s := part.ToString()
		parts = append(parts, s)
		total = sum(total, mul(stringLen(s), unitSize))
		r.checkMemory(total)
		if i+1 == n || i >= int64(len(subs)) {
			continue
		}
		s = subs[i].ToString()
		args = append(args, s)
		total = sum(total, mul(stringLen(s), unitSize))
		r.checkMemory(total)
	}
	template := r.vm.NewObject()
	_ = template.DefineDataProperty("raw", r.vm.NewArray(parts...), js.FLAG_FALSE, js.FLAG_FALSE, js.FLAG_FALSE)
	args[0] = template
	return r.holdString(r.callOrig(orig, call.This, args))
}

// jsonExtra is the most JSON.stringify writes around a value besides its key, its indentation and itself: a comma and a newline before it and its closing bracket, its brackets, the quotes and colon of its key and a space after, or the null, true or false it may be.
const jsonExtra = 16

// stringify is JSON.stringify, which checks each value as it reaches it, through a replacer of its own around the bundle's, before it writes it: how deep it lies, which past maxNesting throws a RangeError before the original goes into it, and under Limits.Memory what it writes: the value's key, the value itself when it is a string, a number or raw JSON, and the indentation of its line and of its closing bracket, which grows with its depth. A list of the properties to write, which the original would take in place of a replacer, is read here once as the original reads it, and the replacer hands the original each object to write as a proxy with just those properties, in that order, read from the object as the original would read them, by project. Raw JSON, which isRawJSON tells, is written as it is.
func (r *runtime) stringify(call js.FunctionCall, orig, isArray, isRawJSON, project js.Callable) js.Value {
	m := r.sandbox.memory
	// The levels it goes down count toward the nesting while it runs, as far as the last value it reached, so that what a getter, toJSON or the replacer calls has only the rest.
	outer := r.sandbox.nesting
	defer func() { r.sandbox.nesting = outer }()
	args := slices.Clone(call.Arguments)
	for len(args) < 3 {
		args = append(args, js.Undefined())
	}
	var user js.Callable
	var keys js.Value
	if rep, ok := args[1].(*js.Object); ok {
		if r.callOrig(isArray, js.Undefined(), []js.Value{rep}).ToBoolean() {
			keys = r.propertyList(rep)
		} else {
			user, _ = js.AssertFunction(rep)
		}
	}
	// The indentation a level, read once as the original would read it and handed to it so: at most 10 bytes.
	var gap int64
	if o, ok := args[2].(*js.Object); ok {
		switch o.ClassName() {
		case "Number":
			args[2] = o.ToNumber()
		case "String":
			args[2] = o.ToString()
		}
	}
	if s, ok := args[2].(js.String); ok {
		gap = min(int64(s.Length())*3, 10)
	} else if isNumber(args[2]) {
		if f := args[2].ToFloat(); f >= 1 {
			gap = int64(min(f, 10))
		}
	}
	// How deep each object or array written is, by the objects that hold them, the outermost 1.
	depths := map[*js.Object]int64{}
	var total int64
	args[1] = r.function(func(c js.FunctionCall) js.Value {
		key, v := c.Argument(0), c.Argument(1)
		var level int64
		if holder, ok := c.This.(*js.Object); ok {
			level = depths[holder]
		}
		r.sandbox.nesting = outer + int(level)
		if user != nil {
			v = r.callOrig(user, c.This, []js.Value{key, v})
		}
		x, ok := v.(*js.Object)
		raw := ok && r.callOrig(isRawJSON, js.Undefined(), []js.Value{x}).ToBoolean()
		if ok && !raw {
			// What the original goes into, a level deeper; raw JSON it writes as it is.
			r.nest()
			if keys != nil && r.projects(x, isArray) {
				x = r.callOrig(project, js.Undefined(), []js.Value{x, keys}).(*js.Object)
				v = x
			}
			depths[x] = level + 1
		}
		if m == nil {
			return v
		}
		n := sum(jsonExtra, mul(2, level, gap), jsonLen(key))
		switch {
		case raw:
			// Raw JSON is written as it is, its units in at most 3 bytes each.
			n = sum(n, mul(stringLen(x.Get("rawJSON")), 3))
		case isNumber(v):
			n = sum(n, int64(len(v.String())))
		default:
			n = sum(n, jsonLen(v))
		}
		total = sum(total, n)
		r.checkMemory(total)
		return v
	})
	v := r.callOrig(orig, call.This, args)
	if m == nil {
		return v
	}
	return r.holdString(v)
}

// propertyList is the list of the properties to write that JSON.stringify makes of rep, an array, as goja's makes it: its strings and numbers, and its String and Number objects as strings, each once.
func (r *runtime) propertyList(rep *js.Object) js.Value {
	n := r.lengthOf(rep)
	if r.sandbox.memory != nil {
		r.checkMemory(mul(n, valueSize))
	}
	seen := map[string]bool{}
	var names []any
	for i := int64(0); i < n; i++ {
		v := rep.Get(strconv.FormatInt(i, 10))
		var name string
		switch x := v.(type) {
		case js.String:
			name = x.String()
		case *js.Object:
			if c := x.ClassName(); c != "Number" && c != "String" {
				continue
			}
			name = x.String()
		default:
			if !isNumber(v) {
				continue
			}
			name = v.String()
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return r.vm.NewArray(names...)
}

// projects reports whether JSON.stringify writes o as an object of its properties, which a list of them selects: one that is not a function, an array, raw JSON or a primitive's wrapper.
func (r *runtime) projects(o *js.Object, isArray js.Callable) bool {
	if _, ok := js.AssertFunction(o); ok || r.callOrig(isArray, js.Undefined(), []js.Value{o}).ToBoolean() {
		return false
	}
	switch o.ClassName() {
	case "Number", "String", "Boolean", "Symbol", "BigInt":
		return false
	}
	return true
}

// isNumber reports whether v is a number.
func isNumber(v js.Value) bool {
	if _, ok := v.(*js.Object); ok || v == nil {
		return false
	}
	t := v.ExportType()
	return t != nil && (t.Kind() == reflect.Int64 || t.Kind() == reflect.Float64)
}

// jsonLen is the most bytes JSON.stringify writes v, a string, in, its quotes included: goja writes a string as UTF-8, escaping quotes, backslashes, control characters and lone surrogates; 0 for anything else.
func jsonLen(v js.Value) int64 {
	s, ok := v.(js.String)
	if !ok {
		return 0
	}
	n := int64(2)
	for i, l := 0, s.Length(); i < l; i++ {
		switch c := s.CharAt(i); {
		case c == '"' || c == '\\' || c == '\b' || c == '\t' || c == '\n' || c == '\f' || c == '\r':
			n += 2
		case c < 0x20:
			n += 6
		case c < 0x80:
			n++
		case c < 0x800:
			n += 2
		case utf16.IsSurrogate(rune(c)):
			if c < 0xDC00 && i+1 < l {
				if d := s.CharAt(i + 1); d >= 0xDC00 && d < 0xE000 {
					// A pair, one code point of 4 bytes.
					n += 4
					i++
					continue
				}
			}
			n += 6
		default:
			n += 3
		}
	}
	return n
}

// pinned is o as a built-in that reads its length and then its elements should see it, with that length, read here once: o itself when it is an array, whose length is a number of its own, or else a proxy of o that answers the length, and with iterator its iterator as nothing, as they were read here, so that a getter, a proxy or a valueOf cannot give the built-in more to allocate than was checked. It reads the rest of o, and sets it, as the built-in would.
func (r *runtime) pinned(o *js.Object, iterator bool) (js.Value, int64) {
	if o.ExportType() == arrayType {
		return o, r.lengthOf(o)
	}
	l := o.Get("length")
	if l == nil {
		l = js.Undefined()
	}
	n := lengthOf(l.ToNumber())
	if _, ok := l.(*js.Object); ok {
		l = r.vm.ToValue(n)
	}
	return r.callOrig(r.sandbox.memory.pin, js.Undefined(), []js.Value{o, l, r.vm.ToValue(iterator)}), n
}

// source is o as Array.from and the typed arrays' constructors should see it, with how many elements they make of it when it is an array-like: an array itself, an iterable one that iterates as o does but whose iterator it reads here, once, and an array-like pinned (pinned).
func (r *runtime) source(o *js.Object) (js.Value, int64) {
	if o.ExportType() == arrayType {
		return o, r.lengthOf(o)
	}
	if it := r.callOrig(r.sandbox.memory.iterate, js.Undefined(), []js.Value{o}); !js.IsUndefined(it) {
		return it, 0
	}
	return r.pinned(o, true)
}

// argumentList is o, the arguments of apply or construct, as the original would make them a list: an array itself, or else an array of o's elements read here once, as the original would read them, so that it makes a list of as many as were checked.
func (r *runtime) argumentList(o *js.Object) js.Value {
	if o.ExportType() == arrayType {
		r.checkMemory(mul(r.lengthOf(o), valueSize))
		return o
	}
	l := o.Get("length")
	if l == nil {
		l = js.Undefined()
	}
	n := lengthOf(l.ToNumber())
	// The copy and the list the original makes of it.
	r.checkMemory(mul(n, 2*valueSize))
	parts := make([]any, n)
	for i := range parts {
		e := o.Get(strconv.Itoa(i))
		if e == nil {
			e = js.Undefined()
		}
		parts[i] = e
	}
	return r.vm.NewArray(parts...)
}

// memoryProgram makes what the wrappers hand the originals in place of what the bundle gave them, of the built-ins as they were before it ran: pin, a proxy of an object o that answers its length, and its iterator when noIterator is set, as given, and reads and sets the rest of o as a built-in would; iterate, an iterable that iterates as o does, its iterator read once, or undefined when o has none; and project, a proxy with o's properties keys, in that order, as JSON.stringify writes an object with a list of them.
var memoryProgram = js.MustCompile("memory.js", `(function (Proxy, get, set, apply, iterator) {
	"use strict";
	const descriptor = () => ({ value: undefined, writable: true, enumerable: true, configurable: true });
	return {
		pin: (o, length, noIterator) => new Proxy(o, {
			__proto__: null,
			get: (t, k) => k === "length" ? length : noIterator && k === iterator ? undefined : get(t, k),
			set: (t, k, v) => set(t, k, v),
		}),
		iterate: (o) => {
			const it = o[iterator];
			return it === undefined || it === null ? undefined : { [iterator]: () => apply(it, o, []) };
		},
		project: (o, keys) => new Proxy({}, {
			__proto__: null,
			ownKeys: () => keys,
			getOwnPropertyDescriptor: descriptor,
			get: (t, k) => get(o, k),
		}),
	};
})(Proxy, Reflect.get, Reflect.set, Reflect.apply, Symbol.iterator)`, true)

var arrayType = reflect.TypeFor[[]any]()

// isTyped reports whether o is a typed array.
func isTyped(o *js.Object) bool {
	t := o.ExportType()
	return t != nil && t != arrayBufferType && t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Interface
}

// checkMemory stops the runtime when n more bytes would not fit its budget, even once what it no longer holds is collected.
func (r *runtime) checkMemory(n int64) {
	m := r.sandbox.memory
	if n > m.room() && (n > m.limit || n > m.collect()) {
		r.exceedMemory()
	}
}

// room is how many more bytes the budget holds.
func (m *memory) room() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.limit - m.used
}

// collect runs the Go collector and releases what it found unreachable, returning the room left. The collector runs when the host's heap has grown by as much as it held after the last run, which in a host that holds a lot is long after a bundle's garbage alone filled its budget, and cleanups run after it on a goroutine of their own: without this, a bundle that only makes and drops buffers would stop at its budget.
func (m *memory) collect() int64 {
	goruntime.GC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, h := range m.held {
		if !h.live() {
			m.used -= h.n
			delete(m.held, key)
		}
	}
	return m.limit - m.used
}

// exceedMemory stops the runtime at its memory quota, unwinding the built-in that would go past it.
func (r *runtime) exceedMemory() {
	err := &core.Error{Code: core.CodeQuotaExceeded, Resource: "memory",
		Message: fmt.Sprintf("tsx: the sandbox's quota of %d bytes of memory is used up", r.sandbox.memory.limit)}
	r.stop(err)
	panic(r.goError(err))
}

// hold charges n bytes for what p points to, or what n is more than it was charged before, until the collector frees it, and stops the runtime past its budget.
func hold[T any](r *runtime, p *T, n int64) {
	if p == nil || n < heldMin {
		return
	}
	m := r.sandbox.memory
	key := weak.Make(p)
	m.mu.Lock()
	had, ok := m.held[key]
	if n > had.n {
		m.used += n - had.n
		m.held[key] = holding{n: n, live: func() bool { return key.Value() != nil }}
	}
	over := m.used > m.limit
	m.mu.Unlock()
	if !ok {
		goruntime.AddCleanup(p, m.release, any(key))
	}
	if over && m.collect() < 0 {
		r.exceedMemory()
	}
}

// release frees what the allocation key held, once it is collected.
func (m *memory) release(key any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.used -= m.held[key].n
	delete(m.held, key)
}

// holdBytes charges the bytes of v, an ArrayBuffer or a typed array a built-in made, and returns it.
func (r *runtime) holdBytes(v js.Value) js.Value {
	if b := r.bytesOf(v); len(b) > 0 {
		hold(r, &b[0], int64(len(b)))
	}
	return v
}

// bytesOf is the memory of v, an ArrayBuffer or a typed array, nil for anything else.
func (r *runtime) bytesOf(v js.Value) []byte {
	o, ok := v.(*js.Object)
	if !ok {
		return nil
	}
	if o.ExportType() != arrayBufferType && !isTyped(o) {
		return nil
	}
	var b []byte
	if r.vm.ExportTo(o, &b) != nil {
		return nil
	}
	return b
}

// elementsOf is how many elements v, a typed array, has, 0 for anything else.
func (r *runtime) elementsOf(v js.Value) int64 {
	if b := r.bytesOf(v); len(b) > 0 {
		if t := v.(*js.Object).ExportType(); t != arrayBufferType {
			return int64(len(b)) / int64(t.Elem().Size())
		}
	}
	return 0
}

// holdString charges the memory of v, a string a built-in made, and returns it.
func (r *runtime) holdString(v js.Value) js.Value {
	if p, n := js.StringData(v); p != nil {
		hold(r, p, int64(n))
	}
	return v
}

// holdArray charges the values of v, an array a built-in made, and returns it.
func (r *runtime) holdArray(v js.Value) js.Value {
	if o, ok := v.(*js.Object); ok {
		hold(r, o, mul(r.lengthOf(o), valueSize))
	}
	return v
}

// lengthOf is o's length as JS's ToLength reads it.
func (r *runtime) lengthOf(o *js.Object) int64 {
	if l := o.Get("length"); l != nil {
		return lengthOf(l.ToNumber())
	}
	return 0
}

// lengthOf is a number as a length, from 0 to 2⁵³-1.
func lengthOf(v js.Value) int64 {
	switch f := v.ToFloat(); {
	case math.IsNaN(f) || f <= 0:
		return 0
	case f >= 1<<53-1:
		return 1<<53 - 1
	default:
		return int64(f)
	}
}

// relIndex is v, a number or undefined for def, as an index into n elements counted from the end when negative, as Array.prototype.fill reads it.
func relIndex(v js.Value, n, def int64) int64 {
	if js.IsUndefined(v) {
		return def
	}
	f := math.Trunc(v.ToFloat())
	switch {
	case math.IsNaN(f):
		return 0
	case f < 0:
		return int64(max(float64(n)+f, 0))
	default:
		return int64(min(f, float64(n)))
	}
}

// stringLen is the length of v, a string, in UTF-16 code units.
func stringLen(v js.Value) int64 {
	if s, ok := v.(js.String); ok {
		return int64(s.Length())
	}
	return 0
}

// mul multiplies sizes, saturating rather than overflowing.
func mul(n ...int64) int64 {
	p := int64(1)
	for _, x := range n {
		if x <= 0 {
			return 0
		}
		if p > math.MaxInt64/x {
			return math.MaxInt64
		}
		p *= x
	}
	return p
}

// sum adds sizes, saturating rather than overflowing.
func sum(n ...int64) int64 {
	s := int64(0)
	for _, x := range n {
		if s > math.MaxInt64-x {
			return math.MaxInt64
		}
		s += x
	}
	return s
}
