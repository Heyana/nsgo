//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestExplicitFullScreenRestoresWindowSize(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	scopes := make(chan *core.Scope, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		scopes <- cx
		return core.View{}
	}, Options{Title: "nsgo-explicit-full-screen", Width: 320, Height: 220})
	cx := <-scopes
	var target driver.DisplayInfo
	var supported bool
	w.onLoop(t, func() {
		if drawsOnX11() {
			if !bindX11Desktops() || gdkX11ScreenSupportsNetWMHint(gdkX11DisplayGetScreen(gdkDisplayGetDefault()), cstr("_NET_WM_STATE_FULLSCREEN")) == 0 {
				return
			}
		}
		if found := displays(); len(found) > 0 {
			target, supported = found[len(found)-1], true
		}
	})
	if !supported {
		t.Skip("requires a display and a window manager that supports full screen")
	}
	read := func() (bool, driver.Size, string) {
		var full bool
		var size driver.Size
		var displayID string
		w.onLoop(t, func() {
			full = w.env.FullScreen
			size = driver.Size{Width: driver.Points(gtkWidgetGetWidth(w.win)), Height: driver.Points(gtkWidgetGetHeight(w.win))}
			if surface := gtkNativeGetSurface(w.win); surface != 0 {
				if monitor := gdkDisplayGetMonitorAtSurface(gdkDisplayGetDefault(), surface); monitor != 0 {
					displayID = monitorDisplayID(monitor)
				}
			}
		})
		return full, size, displayID
	}
	_, before, _ := read()
	wait := func(full bool, size driver.Size) {
		t.Helper()
		for limit := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			gotFull, gotSize, gotDisplay := read()
			if gotFull == full && gotSize == size && (!full || gotDisplay == target.ID) {
				return
			}
			if time.Now().After(limit) {
				t.Fatalf("full screen = %v, size = %v, display = %q; want %v, %v on %q", gotFull, gotSize, gotDisplay, full, size, target.ID)
			}
		}
	}
	cx.Post(func() {
		cx.SetFullScreen(true, target.ID)
		cx.SetFullScreen(true, target.ID)
	})
	wait(true, target.Frame.Size)
	cx.Post(func() { cx.SetFullScreen(false) })
	wait(false, before)
}
