package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// Remount makes the next driver.MountBatch recreate this tree's native views without rerunning its components or disposing their scopes. The platform must discard the old native tree before applying that batch, retaining its process resources and animation graph.
func (t *Tree) Remount() {
	t.rt.check()
	if t.closed || t.remounting {
		return
	}
	for _, n := range t.nodes {
		if n == nil {
			continue
		}
		n.created = false
		n.sent = sent{viewStyle: &defaultViewStyle, props: &zeroViewProps}
		n.flags &^= flagSentVSOwned
		if n.deleted {
			continue
		}
		n.childrenDirty, n.dirty, n.dirtyKids = true, true, true
		if n.text != nil {
			n.text.dropTextLayout()
			n.text.resent = true
		}
		if sc := n.scroll; sc != nil {
			sc.sticky = nil
			sc.issued, sc.applied, sc.pending = 0, 0, nil
			t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemZoomTo, ID: n.id, Zoom: &driver.ZoomRequest{Scale: sc.zoom}})
			// The current offset may exceed the unzoomed content's bounds.
			t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemScrollTo, ID: n.id, Point: sc.offset})
			sc.issue(true, false, sc.offset)
		}
		if n.canvas != nil {
			n.canvas.fresh = true
		}
		if n.shader != nil {
			n.shader.fresh = true
		}
		if n.gpu != nil {
			n.gpu.fresh = true
		}
		if n.fx != nil {
			n.fx.fresh = true
		}
		if n.native != nil {
			n.native.sent, n.native.bound, n.native.named = nil, nil, false
		}
		if n.screen != nil {
			n.screen.sentOK = false
		}
		if n.anim != nil {
			n.anim.sent = [driver.PropCount]uint32{}
		}
		if n.bound != nil {
			b := *n.bound
			n.nodeExtra.bound = nil
			t.rebind(n, b)
		}
	}
	t.directionSent = false
	if t.focus != nil && !t.focus.deleted {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemFocus, ID: t.focus.id})
	}
	t.remounting = true
	t.full, t.changed = true, true
}
