package imagedecode

import (
	"math"
	"slices"
)

// The rasterizer turns polygons into coverage: for each row of pixels it
// samples subsamples horizontal lines, finds where edges cross each line,
// keeps the spans inside by the nonzero or even-odd rule and adds the
// exact length of each span within each pixel. Coverage is exact across a
// row and sampled down it.
const subsamples = 16

// maxRasterWork bounds the edge rows (an edge times the pixel rows it
// spans) and maxRasterPoints the points one drawing may rasterize, so a
// small document cannot ask for minutes of work.
const (
	maxRasterWork   = 1 << 26
	maxRasterPoints = 1 << 24
)

type point struct{ x, y float64 }

func (p point) add(q point) point      { return point{p.x + q.x, p.y + q.y} }
func (p point) sub(q point) point      { return point{p.x - q.x, p.y - q.y} }
func (p point) mul(s float64) point    { return point{p.x * s, p.y * s} }
func (p point) dot(q point) float64    { return p.x*q.x + p.y*q.y }
func (p point) cross(q point) float64  { return p.x*q.y - p.y*q.x }
func (p point) length() float64        { return math.Hypot(p.x, p.y) }
func (p point) perp() point            { return point{-p.y, p.x} }
func (p point) finite() bool           { return !math.IsNaN(p.x+p.y) && !math.IsInf(p.x+p.y, 0) }
func lerp(a, b point, t float64) point { return point{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t} }
func (p point) near(q point) bool      { return math.Abs(p.x-q.x) < 1e-9 && math.Abs(p.y-q.y) < 1e-9 }
func (p point) unit() point            { return p.mul(1 / p.length()) }

// edge is a line of a polygon, top to bottom.
type edge struct {
	x0, y0, y1 float64 // where it starts and the rows it spans
	dxdy       float64
	dir        int32 // +1 where the polygon goes down, -1 up
}

type crossing struct {
	x   float64
	dir int32
}

type rasterizer struct {
	w, h   int
	edges  []edge
	acc    []float32 // coverage deltas: a row's coverage is their running sum
	cov    []float32
	cross  []crossing
	active []int
	work   int
	points int
	tmp    []point
}

func newRasterizer(w, h int) *rasterizer {
	return &rasterizer{w: w, h: h, acc: make([]float32, w+2), cov: make([]float32, w)}
}

func (r *rasterizer) line(a, b point) {
	if a.y == b.y || !a.finite() || !b.finite() {
		return
	}
	dir := int32(1)
	if a.y > b.y {
		a, b, dir = b, a, -1
	}
	// Edges beyond the bottom, the top or the right change no winding a
	// pixel sees.
	if b.y <= 0 || a.y >= float64(r.h) || min(a.x, b.x) >= float64(r.w) {
		return
	}
	r.edges = append(r.edges, edge{x0: a.x, y0: a.y, y1: b.y, dxdy: (b.x - a.x) / (b.y - a.y), dir: dir})
}

// polygon adds a closed polygon.
func (r *rasterizer) polygon(pts []point) {
	n := len(pts)
	if n < 2 {
		return
	}
	r.points += n
	for i := range n {
		r.line(pts[i], pts[(i+1)%n])
	}
}

// convex adds a convex polygon wound clockwise on screen, so that pieces
// added this way fill their union by the nonzero rule.
func (r *rasterizer) convex(pts ...point) {
	area := 0.0
	for i := range pts {
		area += pts[i].cross(pts[(i+1)%len(pts)])
	}
	if area < 0 {
		slices.Reverse(pts)
	}
	r.polygon(pts)
}

// fill computes the coverage of the polygons added since the last fill and
// calls span with each row that has any: cov holds the coverage from 0 to
// 1 of pixels x0 on. It reports false when the work is over budget.
func (r *rasterizer) fill(evenOdd bool, span func(y, x0 int, cov []float32)) bool {
	edges := r.edges
	r.edges = r.edges[:0]
	if len(edges) == 0 {
		return true
	}
	if r.points > maxRasterPoints {
		return false
	}
	slices.SortFunc(edges, func(a, b edge) int {
		switch {
		case a.y0 < b.y0:
			return -1
		case a.y0 > b.y0:
			return 1
		}
		return 0
	})
	top, bottom := float64(r.h), 0.0
	for i := range edges {
		e := &edges[i]
		top, bottom = min(top, e.y0), max(bottom, e.y1)
		r.work += int(min(e.y1, float64(r.h))-max(e.y0, 0)) + 1
	}
	if r.work > maxRasterWork {
		return false
	}
	inside := func(w int32) bool { return w != 0 }
	if evenOdd {
		inside = func(w int32) bool { return w&1 != 0 }
	}
	const step = 1.0 / subsamples
	const weight = float32(step)
	w := float64(r.w)
	acc := r.acc
	lo, hi := r.w, -1 // the acc indexes this row touched
	add := func(x float64, v float32) {
		x = min(max(x, 0), w)
		i := int(x)
		f := float32(x - float64(i))
		acc[i] += v * (1 - f)
		acc[i+1] += v * f
		lo, hi = min(lo, i), max(hi, i+1)
	}
	next := 0
	active := r.active[:0]
	for y := max(int(math.Floor(top)), 0); y < min(int(math.Ceil(bottom)), r.h); y++ {
		fy := float64(y)
		for next < len(edges) && edges[next].y0 < fy+1 {
			active = append(active, next)
			next++
		}
		kept := active[:0]
		for _, i := range active {
			if edges[i].y1 > fy {
				kept = append(kept, i)
			}
		}
		active = kept
		if len(active) == 0 {
			continue
		}
		for k := range subsamples {
			sy := fy + (float64(k)+0.5)*step
			cross := r.cross[:0]
			for _, i := range active {
				e := &edges[i]
				if sy >= e.y0 && sy < e.y1 {
					cross = append(cross, crossing{e.x0 + (sy-e.y0)*e.dxdy, e.dir})
				}
			}
			r.cross = cross
			if len(cross) == 0 {
				continue
			}
			slices.SortFunc(cross, func(a, b crossing) int {
				switch {
				case a.x < b.x:
					return -1
				case a.x > b.x:
					return 1
				}
				return 0
			})
			var wind int32
			var start float64
			for _, c := range cross {
				was := inside(wind)
				wind += c.dir
				if is := inside(wind); is && !was {
					start = c.x
				} else if was && !is && c.x > start {
					add(start, weight)
					add(c.x, -weight)
				}
			}
			if inside(wind) {
				add(start, weight)
				add(w, -weight)
			}
		}
		if hi < 0 {
			continue
		}
		end := min(hi, r.w)
		var sum float32
		any := false
		for x := lo; x < end; x++ {
			sum += acc[x]
			c := min(sum, 1)
			if c < 1.0/512 {
				c = 0
			} else {
				any = true
			}
			r.cov[x] = c
		}
		clear(acc[lo : hi+1])
		if any {
			span(y, lo, r.cov[lo:end])
		}
		lo, hi = r.w, -1
	}
	r.active = active[:0]
	return true
}
