package terminal

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// graphicsRune identifies the characters whose geometry belongs to the grid,
// rather than the font. Keep their colors unchanged by minimum contrast.
func graphicsRune(r rune) bool {
	return r >= 0x2500 && r <= 0x259f || r >= 0xe0b0 && r <= 0xe0b7
}

// drawGraphics fills the cell exactly, including at fractional point sizes.
// Shared cell edges round to the same device pixel on both sides of a join.
func drawGraphics(d *core.Drawing, r rune, x, y, w, h, ratio float32, fg core.Color) bool {
	if !graphicsRune(r) {
		return false
	}
	ratio = max(ratio, 1)
	snap := func(n float32) float32 { return float32(math.Round(float64(n*ratio))) / ratio }
	x0, y0, x1, y1 := snap(x), snap(y), snap(x+w), snap(y+h)
	cx, cy := snap(x+w/2), snap(y+h/2)
	thin := max(snap(w/8), 1/ratio)
	fill := func(a, b, c, e float32) {
		a, b, c, e = snap(a), snap(b), snap(c), snap(e)
		if c > a && e > b {
			d.FillRect(a, b, c-a, e-b)
		}
	}
	d.Save().SetFillColor(fg).SetStrokeColor(fg).SetLineWidth(thin)
	defer d.Restore()
	d.BeginPath().MoveTo(x0, y0).LineTo(x1, y0).LineTo(x1, y1).LineTo(x0, y1).ClosePath().Clip()
	if r >= 0x2580 && r <= 0x259f {
		switch {
		case r == 0x2580:
			fill(x0, y0, x1, y+h/2)
		case r <= 0x2588:
			fill(x0, y+h*(1-float32(r-0x2580)/8), x1, y1)
		case r <= 0x258f:
			fill(x0, y0, x+w*float32(0x2590-r)/8, y1)
		case r == 0x2590:
			fill(x+w/2, y0, x1, y1)
		case r <= 0x2593:
			// Shade cells have a stable coverage at every density, with no
			// font-dependent margins or joins between adjacent cells.
			d.SetFillColor(fg.Faded(float32(r-0x2590) / 4))
			fill(x0, y0, x1, y1)
		case r == 0x2594:
			fill(x0, y0, x1, y+h/8)
		case r == 0x2595:
			fill(x+w*7/8, y0, x1, y1)
		default:
			// Quadrants: upper-left, upper-right, lower-left, lower-right.
			mask := [...]uint8{4, 8, 1, 13, 9, 7, 11, 2, 6, 14}[r-0x2596]
			for q := range 4 {
				if mask&(1<<q) != 0 {
					a, b := x+w*float32(q%2)/2, y+h*float32(q/2)/2
					fill(a, b, a+w/2, b+h/2)
				}
			}
		}
		return true
	}
	if r >= 0xe0b0 {
		left := r == 0xe0b2 || r == 0xe0b3 || r == 0xe0b6 || r == 0xe0b7
		base, tip := x0, x1
		if left {
			base, tip = x1, x0
		}
		d.BeginPath().MoveTo(base, y0)
		if r >= 0xe0b4 {
			const k = 0.55228475
			d.CubicTo(base+(tip-base)*k, y0, tip, cy-(y1-y0)*k/2, tip, cy)
			d.CubicTo(tip, cy+(y1-y0)*k/2, base+(tip-base)*k, y1, base, y1)
		} else {
			d.LineTo(tip, cy).LineTo(base, y1)
		}
		if r%2 == 0 {
			d.ClosePath().Fill()
		} else {
			d.Stroke()
		}
		return true
	}
	b := boxLines[r-0x2500]
	if b.kind >= 1 && b.kind <= 3 {
		d.BeginPath()
		if b.kind != 2 {
			d.MoveTo(x0, y1).LineTo(x1, y0)
		}
		if b.kind != 1 {
			d.MoveTo(x0, y0).LineTo(x1, y1)
		}
		d.Stroke()
		return true
	}
	if b.kind == 4 {
		// The arc joins the cell's center lines with a tangent at each
		// edge, so it meets the straight cells on either side.
		a, z := x0, y0
		if b.right != 0 {
			a = x1
		}
		if b.down != 0 {
			z = y1
		}
		d.BeginPath().MoveTo(a, cy).QuadTo(cx, cy, cx, z).Stroke()
		return true
	}
	if b.kind >= 5 {
		n := int(b.kind) - 3
		for i := range n {
			lo, hi := (float32(i)+0.15)/float32(n), (float32(i)+0.85)/float32(n)
			weight := max(b.left, b.up)
			thickness := thin * float32(weight)
			if b.left != 0 {
				fill(x+w*lo, cy-thickness/2, x+w*hi, cy+thickness/2)
			} else {
				fill(cx-thickness/2, y+h*lo, cx+thickness/2, y+h*hi)
			}
		}
		return true
	}
	// Double corners route the two lanes around the corner separately.
	// Three/four-way junctions keep both complete parallel center lines.
	joinEnd := func(edge, center float32, negative, positive, opposite, weight uint8, off float32) float32 {
		direction := float32(1)
		if edge < center {
			direction = -1
		}
		if opposite != 0 {
			return center
		}
		if negative == 3 && positive == 3 {
			return center + direction*thin
		}
		if negative == 3 && positive == 0 || positive == 3 && negative == 0 {
			if weight != 3 {
				return center - direction*thin
			}
			perpendicular := float32(1)
			if negative == 3 {
				perpendicular = -1
			}
			return center + off*direction*perpendicular
		}
		return center
	}
	horizontal := func(edge float32, weight uint8) {
		if weight == 0 {
			return
		}
		opposite := b.left
		if edge < cx {
			opposite = b.right
		}
		if weight != 3 {
			end := joinEnd(edge, cx, b.up, b.down, opposite, weight, 0)
			lo, hi := min(edge, end), max(edge, end)
			t := thin * float32(weight)
			fill(lo, cy-t/2, hi+thin/2, cy+t/2)
			return
		}
		for _, off := range []float32{-thin, thin} {
			end := joinEnd(edge, cx, b.up, b.down, opposite, weight, off)
			fill(min(edge, end)-thin/2, cy+off-thin/2, max(edge, end)+thin/2, cy+off+thin/2)
		}
	}
	vertical := func(edge float32, weight uint8) {
		if weight == 0 {
			return
		}
		opposite := b.up
		if edge < cy {
			opposite = b.down
		}
		if weight != 3 {
			end := joinEnd(edge, cy, b.left, b.right, opposite, weight, 0)
			lo, hi := min(edge, end), max(edge, end)
			t := thin * float32(weight)
			fill(cx-t/2, lo, cx+t/2, hi+thin/2)
			return
		}
		for _, off := range []float32{-thin, thin} {
			end := joinEnd(edge, cy, b.left, b.right, opposite, weight, off)
			fill(cx+off-thin/2, min(edge, end)-thin/2, cx+off+thin/2, max(edge, end)+thin/2)
		}
	}
	// Clip strips to the cell. Their center overlap closes mixed-weight
	// junctions while the cell edge is the exact end of every outer arm.
	horizontal(x0, b.left)
	horizontal(x1, b.right)
	vertical(y0, b.up)
	vertical(y1, b.down)
	return true
}
