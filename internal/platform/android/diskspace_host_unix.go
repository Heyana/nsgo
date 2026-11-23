//go:build nsgoandroidhost && (darwin || (linux && !android))

package android

import (
	"math"
	"syscall"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func hostDiskSpace(path string) (driver.DiskSpace, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return driver.DiskSpace{}, err
	}
	blockSize := uint64(hostFileSystemBlockSize(stats))
	bytes := func(blocks uint64) int64 {
		if blockSize != 0 && blocks > math.MaxInt64/blockSize {
			return math.MaxInt64
		}
		return int64(blocks * blockSize)
	}
	free := bytes(stats.Bavail)
	return driver.DiskSpace{Total: bytes(stats.Blocks), Available: free, Opportunistic: free}, nil
}
