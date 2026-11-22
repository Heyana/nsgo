//go:build ios && nsgotest

package apple_test

import (
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// UIKit must start before device-idiom and Now Playing tests run. A plain
// simctl-spawned library process does not identify itself as an Apple TV app.
func TestMain(m *testing.M) {
	nsgotest.MainFunc(func() int {
		if nsgotest.Target() != "" {
			ready := make(chan struct{})
			apple.OnMain(func() { close(ready) })
			<-ready
		}
		return m.Run()
	})
}

func TestTVPictureInPictureNativePolicy(t *testing.T) {
	if nsgotest.Target() != "tvos_simulator" {
		t.Skip("requires tvOS")
	}
	app := nsgotest.Start(t, func(*core.Scope) core.Node { return core.View{} })
	app.Settle()
	result := make(chan error, 1)
	apple.OnMain(func() { result <- apple.CheckTVPictureInPictureForTest() })
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the native TV PiP policy check did not complete")
	}
}
