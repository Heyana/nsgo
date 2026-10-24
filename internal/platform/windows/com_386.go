//go:build windows

package windows

import (
	"math"
	"syscall"
	"unsafe"
)

// callF calls method with args, the ones whose bit is set in floats being
// float32 bits: x86 takes every argument on the stack, a word each.
//
//go:uintptrescapes
func (o *comObject) callF(method int, floats uint32, args ...uintptr) uintptr {
	return o.call(method, args...)
}

// callPoint calls method with a D2D1_POINT_2F by value first, two words
// on the stack, then args.
//
//go:uintptrescapes
func (o *comObject) callPoint(method int, x, y float32, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(buf[:0], f32(x), f32(y))
	return o.call(method, append(all, args...)...)
}

// callSize calls method with a struct of two 32-bit integers by value
// first, such as D2D1_SIZE_U: two words.
//
//go:uintptrescapes
func (o *comObject) callSize(method int, a, b uint32, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(buf[:0], uintptr(a), uintptr(b))
	return o.call(method, append(all, args...)...)
}

// callRange calls method with args, then a DWRITE_TEXT_RANGE by value: two
// words.
//
//go:uintptrescapes
func (o *comObject) callRange(method int, floats uint32, start, length uint32, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(append(buf[:0], args...), uintptr(start), uintptr(length))
	return o.call(method, all...)
}

// callDouble calls method with one double: two words on the stack, low
// first.
func (o *comObject) callDouble(method int, v float64) uintptr {
	bits := math.Float64bits(v)
	return o.call(method, uintptr(uint32(bits)), uintptr(bits>>32))
}

// callRect calls method with a RECT by value: four words on the stack.
func (o *comObject) callRect(method int, r rect) uintptr {
	return o.call(method, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
}

// pointArgs returns a POINT by value as arguments: two words.
func pointArgs(x, y int32) []uintptr { return []uintptr{uintptr(x), uintptr(y)} }

// u64Args returns a 64-bit integer as arguments: two words, low first.
func u64Args(v uint64) []uintptr { return []uintptr{uintptr(uint32(v)), uintptr(v >> 32)} }

// doubleCallAddr is doublecall's address (com_386.s).
var doubleCallAddr uintptr

// doubleFrame is what doublecall reads and writes: the method, its
// object and the double it returned.
type doubleFrame struct {
	fn, this uintptr
	result   float64
}

// getDouble calls method, which takes no arguments and returns a double
// in ST(0), which SyscallN does not return: doublecall stores it.
func (o *comObject) getDouble(method int) float64 {
	f := &doubleFrame{fn: o.vtbl[method], this: uintptr(unsafe.Pointer(o))}
	syscall.SyscallN(doubleCallAddr, uintptr(unsafe.Pointer(f)))
	return f.result
}
