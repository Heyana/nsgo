//go:build windows

package windows

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestNetworkMonitorReadsNetworkListManager(t *testing.T) {
	updates := make(chan driver.Network, 8)
	_, stop := watchNetwork(func(value driver.Network) {
		select {
		case updates <- value:
		default:
		}
	})
	defer stop()
	select {
	case got := <-updates:
		if !got.Online && got.Expensive {
			t.Errorf("an offline route was reported as metered: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Network List Manager did not report connectivity")
	}
}
