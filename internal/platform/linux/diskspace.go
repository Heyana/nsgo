//go:build linux && !android

package linux

import (
	"fmt"
	"math"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// statvfs begins with these unsigned longs on Linux's 64-bit ABIs. The
// remaining space holds the fields we do not read and libc's reserved fields.
type volumeStats struct {
	blockSize, fragmentSize, blocks, free, available uint64
	_                                                [9]uint64
}

var volumeStat = sync.OnceValue(func() func(string, *volumeStats) int32 {
	symbol, err := purego.Dlsym(purego.RTLD_DEFAULT, "statvfs")
	if err != nil {
		return nil
	}
	var stat func(string, *volumeStats) int32
	purego.RegisterFunc(&stat, symbol)
	return stat
})

func (w *window) diskSpace(path string, count uint32) {
	space, err := volumeDiskSpace(path)
	w.engine.Dispatch(driver.DiskSpaceEvent(count, space, err))
}

func volumeDiskSpace(path string) (driver.DiskSpace, error) {
	stat := volumeStat()
	if path == "" || stat == nil {
		return driver.DiskSpace{}, driver.ErrDiskSpaceUnavailable
	}
	var info volumeStats
	if stat(path, &info) != 0 {
		return driver.DiskSpace{}, fmt.Errorf("statvfs %q: could not read volume capacity", path)
	}
	total := bytesInBlocks(info.blocks, info.fragmentSize)
	free := bytesInBlocks(info.available, info.fragmentSize)
	return driver.DiskSpace{Total: total, Available: free, Opportunistic: free}, nil
}

func bytesInBlocks(blocks, size uint64) int64 {
	if size != 0 && blocks > math.MaxInt64/size {
		return math.MaxInt64
	}
	return int64(blocks * size)
}
