//go:build android && arm64 && android && !cgo

package purego

import (
	"errors"
	"unsafe"
)

// bionic exports the dynamic-loader API from libdl.so. These imports keep the
// arm64 Android binary cgo-free; the assembly stubs below turn the imported
// symbols into addresses that SyscallN can call.
//go:cgo_import_dynamic purego_android_dlopen_import dlopen "libdl.so"
//go:cgo_import_dynamic purego_android_dlsym_import dlsym "libdl.so"
//go:cgo_import_dynamic purego_android_dlerror_import dlerror "libdl.so"
//go:cgo_import_dynamic purego_android_dlclose_import dlclose "libdl.so"

//go:linkname androidDlopen purego_android_dlopen
var androidDlopen uint8

//go:linkname androidDlsym purego_android_dlsym
var androidDlsym uint8

//go:linkname androidDlerror purego_android_dlerror
var androidDlerror uint8

//go:linkname androidDlclose purego_android_dlclose
var androidDlclose uint8

func dlopenPath(path string, mode int) (uintptr, error) {
	p := append([]byte(path), 0)
	r, _, _ := SyscallN(uintptr(unsafe.Pointer(&androidDlopen)), uintptr(unsafe.Pointer(&p[0])), uintptr(mode))
	if r == 0 {
		return 0, dlerrorString()
	}
	return r, nil
}

func dlsymName(handle uintptr, name string) (uintptr, error) {
	p := append([]byte(name), 0)
	r, _, _ := SyscallN(uintptr(unsafe.Pointer(&androidDlsym)), handle, uintptr(unsafe.Pointer(&p[0])))
	if r == 0 {
		return 0, dlerrorString()
	}
	return r, nil
}

func dlcloseHandle(handle uintptr) error {
	r, _, _ := SyscallN(uintptr(unsafe.Pointer(&androidDlclose)), handle)
	if r != 0 {
		return dlerrorString()
	}
	return nil
}

//go:nocheckptr
func androidPointer(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}

func dlerrorString() error {
	r, _, _ := SyscallN(uintptr(unsafe.Pointer(&androidDlerror)))
	if r == 0 {
		return errors.New("bionic dynamic-loader error")
	}
	p := unsafe.Slice((*byte)(androidPointer(r)), 1<<20)
	n := 0
	for n < len(p) && p[n] != 0 {
		n++
	}
	return errors.New(string(p[:n]))
}
