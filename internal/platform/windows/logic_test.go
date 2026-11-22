//go:build windows

package windows

import (
	"image"
	"math"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestRedrawAllDrawsTheWholeWindow checks that the frame after redrawAll
// draws every area, even with nothing queued: a window that lost its web
// views must redraw what the composition showed over it.
func TestRedrawAllDrawsTheWholeWindow(t *testing.T) {
	w := &window{}
	w.redrawAll()
	_, _, changedAll, drawAll := w.frameDamage(rect{0, 0, 100, 100}, false)
	if !changedAll || !drawAll {
		t.Fatalf("redrawAll's frame drew parts: changedAll %v, drawAll %v", changedAll, drawAll)
	}
	if w.damage.whole {
		t.Fatal("the whole-window mark survived its frame")
	}
}

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

func TestMixMovesOnlyThePropertiesAsked(t *testing.T) {
	from := driver.ViewStyle{Opacity: 0, Background: driver.RGB(0xff0000), BorderWidth: 0,
		Transform: driver.Transform{TranslateX: 0}, ShadowRadius: 0}
	to := driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x0000ff), BorderWidth: 10,
		Transform: driver.Transform{TranslateX: 20}, ShadowRadius: 4}
	mixed := driver.MixStyles(from, to, 0.5, driver.TransitionOpacity|driver.TransitionBackground)
	if mixed.Opacity != 0.5 {
		t.Fatalf("opacity = %v", mixed.Opacity)
	}
	if mixed.Background != driver.RGBA(0x800080, 1) {
		t.Fatalf("background = %08x", mixed.Background)
	}
	// Everything without its property comes from the target.
	if mixed.BorderWidth != 10 || mixed.Transform.TranslateX != 20 {
		t.Fatalf("untouched fields = %+v", mixed)
	}

	full := driver.MixStyles(from, to, 0.5, allProperties)
	if full.Transform.TranslateX != 10 || full.ShadowRadius != 2 {
		t.Fatalf("full mix = %+v", full)
	}
	// A style that does not change keeps its look.
	same := driver.MixStyles(to, to, 0.5, allProperties)
	if same.Opacity != to.Opacity || same.Background != to.Background || same.BorderWidth != to.BorderWidth ||
		same.Transform.TranslateX != to.Transform.TranslateX {
		t.Fatalf("unchanged = %+v, want %+v", same, to)
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

func TestGlobalHotKey(t *testing.T) {
	for _, test := range []struct {
		key        string
		mods, want uintptr
		ok         bool
	}{
		{"mod+shift+s", modControl | modShift, 'S', true},
		{"meta+alt+f12", modWin | modAlt, 0x7B, true},
		{"ctrl+plus", modControl, 0xBB, true},
		{"pageup", 0, 0x21, true},
		{"unknown+a", 0, 0, false},
		{"shift", 0, 0, false},
	} {
		mods, vk, ok := globalHotKey(test.key)
		if mods != uint32(test.mods) || vk != test.want || ok != test.ok {
			t.Errorf("globalHotKey(%q) = (%#x, %#x, %v), want (%#x, %#x, %v)", test.key, mods, vk, ok, test.mods, test.want, test.ok)
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

func TestBackdropFor(t *testing.T) {
	for _, c := range []struct {
		m    driver.WindowMaterial
		want int32
	}{
		{driver.WindowMaterialNone, backdropNone},
		{driver.WindowMaterialMica, backdropMica},
		{driver.WindowMaterialTabbed, backdropMicaAlt},
		{driver.WindowMaterialAcrylic, backdropAcrylic},
		{driver.WindowMaterialSidebar, backdropMica},
		{driver.WindowMaterialMenu, backdropAcrylic},
		{driver.WindowMaterial(100), backdropMica},
	} {
		if got := backdropFor(c.m); got != c.want {
			t.Errorf("backdropFor(%d) = %d, want %d", c.m, got, c.want)
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
	s := &imageSlot{src: driver.ImageSource{}, pic: &picture{width: 400, height: 200}}
	r := rectF{5, 0, 105, 100}
	fit := func(mode driver.ResizeMode, pos driver.ObjectPosition, s *imageSlot) rectF {
		return w.fit(&view{image: driver.ImageConfig{ResizeMode: mode, Position: pos}}, s, r)
	}
	if dst := fit(driver.Stretch, driver.ObjectPosition{}, s); dst != r {
		t.Fatalf("stretch = %+v", dst)
	}
	if dst := fit(driver.Contain, driver.ObjectPosition{}, s); dst != (rectF{5, 25, 105, 75}) {
		t.Fatalf("contain = %+v", dst)
	}
	if dst := fit(driver.Cover, driver.ObjectPosition{}, s); dst != (rectF{-45, 0, 155, 100}) {
		t.Fatalf("cover = %+v", dst)
	}
	if dst := fit(driver.Cover, driver.ObjectPosition{X: core.PositionStart}, s); dst != (rectF{5, 0, 205, 100}) {
		t.Fatalf("cover at the left = %+v", dst)
	}
	if dst := fit(driver.Center, driver.ObjectPosition{}, s); dst != (rectF{-145, -50, 255, 150}) {
		t.Fatalf("center of a large picture = %+v", dst)
	}
	if dst := fit(driver.ScaleDown, driver.ObjectPosition{}, s); dst != (rectF{5, 25, 105, 75}) {
		t.Fatalf("scale-down of a large picture = %+v", dst)
	}
	// A picture smaller than the box stays its own size.
	small := &imageSlot{src: driver.ImageSource{}, pic: &picture{width: 40, height: 20}}
	if dst := fit(driver.ScaleDown, driver.ObjectPosition{}, small); dst != (rectF{35, 40, 75, 60}) {
		t.Fatalf("scale-down = %+v", dst)
	}
	small.src.URI = "logo@2x.png"
	if dst := fit(driver.Center, driver.ObjectPosition{}, small); dst != (rectF{45, 45, 65, 55}) {
		t.Fatalf("center @2x = %+v", dst)
	}
	// A declared size wins over the URI's density.
	small.src.Size = driver.Size{Width: 80, Height: 40}
	if dst := fit(driver.Center, driver.ObjectPosition{Y: core.PositionEnd}, small); dst != (rectF{15, 60, 95, 100}) {
		t.Fatalf("center with size at the bottom = %+v", dst)
	}
	// Edges fall on whole pixels.
	w.scale = 2
	odd := &imageSlot{pic: &picture{width: 3, height: 3}}
	if dst := fit(driver.Center, driver.ObjectPosition{}, odd); dst != (rectF{53.5, 48.5, 56.5, 51.5}) {
		t.Fatalf("center at 2x = %+v", dst)
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

func TestColorsConvertForDirect2D(t *testing.T) {
	if got := colorOf(driver.RGBA(0xff0000, 0.5)); got.r != 1 || got.g != 0 || got.b != 0 || got.a < 0.49 || got.a > 0.51 {
		t.Fatalf("colorOf = %+v", got)
	}
}

func TestHitFindsTheTopmostMatchAndSkipsHiddenViews(t *testing.T) {
	below := &view{id: 2, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 50, Height: 50}}}
	above := &view{id: 3, kind: driver.KindView, frame: driver.Bounds{Origin: driver.Point{X: 20}, Size: driver.Size{Width: 50, Height: 50}}}
	root := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}, children: []*view{below, above}}
	all := func(*view) bool { return true }
	if got := hit(root, driver.Point{X: 30, Y: 30}, all); got != above {
		t.Fatalf("hit = %p, want the later child %p", got, above)
	}
	above.style.Hidden = true
	if got := hit(root, driver.Point{X: 30, Y: 30}, all); got != below {
		t.Fatalf("hidden child = %p, want %p", got, below)
	}
	if got := hit(root, driver.Point{X: 300, Y: 300}, all); got != nil {
		t.Fatalf("outside = %+v", got)
	}
	// A scroll view's children move by their offset.
	scroll := &view{id: 4, kind: driver.KindScroll, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}},
		offset: driver.Point{Y: 40}, children: []*view{{
			id: 5, kind: driver.KindView, frame: driver.Bounds{Origin: driver.Point{Y: 60}, Size: driver.Size{Width: 20, Height: 20}},
		}}}
	if got := hit(scroll, driver.Point{X: 10, Y: 30}, all); got == nil || got.id != 5 {
		t.Fatalf("scrolled hit = %+v", got)
	}
}

func TestScrollsAtReturnsTheScrollViewsAroundAPoint(t *testing.T) {
	visible := driver.ViewStyle{Opacity: 1}
	inner := &view{id: 3, kind: driver.KindScroll, style: visible, frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 50, Height: 50}}}
	outer := &view{id: 2, kind: driver.KindScroll, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}, children: []*view{inner}}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}, children: []*view{outer}}
	got := scrollsAt(root, driver.Point{X: 30, Y: 30}, nil)
	if len(got) != 2 || got[0] != outer || got[1] != inner {
		t.Fatalf("scrollsAt = %v", got)
	}
	if got := scrollsAt(root, driver.Point{X: 300}, nil); len(got) != 0 {
		t.Fatalf("outside = %v", got)
	}
}

// TestScrollsAtPassesThroughViewsThatTakeNoPointer turns the wheel over
// views that let the pointer through, as a DataTable's body and overlays
// do: the scroll view they cover is still the one under it.
func TestScrollsAtPassesThroughViewsThatTakeNoPointer(t *testing.T) {
	visible := driver.ViewStyle{Opacity: 1}
	area := driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}
	at := driver.Point{X: 30, Y: 30}
	scroll := &view{id: 2, kind: driver.KindScroll, style: visible, frame: area}
	body := &view{id: 3, kind: driver.KindView, style: visible, frame: area,
		props: driver.ViewProps{PointerEvents: driver.PointerEventsNone}}
	overlay := &view{id: 4, kind: driver.KindView, style: visible, frame: area,
		props: driver.ViewProps{PointerEvents: driver.PointerEventsBoxNone}}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: area, children: []*view{scroll, body, overlay}}
	if got := scrollsAt(root, at, nil); len(got) != 1 || got[0] != scroll {
		t.Fatalf("scrollsAt through views that take no pointer = %v, want the scroll view", got)
	}
	// A view that takes the pointer keeps the wheel from what it covers.
	overlay.props.PointerEvents = driver.PointerEventsAuto
	if got := scrollsAt(root, at, nil); len(got) != 0 {
		t.Fatalf("scrollsAt under a view that takes the pointer = %v", got)
	}
}

// TestBarAtPassesThroughViewsThatTakeNoPointer grabs the bar of a scroll
// view under the views that let the pointer through.
func TestBarAtPassesThroughViewsThatTakeNoPointer(t *testing.T) {
	visible := driver.ViewStyle{Opacity: 1}
	area := driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}
	scroll := &view{id: 2, kind: driver.KindScroll, style: visible, frame: area,
		size: driver.Size{Width: 100, Height: 300}}
	body := &view{id: 3, kind: driver.KindView, style: visible, frame: area,
		props: driver.ViewProps{PointerEvents: driver.PointerEventsNone}}
	overlay := &view{id: 4, kind: driver.KindView, style: visible, frame: area,
		props: driver.ViewProps{PointerEvents: driver.PointerEventsBoxNone}}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: area, children: []*view{scroll, body, overlay}}
	at := driver.Point{X: 95, Y: 40}
	if got, local := barAt(root, at); got != scroll || local != at {
		t.Fatalf("barAt = %v at %v, want the scroll view at %v", got, local, at)
	}
	overlay.props.PointerEvents = driver.PointerEventsAuto
	if got, _ := barAt(root, at); got != nil {
		t.Fatalf("barAt under a view that takes the pointer = %v", got)
	}
}

// TestWheelTurnsTheViewsAlongItsAxis turns a vertical wheel over a
// vertical scroll view holding a horizontal one, as a DataTable's is: the
// page scrolls, the row inside it only on a tilt, and a row alone takes a
// vertical wheel too.
func TestWheelTurnsTheViewsAlongItsAxis(t *testing.T) {
	visible := driver.ViewStyle{Opacity: 1}
	vertical := &view{id: 2, kind: driver.KindScroll, style: visible,
		frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}, size: driver.Size{Width: 100, Height: 300}}
	horizontal := &view{id: 3, kind: driver.KindScroll, style: visible, scroll: driver.ScrollConfig{Horizontal: true},
		frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 80, Height: 80}},
		size:  driver.Size{Width: 300, Height: 80}}
	under := []*view{vertical, horizontal}
	if v, to := wheelTurn(under, -1, false); v != vertical || to.Y != 48 || to.X != 0 {
		t.Fatalf("vertical wheel = %v at %v, want the vertical view at Y 48", v, to)
	}
	if v, to := wheelTurn(under, 1, true); v != horizontal || to.X != 48 || to.Y != 0 {
		t.Fatalf("tilt = %v at %v, want the horizontal view at X 48", v, to)
	}
	// A horizontal view takes a vertical wheel where none is vertical.
	if v, to := wheelTurn([]*view{horizontal}, -1, false); v != horizontal || to.X != 48 {
		t.Fatalf("vertical wheel over a row = %v at %v, want the horizontal view at X 48", v, to)
	}
	// At the vertical view's end, the wheel does not go to the row inside.
	vertical.offset.Y = 200
	if v, _ := wheelTurn(under, -1, false); v != nil {
		t.Fatalf("vertical wheel at the end = %v, want none", v)
	}
}

func TestBackgroundIsTheNearestOpaqueAncestor(t *testing.T) {
	root := &view{id: 1, style: driver.ViewStyle{Background: driver.RGB(0x112233)}}
	clear := &view{id: 2, parent: root}
	colored := &view{id: 3, parent: clear, style: driver.ViewStyle{Background: driver.RGB(0x445566)}}
	if got := background(colored); got != driver.RGB(0x445566) {
		t.Fatalf("background = %08x", got)
	}
	if got := background(clear); got != driver.RGB(0x112233) {
		t.Fatalf("inherited background = %08x", got)
	}
	if got := background(nil); got != driver.RGB(0xffffff) {
		t.Fatalf("default background = %08x", got)
	}
}

func TestSurfaceBoundsAddsAncestorsAndScrollOffsets(t *testing.T) {
	root := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 20},
		Size: driver.Size{Width: 200, Height: 200}}, offset: driver.Point{X: 3, Y: 4}}
	child := &view{id: 2, kind: driver.KindView, parent: root, frame: driver.Bounds{Origin: driver.Point{X: 5, Y: 6},
		Size: driver.Size{Width: 30, Height: 40}}}
	w := &window{root: 1, views: []*view{nil, root, child}}
	b, ok := w.surfaceBounds(child)
	if !ok || b != (driver.Bounds{Origin: driver.Point{X: 12, Y: 22}, Size: driver.Size{Width: 30, Height: 40}}) {
		t.Fatalf("surfaceBounds = %+v, %v", b, ok)
	}
	// A view outside the root's tree has no surface bounds.
	if _, ok := w.surfaceBounds(&view{id: 9}); ok {
		t.Fatal("a detached view reported bounds")
	}
}

func TestMenuKeysSkipEditingRolesAndKeepTheFirst(t *testing.T) {
	w := &window{chrome: chrome{keys: map[string]menuRef{}}}
	w.menuKeys([]driver.MenuEntry{
		{Role: driver.MenuCopy},
		{Label: "Save", Shortcut: "mod+s"},
		{Label: "Save again", Shortcut: "mod+s"},
		{Label: "Disabled", Shortcut: "mod+d", Disabled: true},
		{Label: "Quit", Role: driver.MenuQuit},
	})
	if len(w.chrome.keys) != 2 {
		t.Fatalf("keys = %v", w.chrome.keys)
	}
	if ref := w.chrome.keys["mod+s"]; ref.id != 0 {
		t.Fatalf("mod+s = %+v", ref)
	}
	if ref, ok := w.chrome.keys["alt+f4"]; !ok || ref.role != driver.MenuQuit {
		t.Fatalf("the role's shortcut = %+v, %v", ref, ok)
	}
	if _, ok := w.chrome.keys["mod+c"]; ok {
		t.Fatal("an editing role bound its shortcut")
	}
}

func TestMenuShortcutsLeaveFieldsTheirKeys(t *testing.T) {
	w := &window{chrome: chrome{keys: map[string]menuRef{}}}
	w.menuKeys([]driver.MenuEntry{{Label: "Run", Shortcut: "mod+r", NotInText: true}})
	if ref := w.chrome.keys["mod+r"]; !ref.notInText {
		t.Fatalf("mod+r = %+v", ref)
	}
	w.focus = &textInput{}
	if w.menuShortcut("mod+r") {
		t.Fatal("the menu took a focused field's key")
	}
}

// TestMenuKeysOfTheNewEditingRoles checks that Paste and Match Style, which no field handles the key of, takes its shortcut from the menu, Delete leaves its own to the field, and neither has a shortcut of its own off macOS.
func TestMenuKeysOfTheNewEditingRoles(t *testing.T) {
	w := &window{chrome: chrome{keys: map[string]menuRef{}}}
	w.menuKeys([]driver.MenuEntry{{Role: driver.MenuPasteAndMatchStyle}, {Role: driver.MenuDelete},
		{Role: driver.MenuPasteAndMatchStyle, Shortcut: "ctrl+shift+v"}, {Role: driver.MenuDelete, Shortcut: "delete"}})
	if ref, ok := w.chrome.keys["ctrl+shift+v"]; !ok || ref.role != driver.MenuPasteAndMatchStyle {
		t.Errorf("ctrl+shift+v = %+v, %v; want Paste and Match Style", ref, ok)
	}
	if len(w.chrome.keys) != 1 {
		t.Errorf("keys = %+v, want ctrl+shift+v alone", w.chrome.keys)
	}
}

func TestMenuHelpers(t *testing.T) {
	for _, r := range []driver.MenuRole{driver.MenuAbout, driver.MenuHide, driver.MenuHideOthers, driver.MenuShowAll,
		driver.MenuServices, driver.MenuBringAllToFront} {
		if !driver.MenuRoleUnsupported(r, driver.OSWindows, false) {
			t.Errorf("%v is supported", r)
		}
	}
	if driver.MenuRoleUnsupported(driver.MenuQuit, driver.OSWindows, false) || driver.MenuRoleUnsupported(driver.MenuCopy, driver.OSWindows, false) {
		t.Fatal("a supported role is unsupported")
	}
	for _, speaks := range []bool{false, true} {
		speechAs(t, speaks)
		if driver.MenuRoleUnsupported(driver.MenuStartSpeaking, driver.OSWindows, speaks) == speaks || driver.MenuRoleUnsupported(driver.MenuStopSpeaking, driver.OSWindows, speaks) == speaks {
			t.Errorf("with speech %v, the Speech items are unsupported: %v", speaks, !speaks)
		}
	}
	if got := shortcutOf(&driver.MenuEntry{Role: driver.MenuQuit}); got != "alt+f4" {
		t.Fatalf("quit shortcut = %q", got)
	}
	if got := shortcutOf(&driver.MenuEntry{Role: driver.MenuQuit, Shortcut: "mod+q"}); got != "mod+q" {
		t.Fatalf("given shortcut = %q", got)
	}
	if boolArg(true) != 1 || boolArg(false) != 0 {
		t.Fatal("boolArg")
	}
	if !repeated(1<<30) || repeated(0) {
		t.Fatal("repeated")
	}
}

// TestWebEditCommandsCoverTheEditingRoles checks that a page has a command for every role chooseMenu edits text with, Paste and Match Style and those whose keys a field takes, and for nothing else.
func TestWebEditCommandsCoverTheEditingRoles(t *testing.T) {
	for r := range driver.MenuRole(64) {
		if has, edits := webEditCommands[r] != "", r.EditsText() || r == driver.MenuPasteAndMatchStyle; has != edits {
			t.Errorf("role %d has a page command %v, edits text %v", r, has, edits)
		}
	}
}

func TestPixelConversionsUseTheWindowScale(t *testing.T) {
	w := &window{scale: 1.5}
	if got := w.px(10); got != 15 {
		t.Fatalf("px = %d", got)
	}
	if got := w.point(15, 30); got != (driver.Point{X: 10, Y: 20}) {
		t.Fatalf("point = %+v", got)
	}
}

func testWindow() *window {
	return &window{scale: 1}
}

func TestSpanAttachmentsTakeTheirPlaceAndGrowTheLine(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: driver.RGB(0)}
	text := "ab " + string(driver.AttachmentRune) + " cd"
	at := &driver.AttributedText{Text: text, Style: style,
		Spans:       []driver.TextSpan{{Start: 0, End: 3, Style: style}, {Start: 3, End: 6, Style: style}, {Start: 6, End: len(text), Style: style}},
		Attachments: []driver.TextAttachment{{Offset: 3, Size: driver.Size{Width: 40, Height: 60}}}}
	plain := &driver.AttributedText{Text: "ab x cd", Style: style}
	pt, pp := w.Prepare(at), w.Prepare(plain)
	defer pt.Release()
	defer pp.Release()
	size, plainSize := pt.Measure(driver.Auto), pp.Measure(driver.Auto)
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
	if !shown || box.Size != (driver.Size{Width: 40, Height: 60}) || box.Origin.X <= 0 || math.Abs(float64(box.Origin.Y)) > 0.5 {
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
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: driver.RGB(0), LineHeight: 24}
	text := "ab " + string(driver.AttachmentRune) + "\ncd\nef"
	at := &driver.AttributedText{Text: text, Style: style,
		Attachments: []driver.TextAttachment{{Offset: 3, Size: driver.Size{Width: 40, Height: 60}}}}
	p := w.Prepare(at)
	defer p.Release()
	size := p.Measure(driver.Auto)
	want := driver.Points(float64(60 + 24 + 24 + 24 - p.(*preparedText).baseline))
	if math.Abs(float64(size.Height-want)) > 0.01 {
		t.Errorf("height = %v, want the attachment's line, grown, and two of 24 (%v)", size.Height, want)
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

// TestInputLinesTakeTheLineHeight checks that a multiline field lays its lines out at its style's line height from its first layout on, each baseline where a text with the same style puts it, and that the field's lines go back to DirectWrite's own height when the style drops it (#310).
func TestInputLinesTakeTheLineHeight(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: driver.RGB(0), LineHeight: 25.5}
	p := w.Prepare(&driver.AttributedText{Text: "ab", Style: style})
	want := p.(*preparedText).baseline
	p.Release()
	in := &textInput{config: driver.InputConfig{Multiline: true, Style: style}, width: 300}
	defer in.release()
	units := []uint16{'a', 'b', '\n', 'c', 'd', '\n', 'e', 'f'}
	lines := func() []lineMetrics {
		layout := in.makeLayout(units, dwAlignLeading)
		if layout == nil {
			t.Fatal("no layout")
		}
		defer layout.release()
		m := make([]lineMetrics, 8)
		var count uint32
		layout.call(layoutGetLineMetrics, uintptr(unsafe.Pointer(&m[0])), uintptr(len(m)), uintptr(unsafe.Pointer(&count)))
		return m[:count]
	}
	for i := range 2 {
		got := lines()
		if len(got) != 3 {
			t.Fatalf("layout %d: %d lines, want 3", i, len(got))
		}
		for _, l := range got {
			if l.height != 25.5 || math.Abs(float64(l.baseline-want)) > 0.01 {
				t.Errorf("layout %d: a line %v tall with its baseline at %v, want 25.5 and a text's %v", i, l.height, l.baseline, want)
			}
		}
	}
	in.config.Style.LineHeight = 0
	for _, l := range lines() {
		if l.height == 25.5 {
			t.Errorf("without a line height a line is still %v tall", l.height)
		}
	}
}

func TestSpanAttachmentsPastTheLineLimitAreCut(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: driver.RGB(0)}
	text := "first line words\n" + string(driver.AttachmentRune)
	at := &driver.AttributedText{Text: text, Style: style, MaxLines: 1,
		Spans:       []driver.TextSpan{{Start: 0, End: len(text), Style: style}},
		Attachments: []driver.TextAttachment{{Offset: len(text) - 3, Size: driver.Size{Width: 10, Height: 10}}}}
	p := w.Prepare(at)
	defer p.Release()
	l := p.Layout(p.Measure(driver.Auto))
	defer l.Release()
	if _, shown := l.(core.AttachmentLayout).Attachment(0); shown {
		t.Error("an attachment on a line past the limit is shown")
	}
}

func TestSpanBackgroundsBoxEachLineWithoutHangingSpaces(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: driver.RGB(0)}
	text := "aa bbbb cccc dd"
	bg := driver.TextBackground{Color: driver.RGB(0xff0000), Radius: 100}
	at := &driver.AttributedText{Text: text, Style: style,
		Spans: []driver.TextSpan{{Start: 0, End: 3, Style: style}, {Start: 3, End: 13, Style: style, Background: bg},
			{Start: 13, End: len(text), Style: style}}}
	p := w.Prepare(at)
	defer p.Release()
	one := p.Measure(driver.Auto)
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
	narrow := p.Measure(driver.Points(b.r.left + (float32(one.Width)-b.r.left)/2))
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
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 13, Weight: driver.FontWeightSemibold},
		Color: driver.RGB(0), LineHeight: 21.7}
	p := w.Prepare(&driver.AttributedText{Text: "Reaches the system through", Style: style})
	defer p.Release()
	natural := p.Measure(driver.Auto)
	narrow := driver.Points(float64(natural.Width) * 0.55)
	size := p.Measure(narrow)
	l := p.Layout(size).(*textLayout)
	defer l.Release()
	if len(l.lines) < 2 {
		t.Fatalf("%v wide text drew %d line(s) at %v, want at least 2", natural.Width, len(l.lines), narrow)
	}
	want := driver.Points(float64(len(l.lines)) * 21.7)
	if math.Abs(float64(size.Height-want)) > 0.01 {
		t.Errorf("measured height %v, want %v for %d lines of 21.7", size.Height, want, len(l.lines))
	}
	for i := 1; i < len(l.lines); i++ {
		if l.lines[i].top < l.lines[i-1].top+l.lines[i-1].height-0.01 {
			t.Errorf("line %d starts at %v, before the last ends", i, l.lines[i].top)
		}
	}
}

// TestMeasuredWidthsKeepTheirFractions checks that a text measures what DirectWrite does, fractions kept for the core to round up to a device pixel, and that a box of the measured width so rounded, at any scale, draws the text on the line it measured.
func TestMeasuredWidthsKeepTheirFractions(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 13, Weight: 400}, Color: driver.RGB(0)}
	fractional := false
	for _, text := range []string{"Hello world", "1810.HK", "-9.27%", "Reaches the system through"} {
		p := w.Prepare(&driver.AttributedText{Text: text, Style: style})
		natural := p.Measure(driver.Auto)
		fractional = fractional || float64(natural.Width) != math.Ceil(float64(natural.Width))
		for _, ratio := range []float64{0, 1, 1.25, 1.5, 2} {
			width := natural.Width
			if ratio > 0 {
				width = driver.Points(math.Ceil(float64(natural.Width)*ratio-1e-3) / ratio)
			}
			l := p.Layout(driver.Size{Width: width, Height: natural.Height}).(*textLayout)
			if len(l.lines) != 1 {
				t.Errorf("%q, %v wide, drew %d lines %v wide (%v pixels a point)", text, natural.Width, len(l.lines), width, ratio)
			}
			l.Release()
		}
		p.Release()
	}
	if !fractional {
		t.Error("every text measured a whole number of points: the engine rounded before the core")
	}
}

// TestFallbackFontsDrawMissingGlyphs checks that DirectWrite sets a rune
// the text's font lacks in a family that has it, so it takes that
// family's width rather than the missing-glyph box's.
func TestFallbackFontsDrawMissingGlyphs(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 32}, Color: driver.RGB(0)}
	for _, c := range []struct{ text, family string }{
		{"😀", "Segoe UI Emoji"},
		{"中", "Microsoft YaHei UI"},
	} {
		p := w.Prepare(&driver.AttributedText{Text: c.text, Style: style})
		got := p.Measure(driver.Auto).Width
		p.Release()
		fallback := style
		fallback.Font.Family = c.family
		q := w.Prepare(&driver.AttributedText{Text: c.text, Style: fallback})
		want := q.Measure(driver.Auto).Width
		q.Release()
		if got != want || got == 0 {
			t.Errorf("%q is %v wide, the %s glyph is %v", c.text, got, c.family, want)
		}
	}
}

// TestFallbackFontsFollowTheFamilyList sets the Thai letters Courier New lacks in the next family of the list, Tahoma, where DirectWrite's own fallback would pick its Thai face, and still falls back to the system for what no family of the list has.
func TestFallbackFontsFollowTheFamilyList(t *testing.T) {
	w := testWindow()
	width := func(text, family string) driver.Points {
		p := w.Prepare(&driver.AttributedText{Text: text, Style: driver.TextStyle{Font: driver.Font{Family: family, Size: 32}, Color: driver.RGB(0)}})
		defer p.Release()
		return p.Measure(driver.Auto).Width
	}
	for _, c := range []struct{ text, list, family string }{
		{"กกกก", "Courier New, Tahoma", "Tahoma"},
		{"😀", "Courier New, Times New Roman", "Segoe UI Emoji"},
	} {
		got, want := width(c.text, c.list), width(c.text, c.family)
		if got != want || got == 0 {
			t.Errorf("%q in %q is %v wide, the %s glyphs are %v", c.text, c.list, got, c.family, want)
		}
	}
}

// TestSingleLineTextTruncatesInsteadOfWrapping checks MaxLines 1 keeps
// the text on one line, trimmed to the box's width.
func TestSingleLineTextTruncatesInsteadOfWrapping(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16}, Color: driver.RGB(0)}
	at := &driver.AttributedText{Text: "one two three four", Style: style, MaxLines: 1}
	p := w.Prepare(at)
	defer p.Release()
	full := p.Measure(driver.Auto)
	two := w.Prepare(&driver.AttributedText{Text: "one two", Style: style})
	twoSize := two.Measure(driver.Auto)
	two.Release()
	narrow := driver.Points(float64(twoSize.Width) - 1)
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

// TestLimitedTextUsesAvailableWidth checks that text truncated past MaxLines
// measures wide enough for its truncated final line to match laying out at the
// offered width, and that repeated Measure calls reset the temporary height
// limit (#1549).
func TestLimitedTextUsesAvailableWidth(t *testing.T) {
	w := testWindow()
	const width driver.Points = 72
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 13}, Color: driver.RGB(0)}
	for _, tc := range []struct {
		name, text string
		lines      int
		mode       driver.EllipsizeMode
	}{
		{"one line tail", "Earth Science Weekly", 1, driver.EllipsizeTail},
		{"one line middle", "Earth Science Weekly", 1, driver.EllipsizeMiddle},
		{"two lines tail", "Short line\nA Supercalifragilisticexpialidocious", 2, driver.EllipsizeTail},
		{"two lines middle", "Short line\nA Supercalifragilisticexpialidocious", 2, driver.EllipsizeMiddle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := w.Prepare(&driver.AttributedText{Text: tc.text, Style: style, MaxLines: tc.lines, Ellipsize: tc.mode})
			defer p.Release()
			size := p.Measure(width)
			if size.Width > width+0.01 {
				t.Fatalf("measured width %v exceeds %v", size.Width, width)
			}
			fitted := p.Layout(size).(*textLayout)
			defer fitted.Release()
			fixed := p.Layout(driver.Size{Width: width, Height: size.Height}).(*textLayout)
			defer fixed.Release()
			got, want := fitted.TextLines(), fixed.TextLines()
			if len(got) != tc.lines || len(want) != tc.lines {
				t.Fatalf("got %d lines at measured size %v and %d at width %v, want %d", len(got), size, len(want), width, tc.lines)
			}
			if g, ww := got[tc.lines-1].Bounds.Size.Width, want[tc.lines-1].Bounds.Size.Width; g < ww-0.5 {
				t.Errorf("last line is %v points wide in the measured %v-point box, want %v as in the %v-point box", g, size.Width, ww, width)
			}
			// Re-measuring at Auto must not keep the narrower height limit.
			auto := p.Measure(driver.Auto)
			if auto.Width < size.Width-0.5 {
				t.Errorf("auto width %v shrank below bounded width %v", auto.Width, size.Width)
			}
		})
	}
}

// TestLimitedTextKeepsWidthAtHardBreak checks that text cut at an explicit
// newline does not expand to fill unused horizontal room (#1549).
func TestLimitedTextKeepsWidthAtHardBreak(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 13}, Color: driver.RGB(0)}
	p := w.Prepare(&driver.AttributedText{Text: "Short\nline", Style: style, MaxLines: 1})
	defer p.Release()
	if bounded, intrinsic := p.Measure(200), p.Measure(driver.Auto); bounded != intrinsic {
		t.Errorf("text cut at a hard break measures %v with room to spare, want intrinsic size %v", bounded, intrinsic)
	}
}

// TestIndexAtFindsPlainTextOffsets checks that a press on plain text maps
// to the UTF-8 offset under it, as a press on spans does.
func TestIndexAtFindsPlainTextOffsets(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16}, Color: driver.RGB(0)}
	p := w.Prepare(&driver.AttributedText{Text: "ab cd", Style: style})
	defer p.Release()
	l := p.Layout(p.Measure(driver.Auto)).(*textLayout)
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

// TestTextEllipsizesJustifiesAndReportsLines checks the head and middle
// cuts, clipping, justified lines, decoration styles and the lines
// OnTextLayout reports.
func TestTextEllipsizesJustifiesAndReportsLines(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16}, Color: driver.RGB(0)}
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit"
	for _, mode := range []driver.EllipsizeMode{driver.EllipsizeHead, driver.EllipsizeMiddle} {
		p := w.Prepare(&driver.AttributedText{Text: text, Style: style, MaxLines: 1, Ellipsize: mode})
		size := p.Measure(120)
		l := p.Layout(size).(*textLayout)
		lines := l.TextLines()
		if len(lines) != 1 || lines[0].Bounds.Size.Width > 120.5 {
			t.Errorf("mode %v: lines %+v, want one within 120", mode, lines)
		}
		var m textMetrics
		l.layout.call(layoutGetMetrics, uintptr(unsafe.Pointer(&m)))
		if m.width > 120.5 {
			t.Errorf("mode %v: the cut line is %v wide", mode, m.width)
		}
		l.Release()
		p.Release()
	}
	p := w.Prepare(&driver.AttributedText{Text: text, Style: style, MaxLines: 1, Ellipsize: driver.EllipsizeClip})
	l := p.Layout(p.Measure(120)).(*textLayout)
	if l.clip.empty() || l.clip.right != 120 {
		t.Errorf("clip = %v, want the box", l.clip)
	}
	l.Release()
	p.Release()

	justified := style
	justified.Align = driver.TextAlignJustify
	justified.Decoration, justified.DecorationStyle, justified.DecorationColor =
		driver.TextDecorationUnderline, driver.DecorationWavy, driver.RGB(0xff0000)
	p = w.Prepare(&driver.AttributedText{Text: "aa bb cc dd ee ff gg hh ii jj kk ll mm nn oo pp", Style: justified})
	defer p.Release()
	l = p.Layout(driver.Size{Width: 120, Height: p.Measure(120).Height}).(*textLayout)
	defer l.Release()
	lines := l.TextLines()
	if len(lines) < 2 || lines[1].Start != lines[0].End {
		t.Fatalf("lines = %+v, want several in a row", lines)
	}
	if w := lines[0].Bounds.Size.Width; w < 118 {
		t.Errorf("the justified first line is %v wide, want 120", w)
	}
	if lines[0].CapHeight <= lines[0].XHeight || lines[0].XHeight <= 0 {
		t.Errorf("metrics %+v", lines[0])
	}
	if len(l.decorations) == 0 || l.decorations[0].style != driver.DecorationWavy || l.decorations[0].color != driver.RGB(0xff0000) {
		t.Errorf("decorations = %+v, want red and wavy", l.decorations)
	}
}

// speechAs has the backend say speech is available, or not, for the rest of the test.
func speechAs(t *testing.T, ok bool) {
	t.Helper()
	was := speakAvailable
	speakAvailable = func() bool { return ok }
	t.Cleanup(func() { speakAvailable = was })
}

// TestStartSpeakingReadsTheSelection checks what Start Speaking reads of a field: its selection, or its whole text without one, and nothing of a password field or of none.
func TestStartSpeakingReadsTheSelection(t *testing.T) {
	in := &textInput{model: *editModelOf("日本語のテキスト")}
	if got := in.speechText(); got != "日本語のテキスト" {
		t.Errorf("without a selection it reads %q", got)
	}
	in.model.anchor, in.model.caret = len(in.model.text), len("日本語の")
	if got := in.speechText(); got != "テキスト" {
		t.Errorf("a selection made backwards reads %q", got)
	}
	in.config.Masked = true
	if got := in.speechText(); got != "" {
		t.Errorf("a password field reads %q", got)
	}
	in.config.Multiline = true
	if got := in.speechText(); got != "テキスト" {
		t.Errorf("a masked multiline field, which shows its text, reads %q", got)
	}
	if got := (*textInput)(nil).speechText(); got != "" {
		t.Errorf("no field reads %q", got)
	}
}

func editModelOf(text string) *editModel {
	m := &editModel{}
	m.setText(text)
	return m
}

func TestEditModelEdits(t *testing.T) {
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
	if xAlign(driver.TextAlignCenter, false) != 0.5 || xAlign(driver.TextAlignEnd, false) != 1 || xAlign(0, false) != 0 ||
		xAlign(driver.TextAlignCenter, true) != 0.5 || xAlign(driver.TextAlignEnd, true) != 0 || xAlign(0, true) != 1 {
		t.Fatal("xAlign")
	}
}

// TestFocusFollowsModalsAndTab checks the focus without a window: Tab
// goes around the fields of the newest modal, and a modal takes the focus
// from a field under it until it goes.
func TestFocusFollowsModalsAndTab(t *testing.T) {
	w := &window{root: 1}
	visible := driver.ViewStyle{Opacity: 1}
	mk := func(id driver.NodeID, kind driver.ViewKind, parent *view) *view {
		v := &view{id: id, kind: kind, style: visible, parent: parent}
		if kind == driver.KindInput {
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
	root := mk(1, driver.KindView, nil)
	a := mk(2, driver.KindInput, root)
	b := mk(3, driver.KindInput, root)
	if !w.tab(false) || w.focus != a.field || !w.tab(false) || w.focus != b.field || !w.tab(false) || w.focus != a.field {
		t.Fatal("Tab does not go around the fields")
	}
	if !w.tab(true) || w.focus != b.field {
		t.Fatal("Shift+Tab does not go back")
	}
	modal := mk(4, driver.KindModal, root)
	c := mk(5, driver.KindInput, modal)
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
	w := &window{animator: driver.NewAnimator(func(driver.Event) {}), drops: map[*view]bool{}, hosts: map[*component]bool{}}
	frame := func(id driver.NodeID, x, y, width, height driver.Points) driver.MountItem {
		return driver.MountItem{Kind: driver.ItemFrame, ID: id, Frame: driver.Bounds{Origin: driver.Point{X: x, Y: y},
			Size: driver.Size{Width: width, Height: height}}}
	}
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		frame(1, 0, 0, 300, 300), frame(2, 0, 0, 100, 100), frame(3, 10, 10, 20, 20), frame(4, 0, 100, 50, 50),
	}})
	root, old, child, mover := w.views[1], w.views[2], w.views[3], w.views[4]
	tr := &driver.LayoutTransition{Duration: time.Second}
	w.apply(&driver.MountBatch{Root: 1, LayoutAnimation: &driver.LayoutAnimation{Update: tr, Delete: tr, Create: tr},
		Items: []driver.MountItem{
			{Kind: driver.ItemDelete, ID: 2},
			{Kind: driver.ItemDelete, ID: 3},
			{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
			{Kind: driver.ItemInsert, ID: 2, Parent: 1, Before: 4},
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
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{frame(4, 0, 0, 50, 50), {Kind: driver.ItemDelete, ID: 2}}})
	if mover.moving != nil || frameOf(mover).Origin.Y != 0 || len(root.leaving) != 0 || !fresh.deleted {
		t.Fatal("a batch without a layout animation animated")
	}
}

// TestFrameTransitionMovesOnlyItsView moves a view by its frame item's
// own transition, and lets a batch's Update win over it.
func TestFrameTransitionMovesOnlyItsView(t *testing.T) {
	w := &window{animator: driver.NewAnimator(func(driver.Event) {}), drops: map[*view]bool{}, hosts: map[*component]bool{}}
	frame := func(id driver.NodeID, width driver.Points, tr *driver.LayoutTransition) driver.MountItem {
		return driver.MountItem{Kind: driver.ItemFrame, ID: id, Frame: driver.Bounds{Size: driver.Size{Width: width, Height: 10}},
			Transition: tr}
	}
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		frame(1, 300, nil), frame(2, 20, nil), frame(3, 20, nil),
	}})
	bar, other := w.views[2], w.views[3]
	own := driver.Transition{Duration: time.Second, Easing: driver.EaseOut}.Layout()
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{frame(2, 200, own), frame(3, 200, nil)}})
	switch {
	case bar.moving == nil || bar.moving.tr != *own || frameOf(bar).Size.Width != 20 || bar.frame.Size.Width != 200:
		t.Fatalf("the bar moves %+v from %v", bar.moving, frameOf(bar))
	case other.moving != nil || frameOf(other).Size.Width != 200:
		t.Fatal("a frame without a transition animated")
	}
	update := &driver.LayoutTransition{Duration: 2 * time.Second}
	w.apply(&driver.MountBatch{Root: 1, LayoutAnimation: &driver.LayoutAnimation{Update: update},
		Items: []driver.MountItem{frame(2, 100, own)}})
	if bar.moving == nil || bar.moving.tr != *update {
		t.Fatalf("the batch's Update did not win: %+v", bar.moving)
	}
}

func TestTextLayoutsHitTestTheirCharacters(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16}, Color: driver.RGB(0)}
	layout := func(text string) *textLayout {
		p := w.Prepare(&driver.AttributedText{Text: text, Style: style})
		t.Cleanup(p.Release)
		l := p.Layout(p.Measure(driver.Auto)).(*textLayout)
		t.Cleanup(l.Release)
		return l
	}
	l := layout("abc def\nghi")
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
	if r := layout("ab \u05d0\u05d1\u05d2 cd").RangeRects(1, 5, nil); len(r) != 2 {
		t.Fatalf("a range from b into the Hebrew covers %v, want two boxes", r)
	}
}

// TestMonitorLayoutKeepsMixedScalesApart lays out a 100% primary monitor with a 200% one on its right and a 150% one on its left: their frames must not overlap, so a frame on any of them picks that one, and a window's frame read through its monitor must come back as the same pixels.
func TestMonitorLayoutKeepsMixedScalesApart(t *testing.T) {
	monitor := func(id uintptr, r rect, primary bool) foundMonitor {
		m := foundMonitor{monitor: id, info: monitorInfoEx{Monitor: r, Work: r}}
		if primary {
			m.info.Flags = 1
		}
		m.info.Work.Bottom -= 40
		return m
	}
	all := []foundMonitor{
		monitor(1, rect{0, 0, 1920, 1080}, true),
		monitor(2, rect{1920, 0, 1920 + 3840, 2160}, false),
		monitor(3, rect{-2880, 0, 0, 1620}, false),
	}
	placements := layoutMonitors(all, []float64{1, 2, 1.5})
	if len(placements) != len(all) {
		t.Fatalf("%d placements; want %d", len(placements), len(all))
	}
	if f := placements[0].display.Frame; f.Origin != (driver.Point{}) || f.Size != (driver.Size{Width: 1920, Height: 1080}) {
		t.Fatalf("primary frame %+v; want 0,0 1920x1080", f)
	}
	for i, a := range placements {
		for _, b := range placements[i+1:] {
			fa, fb := a.display.Frame, b.display.Frame
			if fa.Origin.X < fb.Origin.X+fb.Size.Width && fb.Origin.X < fa.Origin.X+fa.Size.Width &&
				fa.Origin.Y < fb.Origin.Y+fb.Size.Height && fb.Origin.Y < fa.Origin.Y+fa.Size.Height {
				t.Fatalf("frames %+v and %+v overlap", fa, fb)
			}
		}
	}
	// Windows on each monitor, in pixels: one near the 200% monitor's left edge, whose center in per-monitor points fell inside the primary.
	for _, c := range []struct {
		on  int
		win rect
	}{
		{1, rect{2000, 100, 3600, 1300}},
		{0, rect{1500, 100, 1900, 400}},
		{2, rect{-2800, 50, -1600, 950}},
	} {
		p := placements[c.on]
		corner := p.point(c.win.Left, c.win.Top)
		f := driver.WindowFrame{X: corner.X, Y: corner.Y, Move: true,
			Width: driver.Points(float64(c.win.Right-c.win.Left) / p.scale), Height: driver.Points(float64(c.win.Bottom-c.win.Top) / p.scale)}
		got, ok := choosePlacement(placements, f)
		if !ok || got.monitor != p.monitor {
			t.Fatalf("frame %+v of a window on monitor %d chose monitor %d", f, p.monitor, got.monitor)
		}
		x, y := got.pixel(f.X, f.Y)
		if back := (rect{x, y, x + got.px(f.Width), y + got.px(f.Height)}); back != c.win {
			t.Fatalf("window %+v came back as %+v", c.win, back)
		}
	}
}

// TestTrayEventsDecodeVersion4 checks that a NOTIFYICON_VERSION_4 callback is read with the event in LOWORD(lParam) and the icon in HIWORD(lParam), and that a click counts once: by NIN_SELECT or WM_CONTEXTMENU, not the raw button-up that comes before it.
func TestTrayEventsDecodeVersion4(t *testing.T) {
	const id = 3
	for _, c := range []struct {
		event uint32
		index int
		ok    bool
	}{
		{ninSelect, 0, true},
		{ninKeySelect, 0, true},
		{wmContextMenu, 1, true},
		{wmMButtonUp, 0, true},
		{wmLButtonUp, 0, false},
		{wmRButtonUp, 0, false},
		{0x0200, 0, false}, // WM_MOUSEMOVE
	} {
		gotID, index, ok := trayEvent(uintptr(id<<16 | c.event))
		if ok != c.ok || ok && (gotID != id || index != c.index) {
			t.Errorf("event %#x: id %d, index %d, ok %v; want id %d, index %d, ok %v", c.event, gotID, index, ok, id, c.index, c.ok)
		}
	}
}

// TestRecycleBinKeeps checks trashFile's size check: Windows 11 deletes a file over its volume's Recycle Bin size for good, even with FOFX_RECYCLEONDELETE and nothing in the bin, so trashFile refuses a file the bin may not keep. A 1 MB bin kept a file of 1 MiB and deleted one a byte larger; the check refuses the first too, and a bin it cannot size.
func TestRecycleBinKeeps(t *testing.T) {
	const mb = 1 << 20
	const volume = 200_000 * mb
	for _, c := range []struct {
		size           uint64
		maxMB, percent uint32
		volume         uint64
		keeps          bool
	}{
		{0, 1, 100, volume, true},
		{mb - 1, 1, 100, volume, true},
		{mb, 1, 100, volume, false},
		{mb + 1, 1, 100, volume, false},
		{3 * mb, 7167, 100, volume, true},
		{7167 * mb, 7167, 100, volume, false},
		// MaxCapacity unknown reads as 0, as does a volume of unknown size.
		{0, 0, 100, volume, false},
		{mb, 7167, 100, 0, false},
		// The policy's percentage of the volume caps the bin.
		{mb / 2, 7167, 1, 100 * mb, true},
		{2 * mb, 7167, 1, 100 * mb, false},
		{1 << 40, 0xFFFFFFFF, 100, 1 << 50, true},
	} {
		if got := recycleBinKeeps(c.size, c.maxMB, c.percent, c.volume); got != c.keeps {
			t.Errorf("recycleBinKeeps(%d, %d MB, %d%% of %d) = %v, want %v", c.size, c.maxMB, c.percent, c.volume, got, c.keeps)
		}
	}
}

func TestTaskbarProgressMapsStatesAndFractions(t *testing.T) {
	for _, tc := range []struct {
		in        driver.TaskProgress
		state     uintptr
		completed int
	}{
		{driver.TaskProgress{}, tbpfNoProgress, -1},
		{driver.TaskProgress{State: driver.TaskProgressIndeterminate, Value: .5}, tbpfIndeterminate, -1},
		{driver.TaskProgress{State: driver.TaskProgressNormal, Value: .29}, tbpfNormal, 290},
		{driver.TaskProgress{State: driver.TaskProgressPaused, Value: .4}, tbpfPaused, 400},
		{driver.TaskProgress{State: driver.TaskProgressError, Value: 1}, tbpfError, taskbarProgressTotal},
		{driver.TaskProgress{State: driver.TaskProgressNormal, Value: 2}, tbpfNormal, taskbarProgressTotal},
	} {
		if state, completed := taskbarProgress(tc.in); state != tc.state || completed != tc.completed {
			t.Errorf("taskbarProgress(%+v) = %d, %d, want %d, %d", tc.in, state, completed, tc.state, tc.completed)
		}
	}
}

func TestTaskProgressOfSurfacesWithoutAButtonShowsOnTheAppsWindows(t *testing.T) {
	root := &window{primary: true, opts: Options{Hidden: true}}
	opened := &window{hostWindow: root}
	panel := &window{hostWindow: root, kind: driver.SurfacePanel}
	tray := &window{hostWindow: root, kind: driver.SurfaceTrayIcon}
	if root.ownsTaskbarButton() || panel.ownsTaskbarButton() || tray.ownsTaskbarButton() || !opened.ownsTaskbarButton() {
		t.Fatal("only the opened window has a taskbar button")
	}
	if shown := (&window{primary: true}); !shown.ownsTaskbarButton() {
		t.Error("a shown root window has no taskbar button")
	}
	busy := driver.TaskProgress{State: driver.TaskProgressNormal, Value: .5}
	root.setTaskProgress(busy)
	if opened.shownProgress() != busy || opened.progress != (driver.TaskProgress{}) {
		t.Errorf("the opened window shows %+v and keeps %+v", opened.shownProgress(), opened.progress)
	}
	// The app's tree commits before a panel's, so the panel's earlier call can arrive after the app's later one, and changes nothing.
	cleared := driver.TaskProgress{Call: 3}
	root.setTaskProgress(cleared)
	panel.setTaskProgress(driver.TaskProgress{State: driver.TaskProgressNormal, Value: 1, Call: 2})
	if opened.shownProgress() != cleared {
		t.Errorf("the opened window shows %+v after an earlier call arrived late, want the later clear", opened.shownProgress())
	}
	busy.Call = 4
	root.setTaskProgress(busy)
	own := driver.TaskProgress{State: driver.TaskProgressPaused, Value: .2}
	opened.setTaskProgress(own)
	if opened.shownProgress() != own {
		t.Errorf("the opened window shows %+v over its own", opened.shownProgress())
	}
	opened.setTaskProgress(driver.TaskProgress{})
	if opened.shownProgress() != busy {
		t.Errorf("a cleared window shows %+v, want the app's again", opened.shownProgress())
	}
}

// TestFileFilterNamesTheTypes checks the file dialog's type menu: the app's name for its types, the old English words without one, and every file where a type has no extensions.
func TestFileFilterNamesTheTypes(t *testing.T) {
	for _, c := range []struct {
		r             driver.FilesRequest
		name, pattern string
	}{
		{driver.FilesRequest{Types: []string{".mp4", ".mkv"}, TypesName: "视频"}, "视频", "*.mkv;*.mp4"},
		{driver.FilesRequest{Types: []string{".zip"}}, "Files", "*.zip"},
		{driver.FilesRequest{}, "All files", "*.*"},
		{driver.FilesRequest{TypesName: "Anything"}, "Anything", "*.*"},
		{driver.FilesRequest{Types: []string{"application/x-nsgo-unknown"}}, "All files", "*.*"},
	} {
		if name, pattern := fileFilter(c.r); name != c.name || pattern != c.pattern {
			t.Errorf("fileFilter(%+v) = %q, %q, want %q, %q", c.r, name, pattern, c.name, c.pattern)
		}
	}
}

// TestStatusAndSearchFieldElements checks what UI Automation reads of a status, a status bar named by the texts inside it with the system's own type name, and of a search field, an edit named a search box in the language the app shows.
func TestStatusAndSearchFieldElements(t *testing.T) {
	e := &uiaElement{}
	text := func(s string) *view {
		return &view{kind: driver.KindText, plain: s, props: driver.ViewProps{Role: driver.RoleText}}
	}
	status := &view{kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleStatus, LiveRegion: driver.LiveRegionPolite},
		children: []*view{text("3"), text("files saved")}}
	if s := e.spec(status); s.control != ctStatusBar || s.name != "3 files saved" || s.localizedType != "" || s.live != driver.LiveRegionPolite {
		t.Errorf("a status is control %d named %q, type %q, live %d", s.control, s.name, s.localizedType, s.live)
	}
	search := &view{kind: driver.KindInput, props: driver.ViewProps{Role: driver.RoleSearchField, Label: "Find"}}
	if s := e.spec(search); s.control != ctEdit || s.name != "Find" || s.localizedType != "search box" {
		t.Errorf("a search field is control %d named %q, type %q", s.control, s.name, s.localizedType)
	}
	// Named in the language the app shows, not English.
	chinese := &uiaElement{w: &window{language: "zh-Hans"}}
	if s := chinese.spec(search); s.control != ctEdit || s.localizedType != "搜索框" {
		t.Errorf("in Chinese, a search field is control %d, type %q", s.control, s.localizedType)
	}
	field := &view{kind: driver.KindInput, props: driver.ViewProps{Role: driver.RoleTextField, Label: "Name"}}
	if s := e.spec(field); s.control != ctEdit || s.localizedType != "" {
		t.Errorf("a text field is control %d, type %q", s.control, s.localizedType)
	}
}

func TestMultiSelectableExposesSelectionProviderState(t *testing.T) {
	e := &uiaElement{}
	grid := &view{kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleGrid, MultiSelectable: true}}
	if s := e.spec(grid); !s.multiselectable || s.selectable {
		t.Fatalf("multi-select grid spec = %+v, want provider without item pattern", s)
	}
	single := &view{kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleGrid}}
	if s := e.spec(single); s.multiselectable {
		t.Fatalf("single-select grid spec = %+v, want no selection provider", s)
	}
}

func TestSelectionItemsKeepTheirPatternWhenUnselected(t *testing.T) {
	e := &uiaElement{}
	for _, selected := range []bool{false, true} {
		row := &view{kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleRow, SelectionItem: true, Selected: selected}}
		if s := e.spec(row); !s.selectable || s.selected != selected {
			t.Errorf("selection item selected=%v has selectable=%v selected=%v", selected, s.selectable, s.selected)
		}
	}
	row := &view{kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleRow}}
	if s := e.spec(row); s.selectable {
		t.Error("a row without selection support exposed SelectionItem")
	}
}

func TestScrollMoveIndicatorOnlyContinuesUserMomentum(t *testing.T) {
	for name, v := range map[string]*view{
		"app move":      {},
		"animated move": {scrolling: &scrollbar.OffsetAnimation{}},
		"user momentum": {scrolling: &scrollbar.OffsetAnimation{Momentum: true}},
	} {
		if got := scrollMoveShowsIndicator(v); got != (name == "user momentum") {
			t.Errorf("%s: scrollMoveShowsIndicator = %v", name, got)
		}
	}
}

// TestWindowCapabilitiesNeedAWindowOfTheSurfacesOwn checks that the environment reports levels and placement only for a surface with a top-level window of its own, as macOS does: not an embedded view, the app's own hidden surface or a tray icon's.
func TestWindowCapabilitiesNeedAWindowOfTheSurfacesOwn(t *testing.T) {
	for _, c := range []struct {
		name string
		w    *window
		want bool
	}{
		{"window", &window{primary: true}, true},
		{"opened window", &window{kind: driver.SurfaceWindow}, true},
		{"panel", &window{kind: driver.SurfacePanel}, true},
		{"embedded", &window{embedded: true}, false},
		{"app's own surface", &window{primary: true, opts: Options{Hidden: true}}, false},
		{"tray icon", &window{kind: driver.SurfaceTrayIcon, opts: Options{Hidden: true}}, false},
	} {
		if got := c.w.showsOwnWindow(); got != c.want {
			t.Errorf("%s: showsOwnWindow = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPixelsReplaceThePictureInPlace(t *testing.T) {
	s := &imageSlot{}
	bitmap := &comObject{}
	s.bitmap = bitmap
	frame := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(frame.Pix, []byte{0, 0, 0, 0, 0xff, 0, 0, 0xff})
	s.showPixels(driver.ImageSource{Pixels: frame, Version: 1})
	if s.bitmap != bitmap {
		t.Fatal("a pixel frame dropped the reusable bitmap")
	}
	if s.pic == nil || s.pic.width != 2 || s.pic.height != 1 || s.load != nil || s.fade != 0 {
		t.Fatalf("slot = %+v, want the frame's picture at once", s)
	}
	// Premultiplied BGRA, as Direct2D takes it, alpha kept.
	if got := s.pic.frames[0].Pix; got[3] != 0 || got[4] != 0 || got[6] != 0xff || got[7] != 0xff {
		t.Fatalf("pixels = %v", got)
	}
	if got := natural(s); got != (driver.Size{Width: 2, Height: 1}) {
		t.Fatalf("natural size = %v, want a point a pixel", got)
	}
	if !sameSource(s.src, driver.ImageSource{Pixels: frame, Version: 1}) || sameSource(s.src, driver.ImageSource{Pixels: frame, Version: 2}) {
		t.Fatal("a source is the same frame only at its version")
	}
	old := &s.pic.frames[0].Pix[0]
	frame.Pix[0] = 0x80
	s.showPixels(driver.ImageSource{Pixels: frame, Version: 2})
	if &s.pic.frames[0].Pix[0] != old || s.pic.frames[0].Pix[2] != 0x80 || sameSource(s.src, driver.ImageSource{Pixels: frame, Version: 1}) {
		t.Fatal("same-sized pixel frames did not reuse the upload buffer")
	}
}

// TestMultilineInputScrollsUnderPadding checks that a multiline textInput clips
// to its padding box, runs its scroll indicator along the padding box's outer
// edge, and scrolls vertically across its content height (#1598).
func TestMultilineInputScrollsUnderPadding(t *testing.T) {
	pad := driver.Edges{Top: 18, Right: 20, Bottom: 20, Left: 12}
	in := &textInput{
		config: driver.InputConfig{
			Multiline: true,
			Padding:   pad,
			Content:   driver.Bounds{Origin: driver.Point{X: 13, Y: 19}, Size: driver.Size{Width: 266, Height: 160}},
		},
		width:         266,
		height:        160,
		contentHeight: 400,
	}
	wantClip := rectF{left: 1, top: 1, right: 299, bottom: 199}
	if got := in.clipRect(); got != wantClip {
		t.Fatalf("multiline clipRect = %+v, want padding box %+v", got, wantClip)
	}
	thumb, _, ok := in.bar().Indicator()
	if !ok {
		t.Fatal("expected scroll indicator for overflowing multiline input")
	}
	if wantRight := driver.Points(298) - 3; thumb.Origin.X+thumb.Size.Width != wantRight {
		t.Errorf("indicator right edge = %v in padding box, want %v", thumb.Origin.X+thumb.Size.Width, wantRight)
	}
	if !in.wheelScroll(-1) || in.scroll <= 0 {
		t.Fatalf("wheelScroll(-1) did not scroll down: scroll=%v", in.scroll)
	}
	if !in.wheelScroll(-100) || in.scroll != 240 {
		t.Fatalf("wheelScroll clamp: scroll=%v, want 240 (400-160)", in.scroll)
	}
	in.config.Multiline = false
	if got := in.clipRect(); got.left != 13 || got.right != 279 {
		t.Errorf("single-line clipRect = %+v, want horizontal bounds [13, 279]", got)
	}
}

func TestTextInkBoundsAndDamageExtent(t *testing.T) {
	box := driver.Bounds{Origin: driver.Point{X: 2, Y: 4}, Size: driver.Size{Width: 60, Height: 18}}
	// An underline offset below the box, a double line and a wavy line reach past the box.
	l := &textLayout{
		lines: []lineBox{{top: 0, height: 18}},
		decorations: []decoration{
			{r: rectF{0, 26, 60, 29}, color: driver.RGB(0xff0000), style: driver.DecorationSolid},
			{r: rectF{0, 22, 60, 25}, color: driver.RGB(0x00ff00), style: driver.DecorationDouble},
			{r: rectF{0, -6, 60, -2}, color: driver.RGB(0x0000ff), style: driver.DecorationWavy},
		},
	}
	got := l.inkBounds(box)
	// Solid reaches y=4+29=33, double reaches y=4+25+3=32, wavy reaches top y=4-6-4=-6.
	if got.top > -6 || got.bottom < 33 || got.left > 2 || got.right < 62 {
		t.Fatalf("decoration inkBounds = %+v, want top <= -6, bottom >= 33, left <= 2, right >= 62", got)
	}

	// A text shadow reaches past the box by its offset and blur.
	shadowed := &textLayout{
		lines:  []lineBox{{top: 0, height: 18}},
		shadow: driver.TextShadow{Color: driver.RGB(0x00ff00), Offset: driver.Point{X: 12, Y: 10}, Radius: 4},
	}
	sg := shadowed.inkBounds(box)
	if sg.right < 2+60+12+8 || sg.bottom < 4+18+10+8 {
		t.Fatalf("shadow inkBounds = %+v, want right >= 82 and bottom >= 40", sg)
	}

	// EllipsizeClip bounds the drawn ink to l.clip.
	clipped := &textLayout{
		lines:       []lineBox{{top: 0, height: 18}},
		decorations: []decoration{{r: rectF{0, 26, 60, 29}, color: driver.RGB(0xff0000), style: driver.DecorationSolid}},
		clip:        rectF{0, 0, 60, 18},
	}
	if cg := clipped.inkBounds(box); cg.bottom > 4+18 {
		t.Fatalf("clipped inkBounds = %+v, want bottom <= 22", cg)
	}

	// viewExtent includes text ink only when the view does not clip, and
	// measuring.visit still clips it against a clipping ancestor.
	bounds := rectF{0, 0, 64, 22}
	v := &view{kind: driver.KindText, frame: driver.Bounds{Origin: driver.Point{X: 20, Y: 20}, Size: driver.Size{Width: 64, Height: 22}},
		content: box, text: shadowed, style: driver.ViewStyle{Opacity: 1}}
	ext := viewExtent(v, &v.style, bounds)
	if ext.right < sg.right+1 || ext.bottom < sg.bottom+1 {
		t.Fatalf("unclipped viewExtent = %+v, want at least %+v", ext, sg)
	}
	v.style.Clip = true
	clipExt := viewExtent(v, &v.style, bounds)
	if clipExt != extent(&v.style, bounds) {
		t.Fatalf("clipped viewExtent = %+v, want %+v", clipExt, extent(&v.style, bounds))
	}

	// An ancestor with Clip intersects the text's measured damage area.
	v.style.Clip = false
	v.queued = true
	parent := &view{kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 90, Height: 45}},
		style: driver.ViewStyle{Opacity: 1, Clip: true}, children: []*view{v}}
	v.parent = parent
	w := &window{animator: driver.NewAnimator(nil)}
	m := measuring{w: w}
	m.visit(parent, identity, 0, 0, rectF{0, 0, 200, 200}, true, false)
	if v.area.right > 90 || v.area.bottom > 45 {
		t.Fatalf("ancestor-clipped area = %+v, want within 90x45", v.area)
	}
	parent.style.Clip = false
	m.visit(parent, identity, 0, 0, rectF{0, 0, 200, 200}, true, false)
	if v.area.right <= 20+64 || v.area.bottom <= 20+22 {
		t.Fatalf("unclipped area = %+v, want past view box (84, 42)", v.area)
	}
}

type noopPreparedText struct{}

func (noopPreparedText) Measure(driver.Points) driver.Size    { return driver.Size{} }
func (noopPreparedText) Layout(driver.Size) driver.TextLayout { return noopPreparedText{} }
func (noopPreparedText) Release()                             {}

type noopPlatform struct{}

func (noopPlatform) Prepare(*driver.AttributedText) driver.PreparedText { return noopPreparedText{} }
func (noopPlatform) Mount(*driver.MountBatch)                           {}
func (noopPlatform) RequestFrame()                                      {}
func (noopPlatform) SubmitGPU(driver.GPUFrame)                          {}

func TestTouchBrakingClaimsGlideAndSnap(t *testing.T) {
	var events []driver.Event
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) { events = append(events, ev) },
	})
	t.Cleanup(eng.Close)

	root := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 400}}}
	scroll := &view{
		id:     2,
		kind:   driver.KindScroll,
		parent: root,
		frame:  driver.Bounds{Size: driver.Size{Width: 300, Height: 400}},
		size:   driver.Size{Width: 300, Height: 2000},
		events: 1<<driver.EventScrollDragStart | 1<<driver.EventScrollDragEnd |
			1<<driver.EventMomentumStart | 1<<driver.EventMomentumEnd | 1<<driver.EventScrollEnd,
	}
	root.children = []*view{scroll}
	w := &window{
		engine:   eng,
		root:     root.id,
		views:    []*view{nil, root, scroll},
		animator: driver.NewAnimator(func(driver.Event) {}),
		ticking:  true,
	}
	count := func(kind driver.EventKind) int {
		n := 0
		for _, ev := range events {
			if ev.Kind == kind {
				n++
			}
		}
		return n
	}
	at := driver.Point{X: 100, Y: 200}

	// 1. Touch down stopping a glide claims the touch until lift without starting a drag or emitting scroll end.
	m := scrollbar.Release(&scroll.scroll, scroll.frame.Size, scroll.size, driver.Point{Y: 100}, 1.2, 0, 1600, time.Now())
	scroll.offset = driver.Point{Y: 100}
	scroll.glide = &m
	events = nil
	if !w.touchDown(1, at) {
		t.Fatal("touchDown on a gliding scroll view returned false; want true")
	}
	if scroll.glide != nil {
		t.Fatal("touchDown did not clear glide")
	}
	if !w.touchMove(driver.Point{X: 100, Y: 196}) {
		t.Fatal("sub-slop touchMove after braking returned false; want true")
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking without drag returned false; want true")
	}
	if got := count(driver.EventMomentumEnd); got != 1 {
		t.Errorf("momentum end count = %d, want 1", got)
	}
	if count(driver.EventScrollDragStart) != 0 || count(driver.EventScrollDragEnd) != 0 || count(driver.EventScrollEnd) != 0 {
		t.Errorf("braking tap emitted unexpected events: %+v", events)
	}

	// 2. Touch down stopping a snap claims the touch too.
	scroll.scrolling = &scrollbar.OffsetAnimation{From: driver.Point{Y: 100}, To: driver.Point{Y: 400}, Start: time.Now(), Momentum: true}
	events = nil
	if !w.touchDown(2, at) {
		t.Fatal("touchDown on a snapping scroll view returned false; want true")
	}
	if scroll.scrolling != nil {
		t.Fatal("touchDown did not clear snap")
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking a snap returned false; want true")
	}
	if got := count(driver.EventMomentumEnd); got != 1 {
		t.Errorf("snap brake momentum end count = %d, want 1", got)
	}
	if count(driver.EventScrollDragStart) != 0 || count(driver.EventScrollDragEnd) != 0 || count(driver.EventScrollEnd) != 0 {
		t.Errorf("snap braking tap emitted unexpected events: %+v", events)
	}

	// 3. Touch down stopping an animated ScrollTo claims the touch without reporting momentum end.
	scroll.scrolling = &scrollbar.OffsetAnimation{From: driver.Point{Y: 100}, To: driver.Point{Y: 400}, Start: time.Now()}
	events = nil
	if !w.touchDown(3, at) {
		t.Fatal("touchDown on an animated scroll returned false; want true")
	}
	if scroll.scrolling != nil {
		t.Fatal("touchDown did not clear animated scroll")
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking animated scroll returned false; want true")
	}
	if count(driver.EventMomentumEnd) != 0 {
		t.Errorf("animated scroll brake reported momentum end: %+v", events)
	}

	// 4. Braking touch that drags past TouchSlop scrolls smoothly.
	scroll.glide = &m
	scroll.offset = driver.Point{Y: 100}
	events = nil
	if !w.touchDown(4, at) {
		t.Fatal("touchDown before braking drag returned false")
	}
	if !w.touchMove(driver.Point{X: 100, Y: 195}) {
		t.Fatal("sub-slop move during braking drag returned false")
	}
	if count(driver.EventScrollDragStart) != 0 {
		t.Fatal("sub-slop move started drag")
	}
	if !w.touchMove(driver.Point{X: 100, Y: 150}) {
		t.Fatal("move past slop during braking drag returned false")
	}
	if !w.touchMove(driver.Point{X: 100, Y: 110}) {
		t.Fatal("continued move during braking drag returned false")
	}
	if count(driver.EventScrollDragStart) != 1 {
		t.Errorf("drag start count = %d, want 1", count(driver.EventScrollDragStart))
	}
	if scroll.offset.Y <= 100 {
		t.Errorf("offset.Y = %v, want > 100 after braking drag", scroll.offset.Y)
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking drag returned false")
	}
	if count(driver.EventScrollDragEnd) != 1 {
		t.Errorf("drag end count = %d, want 1", count(driver.EventScrollDragEnd))
	}
	scroll.glide = nil

	// 5. Touch at rest is not claimed, so mouse promotion delivers the press.
	events = nil
	if w.touchDown(5, at) {
		t.Fatal("touchDown at rest returned true; want false")
	}
	if w.touchMove(driver.Point{X: 100, Y: 196}) {
		t.Fatal("sub-slop touchMove at rest returned true; want false")
	}
	if w.touchUp() {
		t.Fatal("touchUp at rest without drag returned true; want false")
	}

	// 6. NoScroll leaves touches alone even if a motion were present.
	scroll.scroll.Behavior.NoScroll = true
	scroll.glide = &m
	if w.touchDown(6, at) {
		t.Fatal("touchDown on NoScroll view returned true; want false")
	}
	if scroll.glide == nil {
		t.Fatal("NoScroll view had its glide cleared")
	}
}

// TestCanvasPartsDropTheirElements gives a client's elements to a canvas's parts: a drawing with fewer parts drops those of the parts it lost, and deleting the canvas the rest.
func TestCanvasPartsDropTheirElements(t *testing.T) {
	w := &window{scale: 1, animator: driver.NewAnimator(func(driver.Event) {}), drops: map[*view]bool{}, hosts: map[*component]bool{}}
	parts := func(n int) *driver.Drawing {
		return &driver.Drawing{Nodes: make([]driver.CanvasNode, n)}
	}
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindCanvas},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemCanvas, ID: 2, Drawing: parts(3)},
	}})
	for part := range 4 {
		w.element(2, part)
	}
	held := func() (parts []int) {
		for part := range 4 {
			if w.uia[uiaKey(2, part)] != nil {
				parts = append(parts, part)
			}
		}
		return parts
	}
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{{Kind: driver.ItemCanvas, ID: 2, Drawing: parts(1)}}})
	if got := held(); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("a drawing of one part leaves the elements of parts %v, want [0 1]", got)
	}
	w.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{{Kind: driver.ItemDelete, ID: 2}}})
	if got := held(); got != nil {
		t.Fatalf("a deleted canvas leaves the elements of parts %v", got)
	}
}

func TestBackdropTransformAndTarget(t *testing.T) {
	// A translated host with overflow maps its local points (x, y) from target DIPs to (x + ov, y + ov) in the backdrop picture's DIPs.
	cases := []struct {
		name  string
		world matrix
		ov    float64
	}{
		{"translated", translation(40, 25), 0},
		{"translated_overflow", translation(40, 25), 12},
		{"scaled_translated", scaling(2, 1.5).then(translation(30, 10)), 8},
		{"rotated", matrix{a: 0, b: 1, c: -1, d: 0, e: 100, f: 50}, 6},
	}
	for _, tc := range cases {
		toPic := backdropTransform(tc.world, tc.ov)
		for _, pt := range [][2]float64{{0, 0}, {10, 20}, {-tc.ov, -tc.ov}} {
			tx, ty := tc.world.apply(pt[0], pt[1])
			px, py := toPic.apply(tx, ty)
			if !closeTo(px, pt[0]+tc.ov) || !closeTo(py, pt[1]+tc.ov) {
				t.Errorf("%s: local (%g,%g) -> target (%g,%g) -> pic (%g,%g), want (%g,%g)",
					tc.name, pt[0], pt[1], tx, ty, px, py, pt[0]+tc.ov, pt[1]+tc.ov)
			}
		}
	}

	// backdropTarget prefers the painter's explicit target size and alpha mode, falling back to the window's swap chain when unset.
	p := &painter{w: &window{}}
	if _, _, _, ok := p.backdropTarget(); ok {
		t.Fatal("backdropTarget succeeded with no target and no swap chain")
	}
	p.w.swap = &swapChain{width: 320, height: 200, composed: true}
	if w, h, alpha, ok := p.backdropTarget(); !ok || w != 320 || h != 200 || !alpha {
		t.Fatalf("swap fallback = (%d, %d, %v, %v), want (320, 200, true, true)", w, h, alpha, ok)
	}
	p.targetWidth, p.targetHeight, p.targetAlpha = 64, 48, false
	if w, h, alpha, ok := p.backdropTarget(); !ok || w != 64 || h != 48 || alpha {
		t.Fatalf("explicit target = (%d, %d, %v, %v), want (64, 48, false, true)", w, h, alpha, ok)
	}
}

func TestPreciseScrollDeliversTouchpadPhasesToOnWheel(t *testing.T) {
	var events []driver.Event
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) { events = append(events, ev) },
	})
	t.Cleanup(eng.Close)

	visible := driver.ViewStyle{Opacity: 1}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 300}}}
	outerScroll := &view{
		id: 2, kind: driver.KindScroll, parent: root, style: visible,
		frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 300}},
		size:  driver.Size{Width: 300, Height: 1000},
	}
	wheelView := &view{
		id: 3, kind: driver.KindView, parent: outerScroll, style: visible,
		frame:  driver.Bounds{Origin: driver.Point{X: 20, Y: 30}, Size: driver.Size{Width: 200, Height: 160}},
		events: 1 << driver.EventWheel,
	}
	innerScroll := &view{
		id: 4, kind: driver.KindScroll, parent: wheelView, style: visible,
		frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 80, Height: 80}},
		size:  driver.Size{Width: 80, Height: 400},
	}
	wheelView.children = []*view{innerScroll}
	outerScroll.children = []*view{wheelView}
	root.children = []*view{outerScroll}

	w := &window{
		engine:   eng,
		root:     root.id,
		views:    []*view{nil, root, outerScroll, wheelView, innerScroll},
		scale:    1.5,
		animator: driver.NewAnimator(func(driver.Event) {}),
		ticking:  true,
	}

	// Scroll over wheelView (outside innerScroll): OnWheel takes it before outerScroll, with GestureBegan -> GestureChanged* -> GestureEnded, Precise: true, and exact inverted Delta.
	at := driver.Point{X: 150, Y: 120}
	offsetDelta := driver.Point{X: -40, Y: -60}
	g := w.beginPreciseScroll(at)
	for _, step := range preciseScrollSteps(offsetDelta) {
		w.stepPreciseScroll(&g, at, step)
	}
	w.endPreciseScroll(&g, at)

	if outerScroll.offset != (driver.Point{}) {
		t.Fatalf("outer scroll view moved to %v when OnWheel should have taken the gesture", outerScroll.offset)
	}
	if len(events) < 3 {
		t.Fatalf("got %d wheel events, want at least 3 (began, changed..., ended): %+v", len(events), events)
	}
	if events[0].Kind != driver.EventWheel || events[0].ID != wheelView.id || events[0].Phase != driver.GestureBegan || !events[0].Precise || events[0].Delta != (driver.Point{}) {
		t.Fatalf("first event = %+v, want GestureBegan with Precise:true and zero delta", events[0])
	}
	last := events[len(events)-1]
	if last.Kind != driver.EventWheel || last.ID != wheelView.id || last.Phase != driver.GestureEnded || !last.Precise || last.Delta != (driver.Point{}) {
		t.Fatalf("last event = %+v, want GestureEnded with Precise:true and zero delta", last)
	}
	var sum driver.Point
	for i, ev := range events[1 : len(events)-1] {
		if ev.Kind != driver.EventWheel || ev.ID != wheelView.id || ev.Phase != driver.GestureChanged || !ev.Precise {
			t.Fatalf("middle event[%d] = %+v, want GestureChanged with Precise:true", i, ev)
		}
		if ev.Point != (driver.Point{X: 130, Y: 90}) || ev.Page != at {
			t.Fatalf("middle event[%d] coords = (%v, %v), want ({130 90}, %v)", i, ev.Point, ev.Page, at)
		}
		sum.X += ev.Delta.X
		sum.Y += ev.Delta.Y
	}
	if math.Abs(float64(sum.X-40)) > 1e-3 || math.Abs(float64(sum.Y-60)) > 1e-3 {
		t.Fatalf("summed wheel delta = %v, want {40 60}", sum)
	}

	// A scroll view inside an OnWheel view takes the scroll first, leaving OnWheel silent.
	events = nil
	overInner := driver.Point{X: 50, Y: 60}
	g = w.beginPreciseScroll(overInner)
	for _, step := range preciseScrollSteps(driver.Point{Y: 72.5}) {
		w.stepPreciseScroll(&g, overInner, step)
	}
	w.endPreciseScroll(&g, overInner)
	for _, ev := range events {
		if ev.Kind == driver.EventWheel {
			t.Fatalf("unexpected EventWheel when scrolling inner scroll view: %+v", ev)
		}
	}
	if math.Abs(float64(innerScroll.offset.Y-72.5)) > 1e-3 {
		t.Fatalf("innerScroll.offset.Y = %v, want 72.5", innerScroll.offset.Y)
	}

	// Deleting the latched OnWheel view mid-gesture emits no further events.
	events = nil
	g = w.beginPreciseScroll(at)
	wheelView.deleted = true
	w.stepPreciseScroll(&g, at, driver.Point{Y: -10})
	w.endPreciseScroll(&g, at)
	if len(events) != 1 || events[0].Phase != driver.GestureBegan {
		t.Fatalf("events after view deleted mid-gesture = %+v, want only GestureBegan", events)
	}
	wheelView.deleted = false

	// Real mouse-wheel wheelTaken still scales by wheelNotch (99pt/notch) with GestureChanged.
	events = nil
	if !w.wheelTaken(at, 120, false, 0) {
		t.Fatal("wheelTaken returned false for real wheel notch")
	}
	if len(events) != 1 || events[0].Phase != driver.GestureChanged || events[0].Precise || events[0].Delta != (driver.Point{Y: wheelNotch}) {
		t.Fatalf("real wheelTaken event = %+v, want GestureChanged Precise:false Delta:{0 %v}", events, wheelNotch)
	}
}

func TestPreciseScrollMovesNativeScrollViewsByExactPoints(t *testing.T) {
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{})
	t.Cleanup(eng.Close)

	visible := driver.ViewStyle{Opacity: 1}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 400, Height: 400}}}
	outerV := &view{
		id: 2, kind: driver.KindScroll, parent: root, style: visible,
		frame: driver.Bounds{Size: driver.Size{Width: 400, Height: 400}},
		size:  driver.Size{Width: 400, Height: 1000},
	}
	innerV := &view{
		id: 3, kind: driver.KindScroll, parent: outerV, style: visible,
		frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 150, Height: 100}},
		size:  driver.Size{Width: 150, Height: 150}, // max scroll 50
	}
	innerH := &view{
		id: 4, kind: driver.KindScroll, parent: outerV, style: visible,
		scroll: driver.ScrollConfig{Horizontal: true},
		frame:  driver.Bounds{Origin: driver.Point{X: 10, Y: 150}, Size: driver.Size{Width: 200, Height: 100}},
		size:   driver.Size{Width: 600, Height: 100},
	}
	outerV.children = []*view{innerV, innerH}
	root.children = []*view{outerV}

	w := &window{
		engine:   eng,
		root:     root.id,
		views:    []*view{nil, root, outerV, innerV, innerH},
		scale:    2,
		animator: driver.NewAnimator(func(driver.Event) {}),
		ticking:  true,
	}

	runScroll := func(at, delta driver.Point) {
		g := w.beginPreciseScroll(at)
		for _, step := range preciseScrollSteps(delta) {
			w.stepPreciseScroll(&g, at, step)
		}
		w.endPreciseScroll(&g, at)
	}

	// 1. Exact vertical distance on outer scroll view.
	runScroll(driver.Point{X: 300, Y: 300}, driver.Point{Y: 80})
	if math.Abs(float64(outerV.offset.Y-80)) > 1e-3 {
		t.Fatalf("outerV.offset.Y = %v, want 80", outerV.offset.Y)
	}
	outerV.offset = driver.Point{}

	// 2. Exact horizontal distance on horizontal scroll view inside vertical scroll view.
	runScroll(driver.Point{X: 50, Y: 180}, driver.Point{X: 65.5})
	if math.Abs(float64(innerH.offset.X-65.5)) > 1e-3 || outerV.offset.Y != 0 {
		t.Fatalf("after horizontal scroll: innerH=%v outerV=%v, want innerH.X=65.5 outerV.Y=0", innerH.offset, outerV.offset)
	}

	// 3. Vertical scroll over innerH scrolls outerV along the vertical axis instead.
	runScroll(driver.Point{X: 50, Y: 180}, driver.Point{Y: 45})
	if math.Abs(float64(outerV.offset.Y-45)) > 1e-3 || math.Abs(float64(innerH.offset.X-65.5)) > 1e-3 {
		t.Fatalf("after vertical scroll over innerH: outerV=%v innerH=%v", outerV.offset, innerH.offset)
	}
	outerV.offset = driver.Point{}

	// 4. Nested vertical scroll views: gesture latches innerV and stops at its edge (50) without spilling into outerV.
	runScroll(driver.Point{X: 50, Y: 50}, driver.Point{Y: 80})
	if math.Abs(float64(innerV.offset.Y-50)) > 1e-3 || outerV.offset.Y != 0 {
		t.Fatalf("latched nested gesture: innerV=%v outerV=%v, want innerV.Y=50 outerV.Y=0", innerV.offset, outerV.offset)
	}

	// 5. Starting a new gesture when innerV is already at its edge scrolls outerV by the exact distance.
	runScroll(driver.Point{X: 50, Y: 50}, driver.Point{Y: 63.25})
	if math.Abs(float64(outerV.offset.Y-63.25)) > 1e-3 {
		t.Fatalf("outerV after innerV at edge = %v, want 63.25", outerV.offset.Y)
	}
}

func TestAccessibilityLevelAndSpinButton(t *testing.T) {
	heading := &view{id: 1, kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleHeader, Label: "Section", Level: 3}}
	unlevelled := &view{id: 2, kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleHeader, Label: "Title"}}
	deepHeading := &view{id: 3, kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleHeader, Label: "Deep", Level: 12}}
	treeItem := &view{id: 4, kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleTreeItem, Label: "Child", Level: 2}}
	plain := &view{id: 5, kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleTreeItem, Label: "Plain"}}
	spin := &view{id: 6, kind: driver.KindInput, field: &textInput{}, props: driver.ViewProps{Role: driver.RoleSpinButton, Label: "Count"}}
	w := &window{views: []*view{nil, heading, unlevelled, deepHeading, treeItem, plain, spin}}

	prop := func(id driver.NodeID, propID uintptr) variant {
		var out variant
		if code := uiaGetPropertyValue(&w.element(id, 0).simple, propID, &out); code != sOK {
			t.Fatalf("node %d prop %d failed: %x", id, propID, code)
		}
		return out
	}

	if v := prop(1, propHeadingLevel); v.vt != vtI4 || int32(v.val) != headingLevel1+2 {
		t.Errorf("heading level 3 HeadingLevel = %+v, want %d", v, headingLevel1+2)
	}
	if v := prop(1, propLevel); v.vt != vtI4 || int32(v.val) != 3 {
		t.Errorf("heading level 3 Level = %+v, want 3", v)
	}
	if v := prop(2, propHeadingLevel); v.vt != vtI4 || int32(v.val) != headingLevel1 {
		t.Errorf("unlevelled heading HeadingLevel = %+v, want %d", v, headingLevel1)
	}
	if v := prop(2, propLevel); v.vt != vtEmpty {
		t.Errorf("unlevelled heading Level = %+v, want empty", v)
	}
	if v := prop(3, propHeadingLevel); v.vt != vtI4 || int32(v.val) != headingLevel1+8 {
		t.Errorf("heading level 12 HeadingLevel = %+v, want clamped %d", v, headingLevel1+8)
	}
	if v := prop(3, propLevel); v.vt != vtI4 || int32(v.val) != 12 {
		t.Errorf("heading level 12 Level = %+v, want 12", v)
	}
	if v := prop(4, propHeadingLevel); v.vt != vtEmpty {
		t.Errorf("tree item HeadingLevel = %+v, want empty", v)
	}
	if v := prop(4, propLevel); v.vt != vtI4 || int32(v.val) != 2 {
		t.Errorf("tree item level 2 Level = %+v, want 2", v)
	}
	if v := prop(5, propLevel); v.vt != vtEmpty {
		t.Errorf("unlevelled tree item Level = %+v, want empty", v)
	}
	if v := prop(6, propControlType); v.vt != vtI4 || int32(v.val) != ctSpinner {
		t.Errorf("spin button input ControlType = %+v, want %d", v, ctSpinner)
	}
}

func TestScrollZoomAndHitTesting(t *testing.T) {
	// Moving an offset reports the scroll to the engine.
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{})
	t.Cleanup(eng.Close)
	w := &window{engine: eng, animator: driver.NewAnimator(nil), env: driver.Environment{ReduceMotion: true}}
	child := &view{id: 2, kind: driver.KindView, frame: driver.Bounds{Origin: driver.Point{X: 20, Y: 30}, Size: driver.Size{Width: 40, Height: 40}},
		style: driver.ViewStyle{Opacity: 1}}
	scroll := &view{id: 1, kind: driver.KindScroll, frame: driver.Bounds{Size: driver.Size{Width: 200, Height: 200}},
		size: driver.Size{Width: 200, Height: 200}, style: driver.ViewStyle{Opacity: 1}, children: []*view{child}}
	child.parent = scroll
	w.views = []*view{nil, scroll, child}
	w.root = 1

	old := scroll.scroll
	scroll.scroll = driver.ScrollConfig{MinZoomScale: 0.5, MaxZoomScale: 4, ZoomScale: 2}
	w.configureScrollZoom(scroll, &old)
	if scroll.zoomScale() != 2 || scroll.zoomValue() != 2 {
		t.Fatalf("initial zoom = %v / %v, want 2", scroll.zoomScale(), scroll.zoomValue())
	}
	// At zoom 2, offset 0, child at content (20,30)+(40,40) occupies viewport (40,60)..(120,140).
	if got := pick(scroll, driver.Point{X: 50, Y: 70}); got != child {
		t.Fatalf("pick at zoomed child = %v, want child", got)
	}
	if got := pick(scroll, driver.Point{X: 25, Y: 35}); got != scroll {
		t.Fatalf("pick at unzoomed origin = %v, want scroll", got)
	}
	// ItemZoomTo around (50, 50) at scale 3 moves offset to (50*3-100, 50*3-100) = (50, 50).
	w.zoomTo(scroll, &driver.ZoomRequest{Scale: 3, Center: &driver.Point{X: 50, Y: 50}}, false)
	if scroll.zoomScale() != 3 || scroll.offset != (driver.Point{X: 50, Y: 50}) {
		t.Fatalf("after zoomTo(3): zoom=%v offset=%+v, want 3 and (50, 50)", scroll.zoomScale(), scroll.offset)
	}
	// Wheel pan on a zooming view scrolls both vertically and horizontally.
	if v, to := wheelTurn([]*view{scroll}, -1, false); v != scroll || to.Y <= scroll.offset.Y {
		t.Fatalf("vertical wheelTurn = (%v, %+v), want scroll moving down", v, to)
	}
	if v, to := wheelTurn([]*view{scroll}, 1, true); v != scroll || to.X <= scroll.offset.X {
		t.Fatalf("horizontal wheelTurn = (%v, %+v), want scroll moving right", v, to)
	}
	// Ctrl+wheel zooms around pointer and clamps to MaxZoomScale.
	if !w.wheelZoom(driver.Point{X: 100, Y: 100}, 120*10) {
		t.Fatal("wheelZoom did not claim zooming scroll view")
	}
	if scroll.zoomScale() != 4 {
		t.Fatalf("clamped wheelZoom = %v, want 4", scroll.zoomScale())
	}
}

func TestOffscreenCanvasScaleFollowsWorldTransform(t *testing.T) {
	if got := offscreenScale(100, 100, 2, identity); got != 2 {
		t.Fatalf("identity offscreenScale = %v, want 2", got)
	}
	if got := offscreenScale(100, 100, 2, scaling(2.2, 2.2)); got != 5 {
		t.Fatalf("2.2x offscreenScale = %v, want 5 (2 * RasterStep(2.2)=2.5)", got)
	}
	if got := offscreenScale(100, 100, 2, scaling(8, 8)); got != 8 {
		t.Fatalf("8x offscreenScale = %v, want 8 (capped at 4x)", got)
	}
	// 2048x2048 at baseScale 2 is already 4096x4096 -> cannot upscale past budget.
	if got := offscreenScale(2048, 2048, 2, scaling(3, 3)); got != 2 {
		t.Fatalf("at-budget offscreenScale = %v, want 2", got)
	}
}
