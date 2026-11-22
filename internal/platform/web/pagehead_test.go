//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPageHeadRestoresTheDocumentAndKeepsTextAsData(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	title := doc.Get("title").String()
	meta := doc.Call("createElement", "meta")
	meta.Call("setAttribute", "name", "description")
	meta.Call("setAttribute", "content", "Host description")
	doc.Get("head").Call("appendChild", meta)
	t.Cleanup(func() { meta.Call("remove"); doc.Set("title", title) })
	s := &surface{doc: doc, whole: true}
	s.setPageHead(driver.PageHead{Title: "Page <script>text</script>", Description: `"<b>not HTML</b>`, HasTitle: true, HasDescription: true})
	if doc.Get("title").String() != "Page <script>text</script>" || meta.Call("getAttribute", "content").String() != `"<b>not HTML</b>` {
		t.Fatal("head metadata was not written as plain text")
	}
	s.setPageTitle("New host title")
	if doc.Get("title").String() != "Page <script>text</script>" {
		t.Fatal("window title bypassed the active page")
	}
	s.setPageHead(driver.PageHead{HasTitle: true, Title: "", HasDescription: true, Description: ""})
	if doc.Get("title").String() != "" || meta.Call("getAttribute", "content").String() != "" {
		t.Fatal("explicit empty metadata was inherited")
	}
	s.setPageHead(driver.PageHead{})
	if doc.Get("title").String() != "New host title" || meta.Call("getAttribute", "content").String() != "Host description" {
		t.Fatal("original head was not restored")
	}
}

func TestPageHeadEmbeddedSurfaceLeavesItsHostAlone(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	title := doc.Get("title").String()
	s := &surface{doc: doc}
	s.setPageHead(driver.PageHead{Title: "Embedded", Description: "Hidden", HasTitle: true, HasDescription: true})
	s.setPageTitle("Other title")
	if doc.Get("title").String() != title || s.pageHead.started {
		t.Fatal("embedded app changed host metadata")
	}
}
