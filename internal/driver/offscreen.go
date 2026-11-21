package driver

import "math"

// OffscreenCanvas is a recorded 2D drawing that platforms render once
// into an offscreen surface keyed by (ID, Version) and reuse as an
// image source across later frames until Version changes or
// ItemReleaseOffscreen releases ID. Once handed to the platform it
// never changes.
type OffscreenCanvas struct {
	ID      uint64
	Version uint64
	// Size is the logical size in points.
	Size Size
	// Scale is the backing pixels per point; values <= 0 default to 1.
	Scale float32
	// Drawing holds the recorded operations to render into the offscreen
	// surface.
	Drawing *Drawing
}

// EffectiveScale returns o's backing pixels per point, defaulting to 1
// when Scale is not positive or finite.
func (o *OffscreenCanvas) EffectiveScale() float32 {
	if o == nil || !(o.Scale > 0) || math.IsInf(float64(o.Scale), 0) {
		return 1
	}
	return o.Scale
}

// PixelSize returns the backing bitmap dimensions in pixels (width, height),
// or (0, 0) when o is nil or has a non-positive logical size.
func (o *OffscreenCanvas) PixelSize() (int, int) {
	if o == nil || !(o.Size.Width > 0) || !(o.Size.Height > 0) {
		return 0, 0
	}
	scale := float64(o.EffectiveScale())
	w := int(math.Ceil(float64(o.Size.Width) * scale))
	h := int(math.Ceil(float64(o.Size.Height) * scale))
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// Same reports whether o and other refer to the same offscreen canvas at
// the same version.
func (o *OffscreenCanvas) Same(other *OffscreenCanvas) bool {
	if o == other {
		return true
	}
	if o == nil || other == nil {
		return false
	}
	return o.ID == other.ID && o.Version == other.Version
}
