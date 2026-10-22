//go:build windows

package windows

import (
	"math"
	"testing"

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
