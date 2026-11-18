package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestMonitors guards the event-only path: an EventMonitors event reaches
// the environment's screens, which Monitors then reads back. It does not
// call setMonitors directly.
func TestMonitors(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, nil)
	h.tree.Dispatch(driver.Event{Kind: driver.EventMonitors, Monitors: []driver.Monitor{{
		Frame:        driver.WindowFrame{Width: 1440, Height: 900},
		VisibleFrame: driver.WindowFrame{Y: 25, Width: 1440, Height: 850},
		Scale:        2,
	}}})
	got := h.tree.root.Env().Monitors()
	if len(got) != 1 {
		t.Fatalf("Monitors() gave %d screens, want 1", len(got))
	}
	if got[0].VisibleFrame.Height != 850 || got[0].Scale != 2 {
		t.Fatalf("Monitors()[0] = %+v, want the event's", got[0])
	}
}
