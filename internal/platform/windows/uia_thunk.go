//go:build windows && (amd64 || arm64)

package windows

import "syscall"

// The thunks' targets, read by uia_*.s, and their addresses.
var (
	uiaFromPointTarget    = syscall.NewCallback(uiaFromPointWords)
	uiaSetRangeTarget     = syscall.NewCallback(uiaSetRangeWords)
	uiaFromPointThunkAddr uintptr
	uiaSetRangeThunkAddr  uintptr
)

func fromPointSlot() uintptr { return uiaFromPointThunkAddr }
func setRangeSlot() uintptr  { return uiaSetRangeThunkAddr }
