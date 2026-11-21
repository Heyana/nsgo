//go:build linux && !android

package linux

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCanvasReplaysIntoTheSurface plays a recorded drawing into an image
// surface and reads the pixels back.
func TestCanvasReplaysIntoTheSurface(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	surface := cairoImageSurfaceCreate(0, 40, 40)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	d := &driver.Drawing{}
	d.SetFillColor(driver.RGB(0x00ff00))
	d.FillRect(10, 10, 20, 20)
	(&window{animator: driver.NewAnimator(nil)}).playCanvas(&view{drawing: d}, cr, rect{w: 40, h: 40})
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	if got := pixelAt(data, stride, 20, 20); got[1] != 0xff || got[0] != 0 || got[2] != 0 {
		t.Fatalf("filled pixel = %v, want green", got)
	}
	if got := pixelAt(data, stride, 5, 5); got[3] != 0 {
		t.Fatalf("outside pixel = %v, want transparent", got)
	}
}

// TestImageDrawsItsPixels draws a decoded picture into an image surface.
func TestImageDrawsItsPixels(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	surface := cairoImageSurfaceCreate(0, 20, 20)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	pix := make([]byte, 4*4*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i+2], pix[i+3] = 0xff, 0xff // red and opaque, in premultiplied BGRA
	}
	v := &view{
		image:  driver.ImageConfig{ResizeMode: driver.Stretch},
		source: imageSlot{pic: &picture{width: 4, height: 4, pix: pix}},
	}
	p := &painter{w: &window{}, cr: cr}
	p.drawImage(v, 0, rect{w: 20, h: 20})
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	if got := pixelAt(data, stride, 10, 10); got[2] != 0xff || got[0] != 0 || got[1] != 0 {
		t.Fatalf("picture pixel = %v, want red", got)
	}
}

// halfClear returns w by h pixels whose left half is transparent and right half c, premultiplied as image.RGBA is, as the core copies them.
func halfClear(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := w / 2; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// TestPixelsDrawWithTheirAlphaAndSwapInPlace shows an image view's pixels over blue: the transparent half shows the blue, and a new version replaces the frame with none blank in between.
func TestPixelsDrawWithTheirAlphaAndSwapInPlace(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	surface := cairoImageSurfaceCreate(0, 40, 20)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	w := &window{}
	v := &view{}
	draw := func() {
		cairoSetSourceRGBA(cr, 0, 0, 1, 1)
		cairoPaint(cr)
		(&painter{w: w, cr: cr}).drawImage(v, 0, rect{w: 40, h: 20})
		cairoSurfaceFlush(surface)
	}
	red := halfClear(40, 20, color.RGBA{R: 0xff, A: 0xff})
	if w.setImage(v, &driver.ImageConfig{Source: driver.ImageSource{Pixels: red, Version: 1}, ResizeMode: driver.Stretch}) {
		t.Fatal("pixels asked for a load")
	}
	draw()
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	if got := pixelAt(data, stride, 5, 10); got[0] != 0xff || got[2] != 0 {
		t.Fatalf("transparent half = %v, want the blue under it", got)
	}
	if got := pixelAt(data, stride, 35, 10); got[2] != 0xff || got[0] != 0 {
		t.Fatalf("opaque half = %v, want red", got)
	}
	green := halfClear(40, 20, color.RGBA{G: 0xff, A: 0xff})
	w.setImage(v, &driver.ImageConfig{Source: driver.ImageSource{Pixels: green, Version: 2}, ResizeMode: driver.Stretch})
	if v.source.pic == nil || v.source.load != nil || v.source.fade != 0 {
		t.Fatal("a new version blanked, loaded or faded")
	}
	draw()
	if got := pixelAt(data, stride, 35, 10); got[1] != 0xff || got[2] != 0 {
		t.Fatalf("second frame = %v, want green", got)
	}
}

// TestCanvasDrawsPixelsAtOnce draws pixels on a canvas the first time it plays, with no load to wait for.
func TestCanvasDrawsPixelsAtOnce(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	surface := cairoImageSurfaceCreate(0, 40, 20)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	d := &driver.Drawing{}
	d.DrawImageIn(driver.ImageSource{Pixels: halfClear(4, 2, color.RGBA{R: 0xff, A: 0xff}), Version: 1}, 0, 0, 40, 20)
	(&window{animator: driver.NewAnimator(nil)}).playCanvas(&view{drawing: d}, cr, rect{w: 40, h: 20})
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	if got := pixelAt(data, stride, 27, 10); got[2] != 0xff || got[3] != 0xff {
		t.Fatalf("drawn pixel = %v, want red", got)
	}
	if got := pixelAt(data, stride, 5, 10); got[3] != 0 {
		t.Fatalf("transparent pixel = %v, want transparent", got)
	}
}

// layoutText prepares t with the engine thread's Pango and lays it out at
// width.
func layoutText(t *testing.T, w *window, text *driver.AttributedText, width driver.Points) *textLayout {
	t.Helper()
	p := w.Prepare(text)
	t.Cleanup(p.Release)
	size := p.Measure(width)
	if !width.IsAuto() {
		size.Width = width
	}
	return p.Layout(size).(*textLayout)
}

// drawText draws l into a new image surface of size and returns its
// pixels and stride.
func drawText(t *testing.T, l *textLayout, width, height int) (uintptr, int) {
	t.Helper()
	surface := cairoImageSurfaceCreate(0, int32(width), int32(height))
	cr := cairoCreate(surface)
	l.drawBackgrounds(cr, 0, 0)
	l.draw(cr, 0, 0)
	cairoSurfaceFlush(surface)
	t.Cleanup(func() {
		l.Release()
		cairoDestroy(cr)
		cairoSurfaceDestroy(surface)
	})
	return cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
}

var textStyle = driver.TextStyle{Font: driver.Font{Size: 16}, Color: driver.RGB(0x000000)}

// TestFontSizesAreLogicalPixels measures a text's size in logical pixels, as a view's: Pango's point sizes would grow it by 96/72 at its default resolution. A 20-point M is under an em wide, so four fit in 80 points where they took about 92, and a line is under 30 points high where it was about 31. The span checks a span's size attribute and the canvas its font description.
func TestFontSizesAreLogicalPixels(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	font := driver.Font{Family: "Sans", Size: 20}
	measure := func(text *driver.AttributedText) driver.Size {
		p := w.Prepare(text)
		defer p.Release()
		return p.Measure(driver.Auto)
	}
	if size := measure(&driver.AttributedText{Text: "MMMM", Style: driver.TextStyle{Font: font, Color: driver.RGB(0)}}); size.Width <= 40 || size.Width > 80 || size.Height > 30 {
		t.Errorf("20-point text measured %v; want it in logical pixels", size)
	}
	small := driver.TextStyle{Font: driver.Font{Family: "Sans", Size: 10}, Color: driver.RGB(0)}
	if size := measure(&driver.AttributedText{Text: "xxMMMM", Style: small, Spans: []driver.TextSpan{{Start: 2, End: 6, Style: driver.TextStyle{Font: font, Color: driver.RGB(0)}}}}); size.Width > 80+20 || size.Height > 30 {
		t.Errorf("a 20-point span measured %v; want it in logical pixels", size)
	}

	surface := cairoImageSurfaceCreate(0, 1, 1)
	cr := cairoCreate(surface)
	defer func() { cairoDestroy(cr); cairoSurfaceDestroy(surface) }()
	l := canvasLayout(cr, "MMMM", font)
	defer gObjectUnref(l)
	var width, height int32
	pangoLayoutGetSize(l, &width, &height)
	if w, h := fromPango(width), fromPango(height); w <= 40 || w > 80 || h > 30 {
		t.Errorf("a canvas's 20-point text measured %vx%v; want it in logical pixels", w, h)
	}
}

// TestMeasuredSizesKeepTheirFractions checks that Linux measures text in Pango
// units (pango_layout_get_size) rather than whole points, keeping fractions for
// the core to round up to a device pixel, and that a box of the measured width
// rounded up at any scale lays the text out on the line it measured (#1603).
func TestMeasuredSizesKeepTheirFractions(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	defer w.releaseTextContexts()

	plain := driver.TextStyle{Font: driver.Font{Family: "Sans", Size: 13, Weight: 400}, Color: driver.RGB(0)}
	for _, text := range []string{"Hello world", "1810.HK", "-9.27%", "Reaches the system through"} {
		p := w.Prepare(&driver.AttributedText{Text: text, Style: plain}).(*preparedText)
		natural := p.Measure(driver.Auto)
		var pw, ph int32
		pangoLayoutGetSize(p.layout, &pw, &ph)
		want := driver.Size{Width: fromPango(pw), Height: fromPango(ph)}
		if natural != want {
			t.Errorf("%q: Measure(Auto) = %v, want Pango size %v (%dx%d units)", text, natural, want, pw, ph)
		}
		for _, ratio := range []float64{0, 1, 1.25, 1.5, 2, 3} {
			width := natural.Width
			if ratio > 0 {
				width = driver.Points(math.Ceil(float64(natural.Width)*ratio-1e-3) / ratio)
				if width < natural.Width {
					width += driver.Points(1 / ratio)
				}
			}
			l := p.Layout(driver.Size{Width: width, Height: natural.Height}).(*textLayout)
			if len(l.lines) != 1 {
				t.Errorf("%q, %v wide, laid out %d lines at %v (%vx scale)", text, natural.Width, len(l.lines), width, ratio)
			}
			l.Release()
		}
		p.Release()
	}

	// Fractional line height and letter spacing keep their Pango-unit fractions
	// rather than rounding up to whole points, so at 2x scale the core rounds
	// up to a half-point device pixel.
	frac := driver.TextStyle{
		Font:          driver.Font{Family: "Sans", Size: 13, Weight: 400},
		LineHeight:    20.3,
		LetterSpacing: 1.3,
		Color:         driver.RGB(0),
	}
	p := w.Prepare(&driver.AttributedText{Text: "Fractional metrics", Style: frac}).(*preparedText)
	natural := p.Measure(driver.Auto)
	if math.Abs(float64(natural.Height)-20.3) > 0.01 {
		t.Errorf("fractional line height measured %v, want ~20.3", natural.Height)
	}
	if float64(natural.Height) == math.Ceil(float64(natural.Height)) {
		t.Errorf("fractional line height %v rounded to whole point", natural.Height)
	}
	if float64(natural.Width) == math.Ceil(float64(natural.Width)) {
		t.Errorf("fractional letter-spaced width %v rounded to whole point", natural.Width)
	}
	if l := p.Layout(natural).(*textLayout); len(l.lines) != 1 {
		t.Errorf("fractional text laid out %d lines at its own measured width %v, want 1", len(l.lines), natural.Width)
	} else {
		l.Release()
	}
	p.Release()

	// Verify end-to-end with core.Tree at 2x scale: 20.3pt height rounds up to
	// 20.5pt (41 device pixels), not 21pt (42 device pixels).
	root := core.NewRoot(core.RootOptions{})
	t.Cleanup(root.Dispose)
	tree := core.NewTree(root, w)
	tree.Resize(core.Size{Width: 400, Height: 400}, core.Geometry{})
	tree.SetEnvironment(core.Environment{PixelRatio: 2})
	tree.Mount(func(cx *core.Scope) core.Node {
		return core.View{
			Style: core.Style{AlignItems: core.AlignStart},
			Children: []core.Node{
				core.Text{
					Content: "Fractional metrics",
					Style: core.Style{
						FontFamily:    "Sans",
						FontSize:      13,
						LineHeight:    20.3,
						LetterSpacing: 1.3,
					},
				},
			},
		}
	})
	batch := tree.Commit()
	var textID driver.NodeID
	frames := map[driver.NodeID]core.Bounds{}
	for _, it := range batch.Items {
		switch it.Kind {
		case driver.ItemText:
			textID = it.ID
			t.Cleanup(it.Text.Layout.Release)
		case driver.ItemFrame:
			frames[it.ID] = it.Frame
		}
	}
	got, ok := frames[textID]
	if !ok {
		t.Fatal("no frame committed for fractional text node")
	}
	if math.Abs(float64(got.Size.Height)-20.5) > 1e-3 {
		t.Errorf("at 2x scale, 20.3pt line-height text laid out %v tall; want 20.5pt (41 device pixels)", got.Size.Height)
	}
}

// TestLetterSpacingFitsAtItsMeasuredWidth lays letter-spaced texts out at the width they measured, with no limit and within a wider or narrower one, and expects the lines they measured: Pango leaves a line's outer letter spacing out of its width but counts it when it breaks lines (#489).
func TestLetterSpacingFitsAtItsMeasuredWidth(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	style := func(spacing driver.Points) driver.TextStyle {
		return driver.TextStyle{Font: driver.Font{Family: "Sans", Size: 20}, LetterSpacing: spacing, Color: driver.RGB(0)}
	}
	title := "悦灵辞书 Lexicon"
	for _, c := range []struct {
		name  string
		text  driver.AttributedText
		width driver.Points
	}{
		{"natural", driver.AttributedText{Text: title, Style: style(2)}, driver.Auto},
		{"fractional", driver.AttributedText{Text: title, Style: style(1.3)}, driver.Auto},
		{"wider box", driver.AttributedText{Text: title, Style: style(2)}, 600},
		{"wrapped", driver.AttributedText{Text: "spaced words wrap across several lines here", Style: style(3)}, 120},
		{"spaced first span", driver.AttributedText{Text: title, Style: style(0), Spans: []driver.TextSpan{
			{Start: 0, End: len("悦灵辞书"), Style: style(6)},
		}}, driver.Auto},
		{"spaced last span", driver.AttributedText{Text: title, Style: style(0), Spans: []driver.TextSpan{
			{Start: len(title) - len("Lexicon"), End: len(title), Style: style(4)},
		}}, driver.Auto},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := &window{}
			p := w.Prepare(&c.text)
			defer p.Release()
			at := p.Layout(driver.Size{Width: c.width}).(*textLayout)
			want := len(at.lines)
			size := p.Measure(c.width)
			if !c.width.IsAuto() && size.Width > c.width {
				t.Fatalf("measured %v within %v", size, c.width)
			}
			got := len(p.Layout(size).(*textLayout).lines)
			if got != want {
				t.Fatalf("laid out at its measured %v, %d lines; want %d", size, got, want)
			}
		})
	}
}

// TestSpanBackgroundsBoxEachLine draws a span's background from its
// font's ascent to its descent, a box per line, without its line's
// hanging space.
func TestSpanBackgroundsBoxEachLine(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	text := "a x      x b"
	start, end := 2, 10
	l := layoutText(t, w, &driver.AttributedText{Text: text, Style: textStyle, Spans: []driver.TextSpan{
		{Start: 0, End: start, Style: textStyle},
		{Start: start, End: end, Style: textStyle, Background: driver.TextBackground{Color: driver.RGB(0xff0000), Radius: 4}},
		{Start: end, End: len(text), Style: textStyle},
	}}, driver.Auto)
	if len(l.under) != 1 {
		t.Fatalf("background boxes = %v, want one", l.under)
	}
	font := w.fontExtent(textStyle.Font, uiFontFamily())
	box := l.under[0]
	if !closeTo(box.h, float64(font.Ascent+font.Descent)) || box.r != 4 || box.x <= 0 {
		t.Fatalf("box = %+v, want the font's %v tall, after the first word", box, font)
	}
	data, stride := drawText(t, l, int(l.width)+20, int(l.height)+4)
	// The spaces between the span's words are on its background.
	mid := pixelAt(data, stride, int(box.x+box.w/2), int(box.y+box.h/2))
	if mid[2] != 0xff || mid[1] != 0 || mid[0] != 0 {
		t.Fatalf("background pixel = %v, want red", mid)
	}
	if out := pixelAt(data, stride, int(box.x+box.w)+2, int(box.y+box.h/2)); out[2] == 0xff && out[1] == 0 {
		t.Fatalf("pixel past the span = %v, want no background", out)
	}

	// Wrapped, a span gets a box per line, and the space it ends a line
	// with is not painted.
	text = "aaaa bbbb cccc"
	l = layoutText(t, w, &driver.AttributedText{Text: text, Style: textStyle, Spans: []driver.TextSpan{
		{Start: 0, End: len(text), Style: textStyle, Background: driver.TextBackground{Color: driver.RGB(0x00ff00)}},
	}, MaxLines: 0}, driver.Auto)
	one := l.under[0].w
	l = layoutText(t, w, &driver.AttributedText{Text: text, Style: textStyle, Spans: []driver.TextSpan{
		{Start: 0, End: len(text), Style: textStyle, Background: driver.TextBackground{Color: driver.RGB(0x00ff00)}},
	}}, driver.Points(one*0.45))
	if len(l.under) != 3 {
		t.Fatalf("wrapped boxes = %d, want 3", len(l.under))
	}
	if l.under[1].y <= l.under[0].y || l.under[0].w > one*0.45 {
		t.Fatalf("wrapped boxes = %+v", l.under)
	}
}

// TestStrikethroughDrawsOnSpansAndTexts draws a line through a whole
// text and through a span.
func TestStrikethroughDrawsOnSpansAndTexts(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	// Ink in the middle of the x height: "u" has none there but its
	// stems, which the line through it adds to.
	ink := func(text *driver.AttributedText) int {
		l := layoutText(t, w, text, driver.Auto)
		data, stride := drawText(t, l, int(l.width)+4, int(l.height)+4)
		font := w.fontExtent(textStyle.Font, uiFontFamily())
		y := int(font.Ascent - font.XHeight/2)
		n := 0
		for x := 0; x < int(l.width); x++ {
			for dy := -2; dy <= 2; dy++ {
				if pixelAt(data, stride, x, y+dy)[3] > 0x80 {
					n++
				}
			}
		}
		return n
	}
	struck := textStyle
	struck.Decoration = driver.TextDecorationLineThrough
	plain := ink(&driver.AttributedText{Text: "uuuuuu", Style: textStyle})
	if whole := ink(&driver.AttributedText{Text: "uuuuuu", Style: struck}); whole <= plain {
		t.Errorf("struck text ink = %d, plain %d: no line through it", whole, plain)
	}
	span := ink(&driver.AttributedText{Text: "uuuuuu", Style: textStyle, Spans: []driver.TextSpan{
		{Start: 0, End: 3, Style: textStyle}, {Start: 3, End: 6, Style: struck},
	}})
	if span <= plain {
		t.Errorf("struck span ink = %d, plain %d: no line through it", span, plain)
	}
}

// TestAttachmentsTakeTheirBoxInTheLine gives a placeholder the
// attachment's width and extent, draws nothing for it and reports its
// box.
func TestAttachmentsTakeTheirBoxInTheLine(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	font := w.fontExtent(textStyle.Font, uiFontFamily())
	placeholder := string(driver.AttachmentRune)
	text := "ab " + placeholder + " cd"
	at := driver.TextAttachment{Offset: 3, Size: driver.Size{Width: 30, Height: 40}}
	attributed := func(at driver.TextAttachment, style driver.TextStyle) *driver.AttributedText {
		return &driver.AttributedText{Text: text, Style: style, Attachments: []driver.TextAttachment{at},
			Spans: []driver.TextSpan{{Start: 0, End: 3, Style: style}, {Start: 3, End: 3 + len(placeholder), Style: style},
				{Start: 3 + len(placeholder), End: len(text), Style: style}}}
	}
	plain := layoutText(t, w, &driver.AttributedText{Text: "ab  cd", Style: textStyle}, driver.Auto)
	l := layoutText(t, w, attributed(at, textStyle), driver.Auto)
	box, shown := l.Attachment(0)
	if !shown {
		t.Fatal("the attachment is not shown")
	}
	// On the baseline, 40 tall: the line grows above the font.
	if box.Size != at.Size || box.Origin.Y != 0 || box.Origin.X <= 0 {
		t.Fatalf("attachment box = %v, want 30x40 at the top", box)
	}
	if got, want := l.height, 40+font.Descent; got < want-1 || got > want+1 {
		t.Fatalf("line height = %v, want about %v", got, want)
	}
	if got, want := l.width, plain.width+30; got < want-1 || got > want+1 {
		t.Fatalf("width = %v, want about %v", got, want)
	}
	data, stride := drawText(t, l, int(l.width)+4, int(l.height)+4)
	for y := int(box.Origin.Y); y < int(box.Origin.Y+box.Size.Height); y++ {
		for x := int(box.Origin.X) + 1; x < int(box.Origin.X+box.Size.Width)-1; x++ {
			if pixelAt(data, stride, x, y)[3] != 0 {
				t.Fatalf("the placeholder drew at %d,%d", x, y)
			}
		}
	}

	// Middle: its middle half the x height above the baseline.
	at.Align = driver.VerticalAlignMiddle
	l = layoutText(t, w, attributed(at, textStyle), driver.Auto)
	box, _ = l.Attachment(0)
	baseline := box.Origin.Y + at.Ascent(font)
	if mid := box.Origin.Y + 20; !closeTo(float64(baseline-mid), float64(font.XHeight/2)) {
		t.Fatalf("middle attachment at %v, baseline %v, x height %v", box, baseline, font.XHeight)
	}

	// A fixed line height still grows where an attachment reaches past it.
	fixed := textStyle
	fixed.LineHeight = 20
	at.Align = driver.VerticalAlignBaseline
	l = layoutText(t, w, attributed(at, fixed), driver.Auto)
	if l.height < 40 {
		t.Fatalf("line height = %v with a 40 tall attachment", l.height)
	}
}

// TestAttachmentsPastTheLineLimitAreCut reports an attachment on a line
// the line limit drops as cut off.
func TestAttachmentsPastTheLineLimitAreCut(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	text := "aaaa bbbb cccc " + string(driver.AttachmentRune)
	at := driver.TextAttachment{Offset: len("aaaa bbbb cccc "), Size: driver.Size{Width: 10, Height: 10}}
	one := layoutText(t, w, &driver.AttributedText{Text: "aaaa", Style: textStyle}, driver.Auto).width
	for _, c := range []struct {
		lines int
		shown bool
	}{{0, true}, {2, false}} {
		l := layoutText(t, w, &driver.AttributedText{Text: text, Style: textStyle, MaxLines: c.lines,
			Attachments: []driver.TextAttachment{at}}, one+2)
		box, shown := l.Attachment(0)
		if shown != c.shown {
			t.Errorf("max lines %d: shown = %v, want %v", c.lines, shown, c.shown)
		}
		if shown && box.Origin.Y < l.height/2 {
			t.Errorf("attachment on the last line at %v, text %v tall", box, l.height)
		}
	}
}

// TestTextEllipsizesJustifiesAndReportsLines checks the ellipsis modes,
// clipping, justified lines and the lines OnTextLayout reports.
func TestTextEllipsizesJustifiesAndReportsLines(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit"
	for _, mode := range []driver.EllipsizeMode{driver.EllipsizeTail, driver.EllipsizeHead, driver.EllipsizeMiddle, driver.EllipsizeClip} {
		l := layoutText(t, w, &driver.AttributedText{Text: text, Style: textStyle, MaxLines: 1, Ellipsize: mode}, 120)
		lines := l.TextLines()
		if len(lines) != 1 {
			t.Fatalf("mode %v: %d lines, want 1", mode, len(lines))
		}
		if lines[0].CapHeight <= lines[0].XHeight || lines[0].XHeight <= 0 || lines[0].Ascender <= 0 {
			t.Errorf("mode %v: metrics %+v", mode, lines[0])
		}
		data, stride := drawText(t, l, 160, int(l.height)+2)
		inked := 0
		for x := 122; x < 160; x++ {
			for y := 0; y < int(l.height); y++ {
				if pixelAt(data, stride, x, y)[3] > 0x80 {
					inked++
				}
			}
		}
		if inked > 0 {
			t.Errorf("mode %v: %d pixels drawn past the box", mode, inked)
		}
	}
	words := "aa bb cc dd ee ff gg hh ii jj kk ll mm nn oo pp"
	justified := textStyle
	justified.Align = driver.TextAlignJustify
	l := layoutText(t, w, &driver.AttributedText{Text: words, Style: justified}, 120)
	lines := l.TextLines()
	if len(lines) < 2 || lines[1].Start != lines[0].End {
		t.Fatalf("lines = %+v, want several in a row", lines)
	}
	if w := lines[0].Bounds.Size.Width; w < 118 {
		t.Errorf("justified first line %v wide, want stretched to 120", w)
	}
}

func TestTextLayoutsHitTestTheirCharacters(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	l := layoutText(t, w, &driver.AttributedText{Text: "abc def\nghi", Style: textStyle}, 400)
	lines := l.TextLines()
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	first := lines[0].Bounds
	if c := l.HitTest(driver.Point{X: first.Size.Width + 50, Y: first.Origin.Y + 2}); c.Offset != 7 {
		t.Fatalf("past the first line: %+v, want 7", c)
	}
	if c := l.HitTest(driver.Point{X: -5, Y: first.Size.Height + 4}); c.Offset != 8 {
		t.Fatalf("the second line's start: %+v, want 8", c)
	}
	a, ok := l.CaretRect(driver.Caret{Offset: 1})
	b, _ := l.CaretRect(driver.Caret{Offset: 2})
	if !ok || a.Origin.X <= 0 || b.Origin.X <= a.Origin.X {
		t.Fatalf("carets %v and %v should step right", a, b)
	}
	if rects := l.RangeRects(5, 9, nil); len(rects) != 2 || rects[1].Origin.Y <= rects[0].Origin.Y {
		t.Fatalf("a range across the break covers %v", rects)
	}
	mixed := layoutText(t, w, &driver.AttributedText{Text: "ab אבג cd", Style: textStyle}, 400)
	if r := mixed.RangeRects(1, 5, nil); len(r) != 2 {
		t.Fatalf("a range from b into the Hebrew covers %v, want two boxes", r)
	}
}
