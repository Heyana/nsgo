package linux

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file builds on every system, so unit tests check Linux's Pango unit
// conversions and fractional measurement rules on any host.

// Pango units per logical pixel. Pango's widths and spacing and its absolute
// font sizes use these, so a text's size means what a view's does; its point
// sizes would scale by the context's resolution.
const pangoScale = 1024

func toPango(p driver.Points) int32 {
	v := math.Round(float64(p) * pangoScale)
	if v >= math.MaxInt32 {
		return math.MaxInt32
	}
	if v <= math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

func fromPango(v int32) driver.Points { return driver.Points(v) / pangoScale }

// pangoWrapWidth is the width in Pango units Pango wraps and ellipsizes at,
// or -1 for unbounded width (Auto or a single-line clipped layout drawn whole,
// which its box cuts).
func pangoWrapWidth(width driver.Points, clippedOneLine bool) int32 {
	if width.IsAuto() || clippedOneLine {
		return -1
	}
	if width <= 0 {
		return 0
	}
	return toPango(width)
}

// letterSpace splits a letter spacing into the Pango units Pango puts before
// and after a cluster, as its distribute_letter_spacing does; negative spacing
// counts as none.
func letterSpace(s driver.Points) (before, after int32) {
	spacing := int32(s * pangoScale)
	if spacing <= 0 {
		return 0, 0
	}
	before = spacing / 2
	if spacing&(pangoScale-1) == 0 {
		before = (before + pangoScale/2) &^ (pangoScale - 1)
	}
	return before, spacing - before
}

// pangoSpacedWidth clamps a letter-spaced layout's required width in Pango
// units to limit when bounded (limit >= 0).
func pangoSpacedWidth(need, limit int32) int32 {
	if limit >= 0 {
		return min(need, limit)
	}
	return need
}

// pangoMeasuredSize converts a Pango layout's logical size (w, h in Pango
// units) to fractional points, leaving device-pixel rounding to the core. When
// a bounded width rounded up by less than half a Pango unit to limit and the
// layout filled limit exactly, the returned width stays within width while
// pangoWrapWidth(size.Width, false) still yields limit.
func pangoMeasuredSize(w, h int32, width driver.Points, limit int32) driver.Size {
	size := driver.Size{Width: fromPango(w), Height: fromPango(h)}
	if !width.IsAuto() && limit >= 0 && w == limit {
		size.Width = min(size.Width, max(width, 0))
	}
	return size
}
