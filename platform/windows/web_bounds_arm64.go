//go:build windows

package windows

import "unsafe"

// putBounds calls put_Bounds, whose RECT arm64 passes by value in two
// registers.
func putBounds(c *comObject, r rect) {
	words := (*[2]uintptr)(unsafe.Pointer(&r))
	c.call(controllerPutBounds, words[0], words[1])
}
