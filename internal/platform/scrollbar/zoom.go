package scrollbar

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Zooming math shared by desktop scroll views (Windows, Linux): axis limits
// with optional centering, rubber-banded zoom bounds, focal-point mapping,
// animated zoom easing, and Ctrl+wheel zoom scaling.

// ZoomAxisLimits returns the offset limits along one axis for a zooming
// scroll view of viewport frame and unzoomed content size at zoom z.
// When CenterContent is set and the zoomed content plus insets fits inside
// the viewport, lo and hi both equal the offset that centers it.
func ZoomAxisLimits(c *driver.ScrollConfig, frame, content driver.Size, z float64, horizontal bool) (lo, hi driver.Points) {
	in := c.Behavior.ContentInset
	lead, trail, length, viewport := in.Top, in.Bottom, content.Height, frame.Height
	if horizontal {
		lead, trail, length, viewport = in.Left, in.Right, content.Width, frame.Width
	}
	length *= driver.Points(z)
	lo = -lead
	if c.CenterContent && c.Zooms() && lead+length+trail < viewport {
		lo -= (viewport - lead - length - trail) / 2
		return lo, lo
	}
	return lo, max(length+trail-viewport, lo)
}

// ClampZoomOffset clamps offset p to both axes' limits at zoom z.
func ClampZoomOffset(c *driver.ScrollConfig, frame, content driver.Size, p driver.Point, z float64) driver.Point {
	xlo, xhi := ZoomAxisLimits(c, frame, content, z, true)
	ylo, yhi := ZoomAxisLimits(c, frame, content, z, false)
	return driver.Point{
		X: min(max(p.X, xlo), xhi),
		Y: min(max(p.Y, ylo), yhi),
	}
}

// LimitZoom keeps scale within c's zoom limits, rubber-banding past them
// while rubber is true unless NoBouncesZoom is set.
func LimitZoom(c *driver.ScrollConfig, scale float64, rubber bool) float64 {
	lo, hi := float64(c.MinZoomScale), float64(c.MaxZoomScale)
	if hi <= lo {
		return 1
	}
	rubber = rubber && !c.NoBouncesZoom
	switch {
	case scale > hi && rubber:
		return hi * math.Pow(scale/hi, 0.3)
	case scale > hi:
		return hi
	case scale < lo && rubber:
		return lo * math.Pow(scale/lo, 0.3)
	case scale < lo:
		return lo
	}
	return scale
}

// ContentAt returns the unzoomed content point at viewport point focus,
// given offset and zoom z.
func ContentAt(offset, focus driver.Point, z float64) driver.Point {
	if z <= 0 {
		z = 1
	}
	zp := driver.Points(z)
	return driver.Point{X: (offset.X + focus.X) / zp, Y: (offset.Y + focus.Y) / zp}
}

// OffsetFor returns the offset that places unzoomed content point c at
// viewport point focus at zoom z.
func OffsetFor(content, focus driver.Point, z float64) driver.Point {
	zp := driver.Points(z)
	return driver.Point{X: content.X*zp - focus.X, Y: content.Y*zp - focus.Y}
}

// ZoomEase interpolates a scroll view's zoom from Scale to Target while
// keeping unzoomed content point moving from From to To at viewport Focus.
type ZoomEase struct {
	From, To, Focus driver.Point
	Scale, Target   float64
	Clamped         bool
}

// NewZoomEase builds a ZoomEase from current (offset, zoom) to targetZoom
// with content point c placed at viewport focus. It returns ok=false when
// the view is already within half a point and 1e-3 scale of the target.
func NewZoomEase(c *driver.ScrollConfig, frame, content driver.Size, offset driver.Point, zoom float64, targetContent, focus driver.Point, targetZoom float64) (ZoomEase, driver.Point, bool) {
	at := ClampZoomOffset(c, frame, content, OffsetFor(targetContent, focus, targetZoom), targetZoom)
	if math.Abs(float64(at.X-offset.X)) < 0.5 && math.Abs(float64(at.Y-offset.Y)) < 0.5 && math.Abs(targetZoom-zoom) < 1e-3 {
		return ZoomEase{}, at, false
	}
	return ZoomEase{
		From:    ContentAt(offset, focus, zoom),
		To:      ContentAt(at, focus, targetZoom),
		Focus:   focus,
		Scale:   zoom,
		Target:  targetZoom,
		Clamped: ClampZoomOffset(c, frame, content, offset, zoom) == offset,
	}, at, true
}

// Step returns the offset and zoom at eased progress k in [0, 1].
func (e ZoomEase) Step(c *driver.ScrollConfig, frame, content driver.Size, k float64) (driver.Point, float64) {
	z := e.Scale + (e.Target-e.Scale)*k
	kp := driver.Points(k)
	pt := driver.Point{X: e.From.X + (e.To.X-e.From.X)*kp, Y: e.From.Y + (e.To.Y-e.From.Y)*kp}
	at := OffsetFor(pt, e.Focus, z)
	if e.Clamped {
		at = ClampZoomOffset(c, frame, content, at, z)
	}
	return at, z
}

// WheelZoom scales current by a Ctrl+wheel delta in points (positive delta
// zooms out, negative delta zooms in, matching web's wheel zoom).
func WheelZoom(current, deltaY float64) float64 {
	return current * math.Exp(-deltaY/100)
}
