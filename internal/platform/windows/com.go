//go:build windows

package windows

// COM through syscall: an interface pointer points to its vtable, and a
// method is the vtable's entry called with the pointer first.
//
// SyscallN passes words, so floats and small structs need care, and each
// architecture differs (Go's internal/runtime/syscall/windows asm):
//
//   - amd64 also copies the first four words to XMM0-3 and pushes the
//     rest, so a float32 passed as its bits lands where the callee reads
//     it, and a struct of eight bytes or less goes in one word.
//   - 386 pushes every word: a float32 is one, a struct of two 32-bit
//     fields two.
//   - arm64 loads only X0-X7, while floats go in the V registers, counted
//     apart from the integers. callF goes through floatcall (com_arm64.s),
//     which loads both; a struct of two floats is two float arguments, one
//     of two 32-bit integers a word.
//
// Methods taking larger structs take pointers to them on every
// architecture, which is what this backend prefers.

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// guid is a COM interface or class ID.
type guid struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

// parseGUID parses "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"; it is only
// given constants.
func parseGUID(s string) guid {
	hex := strings.ReplaceAll(s, "-", "")
	word := func(from, to int) uint64 {
		n, err := strconv.ParseUint(hex[from:to], 16, 64)
		if err != nil {
			panic("bad GUID " + s)
		}
		return n
	}
	g := guid{a: uint32(word(0, 8)), b: uint16(word(8, 12)), c: uint16(word(12, 16))}
	for i := range g.d {
		g.d[i] = byte(word(16+2*i, 18+2*i))
	}
	return g
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

func hresultError(what string, hr uintptr) error {
	return errors.New(what + ": HRESULT 0x" + strconv.FormatUint(uint64(uint32(hr)), 16))
}

// comObject is what a COM interface pointer points to: its vtable first.
// The array's length only bounds the indexes; Direct2D's device context
// has the longest vtable used here.
type comObject struct{ vtbl *[128]uintptr }

// call calls method on o with integer and pointer arguments. They may be
// Go pointers converted to uintptr, which the directive keeps on the heap
// during the call.
//
//go:uintptrescapes
func (o *comObject) call(method int, args ...uintptr) uintptr {
	var buf [12]uintptr
	all := append(buf[:0], uintptr(unsafe.Pointer(o)))
	r, _, _ := syscall.SyscallN(o.vtbl[method], append(all, args...)...)
	return r
}

func (o *comObject) release() {
	if o != nil {
		o.call(comRelease)
	}
}

func (o *comObject) addRef() {
	if o != nil {
		o.call(comAddRef)
	}
}

// query returns o's interface iid, nil if it has none.
func (o *comObject) query(iid *guid) *comObject {
	var out *comObject
	if failed(o.call(comQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))) {
		return nil
	}
	return out
}

// ptr returns o as an argument.
func (o *comObject) ptr() uintptr { return uintptr(unsafe.Pointer(o)) }

// f32 returns v as a float argument: the bits of a float32, which callF
// is told about.
func f32(v float32) uintptr { return uintptr(math.Float32bits(v)) }

// release releases each object and sets it to nil.
func releaseAll(objects ...**comObject) {
	for _, o := range objects {
		(*o).release()
		*o = nil
	}
}
