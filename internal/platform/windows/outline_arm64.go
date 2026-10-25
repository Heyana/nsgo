//go:build windows && arm64

package windows

import "syscall"

// outlinerVtbl is IDWriteTextRenderer's. arm64 passes floats in registers
// of their own, so the callbacks' words skip them.
var outlinerVtbl = [10]uintptr{
	syscall.NewCallback(outlinerQueryInterface),
	syscall.NewCallback(outlinerAddRef),
	syscall.NewCallback(outlinerRelease),
	syscall.NewCallback(outlinerSnapping),
	syscall.NewCallback(outlinerTransform),
	syscall.NewCallback(outlinerPixelsPerDip),
	syscall.NewCallback(func(o *outliner, ctx, mode uintptr, run *glyphRun, desc *glyphRunDescription, effect uintptr) uintptr {
		return o.glyphRun(run, desc)
	}),
	syscall.NewCallback(func(o *outliner, ctx, line, effect uintptr) uintptr { return sOK }),
	syscall.NewCallback(func(o *outliner, ctx, line, effect uintptr) uintptr { return sOK }),
	syscall.NewCallback(func(o *outliner, ctx, inline, sideways, rtl, effect uintptr) uintptr { return sOK }),
}
