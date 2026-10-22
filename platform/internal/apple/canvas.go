//go:build darwin

package apple

import (
	"structs"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/core"
)

var (
	cgContextRotateCTM          func(ctx uintptr, a float64)
	cgContextBeginPath          func(ctx uintptr)
	cgContextMoveToPoint        func(ctx uintptr, x, y float64)
	cgContextAddLineToPoint     func(ctx uintptr, x, y float64)
	cgContextAddQuadCurve       func(ctx uintptr, cx, cy, x, y float64)
	cgContextAddCurve           func(ctx uintptr, x1, y1, x2, y2, x, y float64)
	cgContextAddArc             func(ctx uintptr, x, y, r, a0, a1 float64, clockwise int32)
	cgContextAddRect            func(ctx uintptr, r CGRect)
	cgContextAddEllipseInRect   func(ctx uintptr, r CGRect)
	cgContextAddPath            func(ctx, path uintptr)
	cgContextCopyPath           func(ctx uintptr) uintptr
	cgContextIsPathEmpty        func(ctx uintptr) bool
	cgContextClosePath          func(ctx uintptr)
	cgContextFillPath           func(ctx uintptr)
	cgContextStrokePath         func(ctx uintptr)
	cgContextClip               func(ctx uintptr)
	cgContextSetFillColor       func(ctx, color uintptr)
	cgContextSetStrokeColor     func(ctx, color uintptr)
	cgContextSetLineWidth       func(ctx uintptr, w float64)
	cgContextSetLineCap         func(ctx uintptr, c int32)
	cgContextSetLineJoin        func(ctx uintptr, j int32)
	cgContextSetAlpha           func(ctx uintptr, a float64)
	cgContextClearRect          func(ctx uintptr, r CGRect)
	cgContextSetTextMatrix      func(ctx uintptr, m affine)
	cgPathCreateWithRoundedRect func(r CGRect, rw, rh float64, transform uintptr) uintptr
)

// affine is CGAffineTransform.
type affine struct {
	_                structs.HostLayout
	A, B, C, D, X, Y float64
}

func loadCanvas(cg uintptr) {
	reg := purego.RegisterLibFunc
	reg(&cgContextRotateCTM, cg, "CGContextRotateCTM")
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
	reg(&cgContextStrokePath, cg, "CGContextStrokePath")
	reg(&cgContextClip, cg, "CGContextClip")
	reg(&cgContextSetFillColor, cg, "CGContextSetFillColorWithColor")
	reg(&cgContextSetStrokeColor, cg, "CGContextSetStrokeColorWithColor")
	reg(&cgContextSetLineWidth, cg, "CGContextSetLineWidth")
	reg(&cgContextSetLineCap, cg, "CGContextSetLineCap")
	reg(&cgContextSetLineJoin, cg, "CGContextSetLineJoin")
	reg(&cgContextSetAlpha, cg, "CGContextSetAlpha")
	reg(&cgContextClearRect, cg, "CGContextClearRect")
	reg(&cgContextSetTextMatrix, cg, "CGContextSetTextMatrix")
	reg(&cgPathCreateWithRoundedRect, cg, "CGPathCreateWithRoundedRect")
}

// Play draws d into ctx, a view's context with its y axis pointing down,
// as a web canvas would: CoreGraphics adds path points through the
// transform current when they are added, as the canvas does, and a fill,
// stroke or clip keeps the path.
func Play(ctx uintptr, d *core.Drawing) {
	if ctx == 0 || d == nil {
		return
	}
	cgContextSaveGState(ctx)
	defer cgContextRestoreGState(ctx)
	// The fill color, which text needs as an attribute, is saved and
	// restored with the rest of the state.
	fill := []core.Color{0x000000ff}
	cgContextBeginPath(ctx)
	// keep runs a path-consuming call and adds the path back.
	keep := func(fn func(uintptr)) {
		if cgContextIsPathEmpty(ctx) {
			return
		}
		path := cgContextCopyPath(ctx)
		fn(ctx)
		cgContextAddPath(ctx, path)
		Release(path)
	}
	color := func(set func(ctx, color uintptr), c core.Color) {
		ref := CGColor(c)
		set(ctx, ref)
		Release(ref)
	}
	color(cgContextSetFillColor, fill[0])
	color(cgContextSetStrokeColor, fill[0])
	f := func(v float32) float64 { return float64(v) }
	for _, op := range d.Ops {
		switch op.Kind {
		case core.DrawSave:
			cgContextSaveGState(ctx)
			fill = append(fill, fill[len(fill)-1])
		case core.DrawRestore:
			if len(fill) > 1 {
				cgContextRestoreGState(ctx)
				fill = fill[:len(fill)-1]
			}
		case core.DrawTranslate:
			cgContextTranslateCTM(ctx, f(op.X), f(op.Y))
		case core.DrawScale:
			cgContextScaleCTM(ctx, f(op.X), f(op.Y))
		case core.DrawRotate:
			cgContextRotateCTM(ctx, f(op.A))
		case core.DrawBeginPath:
			cgContextBeginPath(ctx)
		case core.DrawMoveTo:
			cgContextMoveToPoint(ctx, f(op.X), f(op.Y))
		case core.DrawLineTo:
			cgContextAddLineToPoint(ctx, f(op.X), f(op.Y))
		case core.DrawQuadTo:
			cgContextAddQuadCurve(ctx, f(op.X1), f(op.Y1), f(op.X), f(op.Y))
		case core.DrawCubicTo:
			cgContextAddCurve(ctx, f(op.X1), f(op.Y1), f(op.X2), f(op.Y2), f(op.X), f(op.Y))
		case core.DrawArc:
			// With y pointing down, CoreGraphics' clockwise is the
			// canvas's counterclockwise.
			var clockwise int32
			if op.CCW {
				clockwise = 1
			}
			cgContextAddArc(ctx, f(op.X), f(op.Y), f(op.R), f(op.A0), f(op.A1), clockwise)
		case core.DrawRect:
			cgContextAddRect(ctx, Rect(f(op.X), f(op.Y), f(op.W), f(op.H)))
		case core.DrawRoundRect:
			w, h := max(f(op.W), 0), max(f(op.H), 0)
			r := min(max(f(op.R), 0), w/2, h/2)
			path := cgPathCreateWithRoundedRect(Rect(f(op.X), f(op.Y), w, h), r, r, 0)
			cgContextAddPath(ctx, path)
			Release(path)
		case core.DrawEllipse:
			cgContextAddEllipseInRect(ctx, Rect(f(op.X-op.R), f(op.Y-op.RY), 2*f(op.R), 2*f(op.RY)))
		case core.DrawClosePath:
			if !cgContextIsPathEmpty(ctx) {
				cgContextClosePath(ctx)
			}
		case core.DrawFill:
			keep(cgContextFillPath)
		case core.DrawStroke:
			keep(cgContextStrokePath)
		case core.DrawClip:
			keep(cgContextClip)
		case core.DrawFillColor:
			fill[len(fill)-1] = op.Color
			color(cgContextSetFillColor, op.Color)
		case core.DrawStrokeColor:
			color(cgContextSetStrokeColor, op.Color)
		case core.DrawLineWidth:
			cgContextSetLineWidth(ctx, f(op.W))
		case core.DrawLineCap:
			cgContextSetLineCap(ctx, int32(op.Cap))
		case core.DrawLineJoin:
			cgContextSetLineJoin(ctx, int32(op.Join))
		case core.DrawAlpha:
			cgContextSetAlpha(ctx, f(op.A))
		case core.DrawClear:
			cgContextClearRect(ctx, Rect(f(op.X), f(op.Y), f(op.W), f(op.H)))
		case core.DrawFillText:
			fillText(ctx, op, fill[len(fill)-1])
		}
	}
}

// fillText draws one line of text with its baseline starting at the op's
// point.
func fillText(ctx uintptr, op core.DrawOp, c core.Color) {
	str := attributed(&core.AttributedText{Text: op.Text, Style: core.TextStyle{Font: op.Font, Color: c}}, nil)
	line := ctLineCreateWithAttrString(str)
	Release(str)
	// Glyphs draw upright in a context whose y axis points down.
	cgContextSetTextMatrix(ctx, affine{A: 1, D: -1})
	cgContextSetTextPosition(ctx, float64(op.X), float64(op.Y))
	ctLineDraw(line, ctx)
	Release(line)
}
