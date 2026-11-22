package core

import (
	"math"
	"testing"
)

func TestWidthAwareEmCanvasKeepsItsFontSize(t *testing.T) {
	var width *Signal[Points]
	var units *Signal[float32]
	var font Points
	h := newHost(t, Size{Width: 500, Height: 400}, func(cx *Scope) Node {
		width, units = NewSignal(cx, Points(120)), NewSignal(cx, float32(18))
		return View{StyleFunc: func() Style { return Style{Width: Pt(width.Get()), FontSize: 20} }, Children: []Node{
			Canvas{TestID: "wrapped", MeasureEm: func(w, fs Points) EmBox {
				font = fs
				n := units.Get()
				if w.IsAuto() {
					return EmBox{Width: n, Ascent: .8, Descent: .2}
				}
				line := max(float32(w/fs), 1)
				rows := float32(math.Ceil(float64(n / line)))
				return EmBox{Width: min(line, n), Ascent: .8, Descent: rows - .8}
			}},
		}}
	})
	check := func(w, height Points) {
		t.Helper()
		h.commit()
		if got := h.find("wrapped").frame.Size; got != (Size{Width: w, Height: height}) || font != 20 {
			t.Fatalf("canvas=%v font=%v, want %vx%v at 20pt", got, font, w, height)
		}
	}
	check(120, 60)
	width.Set(240)
	check(240, 40)
	width.Set(120)
	check(120, 60)
	units.Set(24)
	check(120, 80)
}

func TestWidthAwareEmCanvasReadsFontChangesWithFixedFrame(t *testing.T) {
	var fontSize *Signal[Points]
	var measured Points
	h := newHost(t, Size{Width: 300, Height: 100}, func(cx *Scope) Node {
		fontSize = NewSignal(cx, Points(16))
		return Canvas{TestID: "fixed", StyleFunc: func() Style {
			return Style{Width: Pt(120), Height: Pt(40), FontSize: fontSize.Get()}
		}, MeasureEm: func(w, fs Points) EmBox {
			measured = fs
			return EmBox{Width: 6, Ascent: 1, Descent: 1}
		}}
	})
	h.commit()
	fontSize.Set(24)
	h.commit()
	if measured != 24 || h.find("fixed").frame.Size != (Size{Width: 120, Height: 40}) {
		t.Fatalf("font=%v frame=%v", measured, h.find("fixed").frame.Size)
	}
}
