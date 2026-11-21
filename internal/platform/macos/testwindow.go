//go:build darwin && !ios

package macos

import (
	"runtime"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

var classTestWindow objc.Class

// unconstrainedTestWindow returns the proposed frame through NSInvocation so
// the Objective-C runtime handles CGRect's different return ABIs on Apple
// silicon and Intel. Normal application windows still use AppKit's constraint.
func unconstrainedTestWindow() objc.Class {
	if classTestWindow != 0 {
		return classTestWindow
	}
	lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW)
	if err != nil {
		panic(err)
	}
	forward := "_objc_msgForward"
	if runtime.GOARCH == "amd64" {
		forward = "_objc_msgForward_stret"
	}
	imp, err := purego.Dlsym(lib, forward)
	if err != nil {
		panic(err)
	}
	constrain := sel("constrainFrameRect:toScreen:")
	// NsgoWindow's inherited callbacks must continue computing their super
	// calls from NsgoWindow, so register this subclass directly with purego.
	class, err := objc.RegisterClass("NsgoTestWindow", classWindow, nil, nil, []objc.MethodDef{{
		Cmd: sel("forwardInvocation:"),
		Fn: func(self objc.ID, cmd objc.SEL, call objc.ID) {
			if objc.Send[objc.SEL](call, sel("selector")) != constrain {
				apple.Super(self, cmd, call)
				return
			}
			var frame apple.CGRect
			call.Send(sel("getArgument:atIndex:"), &frame, 2)
			call.Send(sel("setReturnValue:"), &frame)
		},
	}})
	if err != nil {
		panic(err)
	}
	var instanceMethod func(objc.Class, objc.SEL) uintptr
	var methodEncoding func(uintptr) string
	purego.RegisterLibFunc(&instanceMethod, lib, "class_getInstanceMethod")
	purego.RegisterLibFunc(&methodEncoding, lib, "method_getTypeEncoding")
	encoding := methodEncoding(instanceMethod(classWindow, constrain))
	if !class.AddMethod(constrain, objc.IMP(imp), encoding) {
		panic("macos: cannot configure test window frame constraint")
	}
	classTestWindow = class
	return class
}
