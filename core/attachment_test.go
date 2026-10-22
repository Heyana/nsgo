package core

import "testing"

func TestAttachmentsFlowInText(t *testing.T) {
	var wide *Signal[bool]
	var pressed int
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		wide = NewSignal(cx, false)
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", Spans: []Span{
				{Content: "ab "},
				{Attachment: View{TestID: "box", OnPress: func() { pressed++ },
					StyleFunc: func() Style {
						if wide.Get() {
							return Style{Width: Pt(40), Height: Pt(6)}
						}
						return Style{Width: Pt(10), Height: Pt(6)}
					}}},
				{Content: " cd"},
			}},
		}}
	})
	b := h.commit()
	text, box := h.find("text"), h.find("box")
	wrapper := box.parent
	if wrapper.attach == nil || wrapper.parent != text {
		t.Fatal("the attachment's view is not a child of the text")
	}
	inserted := false
	for _, it := range b.Items {
		if it.Kind == ItemInsert && it.ID == wrapper.id && it.Parent == text.id {
			inserted = true
		}
	}
	if !inserted {
		t.Error("the attachment's view is not inserted into the text's")
	}
	if got := text.text.text.Text; got != "ab ￼ cd" {
		t.Errorf("text = %q, want a placeholder for the attachment", got)
	}
	// Characters are 6 wide and lines 12 tall; the box stands on the
	// baseline, 6 above it.
	if got, want := wrapper.frame, (Bounds{Point{18, 6}, Size{10, 6}}); got != want {
		t.Errorf("attachment frame = %v, want %v", got, want)
	}
	if got := h.tree.HitTest(Point{20, 8}); got != box.id {
		t.Errorf("hit test = %d, want the attachment %d", got, box.id)
	}

	wide.Set(true)
	h.commit()
	if got, want := wrapper.frame.Size, (Size{40, 6}); got != want {
		t.Errorf("resized attachment = %v, want %v", got, want)
	}
	if got := text.text.text.Attachments[0].Size.Width; got != 40 {
		t.Errorf("text prepared with width %v, want 40", got)
	}
}

func TestAttachmentsAlignAndGrowLines(t *testing.T) {
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", Spans: []Span{
				{Content: "a"},
				// The attachment's own view says where it sits.
				{Attachment: View{TestID: "box", Style: Style{Width: Pt(10), Height: Pt(30), VerticalAlign: VerticalAlignMiddle}}},
			}},
		}}
	})
	h.commit()
	text, wrapper := h.find("text"), h.find("box").parent
	// Middle: its top (30 + 5) / 2 above the baseline, 12.5 below it.
	if got, want := text.frame.Size.Height, Points(30); got != want {
		t.Errorf("line height = %v, want %v", got, want)
	}
	if got := wrapper.frame.Origin; got != (Point{6, 0}) {
		t.Errorf("attachment origin = %v, want 6,0", got)
	}
}

func TestAttachmentsCutOffHide(t *testing.T) {
	h := newHost(t, Size{30, 200}, func(cx *Scope) Node {
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", MaxLines: 1, Spans: []Span{
				{Content: "abcd "},
				{Attachment: View{TestID: "box", Style: Style{Width: Pt(10), Height: Pt(6)}}},
			}},
		}}
	})
	h.commit()
	wrapper := h.find("box").parent
	if !wrapper.viewStyle().Hidden {
		t.Error("an attachment past the line limit is shown")
	}
}

func TestSpanBackgrounds(t *testing.T) {
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		return Text{TestID: "text", Spans: []Span{
			{Content: "a "},
			{Content: "code", Style: Style{Background: RGB(0xeeeeee), CornerRadius: 3}},
		}}
	})
	h.commit()
	spans := h.find("text").text.text.Spans
	if got, want := spans[1].Background, (TextBackground{RGB(0xeeeeee), 3}); got != want {
		t.Errorf("background = %+v, want %+v", got, want)
	}
	if spans[0].Background != (TextBackground{}) {
		t.Error("a span without a background has one")
	}
}
