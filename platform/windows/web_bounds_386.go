//go:build windows

package windows

// putBounds calls put_Bounds, whose RECT x86 passes by value on the stack.
func putBounds(c *comObject, r rect) {
	c.call(controllerPutBounds, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
}
