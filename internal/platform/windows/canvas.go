//go:build windows

package windows

import (
	"math"
	"unicode/utf16"
	"unsafe"

	"github.com/oneo-me/nsgo/internal/driver"
)

// canvasState is what Save and Restore keep.
type canvasState struct {
	ctm          matrix // the drawing's transform, to the view's points
	fill, stroke driver.Color
	width        float32
	cap          driver.LineCap
	join         driver.LineJoin
	alpha        float32
	clips        int // the clip layers pushed since the last Save
}

type segKind uint8

const (
	segMove segKind = iota
	segLine
	segCubic
	segClose
)

// segment is a piece of the path, in the view's points.
type segment struct {
	kind segKind
	pts  [3]pointF
}

// canvasPlayer plays a Drawing as the web's 2D context does: it builds
// the path in the view's points, as each operation's transform maps it,
// and has Direct2D fill, stroke or clip to it.
type canvasPlayer struct {
	p       *painter
	world   matrix // the view's
	st      canvasState
	stack   []canvasState
	path    []segment
	cur     pointF
	start   pointF
	has     bool
	strokes map[[2]uint32]*comObject
	behind  driver.Color
}

// canvas plays v's drawing into its width by height points.
func (p *painter) canvas(v *view, width, height float32) {
	c := &canvasPlayer{
		p:       p,
		world:   p.m,
		st:      canvasState{ctm: identity, fill: 0x000000ff, stroke: 0x000000ff, width: 1, alpha: 1},
		strokes: map[[2]uint32]*comObject{},
		behind:  background(v),
	}
	for i := range v.drawing.Ops {
		c.play(&v.drawing.Ops[i])
	}
	for n := c.st.clips; n > 0; n-- {
		p.pop()
	}
	for _, s := range c.stack {
		for n := s.clips; n > 0; n-- {
			p.pop()
		}
	}
	for _, s := range c.strokes {
		s.release()
	}
	p.setTransform(c.world)
}

func (c *canvasPlayer) play(op *driver.DrawOp) {
	x, y := float64(op.X), float64(op.Y)
	switch op.Kind {
	case driver.DrawSave:
		c.stack = append(c.stack, c.st)
		c.st.clips = 0
	case driver.DrawRestore:
		if n := len(c.stack); n > 0 {
			for ; c.st.clips > 0; c.st.clips-- {
				c.p.pop()
			}
			c.st = c.stack[n-1]
			c.stack = c.stack[:n-1]
		}
	case driver.DrawTranslate:
		c.st.ctm = translation(x, y).then(c.st.ctm)
	case driver.DrawScale:
		c.st.ctm = scaling(x, y).then(c.st.ctm)
	case driver.DrawRotate:
		sin, cos := math.Sincos(float64(op.A))
		c.st.ctm = matrix{a: cos, b: sin, c: -sin, d: cos}.then(c.st.ctm)
	case driver.DrawBeginPath:
		c.path, c.has = c.path[:0], false
	case driver.DrawMoveTo:
		c.moveTo(x, y)
	case driver.DrawLineTo:
		c.lineTo(x, y)
	case driver.DrawQuadTo:
		// The cubic with the same curve; affine maps keep it.
		if !c.has {
			c.moveTo(float64(op.X1), float64(op.Y1))
		}
		q := c.toView(float64(op.X1), float64(op.Y1))
		e := c.toView(x, y)
		c1 := pointF{c.cur.x + 2.0/3*(q.x-c.cur.x), c.cur.y + 2.0/3*(q.y-c.cur.y)}
		c2 := pointF{e.x + 2.0/3*(q.x-e.x), e.y + 2.0/3*(q.y-e.y)}
		c.cubic(c1, c2, e)
	case driver.DrawCubicTo:
		if !c.has {
			c.moveTo(float64(op.X1), float64(op.Y1))
		}
		c.cubic(c.toView(float64(op.X1), float64(op.Y1)), c.toView(float64(op.X2), float64(op.Y2)), c.toView(x, y))
	case driver.DrawArc:
		c.arc(op)
	case driver.DrawRect:
		w, h := float64(op.W), float64(op.H)
		c.moveTo(x, y)
		c.lineTo(x+w, y)
		c.lineTo(x+w, y+h)
		c.lineTo(x, y+h)
		c.closePath()
		c.moveTo(x, y)
	case driver.DrawRoundRect:
		w, h := float64(op.W), float64(op.H)
		r := min(float64(op.R), math.Abs(w)/2, math.Abs(h)/2)
		c.moveTo(x+r, y)
		c.lineTo(x+w-r, y)
		c.ellipseArc(x+w-r, y+r, r, r, -math.Pi/2, math.Pi/2)
		c.lineTo(x+w, y+h-r)
		c.ellipseArc(x+w-r, y+h-r, r, r, 0, math.Pi/2)
		c.lineTo(x+r, y+h)
		c.ellipseArc(x+r, y+h-r, r, r, math.Pi/2, math.Pi/2)
		c.lineTo(x, y+r)
		c.ellipseArc(x+r, y+r, r, r, math.Pi, math.Pi/2)
		c.closePath()
	case driver.DrawEllipse:
		rx, ry := float64(op.R), float64(op.RY)
		c.moveTo(x+rx, y)
		c.ellipseArc(x, y, rx, ry, 0, 2*math.Pi)
		c.closePath()
	case driver.DrawClosePath:
		c.closePath()
	case driver.DrawFill:
		c.fill()
	case driver.DrawStroke:
		c.stroke()
	case driver.DrawClip:
		if g := c.geometry(); g != nil {
			c.p.pushLayer(1, g)
			g.release()
			c.st.clips++
		}
	case driver.DrawFillColor:
		c.st.fill = op.Color
	case driver.DrawStrokeColor:
		c.st.stroke = op.Color
	case driver.DrawLineWidth:
		c.st.width = op.W
	case driver.DrawLineCap:
		c.st.cap = op.Cap
	case driver.DrawLineJoin:
		c.st.join = op.Join
	case driver.DrawAlpha:
		c.st.alpha = min(max(op.A, 0), 1)
	case driver.DrawFillText:
		c.text(op)
	case driver.DrawClear:
		// What is behind cannot show through: clearing paints the
		// background behind the canvas.
		path, has, cur, start := c.path, c.has, c.cur, c.start
		c.path, c.has = nil, false
		c.play(&driver.DrawOp{Kind: driver.DrawRect, X: op.X, Y: op.Y, W: op.W, H: op.H})
		if g := c.geometry(); g != nil {
			c.p.fillGeometry(g, c.behind|0xff)
			g.release()
		}
		c.path, c.has, c.cur, c.start = path, has, cur, start
	}
}

func (c *canvasPlayer) toView(x, y float64) pointF {
	px, py := c.st.ctm.apply(x, y)
	return pointF{float32(px), float32(py)}
}

func (c *canvasPlayer) moveTo(x, y float64) {
	c.cur = c.toView(x, y)
	c.start, c.has = c.cur, true
	c.path = append(c.path, segment{kind: segMove, pts: [3]pointF{c.cur}})
}

func (c *canvasPlayer) lineTo(x, y float64) {
	if !c.has {
		c.moveTo(x, y)
		return
	}
	c.cur = c.toView(x, y)
	c.path = append(c.path, segment{kind: segLine, pts: [3]pointF{c.cur}})
}

func (c *canvasPlayer) cubic(c1, c2, end pointF) {
	c.cur = end
	c.path = append(c.path, segment{kind: segCubic, pts: [3]pointF{c1, c2, end}})
}

func (c *canvasPlayer) closePath() {
	if c.has {
		c.path = append(c.path, segment{kind: segClose})
		c.cur = c.start
	}
}

// ellipseArc adds Bézier curves along an ellipse around (cx, cy) from
// angle a0 by sweep, from where the path is.
func (c *canvasPlayer) ellipseArc(cx, cy, rx, ry, a0, sweep float64) {
	n := max(int(math.Ceil(math.Abs(sweep)/(math.Pi/2)-1e-9)), 1)
	step := sweep / float64(n)
	k := 4.0 / 3 * math.Tan(step/4)
	a := a0
	for range n {
		sin0, cos0 := math.Sincos(a)
		sin1, cos1 := math.Sincos(a + step)
		c.cubic(
			c.toView(cx+rx*(cos0-k*sin0), cy+ry*(sin0+k*cos0)),
			c.toView(cx+rx*(cos1+k*sin1), cy+ry*(sin1-k*cos1)),
			c.toView(cx+rx*cos1, cy+ry*sin1))
		a += step
	}
}

// arc adds an arc as the web's arc() does: joined to the path by a line.
func (c *canvasPlayer) arc(op *driver.DrawOp) {
	x, y, r := float64(op.X), float64(op.Y), float64(op.R)
	a0, a1 := float64(op.A0), float64(op.A1)
	const full = 2 * math.Pi
	mod := func(v float64) float64 { return v - full*math.Floor(v/full) }
	var sweep float64
	switch {
	case !op.CCW && a1-a0 >= full:
		sweep = full
	case !op.CCW:
		sweep = mod(a1 - a0)
	case a0-a1 >= full:
		sweep = -full
	default:
		sweep = -mod(a0 - a1)
	}
	sx, sy := x+r*math.Cos(a0), y+r*math.Sin(a0)
	if c.has {
		c.lineTo(sx, sy)
	} else {
		c.moveTo(sx, sy)
	}
	if sweep != 0 {
		c.ellipseArc(x, y, r, r, a0, sweep)
	}
}

// geometry returns the path as a Direct2D geometry filled by the nonzero
// rule, as a canvas fills, or nil when it is empty.
func (c *canvasPlayer) geometry() *comObject {
	if len(c.path) == 0 {
		return nil
	}
	return c.p.pathGeometry(d2dFillWinding, func(sink *comObject) {
		open := false
		var start pointF
		for i := range c.path {
			s := &c.path[i]
			switch s.kind {
			case segMove:
				if open {
					sink.call(sinkEndFigure, d2dFigureOpen)
				}
				start = s.pts[0]
				sink.callPoint(sinkBeginFigure, start.x, start.y, d2dFigureFilled)
				open = true
			case segLine:
				if open {
					sink.call(sinkAddLines, uintptr(unsafe.Pointer(&s.pts[0])), 1)
				}
			case segCubic:
				if open {
					b := bezierSegment{s.pts[0], s.pts[1], s.pts[2]}
					sink.call(sinkAddBeziers, uintptr(unsafe.Pointer(&b)), 1)
				}
			case segClose:
				if open {
					sink.call(sinkEndFigure, d2dFigureClosed)
					open = false
					// What follows a close starts where the figure did.
					if i+1 < len(c.path) && c.path[i+1].kind != segMove {
						sink.callPoint(sinkBeginFigure, start.x, start.y, d2dFigureFilled)
						open = true
					}
				}
			}
		}
		if open {
			sink.call(sinkEndFigure, d2dFigureOpen)
		}
	})
}

// color applies the global alpha to col.
func (c *canvasPlayer) color(col driver.Color) driver.Color {
	a := math.Round(float64(col&0xff) * float64(c.st.alpha))
	return col&^0xff | driver.Color(a)
}

func (c *canvasPlayer) fill() {
	col := c.color(c.st.fill)
	if !visible(col) {
		return
	}
	if g := c.geometry(); g != nil {
		c.p.fillGeometry(g, col)
		g.release()
	}
}

func (c *canvasPlayer) stroke() {
	col := c.color(c.st.stroke)
	width := float32(float64(c.st.width) * c.st.ctm.stretch())
	if !visible(col) || width <= 0 {
		return
	}
	g := c.geometry()
	if g == nil {
		return
	}
	defer g.release()
	style := c.strokeStyle()
	var sp uintptr
	if style != nil {
		sp = style.ptr()
	}
	c.p.dc.callF(rtDrawGeometry, 0b100, g.ptr(), c.p.solid(col).ptr(), f32(width), sp)
}

// strokeStyle returns the stroke style of the line's ends and joins,
// made once per pass.
func (c *canvasPlayer) strokeStyle() *comObject {
	lineCap := uint32(d2dCapFlat)
	switch c.st.cap {
	case driver.CapRound:
		lineCap = d2dCapRound
	case driver.CapSquare:
		lineCap = d2dCapSquare
	}
	join := uint32(d2dJoinMiterOrBevel)
	switch c.st.join {
	case driver.JoinRound:
		join = d2dJoinRound
	case driver.JoinBevel:
		join = d2dJoinBevel
	}
	key := [2]uint32{lineCap, join}
	if s, ok := c.strokes[key]; ok {
		return s
	}
	props := strokeStyleProperties{startCap: lineCap, endCap: lineCap, dashCap: lineCap, lineJoin: join, miterLimit: 10}
	var s *comObject
	if failed(c.p.g.factory.call(d2dFactoryCreateStrokeStyle, uintptr(unsafe.Pointer(&props)), 0, 0,
		uintptr(unsafe.Pointer(&s)))) {
		s = nil
	}
	c.strokes[key] = s
	return s
}

// text draws op's text with its baseline at its point, in the drawing's
// transform.
func (c *canvasPlayer) text(op *driver.DrawOp) {
	col := c.color(c.st.fill)
	if !visible(col) || op.Text == "" {
		return
	}
	layout := plainLayout(op.Text, op.Font)
	if layout == nil {
		return
	}
	defer layout.release()
	var line lineMetrics
	var count uint32
	layout.call(layoutGetLineMetrics, uintptr(unsafe.Pointer(&line)), 1, uintptr(unsafe.Pointer(&count)))
	c.p.setTransform(c.st.ctm.then(c.world))
	c.p.dc.callPoint(rtDrawTextLayout, op.X, op.Y-line.baseline, layout.ptr(), c.p.solid(col).ptr(),
		d2dDrawTextEnableColorFont)
	c.p.setTransform(c.world)
}

// plainLayout returns text in font on one line, or nil.
func plainLayout(text string, font driver.Font) *comObject {
	format := textFormat(font)
	if format == nil {
		return nil
	}
	defer format.release()
	units := utf16.Encode([]rune(text))
	if len(units) == 0 {
		return nil
	}
	format.call(formatSetWordWrapping, dwNoWrap)
	var layout *comObject
	if failed(dwriteFactory().callF(dwCreateTextLayout, 0b11000, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)),
		format.ptr(), f32(unbounded), f32(unbounded), uintptr(unsafe.Pointer(&layout)))) {
		return nil
	}
	return layout
}
