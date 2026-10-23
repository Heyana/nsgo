//go:build windows

package windows

import (
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/imagedecode"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestMatrixComposesAndInverts(t *testing.T) {
	// Points go through the first matrix, then the second.
	m := translation(10, 0).then(scaling(2, 3))
	if x, y := m.apply(1, 1); x != 22 || y != 3 {
		t.Fatalf("apply = %g, %g; want 22, 3", x, y)
	}
	if x, y := m.invert().apply(22, 3); !closeTo(x, 1) || !closeTo(y, 1) {
		t.Fatalf("invert = %g, %g; want 1, 1", x, y)
	}
	if !m.axisAligned() || !closeTo(m.stretch(), math.Sqrt(6)) {
		t.Fatalf("axisAligned %v, stretch %g", m.axisAligned(), m.stretch())
	}
	if b := m.bound(rect{0, 0, 2, 2}); b != (rect{20, 0, 24, 6}) {
		t.Fatalf("bound = %+v", b)
	}
	// A singular matrix inverts to the identity rather than dividing by
	// zero.
	if got := (matrix{}).invert(); got != identity {
		t.Fatalf("singular invert = %+v", got)
	}
}

func TestTransformOfTurnsAroundTheCentre(t *testing.T) {
	r := core.Size{Width: 100, Height: 100}
	if got := transformOf(core.Transform{TranslateX: 10, TranslateY: -5}, r); got != translation(10, -5) {
		t.Fatalf("translate = %+v", got)
	}
	m := transformOf(core.Transform{ScaleX: 2, ScaleY: 2}, r)
	if x, y := m.apply(0, 0); x != -50 || y != -50 {
		t.Fatalf("scaled corner = %g, %g", x, y)
	}
	// A quarter turn in degrees sends (100, 50) to (50, 100) around the
	// centre.
	m = transformOf(core.Transform{Rotate: 90}, r)
	if x, y := m.apply(100, 50); !closeTo(x, 50) || !closeTo(y, 100) {
		t.Fatalf("rotated point = %g, %g", x, y)
	}
}

func TestMixMovesOnlyThePropertiesAsked(t *testing.T) {
	from := core.ViewStyle{Opacity: 0, Background: core.RGB(0xff0000), BorderWidth: 0,
		Transform: core.Transform{TranslateX: 0}, ShadowRadius: 0}
	to := core.ViewStyle{Opacity: 1, Background: core.RGB(0x0000ff), BorderWidth: 10,
		Transform: core.Transform{TranslateX: 20}, ShadowRadius: 4}
	mixed := mix(from, to, 0.5, core.TransitionOpacity|core.TransitionBackground)
	if mixed.Opacity != 0.5 {
		t.Fatalf("opacity = %v", mixed.Opacity)
	}
	if mixed.Background != core.RGBA(0x800080, 1) {
		t.Fatalf("background = %08x", mixed.Background)
	}
	// Everything without its property comes from the target.
	if mixed.BorderWidth != 10 || mixed.Transform.TranslateX != 20 {
		t.Fatalf("untouched fields = %+v", mixed)
	}

	full := mix(from, to, 0.5, allProperties)
	if full.Transform.TranslateX != 10 || full.ShadowRadius != 2 {
		t.Fatalf("full mix = %+v", full)
	}
	// A style that does not change keeps its look.
	same := mix(to, to, 0.5, allProperties)
	if same.Opacity != to.Opacity || same.Background != to.Background || same.BorderWidth != to.BorderWidth ||
		same.Transform.TranslateX != to.Transform.TranslateX {
		t.Fatalf("unchanged = %+v, want %+v", same, to)
	}
}

func TestMixColorFadesThroughTheVisibleColor(t *testing.T) {
	// A transparent start takes the target's hue, so it does not pass
	// through black.
	got := mixColor(core.RGBA(0xff0000, 0), core.RGBA(0x0000ff, 1), 0.5)
	if got != core.RGBA(0x0000ff, 0.5) {
		t.Fatalf("mix = %08x", got)
	}
	if got := mixColor(core.RGB(0x123456), core.RGB(0x123456), 0.3); got != core.RGB(0x123456) {
		t.Fatalf("same colors = %08x", got)
	}
}

func TestKeyNamesMatchOnKeys(t *testing.T) {
	for _, c := range []struct {
		vk   uintptr
		want string
	}{
		{'A', "a"}, {'Z', "z"}, {'0', "0"}, {'9', "9"},
		{0x60, "0"}, {0x69, "9"}, {0x70, "f1"}, {0x87, "f24"},
		{0x1B, "escape"}, {0x0D, "enter"}, {0x20, "space"},
		{0x10, ""}, {0x11, ""}, {0x12, ""},
	} {
		if got := keyName(c.vk); got != c.want {
			t.Errorf("keyName(0x%X) = %q, want %q", c.vk, got, c.want)
		}
	}
}

func TestShortcutLabelsUseWindowsNames(t *testing.T) {
	for _, c := range []struct{ key, want string }{
		{"mod+shift+n", "Ctrl+Shift+N"},
		{"ctrl+alt+delete", "Ctrl+Alt+Del"},
		{"meta+pageup", "Win+PgUp"},
		{"pagedown", "PgDn"},
		{"escape", "Esc"},
		{"f5", "F5"},
		{"a", "A"},
	} {
		if got := shortcutLabel(c.key); got != c.want {
			t.Errorf("shortcutLabel(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestRightToLeftFollowsTheLocale(t *testing.T) {
	for _, c := range []struct {
		locale string
		want   bool
	}{{"ar-SA", true}, {"he-IL", true}, {"en-US", false}, {"zh-CN", false}, {"", false}} {
		if got := rightToLeft(c.locale); got != c.want {
			t.Errorf("rightToLeft(%q) = %v, want %v", c.locale, got, c.want)
		}
	}
}

func TestImageDensityFromTheURI(t *testing.T) {
	for _, c := range []struct {
		uri  string
		want float64
	}{
		{"logo.png", 1},
		{"logo@2x.png", 2},
		{"C:/images/logo@1.5x.png", 1.5},
		{"logo@3X.png", 1}, // the width suffix is lowercase
		{"logo@2.png", 1},
		{"logo@x.png", 1},
		{"logo@-2x.png", 1},
		{"a/b@2x", 2},
	} {
		if got := density(c.uri); got != c.want {
			t.Errorf("density(%q) = %g, want %g", c.uri, got, c.want)
		}
	}
}

func TestImageFitModes(t *testing.T) {
	w := &window{scale: 1}
	s := &imageSlot{src: core.ImageSource{}, pic: &picture{width: 400, height: 200}}
	r := rect{0, 0, 100, 100}
	if src, dst := w.fit(s, core.Stretch, r); src != (rect{0, 0, 400, 200}) || dst != r {
		t.Fatalf("stretch = %+v, %+v", src, dst)
	}
	if _, dst := w.fit(s, core.Contain, r); dst != (rect{0, 25, 100, 75}) {
		t.Fatalf("contain = %+v", dst)
	}
	if src, dst := w.fit(s, core.Cover, r); src != (rect{100, 0, 300, 200}) || dst != r {
		t.Fatalf("cover = %+v, %+v", src, dst)
	}
	// A picture smaller than the box stays its own size, centered.
	small := &imageSlot{src: core.ImageSource{}, pic: &picture{width: 40, height: 20}}
	if _, dst := w.fit(small, core.Center, r); dst != (rect{30, 40, 70, 60}) {
		t.Fatalf("center = %+v", dst)
	}
	small.src.URI = "logo@2x.png"
	if _, dst := w.fit(small, core.Center, r); dst != (rect{40, 45, 60, 55}) {
		t.Fatalf("center @2x = %+v", dst)
	}
	// A declared size wins over the URI's density.
	small.src.Size = core.Size{Width: 80, Height: 40}
	if _, dst := w.fit(small, core.Center, r); dst != (rect{10, 30, 90, 70}) {
		t.Fatalf("center with size = %+v", dst)
	}
}

func TestCenteredRoundsToWholePixels(t *testing.T) {
	if got := centered(rect{0, 0, 100, 50}, 30, 20); got != (rect{35, 15, 65, 35}) {
		t.Fatalf("centered = %+v", got)
	}
	if got := centered(rect{0, 0, 10, 10}, 3, 3); got != (rect{4, 4, 7, 7}) {
		t.Fatalf("odd centered = %+v", got)
	}
}

func TestColorsConvertForDirect2D(t *testing.T) {
	if got := colorOf(core.RGBA(0xff0000, 0.5)); got.r != 1 || got.g != 0 || got.b != 0 || got.a < 0.49 || got.a > 0.51 {
		t.Fatalf("colorOf = %+v", got)
	}
}

func TestHitFindsTheTopmostMatchAndSkipsHiddenViews(t *testing.T) {
	below := &view{id: 2, kind: core.KindView, frame: core.Bounds{Size: core.Size{Width: 50, Height: 50}}}
	above := &view{id: 3, kind: core.KindView, frame: core.Bounds{Origin: core.Point{X: 20}, Size: core.Size{Width: 50, Height: 50}}}
	root := &view{id: 1, kind: core.KindView, frame: core.Bounds{Size: core.Size{Width: 100, Height: 100}}, children: []*view{below, above}}
	all := func(*view) bool { return true }
	if got := hit(root, core.Point{X: 30, Y: 30}, all); got != above {
		t.Fatalf("hit = %p, want the later child %p", got, above)
	}
	above.style.Hidden = true
	if got := hit(root, core.Point{X: 30, Y: 30}, all); got != below {
		t.Fatalf("hidden child = %p, want %p", got, below)
	}
	if got := hit(root, core.Point{X: 300, Y: 300}, all); got != nil {
		t.Fatalf("outside = %+v", got)
	}
	// A scroll view's children move by their offset.
	scroll := &view{id: 4, kind: core.KindScroll, frame: core.Bounds{Size: core.Size{Width: 100, Height: 100}},
		offset: core.Point{Y: 40}, children: []*view{{
			id: 5, kind: core.KindView, frame: core.Bounds{Origin: core.Point{Y: 60}, Size: core.Size{Width: 20, Height: 20}},
		}}}
	if got := hit(scroll, core.Point{X: 10, Y: 30}, all); got == nil || got.id != 5 {
		t.Fatalf("scrolled hit = %+v", got)
	}
}

func TestScrollsAtReturnsTheScrollViewsAroundAPoint(t *testing.T) {
	visible := core.ViewStyle{Opacity: 1}
	inner := &view{id: 3, kind: core.KindScroll, style: visible, frame: core.Bounds{Origin: core.Point{X: 10, Y: 10}, Size: core.Size{Width: 50, Height: 50}}}
	outer := &view{id: 2, kind: core.KindScroll, style: visible, frame: core.Bounds{Size: core.Size{Width: 100, Height: 100}}, children: []*view{inner}}
	root := &view{id: 1, kind: core.KindView, style: visible, frame: core.Bounds{Size: core.Size{Width: 100, Height: 100}}, children: []*view{outer}}
	got := scrollsAt(root, core.Point{X: 30, Y: 30}, nil)
	if len(got) != 2 || got[0] != outer || got[1] != inner {
		t.Fatalf("scrollsAt = %v", got)
	}
	if got := scrollsAt(root, core.Point{X: 300}, nil); len(got) != 0 {
		t.Fatalf("outside = %v", got)
	}
}

func TestBackgroundIsTheNearestOpaqueAncestor(t *testing.T) {
	root := &view{id: 1, style: core.ViewStyle{Background: core.RGB(0x112233)}}
	clear := &view{id: 2, parent: root}
	colored := &view{id: 3, parent: clear, style: core.ViewStyle{Background: core.RGB(0x445566)}}
	if got := background(colored); got != core.RGB(0x445566) {
		t.Fatalf("background = %08x", got)
	}
	if got := background(clear); got != core.RGB(0x112233) {
		t.Fatalf("inherited background = %08x", got)
	}
	if got := background(nil); got != core.RGB(0xffffff) {
		t.Fatalf("default background = %08x", got)
	}
}

func TestSurfaceBoundsAddsAncestorsAndScrollOffsets(t *testing.T) {
	root := &view{id: 1, kind: core.KindView, frame: core.Bounds{Origin: core.Point{X: 10, Y: 20},
		Size: core.Size{Width: 200, Height: 200}}, offset: core.Point{X: 3, Y: 4}}
	child := &view{id: 2, kind: core.KindView, parent: root, frame: core.Bounds{Origin: core.Point{X: 5, Y: 6},
		Size: core.Size{Width: 30, Height: 40}}}
	w := &window{root: 1, views: []*view{nil, root, child}}
	b, ok := w.surfaceBounds(child)
	if !ok || b != (core.Bounds{Origin: core.Point{X: 12, Y: 22}, Size: core.Size{Width: 30, Height: 40}}) {
		t.Fatalf("surfaceBounds = %+v, %v", b, ok)
	}
	// A view outside the root's tree has no surface bounds.
	if _, ok := w.surfaceBounds(&view{id: 9}); ok {
		t.Fatal("a detached view reported bounds")
	}
}

func TestMenuKeysSkipEditingRolesAndKeepTheFirst(t *testing.T) {
	w := &window{chrome: chrome{keys: map[string]menuRef{}}}
	w.menuKeys([]core.MenuEntry{
		{Role: core.MenuCopy},
		{Label: "Save", Shortcut: "mod+s"},
		{Label: "Save again", Shortcut: "mod+s"},
		{Label: "Disabled", Shortcut: "mod+d", Disabled: true},
		{Label: "Quit", Role: core.MenuQuit},
	})
	if len(w.chrome.keys) != 2 {
		t.Fatalf("keys = %v", w.chrome.keys)
	}
	if ref := w.chrome.keys["mod+s"]; ref.id != 0 {
		t.Fatalf("mod+s = %+v", ref)
	}
	if ref, ok := w.chrome.keys["alt+f4"]; !ok || ref.role != core.MenuQuit {
		t.Fatalf("the role's shortcut = %+v, %v", ref, ok)
	}
	if _, ok := w.chrome.keys["mod+c"]; ok {
		t.Fatal("an editing role bound its shortcut")
	}
}

func TestMenuHelpers(t *testing.T) {
	for _, r := range []core.MenuRole{core.MenuAbout, core.MenuHide, core.MenuHideOthers, core.MenuShowAll} {
		if !unsupported(r) {
			t.Errorf("%v is supported", r)
		}
	}
	if unsupported(core.MenuQuit) || unsupported(core.MenuCopy) {
		t.Fatal("a supported role is unsupported")
	}
	if got := shortcutOf(&core.MenuEntry{Role: core.MenuQuit}); got != "alt+f4" {
		t.Fatalf("quit shortcut = %q", got)
	}
	if got := shortcutOf(&core.MenuEntry{Role: core.MenuQuit, Shortcut: "mod+q"}); got != "mod+q" {
		t.Fatalf("given shortcut = %q", got)
	}
	if boolArg(true) != 1 || boolArg(false) != 0 {
		t.Fatal("boolArg")
	}
	if !repeated(1<<30) || repeated(0) {
		t.Fatal("repeated")
	}
}

func TestPixelConversionsUseTheWindowScale(t *testing.T) {
	w := &window{scale: 1.5}
	if got := w.px(10); got != 15 {
		t.Fatalf("px = %d", got)
	}
	if got := w.point(15, 30); got != (core.Point{X: 10, Y: 20}) {
		t.Fatalf("point = %+v", got)
	}
}

func testWindow() *window {
	return &window{scale: 1}
}

func TestSpanAttachmentsTakeTheirPlaceAndGrowTheLine(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: core.RGB(0)}
	text := "ab " + string(core.AttachmentRune) + " cd"
	at := &core.AttributedText{Text: text, Style: style,
		Spans:       []core.TextSpan{{Start: 0, End: 3, Style: style}, {Start: 3, End: 6, Style: style}, {Start: 6, End: len(text), Style: style}},
		Attachments: []core.TextAttachment{{Offset: 3, Size: core.Size{Width: 40, Height: 60}}}}
	plain := &core.AttributedText{Text: "ab x cd", Style: style}
	pt, pp := w.Prepare(at), w.Prepare(plain)
	defer pt.Release()
	defer pp.Release()
	size, plainSize := pt.Measure(core.Auto), pp.Measure(core.Auto)
	if size.Height < 60 || size.Height <= plainSize.Height {
		t.Fatalf("height with a 60pt attachment = %v, plain %v", size.Height, plainSize.Height)
	}
	l := pt.Layout(size)
	defer l.Release()
	al, ok := l.(core.AttachmentLayout)
	if !ok {
		t.Fatal("the layout reports no attachments")
	}
	box, shown := al.Attachment(0)
	if !shown || box.Size != (core.Size{Width: 40, Height: 60}) || box.Origin.X <= 0 || math.Abs(float64(box.Origin.Y)) > 0.5 {
		t.Fatalf("attachment = %+v, %v; want 40×60 after the first word, at the top of its line", box, shown)
	}
	// It stands on the baseline.
	baseline := l.(*textLayout).lines[0].baseline
	if bottom := float32(box.Origin.Y + box.Size.Height); math.Abs(float64(bottom-baseline)) > 0.01 {
		t.Errorf("attachment bottom = %v, want the baseline %v", bottom, baseline)
	}
	if _, shown := al.Attachment(1); shown {
		t.Error("a missing attachment is shown")
	}
}

// TestAttachmentsGrowLinesOfAFixedHeight checks that a line height does
// not cut an attachment taller than it: its line grows, and the lines
// after it move down.
func TestAttachmentsGrowLinesOfAFixedHeight(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: core.RGB(0), LineHeight: 24}
	text := "ab " + string(core.AttachmentRune) + "\ncd\nef"
	at := &core.AttributedText{Text: text, Style: style,
		Attachments: []core.TextAttachment{{Offset: 3, Size: core.Size{Width: 40, Height: 60}}}}
	p := w.Prepare(at)
	defer p.Release()
	size := p.Measure(core.Auto)
	if want := core.Points(math.Ceil(float64(60 + 24 + 24 + 24 - p.(*preparedText).baseline))); size.Height != want {
		t.Errorf("height = %v, want the attachment's line, grown, and two of 24", size.Height)
	}
	l := p.Layout(size).(*textLayout)
	defer l.Release()
	// Every line moves down by what the first grew above its baseline.
	b := p.(*preparedText).baseline
	if len(l.lines) != 3 || len(l.groups) != 1 || math.Abs(float64(l.groups[0].shift-(60-b))) > 0.01 {
		t.Fatalf("lines %+v, groups %+v; want all moved down by %v", l.lines, l.groups, 60-b)
	}
	if box, _ := l.Attachment(0); box.Origin.Y != 0 {
		t.Errorf("the attachment is at %v, want the top", box.Origin.Y)
	}
}

func TestSpanAttachmentsPastTheLineLimitAreCut(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: core.RGB(0)}
	text := "first line words\n" + string(core.AttachmentRune)
	at := &core.AttributedText{Text: text, Style: style, MaxLines: 1,
		Spans:       []core.TextSpan{{Start: 0, End: len(text), Style: style}},
		Attachments: []core.TextAttachment{{Offset: len(text) - 3, Size: core.Size{Width: 10, Height: 10}}}}
	p := w.Prepare(at)
	defer p.Release()
	l := p.Layout(p.Measure(core.Auto))
	defer l.Release()
	if _, shown := l.(core.AttachmentLayout).Attachment(0); shown {
		t.Error("an attachment on a line past the limit is shown")
	}
}

func TestSpanBackgroundsBoxEachLineWithoutHangingSpaces(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: core.RGB(0)}
	text := "aa bbbb cccc dd"
	bg := core.TextBackground{Color: core.RGB(0xff0000), Radius: 100}
	at := &core.AttributedText{Text: text, Style: style,
		Spans: []core.TextSpan{{Start: 0, End: 3, Style: style}, {Start: 3, End: 13, Style: style, Background: bg},
			{Start: 13, End: len(text), Style: style}}}
	p := w.Prepare(at)
	defer p.Release()
	one := p.Measure(core.Auto)
	l := p.Layout(one).(*textLayout)
	if len(l.backgrounds) != 1 {
		t.Fatalf("%d background boxes on one line, want 1", len(l.backgrounds))
	}
	b := l.backgrounds[0]
	m := metricsOf(style.Font)
	height := (m.ascent + m.descent) * 16
	if math.Abs(float64(b.r.bottom-b.r.top-height)) > 0.01 || b.r.left <= 0 || b.radius != (b.r.bottom-b.r.top)/2 {
		t.Errorf("box = %+v, want the font's height %v after the first word, radius half of it", b, height)
	}
	l.Release()
	// Wrapped after "bbbb ", the first line's box ends at its ink.
	narrow := p.Measure(core.Points(b.r.left + (float32(one.Width)-b.r.left)/2))
	l = p.Layout(narrow).(*textLayout)
	defer l.Release()
	if len(l.lines) < 2 || len(l.backgrounds) != 2 {
		t.Fatalf("%d lines, %d boxes; want a box on each of 2 lines", len(l.lines), len(l.backgrounds))
	}
	first := l.lines[0]
	_, ink := inkWidth(l.layout, first.start, first.end-first.trailing-first.newline)
	if b := l.backgrounds[0]; b.r.right > ink+0.01 {
		t.Errorf("the first line's box ends at %v, past its last word's end %v", b.r.right, ink)
	}
}

// TestPlainTextMeasuresTheLinesItDraws is a table header's case: bold
// text with a custom line height, wrapped at a narrow width. Measure and
// Layout must break it into the same lines, or the row reserves less than
// the text draws.
func TestPlainTextMeasuresTheLinesItDraws(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 13, Weight: core.FontWeightSemibold},
		Color: core.RGB(0), LineHeight: 21.7}
	p := w.Prepare(&core.AttributedText{Text: "Reaches the system through", Style: style})
	defer p.Release()
	natural := p.Measure(core.Auto)
	narrow := core.Points(float64(natural.Width) * 0.55)
	size := p.Measure(narrow)
	l := p.Layout(size).(*textLayout)
	defer l.Release()
	if len(l.lines) < 2 {
		t.Fatalf("%v wide text drew %d line(s) at %v, want at least 2", natural.Width, len(l.lines), narrow)
	}
	if want := core.Points(math.Ceil(float64(len(l.lines)) * 21.7)); size.Height != want {
		t.Errorf("measured height %v, want %v for %d lines of 21.7", size.Height, want, len(l.lines))
	}
	for i := 1; i < len(l.lines); i++ {
		if l.lines[i].top < l.lines[i-1].top+l.lines[i-1].height-0.01 {
			t.Errorf("line %d starts at %v, before the last ends", i, l.lines[i].top)
		}
	}
}

// TestFallbackFontsDrawMissingGlyphs checks that DirectWrite sets a rune
// the text's font lacks in a family that has it, so it takes that
// family's width rather than the missing-glyph box's.
func TestFallbackFontsDrawMissingGlyphs(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 32}, Color: core.RGB(0)}
	for _, c := range []struct{ text, family string }{
		{"😀", "Segoe UI Emoji"},
		{"中", "Microsoft YaHei UI"},
	} {
		p := w.Prepare(&core.AttributedText{Text: c.text, Style: style})
		got := p.Measure(core.Auto).Width
		p.Release()
		fallback := style
		fallback.Font.Family = c.family
		q := w.Prepare(&core.AttributedText{Text: c.text, Style: fallback})
		want := q.Measure(core.Auto).Width
		q.Release()
		if got != want || got == 0 {
			t.Errorf("%q is %v wide, the %s glyph is %v", c.text, got, c.family, want)
		}
	}
}

// TestSingleLineTextTruncatesInsteadOfWrapping checks MaxLines 1 keeps
// the text on one line, trimmed to the box's width.
func TestSingleLineTextTruncatesInsteadOfWrapping(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16}, Color: core.RGB(0)}
	at := &core.AttributedText{Text: "one two three four", Style: style, MaxLines: 1}
	p := w.Prepare(at)
	defer p.Release()
	full := p.Measure(core.Auto)
	two := w.Prepare(&core.AttributedText{Text: "one two", Style: style})
	twoSize := two.Measure(core.Auto)
	two.Release()
	narrow := core.Points(float64(twoSize.Width) - 1)
	size := p.Measure(narrow)
	if size.Height != full.Height || size.Width > narrow {
		t.Errorf("size %v at %v, want one line's height %v within the width", size, narrow, full.Height)
	}
	l := p.Layout(size).(*textLayout)
	defer l.Release()
	if len(l.lines) != 1 {
		t.Fatalf("%d lines, want one", len(l.lines))
	}
	var m textMetrics
	l.layout.call(layoutGetMetrics, uintptr(unsafe.Pointer(&m)))
	if m.lineCount != 1 || m.width > float32(size.Width)+0.5 {
		t.Errorf("the line is %v wide in %d line(s), want it trimmed to %v", m.width, m.lineCount, size.Width)
	}
}

// TestIndexAtFindsPlainTextOffsets checks that a press on plain text maps
// to the UTF-8 offset under it, as a press on spans does.
func TestIndexAtFindsPlainTextOffsets(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16}, Color: core.RGB(0)}
	p := w.Prepare(&core.AttributedText{Text: "ab cd", Style: style})
	defer p.Release()
	l := p.Layout(p.Measure(core.Auto)).(*textLayout)
	defer l.Release()
	y := l.lines[0].baseline - 2
	if got := l.indexAt(1, y); got != 0 {
		t.Errorf("press at the first character = %d, want 0", got)
	}
	_, a := inkWidth(l.layout, 0, 1)
	if got := l.indexAt(a+1, y); got != 1 {
		t.Errorf("press at the second character = %d, want 1", got)
	}
	_, space := inkWidth(l.layout, 0, 3)
	if got := l.indexAt(space+1, y); got != 3 {
		t.Errorf("press at the second word = %d, want 3", got)
	}
	if got := l.indexAt(1, y+100); got != -1 {
		t.Errorf("press below the text = %d, want -1", got)
	}
}

func editModelOf(text string) *editModel {
	m := &editModel{}
	m.setText(text)
	return m
}

func TestEditModelEditsAsNSHs(t *testing.T) {
	eq := func(got, want any) {
		t.Helper()
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	s := "a😀b"
	eq(utf16Len(s), 4)
	eq(utf16ToByte(s, 2), 5)
	eq(utf16ToByte(s, 3), 5)
	eq(utf16ToByte(s, 99), 6)
	// Values keep the caret, at the end or on a boundary.
	m := editModelOf("hello")
	m.setText("hello world")
	eq(m.caret, 11)
	m.selectRange(2, 2)
	m.setText("héllo")
	eq(m.caret, 1)
	// Typing and deleting.
	m = editModelOf("")
	for _, c := range []string{"a", "b", "c"} {
		m.insert(c)
	}
	eq(m.delete(unitChar, false), true)
	eq(m.text, "ab")
	m.moveTo(0, false)
	eq(m.delete(unitChar, false), false)
	eq(m.delete(unitChar, true), true)
	eq(m.text, "b")
	m = editModelOf("one two three")
	m.delete(unitWord, false)
	eq(m.text, "one two ")
	m.moveTo(0, false)
	m.delete(unitWord, true)
	eq(m.text, "two ")
	// Undo merges runs of typing and of deleting.
	m = editModelOf("")
	for _, c := range []string{"h", "i", " ", "y", "o"} {
		m.insert(c)
	}
	m.delete(unitChar, false)
	m.delete(unitChar, false)
	eq(m.undo(), true)
	eq(m.text, "hi yo")
	eq(m.undo(), true)
	eq(m.text, "")
	eq(m.undo(), false)
	eq(m.redo(), true)
	eq(m.redo(), true)
	eq(m.text, "hi ")
	eq(m.redo(), false)
	m = editModelOf("")
	m.insert("a")
	m.moveTo(1, false)
	m.insert("c")
	m.insert("pasted")
	m.undo()
	eq(m.text, "ac")
	// Clusters, but Backspace takes a code point.
	family := "a👨‍👩‍👧b"
	eq(nextCluster(family, 1), len(family)-1)
	eq(prevCluster(family, len(family)-1), 1)
	eq(nextCluster("🇨🇳🇯🇵", 0), 8)
	m = editModelOf("é")
	m.delete(unitChar, false)
	eq(m.text, "e")
	// Words.
	w := "hello, world  foo"
	eq(wordRight(w, 0), 5)
	eq(wordRight(w, 7), 14)
	eq(wordLeft(w, 14), 7)
	eq(wordLeft(w, 5), 0)
	start, end := wordAt(w, 8)
	eq(start, 7)
	eq(end, 12)
	start, end = wordAt("你好world", 0)
	eq(start, 0)
	eq(end, 6)
	// A selection collapses to the end a step goes to.
	m = editModelOf("abcdef")
	m.selectRange(1, 4)
	eq(m.selected(), "bcd")
	m.step(unitChar, false, false)
	eq(m.caret, 1)
	eq(m.anchor, 1)
	// The composition shows at the caret; a mask shows one bullet a
	// character.
	m = editModelOf("ab")
	m.moveTo(1, false)
	m.composition = &imeText{text: "ni", cursor: 1}
	eq(m.display(), "anib")
	eq(m.toDisplay(2, false), uint32(4))
	eq(m.displayCaret(false), uint32(2))
	eq(m.textAt(2, false), 1)
	eq(m.textAt(4, false), 2)
	m = editModelOf("a😀b")
	eq(m.toDisplay(5, true), uint32(2))
	eq(m.textAt(2, true), 5)
	eq(pasted("a\r\nb\r\n"), "a b")
}

func TestInputHelpers(t *testing.T) {
	if s, e, ok := targetClause([]byte{0, 0, 1, 1, 2}); !ok || s != 2 || e != 4 {
		t.Fatalf("clause = %d, %d, %v", s, e, ok)
	}
	if s, e, ok := targetClause([]byte{3, 0}); !ok || s != 0 || e != 1 {
		t.Fatalf("clause = %d, %d, %v", s, e, ok)
	}
	if _, _, ok := targetClause([]byte{0, 2}); ok {
		t.Fatal("a clause without a target")
	}
	if got := revealAxis(0, 100, 150, 30); got != 80 {
		t.Fatalf("reveal below = %v", got)
	}
	if got := revealAxis(100, 100, 50, 30); got != 50 {
		t.Fatalf("reveal above = %v", got)
	}
	if got := revealAxis(40, 100, 50, 30); got != 40 {
		t.Fatalf("reveal shown = %v", got)
	}
	if xAlign(core.TextAlignCenter) != 0.5 || xAlign(core.TextAlignEnd) != 1 || xAlign(0) != 0 {
		t.Fatal("xAlign")
	}
}

// TestFocusFollowsModalsAndTab checks the focus without a window: Tab
// goes around the fields of the newest modal, and a modal takes the focus
// from a field under it until it goes.
func TestFocusFollowsModalsAndTab(t *testing.T) {
	w := &window{root: 1}
	visible := core.ViewStyle{Opacity: 1}
	mk := func(id core.NodeID, kind core.ViewKind, parent *view) *view {
		v := &view{id: id, kind: kind, style: visible, parent: parent}
		if kind == core.KindInput {
			v.field = newTextInput(w, v)
		}
		if parent != nil {
			parent.children = append(parent.children, v)
		}
		for len(w.views) <= int(id) {
			w.views = append(w.views, nil)
		}
		w.views[id] = v
		return v
	}
	root := mk(1, core.KindView, nil)
	a := mk(2, core.KindInput, root)
	b := mk(3, core.KindInput, root)
	if !w.tab(false) || w.focus != a.field || !w.tab(false) || w.focus != b.field || !w.tab(false) || w.focus != a.field {
		t.Fatal("Tab does not go around the fields")
	}
	if !w.tab(true) || w.focus != b.field {
		t.Fatal("Shift+Tab does not go back")
	}
	modal := mk(4, core.KindModal, root)
	c := mk(5, core.KindInput, modal)
	w.checkFocus()
	if w.focus != nil || w.restore != b.field {
		t.Fatalf("the modal left the focus at %v", w.focus)
	}
	if !w.tab(false) || w.focus != c.field || !w.tab(false) || w.focus != c.field {
		t.Fatal("Tab left the modal")
	}
	w.setFocus(nil)
	w.restore = b.field
	modal.style.Hidden = true
	w.checkFocus()
	if w.focus != b.field {
		t.Fatal("the field did not get the focus back after the modal")
	}
}

// TestFileURLsEscapeAndKeepHosts checks the URLs of dropped files.
func TestFileURLsEscapeAndKeepHosts(t *testing.T) {
	for path, want := range map[string]string{
		`C:\a b\c#d.txt`:     "file:///C:/a%20b/c%23d.txt",
		`D:\日本`:              "file:///D:/%E6%97%A5%E6%9C%AC",
		`\\server\share\x y`: "file://server/share/x%20y",
		`relative\x`:         "",
	} {
		if got := fileURL(path); got != want {
			t.Errorf("fileURL(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestPictureFramesFollowTheirDelaysAndLoops checks which frame an
// animation shows when.
func TestPictureFramesFollowTheirDelaysAndLoops(t *testing.T) {
	ms := time.Millisecond
	pic := &picture{frames: []imagedecode.Frame{{Delay: 100 * ms}, {Delay: 50 * ms}}, plays: 2}
	for _, c := range []struct {
		at    time.Duration
		frame int
		more  bool
	}{{0, 0, true}, {99 * ms, 0, true}, {100 * ms, 1, true}, {160 * ms, 0, true}, {260 * ms, 1, true},
		{300 * ms, 1, false}, {time.Hour, 1, false}} {
		if frame, more := pic.frameAt(c.at); frame != c.frame || more != c.more {
			t.Errorf("at %v: frame %d, more %v; want %d, %v", c.at, frame, more, c.frame, c.more)
		}
	}
	pic.plays = 0
	if frame, more := pic.frameAt(time.Hour + 120*ms); frame != 1 || !more {
		t.Errorf("a looping animation shows frame %d (more %v) after an hour", frame, more)
	}
}

// TestLayoutAnimationKeepsRemovedViewsDrawnApartFromTheirIDs applies
// batches without a window: a Delete animation keeps a removed subtree
// drawn under its parent while its IDs serve new views, an Update moves
// a frame from the one shown, and the end destroys what went.
func TestLayoutAnimationKeepsRemovedViewsDrawnApartFromTheirIDs(t *testing.T) {
	w := &window{animator: core.NewAnimator(func(core.Event) {}), drops: map[*view]bool{}, hosts: map[*component]bool{}}
	frame := func(id core.NodeID, x, y, width, height core.Points) core.MountItem {
		return core.MountItem{Kind: core.ItemFrame, ID: id, Frame: core.Bounds{Origin: core.Point{X: x, Y: y},
			Size: core.Size{Width: width, Height: height}}}
	}
	w.apply(&core.MountBatch{Root: 1, Items: []core.MountItem{
		{Kind: core.ItemCreate, ID: 1, View: core.KindView},
		{Kind: core.ItemCreate, ID: 2, View: core.KindView},
		{Kind: core.ItemCreate, ID: 3, View: core.KindView},
		{Kind: core.ItemCreate, ID: 4, View: core.KindView},
		{Kind: core.ItemInsert, ID: 2, Parent: 1},
		{Kind: core.ItemInsert, ID: 3, Parent: 2},
		{Kind: core.ItemInsert, ID: 4, Parent: 1},
		frame(1, 0, 0, 300, 300), frame(2, 0, 0, 100, 100), frame(3, 10, 10, 20, 20), frame(4, 0, 100, 50, 50),
	}})
	root, old, child, mover := w.views[1], w.views[2], w.views[3], w.views[4]
	tr := &core.LayoutTransition{Duration: time.Second}
	w.apply(&core.MountBatch{Root: 1, LayoutAnimation: &core.LayoutAnimation{Update: tr, Delete: tr, Create: tr},
		Items: []core.MountItem{
			{Kind: core.ItemDelete, ID: 2},
			{Kind: core.ItemDelete, ID: 3},
			{Kind: core.ItemCreate, ID: 2, View: core.KindView},
			{Kind: core.ItemInsert, ID: 2, Parent: 1, Before: 4},
			frame(2, 0, 0, 100, 40), frame(4, 0, 40, 50, 50),
		}})
	fresh := w.views[2]
	switch {
	case fresh == old || fresh == nil || w.views[3] != nil:
		t.Fatal("the deleted IDs do not name the new views")
	case len(root.leaving) != 1 || root.leaving[0].v != old || root.leaving[0].at != 0:
		t.Fatalf("the root keeps %v going, want the old view at 0", root.leaving)
	case !old.retired || !child.retired || len(old.children) != 1 || old.children[0] != child:
		t.Fatal("the subtree going is not kept whole")
	case old.fading == nil || !old.fading.out:
		t.Fatal("the old view does not go out")
	case fresh.fading == nil || fresh.fading.out:
		t.Fatal("the new view does not come in")
	case mover.moving == nil || mover.moving.from.Origin.Y != 100 || frameOf(mover).Origin.Y != 100 || mover.frame.Origin.Y != 40:
		t.Fatalf("the moved view moves from %v to %v", frameOf(mover), mover.frame)
	}
	w.finishLeaving(old)
	if len(root.leaving) != 0 || !old.deleted || old.retired || !child.deleted || w.views[2] != fresh {
		t.Fatal("the end of the animation did not destroy only what went")
	}
	// Without an animation a frame snaps, and a delete is immediate.
	w.apply(&core.MountBatch{Root: 1, Items: []core.MountItem{frame(4, 0, 0, 50, 50), {Kind: core.ItemDelete, ID: 2}}})
	if mover.moving != nil || frameOf(mover).Origin.Y != 0 || len(root.leaving) != 0 || !fresh.deleted {
		t.Fatal("a batch without a layout animation animated")
	}
}
