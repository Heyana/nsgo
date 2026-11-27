//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Run in a browser, which lays out and selects the texts: nsgo test -target web.
func TestCopyTakesAttachmentsCopyTexts(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	s := &surface{doc: doc}
	s.host = doc.Call("createElement", "div")
	doc.Get("body").Call("appendChild", s.host)
	defer s.host.Call("remove")
	text := func(at *driver.AttributedText) js.Value {
		el := doc.Call("createElement", "div")
		el.Set("className", "nsgo-text")
		el.Get("style").Call("setProperty", "user-select", "text")
		s.Prepare(at).(*preparedText).fill(s, el, false, new(spanClasses))
		s.host.Call("appendChild", el)
		return el
	}
	formula := "E ￼ m"
	text(&driver.AttributedText{Text: formula + "hidden",
		Spans:       []driver.TextSpan{{Start: 2, End: 5, Copy: "$E=mc^2$"}, {Start: len(formula), End: len(formula) + 6, Hidden: true}},
		Attachments: []driver.TextAttachment{{Offset: 2, Size: driver.Size{Width: 10, Height: 10}}},
	})
	last := text(&driver.AttributedText{Text: "Last"})

	copied := func() string {
		var got string
		set := js.FuncOf(func(this js.Value, args []js.Value) any { got = args[1].String(); return nil })
		defer set.Release()
		none := js.FuncOf(func(js.Value, []js.Value) any { return nil })
		defer none.Release()
		e := js.Global().Get("Object").New()
		clipboard := js.Global().Get("Object").New()
		clipboard.Set("setData", set)
		e.Set("clipboardData", clipboard)
		e.Set("preventDefault", none)
		s.copySelection(e)
		return got
	}
	sel := js.Global().Call("getSelection")
	r := doc.Call("createRange")
	r.Call("setStart", s.host.Get("firstChild"), 0)
	r.Call("setEnd", last, last.Get("childNodes").Length())
	sel.Call("removeAllRanges")
	sel.Call("addRange", r)
	if got := copied(); got != "E $E=mc^2$ m\nLast" {
		t.Errorf("two texts copy as %q", got)
	}
	r.Call("setEnd", s.host.Get("firstChild"), s.host.Get("firstChild").Get("childNodes").Length())
	sel.Call("removeAllRanges")
	sel.Call("addRange", r)
	if got := copied(); got != "E $E=mc^2$ m" {
		t.Errorf("a text with a formula and a hidden span copies as %q", got)
	}
	sel.Call("removeAllRanges")

	// Selecting a single formula attachment also writes MathML and text/x-tex.
	const mathML = `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block"><msup><mi>x</mi><mn>2</mn></msup></math>`
	only := text(&driver.AttributedText{
		Text: "￼",
		Spans: []driver.TextSpan{{
			Start: 0, End: len("￼"), Copy: "$$\nx^2\n$$",
			CopyItem: &driver.ClipboardItem{Representations: []driver.ClipboardRepresentation{
				{Type: "text/plain", Data: []byte("$$\nx^2\n$$")},
				{Type: "application/mathml+xml", Data: []byte(mathML)},
				{Type: "text/x-tex", Data: []byte("x^2")},
			}},
		}},
		Attachments: []driver.TextAttachment{{Offset: 0, Size: driver.Size{Width: 10, Height: 10}}},
	})
	r.Call("selectNodeContents", only)
	sel.Call("removeAllRanges")
	sel.Call("addRange", r)
	gotMap := map[string]string{}
	setAll := js.FuncOf(func(this js.Value, args []js.Value) any { gotMap[args[0].String()] = args[1].String(); return nil })
	defer setAll.Release()
	none := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	defer none.Release()
	e := js.Global().Get("Object").New()
	clipboard := js.Global().Get("Object").New()
	clipboard.Set("setData", setAll)
	e.Set("clipboardData", clipboard)
	e.Set("preventDefault", none)
	s.copySelection(e)
	sel.Call("removeAllRanges")
	if gotMap["text/plain"] != "$$\nx^2\n$$" || gotMap["application/mathml+xml"] != mathML || gotMap["text/x-tex"] != "x^2" {
		t.Errorf("single formula copy wrote %#v", gotMap)
	}
}
