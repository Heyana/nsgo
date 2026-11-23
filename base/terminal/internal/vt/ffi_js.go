//go:build js && !nsgominiprogram && !nsgoharmony

package vt

import (
	"crypto/rand"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"syscall/js"
	"unsafe"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/library"
)

// Supported reports whether this target can instantiate the embedded module.
const Supported = true
const wasmExport = uintptr(1 << 32)

var wasm struct {
	exports                    js.Value
	memory, array, alloc, free js.Value
	bound                      map[uintptr]js.Value
	guest, host                manifest
	functions                  map[uintptr]string
	callbacks                  []js.Func
	retained                   map[uintptr]wasmAllocation
	replies                    map[uintptr]string
	depth, calls               uint64
}

func open(path string) (handle uintptr, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("WebAssembly module: %v", failure)
		}
	}()
	bytes := library.Wasm()
	wasm.array = js.Global().Get("Uint8Array")
	array := wasm.array.New(len(bytes))
	js.CopyBytesToJS(array, bytes)
	api := js.Global().Get("WebAssembly")
	module := api.Get("Module").New(array)
	instance := api.Get("Instance").New(module)
	wasm.exports = instance.Get("exports")
	wasm.memory = wasm.exports.Get("memory")
	wasm.alloc = wasm.exports.Get("ghostty_wasm_alloc")
	wasm.free = wasm.exports.Get("ghostty_wasm_free")
	wasm.bound = map[uintptr]js.Value{}
	wasm.functions = map[uintptr]string{}
	wasm.retained = map[uintptr]wasmAllocation{}
	wasm.replies = map[uintptr]string{}
	return 1, nil
}

func sym(_ uintptr, name string) (uintptr, error) {
	function := wasm.exports.Get(name)
	if function.Type() != js.TypeFunction {
		return 0, fmt.Errorf("missing WebAssembly export %s", name)
	}
	handle := wasmExport + uintptr(len(wasm.functions))
	wasm.functions[handle] = name
	wasm.bound[handle] = function
	return handle, nil
}

//go:uintptrescapes
func call(fn uintptr, args ...uintptr) uintptr {
	wasm.depth++
	defer func() {
		wasm.depth--
		wasm.calls++
		// Wasm has no asynchronous Go preemption. Long bursts must let finalizers release JavaScript references, outside guest callbacks.
		if wasm.depth == 0 && wasm.calls >= 256 {
			wasm.calls = 0
			runtime.Gosched()
		}
	}()
	name, exported := wasm.functions[fn]
	var function js.Value
	if exported {
		function = wasm.bound[fn]
	} else {
		function = wasm.exports.Get("__indirect_function_table").Call("get", int(fn))
	}
	scope := wasmCall{}
	defer scope.free()
	values := append([]uintptr(nil), args...)
	parameters := []wasmParameter(nil)
	if exported {
		parameters = wasmParameters(name, args)
	} else if typ := wasm.replies[fn]; typ != "" {
		parameters = []wasmParameter{{1, typ, 1}}
	} else {
		panic("vt: unknown guest callback reply")
	}
	for _, param := range parameters {
		if values[param.index] == 0 {
			continue
		}
		values[param.index] = scope.marshal(values[param.index], param.typ, param.count)
	}
	jsArgs := make([]any, len(values))
	for i, value := range values {
		jsArgs[i] = int(value)
	}
	if name == "ghostty_row_get" {
		jsArgs[0] = js.Global().Get("BigInt").Invoke(strconv.FormatUint(uint64(values[0]), 10))
	}
	result := function.Invoke(jsArgs...)
	scope.copyBack()
	if name == "ghostty_key_event_set_utf8" {
		scope.retain(args[0])
	}
	if name == "ghostty_key_event_free" {
		empty := wasmCall{}
		empty.retain(args[0])
	}
	if result.IsUndefined() {
		return 0
	}
	return uintptr(uint32(result.Int()))
}

func registerFunc(pointer any, fn uintptr) {
	switch f := pointer.(type) {
	case *func(uintptr, scrollViewport):
		*f = func(t uintptr, value scrollViewport) { call(fn, t, uintptr(unsafe.Pointer(&value))) }
	case *func(uintptr, point, *GridRef) int32:
		*f = func(t uintptr, value point, out *GridRef) int32 {
			return int32(call(fn, t, uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(out))))
		}
	case *func(uintptr, mousePosition):
		*f = func(event uintptr, value mousePosition) { call(fn, event, uintptr(unsafe.Pointer(&value))) }
	case *func(uintptr, uintptr, selectionFormatOptions, *uintptr, *uintptr) int32:
		*f = func(t, alloc uintptr, value selectionFormatOptions, out, n *uintptr) int32 {
			return int32(call(fn, t, alloc, uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(out)), uintptr(unsafe.Pointer(n))))
		}
	case *func(uintptr, *uintptr, uintptr, formatterOptions) int32:
		*f = func(alloc uintptr, out *uintptr, t uintptr, value formatterOptions) int32 {
			return int32(call(fn, alloc, uintptr(unsafe.Pointer(out)), t, uintptr(unsafe.Pointer(&value))))
		}
	default:
		panic(fmt.Sprintf("vt: no WebAssembly function adapter for %T", pointer))
	}
}

// newCallback installs a typed Wasm trampoline in the guest's function table. A JavaScript function alone is not a valid table entry.
func newCallback(signature string, callback any) uintptr {
	noRead := signature == "mime-reader"
	if noRead {
		signature = "iiii"
	} // userdata, indirect String, indirect Writer
	fn := reflect.ValueOf(callback)
	wrapped := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if noRead {
			return 0
		}
		values := make([]reflect.Value, len(args))
		for i, arg := range args {
			values[i] = reflect.ValueOf(uintptr(uint32(arg.Int())))
		}
		result := fn.Call(values)
		if signature[0] == 'v' {
			return nil
		}
		return int(result[0].Uint())
	})
	wasm.callbacks = append(wasm.callbacks, wrapped)
	bytes := callbackModule(signature)
	array := wasm.array.New(len(bytes))
	js.CopyBytesToJS(array, bytes)
	api := js.Global().Get("WebAssembly")
	module := api.Get("Module").New(array)
	imports := map[string]any{"env": map[string]any{"call": wrapped}}
	instance := api.Get("Instance").New(module, imports)
	table := wasm.exports.Get("__indirect_function_table")
	index := table.Call("grow", 1).Int()
	table.Call("set", index, instance.Get("exports").Get("callback"))
	return uintptr(index)
}

func callbackModule(signature string) []byte {
	section := func(kind byte, body []byte) []byte { return append([]byte{kind, byte(len(body))}, body...) }
	params := len(signature) - 1
	types := []byte{1, 0x60, byte(params)}
	for range params {
		types = append(types, 0x7f)
	}
	if signature[0] == 'v' {
		types = append(types, 0)
	} else {
		types = append(types, 1, 0x7f)
	}
	imports := []byte{1, 3, 'e', 'n', 'v', 4, 'c', 'a', 'l', 'l', 0, 0}
	body := []byte{0}
	for i := range params {
		body = append(body, 0x20, byte(i))
	}
	body = append(body, 0x10, 0, 0x0b)
	module := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	module = append(module, section(1, types)...)
	module = append(module, section(2, imports)...)
	module = append(module, section(3, []byte{1, 0})...)
	module = append(module, section(7, []byte{1, 8, 'c', 'a', 'l', 'l', 'b', 'a', 'c', 'k', 0, 1})...)
	module = append(module, section(10, append([]byte{1, byte(len(body))}, body...))...)
	return module
}

// Browser modules have no OS entropy source. Supply the C API's secure random effect through Go's browser crypto adapter, never through Math.random.
func setupPlatform() {
	callback := newCallback("iiii", func(_, data, n uintptr) uintptr {
		bytes := make([]byte, n)
		if _, err := rand.Read(bytes); err != nil {
			return 0
		}
		writeForeignBytes(data, bytes)
		return 1
	})
	call(fnSysSet, 3 /* GHOSTTY_SYS_OPT_RANDOM_SECURE */, callback)
}
