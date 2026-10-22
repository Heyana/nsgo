//go:build windows && !arm64 && !386

package windows

import "unsafe"

// putBounds calls put_Bounds, whose RECT x64 passes by reference to a
// copy.
func putBounds(c *comObject, r rect) {
	c.call(controllerPutBounds, uintptr(unsafe.Pointer(&r)))
}
