//go:build js && wasm

package web

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/oneo-me/nsgo/internal/driver"
)

var (
	lineCaps  = map[driver.LineCap]string{driver.CapButt: "butt", driver.CapRound: "round", driver.CapSquare: "square"}
	lineJoins = map[driver.LineJoin]string{driver.JoinMiter: "miter", driver.JoinRound: "round", driver.JoinBevel: "bevel"}
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
	w := math.Max(float64(v.size.Width-v.border.Left-v.border.Right), 0)
	h := math.Max(float64(v.size.Height-v.border.Top-v.border.Bottom), 0)
	// Setting the size clears the canvas and resets its context.
	v.inner.Set("width", math.Ceil(w*ratio))
	v.inner.Set("height", math.Ceil(h*ratio))
	ctx := v.inner.Call("getContext", "2d")
	ctx.Call("scale", ratio, ratio)
	for _, op := range v.drawing.Ops {
		play(ctx, &op)
	}
}

func play(ctx js.Value, op *driver.DrawOp) {
	f := func(x float32) float64 { return float64(x) }
	switch op.Kind {
	case driver.DrawSave:
		ctx.Call("save")
	case driver.DrawRestore:
		ctx.Call("restore")
	case driver.DrawTranslate:
		ctx.Call("translate", f(op.X), f(op.Y))
	case driver.DrawScale:
		ctx.Call("scale", f(op.X), f(op.Y))
	case driver.DrawRotate:
		ctx.Call("rotate", f(op.A))
	case driver.DrawBeginPath:
		ctx.Call("beginPath")
	case driver.DrawMoveTo:
		ctx.Call("moveTo", f(op.X), f(op.Y))
	case driver.DrawLineTo:
		ctx.Call("lineTo", f(op.X), f(op.Y))
	case driver.DrawQuadTo:
		ctx.Call("quadraticCurveTo", f(op.X1), f(op.Y1), f(op.X), f(op.Y))
	case driver.DrawCubicTo:
		ctx.Call("bezierCurveTo", f(op.X1), f(op.Y1), f(op.X2), f(op.Y2), f(op.X), f(op.Y))
	case driver.DrawArc:
		ctx.Call("arc", f(op.X), f(op.Y), f(op.R), f(op.A0), f(op.A1), op.CCW)
	case driver.DrawRect:
		ctx.Call("rect", f(op.X), f(op.Y), f(op.W), f(op.H))
	case driver.DrawRoundRect:
		// arcTo rather than roundRect, which older browsers lack.
		x, y, w, h := f(op.X), f(op.Y), f(op.W), f(op.H)
		r := math.Max(math.Min(f(op.R), math.Min(math.Abs(w), math.Abs(h))/2), 0)
		ctx.Call("moveTo", x+r, y)
		ctx.Call("arcTo", x+w, y, x+w, y+h, r)
		ctx.Call("arcTo", x+w, y+h, x, y+h, r)
		ctx.Call("arcTo", x, y+h, x, y, r)
		ctx.Call("arcTo", x, y, x+w, y, r)
		ctx.Call("closePath")
	case driver.DrawEllipse:
		ctx.Call("ellipse", f(op.X), f(op.Y), f(op.R), f(op.RY), 0, 0, 2*math.Pi)
	case driver.DrawClosePath:
		ctx.Call("closePath")
	case driver.DrawFill:
		ctx.Call("fill")
	case driver.DrawStroke:
		ctx.Call("stroke")
	case driver.DrawClip:
		ctx.Call("clip")
	case driver.DrawFillColor:
		ctx.Set("fillStyle", cssColor(op.Color))
	case driver.DrawStrokeColor:
		ctx.Set("strokeStyle", cssColor(op.Color))
	case driver.DrawLineWidth:
		ctx.Set("lineWidth", f(op.W))
	case driver.DrawLineCap:
		ctx.Set("lineCap", lineCaps[op.Cap])
	case driver.DrawLineJoin:
		ctx.Set("lineJoin", lineJoins[op.Join])
	case driver.DrawAlpha:
		ctx.Set("globalAlpha", f(op.A))
	case driver.DrawFillText:
		ctx.Set("font", fontCSS(op.Font))
		ctx.Call("fillText", op.Text, f(op.X), f(op.Y))
	case driver.DrawClear:
		ctx.Call("clearRect", f(op.X), f(op.Y), f(op.W), f(op.H))
	}
}

// fontCSS is the CSS font shorthand of fnt.
func fontCSS(fnt driver.Font) string {
	family := cssFamily(fnt.Family)
	style := ""
	if fnt.Italic {
		style = "italic "
	}
	return style + strconv.Itoa(int(fnt.Weight)) + " " + px(fnt.Size) + " " + family
}
