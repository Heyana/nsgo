//go:build !js

package js

import (
	"time"

	"github.com/nsgo-dev/goja"
)

// Runtime is a JavaScript realm backed by Goja.
type Runtime struct {
	vm *goja.Runtime
}

func New() *Runtime { return &Runtime{vm: goja.New()} }

func (r *Runtime) RunProgram(p *Program) (Value, error) {
	value, err := r.vm.RunProgram((*goja.Program)(p))
	return wrapValue(value), wrapError(err)
}
func (r *Runtime) RunScript(name, source string) (Value, error) {
	value, err := r.vm.RunScript(name, source)
	return wrapValue(value), wrapError(err)
}
func (r *Runtime) RunString(source string) (Value, error) { return r.RunScript("", source) }

func (r *Runtime) GlobalObject() *Object { return (*Object)(r.vm.GlobalObject()) }
func (r *Runtime) Get(name string) Value {
	defer wrapPanic()
	return wrapValue(r.vm.Get(name))
}
func (r *Runtime) Set(name string, value any) error {
	return wrapError(r.vm.Set(name, rawGoValue(value)))
}
func (r *Runtime) NewObject() *Object { return (*Object)(r.vm.NewObject()) }
func (r *Runtime) NewArray(items ...any) *Object {
	values := make([]any, len(items))
	for i, item := range items {
		values[i] = rawGoValue(item)
	}
	return (*Object)(r.vm.NewArray(values...))
}
func (r *Runtime) NewArrayBuffer(data []byte) ArrayBuffer {
	return ArrayBuffer(r.vm.NewArrayBuffer(data))
}
func (r *Runtime) NewPromise() (*Promise, func(any) error, func(any) error) {
	p, resolve, reject := r.vm.NewPromise()
	return (*Promise)(p), func(value any) error {
			return wrapError(resolve(rawGoValue(value)))
		}, func(value any) error {
			return wrapError(reject(rawGoValue(value)))
		}
}
func (r *Runtime) NewTypeError(format string, args ...any) *Object {
	return (*Object)(r.vm.NewTypeError(append([]any{format}, args...)...))
}
func (r *Runtime) NewGoError(err error) *Object { return (*Object)(r.vm.NewGoError(err)) }
func (r *Runtime) New(constructor Value, args ...Value) (*Object, error) {
	o, err := r.vm.New(rawValue(constructor), rawValues(args)...)
	return (*Object)(o), wrapError(err)
}

func (r *Runtime) ToValue(value any) Value {
	defer wrapPanic()
	return wrapValue(r.vm.ToValue(rawGoValue(value)))
}

// ToObject boxes v using the realm's prototypes and returns objects unchanged.
func (r *Runtime) ToObject(v Value) *Object {
	defer wrapPanic()
	return (*Object)(rawValue(v).ToObject(r.vm))
}

func (r *Runtime) ExportTo(v Value, target any) error {
	switch out := target.(type) {
	case *Value:
		*out = v
		return nil
	case **Object:
		*out = nil
		if v != nil && !IsUndefined(v) && !IsNull(v) {
			*out = r.ToObject(v)
		}
		return nil
	case *ArrayBuffer:
		return wrapError(r.vm.ExportTo(rawValue(v), (*goja.ArrayBuffer)(out)))
	}
	return wrapError(r.vm.ExportTo(rawValue(v), target))
}

// UncapFieldNameMapper lowercases the first letter of Go fields and methods.
func UncapFieldNameMapper() FieldNameMapper { return goja.UncapFieldNameMapper() }

func (r *Runtime) SetFieldNameMapper(mapper FieldNameMapper) {
	r.vm.SetFieldNameMapper(mapper)
}
func (r *Runtime) SetTimeSource(now func() time.Time) { r.vm.SetTimeSource(now) }
func (r *Runtime) SetMaxCallStackSize(size int)       { r.vm.SetMaxCallStackSize(size) }
func (r *Runtime) SetAsyncContextTracker(tracker AsyncContextTracker) {
	r.vm.SetAsyncContextTracker(tracker)
}

func (r *Runtime) SetPromiseJobFilter(filter func(any) bool) { r.vm.SetPromiseJobFilter(filter) }
func (r *Runtime) DrainPromiseJobs() error                   { return wrapError(r.vm.DrainPromiseJobs()) }
func (r *Runtime) Interrupt(value any)                       { r.vm.Interrupt(value) }
func (r *Runtime) ClearInterrupt()                           { r.vm.ClearInterrupt() }

// Close has no native resources to release on Goja.
func (*Runtime) Close() {}
