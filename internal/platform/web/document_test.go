//go:build js && wasm

package web

import (
	"strconv"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

func documentFixture(t *testing.T, root func(*core.Scope) core.Node) (*surface, js.Value) {
	t.Helper()
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-document")
	setStyle(mount, map[string]string{"width": "320px", "height": "320px"})
	doc.Get("body").Call("appendChild", mount)
	s := open(root, Options{Mount: "#nsgo-document"}, nil)
	t.Cleanup(func() { s.engine.Close(); mount.Call("remove") })
	return s, mount
}

func documentWait(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("document update did not settle")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDocumentHeadingsAndLinksKeepTheAppLayout(t *testing.T) {
	var cx *core.Scope
	var level *core.Signal[int]
	var href *core.Signal[string]
	_, mount := documentFixture(t, func(scope *core.Scope) core.Node {
		cx = scope
		level, href = core.NewSignal(cx, 2), core.NewSignal(cx, "/docs")
		var children []core.Node
		for n := 1; n <= 6; n++ {
			children = append(children, core.Text{TestID: "h" + strconv.Itoa(n), Content: "Heading", Role: core.RoleHeader,
				AccessibilityLevel: func() int { return n }, Style: core.Style{Height: core.Pt(25)}})
		}
		children = append(children, core.Text{TestID: "live", Content: "Live", Role: core.RoleHeader, AccessibilityLevel: level.Get},
			base.Link{TestID: "link", HrefFunc: href.Get, Style: core.Style{Width: core.Pt(180), Height: core.Pt(35)},
				Children: []core.Node{core.Text{Content: "Read docs"}}})
		return core.View{Style: core.Style{FontSize: 17, Color: core.RGB(0x123456)}, Children: children}
	})
	find := func(id string) js.Value { return mount.Call("querySelector", "[data-testid="+id+"]") }
	documentWait(t, func() bool { return find("link").Truthy() && find("link").Get("tagName").String() == "A" })
	for n := 1; n <= 6; n++ {
		h := find("h" + strconv.Itoa(n))
		style := js.Global().Call("getComputedStyle", h)
		textStyle := js.Global().Call("getComputedStyle", h.Get("firstElementChild"))
		if h.Get("tagName").String() != "H"+strconv.Itoa(n) || style.Get("marginTop").String() != "0px" ||
			textStyle.Get("fontSize").String() != "17px" || style.Get("fontWeight").String() != "400" {
			t.Fatalf("heading %d gained browser styling: %s", n, h.Get("outerHTML").String())
		}
	}
	link := find("link")
	box := link.Call("getBoundingClientRect")
	if link.Call("getAttribute", "href").String() != "/docs" || box.Get("width").Float() != 180 || box.Get("height").Float() != 35 {
		t.Fatal("link lost its address or measured layout")
	}
	cx.Post(func() { level.Set(5); href.Set("/updated") })
	documentWait(t, func() bool {
		return find("live").Get("tagName").String() == "H5" && find("link").Call("getAttribute", "href").String() == "/updated"
	})
	if !find("link").Equal(link) || find("live").Get("textContent").String() != "Live" {
		t.Fatal("a metadata update replaced the link or lost text children")
	}
}

func TestDocumentLinksKeepBrowserAlternativeClicks(t *testing.T) {
	var cx *core.Scope
	var disabled *core.Signal[bool]
	var presses int
	_, mount := documentFixture(t, func(scope *core.Scope) core.Node {
		cx = scope
		disabled = core.NewSignal(cx, false)
		return base.Link{TestID: "link", Href: "/notes", Disabled: disabled.Get,
			OnPress: func(core.PressEvent) { presses++ }, Children: []core.Node{core.Text{Content: "Notes"}}}
	})
	documentWait(t, func() bool { return mount.Call("querySelector", "a").Truthy() })
	link := mount.Call("querySelector", "a")
	click := func(name string, button int, meta bool) bool {
		e := js.Global().Get("MouseEvent").New(name, map[string]any{"bubbles": true, "cancelable": true, "button": button, "metaKey": meta})
		link.Call("dispatchEvent", e)
		return e.Get("defaultPrevented").Bool()
	}
	if !click("click", 0, false) {
		t.Fatal("ordinary link click navigates outside the app")
	}
	documentWait(t, func() bool { return presses == 1 })
	if click("mousedown", 0, true) || click("click", 0, true) || click("mousedown", 1, false) || click("auxclick", 1, false) {
		t.Fatal("a modified or middle click lost the browser's link action")
	}
	if presses != 1 {
		t.Fatal("browser alternative click pressed the app's link")
	}
	cx.Post(func() { disabled.Set(true) })
	documentWait(t, func() bool { return !link.Call("hasAttribute", "href").Bool() })
	if !click("click", 0, true) || presses != 1 {
		t.Fatal("disabled link remained actionable")
	}
}
