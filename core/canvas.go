package core

import "github.com/oneo-me/nsgo/internal/driver"

// Canvas draws what Draw records, at the canvas's size, again whenever a
// signal Draw read or the size changes. The platform plays the recording
// on the main thread; Draw itself runs on the engine.
//
// The Drawing measures text with the platform's text engine
// (MeasureText, TextMetrics) and answers hit tests (IsPointInPath,
// IsPointInStroke) during Draw and after it, so OnPress can test the
// shapes of the last drawing Draw was given. Arguments bound to animated
// nodes (Drawing.Bind) redraw on the platform's frame clock without a
// commit, and Drawing.Node names parts of the drawing for assistive
// technology and tests.
type Canvas struct {
	Draw      func(d *Drawing, size Size)
	Style     Style
	StyleFunc func() Style
	OnPress   func(PressEvent)
	OnLayout  func(Bounds)
	Ref       *Ref
	Label     string
	TestID    string
}

// Render implements Node.
func (c Canvas) Render(*Scope) Node { return c }

type canvasNode struct {
	draw    func(*Drawing, Size)
	size    *Signal[Size]
	drawing *Drawing
	fresh   bool
	// paint and color are the paint properties and text color the
	// canvas inherits, which its drawing's view paints take.
	paint driver.PaintStyle
	color Color
	// paintKey is what paint was resolved from.
	paintKey canvasPaintKey
}

// canvasPaintKey is what a canvas's view paints are resolved from.
type canvasPaintKey struct {
	fill, stroke Paint
	width        Points
	dash         string
	color        Color
}

// resolvePaint takes the paint properties canvas node n inherits and has
// its drawing sent again when they change what it paints.
func (t *Tree) resolvePaint(n *node, inherited *textFields) {
	cn := n.canvas
	k := canvasPaintKey{inherited.Fill, inherited.Stroke, inherited.StrokeWidth, inherited.StrokeDash, n.color}
	if k == cn.paintKey {
		return
	}
	cn.paintKey, cn.paint, cn.color = k, inherited.paint(), n.color
	if cn.drawing != nil && len(cn.drawing.ViewPaints) > 0 {
		cn.fresh = true
	}
}

func (t *Tree) mountCanvas(cx *Scope, parent *node, c Canvas) {
	n := t.newNode(driver.KindCanvas, parent, cx)
	t.bindStyle(cx, n, c.Style, c.StyleFunc)
	t.setRef(n, c.Ref)
	n.h = handlers{press: c.OnPress, layout: c.OnLayout}
	n.props = driver.ViewProps{Role: RoleImage, Label: c.Label, TestID: c.TestID}
	cn := &canvasNode{draw: c.Draw, size: NewSignal(cx, Size{})}
	n.canvas = cn
	t.canvases = append(t.canvases, n)
	cx.renderEffect(func() {
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
		cn.drawing, cn.fresh = d, true
		t.touch(n)
	})
}

// pickDrawingScales has the images d draws name the variant for the
// screen, as Image's do.
func (t *Tree) pickDrawingScales(d *Drawing) {
	for i := range d.Ops {
		if op := &d.Ops[i]; op.Kind == driver.DrawImage && op.Image != nil && op.Image.Scales != 0 {
			src := t.pickScale(*op.Image)
			op.Image = &src
		}
	}
}
