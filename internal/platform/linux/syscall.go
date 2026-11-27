//go:build linux && !android

package linux

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// bindSyscall resolves name in lib and assigns a zero-reflection purego.SyscallN
// wrapper to fn. Only functions whose arguments and return value are pointers,
// integers or booleans (and at most 6 arguments, all in general-purpose
// registers on amd64 and arm64) may use it; functions taking or returning floats
// stay on purego.RegisterLibFunc.
func bindSyscall(fn any, lib uintptr, name string) {
	sym, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(err)
	}
	switch dst := fn.(type) {
	case *func() uintptr:
		*dst = func() uintptr {
			r, _, _ := purego.SyscallN(sym)
			return r
		}
	case *func() int64:
		*dst = func() int64 {
			r, _, _ := purego.SyscallN(sym)
			return int64(r)
		}
	case *func(uintptr):
		*dst = func(a uintptr) {
			purego.SyscallN(sym, a)
		}
	case *func(uintptr) uintptr:
		*dst = func(a uintptr) uintptr {
			r, _, _ := purego.SyscallN(sym, a)
			return r
		}
	case *func(uintptr) int32:
		*dst = func(a uintptr) int32 {
			r, _, _ := purego.SyscallN(sym, a)
			return int32(r)
		}
	case *func(uintptr) int64:
		*dst = func(a uintptr) int64 {
			r, _, _ := purego.SyscallN(sym, a)
			return int64(r)
		}
	case *func(uintptr, uintptr):
		*dst = func(a, b uintptr) {
			purego.SyscallN(sym, a, b)
		}
	case *func(uintptr, uintptr) uintptr:
		*dst = func(a, b uintptr) uintptr {
			r, _, _ := purego.SyscallN(sym, a, b)
			return r
		}
	case *func(uintptr, uintptr, uintptr):
		*dst = func(a, b, c uintptr) {
			purego.SyscallN(sym, a, b, c)
		}
	case *func(uintptr, uintptr, uintptr) uintptr:
		*dst = func(a, b, c uintptr) uintptr {
			r, _, _ := purego.SyscallN(sym, a, b, c)
			return r
		}
	case *func(uintptr, int32):
		*dst = func(a uintptr, b int32) {
			purego.SyscallN(sym, a, uintptr(b))
		}
	case *func(uintptr, bool):
		*dst = func(a uintptr, b bool) {
			purego.SyscallN(sym, a, uintptr(boolInt(b)))
		}
	case *func(int32) uintptr:
		*dst = func(a int32) uintptr {
			r, _, _ := purego.SyscallN(sym, uintptr(a))
			return r
		}
	case *func(uint16, uint16, uint16) uintptr:
		*dst = func(a, b, c uint16) uintptr {
			r, _, _ := purego.SyscallN(sym, uintptr(a), uintptr(b), uintptr(c))
			return r
		}
	case *func(*byte) uintptr:
		*dst = func(a *byte) uintptr {
			r, _, _ := purego.SyscallN(sym, uintptr(unsafe.Pointer(a)))
			runtime.KeepAlive(a)
			return r
		}
	case *func(uintptr, *byte, int32):
		*dst = func(a uintptr, b *byte, c int32) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)), uintptr(c))
			runtime.KeepAlive(b)
		}
	case *func(uintptr, uintptr, int32):
		*dst = func(a, b uintptr, c int32) {
			purego.SyscallN(sym, a, b, uintptr(c))
		}
	case *func(uintptr, int32, int32, int32, uintptr):
		*dst = func(a uintptr, b, c, d int32, e uintptr) {
			purego.SyscallN(sym, a, uintptr(b), uintptr(c), uintptr(d), e)
		}
	case *func(uintptr, *int32, *int32):
		*dst = func(a uintptr, b, c *int32) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(c)))
			runtime.KeepAlive(b)
			runtime.KeepAlive(c)
		}
	case *func(uintptr, *graphenePoint) uintptr:
		*dst = func(a uintptr, b *graphenePoint) uintptr {
			r, _, _ := purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)))
			runtime.KeepAlive(b)
			return r
		}
	case *func(uintptr, *graphenePoint):
		*dst = func(a uintptr, b *graphenePoint) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)))
			runtime.KeepAlive(b)
		}
	case *func(uintptr, *grapheneRect):
		*dst = func(a uintptr, b *grapheneRect) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)))
			runtime.KeepAlive(b)
		}
	case *func(uintptr, *grapheneRect) uintptr:
		*dst = func(a uintptr, b *grapheneRect) uintptr {
			r, _, _ := purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)))
			runtime.KeepAlive(b)
			return r
		}
	case *func(uintptr, *gskRoundedRect):
		*dst = func(a uintptr, b *gskRoundedRect) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)))
			runtime.KeepAlive(b)
		}
	case *func(uintptr, *gdkRGBA, *grapheneRect):
		*dst = func(a uintptr, b *gdkRGBA, c *grapheneRect) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(c)))
			runtime.KeepAlive(b)
			runtime.KeepAlive(c)
		}
	case *func(uintptr, *gskRoundedRect, *[4]float32, *[4]gdkRGBA):
		*dst = func(a uintptr, b *gskRoundedRect, c *[4]float32, d *[4]gdkRGBA) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(d)))
			runtime.KeepAlive(b)
			runtime.KeepAlive(c)
			runtime.KeepAlive(d)
		}
	case *func(uintptr, uintptr, *gdkRGBA):
		*dst = func(a, b uintptr, c *gdkRGBA) {
			purego.SyscallN(sym, a, b, uintptr(unsafe.Pointer(c)))
			runtime.KeepAlive(c)
		}
	case *func(uintptr, uintptr, *gdkRGBA, *graphenePoint) uintptr:
		*dst = func(a, b uintptr, c *gdkRGBA, d *graphenePoint) uintptr {
			r, _, _ := purego.SyscallN(sym, a, b, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(d)))
			runtime.KeepAlive(c)
			runtime.KeepAlive(d)
			return r
		}
	case *func(uintptr, uintptr, *grapheneRect):
		*dst = func(a, b uintptr, c *grapheneRect) {
			purego.SyscallN(sym, a, b, uintptr(unsafe.Pointer(c)))
			runtime.KeepAlive(c)
		}
	case *func(uintptr, uintptr, int32, *grapheneRect):
		*dst = func(a, b uintptr, c int32, d *grapheneRect) {
			purego.SyscallN(sym, a, b, uintptr(c), uintptr(unsafe.Pointer(d)))
			runtime.KeepAlive(d)
		}
	case *func(uintptr, *pangoRectangle, *pangoRectangle):
		*dst = func(a uintptr, b, c *pangoRectangle) {
			purego.SyscallN(sym, a, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(c)))
			runtime.KeepAlive(b)
			runtime.KeepAlive(c)
		}
	case *func(uintptr, int32, *int32, *gValue):
		*dst = func(a uintptr, b int32, c *int32, d *gValue) {
			purego.SyscallN(sym, a, uintptr(b), uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(d)))
			runtime.KeepAlive(c)
			runtime.KeepAlive(d)
		}
	case *func(*gValue, *byte):
		*dst = func(a *gValue, b *byte) {
			purego.SyscallN(sym, uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)))
			runtime.KeepAlive(a)
			runtime.KeepAlive(b)
		}
	case *func(*gValue, int32):
		*dst = func(a *gValue, b int32) {
			purego.SyscallN(sym, uintptr(unsafe.Pointer(a)), uintptr(b))
			runtime.KeepAlive(a)
		}
	case *func(uintptr, unsafe.Pointer, unsafe.Pointer):
		*dst = func(a uintptr, b, c unsafe.Pointer) {
			purego.SyscallN(sym, a, uintptr(b), uintptr(c))
			runtime.KeepAlive(b)
			runtime.KeepAlive(c)
		}
	case *func(uintptr, unsafe.Pointer) uintptr:
		*dst = func(a uintptr, b unsafe.Pointer) uintptr {
			r, _, _ := purego.SyscallN(sym, a, uintptr(b))
			runtime.KeepAlive(b)
			return r
		}
	default:
		panic(fmt.Sprintf("linux: bindSyscall unsupported signature %T for %s", fn, name))
	}
}

// Static NUL-terminated C strings for hot-path GTK/GObject calls.
var (
	cstrNotifyHasFocus = cstr("notify::has-focus")
	cstrNsgoViewName   = cstr("NsgoView")
	cstrEmptyByte      byte
)

// pangoLayoutSetTextStr sets layout's text without allocating a NUL-terminated
// copy: pango_layout_set_text reads exactly len(text) bytes when length >= 0.
func pangoLayoutSetTextStr(layout uintptr, text string) {
	if len(text) == 0 {
		pangoLayoutSetText(layout, &cstrEmptyByte, 0)
		return
	}
	pangoLayoutSetText(layout, unsafe.StringData(text), int32(len(text)))
	runtime.KeepAlive(text)
}
