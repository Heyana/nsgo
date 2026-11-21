//go:build linux && !android

package linux

import "testing"

func TestVolumeDiskSpace(t *testing.T) {
	space, err := volumeDiskSpace(t.TempDir())
	if err != nil || space.Total <= 0 || space.Available < 0 || space.Available > space.Total || space.Opportunistic != space.Available {
		t.Fatalf("native disk space: %+v, %v", space, err)
	}
}
