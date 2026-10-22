//go:build windows

package windows

import (
	"math"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/oneo-me/nsgo/core"
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
	w := &window{scale: 1}
	r := rect{0, 0, 100, 100}
	if got := w.transformOf(core.Transform{TranslateX: 10, TranslateY: -5}, r); got != translation(10, -5) {
		t.Fatalf("translate = %+v", got)
	}
	m := w.transformOf(core.Transform{ScaleX: 2, ScaleY: 2}, r)
	if x, y := m.apply(0, 0); x != -50 || y != -50 {
		t.Fatalf("scaled corner = %g, %g", x, y)
	}
	// A quarter turn in degrees sends (100, 50) to (50, 100) around the
	// centre.
	m = w.transformOf(core.Transform{Rotate: 90}, r)
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

func TestPixelsCompositeAsPremultipliedBGRA(t *testing.T) {
	if got := premultiply(core.RGBA(0xff0000, 0.5)); got != 0x80800000 {
		t.Fatalf("premultiply = %08x", got)
	}
	if got := colorRef(core.RGB(0x123456)); got != 0x563412 {
		t.Fatalf("colorRef = %06x", got)
	}
	// A transparent source leaves the destination alone, and an opaque one
	// replaces it.
	if got := over(0x00000000, 0xff00ff00); got != 0xff00ff00 {
		t.Fatalf("transparent over = %08x", got)
	}
	if got := over(0xff112233, 0xff445566); got != 0xff112233 {
		t.Fatalf("opaque over = %08x", got)
	}
	// Half black over white rounds each channel toward white.
	if got := over(0x80000000, 0xffffffff); got != 0xff7f7f7f {
		t.Fatalf("half black over white = %08x", got)
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
	if got := w.rect(10, 20, core.Size{Width: 30, Height: 40}); got != (rect{15, 30, 60, 90}) {
		t.Fatalf("rect = %+v", got)
	}
	if got := w.point(15, 30); got != (core.Point{X: 10, Y: 20}) {
		t.Fatalf("point = %+v", got)
	}
}

func testWindow() *window {
	return &window{scale: 1, fonts: fontCache{scale: 1, fonts: map[fontKey]*font{}}}
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
	if !shown || box.Size != (core.Size{Width: 40, Height: 60}) || box.Origin.X <= 0 || box.Origin.Y != 0 {
		t.Fatalf("attachment = %+v, %v; want 40×60 after the first word, at the top of its line", box, shown)
	}
	// It stands on the baseline: its bottom is the font's ascent above the
	// bottom of the line, less the descent.
	f := w.fonts.acquire(fontKey{font: style.Font})
	defer w.fonts.release(f)
	if bottom, want := box.Origin.Y+box.Size.Height, size.Height-core.Points(f.height-f.ascent); bottom != want {
		t.Errorf("attachment bottom = %v, want the baseline %v", bottom, want)
	}
	if _, shown := al.Attachment(1); shown {
		t.Error("a missing attachment is shown")
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
	l := p.Layout(one).(*spanLayout)
	if len(l.backgrounds) != 1 {
		t.Fatalf("%d background boxes on one line, want 1", len(l.backgrounds))
	}
	b := l.backgrounds[0]
	f := w.fonts.acquire(fontKey{font: style.Font})
	defer w.fonts.release(f)
	if b.r.Bottom-b.r.Top != f.height || b.r.Left <= 0 || b.radius != float64(f.height)/2 {
		t.Errorf("box = %+v, want the font's height after the first word, radius half of it", b)
	}
	l.Release()
	// Wrapped after "bbbb ", the first line's box ends at its ink.
	narrow := p.Measure(core.Points(float64(b.r.Left) + float64(one.Width-core.Points(b.r.Left))/2))
	l = p.Layout(narrow).(*spanLayout)
	defer l.Release()
	if len(l.lines) < 2 || len(l.backgrounds) != 2 {
		t.Fatalf("%d lines, %d boxes; want a box on each of 2 lines", len(l.lines), len(l.backgrounds))
	}
	var ink int32
	for _, r := range l.runs[l.lines[0].first:l.lines[0].last] {
		if r.text[0] != ' ' { // spaces are not ink
			ink = max(ink, r.x+r.ends[len(r.ends)-1])
		}
	}
	if b := l.backgrounds[0]; b.r.Right > ink {
		t.Errorf("the first line's box ends at %d, past its last word's end %d", b.r.Right, ink)
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
	l := p.Layout(size).(*spanLayout)
	defer l.Release()
	step := w.px(style.LineHeight)
	if len(l.lines) < 2 {
		t.Fatalf("%v wide text drew %d line(s) at %v, want at least 2", natural.Width, len(l.lines), narrow)
	}
	if want := core.Points(float64(len(l.lines)*int(step)) / w.scale); size.Height != want {
		t.Errorf("measured height %v, want %v for %d lines of %dpx", size.Height, want, len(l.lines), step)
	}
	for i := 1; i < len(l.lines); i++ {
		if l.lines[i].top < l.lines[i-1].bottom {
			t.Errorf("line %d starts at %d, before the last ends at %d", i, l.lines[i].top, l.lines[i-1].bottom)
		}
	}
}

// TestFallbackFontsDrawMissingGlyphs checks that a rune the text's font
// lacks is set in the family of fallbackFonts that has it, so it takes
// that family's width rather than the missing-glyph box's.
func TestFallbackFontsDrawMissingGlyphs(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 32}, Color: core.RGB(0)}
	for _, c := range []struct{ text, family string }{
		{"⌘", "Segoe UI Symbol"},
		{"✓", "Segoe UI Symbol"},
		{"😀", "Segoe UI Symbol"},
		{"中", "Microsoft YaHei UI"},
	} {
		if got := w.fallbackFamily([]rune(c.text)[0]); got != c.family {
			t.Errorf("%q falls back to %q, want %q", c.text, got, c.family)
		}
		p := w.Prepare(&core.AttributedText{Text: c.text, Style: style})
		got := p.Measure(core.Auto).Width
		p.Release()
		fallback := style
		fallback.Font.Family = c.family
		q := w.Prepare(&core.AttributedText{Text: c.text, Style: fallback})
		want := q.Measure(core.Auto).Width
		q.Release()
		if got != want {
			t.Errorf("%q is %v wide, the %s glyph is %v", c.text, got, c.family, want)
		}
	}
}

// TestSingleLineTextTruncatesInsteadOfWrapping checks MaxLines 1 keeps
// the text on one line, ending it with an ellipsis at the box's width.
func TestSingleLineTextTruncatesInsteadOfWrapping(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16}, Color: core.RGB(0)}
	at := &core.AttributedText{Text: "one two three four", Style: style, MaxLines: 1}
	p := w.Prepare(at)
	defer p.Release()
	full := p.Measure(core.Auto)
	one := w.Prepare(&core.AttributedText{Text: "one", Style: style})
	oneSize := one.Measure(core.Auto)
	one.Release()
	two := w.Prepare(&core.AttributedText{Text: "one two", Style: style})
	twoSize := two.Measure(core.Auto)
	two.Release()
	// Wide enough for "one t", too narrow for "one two": word wrapping
	// would end the line after "one".
	narrow := core.Points(float64(twoSize.Width) - 1)
	size := p.Measure(narrow)
	if size.Height != full.Height {
		t.Errorf("height %v at %v, want one line's %v", size.Height, narrow, full.Height)
	}
	l := p.Layout(size).(*spanLayout)
	defer l.Release()
	if len(l.lines) != 1 || len(l.runs) == 0 || string(utf16.Decode(l.runs[len(l.runs)-1].text)) != "…" {
		t.Fatalf("%d line(s) %d runs, want one line ending with an ellipsis", len(l.lines), len(l.runs))
	}
	var visible []rune
	for _, r := range l.runs {
		visible = append(visible, utf16.Decode(r.text)...)
	}
	if got := string(visible); !strings.HasPrefix(got, "one t") || oneSize.Width >= narrow {
		t.Errorf("the line shows %q at %v wide, want it truncated from %q, not wrapped after %q",
			got, narrow, "one two three four", "one")
	}
	last := l.runs[len(l.runs)-1]
	if last.x+last.ends[len(last.ends)-1] > w.px(size.Width) {
		t.Errorf("the ellipsis ends past the %v wide box", size.Width)
	}
}

// TestIndexAtFindsPlainTextOffsets checks that a press on plain text maps
// to the UTF-8 offset under it, as a press on spans does.
func TestIndexAtFindsPlainTextOffsets(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16}, Color: core.RGB(0)}
	p := w.Prepare(&core.AttributedText{Text: "ab cd", Style: style})
	defer p.Release()
	l := p.Layout(p.Measure(core.Auto)).(*spanLayout)
	defer l.Release()
	first := l.runs[0]
	if got := l.indexAt(first.x+1, first.baseline); got != 0 {
		t.Errorf("press at the first character = %d, want 0", got)
	}
	if got := l.indexAt(first.x+first.ends[0]+1, first.baseline); got != 1 {
		t.Errorf("press at the second character = %d, want 1", got)
	}
	for _, r := range l.runs {
		if string(utf16.Decode(r.text)) == "cd" {
			if got := l.indexAt(r.x+1, r.baseline); got != 3 {
				t.Errorf("press at the second word = %d, want 3", got)
			}
			return
		}
	}
	t.Fatal("the second word did not lay out")
}

// TestLineBreaksAndSpacesKeepTheirFont checks that control characters,
// which no font has glyphs for, do not send a line break or a tab to a
// fallback font: only the characters actually drawn need one.
func TestLineBreaksAndSpacesKeepTheirFont(t *testing.T) {
	w := testWindow()
	style := core.TextStyle{Font: core.Font{Family: "Segoe UI", Size: 16}, Color: core.RGB(0)}
	p := w.Prepare(&core.AttributedText{Text: "a\nb\tc", Style: style}).(*spanText)
	defer p.Release()
	if len(p.fonts) != 1 {
		t.Errorf("the text holds %d fonts, want its own one", len(p.fonts))
	}
}
