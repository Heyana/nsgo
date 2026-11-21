package driver

import (
	"bytes"
	"fmt"
	"math"
)

// CustomCursorHiDPIScale is the minimum display pixel ratio at which a
// CustomCursor selects its Image2x representation.
const CustomCursorHiDPIScale float32 = 1.5

// CustomCursor describes an image-based pointer cursor on desktops and the web.
// Image is the 1x bitmap or vector asset; Image2x is an optional 2x HiDPI image
// selected automatically when the display scale is at least 1.5. Hotspot is the
// active point in logical points from the top-left corner, clamped to the
// image's bounds. Fallback is the named cursor shown when the image is empty or
// cannot be loaded, and the CSS fallback keyword on the web.
type CustomCursor struct {
	Image    ImageSource
	Image2x  ImageSource
	Hotspot  Point
	Fallback Cursor
}

// Empty reports whether c has neither a 1x nor a 2x cursor image.
func (c *CustomCursor) Empty() bool {
	return c == nil || (c.Image.Empty() && c.Image2x.Empty())
}

// Equal reports whether c and o specify the same custom cursor.
func (c *CustomCursor) Equal(o *CustomCursor) bool {
	if c == o {
		return true
	}
	if c == nil || o == nil {
		return false
	}
	return c.Hotspot == o.Hotspot && c.Fallback == o.Fallback &&
		cursorSourceEqual(c.Image, o.Image) && cursorSourceEqual(c.Image2x, o.Image2x)
}

func cursorSourceEqual(a, b ImageSource) bool {
	if a.URI != b.URI || a.Size != b.Size || a.Scales != b.Scales ||
		a.Pixels != b.Pixels || a.Version != b.Version || len(a.Data) != len(b.Data) {
		return false
	}
	if len(a.Data) > 0 && &a.Data[0] != &b.Data[0] {
		return bytes.Equal(a.Data, b.Data)
	}
	return true
}

// Key returns a cache key identifying c's images, hotspot and fallback.
func (c *CustomCursor) Key() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("1x:%s|2x:%s|hot:%g,%g|fb:%d",
		c.Image.Key(), c.Image2x.Key(), c.Hotspot.X, c.Hotspot.Y, c.Fallback)
}

// SelectSource picks the ImageSource to use for a display with pixel ratio scale,
// returning the chosen source and whether it is the 2x HiDPI image.
// When scale >= 1.5 and Image2x is non-empty, Image2x is chosen; otherwise Image
// is chosen, falling back to Image2x if Image is empty.
func (c *CustomCursor) SelectSource(scale float32) (src ImageSource, is2x bool) {
	if c == nil {
		return ImageSource{}, false
	}
	if scale >= CustomCursorHiDPIScale && !c.Image2x.Empty() {
		return c.Image2x, true
	}
	if !c.Image.Empty() {
		return c.Image, false
	}
	if !c.Image2x.Empty() {
		return c.Image2x, true
	}
	return ImageSource{}, false
}

// EffectiveFallback returns the named fallback cursor for c: c.Fallback when
// set, otherwise named.
func (c *CustomCursor) EffectiveFallback(named Cursor) Cursor {
	if c != nil && c.Fallback != CursorAuto {
		return c.Fallback
	}
	return named
}

// LogicalSize returns the cursor's logical size in points when it can be
// determined from Image/Image2x metadata, Pixels, or a PNG header without
// decoding pixel data; zero when unknown.
func (c *CustomCursor) LogicalSize() Size {
	if c == nil {
		return Size{}
	}
	if s := cursorSourceSize(c.Image, 1); s.Width > 0 && s.Height > 0 {
		return s
	}
	if s := cursorSourceSize(c.Image2x, 2); s.Width > 0 && s.Height > 0 {
		return s
	}
	return Size{}
}

func cursorSourceSize(src ImageSource, defaultDensity float32) Size {
	if src.Empty() {
		return Size{}
	}
	if src.Size.Width > 0 && src.Size.Height > 0 {
		return src.Size
	}
	d := defaultDensity
	if d <= 0 {
		d = 1
	}
	if src.Pixels != nil && !src.Pixels.Rect.Empty() {
		b := src.Pixels.Rect
		return Size{
			Width:  max(Points(1), Points(b.Dx())/Points(d)),
			Height: max(Points(1), Points(b.Dy())/Points(d)),
		}
	}
	if len(src.Data) > 0 {
		if s, err := pngSize(src.Data); err == nil && s.Width > 0 && s.Height > 0 {
			if defaultDensity == 1 {
				if sd := src.Density(); sd > 0 {
					d = sd
				}
			}
			return Size{
				Width:  max(Points(1), s.Width/Points(d)),
				Height: max(Points(1), s.Height/Points(d)),
			}
		}
	}
	return Size{}
}

// ClampHotspot clamps hotspot (in logical points from top-left) to [0, size-1].
// When a dimension of size is <= 0 (unknown), that axis is only clamped to >= 0.
func ClampHotspot(hotspot Point, size Size) Point {
	clampAxis := func(v, limit Points) Points {
		if math.IsNaN(float64(v)) || v < 0 {
			return 0
		}
		if limit > 0 {
			hi := max(Points(0), limit-1)
			if math.IsInf(float64(v), 1) || v > hi {
				return hi
			}
		} else if math.IsInf(float64(v), 1) {
			return 0
		}
		return v
	}
	return Point{
		X: clampAxis(hotspot.X, size.Width),
		Y: clampAxis(hotspot.Y, size.Height),
	}
}

// ClampHotspotPixels converts a logical-point hotspot to pixel coordinates for a
// bitmap of pixW × pixH pixels at the given image density (1 for 1x, 2 for 2x),
// clamped strictly to [0, pixW-1] × [0, pixH-1].
func ClampHotspotPixels(hotspot Point, pixW, pixH int, density float32) (int, int) {
	if pixW <= 0 || pixH <= 0 {
		return 0, 0
	}
	if density <= 0 {
		density = 1
	}
	clamp := func(v Points, limit int) int {
		f := float64(v) * float64(density)
		if math.IsNaN(f) || f <= 0 {
			return 0
		}
		if math.IsInf(f, 1) || f >= float64(limit-1) {
			return limit - 1
		}
		n := int(math.Round(f))
		if n < 0 {
			return 0
		}
		if n >= limit {
			return limit - 1
		}
		return n
	}
	return clamp(hotspot.X, pixW), clamp(hotspot.Y, pixH)
}
