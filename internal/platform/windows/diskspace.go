//go:build windows

package windows

import (
	"fmt"
	"math"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (w *window) diskSpace(path string, count uint32) {
	space, err := volumeDiskSpace(path)
	w.engine.Dispatch(driver.DiskSpaceEvent(count, space, err))
}

func volumeDiskSpace(path string) (driver.DiskSpace, error) {
	if path == "" {
		return driver.DiskSpace{}, driver.ErrDiskSpaceUnavailable
	}
	// GetDiskFreeSpaceExW requires a trailing separator on UNC roots.
	if filepath.VolumeName(path) == path {
		path += string(filepath.Separator)
	}
	dir, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return driver.DiskSpace{}, err
	}
	var available, total uint64
	ok, _, err := getDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(dir)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), 0)
	if ok == 0 {
		return driver.DiskSpace{}, fmt.Errorf("GetDiskFreeSpaceExW: %w", err)
	}
	free := int64(min(available, math.MaxInt64))
	return driver.DiskSpace{Total: int64(min(total, math.MaxInt64)), Available: free, Opportunistic: free}, nil
}
