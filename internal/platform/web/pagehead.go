//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type pageHeadState struct {
	started, hadMeta, hadContent bool
	baseTitle, baseDescription   string
	meta                         js.Value
	values                       driver.PageHead
}

func (s *surface) setPageTitle(title string) {
	if !s.whole {
		return
	}
	s.pageHead.baseTitle = title
	if !s.pageHead.values.HasTitle {
		s.doc.Set("title", title)
	}
}

func (s *surface) setPageHead(values driver.PageHead) {
	if !s.whole {
		return
	}
	h := &s.pageHead
	if !h.started {
		h.started = true
		h.baseTitle = s.doc.Get("title").String()
		h.meta = s.doc.Call("querySelector", `meta[name="description" i]`)
		h.hadMeta = h.meta.Truthy()
		if h.hadMeta {
			h.hadContent = h.meta.Call("hasAttribute", "content").Bool()
			if h.hadContent {
				h.baseDescription = h.meta.Call("getAttribute", "content").String()
			}
		}
	}
	h.values = values
	title := h.baseTitle
	if values.HasTitle {
		title = values.Title
	}
	s.doc.Set("title", title)
	if values.HasDescription {
		if !h.meta.Truthy() {
			h.meta = s.doc.Call("createElement", "meta")
			h.meta.Call("setAttribute", "name", "description")
			h.meta.Call("setAttribute", "data-nsgo-head", "")
			s.doc.Get("head").Call("appendChild", h.meta)
		}
		h.meta.Call("setAttribute", "content", values.Description)
	} else if h.hadMeta {
		if h.hadContent {
			h.meta.Call("setAttribute", "content", h.baseDescription)
		} else {
			h.meta.Call("removeAttribute", "content")
		}
	} else if h.meta.Truthy() {
		h.meta.Call("remove")
		h.meta = js.Undefined()
	}
}
