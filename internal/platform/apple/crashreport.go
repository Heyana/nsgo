//go:build darwin

package apple

import (
	"strings"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// CaptureUncaughtExceptions saves exceptions that reach Foundation's
// uncaught exception handler and chains to the previous handler.
func CaptureUncaughtExceptions(record func(string)) func() {
	foundation := load("/System/Library/Frameworks/Foundation.framework/Foundation")
	var get func() uintptr
	var set func(uintptr)
	purego.RegisterLibFunc(&get, foundation, "NSGetUncaughtExceptionHandler")
	purego.RegisterLibFunc(&set, foundation, "NSSetUncaughtExceptionHandler")
	previous := get()
	handler := purego.NewCallback(func(exception objc.ID) {
		var trace strings.Builder
		trace.WriteString(GoString(exception.Send(sel("name"))))
		trace.WriteString(": ")
		trace.WriteString(GoString(exception.Send(sel("reason"))))
		trace.WriteByte('\n')
		stack := exception.Send(sel("callStackSymbols"))
		for i := range objc.Send[uint](stack, selCount) {
			trace.WriteString(GoString(stack.Send(selObjectAtIndex, i)))
			trace.WriteByte('\n')
		}
		record(trace.String())
		if previous != 0 {
			purego.SyscallN(previous, uintptr(exception))
		}
	})
	set(handler)
	return func() {
		if get() == handler {
			set(previous)
		}
	}
}
