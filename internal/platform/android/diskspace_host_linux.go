//go:build nsgoandroidhost && !android

package android

import "syscall"

func hostFileSystemBlockSize(stats syscall.Statfs_t) int64 {
	if stats.Frsize > 0 {
		return int64(stats.Frsize)
	}
	return int64(stats.Bsize)
}
