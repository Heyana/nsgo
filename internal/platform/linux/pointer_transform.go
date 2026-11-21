//go:build linux && !android

package linux

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// local uses the same placement GTK draws and picks, including native animation values, the current frame and a parent's perspective.
func (v *view) local(p driver.Point) driver.Point {
	local, ok := v.placement(0, 0).Unproject(p)
	if !ok {
		return driver.Point{X: driver.Points(math.Inf(1)), Y: driver.Points(math.Inf(1))}
	}
	return local
}

// surfacePoint maps a point of the window into v's viewport.
func (v *view) surfacePoint(p driver.Point) driver.Point {
	if parent := v.parent; parent != nil {
		p = parent.contentPoint(parent.surfacePoint(p))
	}
	return v.local(p)
}
