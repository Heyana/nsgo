package core

// Overlays and modals stay where they are built in the shadow tree, for
// their owner, context, inherited text style and lifetime, but their
// native views go into their host: the nearest modal around them, or the
// root view. Each is a layout root of its own at its host's size.

type hoistNode struct {
	host    *node
	overlay *Overlay // nil for a modal
	modal   *Modal
	placed  Placement
}

// hostOf returns the root view or the nearest modal around parent.
func (t *Tree) hostOf(parent *node) *node {
	for p := parent; p != nil; p = p.parent {
		if p.kind == KindModal {
			return p
		}
	}
	return t.view
}

func (t *Tree) hoist(n *node, h *hoistNode) {
	h.host = t.hostOf(n.parent)
	n.hoist = h
	h.host.hosted = append(h.host.hosted, n)
	h.host.childrenDirty = true
	if p := n.parent; p != nil {
		t.childrenChanged(p)
	}
}

func (t *Tree) mountOverlay(cx *Scope, parent *node, o Overlay) {
	n := t.newNode(KindView, parent, cx)
	t.hoist(n, &hoistNode{overlay: &o})
	t.bindStyle(cx, n, o.Style, o.StyleFunc)
	n.h = handlers{pressOutside: o.OnPressOutside, cancel: o.OnCancel}
	n.props = ViewProps{Role: o.Role, Label: o.Label, TestID: o.TestID}
	for _, c := range o.Children {
		t.mount(cx, n, c)
	}
}

func (t *Tree) mountModal(cx *Scope, parent *node, m Modal) {
	n := t.newNode(KindModal, parent, cx)
	t.hoist(n, &hoistNode{modal: &m})
	t.bindStyle(cx, n, m.Style, m.StyleFunc)
	n.props = ViewProps{Role: m.Role, Label: m.Label, TestID: m.TestID}
	if n.props.Role == RoleNone {
		n.props.Role = RoleDialog
	}
	// What a modal's content keeps clear of is the surface's safe area.
	inner := cx.Child()
	insetsKey.Provide(inner, func() Edges { return cx.Env().SafeArea() })
	t.within(inner, func() {
		for _, c := range m.Children {
			t.mount(inner, n, c)
		}
	})
}

// placeHoisted lays out and places the overlays and modals host holds,
// and those inside them.
func (t *Tree) placeHoisted(host *node) {
	for _, n := range host.hosted {
		if n.deleted {
			continue
		}
		size := host.frame.Size
		if n.hoist.modal != nil {
			t.layoutRoot(n, size, true)
			n.frame = Bounds{Size: size}
		} else {
			t.placeOverlay(n, host, size)
		}
		// Its native view is its host's child, so a hidden shadow ancestor
		// hides it only through the core.
		if n.hidden = hiddenIn(n); n.hidden {
			n.frame = Bounds{}
		}
		t.syncFrames(n)
		t.placeHoisted(n)
	}
}

// hiddenIn reports whether DisplayNone hides n or a shadow ancestor.
func hiddenIn(n *node) bool {
	for ; n != nil; n = n.parent {
		if n.style.Display == DisplayNone {
			return true
		}
	}
	return false
}

// placeOverlay sizes overlay n to its content within its host and places
// it beside its anchor, or by its insets in its host without one.
func (t *Tree) placeOverlay(n *node, host *node, hostSize Size) {
	o := n.hoist.overlay
	s := n.style
	w, h := hostSize.Width, hostSize.Height
	t.layoutRoot(n, hostSize, false)
	l := &n.lstate.Rounded
	size := Size{Points(l.Size.Width), Points(l.Size.Height)}
	var origin Point
	anchor, ok := t.anchorRect(o.Anchor, host)
	if !ok {
		// Without an anchor, an overlay is placed as an absolute child of
		// its host.
		left, right := s.Left.resolve(w), s.Right.resolve(w)
		if n.rtl {
			left, right = right, left
		}
		tp, b := s.Top.resolve(h), s.Bottom.resolve(h)
		switch {
		case !left.IsAuto():
			origin.X = left
		case !right.IsAuto():
			origin.X = w - right - size.Width
		}
		switch {
		case !tp.IsAuto():
			origin.Y = tp
		case !b.IsAuto():
			origin.Y = h - b - size.Height
		}
	} else {
		safe := t.root.Env().safe.GetUntracked()
		if host.kind == KindModal {
			safe = Edges{}
		}
		area := Bounds{
			Origin: Point{safe.Left + o.Margin, safe.Top + o.Margin},
			Size: Size{max(w-safe.horizontal()-2*o.Margin, 0),
				max(h-safe.vertical()-2*o.Margin, 0)},
		}
		rtl := t.root.Env().rtl.GetUntracked()
		var side Placement
		want := o.Placement
		if want == PlaceDefault {
			want = PlaceBottom
		}
		origin, side = place(anchor, size, area, want, o.Align, o.Offset, rtl)
		if side != n.hoist.placed && o.OnPosition != nil {
			f := o.OnPosition
			t.after(n, func() { f(side) })
		}
		n.hoist.placed = side
	}
	n.frame = Bounds{origin, size}
}

// anchorRect returns ref's bounds in host's coordinates.
func (t *Tree) anchorRect(ref *Ref, host *node) (Bounds, bool) {
	if ref == nil || ref.n == nil || ref.n.deleted {
		return Bounds{}, false
	}
	b := t.surfaceBounds(ref.n)
	if host != t.view {
		hb := t.surfaceBounds(host)
		b.Origin.X -= hb.Origin.X
		b.Origin.Y -= hb.Origin.Y
	}
	return b, true
}

// surfaceBounds returns n's frame in surface points: its native
// ancestors' frames added, their scroll offsets taken off.
func (t *Tree) surfaceBounds(n *node) Bounds {
	b := n.frame
	for p := n.nativeParent(); p != nil; p = p.nativeParent() {
		b.Origin.X += p.frame.Origin.X
		b.Origin.Y += p.frame.Origin.Y
		if p.scroll != nil {
			b.Origin.X -= p.scroll.offset.X
			b.Origin.Y -= p.scroll.offset.Y
		}
	}
	return b
}

// place puts an overlay of size beside anchor inside area, as NSH does:
// on the side asked unless it does not fit there and the opposite side
// fits or has more room, aligned along that side, then clamped into area.
// In right-to-left layouts start and end swap for top and bottom.
func place(anchor Bounds, size Size, area Bounds, side Placement, align Align, offset Points, rtl bool) (Point, Placement) {
	aLeft, aTop := anchor.Origin.X, anchor.Origin.Y
	aRight, aBottom := aLeft+anchor.Size.Width, aTop+anchor.Size.Height
	lo, hi := area.Origin, Point{area.Origin.X + area.Size.Width, area.Origin.Y + area.Size.Height}
	room := func(p Placement) Points {
		switch p {
		case PlaceTop:
			return aTop - offset - lo.Y
		case PlaceBottom:
			return hi.Y - aBottom - offset
		case PlaceLeft:
			return aLeft - offset - lo.X
		}
		return hi.X - aRight - offset
	}
	need := func(p Placement) Points {
		if p == PlaceTop || p == PlaceBottom {
			return size.Height
		}
		return size.Width
	}
	opposite := map[Placement]Placement{PlaceTop: PlaceBottom, PlaceBottom: PlaceTop, PlaceLeft: PlaceRight, PlaceRight: PlaceLeft}[side]
	if room(side) < need(side) && (room(opposite) >= need(opposite) || room(opposite) > room(side)) {
		side = opposite
	}
	along := func(a0, a1, length Points, swap bool) Points {
		start, end := a0, a1-length
		if swap {
			start, end = end, start
		}
		switch align {
		case AlignStart:
			return start
		case AlignEnd:
			return end
		}
		return a0 + (a1-a0-length)/2
	}
	var p Point
	switch side {
	case PlaceTop:
		p = Point{along(aLeft, aRight, size.Width, rtl), aTop - offset - size.Height}
	case PlaceBottom:
		p = Point{along(aLeft, aRight, size.Width, rtl), aBottom + offset}
	case PlaceLeft:
		p = Point{aLeft - offset - size.Width, along(aTop, aBottom, size.Height, false)}
	case PlaceRight:
		p = Point{aRight + offset, along(aTop, aBottom, size.Height, false)}
	}
	p.X = max(min(p.X, hi.X-size.Width), lo.X)
	p.Y = max(min(p.Y, hi.Y-size.Height), lo.Y)
	return p, side
}

// back routes Escape or back: the topmost overlay listening for cancel,
// else the topmost modal, else the innermost stack that can pop.
func (t *Tree) back() {
	host := t.view
	for {
		top := topModal(host)
		if top == nil {
			break
		}
		host = top
	}
	if o := topOverlay(host); o != nil {
		t.within(o.owner, o.h.cancel)
		return
	}
	if host.kind == KindModal {
		if stack := t.innerStack(host); stack != nil && t.pop(stack) {
			return
		}
		if f := host.hoist.modal.OnRequestClose; f != nil {
			t.within(host.owner, func() { f("cancel") })
		}
		return
	}
	if stack := t.innerStack(host); stack != nil && t.pop(stack) {
		return
	}
	t.commands = append(t.commands, MountItem{Kind: ItemBackUnhandled})
	t.changed = true
}

func topModal(host *node) *node {
	for i := len(host.hosted) - 1; i >= 0; i-- {
		if n := host.hosted[i]; !n.deleted && n.kind == KindModal && !hiddenIn(n) {
			return n
		}
	}
	return nil
}

func topOverlay(host *node) *node {
	for i := len(host.hosted) - 1; i >= 0; i-- {
		if n := host.hosted[i]; !n.deleted && n.kind != KindModal && n.h.cancel != nil && !hiddenIn(n) {
			return n
		}
	}
	return nil
}

// pressOutside reports a press outside overlay n unless it landed on its
// anchor or an overlay above it.
func (t *Tree) pressOutside(n *node, at Point) {
	if n.h.pressOutside == nil || hiddenIn(n) {
		return
	}
	if a, ok := t.anchorRect(n.hoist.overlay.Anchor, t.view); ok && a.Contains(at) {
		return
	}
	host := n.hoist.host
	above := false
	for _, o := range host.hosted {
		if o == n {
			above = true
			continue
		}
		if above && !o.deleted && !o.hidden && t.surfaceBounds(o).Contains(at) {
			return
		}
	}
	t.within(n.owner, func() { n.h.pressOutside(at) })
}
