//go:build windows

package windows

import (
	"math"
	"syscall"
	"unsafe"
)

// callF calls method with args, the ones whose bit is set in floats being
// float32 bits: SyscallN already puts them where x64 callees read them.
//
//go:uintptrescapes
func (o *comObject) callF(method int, floats uint32, args ...uintptr) uintptr {
	return o.call(method, args...)
}

// callPoint calls method with a D2D1_POINT_2F by value first, which x64
// passes in one register, then args.
//
//go:uintptrescapes
func (o *comObject) callPoint(method int, x, y float32, args ...uintptr) uintptr {
	var buf [8]uintptr
	all := append(buf[:0], f32(x)|f32(y)<<32)
	return o.call(method, append(all, args...)...)
}

// callSize calls method with a struct of two 32-bit integers by value
// first, such as D2D1_SIZE_U: one register.
//
//go:uintptrescapes
func (o *comObject) callSize(method int, a, b uint32, args ...uintptr) uintptr {
	var buf [8]uintptr
	all := append(buf[:0], uintptr(a)|uintptr(b)<<32)
	return o.call(method, append(all, args...)...)
}

// callRange calls method with args, then a DWRITE_TEXT_RANGE by value: one
// register.
//
//go:uintptrescapes
func (o *comObject) callRange(method int, floats uint32, start, length uint32, args ...uintptr) uintptr {
	var buf [8]uintptr
	all := append(append(buf[:0], args...), uintptr(start)|uintptr(length)<<32)
	return o.call(method, all...)
}

// callDouble calls method with one double: its bits, which SyscallN also
// puts in XMM1.
func (o *comObject) callDouble(method int, v float64) uintptr {
	return o.call(method, uintptr(math.Float64bits(v)))
}

// callRect calls method with a RECT by value, which x64 passes by
// reference to a copy.
func (o *comObject) callRect(method int, r rect) uintptr {
	return o.call(method, uintptr(unsafe.Pointer(&r)))
}

// pointArgs returns a POINT by value as arguments: one word.
func pointArgs(x, y int32) []uintptr { return []uintptr{uintptr(uint32(x)) | uintptr(uint32(y))<<32} }

// u64Args returns a 64-bit integer as arguments: one word.
func u64Args(v uint64) []uintptr { return []uintptr{uintptr(v)} }

// doubleCallAddr is doublecall's address (com_amd64.s).
var doubleCallAddr uintptr

// doubleFrame is what doublecall reads and writes: the method, its
// object and the double it returned.
type doubleFrame struct {
	fn, this uintptr
	result   float64
}

// getDouble calls method, which takes no arguments and returns a double
// in XMM0, which SyscallN does not return: doublecall stores it.
func (o *comObject) getDouble(method int) float64 {
	f := &doubleFrame{fn: o.vtbl[method], this: uintptr(unsafe.Pointer(o))}
	syscall.SyscallN(doubleCallAddr, uintptr(unsafe.Pointer(f)))
	return f.result
}
