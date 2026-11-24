//go:build windows

package windows

import (
	"image"
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasArgumentsQueueWhatTheyChange draws a tree with a canvas whose
// bound value moves a ring, and checks without a window that the change
// queues only an area of the canvas, that the frame's pixels change only
// inside it, that drawing the tree clipped to it gives a whole frame's
// pixels there, and that a view whose own paint misses it paints none.
func TestCanvasArgumentsQueueWhatTheyChange(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	ring := canvastest.Split(nil)[0]
	const width, height = 200, 140
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	w.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 60}})
	root := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: width, Height: height}},
		style: driver.ViewStyle{Opacity: 1, Background: driver.RGB(0xffffff)}}
	// A box whose canvas lies outside it: its own paint is its corner.
	holder := &view{id: 2, kind: driver.KindView, parent: root,
		frame: driver.Bounds{Size: driver.Size{Width: 12, Height: 12}},
		style: driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x2266cc), ShadowColor: driver.RGBA(0, 0.4),
			ShadowRadius: 3}}
	canvas := &view{id: 3, kind: driver.KindCanvas, parent: holder,
		frame: driver.Bounds{Origin: driver.Point{X: 20, Y: 20}, Size: driver.Size{Width: driver.Points(ring.Width),
			Height: driver.Points(ring.Height)}}, style: driver.ViewStyle{Opacity: 1}}
	root.children, holder.children = []*view{holder}, []*view{canvas}
	w.root, w.views = root.id, []*view{nil, root, holder, canvas}
	canvas.drawing = ring.Drawing
	w.animator.BindCanvas(canvas.id, ring.Drawing.Slots)
	measure := func() {
		m := measuring{w: w}
		m.visit(root, identity, 0, 0, rectF{0, 0, width, height}, true, false)
	}
	draw := func(areas []rect) *image.RGBA {
		return drawImage(t, g, width, height, func(p *painter) {
			if areas != nil {
				p.pushAreas(areas, colorF{1, 1, 1, 1})
				if p.paintsOwn(holder) {
					t.Error("the box whose own paint misses the frame's areas paints it")
				}
				if !p.paintsOwn(canvas) {
					t.Error("the canvas the areas cover does not paint")
				}
			}
			p.draw(root, identity, 0, 0)
			if areas != nil {
				p.popAreas()
			}
		}, w)
	}
	measure()
	before := draw(nil)
	w.changedCanvases()
	w.damage = damage{}
	canvas.queued = false
	w.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 120}})
	w.changedCanvases()
	d := &w.damage
	if d.whole || len(d.queued) > 0 || len(d.old) != 1 {
		t.Fatalf("the change queued whole %v, views %d, areas %d; want one area", d.whole, len(d.queued), len(d.old))
	}
	area := pixels(d.old[0], w.scale)
	if canvasArea := int64(ring.Width*ring.Height) * canvastest.Scale * canvastest.Scale; area.area()*4 > canvasArea {
		t.Errorf("the change queued %v, %d pixels of the canvas's %d", area, area.area(), canvasArea)
	}
	after := draw(nil)
	partial := draw([]rect{area})
	in := image.Rect(int(area.Left), int(area.Top), int(area.Right), int(area.Bottom))
	outside, inside := 0, 0
	for y := range after.Rect.Dy() {
		for x := range after.Rect.Dx() {
			a, b := after.RGBAAt(x, y), before.RGBAAt(x, y)
			if image.Pt(x, y).In(in) {
				if c := partial.RGBAAt(x, y); far(c, a) {
					inside++
					t.Logf("%d,%d: %v, whole %v", x, y, c, a)
				}
			} else if a != b {
				outside++
			}
		}
	}
	if outside > 0 {
		t.Errorf("the change moved %d pixels outside the area it queued", outside)
	}
	if inside > 0 {
		t.Errorf("drawing the area differs from a whole frame in %d of its pixels", inside)
	}
}

// far reports whether a and b differ by more than anti-aliasing may.
func far(a, b color.RGBA) bool {
	d := func(x, y uint8) bool { return int(x) > int(y)+3 || int(y) > int(x)+3 }
	return d(a.R, b.R) || d(a.G, b.G) || d(a.B, b.B) || d(a.A, b.A)
}
