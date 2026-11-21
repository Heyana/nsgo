package js

import (
	"reflect"
	"time"
)

// runtimeAPI is the complete method set TSX shares between the adapters.
// Keep engine-specific setup in their build-tagged files.
type runtimeAPI interface {
	RunProgram(*Program) (Value, error)
	RunScript(name, source string) (Value, error)
	RunString(source string) (Value, error)
	GlobalObject() *Object
	Get(name string) Value
	Set(name string, value any) error
	NewObject() *Object
	NewArray(items ...any) *Object
	NewArrayBuffer([]byte) ArrayBuffer
	NewPromise() (*Promise, func(any) error, func(any) error)
	NewTypeError(format string, args ...any) *Object
	NewGoError(error) *Object
	New(constructor Value, args ...Value) (*Object, error)
	ToValue(any) Value
	ToObject(Value) *Object
	ExportTo(Value, any) error
	SetFieldNameMapper(FieldNameMapper)
	SetTimeSource(func() time.Time)
	SetMaxCallStackSize(int)
	SetAsyncContextTracker(AsyncContextTracker)
	SetPromiseJobFilter(func(any) bool)
	DrainPromiseJobs() error
	Interrupt(any)
	ClearInterrupt()
	Close()
}

var _ runtimeAPI = (*Runtime)(nil)

// FieldNameMapper names fields and methods when a Go value enters JS.
type FieldNameMapper interface {
	FieldName(reflect.Type, reflect.StructField) string
	MethodName(reflect.Type, reflect.Method) string
}

// AsyncContextTracker captures and restores context around promise reactions.
type AsyncContextTracker interface {
	Grab() any
	Resumed(any)
	Exited()
}
