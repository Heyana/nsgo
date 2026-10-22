package core

import "strings"

// maxRounds bounds the commits OnLayout handlers may cause in one batch.
const maxRounds = 8

// Commit resolves styles, prepares text, lays the tree out and returns
// what changed since the last commit, or nil when nothing did. Changes
// that OnLayout handlers make go into the same batch.
func (t *Tree) Commit() *MountBatch {
	if !t.changed {
		return nil
	}
	t.items = t.items[:0]
	for round := 0; t.changed && round < maxRounds; round++ {
		t.commitRound()
	}
	if len(t.items) == 0 {
		return nil
	}
	return &MountBatch{Root: t.view.id, Items: append([]MountItem(nil), t.items...)}
}

func (t *Tree) commitRound() {
	t.changed = false
	for _, id := range t.deleted {
		t.items = append(t.items, MountItem{Kind: ItemDelete, ID: id})
	}
	t.fontScale = t.root.Env().fontScale.GetUntracked()
	if rtl := t.root.Env().rtl.GetUntracked(); rtl != t.rtl {
		t.setRTL(rtl)
	}
	live := t.stacks[:0]
	for _, s := range t.stacks {
		if !s.deleted {
			t.showTop(s)
			live = append(live, s)
		}
	}
	t.stacks = live
	t.resolve(t.view, Style{})
	t.layout()
	// Lists fill their windows from the layout, which may need another.
	for range 4 {
		changed := false
		live := t.lists[:0]
		for _, l := range t.lists {
			if !l.deleted {
				live = append(live, l)
				changed = t.within2(l, func() bool { return t.updateList(l) }) || changed
			}
		}
		t.lists = live
		if !changed {
			break
		}
		t.resolve(t.view, Style{})
		t.layout()
	}
	t.sizeCanvases(t.view)
	t.laidOut = t.laidOut[:0]
	t.emit(t.view)
	if t.keysChanged {
		t.keysChanged = false
		t.items = append(t.items, MountItem{Kind: ItemKeymap, Keys: t.keymap()})
	}
	t.items = append(t.items, t.commands...)
	t.commands = t.commands[:0]
	for _, id := range t.deleted {
		t.nodes[id] = nil
		t.free = append(t.free, id)
	}
	t.deleted = t.deleted[:0]
	for _, n := range t.laidOut {
		if !n.deleted && n.h.layout != nil {
			frame := n.frame
			t.within(n.owner, func() { n.h.layout(frame) })
		}
	}
	reports := t.reports
	t.reports = nil
	for _, r := range reports {
		if !r.owner.deleted {
			t.within(r.owner.owner, r.fn)
		}
	}
}

// resolve inherits text styles down the tree and prepares changed text.
func (t *Tree) resolve(n *node, inherited Style) {
	if n.kind != 0 {
		inherited = n.style.inherit(inherited)
	}
	switch {
	case n.text != nil:
		n.props.SelectionColor = inherited.SelectionColor
		tn := n.text
		ts := t.scaled(inherited.textStyle())
		if tn.stale || tn.prepared == nil || ts != tn.style {
			at := AttributedText{Text: tn.content, Style: ts, MaxLines: tn.maxLines}
			if len(tn.spans) > 0 {
				var b strings.Builder
				for _, sp := range tn.spans {
					start := b.Len()
					b.WriteString(sp.content)
					at.Spans = append(at.Spans, TextSpan{Start: start, End: b.Len(),
						Style: t.scaled(sp.style.inherit(inherited).textStyle()), Link: sp.press != nil})
				}
				at.Text = b.String()
			}
			tn.style = ts
			tn.stale = false
			if tn.prepared == nil || !at.equal(&tn.text) {
				if tn.prepared != nil {
					tn.prepared.Release()
				}
				tn.text = at
				tn.prepared = t.text.Prepare(&at)
				tn.resent = true
				n.markLayout()
			}
		}
	case n.input != nil:
		in := n.input
		if ts := t.scaled(inherited.textStyle()); ts != in.config.Style || in.lineH == 0 {
			in.config.Style = ts
			p := t.text.Prepare(&AttributedText{Text: "Ag", Style: ts})
			in.lineH = p.Measure(Auto).Height
			p.Release()
			n.markLayout()
		}
	}
	for _, c := range n.children {
		t.resolve(c, inherited)
	}
}

// sizeCanvases tells canvases their laid-out size, which draws those
// that changed.
func (t *Tree) sizeCanvases(n *node) {
	if n.canvas != nil && n.canvas.size.GetUntracked() != n.frame.Size {
		size := n.frame.Size
		t.within(n.owner, func() { n.canvas.size.Set(size) })
	}
	for _, c := range n.nativeChildren() {
		t.sizeCanvases(c)
	}
}

// within2 runs fn under n's owner and returns its result.
func (t *Tree) within2(n *node, fn func() bool) (r bool) {
	t.within(n.owner, func() { r = fn() })
	return r
}

// scaled applies the user's text size setting.
func (t *Tree) scaled(ts TextStyle) TextStyle {
	if s := Points(t.fontScale); s != 1 && s > 0 {
		ts.Font.Size *= s
		ts.LineHeight *= s
	}
	return ts
}

func (t *Tree) emit(n *node) {
	if n.kind == 0 {
		for _, c := range n.children {
			t.emit(c)
		}
		return
	}
	add := func(it MountItem) { t.items = append(t.items, it) }
	first := !n.created
	if first {
		add(MountItem{Kind: ItemCreate, ID: n.id, View: n.kind})
		n.created = true
	}
	if vs := n.viewStyle(); first || vs != n.sent.viewStyle {
		n.sent.viewStyle = vs
		add(MountItem{Kind: ItemViewStyle, ID: n.id, ViewStyle: &vs})
	}
	if first || n.props != n.sent.props {
		p := n.props
		n.sent.props = p
		add(MountItem{Kind: ItemViewProps, ID: n.id, ViewProps: &p})
	}
	if ev := n.events(); first || ev != n.sent.events {
		n.sent.events = ev
		add(MountItem{Kind: ItemEvents, ID: n.id, Events: ev})
	}
	switch {
	case n.text != nil:
		tn := n.text
		if tn.prepared != nil && (tn.resent || tn.box != tn.sentBox) {
			tn.resent = false
			tn.sentBox = tn.box
			add(MountItem{Kind: ItemText, ID: n.id, Text: &TextContent{
				Text: tn.text.Text, Layout: tn.prepared.Layout(tn.box.Size), Content: tn.box,
			}})
		}
	case n.input != nil:
		in := n.input
		if first || in.config != n.sent.input {
			c := in.config
			n.sent.input = c
			add(MountItem{Kind: ItemInput, ID: n.id, Input: &c})
		}
		if first || in.value != in.shown {
			in.shown = in.value
			add(MountItem{Kind: ItemInputValue, ID: n.id, Value: in.value, Count: in.count})
		}
		if first && in.autoFocus {
			t.command(ItemFocus, n)
		}
	case n.image != nil:
		im := n.image
		c := im.config
		c.Source = t.pickScale(c.Source)
		c.Placeholder = t.pickScale(c.Placeholder)
		if first || !c.same(im.sent) {
			im.sent = c
			add(MountItem{Kind: ItemImage, ID: n.id, Image: &c})
		}
	case n.hoist != nil && n.hoist.modal != nil:
		if first {
			add(MountItem{Kind: ItemModal, ID: n.id, Modal: &ModalConfig{Animation: n.hoist.modal.Animation}})
		}
	case n.canvas != nil:
		if cn := n.canvas; cn.fresh && cn.drawing != nil {
			cn.fresh = false
			add(MountItem{Kind: ItemCanvas, ID: n.id, Drawing: cn.drawing})
		}
	case n.scroll != nil:
		sc := n.scroll
		if first || sc.config != n.sent.scroll {
			c := sc.config
			n.sent.scroll = c
			add(MountItem{Kind: ItemScrollConfig, ID: n.id, Scroll: &c})
		}
		if first || sc.content != n.sent.content {
			n.sent.content = sc.content
			add(MountItem{Kind: ItemScrollContent, ID: n.id, Size: sc.content})
		}
	}
	kids := n.nativeChildren()
	for _, c := range kids {
		t.emit(c)
	}
	if n.childrenDirty {
		t.syncChildren(n, kids)
		n.childrenDirty = false
	}
	if first || n.frame != n.sent.frame {
		n.sent.frame = n.frame
		add(MountItem{Kind: ItemFrame, ID: n.id, Frame: n.frame})
		if n.h.layout != nil {
			t.laidOut = append(t.laidOut, n)
		}
	}
}

func (n *node) viewStyle() ViewStyle {
	vs := n.style.viewStyle()
	vs.Hidden = n.style.Display == DisplayNone
	if n.rtl && vs.Borders != (Edges{}) {
		vs.Borders.Left, vs.Borders.Right = vs.Borders.Right, vs.Borders.Left
		vs.BorderColors.Left, vs.BorderColors.Right = vs.BorderColors.Right, vs.BorderColors.Left
	}
	return vs
}

// syncChildren inserts, moves and removes p's native children so they
// match want, moving as few as the longest run already in order allows.
func (t *Tree) syncChildren(p *node, want []*node) {
	old := p.sent.children
	pos := make(map[*node]int, len(old))
	for i, c := range old {
		if !c.deleted {
			pos[c] = i
		}
	}
	keep := make(map[*node]bool, len(want))
	for _, c := range want {
		keep[c] = true
	}
	for _, c := range old {
		if !c.deleted && !keep[c] {
			t.items = append(t.items, MountItem{Kind: ItemRemove, ID: c.id})
		}
	}
	seq := make([]int, len(want))
	for i, c := range want {
		if j, ok := pos[c]; ok {
			seq[i] = j
		} else {
			seq[i] = -1
		}
	}
	stay := increasingRun(seq)
	for i := len(want) - 1; i >= 0; i-- {
		if stay[i] {
			continue
		}
		var before NodeID
		if i+1 < len(want) {
			before = want[i+1].id
		}
		t.items = append(t.items, MountItem{Kind: ItemInsert, ID: want[i].id, Parent: p.id, Before: before})
	}
	p.sent.children = append(p.sent.children[:0], want...)
}

// increasingRun marks a longest strictly increasing subsequence of seq,
// ignoring negative entries.
func increasingRun(seq []int) []bool {
	stay := make([]bool, len(seq))
	tails := []int{} // index into seq of the smallest tail of each length
	prev := make([]int, len(seq))
	for i, v := range seq {
		if v < 0 {
			continue
		}
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if seq[tails[mid]] < v {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo > 0 {
			prev[i] = tails[lo-1]
		} else {
			prev[i] = -1
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	if len(tails) > 0 {
		for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
			stay[i] = true
		}
	}
	return stay
}
