package linux

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// GDK/X11 keysyms for WebInput translation.
const (
	webKeyShiftL    uint32 = 0xffe1
	webKeyBackSpace uint32 = 0xff08
	webKeyTab       uint32 = 0xff09
	webKeyReturn    uint32 = 0xff0d
	webKeyEscape    uint32 = 0xff1b
	webKeySpace     uint32 = 0x0020
	webKeyHome      uint32 = 0xff50
	webKeyLeft      uint32 = 0xff51
	webKeyUp        uint32 = 0xff52
	webKeyRight     uint32 = 0xff53
	webKeyDown      uint32 = 0xff54
	webKeyPageUp    uint32 = 0xff55
	webKeyPageDown  uint32 = 0xff56
	webKeyEnd       uint32 = 0xff57
	webKeyF1        uint32 = 0xffbe
	webKeyDelete    uint32 = 0xffff
)

// gdkKeymapKey matches GDK 4's GdkKeymapKey layout.
type gdkKeymapKey struct {
	keycode uint32
	group   int32
	level   int32
}

// webLinuxStep is one character step produced from WebInputText.
type webLinuxStep struct {
	keyval uint32
	text   string
}

var webNamedKeyvals = map[string]uint32{
	"enter":      webKeyReturn,
	"return":     webKeyReturn,
	"tab":        webKeyTab,
	"escape":     webKeyEscape,
	"esc":        webKeyEscape,
	"backspace":  webKeyBackSpace,
	"delete":     webKeyDelete,
	"space":      webKeySpace,
	" ":          webKeySpace,
	"left":       webKeyLeft,
	"arrowleft":  webKeyLeft,
	"up":         webKeyUp,
	"arrowup":    webKeyUp,
	"right":      webKeyRight,
	"arrowright": webKeyRight,
	"down":       webKeyDown,
	"arrowdown":  webKeyDown,
	"home":       webKeyHome,
	"end":        webKeyEnd,
	"pageup":     webKeyPageUp,
	"pagedown":   webKeyPageDown,
}

// webCSSPoint converts a CSS-pixel viewport point to widget points at page zoom.
func webCSSPoint(pt driver.Point, zoom float64) (float32, float32) {
	if zoom <= 0 || math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		zoom = 1
	}
	return float32(float64(pt.X) * zoom), float32(float64(pt.Y) * zoom)
}

// webSurfacePixel converts a point in GtkWindow coordinates plus surface
// transform into X11 surface pixels at scale.
func webSurfacePixel(wx, wy, tx, ty, scale float64) (int32, int32) {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = 1
	}
	return int32(math.Round((wx + tx) * scale)), int32(math.Round((wy + ty) * scale))
}

// webKeyvalForName resolves a KeyboardEvent.key name to its GDK/X11 keysym.
func webKeyvalForName(name string) (uint32, bool) {
	if name == "" {
		return 0, false
	}
	lower := strings.ToLower(name)
	if kv, ok := webNamedKeyvals[lower]; ok {
		return kv, true
	}
	if strings.HasPrefix(lower, "f") {
		if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 12 {
			return webKeyF1 + uint32(n-1), true
		}
	}
	if utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		if kv := webKeyvalForRune(r); kv != 0 {
			return kv, true
		}
	}
	return 0, false
}

// webKeyvalForRune converts a Unicode rune to its GDK/X11 keysym, or 0 for
// control characters that have no keysym.
func webKeyvalForRune(r rune) uint32 {
	switch {
	case r == '\n' || r == '\r':
		return webKeyReturn
	case r == '\t':
		return webKeyTab
	case r >= 0x20 && r <= 0x7e:
		return uint32(r)
	case r >= 0xa0 && r <= 0xff:
		return uint32(r)
	case r >= 0x100 && utf8.ValidRune(r):
		return 0x01000000 | uint32(r)
	default:
		return 0
	}
}

// chooseWebKeymapKey picks the best hardware keycode and shift state for a
// keyval from gdk_display_map_keyval's matches. It accepts only shift level 0
// or 1 so characters requiring AltGr or absent from the keymap fall back to
// WebKit's InsertText editing command.
func chooseWebKeymapKey(keys []gdkKeymapKey) (keycode uint32, shift bool, ok bool) {
	bestGroup := int32(math.MaxInt32)
	bestLevel := int32(math.MaxInt32)
	for _, k := range keys {
		if k.keycode == 0 || k.level < 0 || k.level > 1 {
			continue
		}
		if !ok || k.group < bestGroup || (k.group == bestGroup && k.level < bestLevel) {
			keycode, shift, ok = k.keycode, k.level == 1, true
			bestGroup, bestLevel = k.group, k.level
		}
	}
	return keycode, shift, ok
}

// webInputTextSteps splits text into ordered rune steps, collapsing CRLF pairs
// into a single Return key step.
func webInputTextSteps(text string) []webLinuxStep {
	if text == "" {
		return nil
	}
	var out []webLinuxStep
	prevCR := false
	for _, r := range text {
		if r == '\n' && prevCR {
			prevCR = false
			continue
		}
		prevCR = r == '\r'
		s := string(r)
		if r == '\r' {
			s = "\n"
		}
		out = append(out, webLinuxStep{keyval: webKeyvalForRune(r), text: s})
	}
	return out
}
