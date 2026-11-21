package capture_test

import (
	"os"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestDesktopCursorPoint(t *testing.T) {
	target := nsgotest.Target()
	if target != "" && target != "macos" && target != "windows" && target != "linux" {
		t.Skip("desktop pointer query")
	}
	type sample struct {
		p         core.Point
		available bool
		monitors  []core.DisplayInfo
	}
	reading := make(chan sample, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		desktop.CursorPoint(cx, func(p core.Point, available bool) {
			reading <- sample{p, available, desktop.Monitors(cx)}
		})
		return core.Text{Content: "Cursor point"}
	}, nsgotest.Environment(core.Environment{OS: core.OSLinux}))
	app.Settle()
	select {
	case got := <-reading:
		wayland := target == "linux" && (os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland")
		// A disconnected Windows session exposes a virtual WinDisc display,
		// but GetCursorPos requires the input desktop.
		windowsDisconnected := target == "windows" && len(got.monitors) == 1 && got.monitors[0].ID == "WinDisc"
		if target == "" || wayland || windowsDisconnected {
			if got.available || got.p != (core.Point{}) {
				t.Errorf("the pointer was available on a session without a global pointer query: %v", got.p)
			}
			return
		}
		if !got.available || !slices.ContainsFunc(got.monitors, func(d core.DisplayInfo) bool { return d.Frame.Contains(got.p) }) {
			t.Errorf("cursor %v (available = %v) is outside the desktop displays %v", got.p, got.available, got.monitors)
		}
	default:
		t.Fatal("the desktop cursor query did not answer")
	}
}
