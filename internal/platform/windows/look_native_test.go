//go:build windows

package windows

import (
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestWindowShadowKeepsTheSystemFrame checks NoShadow against DWM's state and the frame's metrics: a window with the system's title bar keeps its non-client rendering and frame, NoShadow or not, while one with a hidden title bar loses DWM's non-client rendering and its non-client area, keeping its resize edges, and gets both back with the shadow.
func TestWindowShadowKeepsTheSystemFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	get := dwmapi.NewProc("DwmGetWindowAttribute")
	ncEnabled := func(t *testing.T, hwnd uintptr) bool {
		t.Helper()
		const dwmwaNCRenderingEnabled = 1
		enabled := int32(-1)
		if hr, _, _ := get.Call(hwnd, dwmwaNCRenderingEnabled, uintptr(unsafe.Pointer(&enabled)), 4); failed(hr) {
			t.Fatalf("DwmGetWindowAttribute: %#x", hr)
		}
		return enabled != 0
	}
	// frame returns how much taller and wider the window is than its client area, in pixels.
	frame := func(hwnd uintptr) (int32, int32) {
		var outer, client rect
		getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&outer)))
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		return outer.Right - outer.Left - client.Right, outer.Bottom - outer.Top - client.Bottom
	}
	for _, opened := range []bool{false, true} {
		name := "changed"
		if opened {
			name = "opened"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			hwnd := startAppWith(t, func(*core.Scope) core.Node {
				close(started)
				return core.View{}
			}, Options{Title: "nsgo-shadow-" + name, Width: 180, Height: 100, NoShadow: opened})
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("the app did not start")
			}
			onMain(t, hwnd, func(w *window) {
				w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetShadow, On: true})
				framedX, framedY := frame(hwnd)
				w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetShadow, On: false})
				if !ncEnabled(t, hwnd) {
					t.Error("NoShadow turned off DWM's non-client rendering of a window with the system's title bar")
				}
				if x, y := frame(hwnd); x != framedX || y != framedY || framedY <= 0 {
					t.Errorf("NoShadow changed the system frame from %dx%d to %dx%d", framedX, framedY, x, y)
				}
				style, _, _ := getWindowLongPtrW.Call(hwnd, gwlStyle)
				if style&wsCaption != wsCaption || w.hwnd != hwnd {
					t.Error("NoShadow took the caption or replaced the window")
				}

				// Hiding the title bar of a shadowless window takes its shadow and its whole non-client area.
				w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetTitleBar, TitleBar: driver.TitleBarHidden})
				if ncEnabled(t, hwnd) {
					t.Error("a shadowless window with a hidden title bar kept DWM's non-client rendering")
				}
				if x, y := frame(hwnd); x != 0 || y != 0 {
					t.Errorf("a shadowless window kept a %dx%d non-client frame", x, y)
				}
				var outer rect
				getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&outer)))
				at := func(x, y int32) uintptr { return uintptr(uint32(uint16(int16(y)))<<16 | uint32(uint16(int16(x)))) }
				for _, c := range []struct {
					name string
					x, y int32
					want uintptr
				}{
					{"left", outer.Left + 1, (outer.Top + outer.Bottom) / 2, htLeft},
					{"right", outer.Right - 2, (outer.Top + outer.Bottom) / 2, htRight},
					{"bottom", (outer.Left + outer.Right) / 2, outer.Bottom - 2, htBottom},
					{"bottom left", outer.Left + 1, outer.Bottom - 2, htBottomLeft},
					{"top", (outer.Left + outer.Right) / 2, outer.Top + 1, htTop},
				} {
					if got := w.ncHitTest(at(c.x, c.y)); got != c.want {
						t.Errorf("hit test at the %s edge = %d, want %d", c.name, got, c.want)
					}
				}

				// The shadow returns with the non-client area of a hidden title bar.
				w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetShadow, On: true})
				if !ncEnabled(t, hwnd) {
					t.Error("restoring the shadow left DWM's non-client rendering off")
				}
				if x, _ := frame(hwnd); x <= 0 {
					t.Error("restoring the shadow left the hidden title bar's window without its side borders")
				}
				if w.hwnd != hwnd {
					t.Error("changing the shadow replaced the window")
				}
			})
		})
	}
}
