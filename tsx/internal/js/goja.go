//go:build !js

// Package js is the JavaScript engine package tsx runs apps on: goja on
// every target but the web, where the page's own engine runs them
// (browser.go). Its API is goja's, so on goja it is goja's types under
// other names, and the browser's implements the part tsx uses.
package js

import (
	"reflect"
	"unsafe"

	"github.com/nsgo-dev/goja"
)

type (
	Value              = goja.Value
	Object             = goja.Object
	Runtime            = goja.Runtime
	FunctionCall       = goja.FunctionCall
	ConstructorCall    = goja.ConstructorCall
	Exception          = goja.Exception
	Promise            = goja.Promise
	PromiseState       = goja.PromiseState
	ArrayBuffer        = goja.ArrayBuffer
	Symbol             = goja.Symbol
	Callable           = goja.Callable
	Program            = goja.Program
	InterruptedError   = goja.InterruptedError
	StackOverflowError = goja.StackOverflowError
	String             = goja.String
)

const (
	PromiseStatePending   = goja.PromiseStatePending
	PromiseStateFulfilled = goja.PromiseStateFulfilled
	PromiseStateRejected  = goja.PromiseStateRejected

	FLAG_TRUE  = goja.FLAG_TRUE
	FLAG_FALSE = goja.FLAG_FALSE
)

var engineProperties = EngineProperties{
	CanInterrupt:    true,
	CanLimitRegexps: true,
}

var (
	New                  = goja.New
	Undefined            = goja.Undefined
	Null                 = goja.Null
	IsUndefined          = goja.IsUndefined
	IsNull               = goja.IsNull
	AssertFunction       = goja.AssertFunction
	AssertConstructor    = goja.AssertConstructor
	NewSymbol            = goja.NewSymbol
	MustCompile          = goja.MustCompile
	UncapFieldNameMapper = goja.UncapFieldNameMapper
)

// Caller is the line of JS code r runs now, through the source maps the code carries, as file and line; ok is false while r runs none.
func Caller(r *Runtime) (file string, line int, ok bool) {
	for _, f := range r.CaptureCallStack(16, nil) {
		if p := f.Position(); p.Filename != "" && p.Line > 0 {
			return p.Filename, p.Line, true
		}
	}
	return "", 0, false
}

// StringData is the memory goja keeps s, a string it made, in: where it starts and its size in bytes. goja keeps an ASCII string as a Go string, a byte a unit, and others as UTF-16 with a leading BOM, or, as JSON.stringify returns them, as a Go string it reads as UTF-16 once it needs to, keeping both, which starts where its record of the two does; a concatenation it has not read yet, which keeps its parts, and a value that is no string have none (nil).
func StringData(s Value) (*byte, int) {
	switch v := reflect.ValueOf(s); v.Kind() {
	case reflect.String:
		if str := v.String(); str != "" {
			return unsafe.StringData(str), len(str)
		}
	case reflect.Slice:
		if v.Len() > 0 {
			return (*byte)(v.UnsafePointer()), v.Len() * 2
		}
	case reflect.Pointer:
		if e := v.Elem(); e.Kind() == reflect.Struct {
			if str := e.FieldByName("s"); str.Kind() == reflect.String && str.Len() > 0 {
				return (*byte)(v.UnsafePointer()), str.Len() + 2*s.(String).Length()
			}
		}
	}
	return nil, 0
}
