package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestScopeTextMeasurementsFollowFontChanges(t *testing.T) {
	registered := false
	h := newHost(t, Size{Width: 400, Height: 100}, nil)
	h.tree = NewTree(h.root, fontEngine{&registered})
	h.tree.Resize(Size{Width: 400, Height: 100}, Geometry{})
	measurements := 0
	h.tree.Mount(func(cx *Scope) Node {
		width := NewMemo(cx, func() float32 {
			measurements++
			return cx.MeasureText("hello", Font{Size: 20}).Width
		})
		return Canvas{TestID: "measured", EmFunc: func() EmBox {
			return EmBox{Width: width.Get() / 20, Ascent: .8, Descent: .2}
		}, Style: Style{FontSize: 20, AlignSelf: AlignStart}}
	})
	h.commit()
	before := h.find("measured").frame.Size.Width
	registered = true
	h.tree.FontsChanged()
	h.commit()
	if got := h.find("measured").frame.Size.Width; got != before*2 || measurements != 2 {
		t.Fatalf("font change left stale measurements: width=%v, before=%v, measurements=%d", got, before, measurements)
	}
	h.commit()
	if measurements != 2 {
		t.Fatal("idle commit measured the text again")
	}
}

func TestCanvasDrawsAgainWhenFontsChange(t *testing.T) {
	for _, async := range []bool{false, true} {
		name := "RegisterFont"
		if async {
			name = "FontsChanged"
		}
		t.Run(name, func(t *testing.T) {
			registered := false
			var scope *Scope
			var scale *Signal[float32]
			draws := 0
			h := newHost(t, Size{Width: 300, Height: 100}, nil)
			h.tree = NewTree(h.root, fontEngine{&registered})
			h.tree.Resize(Size{Width: 300, Height: 100}, Geometry{})
			h.tree.Mount(func(cx *Scope) Node {
				scope = cx
				scale = NewSignal(cx, float32(1))
				return Group{Icon{SVG: MustParseSVG([]byte(`<svg width="10" height="10"><path d="M0 0L10 10"/></svg>`))}, Canvas{TestID: "measured", Style: Style{Height: Pt(60)}, Draw: func(d *Drawing, _ Size) {
					draws++
					// Measuring text can affect geometry without recording a text op.
					width := d.TextMetrics("hello", Font{Family: "late font", Size: 20}).Width
					d.FillRect(0, 0, width*scale.Get(), 20)
				}}}
			})
			h.commit()
			width := func() float32 {
				for _, op := range h.find("measured").canvas.drawing.Ops {
					if op.Kind == DrawRect {
						return op.W
					}
				}
				t.Fatal("the canvas recorded no rectangle")
				return 0
			}
			before := width()
			if async {
				registered = true
				h.tree.FontsChanged()
			} else if err := scope.RegisterFont([]byte("font")); err != nil {
				t.Fatal(err)
			}
			batch := h.commit()
			if draws != 2 || h.drawings(batch, "measured") != 1 || width() != before*2 {
				t.Fatalf("after font change: draws=%d, width=%v (before %v), canvas updates=%d", draws, width(), before, h.drawings(batch, "measured"))
			}
			scale.Set(2)
			h.commit()
			if draws != 3 || width() != before*4 {
				t.Fatalf("drawing lost its signal dependency: draws=%d, width=%v", draws, width())
			}
			if h.drawings(h.commit(), "measured") != 0 || draws != 3 {
				t.Fatal("an idle commit drew the canvas again")
			}
		})
	}
}

func TestFontChangeDoesNotDrawDisposedCanvases(t *testing.T) {
	var shown *Signal[bool]
	draws := 0
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		shown = NewSignal(cx, true)
		return Show{When: shown.Get, Then: func(*Scope) Node {
			return Canvas{Style: Style{Height: Pt(40)}, Draw: func(d *Drawing, _ Size) {
				draws++
				d.FillText("hello", 0, 20, Font{Size: 20})
			}}
		}}
	})
	h.commit()
	shown.Set(false)
	h.commit()
	h.tree.FontsChanged()
	batch := h.commit()
	if draws != 1 || kinds(batch)[driver.ItemCanvas] != 0 {
		t.Fatalf("disposed canvas drew again: draws=%d, items=%v", draws, kinds(batch))
	}
}
