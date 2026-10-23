//go:build linux && !android

package linux

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestKeyNamesMatchOnKeys(t *testing.T) {
	for _, c := range []struct {
		keyval uint32
		shift  bool
		want   string
	}{
		{'a', false, "a"}, {'A', false, "a"}, {'Z', false, "z"}, {'0', false, "0"}, {'9', false, "9"},
		{gdkF1, false, "f1"}, {gdkF1 + 23, false, "f24"},
		{gdkEscape, false, "escape"}, {gdkReturn, false, "enter"}, {gdkSpace, false, "space"},
		{gdkPageUp, false, "pageup"}, {gdkDown, false, "down"},
		{'?', true, "/"}, {'+', true, "="}, {'_', true, "-"},
		{0, false, ""},
	} {
		if got := keyName(c.keyval, c.shift); got != c.want {
			t.Errorf("keyName(0x%X, %v) = %q, want %q", c.keyval, c.shift, got, c.want)
		}
	}
}

func TestUnshiftedFindsTheTypingKey(t *testing.T) {
	if got := unshifted('?'); got != '/' {
		t.Fatalf("? types / on a US layout, got %q", got)
	}
	if got := unshifted('A'); got != 'A' {
		t.Fatalf("a letter stayed %q", got)
	}
}

func TestSessionLocaleIsABCP47Tag(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "de_DE.UTF-8@euro")
	t.Setenv("LANG", "fr_FR.UTF-8")
	if got := sessionLocale(); got != "de-DE" {
		t.Fatalf("locale = %q, want de-DE", got)
	}
}

func TestRightToLeftFollowsTheLanguage(t *testing.T) {
	for _, c := range []struct {
		locale string
		want   bool
	}{{"ar-SA", true}, {"he-IL", true}, {"fa", true}, {"en-US", false}, {"zh-CN", false}, {"", false}} {
		if got := rightToLeft(c.locale); got != c.want {
			t.Errorf("rightToLeft(%q) = %v, want %v", c.locale, got, c.want)
		}
	}
}

func TestPanActivation(t *testing.T) {
	if activates(core.PanActivation{}, core.Point{X: 5, Y: 5}) {
		t.Fatal("a short drag activated a pan")
	}
	if !activates(core.PanActivation{}, core.Point{X: 8, Y: 8}) {
		t.Fatal("a drag past the slop did not activate")
	}
	a := core.PanActivation{ActiveOffsetX: 20, ActiveOffsetY: 40}
	if !activates(a, core.Point{X: 25}) || activates(a, core.Point{X: 10, Y: 10}) {
		t.Fatal("active offsets")
	}
	a.FailOffsetY = 30
	if !fails(a, core.Point{Y: 50}) || fails(a, core.Point{Y: 20}) {
		t.Fatal("fail offsets")
	}
	if rest(core.EventPinch) != 1 || rest(core.EventRotation) != 0 {
		t.Fatal("gesture rest values")
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
	if mixed.BorderWidth != 10 || mixed.Transform.TranslateX != 20 {
		t.Fatalf("untouched fields = %+v", mixed)
	}
	full := mix(from, to, 0.5, allProperties)
	if full.Transform.TranslateX != 10 || full.ShadowRadius != 2 {
		t.Fatalf("full mix = %+v", full)
	}
	same := mix(to, to, 0.5, allProperties)
	if same.Opacity != to.Opacity || same.Background != to.Background || same.BorderWidth != to.BorderWidth ||
		same.Transform.TranslateX != to.Transform.TranslateX {
		t.Fatalf("unchanged = %+v, want %+v", same, to)
	}
}

func TestMixColorFadesThroughTheVisibleColor(t *testing.T) {
	got := mixColor(core.RGBA(0xff0000, 0), core.RGBA(0x0000ff, 1), 0.5)
	if got != core.RGBA(0x0000ff, 0.5) {
		t.Fatalf("mix = %08x", got)
	}
	if got := mixColor(core.RGB(0x123456), core.RGB(0x123456), 0.3); got != core.RGB(0x123456) {
		t.Fatalf("same colors = %08x", got)
	}
}

func TestImageDensityFromTheURI(t *testing.T) {
	for _, c := range []struct {
		uri  string
		want float64
	}{
		{"logo.png", 1}, {"logo@2x.png", 2}, {"/images/logo@1.5x.png", 1.5},
		{"logo@3X.png", 1}, {"logo@2.png", 1}, {"logo@x.png", 1}, {"logo@-2x.png", 1}, {"a/b@2x", 2},
	} {
		if got := density(c.uri); got != c.want {
			t.Errorf("density(%q) = %g, want %g", c.uri, got, c.want)
		}
	}
}

func TestImageFitModes(t *testing.T) {
	w := &window{}
	s := &imageSlot{src: core.ImageSource{}, pic: &picture{width: 400, height: 200}}
	r := rect{x: 5, w: 100, h: 100}
	fit := func(mode core.ResizeMode, pos core.ObjectPosition, s *imageSlot) rect {
		return w.fit(&view{image: core.ImageConfig{ResizeMode: mode, Position: pos}}, s, r)
	}
	if dst := fit(core.Stretch, core.ObjectPosition{}, s); dst != r {
		t.Fatalf("stretch = %+v", dst)
	}
	if dst := fit(core.Contain, core.ObjectPosition{}, s); dst != (rect{x: 5, y: 25, w: 100, h: 50}) {
		t.Fatalf("contain = %+v", dst)
	}
	if dst := fit(core.Cover, core.ObjectPosition{}, s); dst != (rect{x: -45, w: 200, h: 100}) {
		t.Fatalf("cover = %+v", dst)
	}
	if dst := fit(core.Cover, core.ObjectPosition{X: core.PositionEnd}, s); dst != (rect{x: -95, w: 200, h: 100}) {
		t.Fatalf("cover at the right = %+v", dst)
	}
	if dst := fit(core.Center, core.ObjectPosition{}, s); dst != (rect{x: -145, y: -50, w: 400, h: 200}) {
		t.Fatalf("center of a large picture = %+v", dst)
	}
	if dst := fit(core.ScaleDown, core.ObjectPosition{}, s); dst != (rect{x: 5, y: 25, w: 100, h: 50}) {
		t.Fatalf("scale-down of a large picture = %+v", dst)
	}
	small := &imageSlot{src: core.ImageSource{}, pic: &picture{width: 40, height: 20}}
	if dst := fit(core.ScaleDown, core.ObjectPosition{}, small); dst != (rect{x: 35, y: 40, w: 40, h: 20}) {
		t.Fatalf("scale-down = %+v", dst)
	}
	small.src.URI = "logo@2x.png"
	if dst := fit(core.Center, core.ObjectPosition{}, small); dst != (rect{x: 45, y: 45, w: 20, h: 10}) {
		t.Fatalf("center @2x = %+v", dst)
	}
	small.src.Size = core.Size{Width: 80, Height: 40}
	if dst := fit(core.Center, core.ObjectPosition{Y: core.PositionStart}, small); dst != (rect{x: 15, w: 80, h: 40}) {
		t.Fatalf("center with size at the top = %+v", dst)
	}
}

func TestImageFadesIn(t *testing.T) {
	s := &imageSlot{}
	now := time.Now()
	if got := s.alpha(now); got != 1 {
		t.Fatalf("alpha without a fade = %v", got)
	}
	s.fadeAt, s.fade = now, 200*time.Millisecond
	if got := s.alpha(now.Add(50 * time.Millisecond)); got != 0.25 {
		t.Fatalf("alpha a quarter in = %v", got)
	}
	if got := s.alpha(now.Add(time.Second)); got != 1 {
		t.Fatalf("alpha after = %v", got)
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
}

func TestEntryStyleCarriesTheTextStyle(t *testing.T) {
	v := &view{id: 4, input: core.InputConfig{Style: core.TextStyle{
		Font:  core.Font{Family: "Noto Sans", Size: 15, Weight: core.FontWeightBold, Italic: true},
		Color: core.RGB(0x123456), Align: core.TextAlignEnd,
	}, PlaceholderColor: core.RGB(0xff0000), SelectionColor: core.RGB(0x00ff00)}}
	css := entryStyle(v)
	for _, want := range []string{"transparent", "#nsgo-input-4 *", "placeholder", "rgba(255,0,0,1.000)", "selection { background-color: rgba(0,255,0,0.302)", "caret-color: rgba(0,255,0", `font-family: "Noto Sans"`, "font-size: 15.000pt", "font-weight: 700",
		"font-style: italic", "rgba(18,52,86"} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS lacks %q:\n%s", want, css)
		}
	}
}

func TestHistoryKeysStepThroughAPage(t *testing.T) {
	for _, c := range []struct {
		keyval, state uint32
		want          int
	}{
		{gdkLeft, gdkAlt, -1}, {gdkKPRight, gdkAlt, 1}, {gdkBack, 0, -1}, {gdkForward, 0, 1},
		{gdkLeft, 0, 0}, {gdkLeft, gdkAlt | gdkShift, 0}, {gdkBack, gdkControl, 0},
		// Lock keys do not count.
		{gdkLeft, gdkAlt | 1<<1, -1},
	} {
		if got := historyKey(c.keyval, c.state); got != c.want {
			t.Errorf("historyKey(0x%X, 0x%X) = %d, want %d", c.keyval, c.state, got, c.want)
		}
	}
}

func TestMediaErrorsKeepTheFirstLine(t *testing.T) {
	gst := "Error from element /GstPlayBin3:playbin3/GstDecodebin3:decodebin3-0: " +
		"Your GStreamer installation is missing a plug-in.\n" +
		"Your GStreamer installation is missing a plug-in.\n" +
		"../gstdecodebin3.c(3258): mq_slot_check_reconfiguration (): /GstPlayBin3\n" +
		"No suitable plugins found"
	if got := mediaError(gst); !strings.HasPrefix(got, "Your GStreamer installation is missing a plug-in. No") ||
		strings.Contains(got, "\n") {
		t.Errorf("mediaError = %q", got)
	}
	if got := mediaError("Resource not found."); got != "Resource not found." {
		t.Errorf("mediaError = %q", got)
	}
}

func TestVideoFitModes(t *testing.T) {
	for mode, want := range map[core.ResizeMode]int32{
		core.Cover: gtkContentFitCover, core.Contain: gtkContentFitContain,
		core.Stretch: gtkContentFitFill, core.Center: gtkContentFitScaleDown,
	} {
		if got := contentFit(mode); got != want {
			t.Errorf("contentFit(%d) = %d, want %d", mode, got, want)
		}
	}
}
