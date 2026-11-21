package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestIconInheritsColorAndSize(t *testing.T) {
	s := MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="8"/></svg>`))
	var tint *Signal[Color]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		tint = NewSignal(cx, RGB(0x112233))
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			View{
				StyleFunc: func() Style {
					return Style{Color: tint.Get(), FontSize: 20, AlignItems: AlignStart}
				},
				Children: []Node{
					Icon{SVG: s, TestID: "icon"},
					Icon{SVG: s, TestID: "wide", Style: Style{Width: Pt(48), Height: Pt(24)}},
				},
			},
		}}
	})
	drawing := func(b *driver.MountBatch, id NodeID) *Drawing {
		for _, it := range b.Items {
			if it.Kind == driver.ItemCanvas && it.ID == id {
				return it.Drawing
			}
		}
		return nil
	}
	b := h.commit()
	icon := h.find("icon")
	if got := icon.frame.Size; got != (Size{Width: 20, Height: 20}) {
		t.Fatalf("icon size = %v, want its font size", got)
	}
	d := drawing(b, icon.id)
	if d == nil {
		t.Fatal("the icon did not draw")
	}
	found := false
	for _, op := range d.Ops {
		if op.Kind == DrawStrokeColor {
			found = true
			if op.Color != RGB(0x112233) {
				t.Errorf("stroke color = %#x, want the inherited text color", op.Color)
			}
		}
	}
	if !found {
		t.Fatal("no stroke color")
	}
	wide := h.find("wide")
	d = drawing(b, wide.id)
	if d == nil {
		t.Fatal("the wide icon did not draw")
	}
	// 24 units in a 24-point-high box, centered in the 48-point width.
	if op := d.Ops[1]; op.Kind != DrawTranslate || op.X != 12 || op.Y != 0 {
		t.Errorf("wide translate = %+v", op)
	}
	if op := d.Ops[2]; op.Kind != DrawScale || op.X != 1 || op.Y != 1 {
		t.Errorf("wide scale = %+v", op)
	}

	tint.Set(RGB(0xff8800))
	b = h.commit()
	d = drawing(b, icon.id)
	if d == nil {
		t.Fatal("a color change did not redraw")
	}
	for _, op := range d.Ops {
		if op.Kind == DrawStrokeColor && op.Color != RGB(0xff8800) {
			t.Errorf("stroke color after change = %#x", op.Color)
		}
	}
}

func TestIconTakesClasses(t *testing.T) {
	s := MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><circle cx="12" cy="12" r="8"/></svg>`))
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{Icon{SVG: s, Class: "size-8 text-[#112233]", TestID: "icon"}}}
	})
	b := h.commit()
	icon := h.find("icon")
	if got := icon.frame.Size; got != (Size{Width: 32, Height: 32}) {
		t.Fatalf("icon size = %v, want size-8's", got)
	}
	for _, it := range b.Items {
		if it.Kind != driver.ItemCanvas || it.ID != icon.id {
			continue
		}
		for _, op := range it.Drawing.Ops {
			if op.Kind == DrawStrokeColor && op.Color != RGB(0x112233) {
				t.Errorf("stroke color = %#x, want the class's", op.Color)
			}
		}
		return
	}
	t.Fatal("the icon did not draw")
}

func TestIconChangesDocument(t *testing.T) {
	square := MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect width="24" height="24"/></svg>`))
	wide := MustParseSVG([]byte(`<svg viewBox="0 0 48 24"><rect width="48" height="24"/></svg>`))
	var doc *Signal[*SVG]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		doc = NewSignal(cx, square)
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			Icon{SVGFunc: doc.Get, TestID: "icon"},
		}}
	})
	b := h.commit()
	icon := h.find("icon")
	if got, want := icon.frame.Size, (Size{Width: DefaultFontSize, Height: DefaultFontSize}); got != want {
		t.Fatalf("icon = %v, want its font size %v", got, want)
	}
	doc.Set(wide)
	b = h.commit()
	if got, want := icon.frame.Size, (Size{Width: DefaultFontSize * 2, Height: DefaultFontSize}); got != want {
		t.Fatalf("after the document changed, icon = %v, want %v", got, want)
	}
	found := false
	for _, it := range b.Items {
		if it.Kind == driver.ItemCanvas && it.ID == icon.id {
			found = true
		}
	}
	if !found {
		t.Fatal("a new document did not draw")
	}
}

func TestSVGViewTakesTheDocumentsSize(t *testing.T) {
	s := MustParseSVG([]byte(`<svg width="30" height="10" viewBox="0 0 3 1"><rect width="3" height="1" fill="red"/></svg>`))
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{AlignItems: AlignStart, FontSize: 20}, Children: []Node{SVGView{SVG: s, TestID: "doc"}}}
	})
	h.commit()
	if got := h.find("doc").frame.Size; got != (Size{Width: 30, Height: 10}) {
		t.Errorf("the view is %v, want the document's size", got)
	}
}

func TestCanvasDrawSVGUsesPlatformTextEngineAndRedrawsOnFontChange(t *testing.T) {
	diagram := MustParseSVG([]byte(`<svg viewBox="0 0 100 50">
		<rect x="0" y="0" width="100" height="50" fill="#eef"/>
		<text x="50" y="30" text-anchor="middle" font-size="16" fill="#112233">Hello</text>
	</svg>`))
	plain := MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="currentColor"/></svg>`))

	registered := false
	var scope *Scope
	h := newHost(t, Size{Width: 300, Height: 200}, nil)
	h.tree = NewTree(h.root, fontEngine{&registered})
	h.tree.Resize(Size{Width: 300, Height: 200}, Geometry{})
	h.tree.Mount(func(cx *Scope) Node {
		scope = cx
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			Canvas{
				TestID: "canvas",
				Style:  Style{Width: Pt(200), Height: Pt(100)},
				Draw: func(d *Drawing, size Size) {
					d.DrawSVGIn(diagram, 0, 0, float32(size.Width), float32(size.Height))
				},
			},
			SVGView{TestID: "svgview", SVG: diagram},
			Icon{TestID: "plain", SVG: plain},
		}}
	})
	h.commit()

	textOffset := func(id string) (DrawOp, bool) {
		for _, op := range h.find(id).canvas.drawing.Ops {
			if op.Kind == DrawFillText {
				return op, true
			}
		}
		return DrawOp{}, false
	}

	canvasBefore, ok := textOffset("canvas")
	if !ok || canvasBefore.Text != "Hello" || canvasBefore.X1 >= 0 {
		t.Fatalf("canvas DrawFillText = %+v (ok=%v)", canvasBefore, ok)
	}
	viewBefore, ok := textOffset("svgview")
	if !ok || viewBefore.Text != "Hello" || viewBefore.X1 != canvasBefore.X1 {
		t.Fatalf("svgview DrawFillText = %+v, want X1=%v", viewBefore, canvasBefore.X1)
	}

	if err := scope.RegisterFont([]byte("font")); err != nil {
		t.Fatal(err)
	}
	batch := h.commit()

	if h.drawings(batch, "canvas") != 1 {
		t.Fatalf("canvas updates after RegisterFont = %d, want 1", h.drawings(batch, "canvas"))
	}
	if h.drawings(batch, "svgview") != 1 {
		t.Fatalf("svgview with text updates after RegisterFont = %d, want 1", h.drawings(batch, "svgview"))
	}
	if h.drawings(batch, "plain") != 0 {
		t.Fatalf("plain icon without text redrew %d times after RegisterFont, want 0", h.drawings(batch, "plain"))
	}

	canvasAfter, _ := textOffset("canvas")
	viewAfter, _ := textOffset("svgview")
	if canvasAfter.X1 != canvasBefore.X1*2 {
		t.Errorf("canvas text offset after font change = %v, want %v", canvasAfter.X1, canvasBefore.X1*2)
	}
	if viewAfter.X1 != viewBefore.X1*2 {
		t.Errorf("svgview text offset after font change = %v, want %v", viewAfter.X1, viewBefore.X1*2)
	}
}
