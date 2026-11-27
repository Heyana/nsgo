package driver

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// Drawing is a recording of 2D drawing operations, modeled on the web's
// CanvasRenderingContext2D: a current path, fill and stroke settings, and
// a transform, saved and restored as a stack. Once handed to the platform
// it never changes.
//
// Methods that need the path or the transform so far (ArcTo, EllipseArc,
// FillPath, the queries) replay the operations recorded since they last
// looked, so a drawing that never asks pays nothing for them.
type Drawing struct {
	Ops []DrawOp
	// Images holds the image sources drawn by DrawImage, DrawVertices and
	// DrawAtlas ops, referenced by DrawOp.ImageIndex (1-based; 0 means none).
	// Consecutive ops drawing the same image share one entry.
	Images []ImageSource
	// Sprites holds the sprite records drawn by DrawAtlas ops, referenced
	// by DrawOp.SpriteStart and DrawOp.SpriteLen.
	Sprites []AtlasSprite
	// Slots bind arguments of Ops to animated nodes: the platform plays
	// the ops with the nodes' values in their place, and plays them again
	// when one changes, with no work on the engine.
	Slots []DrawSlot
	// Nodes are named parts of what it draws, which assistive technology
	// and tests see as the canvas's children.
	Nodes []CanvasNode
	// ViewPaints name the ops whose value is their canvas view's paint
	// (FillFromView and the like), which the core sets before the platform
	// plays the drawing.
	ViewPaints []ViewPaint
	// Resolution multiplies the pixels per point the platform plays the
	// drawing at (core.Canvas.ResolutionScale), and Upscale is how it
	// stretches the fewer pixels to the canvas's box: ResolutionScale reads it.
	Resolution float32
	Upscale    ImageSampling

	text     TextEngine // measures text; nil approximates it
	measured map[textKey]TextMetrics
	marks    map[FontKey]float32 // widthMark's width, by font
	track    *drawTrack
	// pathAside is set while the current path is only the track's,
	// after a FillPath, StrokePath or ClipPath (restorePath).
	pathAside bool

	sampling      ImageSampling
	samplingStack []ImageSampling
	// shader shades the images drawn (SetShader), and shaderStack keeps
	// it for Restore.
	shader      *Shader
	shaderStack []*Shader
	shaded      bool
	// lease counts active platform retains on a pooled drawing (nil for
	// unpooled drawings). Stored behind a pointer so shallow Drawing copies
	// (core's scaled/Painted/copyDrawingFrames) share the lease without
	// copying an atomic value.
	lease *atomic.Int32
}

// NewDrawing returns an empty drawing that measures text with text, as
// the core records a Canvas's: MeasureText, TextMetrics and the
// alignment of texts ask it.
func NewDrawing(text TextEngine) *Drawing { return &Drawing{text: text} }

// NewPooledDrawing returns an empty drawing with a reuse lease so core can
// reset and reuse its buffers across frames once platforms have released it.
func NewPooledDrawing(text TextEngine) *Drawing {
	return &Drawing{text: text, lease: new(atomic.Int32)}
}

// Retain marks d as in use by a platform until a matching Release call.
func (d *Drawing) Retain() {
	if d != nil && d.lease != nil {
		d.lease.Add(1)
	}
}

// Release ends one platform retain of d.
func (d *Drawing) Release() {
	if d != nil && d.lease != nil {
		d.lease.Add(-1)
	}
}

// InUse reports whether any platform still holds a retain on d.
func (d *Drawing) InUse() bool {
	return d != nil && d.lease != nil && d.lease.Load() > 0
}

// RetainBatchDrawings retains every canvas drawing carried by b.
func RetainBatchDrawings(b *MountBatch) {
	if b == nil {
		return
	}
	for i := range b.Items {
		if b.Items[i].Kind == ItemCanvas {
			b.Items[i].Drawing.Retain()
		}
	}
}

// ReleaseBatchDrawings releases every canvas drawing carried by b.
func ReleaseBatchDrawings(b *MountBatch) {
	if b == nil {
		return
	}
	for i := range b.Items {
		if b.Items[i].Kind == ItemCanvas {
			b.Items[i].Drawing.Release()
		}
	}
}

// Reset clears d's recorded operations and state while retaining the capacity
// of its slices (Ops, Images, Sprites, Slots, Nodes, ViewPaints, and internal
// stacks) so subsequent frames record without heap allocations. A drawing
// handed to a platform must not be reset while the platform still reads it.
func (d *Drawing) Reset() {
	clear(d.Ops)
	d.Ops = d.Ops[:0]
	clear(d.Images)
	d.Images = d.Images[:0]
	d.Sprites = d.Sprites[:0]
	d.Slots = d.Slots[:0]
	clear(d.Nodes)
	d.Nodes = d.Nodes[:0]
	clear(d.ViewPaints)
	d.ViewPaints = d.ViewPaints[:0]
	d.Resolution = 0
	d.Upscale = 0
	clear(d.measured)
	clear(d.marks)
	if d.track != nil {
		d.track.reset()
	}
	d.pathAside = false
	d.sampling = ImageSamplingSmooth
	d.samplingStack = d.samplingStack[:0]
	d.shader = nil
	clear(d.shaderStack)
	d.shaderStack = d.shaderStack[:0]
	d.shaded = false
}

// Image returns the image source op draws, or nil when op has none or its
// source is empty.
func (d *Drawing) Image(op *DrawOp) *ImageSource {
	if d == nil || op == nil || op.ImageIndex <= 0 || int(op.ImageIndex) > len(d.Images) {
		return nil
	}
	img := &d.Images[op.ImageIndex-1]
	if img.Empty() {
		return nil
	}
	return img
}

// AtlasSprites returns the sprite list recorded for a DrawAtlas op, or nil
// when op has none.
func (d *Drawing) AtlasSprites(op *DrawOp) AtlasSprites {
	if d == nil || op == nil || op.SpriteLen <= 0 {
		return nil
	}
	start := int(op.SpriteStart)
	end := start + int(op.SpriteLen)
	if start < 0 || end > len(d.Sprites) {
		return nil
	}
	return AtlasSprites(d.Sprites[start:end])
}

// addImage appends img to d.Images and returns its 1-based index, sharing
// the previous entry when consecutive image ops draw the same source.
// Returns 0 when img is empty.
func (d *Drawing) addImage(img ImageSource) int32 {
	if img.Empty() {
		return 0
	}
	if n := len(d.Images); n > 0 && sameDrawingImage(d.Images[n-1], img) {
		return int32(n)
	}
	d.Images = append(d.Images, img)
	return int32(len(d.Images))
}

func sameDrawingImage(a, b ImageSource) bool {
	if a.URI != b.URI || a.Size != b.Size || a.Scales != b.Scales ||
		len(a.Data) != len(b.Data) || (len(a.Data) > 0 && &a.Data[0] != &b.Data[0]) ||
		a.Pixels != b.Pixels || a.Offscreen != b.Offscreen || a.Version != b.Version {
		return false
	}
	if len(a.Variants) == 0 && len(b.Variants) == 0 {
		return true
	}
	return maps.Equal(a.Variants, b.Variants)
}

// ResolutionScale is what d's Resolution multiplies the pixels per point a platform
// plays it at by: within (0, 1), or 1 for a nil d or any other value,
// which plays it at the screen's density.
func (d *Drawing) ResolutionScale() float64 {
	if d == nil || !(d.Resolution > 0 && d.Resolution < 1) {
		return 1
	}
	return float64(d.Resolution)
}

// Smooth reports whether the pixels of d, played below the screen's
// density, are stretched smoothly to the canvas's box rather than to
// their nearest (Upscale); a nil d is smooth.
func (d *Drawing) Smooth() bool { return d == nil || d.Upscale != ImageSamplingNearest }

// CanvasPixels is how many pixels length points take at scale pixels per
// point: rounded up, and at least 1 for a length above 0.
func CanvasPixels(length Points, scale float64) int {
	if !(length > 0) {
		return 0
	}
	return max(int(math.Ceil(float64(length)*scale-1e-6)), 1)
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
	DrawFill        // the current path with the fill paint, by EvenOdd's rule
	DrawStroke      // the current path with the stroke settings
	DrawClip        // to the current path, by EvenOdd's rule
	DrawFillColor   // Color, the fill until the next DrawFillGradient
	DrawStrokeColor // Color, the stroke until the next DrawStrokeGradient
	DrawLineWidth   // W
	DrawLineCap     // Cap
	DrawLineJoin    // Join
	DrawAlpha       // A, the global alpha
	// DrawFillText draws Text in Font with its baseline starting at X+X1,
	// Y+Y1: X, Y is the point the app gave and X1, Y1 how far the
	// recorder moved the text from it for TextAlign and TextBaseline.
	DrawFillText
	DrawClear // X, Y, W, H to transparent

	// DrawFillGradient makes Gradient the fill until the next
	// DrawFillColor. A recording always sets a fill color before it, the
	// gradient's first, so a player that does not paint gradients fills
	// with that color instead.
	DrawFillGradient
	// DrawStrokeGradient makes Gradient the stroke until the next
	// DrawStrokeColor, after a stroke color as DrawFillGradient is.
	DrawStrokeGradient

	// DrawTransform multiplies the transform by the matrix with columns
	// (X1, Y1), (X2, Y2) and (X, Y): the web's transform(a, b, c, d, e,
	// f) with a, b = X1, Y1, c, d = X2, Y2 and e, f = X, Y.
	DrawTransform
	// DrawSetTransform replaces the transform with that matrix, from the
	// canvas's own coordinates.
	DrawSetTransform
	// DrawComposite sets how later drawing mixes with the canvas
	// (Composite).
	DrawComposite
	// DrawLineDash sets the stroke's dashes (Dash; nil or empty is
	// solid) and DrawLineDashOffset where they start (W).
	DrawLineDash
	DrawLineDashOffset
	// DrawMiterLimit sets how far a miter join reaches, in line widths
	// (W).
	DrawMiterLimit
	// DrawShadow sets the shadow of later drawing: Color (transparent is
	// none), offset X, Y and blur R, in the canvas's own coordinates,
	// whatever the transform.
	DrawShadow
	// DrawStrokeText draws Text as DrawFillText does, stroked with the
	// stroke settings.
	DrawStrokeText
	// DrawImage draws Image into the rectangle X, Y, W, H, at its natural
	// size when W and H are 0, from the part X1, Y1, X2 (width), Y2
	// (height) of it in its own points, all of it when X2 and Y2 are 0.
	// The platform loads the image and draws again once it has it.
	DrawImage
	// DrawTextAlign and DrawTextBaseline record the TextAlign and
	// TextBaseline later texts take. The recorder places the texts with
	// them, so players skip these.
	DrawTextAlign
	DrawTextBaseline
	// DrawVertices draws Image through Mesh, with its bounds in X, Y, W, H.
	DrawVertices
	// DrawAtlas draws a list of sprites from Image, in Atlas's order.
	DrawAtlas
	// DrawShader runs Shader's program over the rectangle X, Y, W, H.
	DrawShader
)

// UsesPath reports whether an op of kind k builds on the current path or
// closes, fills, strokes or clips with it.
func (k DrawKind) UsesPath() bool {
	return k.BuildsPath() || k == DrawFill || k == DrawStroke || k == DrawClip
}

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
	// one.
	EvenOdd   bool
	Gradient  *CanvasGradient
	Composite CompositeOp
	Dash      *LineDash
	// ImageIndex is a 1-based index into Drawing.Images (0 means no image).
	ImageIndex int32
	Sampling   ImageSampling
	Mesh       *CanvasMesh
	// SpriteStart and SpriteLen select a DrawAtlas op's sprites in Drawing.Sprites.
	SpriteStart int32
	SpriteLen   int32
	// Tinted multiplies a DrawImage's source channels by Color before it composites.
	Tinted bool
	// Shader shades a DrawShader's rectangle, or the image a DrawImage,
	// DrawVertices or DrawAtlas draws (SetShader); the zero paint draws it
	// plain. Its Lanes are the program's own, without the bridge's, and
	// the drawing's slots bind those its Nodes name, which players ignore.
	Shader ShaderPaint
	// TextAlign and TextBaseline are what a text op was placed with.
	TextAlign    TextAlign
	TextBaseline TextBaseline
}

// LineDash is the lengths of a stroke's dashes and gaps in turn, an even
// number of them.
type LineDash []float32

// DrawArg is an argument of a DrawOp that a DrawSlot binds.
type DrawArg uint8

const (
	ArgX DrawArg = iota
	ArgY
	ArgX1
	ArgY1
	ArgX2
	ArgY2
	ArgW
	ArgH
	ArgR
	ArgA
	ArgA0
	ArgA1
	ArgRY
	// ArgColor takes a color node, such as an interpolation to Colors.
	ArgColor
	// ArgLane is a lane of the op's shader, which only the drawing binds
	// (ShaderPaint).
	ArgLane
)

// DrawSlot binds argument Arg of operation Op to animated node Node.
type DrawSlot struct {
	Op   int
	Arg  DrawArg
	Node uint32
	// lane and channel are what an ArgLane slot sets.
	lane    int
	channel Channel
}

// Set returns op with the slot's argument set to num, or to col for
// ArgColor.
func (s DrawSlot) Set(op DrawOp, num float64, col Color) DrawOp {
	v := float32(num)
	switch s.Arg {
	case ArgX:
		op.X = v
	case ArgY:
		op.Y = v
	case ArgX1:
		op.X1 = v
	case ArgY1:
		op.Y1 = v
	case ArgX2:
		op.X2 = v
	case ArgY2:
		op.Y2 = v
	case ArgW:
		op.W = v
	case ArgH:
		op.H = v
	case ArgR:
		op.R = v
	case ArgA:
		op.A = v
	case ArgA0:
		op.A0 = v
	case ArgA1:
		op.A1 = v
	case ArgRY:
		op.RY = v
	case ArgColor:
		op.Color = col
	case ArgLane:
		return setLane(op, s.lane, s.channel.Of(num, col))
	}
	return op
}

// AnimatedArg is an animated node a drawing binds an argument to.
type AnimatedArg interface {
	// AnimatedNodeID is the node's ID in the animation graph; 0 binds
	// nothing.
	AnimatedNodeID() uint32
}

// CanvasNode is a named part of a drawing, such as a chart's bar or a
// grid's cell: assistive technology and tests see it as a child of the
// canvas, and a press on it presses at its center.
type CanvasNode struct {
	// Bounds is where it is in the canvas, in the canvas's own points,
	// whatever the transform.
	Bounds   Bounds
	Role     Role
	Label    string
	TestID   string
	Selected bool
}

// CompositeOp is how drawing mixes with what the canvas holds, as the
// web's globalCompositeOperation: Porter and Duff's operators, then the
// blend modes.
type CompositeOp uint8

const (
	CompositeSourceOver CompositeOp = iota
	CompositeSourceIn
	CompositeSourceOut
	CompositeSourceAtop
	CompositeDestinationOver
	CompositeDestinationIn
	CompositeDestinationOut
	CompositeDestinationAtop
	CompositeLighter
	CompositeCopy
	CompositeXor
	CompositeMultiply
	CompositeScreen
	CompositeOverlay
	CompositeDarken
	CompositeLighten
	CompositeColorDodge
	CompositeColorBurn
	CompositeHardLight
	CompositeSoftLight
	CompositeDifference
	CompositeExclusion
	CompositeHue
	CompositeSaturation
	CompositeColor
	CompositeLuminosity
)

var compositeNames = [...]string{"source-over", "source-in", "source-out", "source-atop", "destination-over",
	"destination-in", "destination-out", "destination-atop", "lighter", "copy", "xor"}

// Name returns the operator's name on the web.
func (c CompositeOp) Name() string {
	if b, ok := c.Blend(); ok {
		return b.Name()
	}
	if int(c) < len(compositeNames) {
		return compositeNames[c]
	}
	return compositeNames[0]
}

// Blend returns the blend mode of a blend operator, which mixes colors
// and composites as source-over.
func (c CompositeOp) Blend() (BlendMode, bool) {
	if c >= CompositeMultiply && c <= CompositeLuminosity {
		return BlendMultiply + BlendMode(c-CompositeMultiply), true
	}
	return BlendNormal, false
}

// Unbounded reports whether the operator changes the canvas outside what
// is drawn, clearing it there: source-in, source-out, destination-in,
// destination-atop and copy. Players that composite only where a shape
// covers draw it into a layer of the whole canvas first.
func (c CompositeOp) Unbounded() bool {
	switch c {
	case CompositeSourceIn, CompositeSourceOut, CompositeDestinationIn, CompositeDestinationAtop, CompositeCopy:
		return true
	}
	return false
}

// TextBaseline is the line of a text that the point it is drawn at
// sits on, as the web's textBaseline.
type TextBaseline uint8

const (
	BaselineAlphabetic TextBaseline = iota
	BaselineTop
	BaselineHanging
	BaselineMiddle
	BaselineIdeographic
	BaselineBottom
)

// FillRule is how a fill or a clip decides what is inside a path.
type FillRule uint8

const (
	NonZero FillRule = iota
	EvenOdd
)

// TextMetrics are a text's measurements in a font: its advance width, and
// how far its font reaches above and below the baseline, as the line it
// is laid out on reports them.
type TextMetrics struct {
	Width, Ascent, Descent, LineHeight float32
}

// CanvasGradient is a linear, radial or conic color gradient that
// DrawFillGradient and DrawStrokeGradient paint with. Its points are in
// gradient space, which Transform maps into the coordinates of the fill
// or stroke that uses it.
type CanvasGradient struct {
	// Radial makes a radial gradient from the focal circle (X1, Y1, R1)
	// to the outer circle (X2, Y2, R2), as the web's
	// createRadialGradient. Conic makes one around (X1, Y1), offset 0 at
	// Angle radians from the x axis and the offsets turning clockwise, as
	// createConicGradient. Otherwise offset 0 is at (X1, Y1) and offset 1
	// at (X2, Y2).
	Radial, Conic bool
	X1, Y1, R1    float32
	X2, Y2, R2    float32
	Angle         float32
	// Transform is a, b, c, d, e, f, as SVG's matrix(): it maps (x, y) to
	// (a*x + c*y + e, b*x + d*y + f). All zeros is the identity.
	Transform [6]float32
	Spread    GradientSpread
	// Stops are in increasing offsets from 0 to 1, with unpremultiplied
	// colors; between them colors are interpolated.
	Stops []GradientStop
}

// CanvasLinear returns a gradient from (x0, y0) to (x1, y1).
func CanvasLinear(x0, y0, x1, y1 float32, stops ...GradientStop) *CanvasGradient {
	return &CanvasGradient{X1: x0, Y1: y0, X2: x1, Y2: y1, Stops: stops}
}

// CanvasRadial returns a gradient from the circle (x0, y0, r0) to the
// circle (x1, y1, r1).
func CanvasRadial(x0, y0, r0, x1, y1, r1 float32, stops ...GradientStop) *CanvasGradient {
	return &CanvasGradient{Radial: true, X1: x0, Y1: y0, R1: r0, X2: x1, Y2: y1, R2: r1, Stops: stops}
}

// CanvasConic returns a gradient around (x, y) starting at angle radians.
func CanvasConic(angle, x, y float32, stops ...GradientStop) *CanvasGradient {
	return &CanvasGradient{Conic: true, X1: x, Y1: y, Angle: angle, Stops: stops}
}

// Matrix returns g's transform, the identity for all zeros.
func (g *CanvasGradient) Matrix() [6]float32 {
	if g.Transform == ([6]float32{}) {
		return [6]float32{1, 0, 0, 1, 0, 0}
	}
	return g.Transform
}

// At returns g's color at offset t, its stops clamped and interpolated in
// unpremultiplied sRGB, for players that sample gradients themselves.
func (g *CanvasGradient) At(t float64) Color {
	s := g.Stops
	if len(s) == 0 {
		return 0
	}
	switch g.Spread {
	case SpreadRepeat:
		t -= math.Floor(t)
	case SpreadReflect:
		t = math.Mod(math.Abs(t), 2)
		if t > 1 {
			t = 2 - t
		}
	}
	if t <= float64(s[0].Offset) {
		return s[0].Color
	}
	for i := 1; i < len(s); i++ {
		if t <= float64(s[i].Offset) {
			span := float64(s[i].Offset - s[i-1].Offset)
			if span <= 0 {
				return s[i].Color
			}
			return lerpColor(s[i-1].Color, s[i].Color, (t-float64(s[i-1].Offset))/span)
		}
	}
	return s[len(s)-1].Color
}

// GradientStop is a color at an offset of a gradient.
type GradientStop struct {
	Offset float32
	Color  Color
}

// GradientSpread is how a CanvasGradient paints beyond offsets 0 and 1.
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

// op records o, after the current path a FillPath left aside when o
// builds on it or uses it.
func (d *Drawing) op(o DrawOp) *Drawing {
	switch {
	case o.Kind == DrawBeginPath:
		d.pathAside = false
	case o.Kind.UsesPath():
		d.restorePath()
	}
	if o.ImageIndex > 0 {
		o.Sampling = d.sampling
		if d.shader != nil {
			o.Shader = ShaderPaint{d.shader}
		}
	}
	d.Ops = append(d.Ops, o)
	if o.Shader.s != nil {
		d.bindLanes(o.Shader.s)
	}
	return d
}

func (d *Drawing) Save() *Drawing {
	d.samplingStack = append(d.samplingStack, d.sampling)
	d.shaderStack = append(d.shaderStack, d.shader)
	return d.op(DrawOp{Kind: DrawSave})
}

func (d *Drawing) Restore() *Drawing {
	if n := len(d.samplingStack); n != 0 {
		d.sampling, d.samplingStack = d.samplingStack[n-1], d.samplingStack[:n-1]
		d.shader, d.shaderStack = d.shaderStack[n-1], d.shaderStack[:n-1]
	}
	return d.op(DrawOp{Kind: DrawRestore})
}

// SetImageSmoothing selects how subsequent image draws scale raster images.
// Smoothing is on by default. Save and Restore keep this setting.
func (d *Drawing) SetImageSmoothing(enabled bool) *Drawing {
	d.sampling = ImageSamplingSmooth
	if !enabled {
		d.sampling = ImageSamplingNearest
	}
	return d
}

func (d *Drawing) Translate(x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawTranslate, X: x, Y: y})
}

func (d *Drawing) Scale(x, y float32) *Drawing { return d.op(DrawOp{Kind: DrawScale, X: x, Y: y}) }

func (d *Drawing) Rotate(radians float32) *Drawing { return d.op(DrawOp{Kind: DrawRotate, A: radians}) }

// Transform multiplies the transform by the matrix that maps (x, y) to
// (a*x + c*y + e, b*x + d*y + f), as the web's transform.
func (d *Drawing) Transform(a, b, c, dd, e, f float32) *Drawing {
	return d.op(DrawOp{Kind: DrawTransform, X1: a, Y1: b, X2: c, Y2: dd, X: e, Y: f})
}

// SetTransform replaces the transform with that matrix, as the web's
// setTransform.
func (d *Drawing) SetTransform(a, b, c, dd, e, f float32) *Drawing {
	return d.op(DrawOp{Kind: DrawSetTransform, X1: a, Y1: b, X2: c, Y2: dd, X: e, Y: f})
}

// ResetTransform goes back to the canvas's own coordinates.
func (d *Drawing) ResetTransform() *Drawing { return d.SetTransform(1, 0, 0, 1, 0, 0) }

func (d *Drawing) BeginPath() *Drawing { return d.op(DrawOp{Kind: DrawBeginPath}) }

func (d *Drawing) MoveTo(x, y float32) *Drawing { return d.op(DrawOp{Kind: DrawMoveTo, X: x, Y: y}) }

func (d *Drawing) LineTo(x, y float32) *Drawing { return d.op(DrawOp{Kind: DrawLineTo, X: x, Y: y}) }

func (d *Drawing) ClosePath() *Drawing { return d.op(DrawOp{Kind: DrawClosePath}) }

// Fill fills the current path by the nonzero rule.
func (d *Drawing) Fill() *Drawing { return d.op(DrawOp{Kind: DrawFill}) }

// FillEvenOdd fills the current path by the even-odd rule.
func (d *Drawing) FillEvenOdd() *Drawing { return d.op(DrawOp{Kind: DrawFill, EvenOdd: true}) }

func (d *Drawing) Stroke() *Drawing { return d.op(DrawOp{Kind: DrawStroke}) }

// Clip clips later drawing to the current path, by the nonzero rule.
func (d *Drawing) Clip() *Drawing { return d.op(DrawOp{Kind: DrawClip}) }

// ClipEvenOdd clips by the even-odd rule.
func (d *Drawing) ClipEvenOdd() *Drawing { return d.op(DrawOp{Kind: DrawClip, EvenOdd: true}) }

// QuadTo adds a quadratic curve through control point (cx, cy).
func (d *Drawing) QuadTo(cx, cy, x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawQuadTo, X1: cx, Y1: cy, X: x, Y: y})
}

// CubicTo adds a cubic curve through control points 1 and 2.
func (d *Drawing) CubicTo(x1, y1, x2, y2, x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawCubicTo, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
}

// Arc adds an arc of radius r around (x, y) from angle a0 to a1, joined
// to the path by a line.
func (d *Drawing) Arc(x, y, r, a0, a1 float32, ccw bool) *Drawing {
	return d.op(DrawOp{Kind: DrawArc, X: x, Y: y, R: r, A0: a0, A1: a1, CCW: ccw})
}

// ArcTo adds an arc of radius r touching the line from the current point
// to (x1, y1) and the line from there to (x2, y2), joined to the path by
// a line, as the web's arcTo.
func (d *Drawing) ArcTo(x1, y1, x2, y2, r float32) *Drawing {
	t := d.tracked()
	if !t.has {
		return d.MoveTo(x1, y1)
	}
	inv, ok := t.st.m.invert()
	if !ok {
		return d
	}
	p0 := inv.apply(t.cur)
	d.restorePath()
	d.Ops = appendArcTo(d.Ops, p0, x1, y1, x2, y2, r)
	return d
}

// EllipseArc adds an arc of the ellipse around (x, y) with radii rx and
// ry turned by rotation, from angle a0 to a1, joined to the path by a
// line, as the web's ellipse. Its curves are cubic Béziers.
func (d *Drawing) EllipseArc(x, y, rx, ry, rotation, a0, a1 float32, ccw bool) *Drawing {
	has := d.tracked().has
	d.restorePath()
	d.Ops = appendEllipseArc(d.Ops, has, x, y, rx, ry, rotation, a0, a1, ccw)
	return d
}

func (d *Drawing) Rect(x, y, w, h float32) *Drawing {
	return d.op(DrawOp{Kind: DrawRect, X: x, Y: y, W: w, H: h})
}

// RoundRect adds a rectangle with corners of radius r.
func (d *Drawing) RoundRect(x, y, w, h, r float32) *Drawing {
	return d.op(DrawOp{Kind: DrawRoundRect, X: x, Y: y, W: w, H: h, R: r})
}

// RoundRectCorners adds a rectangle with a radius for each corner, from
// the top left one clockwise, scaled down together where they overlap,
// as the web's roundRect.
func (d *Drawing) RoundRectCorners(x, y, w, h, topLeft, topRight, bottomRight, bottomLeft float32) *Drawing {
	d.restorePath()
	d.Ops = appendRoundRect(d.Ops, x, y, w, h, [4]float32{topLeft, topRight, bottomRight, bottomLeft})
	return d
}

// Ellipse adds the whole ellipse around (x, y) with radii rx and ry, as a
// subpath of its own.
func (d *Drawing) Ellipse(x, y, rx, ry float32) *Drawing {
	return d.op(DrawOp{Kind: DrawEllipse, X: x, Y: y, R: rx, RY: ry})
}

// FillPath fills p by rule, leaving the current path as it is.
func (d *Drawing) FillPath(p *Path, rule FillRule) *Drawing {
	return d.withPath(p, DrawOp{Kind: DrawFill, EvenOdd: rule == EvenOdd})
}

// StrokePath strokes p, leaving the current path as it is.
func (d *Drawing) StrokePath(p *Path) *Drawing { return d.withPath(p, DrawOp{Kind: DrawStroke}) }

// ClipPath clips later drawing to p by rule, leaving the current path as
// it is.
func (d *Drawing) ClipPath(p *Path, rule FillRule) *Drawing {
	return d.withPath(p, DrawOp{Kind: DrawClip, EvenOdd: rule == EvenOdd})
}

// withPath runs use on p as the path. The current path stays the track's
// and leaves the recording until an op builds on it or uses it
// (restorePath), so a run of FillRects after a long line records each
// rectangle alone.
func (d *Drawing) withPath(p *Path, use DrawOp) *Drawing {
	t := d.tracked()
	d.Ops = append(d.Ops, DrawOp{Kind: DrawBeginPath})
	if p != nil {
		d.Ops = append(d.Ops, p.ops...)
	}
	d.Ops = append(d.Ops, use, DrawOp{Kind: DrawBeginPath})
	// The track skips them: a path's ops change neither the transform
	// nor the current path.
	t.at = len(d.Ops)
	d.pathAside = len(t.pathOps) > 0
	return d
}

// restorePath records the current path a FillPath left aside again
// where it was built, under the transforms it was built in.
func (d *Drawing) restorePath() {
	if !d.pathAside {
		return
	}
	d.pathAside = false
	t := d.tracked()
	saved := t.pathOps
	// What follows builds the path again, in the track too.
	t.segs, t.pathOps, t.has = t.segs[:0], nil, false
	d.Save()
	m := t.st.m
	for _, po := range saved {
		if po.m != m {
			m = po.m
			d.op(DrawOp{Kind: DrawSetTransform, X1: float32(m.a), Y1: float32(m.b), X2: float32(m.c),
				Y2: float32(m.d), X: float32(m.e), Y: float32(m.f)})
		}
		at := len(d.Ops)
		d.op(d.Ops[po.op])
		for _, s := range d.Slots {
			if s.Op == po.op {
				s.Op = at
				d.Slots = append(d.Slots, s)
			}
		}
	}
	d.Restore()
}

func (d *Drawing) SetFillColor(c Color) *Drawing { return d.op(DrawOp{Kind: DrawFillColor, Color: c}) }

func (d *Drawing) SetStrokeColor(c Color) *Drawing {
	return d.op(DrawOp{Kind: DrawStrokeColor, Color: c})
}

// FillFromView fills with the canvas view's Fill style, inherited as CSS
// inherits it, or with initial where no view sets one.
func (d *Drawing) FillFromView(initial Paint) *Drawing {
	d.ViewPaints = append(d.ViewPaints, ViewPaint{Op: len(d.Ops), Kind: ViewFill, Initial: initial})
	return d.SetFillColor(initial.Color(0))
}

// StrokeFromView strokes with the canvas view's Stroke style, or with
// initial where no view sets one.
func (d *Drawing) StrokeFromView(initial Paint) *Drawing {
	d.ViewPaints = append(d.ViewPaints, ViewPaint{Op: len(d.Ops), Kind: ViewStroke, Initial: initial})
	return d.SetStrokeColor(initial.Color(0))
}

// LineWidthFromView sets the line width to the canvas view's
// StrokeWidth style, 1 where no view sets one.
func (d *Drawing) LineWidthFromView() *Drawing {
	d.ViewPaints = append(d.ViewPaints, ViewPaint{Op: len(d.Ops), Kind: ViewStrokeWidth})
	return d.SetLineWidth(1)
}

// LineDashFromView dashes lines as the canvas view's StrokeDash style
// says, solid where no view sets one.
func (d *Drawing) LineDashFromView() *Drawing {
	d.ViewPaints = append(d.ViewPaints, ViewPaint{Op: len(d.Ops), Kind: ViewStrokeDash})
	return d.SetLineDash()
}

// FillBackgroundFromView fills the current path with the canvas view's
// background, Background and Gradients, placed over area as over a box of
// that size, as CSS's clip-path shows a box's background. The view then
// draws no background of its own, so a canvas that takes it draws it once,
// inside its shapes; one with nothing to fill yet records this over an
// empty path, so its view never shows the background as a box.
func (d *Drawing) FillBackgroundFromView(area Bounds) *Drawing {
	d.Save().SetFillColor(0)
	d.ViewPaints = append(d.ViewPaints, ViewPaint{Op: len(d.Ops), Kind: ViewBackground, Area: area})
	return d.Fill().Restore()
}

// SetFillGradient fills with g until the next SetFillColor.
func (d *Drawing) SetFillGradient(g *CanvasGradient) *Drawing {
	d.SetFillColor(g.first())
	return d.op(DrawOp{Kind: DrawFillGradient, Gradient: g})
}

// SetStrokeGradient strokes with g until the next SetStrokeColor.
func (d *Drawing) SetStrokeGradient(g *CanvasGradient) *Drawing {
	d.SetStrokeColor(g.first())
	return d.op(DrawOp{Kind: DrawStrokeGradient, Gradient: g})
}

func (g *CanvasGradient) first() Color {
	if g == nil || len(g.Stops) == 0 {
		return 0
	}
	return g.Stops[0].Color
}

func (d *Drawing) SetLineWidth(w float32) *Drawing { return d.op(DrawOp{Kind: DrawLineWidth, W: w}) }

func (d *Drawing) SetLineCap(c LineCap) *Drawing { return d.op(DrawOp{Kind: DrawLineCap, Cap: c}) }

func (d *Drawing) SetLineJoin(j LineJoin) *Drawing { return d.op(DrawOp{Kind: DrawLineJoin, Join: j}) }

// SetMiterLimit sets how far miter joins reach, in line widths; 10 by
// default.
func (d *Drawing) SetMiterLimit(limit float32) *Drawing {
	return d.op(DrawOp{Kind: DrawMiterLimit, W: limit})
}

// SetLineDash dashes strokes with lengths of dash and gap in turn, an odd
// list repeated to make it even; none is solid.
func (d *Drawing) SetLineDash(lengths ...float32) *Drawing {
	for _, l := range lengths {
		if l < 0 || l != l || math.IsInf(float64(l), 0) {
			return d
		}
	}
	dash := LineDash(slices.Clone(lengths))
	if len(dash)%2 == 1 {
		dash = append(dash, dash...)
	}
	return d.op(DrawOp{Kind: DrawLineDash, Dash: &dash})
}

// SetLineDashOffset starts the dashes offset into their pattern.
func (d *Drawing) SetLineDashOffset(offset float32) *Drawing {
	return d.op(DrawOp{Kind: DrawLineDashOffset, W: offset})
}

// SetAlpha multiplies everything drawn after it by alpha.
func (d *Drawing) SetAlpha(alpha float32) *Drawing { return d.op(DrawOp{Kind: DrawAlpha, A: alpha}) }

// SetComposite sets how later drawing mixes with the canvas.
func (d *Drawing) SetComposite(c CompositeOp) *Drawing {
	return d.op(DrawOp{Kind: DrawComposite, Composite: c})
}

// SetShadow shadows later drawing in color, offset by dx, dy and blurred
// by blur (twice the Gaussian's deviation), in the canvas's own points
// whatever the transform. A transparent color draws none.
func (d *Drawing) SetShadow(color Color, dx, dy, blur float32) *Drawing {
	return d.op(DrawOp{Kind: DrawShadow, Color: color, X: dx, Y: dy, R: max(blur, 0)})
}

// SetTextAlign sets where later texts sit along the point they are drawn
// at: start and left put it at their left end, end and right at their
// right end, center at their middle.
func (d *Drawing) SetTextAlign(a TextAlign) *Drawing {
	return d.op(DrawOp{Kind: DrawTextAlign, TextAlign: a})
}

// SetTextBaseline sets which line of later texts the point they are
// drawn at sits on.
func (d *Drawing) SetTextBaseline(b TextBaseline) *Drawing {
	return d.op(DrawOp{Kind: DrawTextBaseline, TextBaseline: b})
}

// FillText draws text at (x, y), placed by the text alignment and
// baseline: its baseline's start, by default.
func (d *Drawing) FillText(text string, x, y float32, font Font) *Drawing {
	return d.drawText(DrawFillText, text, x, y, font)
}

// StrokeText draws text's outline with the stroke settings, placed as
// FillText places it.
func (d *Drawing) StrokeText(text string, x, y float32, font Font) *Drawing {
	return d.drawText(DrawStrokeText, text, x, y, font)
}

func (d *Drawing) drawText(kind DrawKind, text string, x, y float32, font Font) *Drawing {
	font = drawFont(font)
	op := DrawOp{Kind: kind, Text: text, X: x, Y: y, Font: font}
	t := d.tracked()
	op.TextAlign, op.TextBaseline = t.st.align, t.st.baseline
	if op.TextAlign == TextAlignCenter || op.TextAlign == TextAlignEnd || op.TextBaseline != BaselineAlphabetic {
		m := d.TextMetrics(text, font)
		switch op.TextAlign {
		case TextAlignCenter:
			op.X1 = -m.Width / 2
		case TextAlignEnd:
			op.X1 = -m.Width
		}
		switch op.TextBaseline {
		case BaselineTop:
			op.Y1 = m.Ascent
		case BaselineHanging:
			op.Y1 = m.Ascent * 0.8
		case BaselineMiddle:
			op.Y1 = (m.Ascent - m.Descent) / 2
		case BaselineIdeographic, BaselineBottom:
			op.Y1 = -m.Descent
		}
	}
	return d.op(op)
}

func drawFont(font Font) Font {
	if font.Size == 0 {
		font.Size = DefaultFontSize
	}
	if font.Weight == 0 {
		font.Weight = FontWeightNormal
	}
	font.Variant &^= FontVariantNormal
	if len(font.Axes) > 0 {
		font.Axes = maps.Clone(font.Axes)
	} else {
		font.Axes = nil
	}
	if len(font.Features) > 0 {
		font.Features = maps.Clone(font.Features)
	} else {
		font.Features = nil
	}
	return font
}

// textKey is a text measured in a font.
type textKey struct {
	text string
	font FontKey
}

// MeasureText returns the advance width of text in font, its trailing
// white space included, as a browser's measureText counts it.
func (d *Drawing) MeasureText(text string, font Font) float32 {
	return d.TextMetrics(text, font).Width
}

// TextMetrics measures text in font with the platform's text engine, as
// its first line lays it out; without one it approximates: every
// character 0.6 of the font size wide, an ascent of 0.8 and a descent of
// 0.2 of it.
func (d *Drawing) TextMetrics(text string, font Font) TextMetrics {
	font = drawFont(font)
	key := textKey{text, FontKeyOf(font)}
	if m, ok := d.measured[key]; ok {
		return m
	}
	size := float32(font.Size)
	m := TextMetrics{Width: 0.6 * size * float32(len([]rune(text))), Ascent: 0.8 * size, Descent: 0.2 * size,
		LineHeight: 1.2 * size}
	if d.text != nil {
		// An empty line still has the font's extent.
		line := d.firstLine(cmp.Or(text, " "), font)
		m.Ascent, m.Descent, m.LineHeight = float32(line.Ascender), float32(line.Descender),
			float32(line.Bounds.Size.Height)
		m.Width = d.advanceLine(text, font, line)
	}
	if d.measured == nil {
		d.measured = map[textKey]TextMetrics{}
	}
	d.measured[key] = m
	return m
}

// widthMark closes a text ending in white space while it is measured.
const widthMark = "x"

// advanceLine returns how far text advances on its first line in font, its
// trailing white space included, taking the width of line, which firstLine
// laid out for the same text. Measuring the text again to read its width
// would lay it out a second time, which on Windows builds another DirectWrite
// layout for every measurement.
//
// Engines lay a line out without the white space at its end, which hangs past
// the line as in CSS, so a text ending in it is measured with a mark after it,
// less the mark.
func (d *Drawing) advanceLine(text string, font Font, line TextLine) float32 {
	if text == "" {
		return 0
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	if !unicode.IsSpace(last) || lineBreak(last) {
		return float32(line.Bounds.Size.Width)
	}
	w := float32(d.firstLine(text+widthMark, font).Bounds.Size.Width) - d.markWidth(font)
	return max(w, 0)
}

// markWidth returns how wide widthMark is in font. Every text ending in white
// space needs it, and it does not change while the drawing lives, so it is
// measured once per font rather than once per such text.
func (d *Drawing) markWidth(font Font) float32 {
	fk := FontKeyOf(font)
	if w, ok := d.marks[fk]; ok {
		return w
	}
	w := float32(d.firstLine(widthMark, font).Bounds.Size.Width)
	if d.marks == nil {
		d.marks = map[FontKey]float32{}
	}
	d.marks[fk] = w
	return w
}

// lineBreak says whether r is one of Unicode's mandatory line breaks.
func lineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// firstLine lays text out unwrapped in font and returns its first line;
// an engine that reports no lines gives its measured size, the em box's
// split of the font size and no width past the box.
func (d *Drawing) firstLine(text string, font Font) TextLine {
	p := d.text.Prepare(&AttributedText{Text: text, Style: TextStyle{Font: font, Color: RGB(0x000000),
		Align: TextAlignStart}})
	defer p.Release()
	fit := p.Measure(Auto)
	l := p.Layout(fit)
	defer l.Release()
	if ll, ok := l.(interface{ TextLines() []TextLine }); ok {
		if lines := ll.TextLines(); len(lines) > 0 {
			return lines[0]
		}
	}
	size := float32(font.Size)
	return TextLine{Bounds: Bounds{Size: fit}, Ascender: Points(0.8 * size), Descender: Points(0.2 * size)}
}

// FillRect fills a rectangle with the fill paint, leaving the current
// path as it is.
func (d *Drawing) FillRect(x, y, w, h float32) *Drawing {
	return d.FillPath(new(Path).Rect(x, y, w, h), NonZero)
}

// StrokeRect strokes a rectangle, leaving the current path as it is.
func (d *Drawing) StrokeRect(x, y, w, h float32) *Drawing {
	return d.StrokePath(new(Path).Rect(x, y, w, h))
}

// Clear makes a rectangle transparent.
func (d *Drawing) Clear(x, y, w, h float32) *Drawing {
	return d.op(DrawOp{Kind: DrawClear, X: x, Y: y, W: w, H: h})
}

// DrawImage draws img at (x, y) at its natural size.
func (d *Drawing) DrawImage(img ImageSource, x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawImage, ImageIndex: d.addImage(img), X: x, Y: y})
}

// DrawImageIn draws img stretched into the rectangle x, y, w, h.
func (d *Drawing) DrawImageIn(img ImageSource, x, y, w, h float32) *Drawing {
	if w == 0 && h == 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawImage, ImageIndex: d.addImage(img), X: x, Y: y, W: w, H: h})
}

// DrawImageFrom draws the part sx, sy, sw, sh of img, in its own points,
// stretched into the rectangle x, y, w, h.
func (d *Drawing) DrawImageFrom(img ImageSource, sx, sy, sw, sh, x, y, w, h float32) *Drawing {
	if w == 0 && h == 0 || sw == 0 && sh == 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawImage, ImageIndex: d.addImage(img), X1: sx, Y1: sy, X2: sw, Y2: sh, X: x, Y: y, W: w, H: h})
}

// DrawSVG draws svg at (x, y) at its intrinsic size, respecting its
// preserveAspectRatio and leaving the drawing's state and current path
// unchanged. Every currentColor takes the drawing's current fill color (or
// black when none is set), and <text> elements are measured and drawn by the
// platform's text engine.
func (d *Drawing) DrawSVG(svg *SVG, x, y float32) *Drawing {
	if svg == nil {
		return d
	}
	w, h := svg.Size()
	return d.DrawSVGIn(svg, x, y, float32(w), float32(h))
}

// DrawSVGIn draws svg fitted into the rectangle (x, y, w, h), respecting its
// preserveAspectRatio and leaving the drawing's state and current path
// unchanged. Every currentColor takes the drawing's current fill color (or
// black when none is set), and <text> elements are measured and drawn by the
// platform's text engine.
func (d *Drawing) DrawSVGIn(svg *SVG, x, y, w, h float32) *Drawing {
	if svg == nil || w <= 0 || h <= 0 {
		return d
	}
	t := d.tracked()
	hasPath := len(t.pathOps) > 0
	color := t.st.fill
	svg.drawAt(d, x, y, w, h, color)
	if len(svg.ops) > 0 || svg.aspectSlice {
		d.Ops = append(d.Ops, DrawOp{Kind: DrawBeginPath})
	}
	if hasPath {
		d.pathAside = true
	}
	// The SVG drew inside Save/Restore and cleared its own path, so neither
	// the tracked state nor any saved current path changed.
	t.at = len(d.Ops)
	return d
}

// DrawTintedImage draws img at its natural size, multiplying each image
// channel by tint's corresponding sRGB or opacity channel before compositing.
func (d *Drawing) DrawTintedImage(img ImageSource, tint Color, x, y float32) *Drawing {
	return d.op(DrawOp{Kind: DrawImage, ImageIndex: d.addImage(img), Tinted: true, Color: tint, X: x, Y: y})
}

// DrawTintedImageIn draws img into a rectangle with the same tint as DrawTintedImage.
func (d *Drawing) DrawTintedImageIn(img ImageSource, tint Color, x, y, w, h float32) *Drawing {
	if w == 0 && h == 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawImage, ImageIndex: d.addImage(img), Tinted: true, Color: tint, X: x, Y: y, W: w, H: h})
}

// DrawTintedImageFrom draws a part of img with the same tint as DrawTintedImage.
func (d *Drawing) DrawTintedImageFrom(img ImageSource, tint Color, sx, sy, sw, sh, x, y, w, h float32) *Drawing {
	if w == 0 && h == 0 || sw == 0 && sh == 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawImage, ImageIndex: d.addImage(img), Tinted: true, Color: tint,
		X1: sx, Y1: sy, X2: sw, Y2: sh, X: x, Y: y, W: w, H: h})
}

// Bind has the platform play the last operation's argument arg with the
// value of node, every frame it changes, in place of the one recorded.
// The queries and the text placement read the recorded value.
func (d *Drawing) Bind(arg DrawArg, node AnimatedArg) *Drawing {
	if node == nil || len(d.Ops) == 0 || d.Ops[len(d.Ops)-1].Kind == DrawVertices {
		return d
	}
	if id := node.AnimatedNodeID(); id != 0 {
		d.Slots = append(d.Slots, DrawSlot{Op: len(d.Ops) - 1, Arg: arg, Node: id})
	}
	return d
}

// Node names a part of the drawing for assistive technology and tests.
func (d *Drawing) Node(n CanvasNode) *Drawing {
	d.Nodes = append(d.Nodes, n)
	return d
}

// Paint is what SVG's paint properties, Fill and Stroke, paint with: a
// color, nothing or the text color. The zero Paint is unset, so the
// property inherits.
type Paint struct {
	kind  viewPaintKind
	color Color
}

type viewPaintKind uint8

const (
	unsetPaint viewPaintKind = iota
	noPaint
	currentPaint
	colorPaint
)

var (
	// PaintNone paints nothing.
	PaintNone = Paint{kind: noPaint}
	// PaintCurrentColor paints with the text color, CSS's currentColor.
	PaintCurrentColor = Paint{kind: currentPaint}
)

// PaintColor paints with c.
func PaintColor(c Color) Paint { return Paint{kind: colorPaint, color: c} }

// IsSet reports whether p is set.
func (p Paint) IsSet() bool { return p.kind != unsetPaint }

// Color returns the color p paints with, current being the text color;
// nothing is transparent.
func (p Paint) Color(current Color) Color {
	switch p.kind {
	case currentPaint:
		return current
	case colorPaint:
		return p.color
	}
	return 0
}

// PaintStyle is the SVG paint properties a canvas view inherits.
type PaintStyle struct {
	Fill, Stroke Paint
	// StrokeWidth is the line width; 0 is unset, CSS's initial 1.
	StrokeWidth Points
	// StrokeDash is the dashes and gaps in turn, repeated when odd; nil is
	// unset and empty solid, as are lengths adding up to 0.
	StrokeDash []Points
	// Background and Gradients are the view's own background, which
	// FillBackgroundFromView fills with.
	Background Color
	Gradients  []Gradient
}

// ViewPaintKind is the paint property a ViewPaint takes.
type ViewPaintKind uint8

const (
	ViewFill ViewPaintKind = iota
	ViewStroke
	ViewStrokeWidth
	ViewStrokeDash
	// ViewBackground fills with the view's background: a fill the core
	// expands into one for its color and one for each gradient.
	ViewBackground
)

// ViewPaint is an op of a drawing whose value is its view's paint.
type ViewPaint struct {
	Op   int
	Kind ViewPaintKind
	// Initial is what a fill or stroke paints with where no view sets
	// one.
	Initial Paint
	// Area is the box a background is placed over.
	Area Bounds
}

// HasBackground reports whether d fills shapes with its view's
// background, which the view then does not draw.
func (d *Drawing) HasBackground() bool {
	for _, v := range d.ViewPaints {
		if v.Kind == ViewBackground {
			return true
		}
	}
	return false
}

// Painted returns d with its view paints set from p, current being the
// view's text color, or d itself when it takes none.
func (d *Drawing) Painted(p *PaintStyle, current Color) *Drawing {
	if len(d.ViewPaints) == 0 {
		return d
	}
	out := &Drawing{Ops: slices.Clone(d.Ops), Images: d.Images, Sprites: d.Sprites,
		Slots: d.Slots, Nodes: d.Nodes, ViewPaints: d.ViewPaints,
		Resolution: d.Resolution, Upscale: d.Upscale, text: d.text, shaded: d.shaded, lease: d.lease}
	for _, v := range d.ViewPaints {
		if v.Op >= len(out.Ops) {
			continue
		}
		op := &out.Ops[v.Op]
		switch v.Kind {
		case ViewFill:
			paint := v.Initial
			if p.Fill.IsSet() {
				paint = p.Fill
			}
			op.Color = paint.Color(current)
		case ViewStroke:
			paint := v.Initial
			if p.Stroke.IsSet() {
				paint = p.Stroke
			}
			op.Color = paint.Color(current)
		case ViewStrokeWidth:
			op.W = 1
			if p.StrokeWidth != 0 {
				op.W = float32(p.StrokeWidth)
			}
		case ViewStrokeDash:
			op.Dash = strokeDash(p.StrokeDash)
		}
	}
	if d.HasBackground() {
		out.Ops, out.Slots = backgroundFills(out.Ops, d.Slots, d.ViewPaints, p)
	}
	return out
}

// backgroundFills replaces each background fill of ops with fills of p's
// background, its color first and the gradients over it, the first on
// top, and moves slots past them along.
func backgroundFills(ops []DrawOp, slots []DrawSlot, paints []ViewPaint, p *PaintStyle) ([]DrawOp, []DrawSlot) {
	out := make([]DrawOp, 0, len(ops)+2*len(p.Gradients))
	areas := map[int]Bounds{}
	for _, v := range paints {
		if v.Kind == ViewBackground {
			areas[v.Op] = v.Area
		}
	}
	// at[i] is where op i went.
	at := make([]int, len(ops))
	for i, op := range ops {
		at[i] = len(out)
		area, ok := areas[i]
		if !ok {
			out = append(out, op)
			continue
		}
		if p.Background&0xff != 0 {
			out = append(out, DrawOp{Kind: DrawFillColor, Color: p.Background}, op)
		}
		for g := len(p.Gradients) - 1; g >= 0; g-- {
			if cg, ok := backgroundGradient(&p.Gradients[g], area); ok {
				out = append(out, DrawOp{Kind: DrawFillColor, Color: cg.first()}, DrawOp{Kind: DrawFillGradient, Gradient: cg}, op)
			}
		}
	}
	moved := make([]DrawSlot, len(slots))
	for i, s := range slots {
		s.Op = at[s.Op]
		moved[i] = s
	}
	return out, moved
}

// backgroundGradient is g placed over area, as a view of area's size
// draws it.
func backgroundGradient(g *Gradient, area Bounds) (*CanvasGradient, bool) {
	gp, ok := g.Paint(area.Size)
	if !ok {
		return nil, false
	}
	x, y := float32(area.Origin.X), float32(area.Origin.Y)
	w, h := float32(area.Size.Width), float32(area.Size.Height)
	switch gp.Kind {
	case GradientRadial:
		// A unit circle stretched to the ellipse's radii.
		cg := CanvasRadial(0, 0, 0, 0, 0, 1, gp.Stops...)
		cg.Transform = [6]float32{max(gp.RX*w, 1e-3), 0, 0, max(gp.RY*h, 1e-3), x + gp.CX*w, y + gp.CY*h}
		return cg, true
	case GradientConic:
		angle := (gp.Angle - 90) * math.Pi / 180
		return CanvasConic(angle, x+gp.CX*w, y+gp.CY*h, gp.Stops...), true
	}
	return CanvasLinear(x+gp.X0*w, y+gp.Y0*h, x+gp.X1*w, y+gp.Y1*h, gp.Stops...), true
}

// strokeDash is the line dash of dashes, solid for none, a negative
// length or lengths adding up to 0.
func strokeDash(dashes []Points) *LineDash {
	var sum Points
	for _, l := range dashes {
		if l < 0 || l != l {
			sum = 0
			break
		}
		sum += l
	}
	dash := LineDash{}
	if sum > 0 {
		for _, l := range dashes {
			dash = append(dash, float32(l))
		}
		if len(dash)%2 == 1 {
			dash = append(dash, dash...)
		}
	}
	return &dash
}
