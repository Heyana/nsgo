package core

import "testing"

func TestContentBoxTracksPaddingInsideAFixedFrame(t *testing.T) {
	var ref Ref
	var padding *Signal[Points]
	var seen Bounds
	h := newHost(t, Size{Width: 500, Height: 300}, func(cx *Scope) Node {
		padding = NewSignal(cx, Points(5))
		return View{Ref: &ref, TestID: "box", Class: "px-4", StyleFunc: func() Style {
			return Style{Width: Pt(200), Height: Pt(100), BorderWidth: 2, Padding: Edges{Top: padding.Get(), Bottom: padding.Get()}}
		}, Children: []Node{View{Style: Style{Width: Pt(300), Height: Pt(10)}}, Func(func(child *Scope) Node {
			child.Effect(func() { seen, _ = ref.ContentBox() })
			return nil
		})}}
	})
	h.commit()
	if seen != (Bounds{Origin: Point{X: 18, Y: 7}, Size: Size{Width: 164, Height: 86}}) {
		t.Fatalf("content box=%v", seen)
	}
	padding.Set(12)
	h.commit()
	if seen.Origin.Y != 14 || seen.Size.Height != 72 || h.find("box").frame.Size != (Size{Width: 200, Height: 100}) {
		t.Fatalf("padding change: content=%v frame=%v", seen, h.find("box").frame.Size)
	}
	var empty *Ref
	if _, ok := empty.ContentBox(); ok {
		t.Fatal("nil reference reported a content box")
	}
}
