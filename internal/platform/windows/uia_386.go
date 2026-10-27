//go:build windows && 386

package windows

import (
	"syscall"
	"unsafe"
)

// On 386 the doubles come on the stack, two words each.

func fromPointSlot() uintptr {
	return syscall.NewCallback(func(i *uiaIface, xlo, xhi, ylo, yhi uintptr, out *unsafe.Pointer) uintptr {
		return uiaFromPointWords(i, uintptr(uint64(xhi)<<32|uint64(xlo)), uintptr(uint64(yhi)<<32|uint64(ylo)), out)
	})
}

func setRangeSlot() uintptr {
	return syscall.NewCallback(func(i *uiaIface, lo, hi uintptr) uintptr {
		return uiaSetRangeWords(i, uintptr(uint64(hi)<<32|uint64(lo)))
	})
}
