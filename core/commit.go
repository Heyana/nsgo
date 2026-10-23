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
	b := &MountBatch{Root: t.view.id, Items: append([]MountItem(nil), t.items...), LayoutAnimation: t.layoutAnim}
	t.layoutAnim = nil
	return b
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
	t.emitAccessories()
	t.emit(t.view)
	t.emitAnimation()
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
	case n.icon != nil:
		// An icon paints in the text color it inherits.
		in := n.icon
		in.text = t.scaled(inherited.textStyle())
		t.within(n.owner, func() { in.color.Set(in.text.Color) })
	case n.text != nil:
		n.props.SelectionColor = inherited.SelectionColor
		tn := n.text
		ts := t.scaled(inherited.textStyle())
		sized := tn.attachments > 0 && t.sizeAttachments(n, inherited)
		if tn.stale || sized || tn.prepared == nil || ts != tn.style {
			at := AttributedText{Text: tn.content, Style: ts, MaxLines: tn.maxLines}
			if len(tn.spans) > 0 {
				var b strings.Builder
				for _, sp := range tn.spans {
					start := b.Len()
					if w := sp.attach; w != nil {
						b.WriteRune(AttachmentRune)
						at.Attachments = append(at.Attachments, TextAttachment{Offset: start,
							Size: w.attach.size, Align: w.verticalAlign()})
						at.Spans = append(at.Spans, TextSpan{Start: start, End: b.Len(), Style: ts})
						continue
					}
					b.WriteString(sp.content)
					at.Spans = append(at.Spans, TextSpan{Start: start, End: b.Len(),
						Style: t.scaled(sp.style.inherit(inherited).textStyle()), Link: sp.press != nil,
						Background: TextBackground{Color: sp.style.Background, Radius: sp.style.CornerRadius}})
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
	if n.text != nil {
		return // its children are attachments, which it resolved
	}
	for _, c := range n.children {
		t.resolve(c, inherited)
	}
}

// attachmentSpace is the space attachments are laid out in: they take
// their content size.
const attachmentSpace = 1 << 20

// sizeAttachments resolves and lays out the attachments of text n, each a
// layout root at its content size, and reports whether a size changed.
func (t *Tree) sizeAttachments(n *node, inherited Style) bool {
	changed := false
	for _, sp := range n.text.spans {
		w := sp.attach
		if w == nil {
			continue
		}
		t.resolve(w, inherited)
		if !w.layoutDirty {
			continue
		}
		t.layoutRoot(w, Size{attachmentSpace, attachmentSpace}, false)
		t.syncFrames(w)
		l := &w.lstate.Rounded
		if size := (Size{Points(l.Size.Width), Points(l.Size.Height)}); size != w.attach.size {
			w.attach.size = size
			changed = true
		}
	}
	return changed
}

// verticalAlign is where attachment view w sits in its line: its span's
// VerticalAlign, else that of the one view the attachment made.
func (w *node) verticalAlign() VerticalAlign {
	if a := w.style.VerticalAlign; a != 0 {
		return a
	}
	if kids := w.flatChildren(); len(kids) == 1 {
		return kids[0].style.VerticalAlign
	}
	return 0
}

// placeAttachments puts the views of text n's attachments where layout
// put their placeholders, hiding those its line limit cut off.
func (n *node) placeAttachments(l TextLayout) {
	al, _ := l.(AttachmentLayout)
	i := 0
	for _, sp := range n.text.spans {
		w := sp.attach
		if w == nil {
			continue
		}
		var b Bounds
		ok := false
		if al != nil {
			b, ok = al.Attachment(i)
		}
		i++
		w.attach.cut = !ok
		if ok {
			b.Origin.X += n.text.box.Origin.X
			b.Origin.Y += n.text.box.Origin.Y
			w.frame = Bounds{Origin: b.Origin, Size: w.attach.size}
		}
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
	if n.style.Animated != nil || n.anim != nil {
		t.bindAnimated(n)
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
			l := tn.prepared.Layout(tn.box.Size)
			if tn.attachments > 0 {
				n.placeAttachments(l)
			}
			add(MountItem{Kind: ItemText, ID: n.id, Text: &TextContent{
				Text: tn.text.Text, Layout: l, Content: tn.box,
			}})
		}
	case n.input != nil:
		in := n.input
		in.config.Accessory = t.accessoryFor(in.accessory)
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
	case n.web != nil:
		if wn := n.web; first || wn.config != wn.sent {
			c := wn.config
			wn.sent = c
			add(MountItem{Kind: ItemWeb, ID: n.id, Web: &c})
		}
	case n.native != nil:
		t.emitNative(n)
	case n.video != nil:
		if vn := n.video; first || vn.config != vn.sent {
			c := vn.config
			vn.sent = c
			add(MountItem{Kind: ItemVideo, ID: n.id, Video: &c})
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
	vs.Hidden = n.style.Display == DisplayNone || (n.attach != nil && n.attach.cut)
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
