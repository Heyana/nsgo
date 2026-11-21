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

func (r *Runtime) RunProgram(p *Program) (Value, error) { return r.vm.RunProgram(p) }
func (r *Runtime) RunScript(name, source string) (Value, error) {
	return r.vm.RunScript(name, source)
}
func (r *Runtime) RunString(source string) (Value, error) { return r.vm.RunString(source) }

func (r *Runtime) GlobalObject() *Object            { return r.vm.GlobalObject() }
func (r *Runtime) Get(name string) Value            { return r.vm.Get(name) }
func (r *Runtime) Set(name string, value any) error { return r.vm.Set(name, value) }
func (r *Runtime) NewObject() *Object               { return r.vm.NewObject() }
func (r *Runtime) NewArray(items ...any) *Object    { return r.vm.NewArray(items...) }
func (r *Runtime) NewArrayBuffer(data []byte) ArrayBuffer {
	return r.vm.NewArrayBuffer(data)
}
func (r *Runtime) NewPromise() (*Promise, func(any) error, func(any) error) {
	return r.vm.NewPromise()
}
func (r *Runtime) NewTypeError(format string, args ...any) *Object {
	return r.vm.NewTypeError(append([]any{format}, args...)...)
}
func (r *Runtime) NewGoError(err error) *Object { return r.vm.NewGoError(err) }
func (r *Runtime) New(constructor Value, args ...Value) (*Object, error) {
	return r.vm.New(constructor, args...)
}

func (r *Runtime) ToValue(value any) Value { return r.vm.ToValue(value) }

// ToObject boxes v using the realm's prototypes and returns objects unchanged.
func (r *Runtime) ToObject(v Value) *Object { return v.ToObject(r.vm) }

func (r *Runtime) ExportTo(v Value, target any) error { return r.vm.ExportTo(v, target) }

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
func (r *Runtime) DrainPromiseJobs() error                   { return r.vm.DrainPromiseJobs() }
func (r *Runtime) Interrupt(value any)                       { r.vm.Interrupt(value) }
func (r *Runtime) ClearInterrupt()                           { r.vm.ClearInterrupt() }

// Close has no native resources to release on Goja.
func (*Runtime) Close() {}
