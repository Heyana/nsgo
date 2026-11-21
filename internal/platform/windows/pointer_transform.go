//go:build windows

package windows

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// pointerMatrix follows place, including transitions and native values.
func (v *view) pointerMatrix() driver.Matrix3D {
	f := frameOf(v)
	m := v.shownStyle().Matrix(f.Size)
	if k, ok := v.keyframe(); ok {
		m = k.Matrix(f.Size).Mul(m)
	}
	if p := v.parent; p != nil {
		at := driver.Point{X: f.Origin.X - p.offset.X, Y: f.Origin.Y - p.offset.Y}
		m = driver.ChildPerspective(p.shownStyle().ChildPerspective, frameOf(p).Size, at).Mul(m)
	}
	if fd := v.fading; fd != nil && fd.tr.Property == driver.LayoutScale {
		cx, cy := float64(f.Size.Width)/2, float64(f.Size.Height)/2
		m = m.Mul(driver.Translate3D(cx, cy, 0)).Mul(driver.Matrix3D{fd.shown, 0, 0, 0, 0, fd.shown, 0, 0,
			0, 0, 1, 0, 0, 0, 0, 1}).Mul(driver.Translate3D(-cx, -cy, 0))
	}
	return m
}

func (v *view) local(p driver.Point) driver.Point {
	local, ok := driver.IntoView(frameOf(v), v.pointerMatrix(), p)
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
