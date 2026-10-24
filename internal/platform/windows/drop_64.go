//go:build windows && (amd64 || arm64)

package windows

import (
	"syscall"
)

// dropVtbl is IDropTarget's. Its methods take a POINTL by value, which
// amd64 and arm64 pass in one register.
var dropVtbl = [7]uintptr{
	syscall.NewCallback(dropQueryInterface),
	syscall.NewCallback(dropAddRef),
	syscall.NewCallback(dropRelease),
	syscall.NewCallback(func(t *dropTarget, data *comObject, keys, pt uintptr, effect *uint32) uintptr {
		return t.enter(data, pointOf(pt), effect)
	}),
	syscall.NewCallback(func(t *dropTarget, keys, pt uintptr, effect *uint32) uintptr {
		return t.over(pointOf(pt), effect)
	}),
	syscall.NewCallback(dropLeave),
	syscall.NewCallback(func(t *dropTarget, data *comObject, keys, pt uintptr, effect *uint32) uintptr {
		return t.drop(data, pointOf(pt), effect)
	}),
}

// pointOf unpacks a POINTL passed in one word.
func pointOf(word uintptr) point { return point{int32(uint32(word)), int32(uint32(word >> 32))} }
