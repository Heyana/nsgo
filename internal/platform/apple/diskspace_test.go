//go:build darwin && !ios

package apple

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestVolumeDiskSpace(t *testing.T) {
	space, err := volumeDiskSpace(t.TempDir(), true)
	if err != nil || space.Total <= 0 || space.Available < 0 || space.Opportunistic < 0 {
		t.Fatalf("native disk space: %+v, %v", space, err)
	}
}

func TestDiskSpaceRequiresDeclaredReason(t *testing.T) {
	DiskSpace(t.TempDir(), 42, true, func(ev driver.Event) {
		if result := ev.Data.(driver.DiskSpaceResult); ev.Count != 42 || !errors.Is(result.Err, driver.ErrDiskSpaceNotDeclared) {
			t.Fatalf("undeclared disk space: %+v", ev)
		}
	})
}
