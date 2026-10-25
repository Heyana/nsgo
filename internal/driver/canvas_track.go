package driver

import "math"

// drawTrack replays a recording for its geometry: the transform with its
// saves, the current path in the canvas's own points and the line style,
// for the operations that read them and for the queries, as NSH's
// point_in_path replays what was recorded.
type drawTrack struct {
	at    int // the ops replayed
	st    trackState
	stack []trackState
	segs  []trackSeg
	// pathOps are the ops that built the current path, with the
	// transform each was recorded under.
	pathOps    []pathOp
	cur, start cpoint // in the canvas's points
	has        bool
}

type trackState struct {
	m        canvasMatrix
	width    float64
	miter    float64
	cap      LineCap
	join     LineJoin
	align    TextAlign
	baseline TextBaseline
}

type pathOp struct {
	op int
	m  canvasMatrix
}

const (
	segMoveTo uint8 = iota
	segLineTo
	segCubicTo
	segClose
)

// trackSeg is a piece of the current path in the canvas's points.
type trackSeg struct {
	kind uint8
	p    [3]cpoint
}

func newTrack() *drawTrack {
	return &drawTrack{st: trackState{m: canvasIdentity, width: 1, miter: 10, align: TextAlignStart}}
}

// tracked replays the ops recorded since the last call.
func (d *Drawing) tracked() *drawTrack {
	if d.track == nil {
		d.track = newTrack()
	}
	t := d.track
	for ; t.at < len(d.Ops); t.at++ {
		t.replay(&d.Ops[t.at], t.at)
	}
	return t
}

// tracked replays the path's ops in its own coordinates.
func (p *Path) tracked() *drawTrack {
	if p.track == nil {
		p.track = newTrack()
	}
	t := p.track
	for ; t.at < len(p.ops); t.at++ {
		t.replay(&p.ops[t.at], t.at)
	}
	return t
}

func (t *drawTrack) replay(op *DrawOp, at int) {
	switch op.Kind {
	case DrawSave:
		t.stack = append(t.stack, t.st)
	case DrawRestore:
		if n := len(t.stack); n > 0 {
			t.st = t.stack[n-1]
			t.stack = t.stack[:n-1]
		}
	case DrawTranslate, DrawScale, DrawRotate, DrawTransform:
		t.st.m = t.st.m.then(local(op))
	case DrawSetTransform:
		t.st.m = opMatrix(op)
	case DrawLineWidth:
		t.st.width = float64(op.W)
	case DrawLineCap:
		t.st.cap = op.Cap
	case DrawLineJoin:
		t.st.join = op.Join
	case DrawMiterLimit:
		t.st.miter = float64(op.W)
	case DrawTextAlign:
		t.st.align = op.TextAlign
	case DrawTextBaseline:
		t.st.baseline = op.TextBaseline
	case DrawBeginPath:
		t.segs, t.pathOps, t.has = t.segs[:0], t.pathOps[:0], false
	case DrawMoveTo, DrawLineTo, DrawQuadTo, DrawCubicTo, DrawArc, DrawRect, DrawRoundRect, DrawEllipse,
		DrawClosePath:
		t.pathOps = append(t.pathOps, pathOp{op: at, m: t.st.m})
		t.add(op, t.st.m)
	}
}

// add adds a path op to the current path through m.
func (t *drawTrack) add(op *DrawOp, m canvasMatrix) {
	pt := func(x, y float32) cpoint { return m.apply(cpoint{float64(x), float64(y)}) }
	switch op.Kind {
	case DrawMoveTo:
		t.moveTo(pt(op.X, op.Y))
	case DrawLineTo:
		t.lineTo(pt(op.X, op.Y))
	case DrawQuadTo:
		q := pt(op.X1, op.Y1)
		if !t.has {
			t.moveTo(q)
		}
		e := pt(op.X, op.Y)
		c1 := cpoint{t.cur.x + 2.0/3*(q.x-t.cur.x), t.cur.y + 2.0/3*(q.y-t.cur.y)}
		c2 := cpoint{e.x + 2.0/3*(q.x-e.x), e.y + 2.0/3*(q.y-e.y)}
		t.cubic(c1, c2, e)
	case DrawCubicTo:
		if !t.has {
			t.moveTo(pt(op.X1, op.Y1))
		}
		t.cubic(pt(op.X1, op.Y1), pt(op.X2, op.Y2), pt(op.X, op.Y))
	case DrawArc:
		for _, o := range appendEllipseArc(nil, t.has, op.X, op.Y, op.R, op.R, 0, op.A0, op.A1, op.CCW) {
			t.add(&o, m)
		}
	case DrawRect:
		x, y, w, h := op.X, op.Y, op.W, op.H
		t.moveTo(pt(x, y))
		t.lineTo(pt(x+w, y))
		t.lineTo(pt(x+w, y+h))
		t.lineTo(pt(x, y+h))
		t.closePath()
		t.moveTo(pt(x, y))
	case DrawRoundRect:
		r := op.R
		for _, o := range appendRoundRect(nil, op.X, op.Y, op.W, op.H, [4]float32{r, r, r, r}) {
			t.add(&o, m)
		}
	case DrawEllipse:
		t.moveTo(pt(op.X+op.R, op.Y))
		for _, o := range appendEllipseArc(nil, true, op.X, op.Y, op.R, op.RY, 0, 0, 2*math.Pi, false)[1:] {
			t.add(&o, m)
		}
		t.closePath()
	case DrawClosePath:
		t.closePath()
	}
}

func (t *drawTrack) moveTo(p cpoint) {
	t.cur, t.start, t.has = p, p, true
	t.segs = append(t.segs, trackSeg{kind: segMoveTo, p: [3]cpoint{p}})
}

func (t *drawTrack) lineTo(p cpoint) {
	if !t.has {
		t.moveTo(p)
		return
	}
	t.cur = p
	t.segs = append(t.segs, trackSeg{kind: segLineTo, p: [3]cpoint{p}})
}

func (t *drawTrack) cubic(c1, c2, e cpoint) {
	t.cur = e
	t.segs = append(t.segs, trackSeg{kind: segCubicTo, p: [3]cpoint{c1, c2, e}})
}

func (t *drawTrack) closePath() {
	if t.has {
		t.segs = append(t.segs, trackSeg{kind: segClose})
		t.cur = t.start
	}
}

// IsPointInPath reports whether the point (x, y), in the canvas's own
// points, is inside the current path by rule, as the web's
// isPointInPath. After Draw returns, a kept drawing answers from all it
// recorded, so press handlers can hit-test its shapes. Bound arguments
// count as recorded.
func (d *Drawing) IsPointInPath(x, y float32, rule FillRule) bool {
	return inside(flatten(d.tracked().segs), cpoint{float64(x), float64(y)}, rule)
}

// IsPointInStroke reports whether the point is on the current path's
// stroke with the line width, caps and transform in effect; joins count
// as round and dashes as solid.
func (d *Drawing) IsPointInStroke(x, y float32) bool {
	t := d.tracked()
	return onStroke(flatten(t.segs), cpoint{float64(x), float64(y)}, t.st)
}

// PathContains reports whether the point is inside p under the current
// transform, by rule.
func (d *Drawing) PathContains(p *Path, x, y float32, rule FillRule) bool {
	return inside(flatten(d.pathSegs(p)), cpoint{float64(x), float64(y)}, rule)
}

// StrokeContains reports whether the point is on p's stroke under the
// current transform and line style.
func (d *Drawing) StrokeContains(p *Path, x, y float32) bool {
	return onStroke(flatten(d.pathSegs(p)), cpoint{float64(x), float64(y)}, d.tracked().st)
}

// pathSegs returns p's segments in the canvas's points under the current
// transform.
func (d *Drawing) pathSegs(p *Path) []trackSeg {
	if p == nil {
		return nil
	}
	m := d.tracked().st.m
	t := &drawTrack{}
	for i := range p.ops {
		t.add(&p.ops[i], m)
	}
	return t.segs
}

// flatten turns segments into polylines, one per subpath, and whether
// each is closed.
func flatten(segs []trackSeg) []polyline {
	var out []polyline
	var cur *polyline
	var last cpoint
	for _, s := range segs {
		switch s.kind {
		case segMoveTo:
			out = append(out, polyline{pts: []cpoint{s.p[0]}})
			cur = &out[len(out)-1]
			last = s.p[0]
		case segLineTo:
			if cur != nil {
				cur.pts = append(cur.pts, s.p[0])
				last = s.p[0]
			}
		case segCubicTo:
			if cur == nil {
				continue
			}
			p0, p1, p2, p3 := last, s.p[0], s.p[1], s.p[2]
			l := math.Hypot(p1.x-p0.x, p1.y-p0.y) + math.Hypot(p2.x-p1.x, p2.y-p1.y) + math.Hypot(p3.x-p2.x, p3.y-p2.y)
			n := min(max(int(math.Ceil(l/2)), 2), 64)
			for i := 1; i <= n; i++ {
				u := float64(i) / float64(n)
				v := 1 - u
				a, b, c, e := v*v*v, 3*v*v*u, 3*v*u*u, u*u*u
				cur.pts = append(cur.pts, cpoint{a*p0.x + b*p1.x + c*p2.x + e*p3.x, a*p0.y + b*p1.y + c*p2.y + e*p3.y})
			}
			last = p3
		case segClose:
			if cur != nil {
				cur.closed = true
				start := cur.pts[0]
				out = append(out, polyline{pts: []cpoint{start}})
				cur = &out[len(out)-1]
				last = start
			}
		}
	}
	return out
}

type polyline struct {
	pts    []cpoint
	closed bool
}

// inside tests p against every subpath closed, by rule.
func inside(lines []polyline, p cpoint, rule FillRule) bool {
	winding := 0
	for _, l := range lines {
		n := len(l.pts)
		for i := range n {
			a, b := l.pts[i], l.pts[(i+1)%n]
			if a.y <= p.y {
				if b.y > p.y && cross(a, b, p) > 0 {
					winding++
				}
			} else if b.y <= p.y && cross(a, b, p) < 0 {
				winding--
			}
		}
	}
	if rule == EvenOdd {
		return winding%2 != 0
	}
	return winding != 0
}

func cross(a, b, p cpoint) float64 { return (b.x-a.x)*(p.y-a.y) - (p.x-a.x)*(b.y-a.y) }

// onStroke tests p against the stroke of lines in the user space of st:
// within half the line width of a segment, with st's caps at the ends of
// open subpaths and round joins.
func onStroke(lines []polyline, p cpoint, st trackState) bool {
	inv, ok := st.m.invert()
	if !ok || st.width <= 0 {
		return false
	}
	p = inv.apply(p)
	hw := st.width / 2
	for _, l := range lines {
		if len(l.pts) < 2 {
			continue
		}
		pts := make([]cpoint, len(l.pts))
		for i, q := range l.pts {
			pts[i] = inv.apply(q)
		}
		if l.closed {
			pts = append(pts, pts[0])
		}
		last := len(pts) - 2
		for i := 0; i <= last; i++ {
			a, b := pts[i], pts[i+1]
			dx, dy := b.x-a.x, b.y-a.y
			l2 := dx*dx + dy*dy
			if l2 == 0 {
				continue
			}
			u := ((p.x-a.x)*dx + (p.y-a.y)*dy) / l2
			end := !l.closed && (i == 0 && u < 0 || i == last && u > 1)
			if end {
				switch st.cap {
				case CapButt:
					continue
				case CapSquare:
					along := math.Abs(u-min(max(u, 0), 1)) * math.Sqrt(l2)
					perp := math.Abs(dx*(p.y-a.y)-dy*(p.x-a.x)) / math.Sqrt(l2)
					if along <= hw && perp <= hw {
						return true
					}
					continue
				}
			}
			u = min(max(u, 0), 1)
			q := cpoint{a.x + u*dx, a.y + u*dy}
			if math.Hypot(p.x-q.x, p.y-q.y) <= hw {
				return true
			}
		}
	}
	return false
}
