package core

import (
	"image"
	"reflect"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Canvas draws what Draw records, at the canvas's size, again whenever a signal Draw read, the size or the available fonts change. The platform plays the recording on the main thread; Draw itself runs on the engine.
//
// The Drawing measures text with the platform's text engine
// (MeasureText, TextMetrics) and answers hit tests (IsPointInPath,
// IsPointInStroke) during Draw and after it, so OnPress can test the
// shapes of the last drawing Draw was given. Arguments bound to animated
// nodes (Drawing.Bind) redraw on the platform's frame clock without a
// commit, and Drawing.Node names parts of the drawing for assistive
// technology and tests.
//
// A drawing that fills shapes with its view's background
// (Drawing.FillBackgroundFromView) takes the canvas's Background and
// Gradients, which the view then does not draw.
type Canvas struct {
	Draw func(d *Drawing, size Size)
	// Class is Tailwind CSS classes, as View's.
	Class     string
	ClassFunc func() string
	Style     Style
	StyleFunc func() Style
	OnPress   func(PressEvent)
	OnLayout  func(Bounds)
	Ref       *Ref
	// Label names the drawing for assistive technology, which reads it as an image; LabelFunc follows what it reads, as Text's does.
	Label     string
	LabelFunc func() string
	TestID    string
	// Em sizes the canvas where its style sets no size, in ems of the font size it inherits, scaled by the user's text size setting as text is: Em.Width wide and Em.Ascent+Em.Descent high, so it grows with the text around it as an icon does. In a text's attachment aligned to the baseline (VerticalAlignBaseline), its baseline, Em.Descent ems above its bottom, sits on the line's, and the text's NoFontScaling and MaxFontSizeMultiplier hold it too. AdjustsFontSizeToFit shrinks its font-relative size with the text. EmFunc follows what it reads, laying the canvas out again; base/formula sizes its formulas so.
	Em     EmBox
	EmFunc func() EmBox
	// MeasureEm measures a width-aware em canvas, such as base/formula's display math, at width in points (Auto when unconstrained) and the resolved, scaled font size. It returns its box in ems without drawing. Em or EmFunc may supply its preferred box. Reads are tracked; the callback also runs during layout, so it must not change signals.
	MeasureEm func(width, fontSize Points) EmBox
	// ResolutionScale multiplies the pixels per point the canvas is drawn at, the screen's density times any zoom or transform around it: 0.5 draws half the pixels per side and stretches them to the canvas's box, so a heavy drawing such as a game's scene has fewer to fill. Zero and values outside (0, 1) draw at the screen's density. ResolutionScaleFunc follows what it reads, playing the last drawing again at the new scale without running Draw. Hit tests, OnPress and the size Draw gets stay in points.
	ResolutionScale     float32
	ResolutionScaleFunc func() float32
	// Sampling is how a canvas drawn below the screen's density (ResolutionScale) is stretched to its box: smoothly by default, or to the nearest pixel for pixel art. SamplingFunc follows what it reads, as ResolutionScaleFunc does.
	Sampling     ImageSampling
	SamplingFunc func() ImageSampling
	// CopyItem supplies extra clipboard representations when a text selection copies this canvas as a single attachment; CopyItemFunc follows what it reads. base/formula provides Presentation MathML and source representations through it.
	CopyItem     *ClipboardItem
	CopyItemFunc func() *ClipboardItem
}

// canvasResolution is the ResolutionScale a canvas sends: r within (0, 1), or 0 for any other value, which draws at the screen's density.
func canvasResolution(r float32) float32 {
	if r > 0 && r < 1 {
		return r
	}
	return 0
}

// EmBox is a canvas's natural size in ems of its font size (Canvas.Em): Width wide, reaching Ascent above its baseline and Descent below it.
type EmBox struct {
	Width, Ascent, Descent float32
}

// sized reports whether e sizes a canvas.
func (e EmBox) sized() bool { return e.Width > 0 || e.Ascent+e.Descent > 0 }

// emNode is what an Em canvas is sized by.
type emNode struct {
	box EmBox
	// fontSize is the scaled font size it inherits, in points.
	fontSize Points
	measure  func(width, fontSize Points) EmBox
	// fitFontSize is a containing fitted text's effective size; zero keeps fontSize.
	fitFontSize Points
}

func (e *emNode) boxAt(width, fontSize Points) EmBox {
	if e.measure == nil {
		return e.box
	}
	if fontSize <= 0 {
		fontSize = DefaultFontSize
	}
	return e.measure(width, fontSize)
}

// naturalSize is what an Em canvas measures without a style size.
func (e *emNode) naturalSize() Size {
	fs := e.fontSize
	if fs <= 0 {
		fs = DefaultFontSize
	}
	return Size{Width: Points(max(e.box.Width, 0)) * fs, Height: Points(max(e.box.Ascent, 0)+max(e.box.Descent, 0)) * fs}
}

// descentAt is the measured box's share below its first baseline.
func (e *emNode) descentAt(size Size) Points {
	box := e.boxAt(size.Width, e.fontSize)
	asc, desc := max(box.Ascent, 0), max(box.Descent, 0)
	if asc+desc <= 0 {
		return 0
	}
	return size.Height * Points(desc/(asc+desc))
}

// Render implements Node.
func (c Canvas) Render(*Scope) Node { return c }

func (c Canvas) mountIn(t *Tree, cx *Scope, parent *node) { t.mountCanvas(cx, parent, c) }

type canvasNode struct {
	draw    func(*Drawing, Size)
	effect  *computation
	size    *Signal[Size]
	drawing *Drawing
	// pool holds reusable drawings for this canvas so steady-state redraws
	// keep Ops, Images and Sprites capacities without overwriting a drawing
	// still retained by a platform.
	pool  []*Drawing
	fresh bool
	// paint and color are the paint properties and text color the
	// canvas inherits, which its drawing's view paints take.
	paint driver.PaintStyle
	color Color
	// paintKey is what paint was resolved from.
	paintKey canvasPaintKey
	// em sizes a canvas with Em or EmFunc.
	em *emNode
	// measureFont invalidates a width-aware recording even when fixed dimensions keep its frame unchanged after a font-size change.
	measureFont *Signal[Points]
	measured    EmBox
	// resolution and sampling are the canvas's ResolutionScale, as
	// canvasResolution keeps it, and Sampling, which its drawing carries.
	resolution float32
	sampling   ImageSampling
	// frames are the copies of the pixels the drawing last sent draws, by the app's buffer and version.
	frames map[pixelKey]*image.RGBA
	// copyItem is the extra clipboard representations supplied when a text selection copies this canvas as a single attachment.
	copyItem *driver.ClipboardItem
}

type pixelKey struct {
	from    *image.RGBA
	version uint64
}

const maxCanvasPool = 4

// nextDrawing returns an idle pooled drawing reset for the next frame, or
// creates a new pooled drawing when none is free (e.g. first two frames or
// while previous frames are still retained by a platform).
func (cn *canvasNode) nextDrawing(text driver.TextEngine) *Drawing {
	for _, d := range cn.pool {
		if d != cn.drawing && !d.InUse() {
			d.Reset()
			return d
		}
	}
	d := driver.NewPooledDrawing(text)
	if len(cn.pool) < maxCanvasPool {
		cn.pool = append(cn.pool, d)
	}
	return d
}

// scaled returns d carrying the canvas's resolution and sampling, in a
// copy where they differ, as a drawing handed to the platform never
// changes.
func (cn *canvasNode) scaled(d *Drawing) *Drawing {
	if d.Resolution == cn.resolution && d.Upscale == cn.sampling {
		return d
	}
	out := *d
	out.Resolution, out.Upscale = cn.resolution, cn.sampling
	return &out
}

// withFrames returns d with the pixels it draws copied, as an image view's are (pixelFrame): those the last drawing sent at the same version keep their copies, so the platform keeps its images.
func (cn *canvasNode) withFrames(d *Drawing) *Drawing {
	out, frames := copyDrawingFrames(d, cn.frames)
	cn.frames = frames
	return out
}

// canvasPaintKey is what a canvas's view paints are resolved from.
type canvasPaintKey struct {
	fill, stroke Paint
	width        Points
	dash         string
	color        Color
	background   Color
}

// resolvePaint takes the paint properties canvas node n inherits, and its
// own background, and has its drawing sent again when they change what it
// paints.
func (t *Tree) resolvePaint(n *node, inherited *textFields) {
	cn := n.canvas
	bg := n.style.Background
	if bg == CurrentColor {
		bg = n.color
	}
	k := canvasPaintKey{inherited.Fill, inherited.Stroke, inherited.StrokeWidth, inherited.StrokeDash, n.color, bg}
	if k == cn.paintKey && slices.EqualFunc(n.style.Gradients, cn.paint.Gradients, sameGradient) {
		return
	}
	cn.paintKey, cn.paint, cn.color = k, inherited.paint(), n.color
	cn.paint.Background, cn.paint.Gradients = bg, n.style.Gradients
	if cn.drawing != nil && len(cn.drawing.ViewPaints) > 0 {
		cn.fresh = true
	}
}

func sameGradient(a, b Gradient) bool { return reflect.DeepEqual(a, b) }

func (t *Tree) mountCanvas(cx *Scope, parent *node, c Canvas) {
	t.enableCanvas()
	n := t.newNode(driver.KindCanvas, parent, cx)
	t.bindClasses(cx, n, c.Class, c.ClassFunc)
	t.bindStyle(cx, n, c.Style, c.StyleFunc)
	t.setRef(n, c.Ref)
	n.h = handlers{press: c.OnPress, layout: c.OnLayout}
	n.props = driver.ViewProps{Role: RoleImage, Label: c.Label, TestID: c.TestID}
	t.bindAccessibility(cx, n, a11y{labelFunc: c.LabelFunc})
	cn := &canvasNode{draw: c.Draw, size: NewSignal(cx, Size{}),
		resolution: canvasResolution(c.ResolutionScale), sampling: c.Sampling,
		copyItem: driver.CloneClipboardItem(c.CopyItem)}
	n.canvas = cn
	if c.CopyItemFunc != nil {
		cx.renderEffect(func() {
			next := driver.CloneClipboardItem(c.CopyItemFunc())
			if !driver.ClipboardItemEqual(next, cn.copyItem) {
				cn.copyItem = next
				t.markAttachmentCopyChanged(n)
			}
		})
	}
	// A new scale or sampling plays the last drawing again.
	replay := func() {
		if cn.drawing != nil {
			cn.fresh = true
			t.touch(n)
		}
	}
	if c.ResolutionScaleFunc != nil {
		cx.renderEffect(func() {
			if r := canvasResolution(c.ResolutionScaleFunc()); r != cn.resolution {
				cn.resolution = r
				replay()
			}
		})
	}
	if c.SamplingFunc != nil {
		cx.renderEffect(func() {
			if s := c.SamplingFunc(); s != cn.sampling {
				cn.sampling = s
				replay()
			}
		})
	}
	t.canvases = append(t.canvases, n)
	if c.Em.sized() || c.EmFunc != nil || c.MeasureEm != nil {
		cn.em = &emNode{box: c.Em, measure: c.MeasureEm}
	}
	if c.MeasureEm != nil {
		cn.measureFont = NewSignal(cx, Points(0))
	}
	if c.EmFunc != nil {
		cx.renderEffect(func() {
			if box := c.EmFunc(); box != cn.em.box {
				cn.em.box = box
				t.relayout(n)
			}
		})
	}
	cn.effect = cx.renderEffect(func() {
		size := cn.size.Get()
		if cn.measureFont != nil {
			width := size.Width
			if width <= 0 {
				width = Auto
			}
			box := cn.em.boxAt(width, cn.measureFont.Get())
			if box != cn.measured {
				cn.measured = box
				t.relayout(n)
			}
		}
		if size == (Size{}) || cn.draw == nil {
			return
		}
		d := cn.nextDrawing(t.text)
		cn.draw(d, size)
		// A canvas that has drawn nothing has nothing to clear.
		if cn.drawing == nil && len(d.Ops) == 0 && len(d.Nodes) == 0 {
			return
		}
		t.pickDrawingScales(d)
		if t.rt.restricted {
			t.refuseDrawingImages(n, d)
		}
		if d.Shaded() {
			t.enableCanvasShaders()
		}
		d.Resolution, d.Upscale = cn.resolution, cn.sampling
		cn.drawing, cn.fresh = d, true
		t.touch(n)
	})
}

// pickDrawingScales has the images d draws name the variant for the
// screen, as Image's do.
func (t *Tree) pickDrawingScales(d *Drawing) {
	for i := range d.Images {
		if d.Images[i].Scales != 0 {
			d.Images[i] = t.pickScale(d.Images[i])
		}
	}
	if d.Shaded() {
		eachShaderImage(d, func(img *ImageSource) {
			if img.Scales != 0 {
				*img = t.pickScale(*img)
			}
		})
	}
}

// eachShaderImage calls fn with each image the shaders of d's ops read,
// in copies of the shaders that d's ops then name: ops sharing a shader
// (SetShader) share its copy.
func eachShaderImage(d *Drawing, fn func(*ImageSource)) {
	var copies map[*driver.Shader]*driver.Shader
	for i := range d.Ops {
		op := &d.Ops[i]
		from := driver.PaintShader(op.Shader)
		if from == nil || len(from.Images) == 0 {
			continue
		}
		c := copies[from]
		if c == nil {
			s := *from
			s.Images = slices.Clone(s.Images)
			for j := range s.Images {
				fn(&s.Images[j])
			}
			if copies == nil {
				copies = map[*driver.Shader]*driver.Shader{}
			}
			c = &s
			copies[from] = c
		}
		op.Shader = driver.NewShaderPaint(c)
	}
}

// refuseDrawingImages drops the images of d the Access of n's owner
// refuses.
func (t *Tree) refuseDrawingImages(n *node, d *Drawing) {
	t.refuseDrawingImagesOwner(n.owner, d)
}

func (t *Tree) refuseDrawingImagesOwner(owner *Scope, d *Drawing) {
	for i := range d.Images {
		t.refuseImageOwner(owner, &d.Images[i])
	}
	if d.Shaded() {
		eachShaderImage(d, func(img *ImageSource) { t.refuseImageOwner(owner, img) })
	}
}

// MeasureText returns the advance width and the metrics of text's first
// line in font, laid out by the platform's text engine as a canvas measures
// it (Drawing.TextMetrics): a column fitted to its widest text, a
// monospaced font's character width. In a reactive function it follows changes
// to the available fonts.
func (cx *Scope) MeasureText(text string, font Font) TextMetrics {
	var engine driver.TextEngine
	if t := cx.tree(); t != nil {
		engine = t.text
		if t.fontsRead == nil {
			t.fontsRead = NewSignal(t.root, t.fonts)
		}
		t.fontsRead.Get()
	}
	return driver.NewDrawing(engine).TextMetrics(text, font)
}
