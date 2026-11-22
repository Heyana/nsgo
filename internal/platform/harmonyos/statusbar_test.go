//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestStatusBarSendsImmersive checks that the status bar's operation carries the immersive request where Index.ets reads it: o[2] the style, o[3] hidden, o[4] immersive, so that the page hides the status bar and the navigation indicator while it is on and shows them again when it ends.
func TestStatusBarSendsImmersive(t *testing.T) {
	for _, c := range []driver.StatusBarConfig{
		{Style: driver.StatusBarLight, Immersive: true},
		{Style: driver.StatusBarDark, Hidden: true},
		{Style: driver.StatusBarAuto},
	} {
		m := &mounter{views: map[driver.NodeID]*view{}}
		m.item(&driver.MountBatch{}, &driver.MountItem{Kind: driver.ItemStatusBar, StatusBar: &c})
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("%+v wrote %d operations, want 1: %v", c, len(got), got)
		}
		o := got[0]
		if len(o) != 5 || o[0] != "sb" {
			t.Fatalf("%+v wrote %v, want sb with style, hidden and immersive", c, o)
		}
		if o[2] != float64(c.Style) || o[3] != c.Hidden || o[4] != c.Immersive {
			t.Fatalf("%+v wrote style %v, hidden %v, immersive %v", c, o[2], o[3], o[4])
		}
	}
}
