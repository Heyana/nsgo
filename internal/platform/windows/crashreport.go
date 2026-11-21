//go:build windows

package windows

import (
	"fmt"
	"structs"
	"sync/atomic"
	"syscall"
	"unsafe"
)

type exceptionRecord struct {
	_             structs.HostLayout
	Code          uint32
	Flags         uint32
	Next, Address uintptr
}

type exceptionPointers struct {
	_       structs.HostLayout
	Record  *exceptionRecord
	Context uintptr
}

// CaptureUnhandledExceptions saves SEH exceptions reaching Windows' top
// level filter, then passes control to the previous filter or to Windows.
func CaptureUnhandledExceptions(record func(string)) func() {
	set := kernel32.NewProc("SetUnhandledExceptionFilter")
	var previous atomic.Uintptr
	filter := syscall.NewCallback(func(p *exceptionPointers) uintptr {
		if p != nil && p.Record != nil {
			record(fmt.Sprintf("Unhandled exception 0x%08X at 0x%X\n", p.Record.Code, p.Record.Address))
		}
		if prev := previous.Load(); prev != 0 {
			result, _, _ := syscall.SyscallN(prev, uintptr(unsafe.Pointer(p)))
			return result
		}
		return 0 // EXCEPTION_CONTINUE_SEARCH
	})
	prev, _, _ := set.Call(filter)
	previous.Store(prev)
	return func() {
		if current, _, _ := set.Call(prev); current != filter {
			set.Call(current)
		}
	}
}
