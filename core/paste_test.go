package core

import "testing"

func TestCaretAndPasteReachTheInputAndAnOverlayAnchorsToTheCaret(t *testing.T) {
	var caret *Signal[Bounds]
	var pasted []PasteEvent
	h := newHost(t, Size{300, 300}, func(cx *Scope) Node {
		caret = NewSignal(cx, Bounds{})
		return View{Style: Style{Padding: All(20)}, Children: []Node{
			Input{TestID: "field", Style: Style{Height: Pt(30)}, PasteBehavior: PasteReport,
				OnCaretMove: func(e CaretEvent) { caret.Set(e.PageBounds) },
				OnPaste:     func(e PasteEvent) { pasted = append(pasted, e) }},
			Overlay{AnchorBounds: caret.Get, Offset: 4, TestID: "mentions",
				Style: Style{Width: Pt(80), Height: Pt(40)}},
		}}
	})
	h.commit()
	field := h.find("field")
	if m := field.events(); !m.Has(EventCaretMove) || !m.Has(EventPaste) {
		t.Fatal("an input with OnCaretMove and OnPaste does not report them")
	}
	if field.input.config.PasteBehavior != PasteReport {
		t.Fatal("PasteBehavior does not reach the platform")
	}
	h.tree.Dispatch(Event{Kind: EventCaretMove, ID: field.id, Point: Point{50, 4}, Size: Size{2, 20}})
	if got, want := caret.GetUntracked(), (Bounds{Point{70, 24}, Size{2, 20}}); got != want {
		t.Fatalf("caret page bounds = %v, want %v", got, want)
	}
	h.commit()
	if got := h.find("mentions").frame.Origin; got != (Point{31, 48}) {
		t.Fatalf("overlay at %v, want centred under the caret, 4 points below", got)
	}
	h.tree.Dispatch(Event{Kind: EventPaste, ID: field.id, Data: &PasteEvent{Text: "hi",
		Files: []PastedFile{{MIME: "image/png", Data: []byte{1}}}}})
	if len(pasted) != 1 || pasted[0].Text != "hi" || len(pasted[0].Files) != 1 {
		t.Fatalf("pasted %+v", pasted)
	}
}
