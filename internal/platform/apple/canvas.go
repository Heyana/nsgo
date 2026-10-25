//go:build darwin

package apple

import (
	"context"
	"log"
	"math"
	"structs"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/imageload"
)

var (
	cgContextRotateCTM           func(ctx uintptr, a float64)
	cgContextConcatCTM           func(ctx uintptr, m affine)
	cgContextGetCTM              func(ctx uintptr) affine
	cgContextBeginPath           func(ctx uintptr)
	cgContextMoveToPoint         func(ctx uintptr, x, y float64)
	cgContextAddLineToPoint      func(ctx uintptr, x, y float64)
	cgContextAddQuadCurve        func(ctx uintptr, cx, cy, x, y float64)
	cgContextAddCurve            func(ctx uintptr, x1, y1, x2, y2, x, y float64)
	cgContextAddArc              func(ctx uintptr, x, y, r, a0, a1 float64, clockwise int32)
	cgContextAddRect             func(ctx uintptr, r CGRect)
	cgContextAddEllipseInRect    func(ctx uintptr, r CGRect)
	cgContextAddPath             func(ctx, path uintptr)
	cgContextCopyPath            func(ctx uintptr) uintptr
	cgContextIsPathEmpty         func(ctx uintptr) bool
	cgContextClosePath           func(ctx uintptr)
	cgContextFillPath            func(ctx uintptr)
	cgContextEOFillPath          func(ctx uintptr)
	cgContextStrokePath          func(ctx uintptr)
	cgContextClip                func(ctx uintptr)
	cgContextEOClip              func(ctx uintptr)
	cgContextReplacePathStroked  func(ctx uintptr)
	cgContextSetFillColor        func(ctx, color uintptr)
	cgContextSetStrokeColor      func(ctx, color uintptr)
	cgContextSetLineWidth        func(ctx uintptr, w float64)
	cgContextSetLineCap          func(ctx uintptr, c int32)
	cgContextSetLineJoin         func(ctx uintptr, j int32)
	cgContextSetMiterLimit       func(ctx uintptr, l float64)
	cgContextSetAlpha            func(ctx uintptr, a float64)
	cgContextSetBlendMode        func(ctx uintptr, mode int32)
	cgContextClearRect           func(ctx uintptr, r CGRect)
	cgContextFillRect            func(ctx uintptr, r CGRect)
	cgContextSetTextMatrix       func(ctx uintptr, m affine)
	cgContextSetTextDrawingMode  func(ctx uintptr, mode int32)
	cgContextBeginTransparency   func(ctx, aux uintptr)
	cgContextEndTransparency     func(ctx uintptr)
	cgContextDrawImage           func(ctx uintptr, r CGRect, image uintptr)
	cgContextDrawLinearGradient  func(ctx, gradient uintptr, start, end CGPoint, options uint32)
	cgContextDrawRadialGradient  func(ctx, gradient uintptr, start CGPoint, r0 float64, end CGPoint, r1 float64, options uint32)
	cgContextDrawConicGradient   func(ctx, gradient uintptr, center CGPoint, angle float64)
	cgGradientCreate             func(space uintptr, components, locations *float64, count uintptr) uintptr
	cgImageCreateWithImageInRect func(image uintptr, r CGRect) uintptr
	cgPathCreateWithRoundedRect  func(r CGRect, rw, rh float64, transform uintptr) uintptr
)

// affine is CGAffineTransform.
type affine struct {
	_                structs.HostLayout
	A, B, C, D, X, Y float64
}

func (m affine) invert() affine {
	det := m.A*m.D - m.B*m.C
	if det == 0 {
		return affine{A: 1, D: 1}
	}
	return affine{A: m.D / det, B: -m.B / det, C: -m.C / det, D: m.A / det,
		X: (m.C*m.Y - m.D*m.X) / det, Y: (m.B*m.X - m.A*m.Y) / det}
}

func loadCanvas(cg uintptr) {
	reg := purego.RegisterLibFunc
	reg(&cgContextRotateCTM, cg, "CGContextRotateCTM")
	reg(&cgContextConcatCTM, cg, "CGContextConcatCTM")
	reg(&cgContextGetCTM, cg, "CGContextGetCTM")
	reg(&cgContextBeginPath, cg, "CGContextBeginPath")
	reg(&cgContextMoveToPoint, cg, "CGContextMoveToPoint")
	reg(&cgContextAddLineToPoint, cg, "CGContextAddLineToPoint")
	reg(&cgContextAddQuadCurve, cg, "CGContextAddQuadCurveToPoint")
	reg(&cgContextAddCurve, cg, "CGContextAddCurveToPoint")
	reg(&cgContextAddArc, cg, "CGContextAddArc")
	reg(&cgContextAddRect, cg, "CGContextAddRect")
	reg(&cgContextAddEllipseInRect, cg, "CGContextAddEllipseInRect")
	reg(&cgContextAddPath, cg, "CGContextAddPath")
	reg(&cgContextCopyPath, cg, "CGContextCopyPath")
	reg(&cgContextIsPathEmpty, cg, "CGContextIsPathEmpty")
	reg(&cgContextClosePath, cg, "CGContextClosePath")
	reg(&cgContextFillPath, cg, "CGContextFillPath")
	reg(&cgContextEOFillPath, cg, "CGContextEOFillPath")
	reg(&cgContextStrokePath, cg, "CGContextStrokePath")
	reg(&cgContextClip, cg, "CGContextClip")
	reg(&cgContextEOClip, cg, "CGContextEOClip")
	reg(&cgContextReplacePathStroked, cg, "CGContextReplacePathWithStrokedPath")
	reg(&cgContextSetFillColor, cg, "CGContextSetFillColorWithColor")
	reg(&cgContextSetStrokeColor, cg, "CGContextSetStrokeColorWithColor")
	reg(&cgContextSetLineWidth, cg, "CGContextSetLineWidth")
	reg(&cgContextSetLineCap, cg, "CGContextSetLineCap")
	reg(&cgContextSetLineJoin, cg, "CGContextSetLineJoin")
	reg(&cgContextSetMiterLimit, cg, "CGContextSetMiterLimit")
	reg(&cgContextSetAlpha, cg, "CGContextSetAlpha")
	reg(&cgContextSetBlendMode, cg, "CGContextSetBlendMode")
	reg(&cgContextClearRect, cg, "CGContextClearRect")
	reg(&cgContextFillRect, cg, "CGContextFillRect")
	reg(&cgContextSetTextMatrix, cg, "CGContextSetTextMatrix")
	reg(&cgContextSetTextDrawingMode, cg, "CGContextSetTextDrawingMode")
	reg(&cgContextBeginTransparency, cg, "CGContextBeginTransparencyLayer")
	reg(&cgContextEndTransparency, cg, "CGContextEndTransparencyLayer")
	reg(&cgContextDrawImage, cg, "CGContextDrawImage")
	reg(&cgContextDrawLinearGradient, cg, "CGContextDrawLinearGradient")
	reg(&cgContextDrawRadialGradient, cg, "CGContextDrawRadialGradient")
	// macOS 14 and iOS 17 draw conic gradients; earlier systems draw
	// them as wedges.
	if _, err := purego.Dlsym(cg, "CGContextDrawConicGradient"); err == nil {
		reg(&cgContextDrawConicGradient, cg, "CGContextDrawConicGradient")
	}
	reg(&cgGradientCreate, cg, "CGGradientCreateWithColorComponents")
	reg(&cgImageCreateWithImageInRect, cg, "CGImageCreateWithImageInRect")
	reg(&cgPathCreateWithRoundedRect, cg, "CGPathCreateWithRoundedRect")
}

// DrawTopDown runs draw with ctx's state saved and its y axis pointing
// down over a layer height tall; flip turns a y-up context, which Core
// Animation gives a layer whose contents are not flipped, top down.
func DrawTopDown(ctx uintptr, height float64, flip bool, draw func()) {
	cgContextSaveGState(ctx)
	defer cgContextRestoreGState(ctx)
	if flip {
		cgContextTranslateCTM(ctx, 0, height)
		cgContextScaleCTM(ctx, 1, -1)
	}
	draw()
}

// CGBlendMode of each CompositeOp.
var blendModes = [...]int32{
	driver.CompositeSourceOver:      0,
	driver.CompositeSourceIn:        18,
	driver.CompositeSourceOut:       19,
	driver.CompositeSourceAtop:      20,
	driver.CompositeDestinationOver: 21,
	driver.CompositeDestinationIn:   22,
	driver.CompositeDestinationOut:  23,
	driver.CompositeDestinationAtop: 24,
	driver.CompositeLighter:         27,
	driver.CompositeCopy:            17,
	driver.CompositeXor:             25,
	driver.CompositeMultiply:        1,
	driver.CompositeScreen:          2,
	driver.CompositeOverlay:         3,
	driver.CompositeDarken:          4,
	driver.CompositeLighten:         5,
	driver.CompositeColorDodge:      6,
	driver.CompositeColorBurn:       7,
	driver.CompositeHardLight:       9,
	driver.CompositeSoftLight:       8,
	driver.CompositeDifference:      10,
	driver.CompositeExclusion:       11,
	driver.CompositeHue:             12,
	driver.CompositeSaturation:      13,
	driver.CompositeColor:           14,
	driver.CompositeLuminosity:      15,
}

// CGTextDrawingMode values.
const (
	textFill   = 0
	textStroke = 1
)

// canvasState is what Save keeps besides Core Graphics' own state: the
// paints, which text and gradients need, and whether a shadow or an
// operator that clears outside the shape is on.
type canvasState struct {
	fill, stroke         driver.Color
	fillGrad, strokeGrad *driver.CanvasGradient
	shadow               bool
	composite            driver.CompositeOp
	// dash and phase are the dash pattern, which an offset sets again.
	dash  []float64
	phase float64
}

// CanvasHost is the view a drawing plays in: the screen's pixels a point,
// and what draws it again once an image it waits for loads, which Owner
// names so it waits once.
type CanvasHost struct {
	Ratio  float64
	Redraw func()
	Owner  any
}

// Play draws ops into ctx, a view's context with its y axis pointing
// down, as a web canvas would: CoreGraphics adds path points through the
// transform current when they are added, as the canvas does, and a fill,
// stroke or clip keeps the path.
func Play(ctx uintptr, ops []driver.DrawOp, host CanvasHost) {
	if ctx == 0 {
		return
	}
	cgContextSaveGState(ctx)
	defer cgContextRestoreGState(ctx)
	p := &player{ctx: ctx, base: cgContextGetCTM(ctx), host: host,
		stack: []canvasState{{fill: 0x000000ff, stroke: 0x000000ff}}}
	cgContextBeginPath(ctx)
	p.color(cgContextSetFillColor, 0x000000ff)
	p.color(cgContextSetStrokeColor, 0x000000ff)
	for i := range ops {
		p.play(&ops[i])
	}
	// What the drawing saved and never restored.
	for range len(p.stack) - 1 {
		cgContextRestoreGState(ctx)
	}
}

type player struct {
	ctx   uintptr
	base  affine // the canvas's own coordinates
	host  CanvasHost
	stack []canvasState
}

func (p *player) st() *canvasState { return &p.stack[len(p.stack)-1] }

func (p *player) color(set func(ctx, color uintptr), c driver.Color) {
	ref := CGColor(c)
	set(p.ctx, ref)
	Release(ref)
}

// keep runs a path-consuming call and adds the path back.
func (p *player) keep(fn func()) {
	if cgContextIsPathEmpty(p.ctx) {
		return
	}
	path := cgContextCopyPath(p.ctx)
	fn()
	cgContextAddPath(p.ctx, path)
	Release(path)
}

func (p *player) play(op *driver.DrawOp) {
	ctx := p.ctx
	f := func(v float32) float64 { return float64(v) }
	switch op.Kind {
	case driver.DrawSave:
		cgContextSaveGState(ctx)
		p.stack = append(p.stack, *p.st())
	case driver.DrawRestore:
		if len(p.stack) > 1 {
			cgContextRestoreGState(ctx)
			p.stack = p.stack[:len(p.stack)-1]
		}
	case driver.DrawTranslate:
		cgContextTranslateCTM(ctx, f(op.X), f(op.Y))
	case driver.DrawScale:
		cgContextScaleCTM(ctx, f(op.X), f(op.Y))
	case driver.DrawRotate:
		cgContextRotateCTM(ctx, f(op.A))
	case driver.DrawTransform:
		cgContextConcatCTM(ctx, affine{A: f(op.X1), B: f(op.Y1), C: f(op.X2), D: f(op.Y2), X: f(op.X), Y: f(op.Y)})
	case driver.DrawSetTransform:
		cgContextConcatCTM(ctx, cgContextGetCTM(ctx).invert())
		cgContextConcatCTM(ctx, p.base)
		cgContextConcatCTM(ctx, affine{A: f(op.X1), B: f(op.Y1), C: f(op.X2), D: f(op.Y2), X: f(op.X), Y: f(op.Y)})
	case driver.DrawBeginPath:
		cgContextBeginPath(ctx)
	case driver.DrawMoveTo:
		cgContextMoveToPoint(ctx, f(op.X), f(op.Y))
	case driver.DrawLineTo:
		if cgContextIsPathEmpty(ctx) {
			cgContextMoveToPoint(ctx, f(op.X), f(op.Y))
		} else {
			cgContextAddLineToPoint(ctx, f(op.X), f(op.Y))
		}
	case driver.DrawQuadTo:
		if cgContextIsPathEmpty(ctx) {
			cgContextMoveToPoint(ctx, f(op.X1), f(op.Y1))
		}
		cgContextAddQuadCurve(ctx, f(op.X1), f(op.Y1), f(op.X), f(op.Y))
	case driver.DrawCubicTo:
		if cgContextIsPathEmpty(ctx) {
			cgContextMoveToPoint(ctx, f(op.X1), f(op.Y1))
		}
		cgContextAddCurve(ctx, f(op.X1), f(op.Y1), f(op.X2), f(op.Y2), f(op.X), f(op.Y))
	case driver.DrawArc:
		// With y pointing down, CoreGraphics' clockwise is the
		// canvas's counterclockwise.
		var clockwise int32
		if op.CCW {
			clockwise = 1
		}
		cgContextAddArc(ctx, f(op.X), f(op.Y), max(f(op.R), 0), f(op.A0), f(op.A1), clockwise)
	case driver.DrawRect:
		cgContextAddRect(ctx, Rect(f(op.X), f(op.Y), f(op.W), f(op.H)))
		cgContextMoveToPoint(ctx, f(op.X), f(op.Y))
	case driver.DrawRoundRect:
		w, h := max(f(op.W), 0), max(f(op.H), 0)
		r := min(max(f(op.R), 0), w/2, h/2)
		path := cgPathCreateWithRoundedRect(Rect(f(op.X), f(op.Y), w, h), r, r, 0)
		cgContextAddPath(ctx, path)
		Release(path)
	case driver.DrawEllipse:
		cgContextAddEllipseInRect(ctx, Rect(f(op.X-op.R), f(op.Y-op.RY), 2*f(op.R), 2*f(op.RY)))
	case driver.DrawClosePath:
		if !cgContextIsPathEmpty(ctx) {
			cgContextClosePath(ctx)
		}
	case driver.DrawFill:
		p.keep(func() { p.fillPath(op.EvenOdd) })
	case driver.DrawStroke:
		p.keep(p.strokePath)
	case driver.DrawClip:
		p.keep(func() {
			if op.EvenOdd {
				cgContextEOClip(ctx)
			} else {
				cgContextClip(ctx)
			}
		})
	case driver.DrawFillColor:
		p.st().fill, p.st().fillGrad = op.Color, nil
		p.color(cgContextSetFillColor, op.Color)
	case driver.DrawStrokeColor:
		p.st().stroke, p.st().strokeGrad = op.Color, nil
		p.color(cgContextSetStrokeColor, op.Color)
	case driver.DrawFillGradient:
		if op.Gradient != nil && len(op.Gradient.Stops) > 0 {
			p.st().fillGrad = op.Gradient
		}
	case driver.DrawStrokeGradient:
		if op.Gradient != nil && len(op.Gradient.Stops) > 0 {
			p.st().strokeGrad = op.Gradient
		}
	case driver.DrawLineWidth:
		cgContextSetLineWidth(ctx, f(op.W))
	case driver.DrawLineCap:
		cgContextSetLineCap(ctx, int32(op.Cap))
	case driver.DrawLineJoin:
		cgContextSetLineJoin(ctx, int32(op.Join))
	case driver.DrawMiterLimit:
		cgContextSetMiterLimit(ctx, f(op.W))
	case driver.DrawLineDash:
		var dash []float64
		if op.Dash != nil {
			for _, l := range *op.Dash {
				dash = append(dash, float64(l))
			}
		}
		p.setDash(dash, -1)
	case driver.DrawLineDashOffset:
		p.setDash(nil, f(op.W))
	case driver.DrawAlpha:
		cgContextSetAlpha(ctx, f(op.A))
	case driver.DrawComposite:
		if int(op.Composite) < len(blendModes) {
			p.st().composite = op.Composite
			cgContextSetBlendMode(ctx, blendModes[op.Composite])
		}
	case driver.DrawShadow:
		// Shadow offsets are in the base space, y up under the flipped
		// context: a positive offset must still go down.
		c := CGColor(op.Color)
		cgContextSetShadowWithColor(ctx, CGSize{Width: f(op.X), Height: -f(op.Y)}, f(op.R), c)
		Release(c)
		p.st().shadow = op.Color&0xff != 0
	case driver.DrawClear:
		cgContextClearRect(ctx, Rect(f(op.X), f(op.Y), f(op.W), f(op.H)))
	case driver.DrawFillText, driver.DrawStrokeText:
		p.text(op)
	case driver.DrawImage:
		p.image(op)
	}
}

// setDash sets the dash pattern to lengths, or its phase when phase is
// not negative.
func (p *player) setDash(lengths []float64, phase float64) {
	st := p.st()
	if phase >= 0 {
		st.phase = phase
	} else {
		st.dash = lengths
	}
	if len(st.dash) == 0 {
		cgContextSetLineDash(p.ctx, 0, nil, 0)
		return
	}
	cgContextSetLineDash(p.ctx, st.phase, &st.dash[0], uintptr(len(st.dash)))
}

// layered runs draw in a transparency layer when what it draws must be
// composited as a whole: under an operator that clears outside the
// shape, or a gradient under a shadow. The layer takes the shadow, alpha
// and operator; inside, drawing is plain.
func (p *player) layered(gradient bool, draw func()) {
	st := p.st()
	if !st.composite.Unbounded() && !(gradient && st.shadow) {
		draw()
		return
	}
	cgContextBeginTransparency(p.ctx, 0)
	cgContextSaveGState(p.ctx)
	cgContextSetBlendMode(p.ctx, 0)
	cgContextSetAlpha(p.ctx, 1)
	cgContextSetShadowWithColor(p.ctx, CGSize{}, 0, 0)
	draw()
	cgContextRestoreGState(p.ctx)
	cgContextEndTransparency(p.ctx)
}

func (p *player) fillPath(evenOdd bool) {
	g := p.st().fillGrad
	p.layered(g != nil, func() {
		if g == nil {
			if evenOdd {
				cgContextEOFillPath(p.ctx)
			} else {
				cgContextFillPath(p.ctx)
			}
			return
		}
		cgContextSaveGState(p.ctx)
		if evenOdd {
			cgContextEOClip(p.ctx)
		} else {
			cgContextClip(p.ctx)
		}
		p.gradient(g)
		cgContextRestoreGState(p.ctx)
	})
}

func (p *player) strokePath() {
	g := p.st().strokeGrad
	p.layered(g != nil, func() {
		if g == nil {
			cgContextStrokePath(p.ctx)
			return
		}
		cgContextSaveGState(p.ctx)
		cgContextReplacePathStroked(p.ctx)
		cgContextClip(p.ctx)
		p.gradient(g)
		cgContextRestoreGState(p.ctx)
	})
}

// Options of CGContextDrawLinearGradient and its radial one: the end
// colors continue past the ends.
const padEnds = 1 | 2

// gradient paints g over the clip, in its own transform.
func (p *player) gradient(g *driver.CanvasGradient) {
	m := g.Matrix()
	cgContextConcatCTM(p.ctx, affine{A: float64(m[0]), B: float64(m[1]), C: float64(m[2]), D: float64(m[3]),
		X: float64(m[4]), Y: float64(m[5])})
	stops := g.Stops
	x1, y1, x2, y2 := float64(g.X1), float64(g.Y1), float64(g.X2), float64(g.Y2)
	r1, r2 := max(float64(g.R1), 0), max(float64(g.R2), 0)
	if g.Spread != driver.SpreadPad && !g.Conic {
		// Repeats as more of the same stops, far enough to cover what
		// the canvas shows.
		const periods = 24
		stops = repeatStops(g, periods)
		dx, dy := x2-x1, y2-y1
		if g.Radial {
			r2 = r1 + (r2-r1)*float64(periods)
			x2, y2 = x1+dx*float64(periods), y1+dy*float64(periods)
		} else {
			x1, y1 = x1-dx*float64(periods/2), y1-dy*float64(periods/2)
			x2, y2 = x2+dx*float64(periods/2-1), y2+dy*float64(periods/2-1)
		}
	}
	grad := cgGradient(stops)
	if grad == 0 {
		return
	}
	defer Release(grad)
	switch {
	case g.Conic:
		if cgContextDrawConicGradient != nil {
			cgContextDrawConicGradient(p.ctx, grad, CGPoint{X: x1, Y: y1}, float64(g.Angle))
		} else {
			p.conicWedges(g)
		}
	case g.Radial:
		cgContextDrawRadialGradient(p.ctx, grad, CGPoint{X: x1, Y: y1}, r1, CGPoint{X: x2, Y: y2}, r2, padEnds)
	default:
		cgContextDrawLinearGradient(p.ctx, grad, CGPoint{X: x1, Y: y1}, CGPoint{X: x2, Y: y2}, padEnds)
	}
}

// repeatStops lays g's stops out over periods copies, mirrored every
// other one for a reflecting gradient, on offsets from 0 to 1.
func repeatStops(g *driver.CanvasGradient, periods int) []driver.GradientStop {
	var out []driver.GradientStop
	for k := range periods {
		flip := g.Spread == driver.SpreadReflect && k%2 == 1
		for i := range g.Stops {
			s := g.Stops[i]
			if flip {
				s = g.Stops[len(g.Stops)-1-i]
				s.Offset = 1 - s.Offset
			}
			s.Offset = (float32(k) + s.Offset) / float32(periods)
			out = append(out, s)
		}
	}
	return out
}

// conicWedges paints a conic gradient as thin wedges, a degree each, for
// systems without CGContextDrawConicGradient.
func (p *player) conicWedges(g *driver.CanvasGradient) {
	const n = 360
	cx, cy := float64(g.X1), float64(g.Y1)
	r := 1e5
	for i := range n {
		a0 := float64(g.Angle) + 2*math.Pi*float64(i)/n
		a1 := float64(g.Angle) + 2*math.Pi*float64(i+1)/n + 0.01
		p.color(cgContextSetFillColor, g.At((float64(i)+0.5)/n))
		cgContextBeginPath(p.ctx)
		cgContextMoveToPoint(p.ctx, cx, cy)
		cgContextAddArc(p.ctx, cx, cy, r, a0, a1, 0)
		cgContextClosePath(p.ctx)
		cgContextFillPath(p.ctx)
	}
}

// cgGradient returns a CGGradient of stops in sRGB, or 0 without stops.
func cgGradient(stops []driver.GradientStop) uintptr {
	if len(stops) == 0 {
		return 0
	}
	comps := make([]float64, 0, 4*len(stops))
	locs := make([]float64, 0, len(stops))
	for _, s := range stops {
		r, g, b, a := s.Color.Components()
		comps = append(comps, r, g, b, a)
		locs = append(locs, min(max(float64(s.Offset), 0), 1))
	}
	return cgGradientCreate(srgb, &comps[0], &locs[0], uintptr(len(stops)))
}

// text draws one line of text at the op's point, moved by its alignment:
// filled with the fill color, or a gradient through the glyphs, or
// stroked with the stroke settings.
func (p *player) text(op *driver.DrawOp) {
	if op.Text == "" {
		return
	}
	st := p.st()
	stroke := op.Kind == driver.DrawStrokeText
	color, grad := st.fill, st.fillGrad
	if stroke {
		color, grad = st.stroke, st.strokeGrad
	}
	str := attributed(&driver.AttributedText{Text: op.Text, Style: driver.TextStyle{Font: op.Font, Color: color}}, nil, nil)
	line := ctLineCreateWithAttrString(str)
	Release(str)
	defer Release(line)
	show := func() {
		// Glyphs draw upright in a context whose y axis points down.
		cgContextSetTextMatrix(p.ctx, affine{A: 1, D: -1})
		cgContextSetTextPosition(p.ctx, float64(op.X+op.X1), float64(op.Y+op.Y1))
		if stroke {
			cgContextSetTextDrawingMode(p.ctx, textStroke)
		}
		ctLineDraw(line, p.ctx)
		cgContextSetTextDrawingMode(p.ctx, textFill)
	}
	p.layered(grad != nil, func() {
		if grad == nil {
			show()
			return
		}
		// The glyphs drawn plain, then the gradient over them alone.
		cgContextBeginTransparency(p.ctx, 0)
		show()
		cgContextSaveGState(p.ctx)
		cgContextSetBlendMode(p.ctx, blendModes[driver.CompositeSourceIn])
		p.gradient(grad)
		cgContextRestoreGState(p.ctx)
		cgContextEndTransparency(p.ctx)
	})
}

// image draws op's image once it is loaded.
func (p *player) image(op *driver.DrawOp) {
	if op.Image == nil {
		return
	}
	img := canvasImage(*op.Image, p.host)
	if img == nil || img.image == 0 {
		return
	}
	f := func(v float32) float64 { return float64(v) }
	w, h := f(op.W), f(op.H)
	if w == 0 && h == 0 {
		w, h = float64(img.points.Width), float64(img.points.Height)
	}
	src := img.image
	if op.X2 != 0 || op.Y2 != 0 {
		// The part named in points of the image, in its pixels.
		k := img.width / max(float64(img.points.Width), 1e-9)
		src = cgImageCreateWithImageInRect(img.image, Rect(f(op.X1)*k, f(op.Y1)*k, f(op.X2)*k, f(op.Y2)*k))
		if src == 0 {
			return
		}
		defer Release(src)
	}
	p.layered(false, func() {
		cgContextSaveGState(p.ctx)
		// Images draw bottom up: flip them into the top-down space.
		cgContextTranslateCTM(p.ctx, f(op.X), f(op.Y)+h)
		cgContextScaleCTM(p.ctx, 1, -1)
		cgContextDrawImage(p.ctx, Rect(0, 0, w, h), src)
		cgContextRestoreGState(p.ctx)
	})
}

// canvasImages are the images canvases draw, decoded once each for the
// process at their full size, by ImageSource.Key; main thread only.
var canvasImages = map[string]*loadedImage{}

// loadedImage is an image canvases draw, or one loading.
type loadedImage struct {
	image         uintptr
	width, height float64 // pixels
	points        driver.Size
	failed        bool
	waiting       map[any]func()
	used          uint64
}

var canvasImageUse uint64

// maxCanvasImages bounds the images kept; the least recently drawn go.
const maxCanvasImages = 64

// canvasImage returns src's image, or nil while it loads, and has the
// host draw again once it has loaded.
func canvasImage(src driver.ImageSource, host CanvasHost) *loadedImage {
	key := src.Key()
	canvasImageUse++
	if im := canvasImages[key]; im != nil {
		im.used = canvasImageUse
		if im.image == 0 && !im.failed && host.Redraw != nil {
			im.waiting[host.Owner] = host.Redraw
		}
		return im
	}
	if len(canvasImages) >= maxCanvasImages {
		var oldest string
		var at uint64 = math.MaxUint64
		for k, im := range canvasImages {
			if im.image != 0 && im.used < at {
				oldest, at = k, im.used
			}
		}
		if im := canvasImages[oldest]; im != nil {
			Release(im.image)
			delete(canvasImages, oldest)
		}
	}
	im := &loadedImage{used: canvasImageUse, waiting: map[any]func(){}}
	if host.Redraw != nil {
		im.waiting[host.Owner] = host.Redraw
	}
	canvasImages[key] = im
	go func() {
		data, err := imageload.Load(context.Background(), src, nil)
		var d decoded
		if err == nil {
			d, err = decode(data, request{mode: driver.Stretch, ratio: max(host.Ratio, 1), scale: uriScale(src.URI)})
		}
		OnMain(func() {
			if err != nil || d.image == 0 {
				im.failed = true
				log.Printf("nsgo: canvas: image %s: %v", key, err)
			} else {
				im.image, im.width, im.height, im.points = d.image, d.width, d.height, d.points
				for _, f := range d.frames[min(len(d.frames), 1):] {
					Release(f)
				}
			}
			waiting := im.waiting
			im.waiting = nil
			for _, fn := range waiting {
				fn()
			}
		})
	}()
	return nil
}

// Tint paints c over what ctx holds in r, where it holds anything: a
// text's bound color over all of its glyphs.
func Tint(ctx uintptr, r CGRect, c driver.Color) {
	cgContextSaveGState(ctx)
	cgContextSetBlendMode(ctx, blendModes[driver.CompositeSourceIn])
	ref := CGColor(c)
	cgContextSetFillColor(ctx, ref)
	Release(ref)
	cgContextFillRect(ctx, r)
	cgContextRestoreGState(ctx)
}

// Damaged is the area of a canvas to draw again after its animated
// arguments played before now play after (driver.CanvasDamage), 2 points
// wider for anti-aliasing and rounded out to whole points; whole is set
// where only drawing all of it is right. An empty area draws nothing.
func Damaged(before, after []driver.DrawOp) (area CGRect, whole bool) {
	d, ok := driver.CanvasDamage(before, after)
	if !ok {
		return CGRect{}, true
	}
	if d.Size.Width <= 0 || d.Size.Height <= 0 {
		return CGRect{}, false
	}
	const aa = 2
	x0, y0 := math.Floor(float64(d.Origin.X)-aa), math.Floor(float64(d.Origin.Y)-aa)
	x1 := math.Ceil(float64(d.Origin.X+d.Size.Width) + aa)
	y1 := math.Ceil(float64(d.Origin.Y+d.Size.Height) + aa)
	return Rect(x0, y0, x1-x0, y1-y0), false
}
