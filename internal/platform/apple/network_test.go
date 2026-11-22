//go:build darwin && !ios

package apple

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestNetworkMonitorReadsTheSystemPath(t *testing.T) {
	updates := make(chan driver.Network, 8)
	initial, stop := (NetworkWatcher{}).WatchNetwork(func(value driver.Network) {
		select {
		case updates <- value:
		default:
		}
	})
	if stop == nil {
		t.Fatal("Network.framework did not start a monitor")
	}
	defer stop()
	select {
	case got := <-updates:
		if got != initial {
			t.Fatalf("initial state = %+v, first path = %+v", initial, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Network.framework did not report the current path")
	}
}
