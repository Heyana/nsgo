//go:build android && arm64

package vt

import "github.com/nsgo-dev/nsgo/internal/purego"

// Supported reports whether the platform can load libghostty-vt.
const Supported = true

// call calls a C function whose arguments are integers or pointers. Go
// pointers converted in the call are moved to the heap and kept alive until
// it returns (go:uintptrescapes), so stack growth cannot move them while the
// library uses them.
//
//go:uintptrescapes
func call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, args...)
	return r
}

func registerFunc(fptr any, addr uintptr)  { purego.RegisterFunc(fptr, addr) }
func newCallback(_ string, fn any) uintptr { return purego.NewCallback(fn) }
func setupPlatform()                       {}
