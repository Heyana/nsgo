//go:build js && wasm

package web

import (
	"net/url"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Document semantics share the app's measured layout and styles. Native controls keep their elements; ordinary views and texts may change tags.
func documentTag(kind driver.ViewKind, p *driver.ViewProps) string {
	if kind != driver.KindView && kind != driver.KindText {
		return ""
	}
	switch p.Role {
	case driver.RoleHeader:
		level := p.Level
		if level == 0 {
			level = 2 // ARIA's default heading level
		}
		return "h" + strconv.Itoa(min(max(level, 1), 6))
	case driver.RoleLink:
		if p.Href != "" {
			return "a"
		}
	}
	return "div"
}

func (s *surface) setDocumentProps(v *view, p *driver.ViewProps) {
	if tag := documentTag(v.kind, p); tag != "" {
		s.retag(v, tag)
	}
	href := ""
	if v.el.Get("tagName").String() == "A" && !p.Disabled {
		if base, err := url.Parse(s.doc.Get("baseURI").String()); err == nil {
			if address, err := base.Parse(p.Href); err == nil && driver.CheckURL(address.String()) == nil {
				href = p.Href
			}
		}
	}
	setAttr(v.el, "href", href)
	if href != "" {
		v.el.Set("draggable", false)
	}
}

// retag preserves the mounted view's children, styles, listeners and live animations when its reactive heading level or link address changes.
func (s *surface) retag(v *view, tag string) {
	old := v.el
	if strings.EqualFold(old.Get("tagName").String(), tag) {
		return
	}
	el := s.doc.Call("createElement", tag)
	attrs := old.Get("attributes")
	for i := 0; i < attrs.Length(); i++ {
		a := attrs.Index(i)
		el.Call("setAttribute", a.Get("name"), a.Get("value"))
	}
	el.Set("nsgoID", int(v.id))
	for old.Get("firstChild").Truthy() {
		el.Call("appendChild", old.Get("firstChild"))
	}
	for i := range v.listeners {
		l := &v.listeners[i]
		if l.target.Equal(old) {
			old.Call("removeEventListener", l.name, l.fn)
			el.Call("addEventListener", l.name, l.fn)
			l.target = el
		}
	}
	if old.Get("getAnimations").Type() == js.TypeFunction {
		animations := old.Call("getAnimations")
		for i := 0; i < animations.Length(); i++ {
			if effect := animations.Index(i).Get("effect"); effect.Truthy() {
				effect.Set("target", el)
			}
		}
	}
	focused := s.doc.Get("activeElement").Equal(old)
	if v.content.Equal(old) {
		v.content = el
	}
	if v.elCSS != nil {
		v.elCSS.style = el.Get("style")
	}
	v.elStyle = el.Get("style")
	v.el = el
	if parent := old.Get("parentNode"); parent.Truthy() {
		parent.Call("replaceChild", el, old)
	}
	s.dom.register(v)
	if focused {
		el.Call("focus", map[string]any{"preventScroll": true})
	}
}

func (s *surface) documentLink(target js.Value) *view {
	for el := target; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		if v := s.viewOf(el); v != nil && v.props.Role == driver.RoleLink && v.props.Href != "" {
			return v
		}
	}
	return nil
}

func browserLinkClick(e js.Value) bool {
	return e.Get("button").Int() != 0 || e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool() ||
		e.Get("shiftKey").Bool() || e.Get("altKey").Bool()
}
