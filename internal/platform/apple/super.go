//go:build darwin

package apple

import (
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
	ownClasses    = map[objc.Class]bool{}
	msgSendSuper2 uintptr
)

type objcSuper struct {
	_        structs.HostLayout
	receiver objc.ID
	class    objc.Class
}

// RegisterClass registers an Objective-C class whose methods may call
// SendSuper.
func RegisterClass(name string, super objc.Class, protocols []*objc.Protocol, methods []objc.MethodDef) objc.Class {
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

// ownClass returns the nearest class of self's that NSGo registered.
func ownClass(self objc.ID) objc.Class {
	c := self.Class()
	for k := c; k != 0; k = k.SuperClass() {
		if ownClasses[k] {
			return k
		}
	}
	return c
}

// SendSuper sends cmd to the superclass of the NSGo class self is an
// instance of, whatever subclass the runtime gave self since.
func SendSuper[T any](self objc.ID, cmd objc.SEL, args ...any) T {
	if msgSendSuper2 == 0 {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		if msgSendSuper2, err = purego.Dlsym(lib, "objc_msgSendSuper2"); err != nil {
			panic(err)
		}
	}
	var fn func(super *objcSuper, cmd objc.SEL, args ...any) T
	purego.RegisterFunc(&fn, msgSendSuper2)
	return fn(&objcSuper{receiver: self, class: ownClass(self)}, cmd, args...)
}

// Super sends cmd to self's NSGo superclass, discarding the result.
func Super(self objc.ID, cmd objc.SEL, args ...any) { SendSuper[objc.ID](self, cmd, args...) }

// Subviews returns a view's subviews, back to front.
func Subviews(view objc.ID) []objc.ID {
	arr := view.Send(objc.RegisterName("subviews"))
	n := objc.Send[uint](arr, selCount)
	out := make([]objc.ID, n)
	for i := range n {
		out[i] = arr.Send(selObjectAtIndex, i)
	}
	return out
}
