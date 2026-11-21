//go:build linux && !android

package linux

import (
	"os"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// cursorPoint reads the X server's root coordinates. GDK's X11 monitors
// share one integer scale. A Wayland session has no desktop-wide pointer
// sample, including when the app uses Xwayland.
func cursorPoint() (driver.Point, bool) {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return driver.Point{}, false
	}
	if !drawsOnX11() {
		return driver.Point{}, false
	}
	display := gdkDisplayGetDefault()
	xdisplay := gdkX11PanelXDisplay(display)
	if xdisplay == 0 {
		return driver.Point{}, false
	}
	var root, child uintptr
	var rootX, rootY, x, y int32
	var mask uint32
	if x11QueryPointer(xdisplay, x11LevelRoot(xdisplay), &root, &child, &rootX, &rootY, &x, &y, &mask) == 0 {
		return driver.Point{}, false
	}
	monitors := gdkDisplayGetMonitors(display)
	if monitors == 0 {
		return driver.Point{}, false
	}
	monitor := gListModelGetItem(monitors, 0)
	if monitor == 0 {
		return driver.Point{}, false
	}
	scale := float64(max(1, gdkMonitorGetScaleFactor(monitor)))
	gObjectUnref(monitor)
	return driver.Point{
		X: driver.Points(float64(rootX) / scale),
		Y: driver.Points(float64(rootY) / scale),
	}, true
}
