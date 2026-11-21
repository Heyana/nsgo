// Package js defines the JavaScript API TSX shares between Goja and the browser.
package js

import "reflect"

// Value is a JavaScript primitive or object. A nil Value denotes a missing
// property, distinct from the JavaScript values null and undefined.
type Value interface {
	Export() any
	ExportType() reflect.Type
	ToInteger() int64
	ToFloat() float64
	ToNumber() Value
	ToString() Value
	ToBoolean() bool
	String() string
	StrictEquals(Value) bool
}

// objectAPI is the object method set both engines expose.
type objectAPI interface {
	Value
	Get(string) Value
	Set(string, any) error
	Keys() []string
	GetSymbol(*Symbol) Value
	SetSymbol(*Symbol, any) error
	DefineDataProperty(string, Value, Flag, Flag, Flag) error
	DefineDataPropertySymbol(*Symbol, Value, Flag, Flag, Flag) error
	DefineAccessorProperty(string, Value, Value, Flag, Flag) error
	Prototype() *Object
	SetPrototype(*Object) error
	ClassName() string
	MarshalJSON() ([]byte, error)
}

var (
	_ objectAPI = (*Object)(nil)
	_ Value     = (*Symbol)(nil)
)

// Callable is a JS function as Go calls it.
type Callable func(this Value, args ...Value) (Value, error)

// Constructor is a JS constructor as Go calls it with new.
type Constructor func(newTarget *Object, args ...Value) (*Object, error)

// Flag is a property attribute's setting.
type Flag int

const (
	FLAG_NOT_SET Flag = iota
	FLAG_FALSE
	FLAG_TRUE
)

// FunctionCall is a call of a Go function from JS.
type FunctionCall struct {
	This      Value
	Arguments []Value
}

// Argument returns argument i, undefined when missing.
func (f FunctionCall) Argument(i int) Value {
	if i < len(f.Arguments) {
		return f.Arguments[i]
	}
	return Undefined()
}

// ConstructorCall is a call of a Go constructor with new.
type ConstructorCall struct {
	This      *Object
	Arguments []Value
	NewTarget *Object
}

// Argument returns argument i, undefined when missing.
func (f ConstructorCall) Argument(i int) Value {
	if i < len(f.Arguments) {
		return f.Arguments[i]
	}
	return Undefined()
}

// PromiseState is a promise's state.
type PromiseState int

const (
	PromiseStatePending PromiseState = iota
	PromiseStateFulfilled
	PromiseStateRejected
)
