//go:build !js

package js

import (
	"io"
	"reflect"

	"github.com/nsgo-dev/goja"
)

// Object has Goja's object identity and only the portable object methods.
type Object goja.Object

// Symbol is a JavaScript symbol.
type Symbol goja.Symbol

// String exposes the UTF-16 units counted by the Goja memory limiter.
type String interface {
	Value
	Length() int
	CharAt(int) uint16
	Reader() io.RuneReader
}

type primitive struct{ raw goja.Value }
type nativeString struct{ primitive }

func (s nativeString) Length() int           { return s.raw.(goja.String).Length() }
func (s nativeString) CharAt(i int) uint16   { return s.raw.(goja.String).CharAt(i) }
func (s nativeString) Reader() io.RuneReader { return s.raw.(goja.String).Reader() }

func wrapValue(v goja.Value) Value {
	switch v := v.(type) {
	case nil:
		return nil
	case *goja.Object:
		return (*Object)(v)
	case *goja.Symbol:
		return (*Symbol)(v)
	case goja.String:
		return nativeString{primitive{v}}
	default:
		return primitive{v}
	}
}

func rawValue(v Value) goja.Value {
	if v == nil {
		return nil
	}
	return v.(interface{ gojaValue() goja.Value }).gojaValue()
}

func wrapValues(vs []goja.Value) []Value {
	out := make([]Value, len(vs))
	for i, v := range vs {
		out[i] = wrapValue(v)
	}
	return out
}

func rawValues(vs []Value) []goja.Value {
	out := make([]goja.Value, len(vs))
	for i, v := range vs {
		out[i] = rawValue(v)
	}
	return out
}

func Undefined() Value           { return wrapValue(goja.Undefined()) }
func Null() Value                { return wrapValue(goja.Null()) }
func IsUndefined(v Value) bool   { return goja.IsUndefined(rawValue(v)) }
func IsNull(v Value) bool        { return goja.IsNull(rawValue(v)) }
func NewSymbol(s string) *Symbol { return (*Symbol)(goja.NewSymbol(s)) }

func (v primitive) gojaValue() goja.Value    { return v.raw }
func (v primitive) Export() any              { return v.raw.Export() }
func (v primitive) ExportType() reflect.Type { return v.raw.ExportType() }
func (v primitive) ToInteger() int64         { return v.raw.ToInteger() }
func (v primitive) ToFloat() float64         { return v.raw.ToFloat() }
func (v primitive) ToString() Value {
	return wrapValue(v.raw.ToString())
}
func (v primitive) ToNumber() Value {
	return wrapValue(v.raw.ToNumber())
}
func (v primitive) ToBoolean() bool           { return v.raw.ToBoolean() }
func (v primitive) String() string            { return v.raw.String() }
func (v primitive) StrictEquals(o Value) bool { return v.raw.StrictEquals(rawValue(o)) }

func (s *Symbol) gojaValue() goja.Value    { return (*goja.Symbol)(s) }
func (s *Symbol) Export() any              { return (*goja.Symbol)(s).Export() }
func (s *Symbol) ExportType() reflect.Type { return (*goja.Symbol)(s).ExportType() }
func (s *Symbol) ToInteger() int64 {
	defer wrapPanic()
	return (*goja.Symbol)(s).ToInteger()
}
func (s *Symbol) ToFloat() float64 {
	defer wrapPanic()
	return (*goja.Symbol)(s).ToFloat()
}
func (s *Symbol) ToString() Value {
	defer wrapPanic()
	return wrapValue((*goja.Symbol)(s).ToString())
}
func (s *Symbol) ToNumber() Value {
	defer wrapPanic()
	return wrapValue((*goja.Symbol)(s).ToNumber())
}
func (s *Symbol) ToBoolean() bool           { return (*goja.Symbol)(s).ToBoolean() }
func (s *Symbol) String() string            { return (*goja.Symbol)(s).String() }
func (s *Symbol) StrictEquals(v Value) bool { return (*goja.Symbol)(s).StrictEquals(rawValue(v)) }

func (o *Object) gojaValue() goja.Value {
	if o == nil {
		return nil
	}
	return (*goja.Object)(o)
}

func (o *Object) ToInteger() int64 {
	defer wrapPanic()
	return (*goja.Object)(o).ToInteger()
}
func (o *Object) ToFloat() float64 {
	defer wrapPanic()
	return (*goja.Object)(o).ToFloat()
}
func (o *Object) ToString() Value {
	defer wrapPanic()
	return wrapValue((*goja.Object)(o).ToString())
}
func (o *Object) ToNumber() Value {
	defer wrapPanic()
	return wrapValue((*goja.Object)(o).ToNumber())
}
func (o *Object) ToBoolean() bool { return true }
func (o *Object) String() string {
	defer wrapPanic()
	return (*goja.Object)(o).String()
}
func (o *Object) StrictEquals(v Value) bool { return (*goja.Object)(o).StrictEquals(rawValue(v)) }

func (o *Object) Export() any {
	defer wrapPanic()
	switch v := (*goja.Object)(o).Export().(type) {
	case *goja.Promise:
		return (*Promise)(v)
	case goja.ArrayBuffer:
		return ArrayBuffer(v)
	case func(goja.FunctionCall) goja.Value:
		return func(c FunctionCall) Value {
			defer wrapPanic()
			return wrapValue(v(goja.FunctionCall{This: rawValue(c.This), Arguments: rawValues(c.Arguments)}))
		}
	default:
		return v
	}
}
func (o *Object) ExportType() reflect.Type {
	defer wrapPanic()
	switch t := (*goja.Object)(o).ExportType(); t {
	case reflect.TypeFor[*goja.Promise]():
		return reflect.TypeFor[*Promise]()
	case reflect.TypeFor[goja.ArrayBuffer]():
		return reflect.TypeFor[ArrayBuffer]()
	case reflect.TypeFor[func(goja.FunctionCall) goja.Value]():
		return reflect.TypeFor[func(FunctionCall) Value]()
	default:
		return t
	}
}

func (o *Object) Get(name string) Value {
	defer wrapPanic()
	return wrapValue((*goja.Object)(o).Get(name))
}
func (o *Object) GetSymbol(s *Symbol) Value {
	defer wrapPanic()
	return wrapValue((*goja.Object)(o).GetSymbol((*goja.Symbol)(s)))
}
func (o *Object) Set(name string, value any) error {
	return wrapError((*goja.Object)(o).Set(name, rawGoValue(value)))
}
func (o *Object) SetSymbol(s *Symbol, value any) error {
	return wrapError((*goja.Object)(o).SetSymbol((*goja.Symbol)(s), rawGoValue(value)))
}
func (o *Object) DefineDataProperty(name string, value Value, writable, configurable, enumerable Flag) error {
	return wrapError((*goja.Object)(o).DefineDataProperty(name, rawValue(value), goja.Flag(writable), goja.Flag(configurable), goja.Flag(enumerable)))
}
func (o *Object) DefineDataPropertySymbol(s *Symbol, value Value, writable, configurable, enumerable Flag) error {
	return wrapError((*goja.Object)(o).DefineDataPropertySymbol((*goja.Symbol)(s), rawValue(value), goja.Flag(writable), goja.Flag(configurable), goja.Flag(enumerable)))
}
func (o *Object) DefineAccessorProperty(name string, getter, setter Value, configurable, enumerable Flag) error {
	return wrapError((*goja.Object)(o).DefineAccessorProperty(name, rawValue(getter), rawValue(setter), goja.Flag(configurable), goja.Flag(enumerable)))
}
func (o *Object) Keys() []string {
	defer wrapPanic()
	return (*goja.Object)(o).Keys()
}
func (o *Object) Prototype() *Object {
	defer wrapPanic()
	return (*Object)((*goja.Object)(o).Prototype())
}
func (o *Object) SetPrototype(proto *Object) error {
	return wrapError((*goja.Object)(o).SetPrototype((*goja.Object)(proto)))
}
func (o *Object) ClassName() string { return (*goja.Object)(o).ClassName() }
func (o *Object) MarshalJSON() ([]byte, error) {
	data, err := (*goja.Object)(o).MarshalJSON()
	return data, wrapError(err)
}

// Promise and ArrayBuffer keep their native backing objects.
type Promise goja.Promise
type ArrayBuffer goja.ArrayBuffer

func (p *Promise) State() PromiseState { return PromiseState((*goja.Promise)(p).State()) }
func (p *Promise) Result() Value       { return wrapValue((*goja.Promise)(p).Result()) }
func (a ArrayBuffer) Bytes() []byte    { return goja.ArrayBuffer(a).Bytes() }

// Exception is the value Goja threw, with its original stack and Go cause.
type Exception goja.Exception

func (e *Exception) Value() Value   { return wrapValue((*goja.Exception)(e).Value()) }
func (e *Exception) Error() string  { return (*goja.Exception)(e).Error() }
func (e *Exception) String() string { return (*goja.Exception)(e).String() }
func (e *Exception) Unwrap() error  { return (*goja.Exception)(e).Unwrap() }

// InterruptedError and StackOverflowError stop a Goja call without
// exposing Goja's exception or stack-frame types.
type InterruptedError goja.InterruptedError
type StackOverflowError goja.StackOverflowError

func (e *InterruptedError) Error() string  { return (*goja.InterruptedError)(e).Error() }
func (e *InterruptedError) String() string { return (*goja.InterruptedError)(e).String() }
func (e *InterruptedError) Unwrap() error  { return (*goja.InterruptedError)(e).Unwrap() }
func (e *InterruptedError) CallError() error {
	return wrapError((*goja.InterruptedError)(e).CallError())
}

func (e *StackOverflowError) Error() string  { return (*goja.StackOverflowError)(e).Error() }
func (e *StackOverflowError) String() string { return (*goja.StackOverflowError)(e).String() }
func (e *StackOverflowError) Unwrap() error  { return (*goja.StackOverflowError)(e).Unwrap() }

// Goja's errors cross into TSX without changing the engine's exception data.
func wrapError(err error) error {
	switch e := err.(type) {
	case *goja.Exception:
		return (*Exception)(e)
	case *goja.InterruptedError:
		return (*InterruptedError)(e)
	case *goja.StackOverflowError:
		return (*StackOverflowError)(e)
	default:
		return err
	}
}

func wrapPanic() {
	if p := recover(); p != nil {
		if e, ok := p.(error); ok {
			p = wrapError(e)
		}
		panic(p)
	}
}

// Restore Goja's exception or value when a TSX callback throws into JS.
func unwrapPanic() {
	if p := recover(); p != nil {
		switch v := p.(type) {
		case *Exception:
			p = (*goja.Exception)(v)
		case *InterruptedError:
			p = (*goja.InterruptedError)(v)
		case *StackOverflowError:
			p = (*goja.StackOverflowError)(v)
		case Value:
			p = rawValue(v)
		}
		panic(p)
	}
}

func AssertFunction(v Value) (Callable, bool) {
	f, ok := goja.AssertFunction(rawValue(v))
	if !ok {
		return nil, false
	}
	return func(this Value, args ...Value) (Value, error) {
		val, err := f(rawValue(this), rawValues(args)...)
		return wrapValue(val), wrapError(err)
	}, true
}

func AssertConstructor(v Value) (Constructor, bool) {
	f, ok := goja.AssertConstructor(rawValue(v))
	if !ok {
		return nil, false
	}
	return func(target *Object, args ...Value) (*Object, error) {
		o, err := f((*goja.Object)(target), rawValues(args)...)
		return (*Object)(o), wrapError(err)
	}, true
}

func rawGoValue(v any) any {
	switch v := v.(type) {
	case Value:
		return rawValue(v)
	case *Promise:
		return (*goja.Promise)(v)
	case ArrayBuffer:
		return goja.ArrayBuffer(v)
	case *ArrayBuffer:
		return (*goja.ArrayBuffer)(v)
	case func(FunctionCall) Value:
		return func(c goja.FunctionCall) goja.Value {
			defer unwrapPanic()
			return rawValue(v(FunctionCall{This: wrapValue(c.This), Arguments: wrapValues(c.Arguments)}))
		}
	case func(ConstructorCall) *Object:
		return func(c goja.ConstructorCall) *goja.Object {
			defer unwrapPanic()
			return (*goja.Object)(v(ConstructorCall{This: (*Object)(c.This),
				Arguments: wrapValues(c.Arguments), NewTarget: (*Object)(c.NewTarget)}))
		}
	default:
		return v
	}
}
