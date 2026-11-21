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
	// Em sizes the canvas where its style sets no size, in ems of the font size it inherits, scaled by the user's text size setting as text is: Em.Width wide and Em.Ascent+Em.Descent high, so it grows with the text around it as an icon does. In a text's attachment aligned to the baseline (VerticalAlignBaseline), its baseline, Em.Descent ems above its bottom, sits on the line's, and the text's NoFontScaling and MaxFontSizeMultiplier hold it too. EmFunc follows what it reads, laying the canvas out again; base/formula sizes its formulas so.
	Em     EmBox
	EmFunc func() EmBox
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
}

// naturalSize is what an Em canvas measures without a style size.
func (e *emNode) naturalSize() Size {
	fs := e.fontSize
	if fs <= 0 {
		fs = DefaultFontSize
	}
	return Size{Width: Points(max(e.box.Width, 0)) * fs, Height: Points(max(e.box.Ascent, 0)+max(e.box.Descent, 0)) * fs}
}

// descent is how far below its baseline the bottom of an Em canvas height high reaches: Em.Descent's share of it.
func (e *emNode) descent(height Points) Points {
	asc, desc := max(e.box.Ascent, 0), max(e.box.Descent, 0)
	if asc+desc <= 0 {
		return 0
	}
	return height * Points(desc/(asc+desc))
}

// Render implements Node.
func (c Canvas) Render(*Scope) Node { return c }

func (c Canvas) mountIn(t *Tree, cx *Scope, parent *node) { t.mountCanvas(cx, parent, c) }

type canvasNode struct {
	draw    func(*Drawing, Size)
	effect  *computation
	size    *Signal[Size]
	drawing *Drawing
	fresh   bool
	// paint and color are the paint properties and text color the
	// canvas inherits, which its drawing's view paints take.
	paint driver.PaintStyle
	color Color
	// paintKey is what paint was resolved from.
	paintKey canvasPaintKey
	// em sizes a canvas with Em or EmFunc.
	em *emNode
	// frames are the copies of the pixels the drawing last sent draws, by the app's buffer and version.
	frames map[pixelKey]*image.RGBA
}

type pixelKey struct {
	from    *image.RGBA
	version uint64
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
	n := t.newNode(driver.KindCanvas, parent, cx)
	t.bindClasses(cx, n, c.Class, c.ClassFunc)
	t.bindStyle(cx, n, c.Style, c.StyleFunc)
	t.setRef(n, c.Ref)
	n.h = handlers{press: c.OnPress, layout: c.OnLayout}
	n.props = driver.ViewProps{Role: RoleImage, Label: c.Label, TestID: c.TestID}
	t.bindAccessibility(cx, n, a11y{labelFunc: c.LabelFunc})
	cn := &canvasNode{draw: c.Draw, size: NewSignal(cx, Size{})}
	n.canvas = cn
	t.canvases = append(t.canvases, n)
	if c.Em.sized() || c.EmFunc != nil {
		cn.em = &emNode{box: c.Em}
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
		if size == (Size{}) || cn.draw == nil {
			return
		}
		d := driver.NewDrawing(t.text)
		cn.draw(d, size)
		// A canvas that has drawn nothing has nothing to clear.
		if cn.drawing == nil && len(d.Ops) == 0 && len(d.Nodes) == 0 {
			return
		}
		t.pickDrawingScales(d)
		if t.rt.restricted {
			t.refuseDrawingImages(n, d)
		}
		cn.drawing, cn.fresh = d, true
		t.touch(n)
	})
}

// pickDrawingScales has the images d draws name the variant for the
// screen, as Image's do.
func (t *Tree) pickDrawingScales(d *Drawing) {
	for i := range d.Ops {
		if op := &d.Ops[i]; op.Image != nil && op.Image.Scales != 0 {
			src := t.pickScale(*op.Image)
			op.Image = &src
		}
	}
}

// refuseDrawingImages drops the images of d the Access of n's owner
// refuses.
func (t *Tree) refuseDrawingImages(n *node, d *Drawing) {
	t.refuseDrawingImagesOwner(n.owner, d)
}

func (t *Tree) refuseDrawingImagesOwner(owner *Scope, d *Drawing) {
	for i := range d.Ops {
		if op := &d.Ops[i]; op.Image != nil {
			src := *op.Image
			t.refuseImageOwner(owner, &src)
			op.Image = &src
		}
	}
}

// MeasureText returns the advance width and the metrics of text's first
// line in font, laid out by the platform's text engine as a canvas measures
// it (Drawing.TextMetrics): a column fitted to its widest text, a
// monospaced font's character width.
func (cx *Scope) MeasureText(text string, font Font) TextMetrics {
	var engine driver.TextEngine
	if t := cx.tree(); t != nil {
		engine = t.text
	}
	return driver.NewDrawing(engine).TextMetrics(text, font)
}
