//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Pressable views keep the arrow, as on the desktops: only Style.Cursor
// names a cursor.
func TestFeelCursorOnlyFromStyle(t *testing.T) {
	press := driver.EventMask(1 << driver.EventPress)
	for _, tc := range []struct {
		name   string
		v      view
		cursor string
	}{
		{"plain", view{}, ""},
		{"pressable", view{events: press}, ""},
		{"long press", view{events: driver.EventMask(1 << driver.EventLongPress)}, ""},
		{"text", view{style: driver.ViewStyle{Cursor: driver.CursorText}}, "text"},
		{"pressable with a cursor", view{events: press, style: driver.ViewStyle{Cursor: driver.CursorColResize}}, "col-resize"},
	} {
		css := string(tc.v.feelCSS(nil))
		got := ""
		for _, d := range strings.Split(css, ";") {
			if name, value, ok := strings.Cut(d, ":"); ok && name == "cursor" {
				got = value
			}
		}
		if got != tc.cursor {
			t.Errorf("%s: cursor %q, want %q (%s)", tc.name, got, tc.cursor, css)
		}
	}
}

// Run in a browser, which lays out the alert: nsgo test -target web.
func TestAlertButtonsKeepTheArrow(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	s := &surface{doc: doc}
	s.env.ReduceMotion = true
	s.showAlert(&driver.AlertConfig{ID: 1, Title: "Title", Actions: []driver.AlertAction{
		{Label: "Cancel", Role: driver.AlertCancel}, {Label: "OK", Role: driver.AlertDefault},
	}})
	a := s.alerts[0]
	defer a.layer.Call("remove")
	for _, b := range a.buttons {
		if c := js.Global().Call("getComputedStyle", b).Get("cursor").String(); c != "default" && c != "auto" {
			t.Errorf("%s: cursor %q", b.Get("textContent").String(), c)
		}
	}
}

// Run in a browser, which computes the spans' styles: nsgo test -target web.
func TestSpanCursorShowsOverItsCharacters(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	s := &surface{doc: doc}
	s.host = doc.Call("createElement", "div")
	doc.Get("body").Call("appendChild", s.host)
	defer s.host.Call("remove")
	style := driver.TextStyle{Font: driver.Font{Size: 16}}
	el := doc.Call("createElement", "div")
	el.Get("style").Call("setProperty", "cursor", "help") // the text's own
	s.Prepare(&driver.AttributedText{Text: "plain link", Style: style, Spans: []driver.TextSpan{
		{Start: 0, End: 6, Style: style},
		{Start: 6, End: 10, Style: style, Cursor: driver.CursorPointer},
	}}).(*preparedText).fill(s, el, false, new(spanClasses))
	s.host.Call("appendChild", el)
	spans := el.Call("querySelectorAll", "span")
	if spans.Length() != 2 {
		t.Fatalf("%d spans, want 2", spans.Length())
	}
	for i, want := range []string{"help", "pointer"} {
		if c := js.Global().Call("getComputedStyle", spans.Index(i)).Get("cursor").String(); c != want {
			t.Errorf("span %d: cursor %q, want %q", i, c, want)
		}
	}
}

// A test's pinned ReduceMotion: false wins over the browser's prefers-reduced-motion, as the page reads the app's pinned environment: the alert fades in. Run in a browser: nsgo test -target web.
func TestPinnedMotionWinsOverTheBrowsers(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	for _, pinned := range []bool{false, true} {
		s := &surface{doc: doc, envPin: func(env driver.Environment) driver.Environment { env.ReduceMotion = pinned; return env }}
		s.env.ReduceMotion = true
		s.showAlert(&driver.AlertConfig{ID: 1, Title: "Title", Actions: []driver.AlertAction{{Label: "OK", Role: driver.AlertDefault}}})
		a := s.alerts[0]
		animated := a.layer.Call("getAnimations").Length() > 0
		a.layer.Call("remove")
		if animated == pinned {
			t.Errorf("pinned ReduceMotion %v: the alert animated %v", pinned, animated)
		}
	}
}
