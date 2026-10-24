package driver

import "math"

// Corners holds a radius per corner of a box. Right to left, the core
// swaps left and right, as it swaps the other sides.
type Corners struct{ TopLeft, TopRight, BottomRight, BottomLeft Points }

// AllCorners returns corners of the same radius.
func AllCorners(r Points) Corners { return Corners{r, r, r, r} }

// Uniform reports whether every corner has the same radius.
func (c Corners) Uniform() bool {
	return c.TopLeft == c.TopRight && c.TopLeft == c.BottomRight && c.TopLeft == c.BottomLeft
}

// Array returns the radii clockwise from the top left.
func (c Corners) Array() [4]Points {
	return [4]Points{c.TopLeft, c.TopRight, c.BottomRight, c.BottomLeft}
}

// Fit scales the radii down, all by one factor, until adjacent ones fit
// along the sides of a box of size, as CSS does with border-radius, and
// clamps negative ones to 0.
func (c Corners) Fit(size Size) Corners {
	r := c.Array()
	for i := range r {
		r[i] = max(r[i], 0)
	}
	f := Points(1)
	side := func(length, a, b Points) {
		if sum := a + b; sum > 0 && length >= 0 && sum*f > length {
			f = length / sum
		}
	}
	side(size.Width, r[0], r[1])
	side(size.Height, r[1], r[2])
	side(size.Width, r[2], r[3])
	side(size.Height, r[3], r[0])
	return Corners{r[0] * f, r[1] * f, r[2] * f, r[3] * f}
}

// Inset returns the radii of a box d inside this one (d negative grows
// it): each shrinks by d, never below 0, as CSS shrinks the inner corners
// of a border and grows a shadow's spread.
func (c Corners) Inset(d Points) Corners {
	r := c.Array()
	for i := range r {
		if r[i] > 0 {
			r[i] = max(r[i]-d, 0)
		}
	}
	return Corners{r[0], r[1], r[2], r[3]}
}

// SegmentKind is what a Segment of an outline does.
type SegmentKind uint8

const (
	// SegmentMove starts the outline at P[0].
	SegmentMove SegmentKind = iota
	// SegmentLine draws a straight line to P[0].
	SegmentLine
	// SegmentCubic draws a cubic Bézier through the control points P[0]
	// and P[1] to P[2].
	SegmentCubic
	// SegmentClose closes the outline.
	SegmentClose
)

// Segment is one step of an outline, in points.
type Segment struct {
	Kind SegmentKind
	P    [3][2]float64
}

// Smoothing is how far continuous corners ease into their edges: Figma's
// corner smoothing at 60%, which matches iOS's.
const Smoothing = 0.6

// RoundedRect returns the outline of the rectangle at x, y of size w, h
// with corners, fitted with Corners.Fit first, appending to out. It runs
// clockwise on screen (y down) from the top edge, or counterclockwise when
// reverse, which makes a hole under the non-zero rule. Continuous corners
// are a circular arc between two cubic transitions spreading the
// curvature over (1 + Smoothing) times the radius of each edge, as NSH's
// shape module draws them; where the box is too small they shrink to
// circular corners, so pills stay pills.
func RoundedRect(out []Segment, x, y, w, h float64, corners Corners, continuous, reverse bool) []Segment {
	c := corners.Fit(Size{Points(w), Points(h)}).Array()
	var radii [4][2]float64
	for i, r := range c {
		radii[i] = [2]float64{float64(r), float64(r)}
	}
	return EllipticRect(out, x, y, w, h, radii, continuous, reverse)
}

// EllipticRect is RoundedRect with each corner's radius along the
// horizontal and the vertical edge, clockwise from the top left, already
// fitted: the inner edge of a border whose sides differ.
func EllipticRect(out []Segment, x, y, w, h float64, radii [4][2]float64, continuous, reverse bool) []Segment {
	tl, tr, br, bl := radii[0], radii[1], radii[2], radii[3]
	x1, y1 := x+w, y+h
	// limit is how far a corner may reach along an edge of length shared
	// with a neighbor of radius other: its share of the edge.
	limit := func(length, r, other float64) float64 {
		if r+other > 0 {
			return length * r / (r + other)
		}
		return 0
	}
	// Each corner: its point, the directions it is entered and left
	// along, its radius along each and how far it may reach along them.
	type spec struct {
		at, u, v               [2]float64
		ru, rv, limitU, limitV float64
	}
	var specs [4]spec
	if reverse {
		specs = [4]spec{
			{[2]float64{x, y}, [2]float64{-1, 0}, [2]float64{0, 1}, tl[0], tl[1], limit(w, tl[0], tr[0]), limit(h, tl[1], bl[1])},
			{[2]float64{x, y1}, [2]float64{0, 1}, [2]float64{1, 0}, bl[1], bl[0], limit(h, bl[1], tl[1]), limit(w, bl[0], br[0])},
			{[2]float64{x1, y1}, [2]float64{1, 0}, [2]float64{0, -1}, br[0], br[1], limit(w, br[0], bl[0]), limit(h, br[1], tr[1])},
			{[2]float64{x1, y}, [2]float64{0, -1}, [2]float64{-1, 0}, tr[1], tr[0], limit(h, tr[1], br[1]), limit(w, tr[0], tl[0])},
		}
	} else {
		specs = [4]spec{
			{[2]float64{x1, y}, [2]float64{1, 0}, [2]float64{0, 1}, tr[0], tr[1], limit(w, tr[0], tl[0]), limit(h, tr[1], br[1])},
			{[2]float64{x1, y1}, [2]float64{0, 1}, [2]float64{-1, 0}, br[1], br[0], limit(h, br[1], tr[1]), limit(w, br[0], bl[0])},
			{[2]float64{x, y1}, [2]float64{-1, 0}, [2]float64{0, -1}, bl[0], bl[1], limit(w, bl[0], br[0]), limit(h, bl[1], tl[1])},
			{[2]float64{x, y}, [2]float64{0, -1}, [2]float64{1, 0}, tl[1], tl[0], limit(h, tl[1], bl[1]), limit(w, tl[0], tr[0])},
		}
	}
	move := func(i int, p [2]float64) Segment {
		if i == 0 {
			return Segment{Kind: SegmentMove, P: [3][2]float64{p}}
		}
		return Segment{Kind: SegmentLine, P: [3][2]float64{p}}
	}
	cubic := func(c1, c2, e [2]float64) Segment {
		return Segment{Kind: SegmentCubic, P: [3][2]float64{c1, c2, e}}
	}
	smoothing := 0.0
	if continuous {
		smoothing = Smoothing
	}
	for i, s := range specs {
		r := max(s.ru, s.rv)
		if r <= 0 {
			out = append(out, move(i, s.at))
			continue
		}
		// pt maps corner-frame coordinates (along u from the corner, along
		// v from it, each scaled to its own radius) to the plane.
		su, sv := s.ru/r, s.rv/r
		pt := func(a, b float64) [2]float64 {
			return [2]float64{s.at[0] + s.u[0]*a*su + s.v[0]*b*sv, s.at[1] + s.u[1]*a*su + s.v[1]*b*sv}
		}
		budget := r
		if su > 0 && sv > 0 {
			budget = max(min(s.limitU/su, s.limitV/sv), r)
		}
		k := cornerParams(r, smoothing, budget)
		out = append(out, move(i, pt(-k.p, 0)))
		transitions := k.a+k.b+k.c > 1e-9*r
		if transitions {
			out = append(out, cubic(pt(-k.p+k.a, 0), pt(-k.p+k.a+k.b, 0), pt(-k.p+k.a+k.b+k.c, k.d)))
		}
		// The arc, symmetric about the corner's diagonal, as one cubic; its
		// circle is centered at (-r, r) in the corner's frame.
		theta := k.arc
		hk := 4.0 / 3.0 * math.Tan(theta/4) * r
		start, end := -math.Pi/4-theta/2, -math.Pi/4+theta/2
		on := func(phi float64) [2]float64 { return [2]float64{-r + r*math.Cos(phi), r + r*math.Sin(phi)} }
		p0, p3 := on(start), on(end)
		t0 := [2]float64{-math.Sin(start), math.Cos(start)}
		t3 := [2]float64{-math.Sin(end), math.Cos(end)}
		out = append(out, cubic(pt(p0[0]+hk*t0[0], p0[1]+hk*t0[1]), pt(p3[0]-hk*t3[0], p3[1]-hk*t3[1]), pt(p3[0], p3[1])))
		if transitions {
			// The first transition mirrored about the diagonal, backwards.
			out = append(out, cubic(pt(0, k.p-k.a-k.b), pt(0, k.p-k.a), pt(0, k.p)))
		}
	}
	return append(out, Segment{Kind: SegmentClose})
}

// InnerRadii returns the radii of the padding box inside borders of
// widths (top, right, bottom, left) around a box with fitted corners:
// each corner less the widths of the sides it joins, elliptical where
// they differ, as CSS shrinks them.
func InnerRadii(c Corners, widths Edges) [4][2]float64 {
	r := c.Array()
	t, ri, b, l := float64(widths.Top), float64(widths.Right), float64(widths.Bottom), float64(widths.Left)
	in := func(r Points, hw, vw float64) [2]float64 {
		return [2]float64{max(float64(r)-hw, 0), max(float64(r)-vw, 0)}
	}
	return [4][2]float64{in(r[0], l, t), in(r[1], ri, t), in(r[2], ri, b), in(r[3], l, b)}
}

// corner is the shape of one corner of radius r: the transition reaches p
// along each edge, its cubic's handles are a and b, it ends c further
// along and d inwards, and the arc spans arc radians.
type corner struct{ p, a, b, c, d, arc float64 }

func cornerParams(r, smoothing, budget float64) corner {
	// Less smoothing where the edge is too short for all of it.
	smoothing = max(min(smoothing, budget/r-1), 0)
	p := min((1+smoothing)*r, budget)
	arc := math.Pi / 2 * (1 - smoothing)
	arcLen := math.Sin(arc/2) * r * math.Sqrt2
	alpha := (math.Pi/2 - arc) / 2
	p3p4 := r * math.Tan(alpha/2)
	beta := math.Pi / 4 * smoothing
	c := p3p4 * math.Cos(beta)
	d := c * math.Tan(beta)
	b := max((p-arcLen-c-d)/3, 0)
	return corner{p: p, a: 2 * b, b: b, c: c, d: d, arc: arc}
}
