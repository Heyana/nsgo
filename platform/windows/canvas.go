//go:build windows

package windows

import (
	"math"
	"unicode/utf16"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var (
	moveToEx       = gdi32.NewProc("MoveToEx")
	lineTo         = gdi32.NewProc("LineTo")
	polyBezierTo   = gdi32.NewProc("PolyBezierTo")
	closeFigure    = gdi32.NewProc("CloseFigure")
	fillPath       = gdi32.NewProc("FillPath")
	strokePath     = gdi32.NewProc("StrokePath")
	widenPath      = gdi32.NewProc("WidenPath")
	extCreatePen   = gdi32.NewProc("ExtCreatePen")
	setPolyFillMod = gdi32.NewProc("SetPolyFillMode")
	textOutW       = gdi32.NewProc("TextOutW")
	setTextAlign   = gdi32.NewProc("SetTextAlign")
)

const (
	winding        = 2
	taBaseline     = 24
	psGeometric    = 0x00010000
	psEndcapRound  = 0x0000
	psEndcapSquare = 0x0100
	psEndcapFlat   = 0x0200
	psJoinRound    = 0x0000
	psJoinBevel    = 0x1000
	psJoinMiter    = 0x2000

	// subpixels is how many GDI units make a pixel in canvas paths, which
	// GDI takes in whole units.
	subpixels = 16
)

type logBrush struct {
	Style uint32
	Color uint32
	Hatch uintptr
}

// canvasState is what Save and Restore keep.
type canvasState struct {
	ctm          matrix // the drawing's transform, in points
	fill, stroke core.Color
	width        float32
	cap          core.LineCap
	join         core.LineJoin
	alpha        float32
	saved        uintptr // the DC's state, for the clipping
}

type segKind uint8

const (
	segMove segKind = iota
	segLine
	segCubic
	segClose
)

// segment is a piece of the path, in the window's pixels.
type segment struct {
	kind segKind
	pts  [3][2]float64
}

// canvasPlayer plays a Drawing as the web's 2D context does: it builds
// the path in the window's pixels, as each operation's transform maps
// it, and has GDI fill, stroke or clip to it; translucent colors fill the
// clipped path through the painter's blend.
type canvasPlayer struct {
	p      *painter
	base   matrix // points to the window's pixels, through the view
	st     canvasState
	stack  []canvasState
	path   []segment
	cur    [2]float64 // in the window's pixels
	start  [2]float64
	has    bool
	fonts  map[core.Font]uintptr
	behind core.Color
}

// canvas plays v's drawing into r.
func (p *painter) canvas(v *view, r rect) {
	saved, _, _ := saveDC.Call(p.dc)
	p.clip(r, 0, rgnAnd)
	c := &canvasPlayer{
		p:      p,
		base:   scaling(p.w.scale, p.w.scale).then(translation(float64(r.Left), float64(r.Top))).then(p.m),
		st:     canvasState{ctm: identity, fill: 0x000000ff, stroke: 0x000000ff, width: 1, alpha: 1},
		fonts:  map[core.Font]uintptr{},
		behind: background(v),
	}
	setGraphicsMode.Call(p.dc, gmAdvanced)
	c.world(scaling(1.0/subpixels, 1.0/subpixels))
	setPolyFillMod.Call(p.dc, winding)
	for i := range v.drawing.Ops {
		c.play(&v.drawing.Ops[i])
	}
	for _, f := range c.fonts {
		deleteObject.Call(f)
	}
	p.restore(saved)
}

func (c *canvasPlayer) world(m matrix) {
	x := m.xform()
	setWorldTransform.Call(c.p.dc, uintptr(unsafe.Pointer(&x)))
}

func (c *canvasPlayer) play(op *core.DrawOp) {
	dc := c.p.dc
	x, y := float64(op.X), float64(op.Y)
	switch op.Kind {
	case core.DrawSave:
		c.st.saved, _, _ = saveDC.Call(dc)
		c.stack = append(c.stack, c.st)
	case core.DrawRestore:
		if n := len(c.stack); n > 0 {
			c.st = c.stack[n-1]
			c.stack = c.stack[:n-1]
			restoreDC.Call(dc, c.st.saved)
		}
	case core.DrawTranslate:
		c.st.ctm = translation(x, y).then(c.st.ctm)
	case core.DrawScale:
		c.st.ctm = scaling(x, y).then(c.st.ctm)
	case core.DrawRotate:
		sin, cos := math.Sincos(float64(op.A))
		c.st.ctm = matrix{a: cos, b: sin, c: -sin, d: cos}.then(c.st.ctm)
	case core.DrawBeginPath:
		c.path, c.has = c.path[:0], false
	case core.DrawMoveTo:
		c.moveTo(x, y)
	case core.DrawLineTo:
		c.lineTo(x, y)
	case core.DrawQuadTo:
		// The cubic with the same curve; affine maps keep it.
		if !c.has {
			c.moveTo(float64(op.X1), float64(op.Y1))
		}
		q := c.toPixels(float64(op.X1), float64(op.Y1))
		e := c.toPixels(x, y)
		c1 := [2]float64{c.cur[0] + 2.0/3*(q[0]-c.cur[0]), c.cur[1] + 2.0/3*(q[1]-c.cur[1])}
		c2 := [2]float64{e[0] + 2.0/3*(q[0]-e[0]), e[1] + 2.0/3*(q[1]-e[1])}
		c.cubic(c1, c2, e)
	case core.DrawCubicTo:
		if !c.has {
			c.moveTo(float64(op.X1), float64(op.Y1))
		}
		c.cubic(c.toPixels(float64(op.X1), float64(op.Y1)), c.toPixels(float64(op.X2), float64(op.Y2)), c.toPixels(x, y))
	case core.DrawArc:
		c.arc(op)
	case core.DrawRect:
		w, h := float64(op.W), float64(op.H)
		c.moveTo(x, y)
		c.lineTo(x+w, y)
		c.lineTo(x+w, y+h)
		c.lineTo(x, y+h)
		c.closePath()
		c.moveTo(x, y)
	case core.DrawRoundRect:
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
	case core.DrawEllipse:
		rx, ry := float64(op.R), float64(op.RY)
		c.moveTo(x+rx, y)
		c.ellipseArc(x, y, rx, ry, 0, 2*math.Pi)
		c.closePath()
	case core.DrawClosePath:
		c.closePath()
	case core.DrawFill:
		c.fill()
	case core.DrawStroke:
		c.stroke()
	case core.DrawClip:
		if c.emit() {
			selectClipPath.Call(dc, rgnAnd)
		}
	case core.DrawFillColor:
		c.st.fill = op.Color
	case core.DrawStrokeColor:
		c.st.stroke = op.Color
	case core.DrawLineWidth:
		c.st.width = op.W
	case core.DrawLineCap:
		c.st.cap = op.Cap
	case core.DrawLineJoin:
		c.st.join = op.Join
	case core.DrawAlpha:
		c.st.alpha = min(max(op.A, 0), 1)
	case core.DrawFillText:
		c.text(op)
	case core.DrawClear:
		// What is behind cannot show through: clearing paints the
		// background behind the canvas.
		saved, _, _ := saveDC.Call(dc)
		path, has, cur, start := c.path, c.has, c.cur, c.start
		c.path, c.has = nil, false
		c.play(&core.DrawOp{Kind: core.DrawRect, X: op.X, Y: op.Y, W: op.W, H: op.H})
		if c.emit() {
			selectClipPath.Call(dc, rgnAnd)
			c.p.fillClip(c.behind | 0xff)
		}
		c.path, c.has, c.cur, c.start = path, has, cur, start
		restoreDC.Call(dc, saved)
	}
}

func (c *canvasPlayer) toPixels(x, y float64) [2]float64 {
	px, py := c.st.ctm.then(c.base).apply(x, y)
	return [2]float64{px, py}
}

func (c *canvasPlayer) moveTo(x, y float64) {
	c.cur = c.toPixels(x, y)
	c.start, c.has = c.cur, true
	c.path = append(c.path, segment{kind: segMove, pts: [3][2]float64{c.cur}})
}

func (c *canvasPlayer) lineTo(x, y float64) {
	if !c.has {
		c.moveTo(x, y)
		return
	}
	c.cur = c.toPixels(x, y)
	c.path = append(c.path, segment{kind: segLine, pts: [3][2]float64{c.cur}})
}

func (c *canvasPlayer) cubic(c1, c2, end [2]float64) {
	c.cur = end
	c.path = append(c.path, segment{kind: segCubic, pts: [3][2]float64{c1, c2, end}})
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
			c.toPixels(cx+rx*(cos0-k*sin0), cy+ry*(sin0+k*cos0)),
			c.toPixels(cx+rx*(cos1+k*sin1), cy+ry*(sin1-k*cos1)),
			c.toPixels(cx+rx*cos1, cy+ry*sin1))
		a += step
	}
}

// arc adds an arc as the web's arc() does: joined to the path by a line.
func (c *canvasPlayer) arc(op *core.DrawOp) {
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

// emit makes the path GDI's, in subpixels, and reports whether it has
// anything.
func (c *canvasPlayer) emit() bool {
	if len(c.path) == 0 {
		return false
	}
	dc := c.p.dc
	unit := func(v float64) uintptr { return uintptr(int32(math.Round(v * subpixels))) }
	beginPath.Call(dc)
	for _, s := range c.path {
		switch s.kind {
		case segMove:
			moveToEx.Call(dc, unit(s.pts[0][0]), unit(s.pts[0][1]), 0)
		case segLine:
			lineTo.Call(dc, unit(s.pts[0][0]), unit(s.pts[0][1]))
		case segCubic:
			var pts [3]point
			for i, q := range s.pts {
				pts[i] = point{int32(math.Round(q[0] * subpixels)), int32(math.Round(q[1] * subpixels))}
			}
			polyBezierTo.Call(dc, uintptr(unsafe.Pointer(&pts[0])), 3)
		case segClose:
			closeFigure.Call(dc)
		}
	}
	endPath.Call(dc)
	return true
}

// color applies the global alpha to col.
func (c *canvasPlayer) color(col core.Color) core.Color {
	a := math.Round(float64(col&0xff) * float64(c.st.alpha))
	return col&^0xff | core.Color(a)
}

func (c *canvasPlayer) fill() {
	col := c.color(c.st.fill)
	if col&0xff == 0 || !c.emit() {
		return
	}
	dc := c.p.dc
	if col&0xff == 0xff {
		brush, _, _ := createSolidBrush.Call(colorRef(col))
		old, _, _ := selectObject.Call(dc, brush)
		fillPath.Call(dc)
		selectObject.Call(dc, old)
		deleteObject.Call(brush)
		return
	}
	saved, _, _ := saveDC.Call(dc)
	selectClipPath.Call(dc, rgnAnd)
	c.p.fillClip(col)
	restoreDC.Call(dc, saved)
}

func (c *canvasPlayer) stroke() {
	col := c.color(c.st.stroke)
	width := float64(c.st.width) * c.st.ctm.then(c.base).stretch() * subpixels
	if col&0xff == 0 || width <= 0 || !c.emit() {
		return
	}
	dc := c.p.dc
	style := uintptr(psGeometric)
	switch c.st.cap {
	case core.CapRound:
		style |= psEndcapRound
	case core.CapSquare:
		style |= psEndcapSquare
	default:
		style |= psEndcapFlat
	}
	switch c.st.join {
	case core.JoinRound:
		style |= psJoinRound
	case core.JoinBevel:
		style |= psJoinBevel
	default:
		style |= psJoinMiter
	}
	lb := logBrush{Color: uint32(colorRef(col))}
	pen, _, _ := extCreatePen.Call(style, uintptr(max(math.Round(width), subpixels)), uintptr(unsafe.Pointer(&lb)), 0, 0)
	if pen == 0 {
		return
	}
	old, _, _ := selectObject.Call(dc, pen)
	if col&0xff == 0xff {
		strokePath.Call(dc)
	} else {
		widenPath.Call(dc)
		saved, _, _ := saveDC.Call(dc)
		selectClipPath.Call(dc, rgnAnd)
		c.p.fillClip(col)
		restoreDC.Call(dc, saved)
	}
	selectObject.Call(dc, old)
	deleteObject.Call(pen)
}

// text draws op's text with its baseline at its point, the font scaled
// with the drawing; translucent text fills its outline.
func (c *canvasPlayer) text(op *core.DrawOp) {
	col := c.color(c.st.fill)
	if col&0xff == 0 || op.Text == "" {
		return
	}
	dc := c.p.dc
	f := c.fonts[op.Font]
	if f == 0 {
		family := fontName(op.Font.Family)
		f, _, _ = createFontW.Call(uintptr(-int32(float64(op.Font.Size)*subpixels+0.5)), 0, 0, 0, uintptr(op.Font.Weight),
			boolArg(op.Font.Italic), 0, 0, defaultCharset, 0, 0, clearType, 0, uintptr(unsafe.Pointer(wide(family))))
		c.fonts[op.Font] = f
	}
	text := utf16.Encode([]rune(op.Text))
	saved, _, _ := saveDC.Call(dc)
	c.world(scaling(1.0/subpixels, 1.0/subpixels).then(c.st.ctm).then(c.base))
	selectObject.Call(dc, f)
	setTextAlign.Call(dc, taBaseline)
	x, y := uintptr(int32(math.Round(float64(op.X)*subpixels))), uintptr(int32(math.Round(float64(op.Y)*subpixels)))
	if col&0xff == 0xff {
		setTextColor.Call(dc, colorRef(col))
		textOutW.Call(dc, x, y, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
	} else {
		beginPath.Call(dc)
		textOutW.Call(dc, x, y, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
		endPath.Call(dc)
		selectClipPath.Call(dc, rgnAnd)
		c.p.fillClip(col)
	}
	restoreDC.Call(dc, saved)
}
