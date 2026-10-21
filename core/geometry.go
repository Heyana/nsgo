// Package core is NSGo's platform-independent core: geometry, styles, the
// reactive graph, elements, the shadow tree, layout and mount items.
//
// It never imports a platform package or cgo, so it builds and tests
// headless on every OS.
package core

import "math"

// Points is a length in platform-independent points (iOS points, Android dp,
// CSS pixels at a zoom of 1).
type Points float32

// Auto marks a dimension the layout computes.
var Auto = Points(float32(math.NaN()))

// IsAuto reports whether p is Auto.
func (p Points) IsAuto() bool { return p != p }

// Point is a position in points.
type Point struct{ X, Y Points }

// Size is a width and a height in points.
type Size struct{ Width, Height Points }

// Bounds is a rectangle: its origin relative to the parent's and its size.
type Bounds struct {
	Origin Point
	Size   Size
}

// Contains reports whether p lies inside b.
func (b Bounds) Contains(p Point) bool {
	return p.X >= b.Origin.X && p.Y >= b.Origin.Y &&
		p.X < b.Origin.X+b.Size.Width && p.Y < b.Origin.Y+b.Size.Height
}

// Edges holds a length per side.
type Edges struct{ Top, Right, Bottom, Left Points }

// All returns edges of the same length on every side.
func All(p Points) Edges { return Edges{p, p, p, p} }

// Symmetric returns edges with vertical and horizontal lengths.
func Symmetric(vertical, horizontal Points) Edges {
	return Edges{vertical, horizontal, vertical, horizontal}
}

// Color is an sRGB color packed as 0xRRGGBBAA, the wire form of mount items.
type Color uint32

// RGB returns an opaque color from 0xRRGGBB.
func RGB(hex uint32) Color { return Color(hex<<8 | 0xff) }

// Components returns the red, green, blue and alpha channels from 0 to 1.
func (c Color) Components() (r, g, b, a float64) {
	return float64(c>>24&0xff) / 255, float64(c>>16&0xff) / 255,
		float64(c>>8&0xff) / 255, float64(c&0xff) / 255
}
