package core

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTextFontScaling(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{FontSize: 10}, Children: []Node{
			Text{Content: "a", TestID: "scaled"},
			Text{Content: "b", TestID: "fixed", NoFontScaling: true},
			Text{Content: "c", TestID: "capped", MaxFontSizeMultiplier: 1.5},
		}}
	})
	h.tree.SetEnvironment(Environment{FontScale: 2})
	h.commit()
	for id, want := range map[string]Points{"scaled": 20, "fixed": 10, "capped": 15} {
		if got := h.find(id).text.style.Font.Size; got != want {
			t.Errorf("%s: font size = %v, want %v", id, got, want)
		}
	}
}

func TestTextLanguageInheritsAndOverrides(t *testing.T) {
	var scope *Scope
	h := newHost(t, Size{Width: 300, Height: 100}, func(cx *Scope) Node {
		scope = cx
		return View{Children: []Node{
			Text{TestID: "default", Content: "直"},
			View{Style: Style{Language: "ja"}, Children: []Node{
				Text{TestID: "inherited", Content: "直"},
				Text{TestID: "span", Spans: []Span{{Content: "直", Style: Style{Language: "zh-Hant"}}, {Content: "直"}}},
			}},
		}}
	})
	h.tree.SetEnvironment(Environment{Locale: "en-US"})
	h.commit()
	language := func(id string) string { return h.find(id).text.text.Style.Font.Language }
	if got := language("default"); got != "en-US" {
		t.Fatalf("default language = %q, want en-US", got)
	}
	if got := language("inherited"); got != "ja" {
		t.Fatalf("inherited language = %q, want ja", got)
	}
	spans := h.find("span").text.text.Spans
	if got := spans[0].Style.Font.Language; got != "zh-Hant" {
		t.Fatalf("span language = %q, want zh-Hant", got)
	}
	if got := spans[1].Style.Font.Language; got != "ja" {
		t.Fatalf("span without a language = %q, want its text's ja", got)
	}
	// Text that sets none follows the app's language as it changes; a set one stays.
	h.tree.SetEnvironment(Environment{Locale: "ko-KR"})
	h.commit()
	if got := language("default"); got != "ko-KR" {
		t.Fatalf("default language after the locale changed = %q, want ko-KR", got)
	}
	if got := language("inherited"); got != "ja" {
		t.Fatalf("inherited language after the locale changed = %q, want ja", got)
	}
	// So does it as the app switches its own at runtime, which marks no node.
	scope.SetLocale("ja-JP")
	h.commit()
	if got := language("default"); got != "ja-JP" {
		t.Fatalf("default language after SetLocale = %q, want ja-JP", got)
	}
}

func TestTextTransformShadowAndDecoration(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{TextTransform: TextTransformUppercase, TextShadowOffset: Point{X: 1, Y: 2}}, Children: []Node{
			Text{TestID: "text", Style: Style{TextDecoration: TextDecorationUnderline, TextDecorationStyle: DecorationWavy,
				TextDecorationColor: RGB(0xff0000)}, Spans: []Span{
				{Content: "hello "},
				{Content: "big world", Style: Style{TextTransform: TextTransformCapitalize}},
			}},
		}}
	})
	h.commit()
	tn := h.find("text").text
	if got, want := tn.text.Text, "HELLO Big World"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := tn.text.Shadow, (driver.TextShadow{Color: Color(0x00000055), Offset: Point{X: 1, Y: 2}}); got != want {
		t.Errorf("shadow = %+v, want %+v", got, want)
	}
	if s := tn.style; s.DecorationStyle != DecorationWavy || s.DecorationColor != RGB(0xff0000) {
		t.Errorf("decoration = %v %v, want wavy red", s.DecorationStyle, s.DecorationColor)
	}
	if tn.text.Ellipsize != EllipsizeTail {
		t.Errorf("ellipsize = %v, want tail by default", tn.text.Ellipsize)
	}
}

func TestSpanCursorReachesTextSpan(t *testing.T) {
	var cursor *Signal[Cursor]
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		cursor = NewSignal(cx, CursorPointer)
		return Text{TestID: "text", Style: Style{Cursor: CursorHelp}, Spans: []Span{
			{Content: "plain "},
			{Content: "link", StyleFunc: func() Style { return Style{Cursor: cursor.Get()} }},
			{Content: " class", Class: "cursor-crosshair"},
		}}
	})
	h.commit()
	cursors := func() []Cursor {
		var out []Cursor
		for _, sp := range h.find("text").text.text.Spans {
			out = append(out, sp.Cursor)
		}
		return out
	}
	// The text's own cursor stays the view's: a span names only its own.
	if got := cursors(); !slices.Equal(got, []Cursor{CursorAuto, CursorPointer, CursorCrosshair}) {
		t.Fatalf("span cursors = %v, want auto, pointer and crosshair", got)
	}
	cursor.Set(CursorAuto)
	h.commit()
	if got := cursors(); !slices.Equal(got, []Cursor{CursorAuto, CursorAuto, CursorCrosshair}) {
		t.Fatalf("span cursors after StyleFunc = %v, want auto, auto and crosshair", got)
	}
}

func TestSpansFuncSendsCursorChanges(t *testing.T) {
	var cursor *Signal[Cursor]
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		cursor = NewSignal(cx, CursorAuto)
		return Text{SpansFunc: func() []Span {
			return []Span{{Content: "see "}, {Content: "docs", Style: Style{Cursor: cursor.Get()}}}
		}}
	})
	sent := func() *driver.TextContent {
		for _, it := range h.commit().Items {
			if it.Kind == driver.ItemText {
				return it.Text
			}
		}
		return nil
	}
	if text := sent(); text == nil || text.Spans[1].Cursor != CursorAuto {
		t.Fatalf("text %+v, want the spans", text)
	}
	cursor.Set(CursorPointer)
	if text := sent(); text == nil || text.Spans[1].Cursor != CursorPointer {
		t.Fatalf("a span's new cursor was not sent: %+v", text)
	}
}

func TestFontVariantAndDecorationLines(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{FontVariant: FontSmallCaps, FontSize: 20, TextUnderlineOffset: Pct(10)}, Children: []Node{
			Text{TestID: "inherited", Content: "abc", Style: Style{TextDecoration: TextDecorationUnderline,
				TextDecorationThickness: Pt(3)}},
			Text{TestID: "own", Content: "123", Style: Style{FontVariant: FontTabularNums}},
			Text{TestID: "normal", Content: "123", Style: Style{FontVariant: FontVariantNormal,
				TextDecoration: TextDecorationLineThrough, TextUnderlineOffset: Pt(0)}},
		}}
	})
	h.commit()
	if s := h.find("inherited").text.style; s.Font.Variant != FontSmallCaps || s.DecorationThickness != 3 ||
		s.UnderlineOffset != Pt(2) {
		t.Errorf("inherited: variant %v, thickness %v, offset %v; want small caps, 3, 2", s.Font.Variant,
			s.DecorationThickness, s.UnderlineOffset)
	}
	if v := h.find("own").text.style.Font.Variant; v != FontTabularNums {
		t.Errorf("own variant = %v, want tabular figures alone", v)
	}
	if s := h.find("normal").text.style; s.Font.Variant != 0 || s.UnderlineOffset != Pt(0) {
		t.Errorf("normal: variant %v, offset %v; want none, and the offset with a line through", s.Font.Variant,
			s.UnderlineOffset)
	}
}

func TestTextFitsItsBox(t *testing.T) {
	var lines []TextLine
	var batch *driver.MountBatch
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			Text{Content: "aaaaaaaaaa", TestID: "text", Style: Style{FontSize: 10, Width: Pt(30), Height: Pt(8)},
				AdjustsFontSizeToFit: true, MinimumFontScale: 0.2, OnTextLayout: func(l []TextLine) { lines = l }},
		}}
	})
	batch = h.commit()
	var text *driver.TextContent
	for _, it := range batch.Items {
		if it.Kind == driver.ItemText {
			text = it.Text
		}
	}
	if text == nil {
		t.Fatal("no text item")
	}
	l := text.Layout.(*ApproxLayout)
	// Ten characters 6 wide at 10 points fit 30 points at a scale of 0.5.
	if len(l.Lines) != 1 || len(lines) != 1 {
		t.Fatalf("lines = %q, reported %+v, want one", l.Lines, lines)
	}
	if w := lines[0].Bounds.Size.Width; w > 30 || w < 28 {
		t.Errorf("line width = %v, want just under 30", w)
	}
	if lines[0].Text != "aaaaaaaaaa" || lines[0].Start != 0 || lines[0].End != 10 {
		t.Errorf("line = %+v, want the whole text", lines[0])
	}
}

func TestTextAlignVertical(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			Text{Content: "a", TestID: "text", Style: Style{FontSize: 10, Height: Pt(40),
				TextAlignVertical: TextAlignVerticalBottom}},
		}}
	})
	b := h.commit()
	for _, it := range b.Items {
		if it.Kind == driver.ItemText {
			if got, want := it.Text.Content, (Bounds{Origin: Point{X: 0, Y: 28}, Size: Size{Width: 6, Height: 12}}); got != want {
				t.Errorf("content = %v, want %v", got, want)
			}
			return
		}
	}
	t.Fatal("no text item")
}

func TestCutToFit(t *testing.T) {
	text := driver.AttributedText{Text: "abcdefghij", Ellipsize: EllipsizeMiddle,
		Spans: []driver.TextSpan{{Start: 0, End: 5}, {Start: 5, End: 10, Link: true}}}
	fits := func(t *driver.AttributedText) bool { return len([]rune(t.Text)) <= 5 }
	got := text.CutToFit(fits)
	if got.Text != "ab…ij" {
		t.Errorf("middle = %q, want ab…ij", got.Text)
	}
	if len(got.Spans) != 2 || !got.Spans[0].Equal(driver.TextSpan{Start: 0, End: 5}) || got.Spans[1].Start != 5 || got.Spans[1].End != 7 {
		t.Errorf("spans = %+v, want the ellipsis in the first", got.Spans)
	}
	text.Ellipsize = EllipsizeHead
	if got := text.CutToFit(fits); got.Text != "…ghij" {
		t.Errorf("head = %q, want …ghij", got.Text)
	}
}

func TestTextDefaultsToFourteenPoints(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		return Text{Content: "a", TestID: "text"}
	})
	h.commit()
	if got := h.find("text").text.style.Font.Size; got != 14 {
		t.Errorf("font size = %v, want 14", got)
	}
}

func TestSpansFuncRestylesTheText(t *testing.T) {
	var color *Signal[Color]
	h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
		color = NewSignal(cx, RGB(0xff0000))
		return Text{SpansFunc: func() []Span {
			return []Span{{Content: "func"}, {Content: " main", Style: Style{Color: color.Get()}}}
		}}
	})
	sent := func() *driver.TextContent {
		for _, it := range h.commit().Items {
			if it.Kind == driver.ItemText {
				return it.Text
			}
		}
		return nil
	}
	if text := sent(); text == nil || text.Text != "func main" || text.Spans[1].Style.Color != RGB(0xff0000) {
		t.Fatalf("text %+v, want the spans", text)
	}
	color.Set(RGB(0x00ff00))
	if text := sent(); text == nil || text.Spans[1].Style.Color != RGB(0x00ff00) {
		t.Fatalf("restyled %+v", text)
	}
	if text := sent(); text != nil {
		t.Fatal("unchanged spans sent the text again")
	}
}

// fontEngine measures text as ApproxTextEngine does, twice as wide once
// a font registered, as a platform's would with the font's glyphs.
type fontEngine struct{ registered *bool }

func (e fontEngine) Prepare(t *driver.AttributedText) driver.PreparedText {
	if *e.registered {
		t.Style.Font.Size *= 2
	}
	return ApproxTextEngine{}.Prepare(t)
}

func (e fontEngine) RegisterFont([]byte) error { *e.registered = true; return nil }

func TestARegisteredFontPreparesTheTextAgain(t *testing.T) {
	registered := false
	var scope *Scope
	h := newHost(t, Size{Width: 400, Height: 400}, nil)
	h.tree = NewTree(h.root, fontEngine{&registered})
	h.tree.Resize(Size{Width: 400, Height: 400}, Geometry{})
	h.tree.Mount(func(cx *Scope) Node {
		scope = cx
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{Text{TestID: "t", Content: "hello"}}}
	})
	h.commit()
	before := h.find("t").frame.Size.Width
	if err := scope.RegisterFont([]byte("font")); err != nil {
		t.Fatal(err)
	}
	h.commit()
	if after := h.find("t").frame.Size.Width; after <= before*1.5 {
		t.Errorf("the text is %v wide after the font registered, %v before: not prepared again", after, before)
	}
}
