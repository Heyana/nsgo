//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/oneo-me/nsgo/internal/driver"
)

// then calls done with what promise resolves to, or fail with why it is
// rejected.
func then(promise js.Value, done, fail func(js.Value)) {
	var ok, bad js.Func
	settle := func(fn func(js.Value), args []js.Value) any {
		ok.Release()
		bad.Release()
		arg := js.Undefined()
		if len(args) > 0 {
			arg = args[0]
		}
		fn(arg)
		return nil
	}
	ok = js.FuncOf(func(_ js.Value, args []js.Value) any { return settle(done, args) })
	bad = js.FuncOf(func(_ js.Value, args []js.Value) any { return settle(fail, args) })
	promise.Call("then", ok, bad)
}

func (s *surface) setClipboard(text string) {
	if c := js.Global().Get("navigator").Get("clipboard"); c.Truthy() {
		then(c.Call("writeText", text), func(js.Value) {}, func(js.Value) { s.copyByCommand(text) })
		return
	}
	s.copyByCommand(text)
}

// copyByCommand copies text through a selected text area, for pages
// without the Clipboard API, such as ones served over plain http.
func (s *surface) copyByCommand(text string) {
	focus := s.doc.Get("activeElement")
	area := s.doc.Call("createElement", "textarea")
	area.Set("value", text)
	setStyle(area, map[string]string{"position": "fixed", "left": "-10000px", "top": "0", "opacity": "0"})
	s.doc.Get("body").Call("appendChild", area)
	area.Call("select")
	s.doc.Call("execCommand", "copy")
	area.Call("remove")
	if focus.Truthy() {
		focus.Call("focus", map[string]any{"preventScroll": true})
	}
}

// readClipboard answers read id with the clipboard's text, or "" when
// the page may not read it.
func (s *surface) readClipboard(id uint32) {
	answer := func(text string) {
		s.engine.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: id, Text: text})
	}
	c := js.Global().Get("navigator").Get("clipboard")
	if !c.Truthy() || !c.Get("readText").Truthy() {
		answer("")
		return
	}
	then(c.Call("readText"), func(v js.Value) {
		if v.Type() == js.TypeString {
			answer(v.String())
		} else {
			answer("")
		}
	}, func(js.Value) { answer("") })
}

// announce has screen readers say text: each message is a new node of a
// live region, so repeating one is read again.
func (s *surface) announce(text string) {
	if !s.live.Truthy() {
		s.live = s.doc.Call("createElement", "div")
		s.live.Call("setAttribute", "role", "status")
		s.live.Call("setAttribute", "aria-live", "polite")
		setStyle(s.live, map[string]string{
			"position": "fixed", "width": "1px", "height": "1px", "overflow": "hidden",
			"clip-path": "inset(50%)", "white-space": "nowrap",
		})
		s.doc.Get("body").Call("appendChild", s.live)
	}
	msg := s.doc.Call("createElement", "div")
	msg.Set("textContent", text)
	s.live.Call("replaceChildren", msg)
}
