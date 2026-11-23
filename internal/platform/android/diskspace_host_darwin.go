//go:build nsgoandroidhost

package android

import "syscall"

func hostFileSystemBlockSize(stats syscall.Statfs_t) int64 { return int64(stats.Bsize) }
