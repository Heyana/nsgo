//go:build linux && !android

package linux

import (
	"math"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
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
		// Modifiers, Caps Lock, Print and Menu: keyvals that are no characters.
		{0xffe1, false, ""}, {0xffe5, false, ""}, {0xff61, false, ""}, {0xff67, false, ""},
	} {
		if got := keyName(c.keyval, c.shift); got != c.want {
			t.Errorf("keyName(0x%X, %v) = %q, want %q", c.keyval, c.shift, got, c.want)
		}
	}
}

func TestPortalTriggersUseXDGNames(t *testing.T) {
	for _, test := range []struct {
		key, want string
		ok        bool
	}{
		{"mod+shift+s", "CTRL+SHIFT+s", true},
		{"meta+alt+space", "LOGO+ALT+space", true},
		{"ctrl+f12", "CTRL+F12", true},
		{"ctrl+plus", "CTRL+plus", true},
		{"pageup", "Page_Up", true},
		{"unknown+a", "", false},
		{"shift", "", false},
	} {
		got, ok := portalTrigger(test.key)
		if got != test.want || ok != test.ok {
			t.Errorf("portalTrigger(%q) = (%q, %v), want (%q, %v)", test.key, got, ok, test.want, test.ok)
		}
	}
}

func TestPortalQuoteEscapesGVariantStrings(t *testing.T) {
	if got := portalQuote("a'b\\c"); got != "'a\\'b\\\\c'" {
		t.Fatalf("portalQuote = %q", got)
	}
}

func TestX11TriggerModifiers(t *testing.T) {
	mods, ok := x11TriggerModifiers([]string{"CTRL", "ALT", "SHIFT", "LOGO"})
	if !ok || mods != x11ControlMask|x11Mod1Mask|x11ShiftMask|x11Mod4Mask {
		t.Fatalf("x11TriggerModifiers = (%#x, %v)", mods, ok)
	}
	if _, ok := x11TriggerModifiers([]string{"NOPE"}); ok {
		t.Fatal("unknown X11 modifier was accepted")
	}
}

// TestX11ShortcutsIgnoreLocksGroupsAndButtons: a grabbed press matches its combination whatever locks, pointer buttons and XKB layout group its state also carries.
func TestX11ShortcutsIgnoreLocksGroupsAndButtons(t *testing.T) {
	x := &x11HotKeys{bindings: map[x11HotKey]uint32{{keycode: 45, modifiers: x11ControlMask | x11Mod1Mask}: 7}}
	press := func(kind int32, state, keycode uint32) (uint32, bool) {
		var ev [24]uint64 // sizeof(XEvent) on LP64
		p := unsafe.Pointer(&ev[0])
		*(*int32)(p) = kind
		*(*uint32)(unsafe.Add(p, 80)) = state
		*(*uint32)(unsafe.Add(p, 84)) = keycode
		return x.match(p)
	}
	const button1, group2 = 1 << 8, 1 << 13
	if id, ok := press(x11KeyPress, x11ControlMask|x11Mod1Mask|x11LockMask|x11Mod2Mask|button1|group2, 45); !ok || id != 7 {
		t.Errorf("a press with locks, a button and the second group = (%d, %v), want (7, true)", id, ok)
	}
	if _, ok := press(x11KeyPress, x11ControlMask|x11Mod1Mask|x11ShiftMask, 45); ok {
		t.Error("a press with Shift added matched Ctrl+Alt")
	}
	if _, ok := press(x11KeyPress+1, x11ControlMask|x11Mod1Mask, 45); ok {
		t.Error("a key release matched")
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
	if activates(driver.PanActivation{}, driver.Point{X: 5, Y: 5}) {
		t.Fatal("a short drag activated a pan")
	}
	if !activates(driver.PanActivation{}, driver.Point{X: 8, Y: 8}) {
		t.Fatal("a drag past the slop did not activate")
	}
	a := driver.PanActivation{ActiveOffsetX: 20, ActiveOffsetY: 40}
	if !activates(a, driver.Point{X: 25}) || activates(a, driver.Point{X: 10, Y: 10}) {
		t.Fatal("active offsets")
	}
	a.FailOffsetY = 30
	if !fails(a, driver.Point{Y: 50}) || fails(a, driver.Point{Y: 20}) {
		t.Fatal("fail offsets")
	}
	if rest(driver.EventPinch) != 1 || rest(driver.EventRotation) != 0 {
		t.Fatal("gesture rest values")
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
	if mixed.BorderWidth != 10 || mixed.Transform.TranslateX != 20 {
		t.Fatalf("untouched fields = %+v", mixed)
	}
	full := driver.MixStyles(from, to, 0.5, allProperties)
	if full.Transform.TranslateX != 10 || full.ShadowRadius != 2 {
		t.Fatalf("full mix = %+v", full)
	}
	same := driver.MixStyles(to, to, 0.5, allProperties)
	if same.Opacity != to.Opacity || same.Background != to.Background || same.BorderWidth != to.BorderWidth ||
		same.Transform.TranslateX != to.Transform.TranslateX {
		t.Fatalf("unchanged = %+v, want %+v", same, to)
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
	s := &imageSlot{src: driver.ImageSource{}, pic: &picture{width: 400, height: 200}}
	r := rect{x: 5, w: 100, h: 100}
	fit := func(mode driver.ResizeMode, pos driver.ObjectPosition, s *imageSlot) rect {
		return w.fit(&view{image: driver.ImageConfig{ResizeMode: mode, Position: pos}}, s, r)
	}
	if dst := fit(driver.Stretch, driver.ObjectPosition{}, s); dst != r {
		t.Fatalf("stretch = %+v", dst)
	}
	if dst := fit(driver.Contain, driver.ObjectPosition{}, s); dst != (rect{x: 5, y: 25, w: 100, h: 50}) {
		t.Fatalf("contain = %+v", dst)
	}
	if dst := fit(driver.Cover, driver.ObjectPosition{}, s); dst != (rect{x: -45, w: 200, h: 100}) {
		t.Fatalf("cover = %+v", dst)
	}
	if dst := fit(driver.Cover, driver.ObjectPosition{X: core.PositionEnd}, s); dst != (rect{x: -95, w: 200, h: 100}) {
		t.Fatalf("cover at the right = %+v", dst)
	}
	if dst := fit(driver.Center, driver.ObjectPosition{}, s); dst != (rect{x: -145, y: -50, w: 400, h: 200}) {
		t.Fatalf("center of a large picture = %+v", dst)
	}
	if dst := fit(driver.ScaleDown, driver.ObjectPosition{}, s); dst != (rect{x: 5, y: 25, w: 100, h: 50}) {
		t.Fatalf("scale-down of a large picture = %+v", dst)
	}
	small := &imageSlot{src: driver.ImageSource{}, pic: &picture{width: 40, height: 20}}
	if dst := fit(driver.ScaleDown, driver.ObjectPosition{}, small); dst != (rect{x: 35, y: 40, w: 40, h: 20}) {
		t.Fatalf("scale-down = %+v", dst)
	}
	small.src.URI = "logo@2x.png"
	if dst := fit(driver.Center, driver.ObjectPosition{}, small); dst != (rect{x: 45, y: 45, w: 20, h: 10}) {
		t.Fatalf("center @2x = %+v", dst)
	}
	small.src.Size = driver.Size{Width: 80, Height: 40}
	if dst := fit(driver.Center, driver.ObjectPosition{Y: core.PositionStart}, small); dst != (rect{x: 15, w: 80, h: 40}) {
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
	w.focus = &view{}
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
		if !driver.MenuRoleUnsupported(r, driver.OSLinux, false) {
			t.Errorf("%v is supported", r)
		}
	}
	if driver.MenuRoleUnsupported(driver.MenuQuit, driver.OSLinux, false) || driver.MenuRoleUnsupported(driver.MenuCopy, driver.OSLinux, false) {
		t.Fatal("a supported role is unsupported")
	}
	for _, speaks := range []bool{false, true} {
		speechAs(t, speaks)
		if driver.MenuRoleUnsupported(driver.MenuStartSpeaking, driver.OSLinux, speaks) == speaks || driver.MenuRoleUnsupported(driver.MenuStopSpeaking, driver.OSLinux, speaks) == speaks {
			t.Errorf("with speech %v, the Speech items are unsupported: %v", speaks, !speaks)
		}
	}
	if got := shortcutOf(&driver.MenuEntry{Role: driver.MenuQuit}); got != "alt+f4" {
		t.Fatalf("quit shortcut = %q", got)
	}
	if got := shortcutOf(&driver.MenuEntry{Role: driver.MenuQuit, Shortcut: "mod+q"}); got != "mod+q" {
		t.Fatalf("given shortcut = %q", got)
	}
}

func TestEntryStyleCarriesTheTextStyle(t *testing.T) {
	v := &view{id: 4, editorNum: 4, input: driver.InputConfig{Style: driver.TextStyle{
		Font:  driver.Font{Family: "Noto Sans", Size: 15, Weight: driver.FontWeightBold, Italic: true},
		Color: driver.RGB(0x123456), Align: driver.TextAlignEnd,
	}, PlaceholderColor: driver.RGB(0xff0000), SelectionColor: driver.RGB(0x00ff00)}}
	css := entryStyle(v)
	for _, want := range []string{"transparent", "#nsgo-input-4 *", "placeholder", "rgba(255,0,0,1.000)", "selection { background-color: rgba(0,255,0,0.302)", "caret-color: rgba(0,255,0", `font-family: "Noto Sans"`, "font-size: 15.000px", "font-weight: 700",
		"font-style: italic", "rgba(18,52,86"} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS lacks %q:\n%s", want, css)
		}
	}
}

// TestEntryStyleCarriesTheLineHeight checks that a text view and its placeholder take the line height and the font, and that a field's one line does not (#310).
func TestEntryStyleCarriesTheLineHeight(t *testing.T) {
	v := &view{id: 4, editorNum: 4, input: driver.InputConfig{Multiline: true, Style: driver.TextStyle{Font: driver.Font{Size: 15}, LineHeight: 25.5}}}
	css := entryStyle(v)
	for _, want := range []string{"#nsgo-input-4-hint { padding", "#nsgo-input-4-hint { font-size: 15.000px", "#nsgo-input-4-hint { line-height: 25.500px; }"} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS lacks %q:\n%s", want, css)
		}
	}
	v.input.Multiline = false
	if css := entryStyle(v); strings.Contains(css, "line-height") {
		t.Errorf("a field's CSS sets a line height:\n%s", css)
	}
}

// TestMultilineEditorBounds checks that a multiline input's scrolled window
// fills the padding box while a single-line entry stays at the content box (#1598).
func TestMultilineEditorBounds(t *testing.T) {
	pad := driver.Edges{Top: 18, Right: 20, Bottom: 20, Left: 12}
	content := driver.Bounds{Origin: driver.Point{X: 13, Y: 19}, Size: driver.Size{Width: 266, Height: 160}}
	in := driver.InputConfig{Multiline: true, Padding: pad, Content: content}
	want := driver.Bounds{Origin: driver.Point{X: 1, Y: 1}, Size: driver.Size{Width: 298, Height: 198}}
	if got := editorBounds(&in); got != want {
		t.Errorf("multiline editorBounds = %+v, want padding box %+v", got, want)
	}
	in.Multiline = false
	if got := editorBounds(&in); got != content {
		t.Errorf("single-line editorBounds = %+v, want content box %+v", got, content)
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
	for mode, want := range map[driver.ResizeMode]int32{
		driver.Cover: gtkContentFitCover, driver.Contain: gtkContentFitContain,
		driver.Stretch: gtkContentFitFill, driver.Center: gtkContentFitScaleDown,
	} {
		if got := contentFit(mode); got != want {
			t.Errorf("contentFit(%d) = %d, want %d", mode, got, want)
		}
	}
}

func TestAStatusReadsItsWholeText(t *testing.T) {
	text := func(s string) *view { return &view{kind: driver.KindText, plain: s} }
	changed := text("3")
	hidden := text("hidden")
	hidden.style.Hidden = true
	status := &view{kind: driver.KindView, props: driver.ViewProps{Role: driver.RoleStatus}, children: []*view{
		text("Saved"), {kind: driver.KindView, children: []*view{changed, text("files")}},
		{kind: driver.KindImage, props: driver.ViewProps{Label: "done"}}, hidden,
	}}
	if got := statusText(status); got != "Saved 3 files done" {
		t.Errorf("a status reads %q", got)
	}
	status.props.Label = "Saving"
	if got := statusText(status); got != "Saving" {
		t.Errorf("a labelled status reads %q", got)
	}
	if got := statusText(text("1 item")); got != "1 item" {
		t.Errorf("a status text reads %q", got)
	}
}

// TestMenuShowsNoMacRolesNorEmptySubmenus checks which entries the GTK menu shows: neither macOS's roles, nor the Speech items where nothing speaks, nor a Help holder with no items, nor a submenu none of whose items it shows.
func TestMenuShowsNoMacRolesNorEmptySubmenus(t *testing.T) {
	for _, speech := range []bool{false, true} {
		speechAs(t, speech)
		for _, c := range []struct {
			e    driver.MenuEntry
			want bool
		}{
			{driver.MenuEntry{Label: "Copy", Role: driver.MenuCopy}, true},
			{driver.MenuEntry{Role: driver.MenuStartSpeaking}, speech},
			{driver.MenuEntry{Role: driver.MenuServices, Items: []driver.MenuEntry{{Label: "Mine", ID: 1}}}, false},
			{driver.MenuEntry{Label: "Help", Role: driver.MenuHelpMenu}, false},
			{driver.MenuEntry{Label: "Help", Role: driver.MenuHelpMenu, Items: []driver.MenuEntry{{Label: "Docs", ID: 1}}}, true},
			{driver.MenuEntry{Label: "Speech", Items: []driver.MenuEntry{{Role: driver.MenuStartSpeaking}, {Separator: true}, {Role: driver.MenuStopSpeaking}}}, speech},
			{driver.MenuEntry{Label: "Empty"}, true},
		} {
			if got := shown(&c.e); got != c.want {
				t.Errorf("with speech %v, shown(%+v) = %v, want %v", speech, c.e, got, c.want)
			}
		}
	}
}

// speechAs has the backend say speech is available, or not, for the rest of the test.
func speechAs(t *testing.T, ok bool) {
	t.Helper()
	was := speakAvailable
	speakAvailable = func() bool { return ok }
	t.Cleanup(func() { speakAvailable = was })
}

// TestStartSpeakingReadsTheSelection checks what Start Speaking reads of a field: the characters GTK's offsets select, in either order, which are not bytes, or the whole text without a selection.
func TestStartSpeakingReadsTheSelection(t *testing.T) {
	for _, c := range []struct {
		text       string
		start, end int
		want       string
	}{
		{"hello world", 6, 11, "world"},
		{"hello world", 3, 3, "hello world"},
		{"日本語のテキスト", 4, 8, "テキスト"},
		{"日本語のテキスト", 3, 0, "日本語"},
		{"a😀b", 1, 2, "😀"},
		{"abc", 1, 9, "bc"},
	} {
		if got := speechText(c.text, c.start, c.end); got != c.want {
			t.Errorf("speechText(%q, %d, %d) = %q, want %q", c.text, c.start, c.end, got, c.want)
		}
	}
}

// TestTitleBarDoubleClickFollowsGTK maps gtk-titlebar-double-click's values to what GTK 4's header bars do with them.
func TestTitleBarDoubleClickFollowsGTK(t *testing.T) {
	for setting, want := range map[string]titleBarAction{
		"": titleBarToggleMaximize, "toggle-maximize": titleBarToggleMaximize,
		"toggle-maximize-horizontally": titleBarToggleMaximize, "toggle-maximize-vertically": titleBarToggleMaximize,
		"minimize": titleBarMinimize, "lower": titleBarLower, "none": titleBarNone, "menu": titleBarShowMenu,
	} {
		if got := titleBarDoubleClickAction(setting); got != want {
			t.Errorf("titleBarDoubleClickAction(%q) = %d, want %d", setting, got, want)
		}
	}
}

func TestTextDecorationAndContentBounds(t *testing.T) {
	l := &textLayout{
		width:  60,
		height: 18,
		under:  []rect{{x: 0, y: -3, w: 60, h: 24}},
		over: []decorationLine{
			{x: 0, y: 28, w: 60, t: 4, style: driver.DecorationSolid},
			{x: 0, y: 24, w: 60, t: 3, style: driver.DecorationDouble},
			{x: 0, y: -5, w: 60, t: 3, style: driver.DecorationWavy},
		},
	}
	dec, ok := l.decorationBounds(4, 6)
	if !ok {
		t.Fatal("decorationBounds reported no decorations")
	}
	// Wavy at y=6-5=1 with amp=max(1.5*3,2)=4.5 and AA=1 reaches top <= -5;
	// solid at y=6+28=34 with t=4 and AA=1 reaches bottom >= 39.
	if dec.y > -5 || dec.y+dec.h < 39 || dec.x > 2 || dec.x+dec.w < 66 {
		t.Fatalf("decorationBounds = %+v, want top <= -5, bottom >= 39, left <= 2, right >= 66", dec)
	}
	cb := l.contentBounds(4, 6, 60, 18)
	if cb.y > dec.y || cb.y+cb.h < dec.y+dec.h || cb.x > dec.x || cb.x+cb.w < dec.x+dec.w {
		t.Fatalf("contentBounds = %+v, want to cover decorationBounds %+v", cb, dec)
	}
	// Empty over reports false.
	if _, ok := (&textLayout{width: 40, height: 16}).decorationBounds(0, 0); ok {
		t.Fatal("decorationBounds reported true for empty over")
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
	prevSource := gdkDeviceGetSource
	gdkDeviceGetSource = func(uintptr) int32 { return gdkSourceTouchscreen }
	t.Cleanup(func() { gdkDeviceGetSource = prevSource })

	var events []driver.Event
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) { events = append(events, ev) },
	})
	t.Cleanup(eng.Close)

	visible := driver.ViewStyle{Opacity: 1}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 400}}}
	scroll := &view{
		id:     2,
		style:  visible,
		kind:   driver.KindScroll,
		parent: root,
		frame:  driver.Bounds{Size: driver.Size{Width: 300, Height: 400}},
		size:   driver.Size{Width: 300, Height: 2000},
		events: 1<<driver.EventScrollDragStart | 1<<driver.EventScrollDragEnd |
			1<<driver.EventMomentumStart | 1<<driver.EventMomentumEnd | 1<<driver.EventScrollEnd,
	}
	child := &view{
		id:     3,
		style:  visible,
		kind:   driver.KindView,
		parent: scroll,
		frame:  driver.Bounds{Size: driver.Size{Width: 300, Height: 2000}},
		events: 1<<driver.EventPressDown | 1<<driver.EventPressUp | 1<<driver.EventPress,
	}
	scroll.children = []*view{child}
	root.children = []*view{scroll}
	w := &window{
		engine:   eng,
		root:     root.id,
		views:    []*view{nil, root, scroll, child},
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
	const touchDevice uintptr = 1
	at := driver.Point{X: 100, Y: 200}

	// 1. Touch down stopping a glide (and touchpad wheelMotion) presses nothing when it lifts.
	m := scrollbar.Release(&scroll.scroll, scroll.frame.Size, scroll.size, driver.Point{Y: 100}, 1.2, 0, 1600, time.Now())
	scroll.offset = driver.Point{Y: 100}
	scroll.glide = &m
	w.wheelMotion = &m
	events = nil
	w.pointerDown(at, 1, touchDevice, 0, 1)
	if scroll.glide != nil || w.wheelMotion != nil {
		t.Fatal("braking pointerDown did not stop glide and wheelMotion")
	}
	if !w.touchMove(driver.Point{X: 100, Y: 196}) {
		t.Fatal("sub-slop touchMove after braking returned false; want true")
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking without drag returned false; want true")
	}
	w.pointerUp(at)
	if count(driver.EventPressDown) != 0 || count(driver.EventPressUp) != 0 || count(driver.EventPress) != 0 {
		t.Errorf("braking touch pressed child: %+v", events)
	}
	if got := count(driver.EventMomentumEnd); got != 1 {
		t.Errorf("momentum end count = %d, want 1", got)
	}
	if count(driver.EventScrollDragStart) != 0 || count(driver.EventScrollDragEnd) != 0 || count(driver.EventScrollEnd) != 0 {
		t.Errorf("braking tap emitted unexpected scroll events: %+v", events)
	}

	// 2. Touch down stopping a snap presses nothing when it lifts.
	scroll.scrolling = &scrollbar.OffsetAnimation{From: driver.Point{Y: 100}, To: driver.Point{Y: 400}, Start: time.Now(), Momentum: true}
	events = nil
	w.pointerDown(at, 1, touchDevice, 0, 1)
	if scroll.scrolling != nil {
		t.Fatal("braking pointerDown did not stop snap")
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking snap returned false; want true")
	}
	w.pointerUp(at)
	if count(driver.EventPressDown) != 0 || count(driver.EventPressUp) != 0 || count(driver.EventPress) != 0 {
		t.Errorf("snap braking touch pressed child: %+v", events)
	}
	if got := count(driver.EventMomentumEnd); got != 1 {
		t.Errorf("snap brake momentum end count = %d, want 1", got)
	}
	if count(driver.EventScrollDragStart) != 0 || count(driver.EventScrollDragEnd) != 0 || count(driver.EventScrollEnd) != 0 {
		t.Errorf("snap braking tap emitted unexpected scroll events: %+v", events)
	}

	// 3. Touch down stopping an animated ScrollTo presses nothing and does not emit momentum end.
	scroll.scrolling = &scrollbar.OffsetAnimation{From: driver.Point{Y: 100}, To: driver.Point{Y: 400}, Start: time.Now()}
	events = nil
	w.pointerDown(at, 1, touchDevice, 0, 1)
	if scroll.scrolling != nil {
		t.Fatal("braking pointerDown did not stop animated scroll")
	}
	if !w.touchUp() {
		t.Fatal("touchUp after braking animated scroll returned false; want true")
	}
	w.pointerUp(at)
	if count(driver.EventPressDown) != 0 || count(driver.EventPressUp) != 0 || count(driver.EventPress) != 0 {
		t.Errorf("animated scroll brake pressed child: %+v", events)
	}
	if count(driver.EventMomentumEnd) != 0 {
		t.Errorf("animated scroll brake reported momentum end: %+v", events)
	}

	// 4. Braking touch that drags past TouchSlop scrolls smoothly without pressing child.
	scroll.glide = &m
	scroll.offset = driver.Point{Y: 100}
	events = nil
	w.pointerDown(at, 1, touchDevice, 0, 1)
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
	w.pointerUp(driver.Point{X: 100, Y: 110})
	if count(driver.EventScrollDragEnd) != 1 {
		t.Errorf("drag end count = %d, want 1", count(driver.EventScrollDragEnd))
	}
	if count(driver.EventPressDown) != 0 || count(driver.EventPressUp) != 0 || count(driver.EventPress) != 0 {
		t.Errorf("braking drag pressed child: %+v", events)
	}
	scroll.glide = nil

	// 5. Touch at rest delivers press down, press up and press to the child.
	events = nil
	w.pointerDown(at, 1, touchDevice, 0, 1)
	if w.touchMove(driver.Point{X: 100, Y: 196}) {
		t.Fatal("sub-slop touchMove at rest returned true; want false")
	}
	if w.touchUp() {
		t.Fatal("touchUp at rest without drag returned true; want false")
	}
	w.pointerUp(at)
	if count(driver.EventPressDown) != 1 || count(driver.EventPressUp) != 1 || count(driver.EventPress) != 1 {
		t.Errorf("tap at rest events = %+v; want 1 PressDown, 1 PressUp, 1 Press", events)
	}

	// 6. NoScroll leaves touches alone.
	scroll.scroll.Behavior.NoScroll = true
	scroll.glide = &m
	events = nil
	w.pointerDown(at, 1, touchDevice, 0, 1)
	w.touchUp()
	w.pointerUp(at)
	if scroll.glide == nil {
		t.Fatal("NoScroll view had its glide cleared")
	}
	if count(driver.EventPressDown) != 1 || count(driver.EventPressUp) != 1 || count(driver.EventPress) != 1 {
		t.Errorf("tap on NoScroll view events = %+v; want 1 PressDown, 1 PressUp, 1 Press", events)
	}
}

func TestFontMetricsStayBounded(t *testing.T) {
	var m map[fontKey]driver.Points
	for i := range maxFontMetrics * 3 {
		remember(&m, fontKey{font: driver.Font{Size: driver.Points(i)}}, driver.Points(i))
		if len(m) > maxFontMetrics {
			t.Fatalf("%d metrics cached, want at most %d", len(m), maxFontMetrics)
		}
	}
	last := fontKey{font: driver.Font{Size: maxFontMetrics*3 - 1}}
	if m[last] != maxFontMetrics*3-1 {
		t.Error("the latest metric is not cached")
	}
}

func TestShieldedTouchpadScrollGlidesAndInterrupts(t *testing.T) {
	var events []driver.Event
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) { events = append(events, ev) },
	})
	t.Cleanup(eng.Close)

	visible := driver.ViewStyle{Opacity: 1}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 400}}}
	scroll := &view{
		id:     2,
		style:  visible,
		kind:   driver.KindScroll,
		parent: root,
		frame:  driver.Bounds{Size: driver.Size{Width: 300, Height: 400}},
		size:   driver.Size{Width: 300, Height: 10000},
		events: 1<<driver.EventMomentumStart | 1<<driver.EventMomentumEnd | 1<<driver.EventScrollEnd,
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

	at := driver.Point{X: 100, Y: 100}
	now := time.Now()

	// 1. Shielded smooth surface-unit scroll deltas followed by a stop event
	// glide past the deltas, decelerate smoothly, and come to rest.
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 1000, now)
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 1008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 1016, now.Add(16*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 0, true, true, true, 1016, now.Add(16*time.Millisecond))
	if scroll.offset.Y != 48 {
		t.Fatalf("offset after deltas = %v, want 48", scroll.offset.Y)
	}
	if scroll.glide == nil || w.wheelMotion == nil {
		t.Fatal("shielded touchpad stop did not start wheel glide")
	}
	if count(driver.EventMomentumStart) != 1 {
		t.Fatalf("momentum start count = %d, want 1", count(driver.EventMomentumStart))
	}
	startOffset := scroll.offset.Y
	stepTime := time.Now()
	prevStep := driver.Points(0)
	prevPos := startOffset
	done := false
	for i := 1; i <= 400; i++ {
		if !w.stepGlide(scroll, stepTime.Add(time.Duration(i)*16*time.Millisecond)) {
			done = true
			break
		}
		step := scroll.offset.Y - prevPos
		if i > 2 && step > prevStep+0.01 {
			t.Fatalf("glide step %d accelerated: %v > %v", i, step, prevStep)
		}
		prevStep, prevPos = step, scroll.offset.Y
	}
	if !done {
		t.Fatal("glide did not finish")
	}
	if scroll.offset.Y <= startOffset+200 {
		t.Fatalf("glided to %v, want well past %v", scroll.offset.Y, startOffset)
	}
	if scroll.glide != nil || w.wheelMotion != nil {
		t.Fatal("completed glide left glide or wheelMotion set")
	}
	if count(driver.EventMomentumEnd) != 1 || count(driver.EventScrollEnd) != 1 {
		t.Fatalf("end events: momentumEnd=%d scrollEnd=%d, want 1/1", count(driver.EventMomentumEnd), count(driver.EventScrollEnd))
	}

	// 2. Touchpad hold (fingers landing over the shield) interrupts an active glide.
	events = nil
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 2000, now)
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 2008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 0, true, true, true, 2008, now.Add(8*time.Millisecond))
	if scroll.glide == nil || w.wheelMotion == nil {
		t.Fatal("second shielded flick did not start glide")
	}
	w.shieldTouchpadHold(gdkTouchpadPhaseBegin, 2040, now.Add(40*time.Millisecond))
	if scroll.glide != nil || w.wheelMotion != nil {
		t.Fatal("shielded touchpad hold did not stop active glide")
	}
	if count(driver.EventMomentumEnd) != 1 {
		t.Fatalf("momentum end after hold = %d, want 1", count(driver.EventMomentumEnd))
	}
	// Lifting fingers without scrolling clears the hold without starting a glide.
	w.shieldTouchpadHold(gdkTouchpadPhaseEnd, 2080, now.Add(80*time.Millisecond))
	if w.shieldScroll.active || scroll.glide != nil {
		t.Fatal("hold end without deltas left tracker active or started glide")
	}

	// 3. A new shielded scroll without a hold also stops an active glide.
	events = nil
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 3000, now)
	w.shieldScrollEvent(0, at, 0, 16, true, true, false, 3008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 0, true, true, true, 3008, now.Add(8*time.Millisecond))
	if scroll.glide == nil {
		t.Fatal("third shielded flick did not start glide")
	}
	w.shieldScrollEvent(0, at, 0, 10, true, true, false, 3050, now.Add(50*time.Millisecond))
	if scroll.glide != nil || w.wheelMotion != nil {
		t.Fatal("new shielded scroll did not stop previous glide")
	}
	if count(driver.EventMomentumEnd) != 1 {
		t.Fatalf("momentum end after new scroll = %d, want 1", count(driver.EventMomentumEnd))
	}
	// Pausing before stop ends cleanly without gliding.
	w.shieldScrollEvent(0, at, 0, 0, true, true, true, 3250, now.Add(250*time.Millisecond))
	if scroll.glide != nil || w.wheelMotion != nil {
		t.Fatal("paused stop unexpectedly started glide")
	}

	// 4. Discrete mouse wheel and Control+scroll never start a glide.
	w.shieldScrollEvent(0, at, 0, 1, false, false, false, 4000, now)
	if w.shieldScroll.active || scroll.glide != nil {
		t.Fatal("discrete wheel activated shieldScroll or started glide")
	}
	w.shieldScrollEvent(gdkControl, at, 0, 16, true, true, false, 4100, now)
	w.shieldScrollEvent(gdkControl, at, 0, 16, true, true, false, 4108, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(gdkControl, at, 0, 0, true, true, true, 4108, now.Add(8*time.Millisecond))
	if w.shieldScroll.active || scroll.glide != nil {
		t.Fatal("Control+scroll activated shieldScroll or started glide")
	}

	// 5. Shift swaps axes so vertical touchpad flicks glide a horizontal scroll view.
	scroll.scroll.Horizontal = true
	scroll.size = driver.Size{Width: 10000, Height: 400}
	scroll.offset = driver.Point{}
	w.shieldScrollEvent(gdkShift, at, 0, 16, true, true, false, 5000, now)
	w.shieldScrollEvent(gdkShift, at, 0, 16, true, true, false, 5008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(gdkShift, at, 0, 0, true, true, true, 5008, now.Add(8*time.Millisecond))
	if scroll.offset.X != 32 || scroll.glide == nil {
		t.Fatalf("Shift horizontal scroll: offset.X=%v glide=%v, want 32 and non-nil", scroll.offset.X, scroll.glide != nil)
	}
	w.stopWheelGlide()

	// 6. ReduceMotion suppresses gliding.
	w.env.ReduceMotion = true
	w.shieldScrollEvent(gdkShift, at, 0, 16, true, true, false, 6000, now)
	w.shieldScrollEvent(gdkShift, at, 0, 16, true, true, false, 6008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(gdkShift, at, 0, 0, true, true, true, 6008, now.Add(8*time.Millisecond))
	if scroll.glide != nil {
		t.Fatal("ReduceMotion did not suppress shielded glide")
	}
}

func TestShieldedTouchpadScrollOnWheelAndCleanup(t *testing.T) {
	var events []driver.Event
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) { events = append(events, ev) },
	})
	t.Cleanup(eng.Close)

	visible := driver.ViewStyle{Opacity: 1}
	root := &view{id: 1, kind: driver.KindView, style: visible, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 400}}}
	taker := &view{
		id:     2,
		style:  visible,
		kind:   driver.KindView,
		parent: root,
		frame:  driver.Bounds{Size: driver.Size{Width: 300, Height: 400}},
		events: 1 << driver.EventWheel,
	}
	root.children = []*view{taker}
	w := &window{
		engine:   eng,
		root:     root.id,
		views:    []*view{nil, root, taker},
		animator: driver.NewAnimator(func(driver.Event) {}),
		ticking:  true,
	}

	at := driver.Point{X: 50, Y: 60}
	now := time.Now()
	w.shieldScrollEvent(0, at, 0, 12, true, true, false, 1000, now)
	w.shieldScrollEvent(0, at, 0, 12, true, true, false, 1008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 12, true, true, false, 1016, now.Add(16*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 0, true, true, true, 1016, now.Add(16*time.Millisecond))

	if len(events) != 4 {
		t.Fatalf("wheel events = %d, want 4 (3 changed + 1 ended)", len(events))
	}
	end := events[3]
	if end.Kind != driver.EventWheel || end.Phase != driver.GestureEnded || !end.Precise {
		t.Fatalf("end event = %+v, want precise EventWheel GestureEnded", end)
	}
	if math.Abs(float64(end.Velocity.Y)-(-1500)) > 5 {
		t.Fatalf("end Velocity.Y = %v, want ~-1500", end.Velocity.Y)
	}
	if w.wheelTaker != nil {
		t.Fatal("wheelTaker was not cleared after GestureEnded")
	}

	// Paused lift over OnWheel still sends GestureEnded with zero velocity.
	events = nil
	w.shieldScrollEvent(0, at, 0, 12, true, true, false, 2000, now)
	w.shieldScrollEvent(0, at, 0, 12, true, true, false, 2008, now.Add(8*time.Millisecond))
	w.shieldScrollEvent(0, at, 0, 0, true, true, true, 2200, now.Add(200*time.Millisecond))
	if len(events) != 3 || events[2].Phase != driver.GestureEnded || events[2].Velocity != (driver.Point{}) {
		t.Fatalf("paused OnWheel events = %+v, want final GestureEnded with zero velocity", events)
	}

	// pointerDown resets an in-progress shieldScroll cleanly.
	w.shieldScrollEvent(0, at, 0, 12, true, true, false, 3000, now)
	if !w.shieldScroll.active {
		t.Fatal("shieldScroll should be active")
	}
	w.pointerDown(at, 1, 0, 0, 1)
	if w.shieldScroll.active {
		t.Fatal("pointerDown did not reset shieldScroll")
	}
}

func TestBackdropPath(t *testing.T) {
	root := &view{id: 1, frame: driver.Bounds{Size: driver.Size{Width: 400, Height: 300}},
		style: driver.ViewStyle{Opacity: 1}}
	scroll := &view{id: 2, parent: root,
		frame:  driver.Bounds{Origin: driver.Point{X: 20, Y: 30}, Size: driver.Size{Width: 200, Height: 150}},
		offset: driver.Point{X: 5.4, Y: 10.6}, // rounds to (5, 11) as allocateView does
		style:  driver.ViewStyle{Opacity: 1}}
	host := &view{id: 3, parent: scroll,
		frame: driver.Bounds{Origin: driver.Point{X: 40, Y: 50}, Size: driver.Size{Width: 80, Height: 60}},
		style: driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Scale: 2}}}
	detached := &view{id: 4, frame: driver.Bounds{Size: driver.Size{Width: 10, Height: 10}},
		style: driver.ViewStyle{Opacity: 1}}

	if _, _, ok := backdropPath(nil, host); ok {
		t.Error("backdropPath(nil, host) succeeded")
	}
	if _, _, ok := backdropPath(root, nil); ok {
		t.Error("backdropPath(root, nil) succeeded")
	}
	if _, _, ok := backdropPath(root, root); ok {
		t.Error("backdropPath(root, root) succeeded; root has no backdrop")
	}
	if _, _, ok := backdropPath(root, detached); ok {
		t.Error("backdropPath(root, detached) succeeded")
	}

	path, surfaceToHost, ok := backdropPath(root, host)
	if !ok {
		t.Fatal("backdropPath(root, host) returned ok=false")
	}
	if len(path) != 2 || path[root] != scroll || path[scroll] != host {
		t.Fatalf("path = %v, want root->scroll->host", path)
	}
	// Verify surfaceToHost is the exact inverse of host's composed placement.
	hostToSurface := root.placement(0, 0).Mul(scroll.placement(0, 0)).Mul(host.placement(5, 11))
	for _, pt := range [][2]float64{{0, 0}, {40, 30}, {80, 60}, {-10, -5}} {
		sx, sy := hostToSurface.Apply(pt[0], pt[1])
		hx, hy := surfaceToHost.Apply(sx, sy)
		if !closeTo(hx, pt[0]) || !closeTo(hy, pt[1]) {
			t.Errorf("local (%g,%g) -> surface (%g,%g) -> local (%g,%g)", pt[0], pt[1], sx, sy, hx, hy)
		}
	}

	// Verify singular matrix (zero scale) is rejected cleanly.
	collapsed := &view{id: 6, parent: root,
		frame: driver.Bounds{Size: driver.Size{Width: 10, Height: 10}},
		style: driver.ViewStyle{Opacity: 1}}
	collapsed.layout = &layoutAnim{fade: &driver.LayoutTransition{Property: driver.LayoutScale}, shownPart: 0}
	if _, _, ok := backdropPath(root, collapsed); ok {
		t.Error("backdropPath with zero layout scale succeeded")
	}
}
