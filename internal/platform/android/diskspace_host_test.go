//go:build nsgoandroidhost && !android

package android

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestHostDiskSpace(t *testing.T) {
	dir := t.TempDir()
	space, err := hostDiskSpace(dir)
	if err != nil || space.Total <= 0 || space.Available < 0 || space.Available > space.Total || space.Opportunistic != space.Available {
		t.Fatalf("host disk space: %+v, %v", space, err)
	}
	if _, err := hostDiskSpace(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("disk space for a missing directory: %v", err)
	}
}
