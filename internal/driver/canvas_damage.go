package driver

import "math"

// CanvasDamage returns the part of a canvas that playing after instead of
// before changes, in the canvas's points, as NSH's DamageScan: before and
// after are one drawing's ops with its bound arguments at two moments, so
// they differ only in those arguments. It replays both for their
// geometry (the transform and the path, line widths, joins and caps,
// shadows) and adds up the bounds of the painting ops that changed or
// paint with state that did (the transform, the path, the alpha, the
// paint, the line's settings, the shadow), under both. Bounds are
// conservative: curves by their control points, arcs by their circle,
// strokes by half their width times the transform's largest scale, times
// the miter limit at miter joins or √2 at square caps, shadows by twice
// their blur past their offset, text by 1.5 em per character and 1.5 em
// above and below its baseline. It reports false where only a full
// redraw is right: a changed clip, changed painting under a composite
// operation other than source-over, which changes pixels outside its
// shape, an image at its natural size, or bounds that are not finite.
// A platform that redraws the area replays the whole drawing clipped to
// it, so the pixels are those of a full redraw.
func CanvasDamage(before, after []DrawOp) (Bounds, bool) {
	if len(before) != len(after) {
		return Bounds{}, false
	}
	var d damageScan
	d.reset()
	for i := range after {
		if !d.step(&before[i], &after[i], before[i] != after[i]) {
			return Bounds{}, false
		}
	}
	if d.empty {
		return Bounds{}, true
	}
	if !finite(d.box.x0) || !finite(d.box.y0) || !finite(d.box.x1) || !finite(d.box.y1) {
		return Bounds{}, false
	}
	return Bounds{Origin: Point{X: Points(d.box.x0), Y: Points(d.box.y0)},
		Size: Size{Width: Points(d.box.x1 - d.box.x0), Height: Points(d.box.y1 - d.box.y0)}}, true
}

func finite(v float64) bool { return !math.IsInf(v, 0) && !math.IsNaN(v) }

// box is a rectangle by its edges; an empty one has x0 > x1.
type box struct{ x0, y0, x1, y1 float64 }

var noBox = box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}

func (b *box) add(p cpoint) {
	b.x0, b.y0 = min(b.x0, p.x), min(b.y0, p.y)
	b.x1, b.y1 = max(b.x1, p.x), max(b.y1, p.y)
}

func (b box) union(o box) box {
	return box{min(b.x0, o.x0), min(b.y0, o.y0), max(b.x1, o.x1), max(b.y1, o.y1)}
}

func (b box) inflate(by float64) box {
	if b.x0 > b.x1 {
		return b
	}
	return box{b.x0 - by, b.y0 - by, b.x1 + by, b.y1 + by}
}

// damageState is the drawing state one replay paints with, and whether
// each part changed between the two replays.
type damageState struct {
	m                canvasMatrix
	width, miter     float64
	cap              LineCap
	join             LineJoin
	shadowX, shadowY float64
	shadowR          float64
	shadow           bool
	composite        CompositeOp
	// moved, painted and composed are set once the transform; the alpha,
	// paints, line or shadow; or the composite operation changed.
	moved, painted, composed bool
}

// damageScan replays two ops lists side by side.
type damageScan struct {
	cur   [2]damageState
	saved [][2]damageState
	// path is the current path's bounds in canvas points, per replay;
	// pathMoved whether it differs between them.
	path      [2]box
	pathMoved bool
	box       box
	empty     bool
}

func (d *damageScan) reset() {
	s := damageState{m: canvasIdentity, width: 1, miter: 10}
	d.cur = [2]damageState{s, s}
	d.path = [2]box{noBox, noBox}
	d.box, d.empty = noBox, true
}

// step plays op a of the first replay and b of the second, which differ
// when changed, and reports false when only a full redraw is right.
func (d *damageScan) step(a, b *DrawOp, changed bool) bool {
	ops := [2]*DrawOp{a, b}
	switch b.Kind {
	case DrawSave:
		d.saved = append(d.saved, d.cur)
	case DrawRestore:
		if n := len(d.saved); n > 0 {
			d.cur, d.saved = d.saved[n-1], d.saved[:n-1]
		}
	case DrawTranslate, DrawScale, DrawRotate, DrawTransform, DrawSetTransform:
		for i, op := range ops {
			if op.Kind == DrawSetTransform {
				d.cur[i].m = opMatrix(op)
			} else {
				d.cur[i].m = d.cur[i].m.then(local(op))
			}
			d.cur[i].moved = d.cur[i].moved || changed
		}
	case DrawBeginPath:
		d.path, d.pathMoved = [2]box{noBox, noBox}, false
	case DrawMoveTo, DrawLineTo, DrawQuadTo, DrawCubicTo, DrawArc, DrawRect, DrawRoundRect, DrawEllipse:
		for i, op := range ops {
			d.addPath(i, op)
		}
		d.pathMoved = d.pathMoved || changed || d.cur[1].moved
	case DrawClosePath:
	case DrawClip:
		if d.pathMoved || changed {
			return false
		}
	case DrawFillColor, DrawStrokeColor, DrawAlpha, DrawFillGradient, DrawStrokeGradient, DrawLineDash,
		DrawLineDashOffset:
		d.cur[0].painted = d.cur[0].painted || changed
		d.cur[1].painted = d.cur[1].painted || changed
	case DrawLineWidth, DrawLineCap, DrawLineJoin, DrawMiterLimit:
		for i, op := range ops {
			s := &d.cur[i]
			switch op.Kind {
			case DrawLineWidth:
				s.width = float64(op.W)
			case DrawLineCap:
				s.cap = op.Cap
			case DrawLineJoin:
				s.join = op.Join
			case DrawMiterLimit:
				s.miter = float64(op.W)
			}
			s.painted = s.painted || changed
		}
	case DrawShadow:
		for i, op := range ops {
			s := &d.cur[i]
			s.shadow = op.Color&0xff != 0
			s.shadowX, s.shadowY, s.shadowR = float64(op.X), float64(op.Y), float64(op.R)
			s.painted = s.painted || changed
		}
	case DrawComposite:
		for i, op := range ops {
			d.cur[i].composite = op.Composite
			d.cur[i].composed = d.cur[i].composed || changed
		}
	case DrawFill, DrawStroke:
		// A gradient maps through the transform as it paints.
		s := &d.cur[1]
		if !changed && !s.painted && !d.pathMoved && !s.moved && !s.composed {
			return true
		}
		return d.paint(func(i int) box {
			if b.Kind == DrawFill {
				return d.path[i]
			}
			return d.path[i].inflate(d.cur[i].strokeReach())
		})
	case DrawFillText, DrawStrokeText, DrawClear, DrawImage:
		s := &d.cur[1]
		if !changed && !s.painted && !s.moved && !s.composed {
			return true
		}
		if b.Kind == DrawImage && (b.W == 0 || b.H == 0 || a.W == 0 || a.H == 0) {
			return false
		}
		return d.paint(func(i int) box { return d.cur[i].opBox(ops[i]) })
	}
	return true
}

// paint adds what a painting op covers in both replays, extent being its
// bounds in replay i before its shadow.
func (d *damageScan) paint(extent func(i int) box) bool {
	for i := range d.cur {
		s := &d.cur[i]
		if s.composite != CompositeSourceOver {
			return false
		}
		b := extent(i)
		if s.shadow {
			reach := 2 * s.shadowR
			b = b.union(box{b.x0 + s.shadowX - reach, b.y0 + s.shadowY - reach, b.x1 + s.shadowX + reach,
				b.y1 + s.shadowY + reach})
		}
		if b.x0 > b.x1 {
			continue
		}
		d.box, d.empty = d.box.union(b), false
	}
	return true
}

// addPath adds what op adds to replay i's path.
func (d *damageScan) addPath(i int, op *DrawOp) {
	m, p := d.cur[i].m, &d.path[i]
	pt := func(x, y float32) { p.add(m.apply(cpoint{float64(x), float64(y)})) }
	rect := func(x0, y0, x1, y1 float32) {
		pt(x0, y0)
		pt(x1, y0)
		pt(x0, y1)
		pt(x1, y1)
	}
	switch op.Kind {
	case DrawMoveTo, DrawLineTo:
		pt(op.X, op.Y)
	case DrawQuadTo:
		pt(op.X1, op.Y1)
		pt(op.X, op.Y)
	case DrawCubicTo:
		pt(op.X1, op.Y1)
		pt(op.X2, op.Y2)
		pt(op.X, op.Y)
	case DrawArc:
		r := float32(math.Abs(float64(op.R)))
		rect(op.X-r, op.Y-r, op.X+r, op.Y+r)
	case DrawRect, DrawRoundRect:
		rect(op.X, op.Y, op.X+op.W, op.Y+op.H)
	case DrawEllipse:
		rx, ry := float32(math.Abs(float64(op.R))), float32(math.Abs(float64(op.RY)))
		rect(op.X-rx, op.Y-ry, op.X+rx, op.Y+ry)
	}
}

// scale is the largest factor m stretches a length by.
func (m canvasMatrix) scale() float64 {
	return max(math.Hypot(m.a, m.b), math.Hypot(m.c, m.d))
}

// strokeReach is how far a stroke reaches past its path.
func (s *damageState) strokeReach() float64 {
	reach := s.width / 2 * s.m.scale()
	switch {
	case s.join == JoinMiter:
		reach *= max(s.miter, 1)
	case s.cap == CapSquare:
		reach *= math.Sqrt2
	}
	return reach
}

// opBox is what a text, clear or image op covers.
func (s *damageState) opBox(op *DrawOp) box {
	b := noBox
	rect := func(x0, y0, x1, y1 float32) {
		for _, c := range [4]cpoint{{float64(x0), float64(y0)}, {float64(x1), float64(y0)},
			{float64(x0), float64(y1)}, {float64(x1), float64(y1)}} {
			b.add(s.m.apply(c))
		}
	}
	switch op.Kind {
	case DrawFillText, DrawStrokeText:
		em := float32(op.Font.Size)
		if em <= 0 {
			em = 10
		}
		x, y := op.X+op.X1, op.Y+op.Y1
		n := float32(len([]rune(op.Text)))
		rect(x-1.5*em, y-1.5*em, x+1.5*em*max(n, 1), y+1.5*em)
		if op.Kind == DrawStrokeText {
			b = b.inflate(s.strokeReach())
		}
	default:
		rect(op.X, op.Y, op.X+op.W, op.Y+op.H)
	}
	return b
}
