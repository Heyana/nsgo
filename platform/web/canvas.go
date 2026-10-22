//go:build js && wasm

package web

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
)

var (
	lineCaps  = map[core.LineCap]string{core.CapButt: "butt", core.CapRound: "round", core.CapSquare: "square"}
	lineJoins = map[core.LineJoin]string{core.JoinMiter: "miter", core.JoinRound: "round", core.JoinBevel: "bevel"}
)

// paint plays a canvas view's drawing into its <canvas>, whose pixels
// follow the pixel ratio. The drawing's origin is inside the border.
func (s *surface) paint(v *view) {
	if v.drawing == nil {
		return
	}
	ratio := float64(s.env.PixelRatio)
	if ratio <= 0 {
		ratio = 1
	}
	w := math.Max(float64(v.size.Width-2*v.border), 0)
	h := math.Max(float64(v.size.Height-2*v.border), 0)
	// Setting the size clears the canvas and resets its context.
	v.inner.Set("width", math.Ceil(w*ratio))
	v.inner.Set("height", math.Ceil(h*ratio))
	ctx := v.inner.Call("getContext", "2d")
	ctx.Call("scale", ratio, ratio)
	for _, op := range v.drawing.Ops {
		play(ctx, &op)
	}
}

func play(ctx js.Value, op *core.DrawOp) {
	f := func(x float32) float64 { return float64(x) }
	switch op.Kind {
	case core.DrawSave:
		ctx.Call("save")
	case core.DrawRestore:
		ctx.Call("restore")
	case core.DrawTranslate:
		ctx.Call("translate", f(op.X), f(op.Y))
	case core.DrawScale:
		ctx.Call("scale", f(op.X), f(op.Y))
	case core.DrawRotate:
		ctx.Call("rotate", f(op.A))
	case core.DrawBeginPath:
		ctx.Call("beginPath")
	case core.DrawMoveTo:
		ctx.Call("moveTo", f(op.X), f(op.Y))
	case core.DrawLineTo:
		ctx.Call("lineTo", f(op.X), f(op.Y))
	case core.DrawQuadTo:
		ctx.Call("quadraticCurveTo", f(op.X1), f(op.Y1), f(op.X), f(op.Y))
	case core.DrawCubicTo:
		ctx.Call("bezierCurveTo", f(op.X1), f(op.Y1), f(op.X2), f(op.Y2), f(op.X), f(op.Y))
	case core.DrawArc:
		ctx.Call("arc", f(op.X), f(op.Y), f(op.R), f(op.A0), f(op.A1), op.CCW)
	case core.DrawRect:
		ctx.Call("rect", f(op.X), f(op.Y), f(op.W), f(op.H))
	case core.DrawRoundRect:
		// arcTo rather than roundRect, which older browsers lack.
		x, y, w, h := f(op.X), f(op.Y), f(op.W), f(op.H)
		r := math.Max(math.Min(f(op.R), math.Min(math.Abs(w), math.Abs(h))/2), 0)
		ctx.Call("moveTo", x+r, y)
		ctx.Call("arcTo", x+w, y, x+w, y+h, r)
		ctx.Call("arcTo", x+w, y+h, x, y+h, r)
		ctx.Call("arcTo", x, y+h, x, y, r)
		ctx.Call("arcTo", x, y, x+w, y, r)
		ctx.Call("closePath")
	case core.DrawEllipse:
		ctx.Call("ellipse", f(op.X), f(op.Y), f(op.R), f(op.RY), 0, 0, 2*math.Pi)
	case core.DrawClosePath:
		ctx.Call("closePath")
	case core.DrawFill:
		ctx.Call("fill")
	case core.DrawStroke:
		ctx.Call("stroke")
	case core.DrawClip:
		ctx.Call("clip")
	case core.DrawFillColor:
		ctx.Set("fillStyle", cssColor(op.Color))
	case core.DrawStrokeColor:
		ctx.Set("strokeStyle", cssColor(op.Color))
	case core.DrawLineWidth:
		ctx.Set("lineWidth", f(op.W))
	case core.DrawLineCap:
		ctx.Set("lineCap", lineCaps[op.Cap])
	case core.DrawLineJoin:
		ctx.Set("lineJoin", lineJoins[op.Join])
	case core.DrawAlpha:
		ctx.Set("globalAlpha", f(op.A))
	case core.DrawFillText:
		ctx.Set("font", fontCSS(op.Font))
		ctx.Call("fillText", op.Text, f(op.X), f(op.Y))
	case core.DrawClear:
		ctx.Call("clearRect", f(op.X), f(op.Y), f(op.W), f(op.H))
	}
}

// fontCSS is the CSS font shorthand of fnt.
func fontCSS(fnt core.Font) string {
	family := fontFamily
	if fnt.Family != "" {
		family = strconv.Quote(fnt.Family) + ", " + fontFamily
	}
	style := ""
	if fnt.Italic {
		style = "italic "
	}
	return style + strconv.Itoa(int(fnt.Weight)) + " " + px(fnt.Size) + " " + family
}
