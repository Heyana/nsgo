//go:build js && wasm

package web

import (
	"slices"
	"strconv"
	"syscall/js"
)

// Styles as classes, as NSH's web runtime does (its web.md, "Styles as
// classes"): no element has an inline style but its geometry and what
// animates, so the page does not parse the same text again for every
// element of every batch. A view's look, what its events make of the
// pointer and selection, and a text's paragraph each go to a class named
// nsgo- and 8 hex digits of the FNV-1a hash of their CSS, inserted as
// .name.name{css} into the surface's sheet the first time it is used: the
// doubled class wins over the runtime's rules of a class and a
// pseudo-class, as inline styles did.
//
// A view whose style keeps changing would add a rule each time: after
// defining ownRulesAfter classes, a slot takes a rule of its own in a
// second sheet, changed in place, which the view's deletion frees for
// another.
//
// The slots and spans that show a class hold it. One no element holds
// stays in the sheet, for a style that comes back, until keepUnused such
// classes have gathered and another is defined, which deletes them all:
// the sheet holds what is shown and a bounded rest.

const (
	ownRulesAfter = 16
	keepUnused    = 256
)

// classSheet is a surface's classes.
type classSheet struct {
	sheet, own js.Value              // the CSSStyleSheets of classes and own rules
	rules      []*classRule          // sheet's rules, in its order
	owns       int                   // the rules own holds
	names      map[string]*classRule // classes by CSS
	taken      map[string]bool
	unused     int // the classes no element holds
	free       []*ownRule
}

// classRule is a class of the sheet.
type classRule struct {
	css, name string
	holders   int // the slots and spans holding it
}

// ownRule is a rule of one slot's own, under a class of its own.
type ownRule struct {
	style js.Value // the rule's CSSStyleDeclaration
	name  string
}

// classSlot is a part of an element's style a class carries.
type classSlot struct {
	css   string     // the CSS last applied
	name  string     // its class on the element, "" for none
	class *classRule // the class it holds, nil for none or an own rule
	count int        // the classes it has defined
	own   *ownRule
}

// spanClasses are the classes the spans an element shows hold.
type spanClasses []*classRule

// cssText builds a declaration block.
type cssText []byte

// add appends name: value; for a value that is not empty.
func (c *cssText) add(name, value string) {
	if value != "" {
		*c = append(append(append(append(*c, name...), ':'), value...), ';')
	}
}

func (s *surface) classSheet() *classSheet {
	c := &s.classes
	if c.names == nil {
		head := s.doc.Get("head")
		for _, id := range []string{"nsgo-classes", "nsgo-own"} {
			el := s.doc.Call("createElement", "style")
			el.Set("id", id)
			head.Call("appendChild", el)
			if id == "nsgo-classes" {
				c.sheet = el.Get("sheet")
			} else {
				c.own = el.Get("sheet")
			}
		}
		c.names, c.taken = map[string]*classRule{}, map[string]bool{}
	}
	return c
}

// class returns the class of css, held by one more holder, and whether
// it inserted its rule: the first time, or after the rule was deleted.
func (c *classSheet) class(css string) (*classRule, bool) {
	if r, ok := c.names[css]; ok {
		if r.holders == 0 {
			c.unused--
		}
		r.holders++
		return r, false
	}
	if c.unused >= keepUnused {
		c.sweep()
	}
	h := uint32(2166136261)
	for i := 0; i < len(css); i++ {
		h = (h ^ uint32(css[i])) * 16777619
	}
	hex := strconv.FormatUint(uint64(h)|1<<32, 16)[1:]
	name := "nsgo-" + hex
	// Two texts of one hash: the later gets a suffix.
	for i := 1; c.taken[name]; i++ {
		name = "nsgo-" + hex + "-" + strconv.Itoa(i)
	}
	r := &classRule{css: css, name: name, holders: 1}
	c.taken[name] = true
	c.names[css] = r
	c.sheet.Call("insertRule", "."+name+"."+name+"{"+css+"}", len(c.rules))
	c.rules = append(c.rules, r)
	return r, true
}

// release lets go of r, which may be nil.
func (c *classSheet) release(r *classRule) {
	if r == nil {
		return
	}
	if r.holders--; r.holders == 0 {
		c.unused++
	}
}

// sweep deletes the rules of the classes no element holds, from the last
// so the indices of those before stay.
func (c *classSheet) sweep() {
	for i := len(c.rules) - 1; i >= 0; i-- {
		if r := c.rules[i]; r.holders == 0 {
			c.sheet.Call("deleteRule", i)
			delete(c.names, r.css)
			delete(c.taken, r.name)
		}
	}
	c.rules = slices.DeleteFunc(c.rules, func(r *classRule) bool { return r.holders == 0 })
	c.unused = 0
}

// releaseSpans lets go of the classes of the spans h holds.
func (c *classSheet) releaseSpans(h *spanClasses) {
	for _, r := range *h {
		c.release(r)
	}
	clear(*h)
	*h = (*h)[:0]
}

// ownRule returns a free rule of a slot's own.
func (c *classSheet) ownRule() *ownRule {
	if n := len(c.free); n > 0 {
		r := c.free[n-1]
		c.free = c.free[:n-1]
		return r
	}
	name := "nsgo-own-" + strconv.Itoa(c.owns)
	c.own.Call("insertRule", "."+name+"."+name+"{}", c.owns)
	r := &ownRule{style: c.own.Get("cssRules").Index(c.owns).Get("style"), name: name}
	c.owns++
	return r
}

// setClass gives el the class of css in slot, taking the old one off.
func (s *surface) setClass(el js.Value, slot *classSlot, css cssText) {
	if string(css) == slot.css {
		return
	}
	slot.css = string(css)
	if slot.own != nil {
		slot.own.style.Set("cssText", slot.css)
		return
	}
	c := s.classSheet()
	var class *classRule
	name := ""
	switch {
	case slot.css == "":
	case slot.count >= ownRulesAfter:
		slot.own = c.ownRule()
		slot.own.style.Set("cssText", slot.css)
		name = slot.own.name
	default:
		var defined bool
		if class, defined = c.class(slot.css); defined {
			slot.count++
		}
		name = class.name
	}
	c.release(slot.class)
	slot.class = class
	if name == slot.name {
		return
	}
	list := el.Get("classList")
	switch {
	case slot.name == "":
		list.Call("add", name)
	case name == "":
		list.Call("remove", slot.name)
	default:
		list.Call("replace", slot.name, name)
	}
	slot.name = name
}

// freeClass lets go of slot's class, and gives its own rule back for
// another slot.
func (s *surface) freeClass(slot *classSlot) {
	s.classes.release(slot.class)
	slot.class = nil
	if r := slot.own; r != nil {
		r.style.Set("cssText", "")
		s.classes.free = append(s.classes.free, r)
		slot.own = nil
	}
}

// freeClasses lets go of a deleted view's classes.
func (s *surface) freeClasses(v *view) {
	s.freeClass(&v.look)
	s.freeClass(&v.feel)
	s.freeClass(&v.textClass)
	if v.input != nil {
		s.freeClass(&v.input.look)
	}
	s.classes.releaseSpans(&v.spans)
}
