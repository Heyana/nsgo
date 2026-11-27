//go:build android && arm64

package purego

import (
	"errors"
	"runtime"
	"unsafe"
)

// Bionic exports the dynamic-loader API from libdl.so without versioned sonames; these dynamic imports keep the arm64 Android executable cgo-free while the assembly stubs expose symbol addresses for SyscallN.
//go:cgo_import_dynamic purego_android_dlopen_import dlopen "libdl.so"
//go:cgo_import_dynamic purego_android_dlsym_import dlsym "libdl.so"
//go:cgo_import_dynamic purego_android_dlerror_import dlerror "libdl.so"
//go:cgo_import_dynamic purego_android_dlclose_import dlclose "libdl.so"
//go:cgo_import_dynamic _ _ "libdl.so"

//go:linkname androidDlopen purego_android_dlopen
var androidDlopen uint8

//go:linkname androidDlsym purego_android_dlsym
var androidDlsym uint8

//go:linkname androidDlerror purego_android_dlerror
var androidDlerror uint8

//go:linkname androidDlclose purego_android_dlclose
var androidDlclose uint8

func dlopenPath(path string, mode int) (uintptr, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var arg uintptr
	var p []byte
	if path != "" {
		p = append([]byte(path), 0)
		arg = uintptr(unsafe.Pointer(&p[0]))
	}
	r, _, _ := SyscallN(uintptr(unsafe.Pointer(&androidDlopen)), arg, uintptr(mode))
	runtime.KeepAlive(p)
	if r == 0 {
		return 0, dlerrorString()
	}
	return r, nil
}

func dlsymName(handle uintptr, name string) (uintptr, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := append([]byte(name), 0)
	r, _, _ := SyscallN(uintptr(unsafe.Pointer(&androidDlsym)), handle, uintptr(unsafe.Pointer(&p[0])))
	runtime.KeepAlive(p)
	if r == 0 {
		return 0, dlerrorString()
	}
	return r, nil
}

func dlcloseHandle(handle uintptr) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
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
