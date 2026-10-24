package driver

// Drawing is a recording of 2D drawing operations, modeled on the web's
// CanvasRenderingContext2D: a current path, fill and stroke settings, and
// a transform, saved and restored as a stack. Once handed to the platform
// it never changes.
type Drawing struct {
	Ops []DrawOp
}

// DrawKind is a drawing operation.
type DrawKind uint8

const (
	DrawSave DrawKind = iota + 1
	DrawRestore
	DrawTranslate // X, Y
	DrawScale     // X, Y
	DrawRotate    // A radians
	DrawBeginPath
	DrawMoveTo    // X, Y
	DrawLineTo    // X, Y
	DrawQuadTo    // X1, Y1 (control), X, Y
	DrawCubicTo   // X1, Y1, X2, Y2 (controls), X, Y
	DrawArc       // X, Y (center), R, A0, A1 (radians), CCW
	DrawRect      // X, Y, W, H
	DrawRoundRect // X, Y, W, H, R
	DrawEllipse   // X, Y (center), RX, RY
	DrawClosePath
	DrawFill        // the current path with the fill color, by EvenOdd's rule
	DrawStroke      // the current path with the stroke settings
	DrawClip        // to the current path, by EvenOdd's rule
	DrawFillColor   // Color, the fill until the next DrawFillGradient
	DrawStrokeColor // Color, the stroke until the next DrawStrokeGradient
	DrawLineWidth   // W
	DrawLineCap     // Cap
	DrawLineJoin    // Join
	DrawAlpha       // A, the global alpha
	DrawFillText    // Text at X, Y (baseline start), in Font
	DrawClear       // X, Y, W, H to transparent

	// DrawFillGradient makes Gradient the fill until the next
	// DrawFillColor. A recording always sets a fill color before it, so a
	// player that does not paint gradients fills with that color instead.
	DrawFillGradient
	// DrawStrokeGradient makes Gradient the stroke until the next
	// DrawStrokeColor, after a stroke color as DrawFillGradient is.
	DrawStrokeGradient
)

// DrawOp is one operation; Kind says which fields it uses.
type DrawOp struct {
	Kind                   DrawKind
	X, Y, X1, Y1, X2, Y2   float32
	W, H, R, A, A0, A1, RY float32
	CCW                    bool
	Color                  Color
	Cap                    LineCap
	Join                   LineJoin
	Text                   string
	Font                   Font
	// EvenOdd fills or clips by the even-odd rule instead of the nonzero
	// one. Players that know only the nonzero rule ignore it.
	EvenOdd  bool
	Gradient *Gradient
}

// Gradient is a linear or radial color gradient that DrawFillGradient and
// DrawStrokeGradient paint with. Its points are in gradient space, which
// Transform maps into the coordinates of the fill or stroke that uses it.
type Gradient struct {
	// Radial makes a radial gradient from the focal circle (X1, Y1, R1)
	// to the outer circle (X2, Y2, R2), as the web's
	// createRadialGradient; otherwise offset 0 is at (X1, Y1) and offset 1
	// at (X2, Y2), and R1 and R2 are unused.
	Radial     bool
	X1, Y1, R1 float32
	X2, Y2, R2 float32
	// Transform is a, b, c, d, e, f, as SVG's matrix(): it maps (x, y) to
	// (a*x + c*y + e, b*x + d*y + f). All zeros is the identity.
	Transform [6]float32
	Spread    GradientSpread
	// Stops are in increasing offsets from 0 to 1, with unpremultiplied
	// colors; between them colors are interpolated.
	Stops []GradientStop
}

// GradientStop is a color at an offset of a Gradient.
type GradientStop struct {
	Offset float32
	Color  Color
}

// GradientSpread is how a Gradient paints beyond offsets 0 and 1.
type GradientSpread uint8

const (
	SpreadPad     GradientSpread = iota // the end colors continue
	SpreadReflect                       // the gradient repeats, mirrored
	SpreadRepeat                        // the gradient repeats
)

// LineCap ends open strokes.
type LineCap uint8

const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

// LineJoin joins stroke segments.
type LineJoin uint8

const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

func (d *Drawing) op(o DrawOp) *Drawing { d.Ops = append(d.Ops, o); return d }

func (d *Drawing) Save() *Drawing { return d.op(DrawOp{Kind: DrawSave}) }

func (d *Drawing) Restore() *Drawing { return d.op(DrawOp{Kind: DrawRestore}) }

func (d *Drawing) Translate(x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawTranslate, X: x, Y: y})
}

func (d *Drawing) Scale(x, y float32) *Drawing { return d.op(DrawOp{Kind: DrawScale, X: x, Y: y}) }

func (d *Drawing) Rotate(radians float32) *Drawing { return d.op(DrawOp{Kind: DrawRotate, A: radians}) }

func (d *Drawing) BeginPath() *Drawing { return d.op(DrawOp{Kind: DrawBeginPath}) }

func (d *Drawing) MoveTo(x, y float32) *Drawing { return d.op(DrawOp{Kind: DrawMoveTo, X: x, Y: y}) }

func (d *Drawing) LineTo(x, y float32) *Drawing { return d.op(DrawOp{Kind: DrawLineTo, X: x, Y: y}) }

func (d *Drawing) ClosePath() *Drawing { return d.op(DrawOp{Kind: DrawClosePath}) }

func (d *Drawing) Fill() *Drawing { return d.op(DrawOp{Kind: DrawFill}) }

func (d *Drawing) Stroke() *Drawing { return d.op(DrawOp{Kind: DrawStroke}) }

func (d *Drawing) Clip() *Drawing { return d.op(DrawOp{Kind: DrawClip}) }

// QuadTo adds a quadratic curve through control point (cx, cy).
func (d *Drawing) QuadTo(cx, cy, x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawQuadTo, X1: cx, Y1: cy, X: x, Y: y})
}

// CubicTo adds a cubic curve through control points 1 and 2.
func (d *Drawing) CubicTo(x1, y1, x2, y2, x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawCubicTo, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
}

// Arc adds an arc of radius r around (x, y) from angle a0 to a1.
func (d *Drawing) Arc(x, y, r, a0, a1 float32, ccw bool) *Drawing {
	return d.op(DrawOp{Kind: DrawArc, X: x, Y: y, R: r, A0: a0, A1: a1, CCW: ccw})
}

func (d *Drawing) Rect(x, y, w, h float32) *Drawing {
	return d.op(DrawOp{Kind: DrawRect, X: x, Y: y, W: w, H: h})
}

// RoundRect adds a rectangle with corners of radius r.
func (d *Drawing) RoundRect(x, y, w, h, r float32) *Drawing {
	return d.op(DrawOp{Kind: DrawRoundRect, X: x, Y: y, W: w, H: h, R: r})
}

// Ellipse adds an ellipse around (x, y) with radii rx and ry.
func (d *Drawing) Ellipse(x, y, rx, ry float32) *Drawing {
	return d.op(DrawOp{Kind: DrawEllipse, X: x, Y: y, R: rx, RY: ry})
}

func (d *Drawing) SetFillColor(c Color) *Drawing { return d.op(DrawOp{Kind: DrawFillColor, Color: c}) }

func (d *Drawing) SetStrokeColor(c Color) *Drawing {
	return d.op(DrawOp{Kind: DrawStrokeColor, Color: c})
}

func (d *Drawing) SetLineWidth(w float32) *Drawing { return d.op(DrawOp{Kind: DrawLineWidth, W: w}) }

func (d *Drawing) SetLineCap(c LineCap) *Drawing { return d.op(DrawOp{Kind: DrawLineCap, Cap: c}) }

func (d *Drawing) SetLineJoin(j LineJoin) *Drawing { return d.op(DrawOp{Kind: DrawLineJoin, Join: j}) }

// SetAlpha multiplies everything drawn after it by alpha.
func (d *Drawing) SetAlpha(alpha float32) *Drawing { return d.op(DrawOp{Kind: DrawAlpha, A: alpha}) }

// FillText draws text with its baseline starting at (x, y).
func (d *Drawing) FillText(text string, x, y float32, font Font) *Drawing {
	if font.Size == 0 {
		font.Size = DefaultFontSize
	}
	if font.Weight == 0 {
		font.Weight = FontWeightNormal
	}
	return d.op(DrawOp{Kind: DrawFillText, Text: text, X: x, Y: y, Font: font})
}

// FillRect fills a rectangle with the fill color, leaving the path.
func (d *Drawing) FillRect(x, y, w, h float32) *Drawing {
	return d.Save().BeginPath().Rect(x, y, w, h).Fill().Restore()
}

// Clear makes a rectangle transparent.
func (d *Drawing) Clear(x, y, w, h float32) *Drawing {
	return d.op(DrawOp{Kind: DrawClear, X: x, Y: y, W: w, H: h})
}
