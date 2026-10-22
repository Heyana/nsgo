package core

// List renders a long run of rows lazily: only the rows within a screen
// of the viewport are mounted, each in a cell that is reused for another
// row when it scrolls away. A cell's content is built once; its ListRow
// changes, so the content's bindings update instead of it being rebuilt.
// Row sizes start as an estimate and are corrected as rows are laid out,
// kept in a Fenwick tree of prefix sums.
type List struct {
	Count func() int
	// Row builds a cell's content; row says which row it shows.
	Row func(cx *Scope, row ListRow) Node
	// EstimatedSize is a row's expected size along the list; 50 when 0.
	EstimatedSize Points
	Horizontal    bool
	HideIndicator bool
	// Gap separates rows.
	Gap    Points
	Header Node
	Footer Node
	// Empty shows while Count is 0.
	Empty Node
	// OnEndReached runs when the last rows come within EndThreshold rows
	// of the viewport (5 when 0), once per count.
	OnEndReached func()
	EndThreshold int
	OnScroll     func(offset Point)
	Style        Style
	StyleFunc    func() Style
	Ref          *Ref
	TestID       string
}

// Render implements Node.
func (l List) Render(*Scope) Node { return l }

// ListRow is the row a list cell shows, as signals, so content reading
// it follows the cell to its next row.
type ListRow struct {
	index, count *Signal[int]
}

// Index returns the row's index.
func (p ListRow) Index() int { return p.index.Get() }

// Count returns the number of rows.
func (p ListRow) Count() int { return p.count.Get() }

// First reports whether the row is the first.
func (p ListRow) First() bool { return p.index.Get() == 0 }

// Last reports whether the row is the last.
func (p ListRow) Last() bool { return p.index.Get() == p.count.Get()-1 }

type listNode struct {
	spec    List
	owner   *Scope
	scroll  *node // the scroll view
	spacer  *node // sized to all rows
	cells   *node // region holding the cells
	empty   *node // region holding Empty
	count   *Signal[int]
	n       int // rows known to the sizes
	sizes   fenwick
	known   []bool // whether a row's size was measured
	byIndex map[int]*listCell
	free    []*listCell
	endAt   int // the count OnEndReached last ran for
}

type listCell struct {
	scope *Scope
	pos   ListRow
	node  *node // the cell view
	index int
}

func (t *Tree) mountList(cx *Scope, parent *node, l List) {
	sc := t.newNode(KindScroll, parent, cx)
	ln := &listNode{spec: l, owner: cx, scroll: sc, byIndex: map[int]*listCell{}, endAt: -1}
	if ln.spec.EstimatedSize <= 0 {
		ln.spec.EstimatedSize = 50
	}
	if ln.spec.EndThreshold <= 0 {
		ln.spec.EndThreshold = 5
	}
	t.bindStyle(cx, sc, scrollDefaults.Merge(l.Style), l.StyleFunc)
	t.setRef(sc, l.Ref)
	onScroll := l.OnScroll
	sc.h = handlers{scroll: func(p Point) {
		if onScroll != nil {
			onScroll(p)
		}
	}}
	sc.props = ViewProps{TestID: l.TestID}
	sc.scroll = &scrollNode{config: ScrollConfig{Horizontal: l.Horizontal, HideIndicator: l.HideIndicator}}
	sc.list = ln
	ln.count = NewSignal(cx, 0)
	if l.Header != nil {
		t.mount(cx, sc, l.Header)
	}
	ln.spacer = t.newNode(KindView, sc, cx)
	ln.cells = t.newRegion(sc, cx)
	if l.Footer != nil {
		t.mount(cx, sc, l.Footer)
	}
	if l.Empty != nil {
		ln.empty = t.newRegion(sc, cx)
		t.mount(cx, ln.empty, Show{When: func() bool { return ln.count.Get() == 0 }, Then: func(*Scope) Node { return l.Empty }})
	}
	cx.renderEffect(func() {
		n := max(l.Count(), 0)
		ln.count.Set(n)
		t.relayout(sc)
	})
	t.lists = append(t.lists, sc)
}

// axis returns the main-axis length of a size or point.
func (ln *listNode) main(s Size) Points {
	if ln.spec.Horizontal {
		return s.Width
	}
	return s.Height
}

func (ln *listNode) offset() Points {
	if ln.spec.Horizontal {
		return ln.scroll.scroll.offset.X
	}
	return ln.scroll.scroll.offset.Y
}

// update measures the cells laid out, moves the window of rows to the
// viewport and reports whether anything needs another layout.
func (t *Tree) updateList(sc *node) bool {
	ln := sc.list
	changed := false
	count := ln.count.GetUntracked()
	if count != ln.n {
		ln.resize(count)
		changed = true
	}
	// Rows laid out since the last update correct the estimates.
	gap := ln.spec.Gap
	for i, c := range ln.byIndex {
		if c.node.layoutDirty || c.node.frame.Size == (Size{}) {
			continue
		}
		size := ln.main(c.node.frame.Size) + gap
		if !ln.known[i] || ln.sizes.at(i) != size {
			ln.sizes.set(i, size)
			ln.known[i] = true
			changed = true
		}
	}
	// The window: a viewport's length before and after the viewport.
	view := ln.main(sc.frame.Size)
	if view <= 0 {
		view = ln.spec.EstimatedSize * 10
	}
	start := ln.headerSize()
	// An offset past the content, after rows went, is where the platform
	// is about to clamp it.
	off := min(ln.offset(), max(start+ln.sizes.sum(count)-view, 0))
	first := ln.sizes.find(max(off-start-view, 0))
	last := ln.sizes.find(off - start + 2*view)
	last = min(last, count-1)
	// Free the cells outside it, then fill it, reusing them.
	for i, c := range ln.byIndex {
		if i < first || i > last {
			delete(ln.byIndex, i)
			ln.free = append(ln.free, c)
		}
	}
	for i := first; i <= last; i++ {
		if _, ok := ln.byIndex[i]; ok {
			continue
		}
		changed = true
		var c *listCell
		if k := len(ln.free); k > 0 {
			c, ln.free = ln.free[k-1], ln.free[:k-1]
			c.pos.index.Set(i)
		} else {
			c = t.newCell(ln, i)
		}
		c.index = i
		ln.byIndex[i] = c
	}
	// Spare cells hide until a row needs them.
	for _, c := range ln.free {
		if c.node.style.Display != DisplayNone {
			c.node.base.Display = DisplayNone
			c.node.restyle()
			t.relayout(c.node)
		}
	}
	for i, c := range ln.byIndex {
		at := start + ln.sizes.sum(i)
		if c.node.base.Display == DisplayNone || ln.cellAt(c) != at {
			c.node.base.Display = 0
			ln.place(c, at)
			c.node.restyle()
			t.relayout(c.node)
			changed = true
		}
		c.pos.count.Set(count)
	}
	total := ln.sizes.sum(count)
	if count > 0 {
		total -= gap
	}
	if ln.main(Size{ln.spacerLen(), ln.spacerLen()}) != total {
		ln.setSpacer(total)
		t.relayout(ln.spacer)
		changed = true
	}
	if f := ln.spec.OnEndReached; f != nil && count > 0 && last >= count-ln.spec.EndThreshold && ln.endAt != count {
		ln.endAt = count
		t.after(sc, f)
	}
	return changed
}

// headerSize is where rows start: the scroll view's padding, the header
// and the gaps after it.
func (ln *listNode) headerSize() Points {
	st := ln.scroll.style
	s := st.Padding.Top + st.borders().Top
	if ln.spec.Horizontal {
		s = st.Padding.Left + st.borders().Left
	}
	for _, c := range ln.scroll.flatChildren() {
		if c == ln.spacer {
			break
		}
		if c.style.Position != Absolute && c.style.Display != DisplayNone {
			s += ln.main(c.frame.Size) + st.Gap
		}
	}
	return s
}

func (ln *listNode) cellAt(c *listCell) Points {
	if ln.spec.Horizontal {
		return Points(c.node.base.Left.value)
	}
	return Points(c.node.base.Top.value)
}

func (ln *listNode) place(c *listCell, at Points) {
	s, p := &c.node.base, ln.scroll.style.Padding
	if ln.spec.Horizontal {
		s.Left, s.Top, s.Bottom, s.Right = Pt(at), Pt(p.Top), Pt(p.Bottom), Length{}
	} else {
		s.Top, s.Left, s.Right, s.Bottom = Pt(at), Pt(p.Left), Pt(p.Right), Length{}
	}
}

func (ln *listNode) spacerLen() Points {
	if ln.spec.Horizontal {
		return Points(ln.spacer.base.Width.value)
	}
	return Points(ln.spacer.base.Height.value)
}

func (ln *listNode) setSpacer(total Points) {
	if ln.spec.Horizontal {
		ln.spacer.base.Width = Pt(total)
	} else {
		ln.spacer.base.Height = Pt(total)
	}
	ln.spacer.restyle()
}

func (ln *listNode) resize(count int) {
	est := ln.spec.EstimatedSize + ln.spec.Gap
	sizes := make([]Points, count)
	known := make([]bool, count)
	for i := range sizes {
		if i < ln.n && ln.known[i] {
			sizes[i], known[i] = ln.sizes.at(i), true
		} else {
			sizes[i] = est
		}
	}
	ln.sizes = newFenwick(sizes)
	ln.known = known
	ln.n = count
	for i, c := range ln.byIndex {
		if i >= count {
			delete(ln.byIndex, i)
			ln.free = append(ln.free, c)
		}
	}
}

func (t *Tree) newCell(ln *listNode, i int) *listCell {
	c := &listCell{scope: &Scope{}, index: i}
	ln.owner.adopt(c.scope)
	c.pos = ListRow{index: NewSignal(c.scope, i), count: NewSignal(c.scope, ln.count.GetUntracked())}
	t.within(c.scope, func() {
		c.node = t.newNode(KindView, ln.cells, c.scope)
		c.node.base = Style{Position: Absolute}
		c.node.restyle()
		t.mount(c.scope, c.node, ln.spec.Row(c.scope, c.pos))
	})
	return c
}

// fenwick is a Fenwick tree of row sizes: prefix sums and the row at an
// offset in O(log n).
type fenwick struct {
	tree []Points // 1-based
	vals []Points
}

func newFenwick(vals []Points) fenwick {
	f := fenwick{tree: make([]Points, len(vals)+1), vals: vals}
	for i, v := range vals {
		f.tree[i+1] += v
		if j := i + 1 + (i+1)&-(i+1); j <= len(vals) {
			f.tree[j] += f.tree[i+1]
		}
	}
	return f
}

func (f *fenwick) at(i int) Points { return f.vals[i] }

func (f *fenwick) set(i int, v Points) {
	d := v - f.vals[i]
	f.vals[i] = v
	for j := i + 1; j < len(f.tree); j += j & -j {
		f.tree[j] += d
	}
}

// sum returns the total of the first n sizes.
func (f *fenwick) sum(n int) Points {
	var s Points
	for j := min(n, len(f.vals)); j > 0; j -= j & -j {
		s += f.tree[j]
	}
	return s
}

// find returns the row at offset: the first i whose sum(i+1) > offset.
func (f *fenwick) find(offset Points) int {
	pos := 0
	step := 1
	for step*2 <= len(f.vals) {
		step *= 2
	}
	for ; step > 0; step /= 2 {
		if next := pos + step; next <= len(f.vals) && f.tree[next] <= offset {
			pos = next
			offset -= f.tree[next]
		}
	}
	return pos
}
