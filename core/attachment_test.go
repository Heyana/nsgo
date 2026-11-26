package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAttachmentsFlowInText(t *testing.T) {
	var wide *Signal[bool]
	var pressed int
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		wide = NewSignal(cx, false)
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", Spans: []Span{
				{Content: "ab "},
				{Attachment: View{TestID: "box", OnPress: func(PressEvent) { pressed++ },
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
		if it.Kind == driver.ItemInsert && it.ID == wrapper.id && it.Parent == text.id {
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
	if got, want := wrapper.frame, (Bounds{Origin: Point{X: 18, Y: 6}, Size: Size{Width: 10, Height: 6}}); got != want {
		t.Errorf("attachment frame = %v, want %v", got, want)
	}
	if got := h.tree.HitTest(Point{X: 20, Y: 8}); got != box.id {
		t.Errorf("hit test = %d, want the attachment %d", got, box.id)
	}

	wide.Set(true)
	h.commit()
	if got, want := wrapper.frame.Size, (Size{Width: 40, Height: 6}); got != want {
		t.Errorf("resized attachment = %v, want %v", got, want)
	}
	if got := text.text.text.Attachments[0].Size.Width; got != 40 {
		t.Errorf("text prepared with width %v, want 40", got)
	}
}

func TestTextAttachmentOverflowIsPressable(t *testing.T) {
	pressed := 0
	h := newHost(t, Size{Width: 200, Height: 200}, func(*Scope) Node {
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", Spans: []Span{
				{Content: "ab "},
				{Attachment: View{TestID: "box", OnPress: func(PressEvent) { pressed++ },
					Style: Style{Width: Pt(10), Height: Pt(6), Transform: Transform{TranslateY: 40}}}},
			}},
		}}
	})
	b := h.commit()
	text, box := h.find("text"), h.find("box")
	var overflow *driver.MountItem
	for i := range b.Items {
		if b.Items[i].Kind == driver.ItemOverflow && b.Items[i].ID == text.id {
			overflow = &b.Items[i]
		}
	}
	if overflow == nil || !overflow.Transformed {
		t.Fatalf("text overflow = %+v, want transformed overflow", overflow)
	}
	at := Point{X: 20, Y: 48}
	if got := h.tree.HitTest(at); got != box.id {
		t.Fatalf("hit test at attachment = %d, want %d", got, box.id)
	}
	h.tree.PressAt(driver.Event{Page: at})
	if pressed != 1 {
		t.Fatalf("presses = %d, want 1", pressed)
	}
}

func TestAttachmentsAlignAndGrowLines(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
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
	if got := wrapper.frame.Origin; got != (Point{X: 6, Y: 0}) {
		t.Errorf("attachment origin = %v, want 6,0", got)
	}
}

func TestTextAttachmentUsesItsFirstTextBaseline(t *testing.T) {
	var offset *Signal[Points]
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		offset = NewSignal(cx, Points(4))
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", Spans: []Span{
				{Content: "a"},
				{Attachment: View{TestID: "attachment", Style: Style{Width: Pt(40), Height: Pt(30)}, Children: []Node{
					View{StyleFunc: func() Style { return Style{Padding: Edges{Top: offset.Get()}} },
						Children: []Node{Text{Content: "ruby", Style: Style{Padding: Edges{Top: 2}}}}},
				}}},
			}},
		}}
	})
	check := func(want Points) {
		t.Helper()
		h.commit()
		if got := h.find("text").text.text.Attachments[0].Descent; got != want {
			t.Fatalf("attachment descent = %v, want %v", got, want)
		}
	}
	check(12) // height 30, with 4 + 2 + 12 above the baseline
	offset.Set(7)
	check(9) // only the text's position in the fixed-height attachment moved
}

func TestAttachmentsCutOffHide(t *testing.T) {
	h := newHost(t, Size{Width: 30, Height: 200}, func(cx *Scope) Node {
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
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return Text{TestID: "text", Spans: []Span{
			{Content: "a "},
			{Content: "code", Style: Style{Background: RGB(0xeeeeee), CornerRadius: 3, BackgroundInsetTop: Pct(62), BackgroundInsetBottom: Pt(2)}},
			{Content: "hidden", Style: Style{Background: RGB(0xff0000), BackgroundInsetTop: Pct(62), Visibility: VisibilityHidden}},
		}}
	})
	h.commit()
	spans := h.find("text").text.text.Spans
	if got, want := spans[1].Background, (driver.TextBackground{Color: RGB(0xeeeeee), Radius: 3, Top: Pct(62), Bottom: Pt(2)}); got != want {
		t.Errorf("background = %+v, want %+v", got, want)
	}
	if spans[0].Background != (driver.TextBackground{}) {
		t.Error("a span without a background has one")
	}
	if !spans[2].Hidden || spans[2].Background != (driver.TextBackground{}) {
		t.Errorf("hidden span kept its background: %+v", spans[2])
	}
}

func TestSpanOpacityFadesItsColors(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return Text{TestID: "text", Style: Style{Color: RGB(0x112233)}, Spans: []Span{
			{Content: "a "},
			{Content: "new", Style: Style{Opacity: Alpha(0.5), Background: RGB(0xeeeeee), TextDecoration: TextDecorationUnderline}},
		}}
	})
	h.commit()
	spans := h.find("text").text.text.Spans
	if c := spans[0].Style.Color; c != RGB(0x112233) {
		t.Errorf("an opaque span's color = %08x", uint32(c))
	}
	half := RGB(0x112233).Faded(0.5)
	if st := spans[1].Style; st.Color != half || st.DecorationColor != half || spans[1].Background.Color != RGB(0xeeeeee).Faded(0.5) {
		t.Errorf("a half faded span = %+v, background %08x", st, uint32(spans[1].Background.Color))
	}
}

func TestSpansFuncAttachmentsFollowTheSpans(t *testing.T) {
	var at *Signal[int]
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		at = NewSignal(cx, 1)
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", SpansFunc: func() []Span {
				box := Span{Attachment: View{TestID: "box", Style: Style{Width: Pt(10), Height: Pt(6)}}}
				switch at.Get() {
				case 0:
					return []Span{{Content: "ab"}}
				case 1:
					return []Span{{Content: "a"}, box, {Content: "b"}}
				}
				return []Span{{Content: "ab"}, box}
			}},
		}}
	})
	h.commit()
	text := h.find("text")
	if got := text.text.text.Text; got != "a￼b" {
		t.Fatalf("text = %q", got)
	}
	first := h.find("box").parent
	if first.parent != text || first.frame.Origin.X != 6 {
		t.Fatalf("attachment %v in %v", first.frame, first.parent)
	}
	at.Set(2)
	h.commit()
	moved := h.find("box").parent
	if !first.deleted || moved.frame.Origin.X != 12 || len(text.children) != 1 {
		t.Fatalf("attachment %v, old deleted %v, %d children", moved.frame, first.deleted, len(text.children))
	}
	at.Set(0)
	h.commit()
	if got := text.text.text.Text; got != "ab" || len(text.children) != 0 || !moved.deleted {
		t.Fatalf("text %q with %d children", got, len(text.children))
	}
}

// countedBox is an attachment that counts how often it is made and disposed.
type countedBox struct {
	id            string
	renders, gone int
}

func (b *countedBox) Render(cx *Scope) Node {
	b.renders++
	cx.Cleanup(func() { b.gone++ })
	return View{TestID: b.id, Style: Style{Width: Pt(10), Height: Pt(6)}}
}

// sizedBox is an attachment of a comparable type, the same when its fields are.
type sizedBox struct {
	id    string
	width Points
}

var sizedRenders int

func (b sizedBox) Render(*Scope) Node {
	sizedRenders++
	return View{TestID: b.id, Style: Style{Width: Pt(b.width), Height: Pt(6)}}
}

// A SpansFunc that runs again with the same attachments in the same order, the text around them split or restyled, keeps their views; another attachment, or one that cannot be compared, is made again.
func TestSpansFuncKeepsTheSameAttachments(t *testing.T) {
	var step *Signal[int]
	box := &countedBox{id: "box"}
	sizedRenders = 0
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		step = NewSignal(cx, 0)
		return Text{TestID: "text", SpansFunc: func() []Span {
			sized := Span{Content: "$x$", Attachment: sizedBox{id: "sized", width: 8}}
			switch step.Get() {
			case 0:
				return []Span{{Content: "abc "}, {Attachment: box}, {Content: " d"}, sized}
			case 1:
				red := Style{Color: RGB(0xff0000)}
				return []Span{{Content: "a"}, {Content: "bc", Style: red}, {Content: " "}, {Attachment: box}, {Content: " "}, {Content: "d", Style: red}, sized}
			case 2:
				return []Span{{Content: "abc "}, {Attachment: &countedBox{id: "other"}}, {Content: " d"}, sized}
			}
			return []Span{{Content: "abc "}, {Attachment: Func(func(*Scope) Node { return View{TestID: "func"} })}, {Content: " d"}, sized}
		}}
	})
	h.commit()
	text := h.find("text")
	first, sized := h.find("box"), h.find("sized")
	step.Set(1)
	h.commit()
	if box.renders != 1 || sizedRenders != 1 || first.deleted || sized.deleted || h.find("box") != first {
		t.Fatalf("restyling made the attachments again: box %d, sized %d, deleted %v %v", box.renders, sizedRenders, first.deleted, sized.deleted)
	}
	red := 0
	for _, sp := range text.text.text.Spans {
		if sp.Style.Color == RGB(0xff0000) {
			red++
		}
	}
	if red != 2 {
		t.Fatalf("restyled spans %+v", text.text.text.Spans)
	}
	if got := text.text.text.Text; got != "abc ￼ d￼" || len(text.children) != 2 {
		t.Fatalf("text %q with %d children", got, len(text.children))
	}
	step.Set(2)
	h.commit()
	if !first.deleted || h.find("other") == nil || len(text.children) != 2 {
		t.Fatalf("another attachment kept the old view: deleted %v, %d children", first.deleted, len(text.children))
	}
	step.Set(3)
	h.commit()
	if h.find("func") == nil || len(text.children) != 2 {
		t.Fatalf("%d children", len(text.children))
	}
}

// A SpansFunc keeps the view of each attachment its new spans still hold, wherever it moves among them: one more makes only the new one, one fewer disposes only that one, and a new order moves the kept views into it.
func TestSpansFuncKeepsEachAttachment(t *testing.T) {
	var step *Signal[int]
	a, b := &countedBox{id: "a"}, &countedBox{id: "b"}
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		step = NewSignal(cx, 0)
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{TestID: "text", SpansFunc: func() []Span {
				switch step.Get() {
				case 0:
					return []Span{{Content: "x"}, {Attachment: a}}
				case 1:
					return []Span{{Content: "x"}, {Attachment: a}, {Content: "y"}, {Attachment: b}}
				case 2:
					return []Span{{Attachment: b}, {Content: "xy"}, {Attachment: a}}
				}
				return []Span{{Content: "xy"}, {Attachment: a}}
			}},
		}}
	})
	h.commit()
	text := h.find("text")
	wa := h.find("a").parent

	step.Set(1)
	h.commit()
	wb := h.find("b").parent
	if a.renders != 1 || a.gone != 0 || wa.deleted || h.find("a").parent != wa {
		t.Fatalf("adding an attachment made the kept one again: renders %d, disposed %d, deleted %v", a.renders, a.gone, wa.deleted)
	}
	if b.renders != 1 || wb.parent != text || len(text.children) != 2 || text.text.attachments != 2 {
		t.Fatalf("the new attachment: renders %d, %d children, %d attachments", b.renders, len(text.children), text.text.attachments)
	}
	// Characters are 6 wide.
	if wa.frame.Origin.X != 6 || wb.frame.Origin.X != 22 {
		t.Fatalf("attachments at %v and %v", wa.frame.Origin, wb.frame.Origin)
	}

	step.Set(2)
	batch := h.commit()
	if a.renders != 1 || b.renders != 1 || a.gone != 0 || b.gone != 0 || wa.deleted || wb.deleted {
		t.Fatalf("reordering made the attachments again: renders %d %d, disposed %d %d", a.renders, b.renders, a.gone, b.gone)
	}
	if got := text.sent.children; len(got) != 2 || got[0] != wb || got[1] != wa {
		t.Fatalf("the text's views are %v, want b then a", got)
	}
	for _, it := range batch.Items {
		if (it.Kind == driver.ItemRemove || it.Kind == driver.ItemDelete) && (it.ID == wa.id || it.ID == wb.id) {
			t.Fatalf("reordering removed an attachment's view: %+v", it)
		}
	}
	if wb.frame.Origin.X != 0 || wa.frame.Origin.X != 22 {
		t.Fatalf("reordered attachments at %v and %v", wb.frame.Origin, wa.frame.Origin)
	}

	step.Set(3)
	h.commit()
	if !wb.deleted || b.gone != 1 || a.gone != 0 || a.renders != 1 || wa.deleted || len(text.children) != 1 || text.text.attachments != 1 {
		t.Fatalf("removing an attachment: b deleted %v disposed %d, a disposed %d renders %d, %d children", wb.deleted, b.gone, a.gone, a.renders, len(text.children))
	}
	if got := text.sent.children; len(got) != 1 || got[0] != wa {
		t.Fatalf("the text's views are %v, want a", got)
	}
}

// TestAttachmentsBesidePositionalClasses places attachments as their text says once a view anywhere has classes that follow its place among its siblings, which have the tree read a parent's children as each is made, before it knows a child is an attachment.
func TestAttachmentsBesidePositionalClasses(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return View{Style: Style{FontSize: 10}, Children: []Node{
			View{Class: "h-1 last:hidden"},
			Text{TestID: "text", Spans: []Span{
				{Content: "a"},
				{Attachment: View{TestID: "box", Style: Style{Width: Pt(10), Height: Pt(30), VerticalAlign: VerticalAlignMiddle}}},
			}},
		}}
	})
	h.commit()
	if got := h.find("box").parent.frame.Origin; got != (Point{X: 6, Y: 0}) {
		t.Errorf("attachment origin = %v, want 6,0", got)
	}
}
