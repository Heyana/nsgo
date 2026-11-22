package ios

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// UIKit key modifier and press type constants from UIKeyConstants.h and UIPress.h.
const (
	webIOSModifierShift = 1 << 17 // UIKeyModifierShift

	webIOSPressTypeNone       = -1
	webIOSPressTypeUpArrow    = 0
	webIOSPressTypeDownArrow  = 1
	webIOSPressTypeLeftArrow  = 2
	webIOSPressTypeRightArrow = 3
)

// webIOSKeySpec describes one key or text step delivered to a WKWebView on iOS.
type webIOSKeySpec struct {
	chars         string
	plain         string
	usage         int // UIKeyboardHIDUsage (USB HID usage page 0x07)
	modifierFlags int // UIKeyModifierFlags
	pressType     int // UIPressType, or webIOSPressTypeNone
	insertText    bool
}

// webTapRecognizerInfo describes one UIGestureRecognizer attached to a
// WKWebView's content view for filtering tap delivery.
type webTapRecognizerInfo struct {
	enabled         bool
	isTap           bool
	tapsRequired    int
	touchesRequired int
	excluded        bool // long-press, pan, swipe, pinch, rotation, hover
}

var webIOSNamedKeys = map[string]webIOSKeySpec{
	"enter":      {chars: "\r", plain: "\r", usage: 0x28, pressType: webIOSPressTypeNone},
	"return":     {chars: "\r", plain: "\r", usage: 0x28, pressType: webIOSPressTypeNone},
	"escape":     {chars: "\x1b", plain: "\x1b", usage: 0x29, pressType: webIOSPressTypeNone},
	"esc":        {chars: "\x1b", plain: "\x1b", usage: 0x29, pressType: webIOSPressTypeNone},
	"backspace":  {chars: "\x7f", plain: "\x7f", usage: 0x2a, pressType: webIOSPressTypeNone},
	"tab":        {chars: "\t", plain: "\t", usage: 0x2b, pressType: webIOSPressTypeNone},
	"space":      {chars: " ", plain: " ", usage: 0x2c, pressType: webIOSPressTypeNone},
	" ":          {chars: " ", plain: " ", usage: 0x2c, pressType: webIOSPressTypeNone},
	"delete":     {chars: "\uf728", plain: "\uf728", usage: 0x4c, pressType: webIOSPressTypeNone},
	"right":      {chars: "\uf703", plain: "\uf703", usage: 0x4f, pressType: webIOSPressTypeRightArrow},
	"arrowright": {chars: "\uf703", plain: "\uf703", usage: 0x4f, pressType: webIOSPressTypeRightArrow},
	"left":       {chars: "\uf702", plain: "\uf702", usage: 0x50, pressType: webIOSPressTypeLeftArrow},
	"arrowleft":  {chars: "\uf702", plain: "\uf702", usage: 0x50, pressType: webIOSPressTypeLeftArrow},
	"down":       {chars: "\uf701", plain: "\uf701", usage: 0x51, pressType: webIOSPressTypeDownArrow},
	"arrowdown":  {chars: "\uf701", plain: "\uf701", usage: 0x51, pressType: webIOSPressTypeDownArrow},
	"up":         {chars: "\uf700", plain: "\uf700", usage: 0x52, pressType: webIOSPressTypeUpArrow},
	"arrowup":    {chars: "\uf700", plain: "\uf700", usage: 0x52, pressType: webIOSPressTypeUpArrow},
	"home":       {chars: "\uf729", plain: "\uf729", usage: 0x4a, pressType: webIOSPressTypeNone},
	"end":        {chars: "\uf72b", plain: "\uf72b", usage: 0x4d, pressType: webIOSPressTypeNone},
	"pageup":     {chars: "\uf72c", plain: "\uf72c", usage: 0x4b, pressType: webIOSPressTypeNone},
	"pagedown":   {chars: "\uf72d", plain: "\uf72d", usage: 0x4e, pressType: webIOSPressTypeNone},
}

var webIOSUSPunctuation = map[rune]webIOSKeySpec{
	'-':  {chars: "-", plain: "-", usage: 0x2d, pressType: webIOSPressTypeNone},
	'_':  {chars: "_", plain: "-", usage: 0x2d, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'=':  {chars: "=", plain: "=", usage: 0x2e, pressType: webIOSPressTypeNone},
	'+':  {chars: "+", plain: "=", usage: 0x2e, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'[':  {chars: "[", plain: "[", usage: 0x2f, pressType: webIOSPressTypeNone},
	'{':  {chars: "{", plain: "[", usage: 0x2f, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	']':  {chars: "]", plain: "]", usage: 0x30, pressType: webIOSPressTypeNone},
	'}':  {chars: "}", plain: "]", usage: 0x30, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'\\': {chars: "\\", plain: "\\", usage: 0x31, pressType: webIOSPressTypeNone},
	'|':  {chars: "|", plain: "\\", usage: 0x31, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	';':  {chars: ";", plain: ";", usage: 0x33, pressType: webIOSPressTypeNone},
	':':  {chars: ":", plain: ";", usage: 0x33, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'\'': {chars: "'", plain: "'", usage: 0x34, pressType: webIOSPressTypeNone},
	'"':  {chars: "\"", plain: "'", usage: 0x34, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'`':  {chars: "`", plain: "`", usage: 0x35, pressType: webIOSPressTypeNone},
	'~':  {chars: "~", plain: "`", usage: 0x35, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	',':  {chars: ",", plain: ",", usage: 0x36, pressType: webIOSPressTypeNone},
	'<':  {chars: "<", plain: ",", usage: 0x36, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'.':  {chars: ".", plain: ".", usage: 0x37, pressType: webIOSPressTypeNone},
	'>':  {chars: ">", plain: ".", usage: 0x37, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'/':  {chars: "/", plain: "/", usage: 0x38, pressType: webIOSPressTypeNone},
	'?':  {chars: "?", plain: "/", usage: 0x38, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'!':  {chars: "!", plain: "1", usage: 0x1e, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'@':  {chars: "@", plain: "2", usage: 0x1f, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'#':  {chars: "#", plain: "3", usage: 0x20, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'$':  {chars: "$", plain: "4", usage: 0x21, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'%':  {chars: "%", plain: "5", usage: 0x22, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'^':  {chars: "^", plain: "6", usage: 0x23, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'&':  {chars: "&", plain: "7", usage: 0x24, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'*':  {chars: "*", plain: "8", usage: 0x25, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	'(':  {chars: "(", plain: "9", usage: 0x26, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
	')':  {chars: ")", plain: "0", usage: 0x27, modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone},
}

// webClickViewPoint converts a CSS-pixel viewport point into WKWebView bounds
// points at pageZoom.
func webClickViewPoint(pt driver.Point, pageZoom float64) (float64, float64) {
	if pageZoom <= 0 || math.IsNaN(pageZoom) || math.IsInf(pageZoom, 0) {
		pageZoom = 1
	}
	return float64(pt.X) * pageZoom, float64(pt.Y) * pageZoom
}

// shouldDeliverWebTap reports whether a gesture recognizer on WKContentView
// should receive a synthetic single-tap touch sequence.
func shouldDeliverWebTap(info webTapRecognizerInfo) bool {
	if !info.enabled || info.excluded {
		return false
	}
	if info.isTap {
		return info.tapsRequired == 1 && info.touchesRequired == 1
	}
	return true
}

// webIOSKeyLookup resolves a KeyboardEvent.key name for WebInputKey on iOS.
func webIOSKeyLookup(name string) (webIOSKeySpec, bool) {
	if name == "" {
		return webIOSKeySpec{}, false
	}
	lower := strings.ToLower(name)
	if spec, ok := webIOSNamedKeys[lower]; ok {
		return spec, true
	}
	if strings.HasPrefix(lower, "f") {
		if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 12 {
			ch := string(rune(0xf704 + n - 1))
			return webIOSKeySpec{chars: ch, plain: ch, usage: 0x3a + n - 1, pressType: webIOSPressTypeNone}, true
		}
	}
	if utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		return webIOSRuneSpec(r), true
	}
	return webIOSKeySpec{}, false
}

// webIOSRuneSpec maps one character to its US USB HID usage and UIKey metadata,
// marking characters outside the US layout for UIKeyInput insertText:.
func webIOSRuneSpec(r rune) webIOSKeySpec {
	switch {
	case r == '\n' || r == '\r':
		return webIOSNamedKeys["enter"]
	case r == '\t':
		return webIOSNamedKeys["tab"]
	case r == ' ':
		return webIOSNamedKeys["space"]
	case r >= 'a' && r <= 'z':
		s := string(r)
		return webIOSKeySpec{chars: s, plain: s, usage: 0x04 + int(r-'a'), pressType: webIOSPressTypeNone}
	case r >= 'A' && r <= 'Z':
		s := string(r)
		return webIOSKeySpec{chars: s, plain: strings.ToLower(s), usage: 0x04 + int(r-'A'), modifierFlags: webIOSModifierShift, pressType: webIOSPressTypeNone}
	case r >= '1' && r <= '9':
		s := string(r)
		return webIOSKeySpec{chars: s, plain: s, usage: 0x1e + int(r-'1'), pressType: webIOSPressTypeNone}
	case r == '0':
		return webIOSKeySpec{chars: "0", plain: "0", usage: 0x27, pressType: webIOSPressTypeNone}
	}
	if spec, ok := webIOSUSPunctuation[r]; ok {
		return spec
	}
	s := string(r)
	return webIOSKeySpec{chars: s, plain: s, pressType: webIOSPressTypeNone, insertText: true}
}

// webIOSTextSteps splits typed text into ordered key or insertText steps,
// collapsing CRLF line breaks into a single Return key press.
func webIOSTextSteps(text string) []webIOSKeySpec {
	if text == "" {
		return nil
	}
	var out []webIOSKeySpec
	prevCR := false
	for _, r := range text {
		if r == '\n' && prevCR {
			prevCR = false
			continue
		}
		prevCR = r == '\r'
		out = append(out, webIOSRuneSpec(r))
	}
	return out
}
