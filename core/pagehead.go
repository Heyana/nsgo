package core

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type pageHeadLayer struct {
	owner  *Scope
	values driver.PageHead
}

type pageHeads struct {
	layers  []*pageHeadLayer
	shown   driver.PageHead
	started bool
}

// BindWebHead is the core's part of web.Head, which apps use: bind each non-nil field to the current owner's lifetime. Native and confined owners have no document metadata to change.
func (cx *Scope) BindWebHead(title, description func() string) {
	t := cx.tree()
	if t == nil || !t.reported.CanSetPageHead || confinement(cx.current()) != nil || title == nil && description == nil {
		return
	}
	l := &pageHeadLayer{owner: cx.current()}
	t.pageHead.layers = append(t.pageHead.layers, l)
	cx.renderEffect(func() {
		values := driver.PageHead{HasTitle: title != nil, HasDescription: description != nil}
		if title != nil {
			values.Title = title()
		}
		if description != nil {
			values.Description = description()
		}
		if values != l.values {
			l.values = values
			t.changed = true
		}
	})
	l.owner.addCleanup(func() {
		t.pageHead.layers = slices.DeleteFunc(t.pageHead.layers, func(o *pageHeadLayer) bool { return o == l })
		t.changed = true
	})
	t.changed = true
}

func (t *Tree) emitPageHead() {
	h := &t.pageHead
	if len(h.layers) == 0 && !h.started {
		return
	}
	values := driver.PageHead{}
	places := t.keyPlaces()
	for _, l := range h.layers {
		if l.owner.disposed || !places.shows(l.owner, true) {
			continue
		}
		if l.values.HasTitle {
			values.Title, values.HasTitle = l.values.Title, true
		}
		if l.values.HasDescription {
			values.Description, values.HasDescription = l.values.Description, true
		}
	}
	if values == h.shown && (h.started || values == (driver.PageHead{})) {
		return
	}
	h.shown, h.started = values, true
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemPageHead, Head: &values})
}
