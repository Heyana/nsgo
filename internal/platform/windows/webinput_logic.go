package windows

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webCDPKeyEvent is the DevTools Input.dispatchKeyEvent payload sent to WebView2
// so keyboard and text input arrives in the page as trusted events with user
// activation.
type webCDPKeyEvent struct {
	Type                  string `json:"type"`
	Key                   string `json:"key,omitempty"`
	Code                  string `json:"code,omitempty"`
	Text                  string `json:"text,omitempty"`
	UnmodifiedText        string `json:"unmodifiedText,omitempty"`
	WindowsVirtualKeyCode int    `json:"windowsVirtualKeyCode,omitempty"`
	NativeVirtualKeyCode  int    `json:"nativeVirtualKeyCode,omitempty"`
	Modifiers             int    `json:"modifiers,omitempty"`
}

const webCDPShift = 8 // CDP Input.dispatchKeyEvent Shift modifier bit

type webKeySpec struct {
	key        string
	code       string
	text       string
	unmodified string
	vk         int
	modifiers  int
}

var webNamedKeys = map[string]webKeySpec{
	"enter":      {key: "Enter", code: "Enter", text: "\r", unmodified: "\r", vk: 0x0D},
	"return":     {key: "Enter", code: "Enter", text: "\r", unmodified: "\r", vk: 0x0D},
	"tab":        {key: "Tab", code: "Tab", vk: 0x09},
	"escape":     {key: "Escape", code: "Escape", vk: 0x1B},
	"esc":        {key: "Escape", code: "Escape", vk: 0x1B},
	"backspace":  {key: "Backspace", code: "Backspace", vk: 0x08},
	"delete":     {key: "Delete", code: "Delete", vk: 0x2E},
	"space":      {key: " ", code: "Space", text: " ", unmodified: " ", vk: 0x20},
	" ":          {key: " ", code: "Space", text: " ", unmodified: " ", vk: 0x20},
	"left":       {key: "ArrowLeft", code: "ArrowLeft", vk: 0x25},
	"arrowleft":  {key: "ArrowLeft", code: "ArrowLeft", vk: 0x25},
	"up":         {key: "ArrowUp", code: "ArrowUp", vk: 0x26},
	"arrowup":    {key: "ArrowUp", code: "ArrowUp", vk: 0x26},
	"right":      {key: "ArrowRight", code: "ArrowRight", vk: 0x27},
	"arrowright": {key: "ArrowRight", code: "ArrowRight", vk: 0x27},
	"down":       {key: "ArrowDown", code: "ArrowDown", vk: 0x28},
	"arrowdown":  {key: "ArrowDown", code: "ArrowDown", vk: 0x28},
	"home":       {key: "Home", code: "Home", vk: 0x24},
	"end":        {key: "End", code: "End", vk: 0x23},
	"pageup":     {key: "PageUp", code: "PageUp", vk: 0x21},
	"pagedown":   {key: "PageDown", code: "PageDown", vk: 0x22},
}

var webUSPunctuation = map[rune]webKeySpec{
	'`':  {key: "`", code: "Backquote", text: "`", unmodified: "`", vk: 0xC0},
	'~':  {key: "~", code: "Backquote", text: "~", unmodified: "`", vk: 0xC0, modifiers: webCDPShift},
	'-':  {key: "-", code: "Minus", text: "-", unmodified: "-", vk: 0xBD},
	'_':  {key: "_", code: "Minus", text: "_", unmodified: "-", vk: 0xBD, modifiers: webCDPShift},
	'=':  {key: "=", code: "Equal", text: "=", unmodified: "=", vk: 0xBB},
	'+':  {key: "+", code: "Equal", text: "+", unmodified: "=", vk: 0xBB, modifiers: webCDPShift},
	'[':  {key: "[", code: "BracketLeft", text: "[", unmodified: "[", vk: 0xDB},
	'{':  {key: "{", code: "BracketLeft", text: "{", unmodified: "[", vk: 0xDB, modifiers: webCDPShift},
	']':  {key: "]", code: "BracketRight", text: "]", unmodified: "]", vk: 0xDD},
	'}':  {key: "}", code: "BracketRight", text: "}", unmodified: "]", vk: 0xDD, modifiers: webCDPShift},
	'\\': {key: "\\", code: "Backslash", text: "\\", unmodified: "\\", vk: 0xDC},
	'|':  {key: "|", code: "Backslash", text: "|", unmodified: "\\", vk: 0xDC, modifiers: webCDPShift},
	';':  {key: ";", code: "Semicolon", text: ";", unmodified: ";", vk: 0xBA},
	':':  {key: ":", code: "Semicolon", text: ":", unmodified: ";", vk: 0xBA, modifiers: webCDPShift},
	'\'': {key: "'", code: "Quote", text: "'", unmodified: "'", vk: 0xDE},
	'"':  {key: "\"", code: "Quote", text: "\"", unmodified: "'", vk: 0xDE, modifiers: webCDPShift},
	',':  {key: ",", code: "Comma", text: ",", unmodified: ",", vk: 0xBC},
	'<':  {key: "<", code: "Comma", text: "<", unmodified: ",", vk: 0xBC, modifiers: webCDPShift},
	'.':  {key: ".", code: "Period", text: ".", unmodified: ".", vk: 0xBE},
	'>':  {key: ">", code: "Period", text: ">", unmodified: ".", vk: 0xBE, modifiers: webCDPShift},
	'/':  {key: "/", code: "Slash", text: "/", unmodified: "/", vk: 0xBF},
	'?':  {key: "?", code: "Slash", text: "?", unmodified: "/", vk: 0xBF, modifiers: webCDPShift},
	'!':  {key: "!", code: "Digit1", text: "!", unmodified: "1", vk: '1', modifiers: webCDPShift},
	'@':  {key: "@", code: "Digit2", text: "@", unmodified: "2", vk: '2', modifiers: webCDPShift},
	'#':  {key: "#", code: "Digit3", text: "#", unmodified: "3", vk: '3', modifiers: webCDPShift},
	'$':  {key: "$", code: "Digit4", text: "$", unmodified: "4", vk: '4', modifiers: webCDPShift},
	'%':  {key: "%", code: "Digit5", text: "%", unmodified: "5", vk: '5', modifiers: webCDPShift},
	'^':  {key: "^", code: "Digit6", text: "^", unmodified: "6", vk: '6', modifiers: webCDPShift},
	'&':  {key: "&", code: "Digit7", text: "&", unmodified: "7", vk: '7', modifiers: webCDPShift},
	'*':  {key: "*", code: "Digit8", text: "*", unmodified: "8", vk: '8', modifiers: webCDPShift},
	'(':  {key: "(", code: "Digit9", text: "(", unmodified: "9", vk: '9', modifiers: webCDPShift},
	')':  {key: ")", code: "Digit0", text: ")", unmodified: "0", vk: '0', modifiers: webCDPShift},
}

// webClickPoint converts a CSS-pixel viewport point into WebView2 physical
// pixels relative to the composition controller's top left.
func webClickPoint(pt driver.Point, rasterScale, zoomFactor float64) (int32, int32) {
	if rasterScale <= 0 || math.IsNaN(rasterScale) || math.IsInf(rasterScale, 0) {
		rasterScale = 1
	}
	if zoomFactor <= 0 || math.IsNaN(zoomFactor) || math.IsInf(zoomFactor, 0) {
		zoomFactor = 1
	}
	scale := rasterScale * zoomFactor
	return int32(math.Round(float64(pt.X) * scale)), int32(math.Round(float64(pt.Y) * scale))
}

// webKeyLookup resolves a KeyboardEvent.key name for WebInputKey.
func webKeyLookup(name string) (webKeySpec, bool) {
	if name == "" {
		return webKeySpec{}, false
	}
	lower := strings.ToLower(name)
	if spec, ok := webNamedKeys[lower]; ok {
		return spec, true
	}
	if strings.HasPrefix(lower, "f") {
		if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 12 {
			f := "F" + strconv.Itoa(n)
			return webKeySpec{key: f, code: f, vk: 0x6F + n}, true
		}
	}
	if utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		return webRuneSpec(r), true
	}
	return webKeySpec{}, false
}

// webRuneSpec resolves one typed character to its US-layout key event metadata,
// falling back to a text-only event for characters not on the US layout.
func webRuneSpec(r rune) webKeySpec {
	switch {
	case r == '\n' || r == '\r':
		return webNamedKeys["enter"]
	case r == '\t':
		return webKeySpec{key: "Tab", code: "Tab", text: "\t", unmodified: "\t", vk: 0x09}
	case r == ' ':
		return webNamedKeys["space"]
	case r >= 'a' && r <= 'z':
		s := string(r)
		return webKeySpec{key: s, code: "Key" + strings.ToUpper(s), text: s, unmodified: s, vk: int('A' + (r - 'a'))}
	case r >= 'A' && r <= 'Z':
		s := string(r)
		return webKeySpec{key: s, code: "Key" + s, text: s, unmodified: strings.ToLower(s), vk: int(r), modifiers: webCDPShift}
	case r >= '0' && r <= '9':
		s := string(r)
		return webKeySpec{key: s, code: "Digit" + s, text: s, unmodified: s, vk: int(r)}
	}
	if spec, ok := webUSPunctuation[r]; ok {
		return spec
	}
	s := string(r)
	return webKeySpec{key: s, text: s, unmodified: s}
}

func webKeyEventPair(spec webKeySpec) (down, up webCDPKeyEvent) {
	downType := "rawKeyDown"
	if spec.text != "" {
		downType = "keyDown"
	}
	down = webCDPKeyEvent{
		Type:                  downType,
		Key:                   spec.key,
		Code:                  spec.code,
		Text:                  spec.text,
		UnmodifiedText:        spec.unmodified,
		WindowsVirtualKeyCode: spec.vk,
		NativeVirtualKeyCode:  spec.vk,
		Modifiers:             spec.modifiers,
	}
	up = webCDPKeyEvent{
		Type:                  "keyUp",
		Key:                   spec.key,
		Code:                  spec.code,
		WindowsVirtualKeyCode: spec.vk,
		NativeVirtualKeyCode:  spec.vk,
		Modifiers:             spec.modifiers,
	}
	return down, up
}

// webKeyEventsForInput translates a text or key WebInput into the ordered
// CDP Input.dispatchKeyEvent sequence WebView2 dispatches as trusted input.
func webKeyEventsForInput(in driver.WebInput) []webCDPKeyEvent {
	switch in.Kind {
	case driver.WebInputText:
		if in.Text == "" {
			return nil
		}
		var out []webCDPKeyEvent
		prevCR := false
		for _, r := range in.Text {
			if r == '\n' && prevCR {
				prevCR = false
				continue
			}
			prevCR = r == '\r'
			down, up := webKeyEventPair(webRuneSpec(r))
			out = append(out, down, up)
		}
		return out
	case driver.WebInputKey:
		spec, ok := webKeyLookup(in.Key)
		if !ok {
			return nil
		}
		down, up := webKeyEventPair(spec)
		return []webCDPKeyEvent{down, up}
	}
	return nil
}
