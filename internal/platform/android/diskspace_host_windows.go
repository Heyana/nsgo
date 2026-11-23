//go:build nsgoandroidhost

package android

import (
	"math"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func hostDiskSpace(path string) (driver.DiskSpace, error) {
	if filepath.VolumeName(path) == path {
		path += string(filepath.Separator)
	}
	dir, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return driver.DiskSpace{}, err
	}
	var available, total uint64
	if err := windows.GetDiskFreeSpaceEx(dir, &available, &total, nil); err != nil {
		return driver.DiskSpace{}, err
	}
	free := int64(min(available, math.MaxInt64))
	return driver.DiskSpace{Total: int64(min(total, math.MaxInt64)), Available: free, Opportunistic: free}, nil
}
