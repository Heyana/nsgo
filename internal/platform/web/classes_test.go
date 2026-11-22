//go:build js && wasm

package web

import (
	"strconv"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Run in a browser, which has style sheets: nsgo test -target web.
func TestClassSheetDropsClassesNoElementHolds(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	s := &surface{doc: doc}
	defer func() {
		for _, id := range []string{"nsgo-classes", "nsgo-own"} {
			doc.Call("getElementById", id).Call("remove")
		}
	}()
	rules := func() int { return s.classes.sheet.Get("cssRules").Length() }
	inSheet := func(name string) bool {
		list := s.classes.sheet.Get("cssRules")
		for i := range list.Length() {
			if list.Index(i).Get("selectorText").String() == "."+name+"."+name {
				return true
			}
		}
		return false
	}
	var keep classSlot
	s.setClass(doc.Call("createElement", "div"), &keep, cssText("color:red;"))
	for i := range 4 * keepUnused {
		var slot classSlot
		s.setClass(doc.Call("createElement", "div"), &slot, cssText("width:"+strconv.Itoa(i)+"px;"))
		s.freeClass(&slot)
	}
	if n := rules(); n > keepUnused+1 {
		t.Errorf("views' classes: %d rules, want at most %d", n, keepUnused+1)
	}
	style := driver.TextStyle{Font: driver.Font{Size: 16}}
	el := doc.Call("createElement", "div")
	var held spanClasses
	for i := range 4 * keepUnused {
		span := style
		span.Font.Size = 16 + driver.Points(i)/100
		s.Prepare(&driver.AttributedText{Text: "text", Style: style, Spans: []driver.TextSpan{
			{Start: 0, End: 4, Style: span},
		}}).(*preparedText).fill(s, el, false, &held)
	}
	if n := rules(); n > keepUnused+2 {
		t.Errorf("spans' classes: %d rules, want at most %d", n, keepUnused+2)
	}
	if len(held) != 1 || !inSheet(held[0].name) {
		t.Errorf("the shown span's class was deleted")
	}
	if !inSheet(keep.name) {
		t.Errorf("a held class was deleted")
	}
}
