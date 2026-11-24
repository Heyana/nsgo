package windowsinstaller

import (
	"reflect"
	"runtime"
	"syscall"
	"unsafe"
)

// Native calls retain and pin pointer arguments until the API returns. Unlike a uintptr helper, this also keeps temporary UTF-16 values and out parameters visible to Go's collector while native callbacks reenter Go.
type nativeDLL struct{ dll *syscall.LazyDLL }
type nativeProc struct{ proc *syscall.LazyProc }

func loadLibrary(name string) nativeDLL            { return nativeDLL{syscall.NewLazyDLL(name)} }
func (d nativeDLL) NewProc(name string) nativeProc { return nativeProc{d.dll.NewProc(name)} }
func ptr[T any](p *T) unsafe.Pointer               { return unsafe.Pointer(p) }
func nativeWord(value any) uintptr {
	if value == nil {
		return 0
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uintptr(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintptr(v.Uint())
	case reflect.UnsafePointer, reflect.Pointer:
		return v.Pointer()
	}
	panic("invalid native installer argument")
}
func nativeArgs(args []any) ([]uintptr, *runtime.Pinner) {
	values := make([]uintptr, len(args))
	pins := new(runtime.Pinner)
	for i, arg := range args {
		if pointer, ok := arg.(unsafe.Pointer); ok && pointer != nil {
			pins.Pin(pointer)
		}
		values[i] = nativeWord(arg)
	}
	return values, pins
}
func (p nativeProc) Call(args ...any) (uintptr, uintptr, error) {
	values, pins := nativeArgs(args)
	defer pins.Unpin()
	a, b, err := p.proc.Call(values...)
	runtime.KeepAlive(args)
	return a, b, err
}
func callAddress(address uintptr, args ...any) (uintptr, uintptr, error) {
	values, pins := nativeArgs(args)
	defer pins.Unpin()
	a, b, err := syscall.SyscallN(address, values...)
	runtime.KeepAlive(args)
	return a, b, err
}
