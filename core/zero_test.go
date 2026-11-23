package core

import "testing"

// TestZeroTakesAwayALength gives a view a padding, a gap and a border, takes
// them away with Zero from its StyleFunc, and checks it lays them out as 0.
func TestZeroTakesAwayALength(t *testing.T) {
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		return View{TestID: "box", Style: Style{Padding: All(10), Gap: 8, BorderWidth: 2},
			StyleFunc: func() Style { return Style{Padding: All(Zero), Gap: Zero, BorderWidth: Zero} },
			Children: []Node{
				View{TestID: "first", Style: Style{Height: Pt(10)}},
				View{TestID: "second", Style: Style{Height: Pt(10)}},
			}}
	})
	h.commit()
	if s := h.find("box").style; s.Padding != (Edges{}) || s.Gap != 0 || s.BorderWidth != 0 {
		t.Fatalf("padding %v, gap %v, border %v; want none", s.Padding, s.Gap, s.BorderWidth)
	}
	if o := h.find("first").frame.Origin; o != (Point{}) {
		t.Fatalf("first child at %v, want the box's corner", o)
	}
	if y := h.find("second").frame.Origin.Y; y != 10 {
		t.Fatalf("second child at %v, want 10 with no gap", y)
	}
}
