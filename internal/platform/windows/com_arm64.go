//go:build windows

package windows

import (
	"math"
	"syscall"
	"unsafe"
)

// floatCallAddr is floatcall's address (com_arm64.s): SyscallN calls it in
// the C convention with a *floatFrame, and it loads the frame's integer
// and float registers before jumping to the method.
var floatCallAddr uintptr

// floatFrame is what floatcall loads: the method, X0-X7 and D0-D7. Its
// layout is the offsets com_arm64.s reads.
type floatFrame struct {
	fn uintptr
	x  [8]uintptr
	d  [8]uint64
}

// callF calls method with args, the ones whose bit is set in floats being
// float32 bits. ARM64 counts integer and float registers apart, so each
// kind fills its own from the first; no method called here takes more
// than eight of either.
//
//go:uintptrescapes
func (o *comObject) callF(method int, floats uint32, args ...uintptr) uintptr {
	f := &floatFrame{fn: o.vtbl[method]}
	f.x[0] = uintptr(unsafe.Pointer(o))
	nx, nd := 1, 0
	for i, a := range args {
		if floats&(1<<i) != 0 {
			f.d[nd] = uint64(a)
			nd++
		} else {
			f.x[nx] = a
			nx++
		}
	}
	r, _, _ := syscall.SyscallN(floatCallAddr, uintptr(unsafe.Pointer(f)))
	return r
}

// callPoint calls method with a D2D1_POINT_2F by value first, which ARM64
// passes as two floats, then args.
//
//go:uintptrescapes
func (o *comObject) callPoint(method int, x, y float32, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(buf[:0], f32(x), f32(y))
	return o.callF(method, 0b11, append(all, args...)...)
}

// callSize calls method with a struct of two 32-bit integers by value
// first, such as D2D1_SIZE_U: one register.
//
//go:uintptrescapes
func (o *comObject) callSize(method int, a, b uint32, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(buf[:0], uintptr(a)|uintptr(b)<<32)
	return o.callF(method, 0, append(all, args...)...)
}

// callRange calls method with args, then a DWRITE_TEXT_RANGE by value: one
// register.
//
//go:uintptrescapes
func (o *comObject) callRange(method int, floats uint32, start, length uint32, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(append(buf[:0], args...), uintptr(start)|uintptr(length)<<32)
	return o.callF(method, floats, all...)
}

// callDouble calls method with one double, in D0.
func (o *comObject) callDouble(method int, v float64) uintptr {
	return o.callF(method, 1, uintptr(math.Float64bits(v)))
}

// callRect calls method with a RECT by value, which ARM64 passes in two
// registers.
func (o *comObject) callRect(method int, r rect) uintptr {
	words := (*[2]uintptr)(unsafe.Pointer(&r))
	return o.call(method, words[0], words[1])
}

// pointArgs returns a POINT by value as arguments: one register.
func pointArgs(x, y int32) []uintptr { return []uintptr{uintptr(uint32(x)) | uintptr(uint32(y))<<32} }

// u64Args returns a 64-bit integer as arguments: one register.
func u64Args(v uint64) []uintptr { return []uintptr{uintptr(v)} }

// doubleCallAddr is doublecall's address (com_arm64.s).
var doubleCallAddr uintptr

// doubleFrame is what doublecall reads and writes: the method, its
// object and the double it returned. Its layout is the offsets
// com_arm64.s reads.
type doubleFrame struct {
	fn, this uintptr
	result   float64
}

// getDouble calls method, which takes no arguments and returns a double
// in D0, which SyscallN does not return: doublecall stores it.
func (o *comObject) getDouble(method int) float64 {
	f := &doubleFrame{fn: o.vtbl[method], this: uintptr(unsafe.Pointer(o))}
	syscall.SyscallN(doubleCallAddr, uintptr(unsafe.Pointer(f)))
	return f.result
}
