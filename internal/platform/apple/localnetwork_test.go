//go:build darwin && !ios

package apple

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestLocalNetworkProbeFindsItself advertises and browses the probe's Bonjour type, which finds its own instance and answers granted to both requests, as macOS lets a test started from a terminal or a CI runner through; the status learned answers checks after it, and the DNS-SD operations end.
func TestLocalNetworkProbeFindsItself(t *testing.T) {
	release := holdMainLoop()
	defer release()
	answers := make(chan int, 2)
	OnMain(func() {
		probeLocalNetwork(func(st int) { answers <- st })
		probeLocalNetwork(func(st int) { answers <- st })
	})
	for range 2 {
		select {
		case st := <-answers:
			if st != driver.PermissionGranted {
				t.Fatalf("the probe answered %d, want granted", st)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("the probe did not find its own instance")
		}
	}
	done := make(chan bool)
	OnMain(func() { done <- localNetwork.probe == nil })
	if !<-done {
		t.Error("the probe still runs after answering")
	}
	if v := localNetwork.learned.Load(); v != driver.PermissionGranted+1 {
		t.Errorf("learned %d, want granted", v-1)
	}
	if LocalNetworkGated() && LocalNetworkStatus() != driver.PermissionGranted {
		t.Errorf("a check after the probe answered %d", LocalNetworkStatus())
	}
}
