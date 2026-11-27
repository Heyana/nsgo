//go:build darwin

package apple

import (
	"reflect"
	"runtime"
	"structs"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Classes NSGo registers call their superclass's methods through
// SendSuper. purego's objc.SendSuper starts from the object's own class,
// which KVO may have replaced with a subclass of NSGo's at run time; the
// call then lands in NSGo's method again and recurses until the stack
// runs out.

var (
	ownClasses         = map[objc.Class]bool{}
	msgSendSuper2      uintptr
	msgSendSuper2Stret uintptr
	objectGetClass     uintptr
	superPoint         func(super *objcSuper, cmd objc.SEL, x, y float64) objc.ID
	selSubviews        = sel("subviews")
)

type objcSuper struct {
	_        structs.HostLayout
	receiver objc.ID
	class    objc.Class
}

// loadSuper loads what a send to a superclass needs, at the first one.
func loadSuper() {
	if msgSendSuper2 != 0 {
		return
	}
	lib := load("/usr/lib/libobjc.A.dylib")
	objectGetClass = symbol(lib, "object_getClass")
	send := symbol(lib, "objc_msgSendSuper2")
	if runtime.GOARCH == "amd64" {
		msgSendSuper2Stret = symbol(lib, "objc_msgSendSuper2_stret")
	}
	purego.RegisterFunc(&superPoint, send)
	msgSendSuper2 = send
}

// RegisterClass registers an Objective-C class whose methods may call
// SendSuper. A selector defined twice panics naming it: the runtime refuses the second method with only its index, and the class is registered at launch, so every app would fail to start.
func RegisterClass(name string, super objc.Class, protocols []*objc.Protocol, methods []objc.MethodDef) objc.Class {
	if cmd, ok := duplicateSelector(methods); ok {
		var selName func(objc.SEL) string
		purego.RegisterLibFunc(&selName, load("/usr/lib/libobjc.A.dylib"), "sel_getName")
		panic("apple: class " + name + " defines " + selName(cmd) + " twice")
	}
	if protocols == nil {
		protocols = []*objc.Protocol{}
	}
	c, err := objc.RegisterClass(name, super, protocols, []objc.FieldDef{}, methods)
	if err != nil {
		panic(err)
	}
	ownClasses[c] = true
	return c
}

// duplicateSelector returns a selector methods define more than once.
func duplicateSelector(methods []objc.MethodDef) (objc.SEL, bool) {
	seen := make(map[objc.SEL]bool, len(methods))
	for _, m := range methods {
		if seen[m.Cmd] {
			return m.Cmd, true
		}
		seen[m.Cmd] = true
	}
	return 0, false
}

// ClassOf returns the runtime class of self without purego reflection.
func ClassOf(self objc.ID) objc.Class {
	c, _, _ := purego.SyscallN(objectGetClass, uintptr(self))
	return objc.Class(c)
}

// ownClass returns the nearest class of self's that NSGo registered.
func ownClass(self objc.ID) objc.Class {
	for k := ClassOf(self); k != 0; k = k.SuperClass() {
		if ownClasses[k] {
			return k
		}
	}
	return ClassOf(self)
}

// SendSuper sends cmd to the superclass of the NSGo class self is an
// instance of, whatever subclass the runtime gave self since.
func SendSuper[T any](self objc.ID, cmd objc.SEL, args ...any) T {
	loadSuper()
	var fn func(super *objcSuper, cmd objc.SEL, args ...any) T
	send := msgSendSuper2
	if runtime.GOARCH == "amd64" {
		if t := reflect.TypeFor[T](); t.Kind() == reflect.Struct && t.Size() > 16 {
			send = msgSendSuper2Stret
		}
	}
	purego.RegisterFunc(&fn, send)
	return fn(&objcSuper{receiver: self, class: ownClass(self)}, cmd, args...)
}

// SendSuperPoint is SendSuper for a method that takes one point and
// returns an object, as hitTest: does. SendSuper registers a function and
// works the point's layout out with reflection at every call, several
// times what the method costs, and a hit test calls it for every view
// under the pointer. This sends through one function registered once,
// with the point as the two doubles every 64-bit Apple target passes it
// as.
func SendSuperPoint(self objc.ID, cmd objc.SEL, p CGPoint) objc.ID {
	loadSuper()
	return superPoint(&objcSuper{receiver: self, class: ownClass(self)}, cmd, p.X, p.Y)
}

// Super sends cmd to self's NSGo superclass, discarding the result.
func Super(self objc.ID, cmd objc.SEL, args ...any) { SendSuper[objc.ID](self, cmd, args...) }

// Subviews returns a view's subviews, back to front.
func Subviews(view objc.ID) []objc.ID {
	arr := objc.ID(Msg(view, selSubviews))
	n := uint(Msg(arr, selCount))
	out := make([]objc.ID, n)
	for i := range n {
		out[i] = objc.ID(Msg(arr, selObjectAtIndex, uintptr(i)))
	}
	return out
}
