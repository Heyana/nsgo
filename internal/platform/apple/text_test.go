//go:build darwin

package apple

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestFittedColumnKeepsWrappedTextAboveNextItem(t *testing.T) {
	root := core.NewRoot(core.RootOptions{})
	t.Cleanup(root.Dispose)
	tree := core.NewTree(root, TextEngine{})
	tree.SetEnvironment(core.Environment{PixelRatio: 2})
	tree.Resize(core.Size{Width: 240, Height: 300}, core.Geometry{})

	var text, next core.Ref
	tree.Mount(func(*core.Scope) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignEnd}, Children: []core.Node{
			core.View{Style: core.Style{MaxWidth: core.Pt(62.5)}, Children: []core.Node{
				core.View{Children: []core.Node{
					core.Text{Ref: &text, Content: strings.Repeat("A\u3002\u7532\u4e59\uff0c\u4e19\u4e01", 4) + "\u4e0b\uff1f",
						Style: core.Style{Language: "zh-Hans", FontSize: 14.5, LineHeight: 23.2}},
				}},
			}},
			core.View{Ref: &next, Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}},
		}}
	})
	for _, item := range tree.Commit().Items {
		if item.Kind == driver.ItemText {
			t.Cleanup(item.Text.Layout.(*Layout).Release)
		}
	}
	textBox, textOK := text.Bounds()
	nextBox, nextOK := next.Bounds()
	if !textOK || !nextOK {
		t.Fatal("text and next item should both have bounds")
	}
	if bottom := textBox.Origin.Y + textBox.Size.Height; bottom > nextBox.Origin.Y {
		t.Fatalf("wrapped text ends at %v, next item starts at %v", bottom, nextBox.Origin.Y)
	}
}

func TestAttachmentUsesBaselineOfFallbackText(t *testing.T) {
	root := core.NewRoot(core.RootOptions{})
	t.Cleanup(root.Dispose)
	tree := core.NewTree(root, TextEngine{})
	tree.Resize(core.Size{Width: 300, Height: 150}, core.Geometry{})

	var outer, inner core.Ref
	tree.Mount(func(*core.Scope) core.Node {
		return core.Text{Ref: &outer, Style: core.Style{FontFamily: "Menlo", FontSize: 22}, Spans: []core.Span{
			{Content: "AB "},
			{Attachment: core.View{Children: []core.Node{
				core.Text{Ref: &inner, Content: "漢字"},
			}}},
		}}
	})
	var outside, inside *Layout
	for _, it := range tree.Commit().Items {
		if it.Kind != driver.ItemText {
			continue
		}
		l := it.Text.Layout.(*Layout)
		t.Cleanup(l.Release)
		if it.Text.Text == "漢字" {
			inside = l
		} else {
			outside = l
		}
	}
	if outside == nil || inside == nil {
		t.Fatal("both texts should be mounted")
	}
	fallback := inside.TextLines()[0].Descender
	plain := TextEngine{}.Prepare(&driver.AttributedText{
		Text: "AB", Style: driver.TextStyle{Font: driver.Font{Family: "Menlo", Size: 22}},
	})
	defer plain.Release()
	plainLayout := plain.Layout(core.Size{Width: 300, Height: 30}).(*Layout)
	defer plainLayout.Release()
	if fallback == plainLayout.TextLines()[0].Descender {
		t.Fatalf("CJK fallback has the same descent as Menlo (%v)", fallback)
	}
	_, outerLines, _ := outside.placed()
	_, innerLines, _ := inside.placed()
	outerBox, outerOK := outer.Bounds()
	innerBox, innerOK := inner.Bounds()
	if !outerOK || !innerOK {
		t.Fatal("both texts should have bounds")
	}
	want := float64(outerBox.Origin.Y) + outerLines[0].Y
	got := float64(innerBox.Origin.Y) + innerLines[0].Y
	if got < want-0.01 || got > want+0.01 {
		t.Fatalf("fallback text baseline = %v, surrounding line baseline = %v", got, want)
	}
}

// render draws text laid out at size into a white RGBA bitmap, as a
// flipped view's drawing does.
func render(t *testing.T, text *driver.AttributedText, size driver.Size) (*image.RGBA, *Layout) {
	t.Helper()
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	var (
		colorSpace func() uintptr
		bitmap     func(data unsafe.Pointer, w, h, bits, row, space uintptr, info uint32) uintptr
	)
	purego.RegisterLibFunc(&colorSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&bitmap, cg, "CGBitmapContextCreate")
	w, h := int(size.Width), int(size.Height)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	space := colorSpace()
	defer Release(space)
	const premultipliedLast = 1
	ctx := bitmap(unsafe.Pointer(&img.Pix[0]), uintptr(w), uintptr(h), 8, uintptr(img.Stride), space, premultipliedLast)
	defer Release(ctx)
	p := TextEngine{}.Prepare(text)
	defer p.Release()
	return draw(t, ctx, img, p, size)
}

// renderWith is render of a text prepared already.
func renderWith(t *testing.T, p driver.PreparedText, size driver.Size) (*image.RGBA, *Layout) {
	t.Helper()
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	var (
		colorSpace func() uintptr
		bitmap     func(data unsafe.Pointer, w, h, bits, row, space uintptr, info uint32) uintptr
	)
	purego.RegisterLibFunc(&colorSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&bitmap, cg, "CGBitmapContextCreate")
	w, h := int(size.Width), int(size.Height)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	space := colorSpace()
	defer Release(space)
	const premultipliedLast = 1
	ctx := bitmap(unsafe.Pointer(&img.Pix[0]), uintptr(w), uintptr(h), 8, uintptr(img.Stride), space, premultipliedLast)
	defer Release(ctx)
	return draw(t, ctx, img, p, size)
}

func draw(t *testing.T, ctx uintptr, img *image.RGBA, p driver.PreparedText, size driver.Size) (*image.RGBA, *Layout) {
	t.Helper()
	w, h := int(size.Width), int(size.Height)
	l := p.Layout(size).(*Layout)
	t.Cleanup(l.Release)
	// The bitmap's rows go top down in memory but its space is y up:
	// flip it as a flipped view's is.
	cgContextTranslateCTM(ctx, 0, float64(h))
	cgContextScaleCTM(ctx, 1, -1)
	l.DrawBackgrounds(ctx, Rect(0, 0, 0, 0))
	l.Draw(ctx, float64(h), Rect(0, 0, float64(w), float64(h)), true)
	if dir := os.Getenv("NSGO_TEXT_PNG"); dir != "" {
		f, err := os.Create(dir + "/" + strings.ReplaceAll(t.Name(), "/", "-") + ".png")
		if err == nil {
			png.Encode(f, img)
			f.Close()
		}
	}
	return img, l
}

// inked counts the pixels in r darker than white, and those that are red.
func inked(img *image.RGBA, r image.Rectangle) (dark, red int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R < 200 || c.G < 200 || c.B < 200 {
				dark++
			}
			if c.R > 180 && c.G < 120 && c.B < 120 {
				red++
			}
		}
	}
	return dark, red
}

func TestTextEllipsizeModes(t *testing.T) {
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor"
	for name, mode := range map[string]driver.EllipsizeMode{"tail": driver.EllipsizeTail, "head": driver.EllipsizeHead,
		"middle": driver.EllipsizeMiddle, "clip": driver.EllipsizeClip} {
		t.Run(name, func(t *testing.T) {
			at := driver.AttributedText{Text: text, Style: driver.TextStyle{Font: driver.Font{Size: 14, Weight: 400}, Color: driver.RGB(0)},
				MaxLines: 2, Ellipsize: mode}
			_, l := render(t, &at, driver.Size{Width: 200, Height: 40})
			lines := l.TextLines()
			if len(lines) != 2 {
				t.Fatalf("%d lines, want 2", len(lines))
			}
			if w := lines[1].Bounds.Size.Width; w > 201 {
				t.Errorf("last line %v wide, want within 200", w)
			}
			if lines[1].End != len(text) {
				t.Errorf("last line ends at %d, want the text's end", lines[1].End)
			}
			if lines[0].CapHeight <= 0 || lines[0].XHeight <= 0 || lines[0].Ascender <= 0 {
				t.Errorf("metrics %+v", lines[0])
			}
		})
	}
}

func TestLimitedTextUsesAvailableWidth(t *testing.T) {
	const width driver.Points = 72
	for _, tc := range []struct {
		name, text string
		lines      int
	}{
		{"one line", "Earth Science Weekly", 1},
		{"two lines", "Short line\nA Supercalifragilisticexpialidocious", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := TextEngine{}.Prepare(&driver.AttributedText{Text: tc.text, MaxLines: tc.lines,
				Style: driver.TextStyle{Font: driver.Font{Size: 13}, Color: driver.RGB(0)}})
			defer p.Release()
			size := p.Measure(width)
			fitted := p.Layout(size).(*Layout)
			defer fitted.Release()
			fixed := p.Layout(driver.Size{Width: width, Height: size.Height}).(*Layout)
			defer fixed.Release()
			got, want := fitted.TextLines(), fixed.TextLines()
			if len(got) != tc.lines || len(want) != tc.lines {
				t.Fatalf("got %d lines at measured size %v and %d at width %v, want %d", len(got), size, len(want), width, tc.lines)
			}
			if g, w := got[tc.lines-1].Bounds.Size.Width, want[tc.lines-1].Bounds.Size.Width; g < w-0.5 {
				t.Errorf("last line is %v points wide in the measured %v-point box, want %v as in the %v-point box", g, size.Width, w, width)
			}
		})
	}
}

func TestLimitedTextKeepsWidthAtHardBreak(t *testing.T) {
	p := TextEngine{}.Prepare(&driver.AttributedText{Text: "Short\nline", MaxLines: 1,
		Style: driver.TextStyle{Font: driver.Font{Size: 13}, Color: driver.RGB(0)}})
	defer p.Release()
	if bounded, intrinsic := p.Measure(200), p.Measure(driver.Auto); bounded != intrinsic {
		t.Errorf("text cut at a hard break measures %v with room to spare, want intrinsic size %v", bounded, intrinsic)
	}
}

func TestLimitedTextAlignsEllipsis(t *testing.T) {
	const width driver.Points = 72
	for _, tc := range []struct {
		name  string
		align driver.TextAlign
		rtl   bool
		flush driver.Points
	}{
		{"center", driver.TextAlignCenter, false, 0.5},
		{"end", driver.TextAlignEnd, false, 1},
		{"RTL start", driver.TextAlignStart, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := driver.AttributedText{Text: "Earth Science Weekly", MaxLines: 1, Style: driver.TextStyle{
				Font: driver.Font{Size: 13}, Color: driver.RGB(0), Align: tc.align, RTL: tc.rtl,
			}}
			_, l := render(t, &at, driver.Size{Width: width, Height: 24})
			lines := l.TextLines()
			if len(lines) != 1 {
				t.Fatalf("got %d lines, want 1", len(lines))
			}
			b := lines[0].Bounds
			want := (width - b.Size.Width) * tc.flush
			if b.Size.Width <= 0 || b.Size.Width > width || b.Origin.X < want-0.5 || b.Origin.X > want+0.5 {
				t.Errorf("truncated line at x=%v, width=%v: want x=%v within %v points", b.Origin.X, b.Size.Width, want, width)
			}
		})
	}
}

func TestTextDecorationsAndShadow(t *testing.T) {
	red := driver.RGB(0xff0000)
	at := driver.AttributedText{Text: "Wavy", Style: driver.TextStyle{Font: driver.Font{Size: 20, Weight: 400}, Color: driver.RGB(0x000000),
		Decoration: driver.TextDecorationUnderline, DecorationStyle: driver.DecorationWavy, DecorationColor: red}}
	img, _ := render(t, &at, driver.Size{Width: 80, Height: 30})
	if _, r := inked(img, img.Rect); r < 10 {
		t.Errorf("a wavy red underline drew %d red pixels", r)
	}
	plain := driver.AttributedText{Text: "Shadow", Style: driver.TextStyle{Font: driver.Font{Size: 20, Weight: 400}, Color: driver.RGB(0xffffff)}}
	img, _ = render(t, &plain, driver.Size{Width: 90, Height: 30})
	if d, _ := inked(img, img.Rect); d != 0 {
		t.Fatalf("white text drew %d dark pixels", d)
	}
	plain.Shadow = driver.TextShadow{Color: driver.RGB(0x000000), Offset: driver.Point{X: 2, Y: 2}}
	img, _ = render(t, &plain, driver.Size{Width: 90, Height: 30})
	if d, _ := inked(img, img.Rect); d < 20 {
		t.Errorf("a black shadow drew %d dark pixels", d)
	}
}

func TestTextJustified(t *testing.T) {
	at := driver.AttributedText{Text: "aa bb cc dd ee ff gg hh ii jj kk ll mm nn oo pp", Style: driver.TextStyle{
		Font: driver.Font{Size: 14, Weight: 400}, Color: driver.RGB(0), Align: driver.TextAlignJustify}}
	_, l := render(t, &at, driver.Size{Width: 120, Height: 80})
	lines := l.TextLines()
	if len(lines) < 2 {
		t.Fatalf("%d lines, want several", len(lines))
	}
	if w := lines[0].Bounds.Size.Width; w < 118 {
		t.Errorf("first line %v wide, want stretched to 120", w)
	}
	if lines[1].Start != lines[0].End {
		t.Errorf("lines %+v do not follow each other", lines[:2])
	}
}

func TestLayoutsHitTestTheirCharacters(t *testing.T) {
	style := driver.TextStyle{Font: driver.Font{Size: 20}, Color: driver.RGB(0), Align: driver.TextAlignStart}
	text := &driver.AttributedText{Text: "abc def\nghi", Style: style}
	p := TextEngine{}.Prepare(text)
	defer p.Release()
	size := p.Measure(driver.Auto)
	l := p.Layout(size).(*Layout)
	defer l.Release()
	lines := l.TextLines()
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	first := lines[0].Bounds
	// Past the first line's end: before its break, on that line.
	if c := l.HitTest(driver.Point{X: first.Size.Width + 50, Y: first.Origin.Y + 2}); c.Offset != 7 {
		t.Fatalf("past the first line: %+v, want 7", c)
	}
	if c := l.HitTest(driver.Point{X: -5, Y: first.Size.Height + 4}); c.Offset != 8 {
		t.Fatalf("the second line's start: %+v, want 8", c)
	}
	a, ok := l.CaretRect(driver.Caret{Offset: 1})
	b, _ := l.CaretRect(driver.Caret{Offset: 2})
	if !ok || a.Origin.X <= 0 || b.Origin.X <= a.Origin.X || a.Size.Height < 20 {
		t.Fatalf("carets %v and %v should step right, line high", a, b)
	}
	if c := l.HitTest(driver.Point{X: (a.Origin.X+b.Origin.X)/2 + 1, Y: 2}); c.Offset != 2 {
		t.Fatalf("right of the middle of b: %+v, want 2", c)
	}
	rects := l.RangeRects(5, 9, nil)
	if len(rects) != 2 || rects[1].Origin.Y <= rects[0].Origin.Y || rects[0].Origin.X+rects[0].Size.Width < first.Size.Width-0.5 {
		t.Fatalf("a range across the break covers %v, want the rest of the first line and the second's start", rects)
	}
	// Mixed directions: the Hebrew is drawn right to left after "ab".
	mixed := TextEngine{}.Prepare(&driver.AttributedText{Text: "ab אבג cd", Style: style})
	defer mixed.Release()
	ml := mixed.Layout(mixed.Measure(driver.Auto)).(*Layout)
	defer ml.Release()
	if r := ml.RangeRects(1, 5, nil); len(r) != 2 {
		t.Fatalf("a range from b into the Hebrew covers %v, want two boxes", r)
	}
	// A run's last character ends where the next run starts.
	bold := style
	bold.Font.Weight = 700
	styled := TextEngine{}.Prepare(&driver.AttributedText{Text: "NSGo runs apps", Style: style,
		Spans: []driver.TextSpan{{Start: 0, End: 4, Style: bold}}})
	defer styled.Release()
	sl := styled.Layout(styled.Measure(driver.Auto)).(*Layout)
	defer sl.Release()
	apps, _ := sl.CaretRect(driver.Caret{Offset: 10})
	if r := sl.RangeRects(10, 14, nil); len(r) != 1 || r[0].Origin.X < apps.Origin.X-0.5 {
		t.Fatalf("the range of apps covers %v, want one box from x %v", r, apps.Origin.X)
	}
	empty := TextEngine{}.Prepare(&driver.AttributedText{Style: style})
	defer empty.Release()
	el := empty.Layout(empty.Measure(driver.Auto)).(*Layout)
	if r, ok := el.CaretRect(driver.Caret{}); !ok || r.Size.Height <= 0 || el.HitTest(driver.Point{X: 5, Y: 5}).Offset != 0 {
		t.Fatalf("an empty text's caret %v %v", r, ok)
	}
	el.Release()
	unread := p.Layout(size).(*Layout)
	unread.Release()
	if c := unread.HitTest(driver.Point{X: 10, Y: 2}); c != (driver.Caret{}) {
		t.Fatalf("a layout released unread answered %+v", c)
	}
}

func TestHiddenSpansSkipped(t *testing.T) {
	black := driver.RGB(0x000000)
	red := driver.RGB(0xff0000)
	base := driver.TextStyle{Font: driver.Font{Family: "Menlo", Size: 20, Weight: 400}, Color: black}

	// 1. A hidden span keeps its layout advance while skipping glyphs,
	//    decorations, background and shadow, and visible spans around it
	//    stay at the exact same positions.
	text := "AAA   BBB   CCC"
	shown := &driver.AttributedText{
		Text:  text,
		Style: base,
		Spans: []driver.TextSpan{
			{Start: 0, End: 6, Style: base},
			{Start: 6, End: 9, Style: base},
			{Start: 9, End: len(text), Style: base},
		},
	}
	hiddenStyle := base
	hiddenStyle.Decoration = driver.TextDecorationUnderline | driver.TextDecorationLineThrough
	hiddenStyle.DecorationColor = red
	withHidden := &driver.AttributedText{
		Text:  text,
		Style: base,
		Spans: []driver.TextSpan{
			{Start: 0, End: 6, Style: base},
			{Start: 6, End: 9, Style: hiddenStyle, Hidden: true, Background: driver.TextBackground{Color: red}},
			{Start: 9, End: len(text), Style: base},
		},
	}
	ps := TextEngine{}.Prepare(shown)
	defer ps.Release()
	ph := TextEngine{}.Prepare(withHidden)
	defer ph.Release()
	sizeS, sizeH := ps.Measure(driver.Auto), ph.Measure(driver.Auto)
	if sizeS != sizeH {
		t.Fatalf("measured size changed: visible %v, hidden %v", sizeS, sizeH)
	}
	canvas := driver.Size{Width: 240, Height: 40}
	imgS, layS := renderWith(t, ps, canvas)
	imgH, layH := renderWith(t, ph, canvas)
	caretStartS, _ := layS.CaretRect(driver.Caret{Offset: 6})
	caretEndS, _ := layS.CaretRect(driver.Caret{Offset: 9})
	caretEndH, _ := layH.CaretRect(driver.Caret{Offset: 9})
	if caretEndS.Origin.X != caretEndH.Origin.X {
		t.Fatalf("trailing span x = %v, want %v", caretEndH.Origin.X, caretEndS.Origin.X)
	}
	midRect := image.Rect(int(caretStartS.Origin.X)+1, 0, int(caretEndS.Origin.X)-1, int(canvas.Height))
	if d, _ := inked(imgS, midRect); d < 20 {
		t.Fatalf("control visible middle span drew only %d dark pixels", d)
	}
	if d, r := inked(imgH, midRect); d != 0 || r != 0 {
		t.Errorf("hidden middle span drew %d dark and %d red pixels", d, r)
	}
	// Visible left and right spans still draw.
	leftRect := image.Rect(0, 0, int(caretStartS.Origin.X)-2, int(canvas.Height))
	rightRect := image.Rect(int(caretEndS.Origin.X)+2, 0, int(canvas.Width), int(canvas.Height))
	if d, _ := inked(imgH, leftRect); d < 20 {
		t.Errorf("left visible span drew only %d dark pixels", d)
	}
	if d, _ := inked(imgH, rightRect); d < 20 {
		t.Errorf("right visible span drew only %d dark pixels", d)
	}

	// 2. Hidden color emoji, decorations and shadow draw nothing at all.
	allHidden := &driver.AttributedText{
		Text:   "😀 Hide",
		Style:  base,
		Shadow: driver.TextShadow{Color: black, Offset: driver.Point{X: 3, Y: 3}, Radius: 2},
		Spans: []driver.TextSpan{{
			Start: 0, End: len("😀 Hide"), Hidden: true,
			Style: driver.TextStyle{
				Font:            base.Font,
				Color:           black,
				Decoration:      driver.TextDecorationUnderline | driver.TextDecorationLineThrough,
				DecorationStyle: driver.DecorationWavy,
				DecorationColor: red,
			},
			Background: driver.TextBackground{Color: red},
		}},
	}
	imgAll, _ := render(t, allHidden, driver.Size{Width: 160, Height: 48})
	if d, r := inked(imgAll, imgAll.Rect); d != 0 || r != 0 {
		t.Errorf("all-hidden emoji+deco+shadow drew %d dark and %d red pixels", d, r)
	}

	// 3. Visible span keeps its solid underline and shadow alongside a hidden span.
	underlined := base
	underlined.Decoration = driver.TextDecorationUnderline
	underlined.DecorationColor = red
	mixed := &driver.AttributedText{
		Text:   "Show     Hide",
		Style:  base,
		Shadow: driver.TextShadow{Color: black, Offset: driver.Point{X: 2, Y: 2}},
		Spans: []driver.TextSpan{
			{Start: 0, End: 4, Style: underlined},
			{Start: 4, End: 9, Style: base},
			{Start: 9, End: 13, Style: underlined, Hidden: true},
		},
	}
	imgMix, layMix := render(t, mixed, driver.Size{Width: 220, Height: 48})
	showEnd, _ := layMix.CaretRect(driver.Caret{Offset: 4})
	hideStart, _ := layMix.CaretRect(driver.Caret{Offset: 9})
	if _, r := inked(imgMix, image.Rect(0, 0, int(showEnd.Origin.X)+4, 48)); r < 10 {
		t.Errorf("visible span alongside hidden span drew only %d red underline pixels", r)
	}
	if d, r := inked(imgMix, image.Rect(int(hideStart.Origin.X), 0, 220, 48)); d != 0 || r != 0 {
		t.Errorf("hidden span with underline and shadow drew %d dark and %d red pixels", d, r)
	}
}
