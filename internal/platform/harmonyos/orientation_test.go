//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestOrientationSendsMask checks that ItemOrientation reaches Index.ets as an "or" operation with the mask in o[2], which applyOrientation turns into the window's preferred orientation, zero restoring the usual policy.
func TestOrientationSendsMask(t *testing.T) {
	for _, mask := range []driver.Orientation{driver.OrientationLandscape, driver.OrientationPortrait | driver.OrientationLandscapeRight, 0} {
		m := &mounter{views: map[driver.NodeID]*view{}}
		m.item(&driver.MountBatch{}, &driver.MountItem{Kind: driver.ItemOrientation, Count: uint32(mask)})
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || len(got[0]) != 3 || got[0][0] != "or" || got[0][2] != float64(mask) {
			t.Fatalf("mask %d wrote %v, want [or 0 %d]", mask, got, mask)
		}
	}
}
