package core

import "github.com/oneo-me/nsgo/internal/driver"

// Canvas draws what Draw records, at the canvas's size, again whenever a
// signal Draw read or the size changes. The platform plays the recording
// on the main thread; Draw itself runs on the engine.
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
}

func (t *Tree) mountCanvas(cx *Scope, parent *node, c Canvas) {
	n := t.newNode(driver.KindCanvas, parent, cx)
	t.bindStyle(cx, n, c.Style, c.StyleFunc)
	t.setRef(n, c.Ref)
	n.h = handlers{press: c.OnPress, layout: c.OnLayout}
	n.props = driver.ViewProps{Role: RoleImage, Label: c.Label, TestID: c.TestID}
	cn := &canvasNode{draw: c.Draw, size: NewSignal(cx, Size{})}
	n.canvas = cn
	cx.renderEffect(func() {
		size := cn.size.Get()
		if size == (Size{}) || cn.draw == nil {
			return
		}
		d := &Drawing{}
		cn.draw(d, size)
		cn.drawing, cn.fresh = d, true
		t.touch(n)
	})
}
