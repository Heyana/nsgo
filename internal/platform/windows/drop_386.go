//go:build windows && 386

package windows

import "syscall"

// dropVtbl is IDropTarget's. Its methods take a POINTL by value, which
// 386 pushes as two words.
var dropVtbl = [7]uintptr{
	syscall.NewCallback(dropQueryInterface),
	syscall.NewCallback(dropAddRef),
	syscall.NewCallback(dropRelease),
	syscall.NewCallback(func(t *dropTarget, data *comObject, keys, x, y uintptr, effect *uint32) uintptr {
		return t.enter(data, point{int32(x), int32(y)}, effect)
	}),
	syscall.NewCallback(func(t *dropTarget, keys, x, y uintptr, effect *uint32) uintptr {
		return t.over(point{int32(x), int32(y)}, effect)
	}),
	syscall.NewCallback(dropLeave),
	syscall.NewCallback(func(t *dropTarget, data *comObject, keys, x, y uintptr, effect *uint32) uintptr {
		return t.drop(data, point{int32(x), int32(y)}, effect)
	}),
}
