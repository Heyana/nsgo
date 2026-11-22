package core

import "slices"

// Bars of the app's in scroll views (NSH's core/scrollbars.rs): a
// ScrollbarMaker draws a scroll view's bar in place of the platform's
// indicator. The bar is a child of the scroll view, last so it draws over
// the content and what comes later goes before it, as large as the
// viewport and pinned over it by a sticky range across the whole content,
// which the platforms apply on the main thread, so it keeps up with a
// scroll that waits for no commit.

// ScrollbarMaker makes a scroll view's bar: base.ScrollbarTheme's.
type ScrollbarMaker interface {
	// Scrollbar is the bar of target, or nil to keep the platform's
	// indicator.
	Scrollbar(cx *Scope, target *Ref, horizontal bool) Node
}

// SetScrollbars is the core's part of base.SetScrollbarTheme, which apps
// use: m draws the bar of every scroll view, list and multiline input of the surface that shows its indicator and has no bar of its own; nil gives them the platform's back.
func (cx *Scope) SetScrollbars(m ScrollbarMaker) {
	t := cx.tree()
	if t == nil || t.bars == m {
		return
	}
	t.bars = m
	for _, n := range t.nodes {
		if n != nil && (n.scroll != nil || n.input != nil) && !n.deleted {
			t.refreshBar(n)
		}
	}
}

// refreshBar has scroll view or multiline input n's bar be what it has now. The host stays once made, empty while nothing draws a bar.
func (t *Tree) refreshBar(n *node) {
	if n.input != nil {
		t.refreshInputBar(n)
		return
	}
	sc := n.scroll
	m := sc.ownBar
	if m == nil && !sc.hideIndicator {
		m = t.bars
	}
	if sc.barHost == nil {
		if m == nil {
			return
		}
		t.mountBarHost(n)
	}
	sc.bar.Set(m)
}

func (t *Tree) refreshInputBar(n *node) {
	in := n.input
	var m ScrollbarMaker
	if in.config.Multiline {
		m = in.ownBar
		if m == nil && !in.hideIndicator {
			m = t.bars
		}
	}
	if in.barHost == nil {
		if m == nil {
			if hide := in.config.Multiline && in.hideIndicator; hide != in.config.HideIndicator {
				in.config.HideIndicator = hide
				t.touch(n)
			}
			return
		}
		t.mountInputBarHost(n)
	}
	if m == nil && in.bar.GetUntracked() == nil {
		if hide := in.config.Multiline && in.hideIndicator; hide != in.config.HideIndicator {
			in.config.HideIndicator = hide
			t.touch(n)
		}
	}
	in.bar.Set(m)
}

// mountInputBarHost makes the view that holds multiline input n's bar. It is laid out as its own root at n's frame size (layoutInputBars) so n stays a leaf for text measurement, and stays fixed over n without sticky scrolling.
func (t *Tree) mountInputBarHost(n *node) {
	in := n.input
	cx := n.owner
	in.bar = NewSignal[ScrollbarMaker](cx, nil)
	if n.ref == nil {
		t.setRef(n, new(Ref))
	}
	host := View{PointerEvents: PointerEventsBoxNone,
		Style: Style{Position: Absolute, Left: Pt(0), Top: Pt(0), Width: Pct(100), Height: Pct(100)},
		Children: []Node{Dynamic(func(cx *Scope) Node {
			m := in.bar.Get()
			var bar Node
			if m != nil {
				bar = m.Scrollbar(cx, n.ref, false)
			}
			if hide := in.config.Multiline && (in.hideIndicator || bar != nil); hide != in.config.HideIndicator {
				in.config.HideIndicator = hide
				t.touch(n)
			}
			return bar
		})}}
	t.within(cx, func() { t.mount(cx, n, host) })
	bar := n.children[len(n.children)-1]
	bar.inputBar = true
	in.barHost = bar
	t.inputBars = append(t.inputBars, bar)
	n.flat = nil
	n.childrenDirty = true
	t.mark(n)
}

// layoutInputBars lays each multiline input's bar host out at its input's frame.
func (t *Tree) layoutInputBars() {
	live := t.inputBars[:0]
	for _, bar := range t.inputBars {
		if bar.deleted || bar.parent == nil || bar.parent.deleted {
			continue
		}
		live = append(live, bar)
		p := bar.parent
		size := p.frame.Size
		if p.hidden {
			size = Size{}
		}
		if bar.hidden != p.hidden {
			bar.hidden = p.hidden
			bar.layoutDirty = true
			t.mark(bar)
		}
		if !bar.layoutDirty && bar.frame.Size == size && bar.fill != nil && *bar.fill == size {
			continue
		}
		t.layoutRoot(bar, size, true)
		t.setFrame(bar, Bounds{Size: size})
		t.syncFrames(bar)
	}
	clear(t.inputBars[len(live):])
	t.inputBars = live
}

// mountBarHost makes the view that holds scroll view n's bar.
func (t *Tree) mountBarHost(n *node) {
	sc := n.scroll
	cx := n.owner
	sc.bar = NewSignal[ScrollbarMaker](cx, nil)
	sc.barSize = NewSignal(cx, n.frame.Size)
	if n.ref == nil {
		t.setRef(n, new(Ref))
	}
	host := View{PointerEvents: PointerEventsBoxNone,
		StyleFunc: func() Style {
			size := sc.barSize.Get()
			return Style{Position: Absolute, Left: Pt(0), Top: Pt(0), Width: Pt(size.Width), Height: Pt(size.Height)}
		},
		Children: []Node{Dynamic(func(cx *Scope) Node {
			m := sc.bar.Get()
			var bar Node
			if m != nil {
				bar = m.Scrollbar(cx, n.ref, sc.config.Horizontal)
			}
			if hide := sc.hideIndicator || bar != nil; hide != sc.config.HideIndicator {
				sc.config.HideIndicator = hide
				t.touch(n)
			}
			return bar
		})}}
	t.within(cx, func() { t.mount(cx, n, host) })
	sc.barHost = n.children[len(n.children)-1]
	if t.sticky == nil {
		t.sticky = map[*node]bool{}
	}
	t.sticky[sc.barHost] = true
}

// sizeBars keeps each bar as large as its viewport.
func (t *Tree) sizeBars() {
	for n := range t.sticky {
		if n.deleted {
			continue
		}
		if p := n.parent; p != nil && p.scroll != nil && p.scroll.barHost == n {
			p.scroll.barSize.Set(p.frame.Size)
		}
	}
}

// addChild puts n last among parent's children, before a scroll view's
// bar.
func addChild(parent, n *node) {
	if sc := parent.scroll; sc != nil && sc.barHost != nil {
		if i := slices.Index(parent.children, sc.barHost); i >= 0 {
			parent.children = slices.Insert(parent.children, i, n)
			return
		}
	}
	parent.children = append(parent.children, n)
}

// contentBottom is how far down scroll view n's content reaches: layout's,
// or with a bar the views' but the bar's host, as tall as the viewport.
func (n *node) contentBottom() Points {
	if n.scroll.barHost == nil {
		return n.scroll.contentEnd
	}
	var end Points
	var walk func(parent *node)
	walk = func(parent *node) {
		for _, c := range parent.children {
			switch {
			case c == n.scroll.barHost:
			case c.kind == 0:
				walk(c)
			default:
				end = max(end, c.frame.Origin.Y+c.frame.Size.Height)
			}
		}
	}
	walk(n)
	return end
}

// barRange is the bar host n's travel: the whole scroll range.
func barRange(n *node) (*node, stickyRange, bool) {
	sv := n.parent
	end := sv.scroll.main(Point{X: sv.scroll.content.Width - sv.frame.Size.Width, Y: sv.scroll.content.Height - sv.frame.Size.Height})
	return sv, stickyRange{Cell: n.id, End: max(end, 0)}, true
}

// scrollWatch is what Scope.WatchScroll holds.
type scrollWatch struct {
	owner  *Scope
	target func() *Ref
	fn     func(ScrollEvent)
	last   ScrollEvent
	said   bool
}

// WatchScroll has fn hear where the scroll view or list target returns is
// whenever its offset, viewport or content changes, from the first time
// it is laid out, as long as cx lives. target is read in each commit, so
// a ref bound later in the same render is found once it is mounted. The
// report follows the commit, or the platform's event, that moved it.
func (cx *Scope) WatchScroll(target func() *Ref, fn func(ScrollEvent)) {
	t := cx.tree()
	if t == nil {
		return
	}
	w := &scrollWatch{owner: cx, target: target, fn: fn}
	t.watches = append(t.watches, w)
	t.changed = true
	cx.Cleanup(func() { t.watches = slices.DeleteFunc(t.watches, func(o *scrollWatch) bool { return o == w }) })
}

// reportWatches runs the watches whose scroll view or multiline input moved.
func (t *Tree) reportWatches() {
	for _, w := range slices.Clone(t.watches) {
		r := Untracked(w.owner, w.target)
		if r == nil || r.n == nil || r.n.deleted {
			continue
		}
		n := r.n
		var e ScrollEvent
		switch {
		case n.scroll != nil:
			e = n.scrollEvent(n.scroll.offset)
		case n.input != nil && n.input.config.Multiline:
			e = t.inputScrollEvent(n, n.input.scroll.offset)
		default:
			continue
		}
		// The offset replays the scrolls not applied yet: it is not stale.
		e.Stale = false
		if w.said && e == w.last {
			continue
		}
		w.said, w.last = true, e
		t.within(w.owner, func() { w.fn(e) })
	}
}
