//go:build linux && !android

package linux

import (
	"math"

	"github.com/oneo-me/nsgo/internal/driver"
)

// canvasState is what Save and Restore keep while a Drawing plays.
type canvasState struct {
	fill, stroke driver.Color
	width        float64
	cap, join    int32
	alpha        float64
}

// playCanvas replays d into cr at r, as the web's 2D context does.
func (w *window) playCanvas(d *driver.Drawing, cr uintptr, r rect) {
	cairoSave(cr)
	cairoRectangle(cr, r.x, r.y, r.w, r.h)
	cairoClip(cr)
	cairoTranslate(cr, r.x, r.y)
	st := canvasState{fill: driver.RGB(0x000000), stroke: driver.RGB(0x000000), width: 1, alpha: 1}
	var stack []canvasState
	for i := range d.Ops {
		op := &d.Ops[i]
		switch op.Kind {
		case driver.DrawSave:
			cairoSave(cr)
			stack = append(stack, st)
		case driver.DrawRestore:
			if n := len(stack); n > 0 {
				st = stack[n-1]
				stack = stack[:n-1]
				cairoRestore(cr)
			}
		case driver.DrawTranslate:
			cairoTranslate(cr, float64(op.X), float64(op.Y))
		case driver.DrawScale:
			cairoScale(cr, float64(op.X), float64(op.Y))
		case driver.DrawRotate:
			cairoRotate(cr, float64(op.A))
		case driver.DrawBeginPath:
			cairoNewPath(cr)
		case driver.DrawMoveTo:
			cairoMoveTo(cr, float64(op.X), float64(op.Y))
		case driver.DrawLineTo:
			cairoLineTo(cr, float64(op.X), float64(op.Y))
		case driver.DrawQuadTo:
			var x0, y0 float64
			cairoGetCurrentPoint(cr, &x0, &y0)
			cairoCurveTo(cr, x0+2*(float64(op.X1)-x0)/3, y0+2*(float64(op.Y1)-y0)/3,
				float64(op.X)+2*(float64(op.X1)-float64(op.X))/3, float64(op.Y)+2*(float64(op.Y1)-float64(op.Y))/3,
				float64(op.X), float64(op.Y))
		case driver.DrawCubicTo:
			cairoCurveTo(cr, float64(op.X1), float64(op.Y1), float64(op.X2), float64(op.Y2), float64(op.X), float64(op.Y))
		case driver.DrawArc:
			if op.CCW {
				cairoArcNegative(cr, float64(op.X), float64(op.Y), float64(op.R), float64(op.A0), float64(op.A1))
			} else {
				cairoArc(cr, float64(op.X), float64(op.Y), float64(op.R), float64(op.A0), float64(op.A1))
			}
		case driver.DrawRect:
			cairoRectangle(cr, float64(op.X), float64(op.Y), float64(op.W), float64(op.H))
		case driver.DrawRoundRect:
			roundRect(cr, rect{x: float64(op.X), y: float64(op.Y), w: float64(op.W), h: float64(op.H), r: float64(op.R)})
		case driver.DrawEllipse:
			cairoSave(cr)
			cairoTranslate(cr, float64(op.X), float64(op.Y))
			cairoScale(cr, float64(op.R), float64(op.RY))
			cairoArc(cr, 0, 0, 1, 0, 2*math.Pi)
			cairoRestore(cr)
		case driver.DrawClosePath:
			cairoClosePath(cr)
		case driver.DrawFill:
			setColorAlpha(cr, st.fill, st.alpha)
			cairoFill(cr)
		case driver.DrawStroke:
			setColorAlpha(cr, st.stroke, st.alpha)
			cairoSetLineWidth(cr, st.width)
			cairoSetLineCap(cr, st.cap)
			cairoSetLineJoin(cr, st.join)
			cairoStroke(cr)
		case driver.DrawClip:
			cairoClip(cr)
		case driver.DrawFillColor:
			st.fill = op.Color
		case driver.DrawStrokeColor:
			st.stroke = op.Color
		case driver.DrawLineWidth:
			st.width = float64(op.W)
		case driver.DrawLineCap:
			st.cap = int32(op.Cap)
		case driver.DrawLineJoin:
			st.join = int32(op.Join)
		case driver.DrawAlpha:
			st.alpha = float64(op.A)
		case driver.DrawFillText:
			drawCanvasText(cr, op.Text, float64(op.X), float64(op.Y), op.Font, st.fill, st.alpha)
		case driver.DrawClear:
			cairoSave(cr)
			cairoSetOperator(cr, operatorClear)
			cairoRectangle(cr, float64(op.X), float64(op.Y), float64(op.W), float64(op.H))
			cairoFill(cr)
			cairoRestore(cr)
		}
	}
	cairoRestore(cr)
}

// drawCanvasText draws text with its baseline at x, y.
func drawCanvasText(cr uintptr, text string, x, y float64, font driver.Font, c driver.Color, alpha float64) {
	if text == "" {
		return
	}
	if font.Size == 0 {
		font.Size = driver.DefaultFontSize
	}
	l := pangoCairoCreateLayout(cr)
	defer gObjectUnref(l)
	desc := pangoFontDescriptionNew()
	defer pangoFontDescriptionFree(desc)
	if font.Family != "" {
		pangoFontDescriptionSetFamily(desc, cstr(font.Family))
	}
	pangoFontDescriptionSetSize(desc, int32(font.Size*pangoScale+0.5))
	if font.Weight != 0 {
		pangoFontDescriptionSetWeight(desc, int32(font.Weight))
	}
	if font.Italic {
		pangoFontDescriptionSetStyle(desc, pangoStyleItalic)
	}
	pangoLayoutSetFontDescription(l, desc)
	pangoLayoutSetText(l, cstr(text), int32(len(text)))
	baseline := float64(pangoLayoutGetBaseline(l)) / pangoScale
	cairoMoveTo(cr, x, y-baseline)
	setColorAlpha(cr, c, alpha)
	pangoCairoShowLayout(cr, l)
}

// setColorAlpha sets c as the source with alpha multiplied by k.
func setColorAlpha(cr uintptr, c driver.Color, k float64) {
	r, g, b, a := c.Components()
	cairoSetSourceRGBA(cr, r, g, b, a*k)
}
