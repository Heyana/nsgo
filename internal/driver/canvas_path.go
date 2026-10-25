package driver

import "math"

// Path is a path to fill, stroke or clip with, as the web's Path2D: it
// records MoveTo, LineTo and the shapes in its own coordinates, which
// take the drawing's transform where it is used. Its arcs of ellipses and
// per-corner rectangles are cubic Béziers.
type Path struct {
	ops   []DrawOp
	track *drawTrack
}

// ParsePath reads SVG path data ("M 0 0 L 10 10 Z") into a path. It
// understands every command of SVG 1.1's path grammar; arcs become cubic
// curves.
func ParsePath(data string) (*Path, error) {
	ops, err := appendPathData(nil, data)
	if err != nil {
		return nil, err
	}
	return &Path{ops: ops}, nil
}

// MustParsePath is ParsePath for path data that is part of the program:
// it panics when the data is in error.
func MustParsePath(data string) *Path {
	p, err := ParsePath(data)
	if err != nil {
		panic(err)
	}
	return p
}

func (p *Path) op(o DrawOp) *Path { p.ops = append(p.ops, o); return p }

func (p *Path) MoveTo(x, y float32) *Path { return p.op(DrawOp{Kind: DrawMoveTo, X: x, Y: y}) }

func (p *Path) LineTo(x, y float32) *Path { return p.op(DrawOp{Kind: DrawLineTo, X: x, Y: y}) }

// QuadTo adds a quadratic curve through control point (cx, cy).
func (p *Path) QuadTo(cx, cy, x, y float32) *Path {
	return p.op(DrawOp{Kind: DrawQuadTo, X1: cx, Y1: cy, X: x, Y: y})
}

// CubicTo adds a cubic curve through control points 1 and 2.
func (p *Path) CubicTo(x1, y1, x2, y2, x, y float32) *Path {
	return p.op(DrawOp{Kind: DrawCubicTo, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
}

func (p *Path) ClosePath() *Path { return p.op(DrawOp{Kind: DrawClosePath}) }

// Arc adds an arc of radius r around (x, y) from angle a0 to a1, joined
// to the path by a line.
func (p *Path) Arc(x, y, r, a0, a1 float32, ccw bool) *Path {
	return p.op(DrawOp{Kind: DrawArc, X: x, Y: y, R: r, A0: a0, A1: a1, CCW: ccw})
}

// ArcTo adds an arc as Drawing.ArcTo does.
func (p *Path) ArcTo(x1, y1, x2, y2, r float32) *Path {
	t := p.tracked()
	if !t.has {
		return p.MoveTo(x1, y1)
	}
	p.ops = appendArcTo(p.ops, t.cur, x1, y1, x2, y2, r)
	return p
}

// EllipseArc adds an arc of an ellipse as Drawing.EllipseArc does.
func (p *Path) EllipseArc(x, y, rx, ry, rotation, a0, a1 float32, ccw bool) *Path {
	p.ops = appendEllipseArc(p.ops, p.tracked().has, x, y, rx, ry, rotation, a0, a1, ccw)
	return p
}

// Ellipse adds the whole ellipse around (x, y) with radii rx and ry.
func (p *Path) Ellipse(x, y, rx, ry float32) *Path {
	return p.op(DrawOp{Kind: DrawEllipse, X: x, Y: y, R: rx, RY: ry})
}

func (p *Path) Rect(x, y, w, h float32) *Path {
	return p.op(DrawOp{Kind: DrawRect, X: x, Y: y, W: w, H: h})
}

// RoundRect adds a rectangle with corners of radius r.
func (p *Path) RoundRect(x, y, w, h, r float32) *Path {
	return p.op(DrawOp{Kind: DrawRoundRect, X: x, Y: y, W: w, H: h, R: r})
}

// RoundRectCorners adds a rectangle as Drawing.RoundRectCorners does.
func (p *Path) RoundRectCorners(x, y, w, h, topLeft, topRight, bottomRight, bottomLeft float32) *Path {
	p.ops = appendRoundRect(p.ops, x, y, w, h, [4]float32{topLeft, topRight, bottomRight, bottomLeft})
	return p
}

// AddPath adds q's subpaths to p.
func (p *Path) AddPath(q *Path) *Path {
	if q != nil {
		p.ops = append(p.ops, q.ops...)
	}
	return p
}

// cpoint is a point in a drawing's math.
type cpoint struct{ x, y float64 }

// canvasMatrix is a 2D affine transform, mapping (x, y) to
// (a*x + c*y + e, b*x + d*y + f).
type canvasMatrix struct{ a, b, c, d, e, f float64 }

var canvasIdentity = canvasMatrix{a: 1, d: 1}

func opMatrix(op *DrawOp) canvasMatrix {
	return canvasMatrix{float64(op.X1), float64(op.Y1), float64(op.X2), float64(op.Y2), float64(op.X), float64(op.Y)}
}

// then returns the transform that applies n, then m: m×n.
func (m canvasMatrix) then(n canvasMatrix) canvasMatrix {
	return canvasMatrix{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

func (m canvasMatrix) apply(p cpoint) cpoint {
	return cpoint{m.a*p.x + m.c*p.y + m.e, m.b*p.x + m.d*p.y + m.f}
}

func (m canvasMatrix) invert() (canvasMatrix, bool) {
	det := m.a*m.d - m.b*m.c
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return canvasMatrix{}, false
	}
	return canvasMatrix{
		a: m.d / det, b: -m.b / det, c: -m.c / det, d: m.a / det,
		e: (m.c*m.f - m.d*m.e) / det, f: (m.b*m.e - m.a*m.f) / det,
	}, true
}

// local is the transform a transform op multiplies by.
func local(op *DrawOp) canvasMatrix {
	switch op.Kind {
	case DrawTranslate:
		return canvasMatrix{a: 1, d: 1, e: float64(op.X), f: float64(op.Y)}
	case DrawScale:
		return canvasMatrix{a: float64(op.X), d: float64(op.Y)}
	case DrawRotate:
		sin, cos := math.Sincos(float64(op.A))
		return canvasMatrix{a: cos, b: sin, c: -sin, d: cos}
	}
	return opMatrix(op)
}

// appendArcTo appends the web's arcTo from p0, the current point in the
// coordinates of the arguments: an arc touching the lines p0-p1 and
// p1-p2, joined by a line, or a line to p1 where there is no arc.
func appendArcTo(ops []DrawOp, p0 cpoint, x1, y1, x2, y2, r float32) []DrawOp {
	if r < 0 {
		return ops
	}
	p1 := cpoint{float64(x1), float64(y1)}
	p2 := cpoint{float64(x2), float64(y2)}
	ax, ay := p0.x-p1.x, p0.y-p1.y
	bx, by := p2.x-p1.x, p2.y-p1.y
	la, lb := math.Hypot(ax, ay), math.Hypot(bx, by)
	cross := ax*by - ay*bx
	if r == 0 || la == 0 || lb == 0 || math.Abs(cross) < 1e-9*la*lb {
		return append(ops, DrawOp{Kind: DrawLineTo, X: x1, Y: y1})
	}
	// The angle at p1 between the two lines, and how far from p1 the arc
	// touches them.
	cos := (ax*bx + ay*by) / (la * lb)
	theta := math.Acos(min(max(cos, -1), 1))
	dist := float64(r) / math.Tan(theta/2)
	ux, uy := ax/la, ay/la
	vx, vy := bx/lb, by/lb
	t1 := cpoint{p1.x + ux*dist, p1.y + uy*dist}
	t2 := cpoint{p1.x + vx*dist, p1.y + vy*dist}
	// The center is along the bisector, r/sin(theta/2) from p1.
	bisx, bisy := ux+vx, uy+vy
	bl := math.Hypot(bisx, bisy)
	h := float64(r) / math.Sin(theta/2)
	c := cpoint{p1.x + bisx/bl*h, p1.y + bisy/bl*h}
	a0 := math.Atan2(t1.y-c.y, t1.x-c.x)
	a1 := math.Atan2(t2.y-c.y, t2.x-c.x)
	// A turn clockwise on screen (y down) has a negative cross product
	// and an arc of increasing angles.
	return append(ops, DrawOp{Kind: DrawArc, X: float32(c.x), Y: float32(c.y), R: r,
		A0: float32(a0), A1: float32(a1), CCW: cross > 0})
}

// arcSweep is the web's sweep of an arc from a0 to a1.
func arcSweep(a0, a1 float64, ccw bool) float64 {
	const full = 2 * math.Pi
	mod := func(v float64) float64 { return v - full*math.Floor(v/full) }
	switch {
	case !ccw && a1-a0 >= full:
		return full
	case !ccw:
		return mod(a1 - a0)
	case a0-a1 >= full:
		return -full
	}
	return -mod(a0 - a1)
}

// appendEllipseArc appends an arc of the ellipse around (x, y) turned by
// rotation as cubic curves, after a line to its start, or a move there
// when the path has no current point.
func appendEllipseArc(ops []DrawOp, has bool, x, y, rx, ry, rotation, a0, a1 float32, ccw bool) []DrawOp {
	if rx < 0 || ry < 0 {
		return ops
	}
	sweep := arcSweep(float64(a0), float64(a1), ccw)
	sinR, cosR := math.Sincos(float64(rotation))
	cx, cy, frx, fry := float64(x), float64(y), float64(rx), float64(ry)
	at := func(t float64) (px, py, dx, dy float64) {
		s, c := math.Sincos(t)
		ex, ey := frx*c, fry*s
		px, py = cx+ex*cosR-ey*sinR, cy+ex*sinR+ey*cosR
		tx, ty := -frx*s, fry*c
		dx, dy = tx*cosR-ty*sinR, tx*sinR+ty*cosR
		return
	}
	sx, sy, _, _ := at(float64(a0))
	kind := DrawLineTo
	if !has {
		kind = DrawMoveTo
	}
	ops = append(ops, DrawOp{Kind: kind, X: float32(sx), Y: float32(sy)})
	if sweep == 0 {
		return ops
	}
	n := max(int(math.Ceil(math.Abs(sweep)/(math.Pi/2)-1e-6)), 1)
	step := sweep / float64(n)
	k := 4.0 / 3 * math.Tan(step/4)
	t := float64(a0)
	for range n {
		x0, y0, d0x, d0y := at(t)
		x1, y1, d1x, d1y := at(t + step)
		ops = append(ops, DrawOp{Kind: DrawCubicTo,
			X1: float32(x0 + k*d0x), Y1: float32(y0 + k*d0y),
			X2: float32(x1 - k*d1x), Y2: float32(y1 - k*d1y),
			X: float32(x1), Y: float32(y1)})
		t += step
	}
	return ops
}

// appendRoundRect appends the web's roundRect: a closed subpath with a
// radius per corner from the top left one clockwise, scaled down together
// where they overlap, then a subpath at (x, y).
func appendRoundRect(ops []DrawOp, x, y, w, h float32, r [4]float32) []DrawOp {
	for i, v := range r {
		r[i] = max(v, 0)
	}
	if w < 0 {
		x, w = x+w, -w
		r[0], r[1], r[2], r[3] = r[1], r[0], r[3], r[2]
	}
	if h < 0 {
		y, h = y+h, -h
		r[0], r[1], r[2], r[3] = r[3], r[2], r[1], r[0]
	}
	scale := float32(1)
	for _, f := range [...]struct{ side, a, b float32 }{
		{w, r[0], r[1]}, {h, r[1], r[2]}, {w, r[2], r[3]}, {h, r[3], r[0]},
	} {
		if s := f.a + f.b; s > f.side && s > 0 {
			scale = min(scale, f.side/s)
		}
	}
	for i := range r {
		r[i] *= scale
	}
	ops = append(ops, DrawOp{Kind: DrawMoveTo, X: x + r[0], Y: y},
		DrawOp{Kind: DrawLineTo, X: x + w - r[1], Y: y})
	ops = appendCorner(ops, x+w-r[1], y+r[1], r[1], r[1], -90, 0)
	ops = append(ops, DrawOp{Kind: DrawLineTo, X: x + w, Y: y + h - r[2]})
	ops = appendCorner(ops, x+w-r[2], y+h-r[2], r[2], r[2], 0, 90)
	ops = append(ops, DrawOp{Kind: DrawLineTo, X: x + r[3], Y: y + h})
	ops = appendCorner(ops, x+r[3], y+h-r[3], r[3], r[3], 90, 180)
	ops = append(ops, DrawOp{Kind: DrawLineTo, X: x, Y: y + r[0]})
	ops = appendCorner(ops, x+r[0], y+r[0], r[0], r[0], 180, 270)
	return append(ops, DrawOp{Kind: DrawClosePath}, DrawOp{Kind: DrawMoveTo, X: x, Y: y})
}
