//go:build windows && (amd64 || 386)

package windows

import "syscall"

// outlinerVtbl is IDWriteTextRenderer's. On amd64 and 386 a float
// argument still takes its place among the words the callback reads,
// which this one leaves unread.
var outlinerVtbl = [10]uintptr{
	syscall.NewCallback(outlinerQueryInterface),
	syscall.NewCallback(outlinerAddRef),
	syscall.NewCallback(outlinerRelease),
	syscall.NewCallback(outlinerSnapping),
	syscall.NewCallback(outlinerTransform),
	syscall.NewCallback(outlinerPixelsPerDip),
	syscall.NewCallback(func(o *outliner, ctx, x, y, mode uintptr, run *glyphRun, desc *glyphRunDescription, effect uintptr) uintptr {
		return o.glyphRun(run, desc)
	}),
	syscall.NewCallback(func(o *outliner, ctx, x, y, line, effect uintptr) uintptr { return sOK }),
	syscall.NewCallback(func(o *outliner, ctx, x, y, line, effect uintptr) uintptr { return sOK }),
	syscall.NewCallback(func(o *outliner, ctx, x, y, inline, sideways, rtl, effect uintptr) uintptr { return sOK }),
}
