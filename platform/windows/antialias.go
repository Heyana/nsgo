//go:build windows

package windows

import (
	"math"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

// GDI cannot fill a fractional edge: RoundRect, CreateRoundRectRgn and
// GDI paths all move a whole pixel at a time, so a rounded view's corners
// come out as stair steps. The painter draws them itself instead. It
// measures how much of each pixel a shape covers and blends the color by
// that, and it clips a rounded subtree by blending what the subtree drew
// at the corners with what lay behind, by the shape's coverage.
//
// Only the corners are measured; the edges of a rounded rectangle lie on
// whole pixels and GDI fills them exactly. A transformed shape has no such
// edges left, so every one of its pixels is measured instead, except for a
// rotated border with a color per side, which GDI still draws.

// subsamples is how many points per axis a pixel's coverage is measured
// at, so coverage has 16 steps.
const subsamples = 4

// shape is what a painter measures coverage of, in the window's pixels: a
// rounded rectangle, less an optional rounded hole, which makes a
// border's ring, and optionally only the share of the ring one side of a
// border owns at the corners.
type shape struct {
	r, hole            rect
	radius, holeRadius float64
	side               uint8 // 0 draws the whole ring; 1 to 4 the top, right, bottom or left of it
}

// distance returns the signed distance from (x, y) to the edge of the
// rounded rectangle r, negative inside it.
func distance(r rect, radius, x, y float64) float64 {
	x0, y0 := float64(r.Left), float64(r.Top)
	x1, y1 := float64(r.Right), float64(r.Bottom)
	radius = min(radius, (x1-x0)*0.5, (y1-y0)*0.5)
	qx := math.Abs(x-(x0+x1)*0.5) - ((x1-x0)*0.5 - radius)
	qy := math.Abs(y-(y0+y1)*0.5) - ((y1-y0)*0.5 - radius)
	lx, ly := max(qx, 0), max(qy, 0)
	return math.Sqrt(lx*lx+ly*ly) + min(max(qx, qy), 0) - radius
}

// contains reports whether the window's point (x, y) is in s.
func (s shape) contains(x, y float64) bool {
	if outside(s.r, s.radius, x, y) {
		return false
	}
	if h := s.hole; !h.empty() {
		x0, y0 := float64(h.Left), float64(h.Top)
		x1, y1 := float64(h.Right), float64(h.Bottom)
		if x >= x0 && x <= x1 && y >= y0 && y <= y1 {
			radius := min(s.holeRadius, (x1-x0)*0.5, (y1-y0)*0.5)
			cx := min(max(x, x0+radius), x1-radius)
			cy := min(max(y, y0+radius), y1-radius)
			if dx, dy := x-cx, y-cy; dx*dx+dy*dy < radius*radius {
				return false
			}
		}
	}
	return s.side == 0 || s.wedge(x, y)
}

// outside reports whether the window's point (x, y) is beyond the rounded
// rectangle's edge. It compares squared distances, which the corner
// measurement does for every sample.
func outside(r rect, radius, x, y float64) bool {
	x0, y0 := float64(r.Left), float64(r.Top)
	x1, y1 := float64(r.Right), float64(r.Bottom)
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return true
	}
	radius = min(radius, (x1-x0)*0.5, (y1-y0)*0.5)
	cx := min(max(x, x0+radius), x1-radius)
	cy := min(max(y, y0+radius), y1-radius)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy > radius*radius
}

// wedge reports whether (x, y) lies on s.side's share of the corners: the
// region nearer that side than either side meeting it there, which is how
// CSS splits corners between the sides' colors.
func (s shape) wedge(x, y float64) bool {
	left, top := x-float64(s.r.Left), y-float64(s.r.Top)
	right, bottom := float64(s.r.Right)-x, float64(s.r.Bottom)-y
	switch s.side {
	case 1:
		return top <= left && top <= right
	case 2:
		return right <= top && right <= bottom
	case 3:
		return bottom <= right && bottom <= left
	default:
		return left <= top && left <= bottom
	}
}

// sampler places a target's pixels in the window's coordinates, with how
// far a pixel's corners reach once mapped back, and the step between its
// samples.
type sampler struct {
	inv            matrix
	x, y           float64 // the target's origin in the window
	margin         float64
	ux, uy, vx, vy float64
}

// sampler returns the painter's sampler for the target it draws into.
func (p *painter) sampler() sampler {
	inv := p.m.invert()
	return sampler{
		inv:    inv,
		x:      float64(p.ox),
		y:      float64(p.oy),
		margin: 0.5 * math.Max(math.Hypot(inv.a+inv.c, inv.b+inv.d), math.Hypot(inv.a-inv.c, inv.b-inv.d)),
		ux:     inv.a / subsamples, uy: inv.b / subsamples,
		vx: inv.c / subsamples, vy: inv.d / subsamples,
	}
}

// cover returns how much of the target's pixel (x, y) s covers, from 0 to
// 255.
func (smp sampler) cover(s shape, x, y int32) uint8 {
	// A pixel is fully covered or empty when its whole area lies inside or
	// outside the shape; only the edge needs its samples taken. The
	// pixel's corners are up to margin away from its centre once mapped
	// back, and a signed distance changes no faster than a distance.
	px, py := float64(x)+smp.x, float64(y)+smp.y
	wx, wy := smp.inv.apply(px+0.5, py+0.5)
	if d := distance(s.r, s.radius, wx, wy); d >= smp.margin {
		return 0
	} else if s.side == 0 && -d >= smp.margin &&
		(s.hole.empty() || distance(s.hole, s.holeRadius, wx, wy) >= smp.margin) {
		// A side's share of a ring is cut by its corners' diagonals, which
		// the pixel's centre and reach cannot settle.
		return 255
	}
	// The pixel's samples, in the window's pixels.
	x0, y0 := smp.inv.apply(px, py)
	var n uint32
	for i := range subsamples {
		for j := range subsamples {
			fx, fy := float64(i)+0.5, float64(j)+0.5
			if s.contains(x0+fx*smp.ux+fy*smp.vx, y0+fx*smp.uy+fy*smp.vy) {
				n++
			}
		}
	}
	return uint8(n * 255 / (subsamples * subsamples))
}

// samples returns how many of target pixel (x, y)'s samples lie in a, and
// how many of the rest lie in b: a and b do not overlap but on the line
// between them.
func (smp sampler) samples(a, b shape, x, y int32) (na, nb uint32) {
	px, py := float64(x)+smp.x, float64(y)+smp.y
	x0, y0 := smp.inv.apply(px, py)
	for i := range subsamples {
		for j := range subsamples {
			fx, fy := float64(i)+0.5, float64(j)+0.5
			qx, qy := x0+fx*smp.ux+fy*smp.vx, y0+fx*smp.uy+fy*smp.vy
			if a.contains(qx, qy) {
				na++
			} else if b.contains(qx, qy) {
				nb++
			}
		}
	}
	return na, nb
}

// paintCoverage blends c over the pixels of the window's rectangles that
// s covers, as much as it covers them.
func (p *painter) paintCoverage(areas []rect, s shape, c core.Color) {
	if c&0xff == 0 || p.target.bits == nil {
		return
	}
	gdiFlush.Call() // the pixels may still be in a batched GDI call
	smp := p.sampler()
	clips := p.clipRects()
	target := unsafe.Slice((*uint32)(p.target.bits), int(p.target.width)*int(p.target.height))
	alpha := uint32(c & 0xff)
	for _, area := range areas {
		box := p.m.bound(area).offset(-p.ox, -p.oy)
		for _, clip := range clips {
			b := box.intersect(clip)
			for y := b.Top; y < b.Bottom; y++ {
				row := target[int(y)*int(p.target.width):]
				for x := b.Left; x < b.Right; x++ {
					if cov := smp.cover(s, x, y); cov > 0 {
						row[x] = blend(row[x], c, uint32(cov)*alpha/255)
					}
				}
			}
		}
	}
}

// paintCorner blends the two sides that meet at a corner, a and b, over
// the pixels of the window's rectangle area. It blends both in one pass:
// a pixel their shared diagonal splits takes its share of each color at
// once, where blending one side and then the other would darken it.
func (p *painter) paintCorner(area rect, a, b shape, ca, cb core.Color) {
	gdiFlush.Call() // the pixels may still be in a batched GDI call
	smp := p.sampler()
	clips := p.clipRects()
	target := unsafe.Slice((*uint32)(p.target.bits), int(p.target.width)*int(p.target.height))
	box := p.m.bound(area).offset(-p.ox, -p.oy)
	for _, clip := range clips {
		r := box.intersect(clip)
		for y := r.Top; y < r.Bottom; y++ {
			row := target[int(y)*int(p.target.width):]
			for x := r.Left; x < r.Right; x++ {
				if na, nb := smp.samples(a, b, x, y); na+nb > 0 {
					row[x] = blendSides(row[x], ca, na, cb, nb)
				}
			}
		}
	}
}

// blendSides returns dst under na samples' worth of ca and nb samples'
// worth of cb, out of the samples a pixel has, each color blending by its
// own alpha.
func blendSides(dst uint32, ca core.Color, na uint32, cb core.Color, nb uint32) uint32 {
	const nsamples = subsamples * subsamples
	ma, mb := uint32(ca&0xff)*na, uint32(cb&0xff)*nb
	if ma+mb == 0 {
		return dst
	}
	k := ma + mb
	rest := uint32(255*nsamples) - k
	r := (uint32(ca>>24&0xff)*ma + uint32(cb>>24&0xff)*mb + (dst>>16&0xff)*rest) / (255 * nsamples)
	g := (uint32(ca>>16&0xff)*ma + uint32(cb>>16&0xff)*mb + (dst>>8&0xff)*rest) / (255 * nsamples)
	b := (uint32(ca>>8&0xff)*ma + uint32(cb>>8&0xff)*mb + (dst&0xff)*rest) / (255 * nsamples)
	return 0xff000000 | r<<16 | g<<8 | b
}

// blend returns dst with k/255 of c over it, c being 0xRRGGBBAA and dst a
// pixel of the target.
func blend(dst uint32, c core.Color, k uint32) uint32 {
	if k >= 255 {
		return 0xff000000 | uint32(c>>8)
	}
	inv := 255 - k
	r := (uint32(c>>24&0xff)*k + (dst>>16&0xff)*inv) / 255
	g := (uint32(c>>16&0xff)*k + (dst>>8&0xff)*inv) / 255
	b := (uint32(c>>8&0xff)*k + (dst&0xff)*inv) / 255
	return 0xff000000 | r<<16 | g<<8 | b
}

// mixPixels returns dst with k/255 of src over it, both being pixels of the
// target.
func mixPixels(dst, src, k uint32) uint32 {
	if k >= 255 {
		return src | 0xff000000
	}
	inv := 255 - k
	r := ((src>>16&0xff)*k + (dst>>16&0xff)*inv) / 255
	g := ((src>>8&0xff)*k + (dst>>8&0xff)*inv) / 255
	b := ((src&0xff)*k + (dst&0xff)*inv) / 255
	return 0xff000000 | r<<16 | g<<8 | b
}

// parts splits a rounded rectangle at its corners: the four squares of the
// corners, which alone have fractional coverage, and the three rectangles
// between them, whose edges are straight. The seven do not overlap and
// cover r.
func parts(r rect, radius float64) (corners [4]rect, middle [3]rect) {
	k := int32(math.Ceil(radius))
	xm, ym := r.Left+(r.Right-r.Left)/2, r.Top+(r.Bottom-r.Top)/2
	xl, xr := min(r.Left+k, xm), max(r.Right-k, xm)
	yt, yb := min(r.Top+k, ym), max(r.Bottom-k, ym)
	corners = [4]rect{
		{r.Left, r.Top, xl, yt}, {xr, r.Top, r.Right, yt},
		{r.Left, yb, xl, r.Bottom}, {xr, yb, r.Right, r.Bottom},
	}
	middle = [3]rect{{xl, r.Top, xr, yt}, {r.Left, yt, r.Right, yb}, {xl, yb, xr, r.Bottom}}
	return corners, middle
}

// sub returns the parts of a outside b, which do not overlap each other
// or b.
func sub(a, b rect) []rect {
	if a.intersect(b).empty() || b.empty() {
		return []rect{a}
	}
	var out []rect
	if b.Top > a.Top {
		out = append(out, rect{a.Left, a.Top, a.Right, min(b.Top, a.Bottom)})
	}
	if b.Bottom < a.Bottom {
		out = append(out, rect{a.Left, max(b.Bottom, a.Top), a.Right, a.Bottom})
	}
	top, bottom := max(a.Top, b.Top), min(a.Bottom, b.Bottom)
	if top < bottom {
		if b.Left > a.Left {
			out = append(out, rect{a.Left, top, min(b.Left, a.Right), bottom})
		}
		if b.Right < a.Right {
			out = append(out, rect{max(b.Right, a.Left), top, a.Right, bottom})
		}
	}
	kept := out[:0]
	for _, r := range out {
		if !r.empty() {
			kept = append(kept, r)
		}
	}
	return kept
}

// sideOf returns the side of a border whose ring a rectangle lies on: the
// top, right, bottom or left.
func sideOf(r, in rect) int {
	switch {
	case r.Bottom <= in.Top:
		return 1
	case r.Top >= in.Bottom:
		return 3
	case r.Right <= in.Left:
		return 4
	default:
		return 2
	}
}

// fillRects fills rectangles with c, inside the painter's clipping.
func (p *painter) fillRects(rects []rect, c core.Color) {
	if c&0xff == 0 {
		return
	}
	if c&0xff == 0xff {
		brush, _, _ := createSolidBrush.Call(colorRef(c))
		for _, r := range rects {
			if !r.empty() {
				fillRect.Call(p.dc, uintptr(unsafe.Pointer(&r)), brush)
			}
		}
		deleteObject.Call(brush)
		return
	}
	for _, r := range rects {
		if !r.empty() {
			p.blend(r, 0, rect{}, 0, c)
		}
	}
}
