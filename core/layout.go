package core

import "math"

// Layout is CSS flexbox with React Native's defaults: column direction,
// no shrinking, children stretched on the cross axis, border-box sizes.
// It supports wrapping, grow, shrink and basis, min and max sizes,
// margins, gaps, aspect ratio, absolute positioning and display none.
//
// Each node is measured at any number of constraints (cached until it or
// a descendant changes) and placed once per layout at its final size.

type measured struct {
	w, h, aw, ah Points
	size         Size
}

func same(a, b Points) bool { return a == b || (a.IsAuto() && b.IsAuto()) }

func sub(a, b Points) Points {
	if a.IsAuto() {
		return Auto
	}
	return max(a-b, 0)
}

func clamp(v, lo, hi Points) Points {
	if !hi.IsAuto() && v > hi {
		v = hi
	}
	if !lo.IsAuto() && v < lo {
		v = lo
	}
	return v
}

func ceil(p Points) Points { return Points(math.Ceil(float64(p))) }

func (n *node) insets() Edges {
	return n.style.Padding.add(All(n.style.BorderWidth))
}

func (n *node) contentBox(size Size) Bounds {
	in := n.insets()
	return Bounds{
		Origin: Point{in.Left, in.Top},
		Size:   Size{max(size.Width-in.horizontal(), 0), max(size.Height-in.vertical(), 0)},
	}
}

// setLimits resolves n's min and max sizes against its parent's inner
// size, dropping cached measures when they change.
func (n *node) setLimits(innerW, innerH Points) {
	s := n.style
	l := [4]Points{s.MinWidth.resolve(innerW), s.MaxWidth.resolve(innerW),
		s.MinHeight.resolve(innerH), s.MaxHeight.resolve(innerH)}
	for i := range l {
		if !same(l[i], n.limits[i]) {
			n.limits = l
			n.cache = n.cache[:0]
			return
		}
	}
}

// measure returns n's border-box size: w and h are definite sizes or
// Auto, aw and ah the space available when they are Auto.
func (t *Tree) measure(n *node, w, h, aw, ah Points) Size {
	for _, m := range n.cache {
		if same(m.w, w) && same(m.h, h) && same(m.aw, aw) && same(m.ah, ah) {
			return m.size
		}
	}
	size := t.measureUncached(n, w, h, aw, ah)
	if len(n.cache) >= 8 {
		n.cache = n.cache[:0]
	}
	n.cache = append(n.cache, measured{w, h, aw, ah, size})
	return size
}

func (t *Tree) measureUncached(n *node, w, h, aw, ah Points) Size {
	s := n.style
	minW, maxW, minH, maxH := n.limits[0], n.limits[1], n.limits[2], n.limits[3]
	if !w.IsAuto() {
		w = clamp(w, minW, maxW)
	}
	if !h.IsAuto() {
		h = clamp(h, minH, maxH)
	}
	if ar := s.AspectRatio; ar > 0 {
		if !w.IsAuto() && h.IsAuto() {
			h = clamp(w/Points(ar), minH, maxH)
		} else if w.IsAuto() && !h.IsAuto() {
			w = clamp(h*Points(ar), minW, maxW)
		}
	}
	if w.IsAuto() && !maxW.IsAuto() && (aw.IsAuto() || aw > maxW) {
		aw = maxW
	}
	if h.IsAuto() && !maxH.IsAuto() && (ah.IsAuto() || ah > maxH) {
		ah = maxH
	}
	in := n.insets()
	switch n.kind {
	case KindText:
		if !w.IsAuto() && !h.IsAuto() {
			return Size{w, h}
		}
		wrap := aw
		if !w.IsAuto() {
			wrap = w
		}
		var m Size
		if n.text.prepared != nil {
			m = n.text.prepared.Measure(sub(wrap, in.horizontal()))
		}
		if w.IsAuto() {
			w = clamp(ceil(m.Width)+in.horizontal(), minW, maxW)
		}
		if h.IsAuto() {
			h = clamp(ceil(m.Height)+in.vertical(), minH, maxH)
		}
		return Size{w, h}
	case KindInput:
		if h.IsAuto() {
			h = clamp(ceil(n.input.lineH)+in.vertical(), minH, maxH)
		}
		if w.IsAuto() {
			w = clamp(160+in.horizontal(), minW, maxW)
		}
		return Size{w, h}
	}
	size := t.flex(n, w, h, aw, ah, false)
	if w.IsAuto() {
		if cw := clamp(size.Width, minW, maxW); cw != size.Width {
			size = t.flex(n, cw, h, aw, ah, false)
			size.Width = cw
		}
	}
	if h.IsAuto() {
		size.Height = clamp(size.Height, minH, maxH)
	}
	return size
}

// place lays out n's content at its final border-box size.
func (t *Tree) place(n *node, size Size) {
	switch n.kind {
	case KindText:
		n.text.box = n.contentBox(size)
	case KindInput:
		n.input.config.Content = n.contentBox(size)
	default:
		t.flex(n, size.Width, size.Height, size.Width, size.Height, true)
	}
	n.layoutDirty = false
}

type flexItem struct {
	n                    *node
	mainStart, mainEnd   Points // margins
	crossStart, crossEnd Points
	defMain, defCross    Points
	minMain, maxMain     Points
	minCross, maxCross   Points
	align                Align
	stretch              bool
	grow, shrink         float32
	basis, hypo, target  Points
	cross                Points
	frozen               bool
	violation            Points
}

func (it *flexItem) marginMain() Points  { return it.mainStart + it.mainEnd }
func (it *flexItem) marginCross() Points { return it.crossStart + it.crossEnd }

// axes maps main and cross lengths to widths and heights.
type axes struct{ row bool }

func (a axes) pick(w, h Points) (main, cross Points) {
	if a.row {
		return w, h
	}
	return h, w
}

func (a axes) size(main, cross Points) Size {
	if a.row {
		return Size{main, cross}
	}
	return Size{cross, main}
}

func (a axes) point(main, cross Points) Point {
	if a.row {
		return Point{main, cross}
	}
	return Point{cross, main}
}

func (a axes) measure(t *Tree, n *node, main, cross, availMain, availCross Points) Size {
	s := a.size(main, cross)
	av := a.size(availMain, availCross)
	return t.measure(n, s.Width, s.Height, av.Width, av.Height)
}

// flex lays n's children out as a flex container of border-box size w by
// h (Auto: sized by content, within aw by ah) and returns n's size. With
// place it also sets the children's frames and places them.
func (t *Tree) flex(n *node, w, h, aw, ah Points, place bool) Size {
	s := n.style
	dir := s.Direction
	if dir == 0 {
		dir = Column
	}
	ax := axes{dir.row()}
	in := n.insets()
	innerW, innerH := sub(w, in.horizontal()), sub(h, in.vertical())
	availW, availH := innerW, innerH
	if availW.IsAuto() {
		availW = sub(aw, in.horizontal())
	}
	if availH.IsAuto() {
		availH = sub(ah, in.vertical())
	}
	ownMain, crossSize := ax.pick(innerW, innerH)
	availMain, availCross := ax.pick(availW, availH)
	mainSize := ownMain
	scroll := n.kind == KindScroll
	if scroll {
		mainSize, availMain = Auto, Auto
	}
	rowGap, colGap := s.Gap, s.Gap
	if s.RowGap != 0 {
		rowGap = s.RowGap
	}
	if s.ColumnGap != 0 {
		colGap = s.ColumnGap
	}
	gapMain, gapCross := ax.pick(colGap, rowGap)

	kids := n.flatChildren()
	items := make([]flexItem, 0, len(kids))
	var abs []*node
	for _, c := range kids {
		if c.style.Display == DisplayNone {
			if place {
				c.frame = Bounds{}
				c.layoutDirty = false
			}
			continue
		}
		c.setLimits(innerW, innerH)
		if c.style.Position == Absolute {
			abs = append(abs, c)
			continue
		}
		items = append(items, t.newItem(c, ax, s.AlignItems, innerW, innerH, mainSize, crossSize, availMain, availCross))
	}

	// Lines.
	wrap := s.Wrap == Wrap || s.Wrap == WrapReverse
	limit := mainSize
	if limit.IsAuto() {
		limit = availMain
	}
	var lines [][]int
	if !wrap || limit.IsAuto() {
		all := make([]int, len(items))
		for i := range all {
			all[i] = i
		}
		lines = [][]int{all}
	} else {
		var line []int
		var used Points
		for i := range items {
			outer := items[i].hypo + items[i].marginMain()
			if len(line) > 0 && used+gapMain+outer > limit {
				lines = append(lines, line)
				line, used = nil, 0
			}
			if len(line) > 0 {
				used += gapMain
			}
			line = append(line, i)
			used += outer
		}
		lines = append(lines, line)
	}

	// Main sizes.
	for _, line := range lines {
		if mainSize.IsAuto() {
			for _, i := range line {
				items[i].target = items[i].hypo
			}
			continue
		}
		space := mainSize - gapMain*Points(max(len(line)-1, 0))
		for _, i := range line {
			space -= items[i].marginMain()
		}
		resolveFlexible(items, line, space)
	}

	// Cross sizes.
	single := len(lines) == 1
	lineCross := make([]Points, len(lines))
	for l, line := range lines {
		for _, i := range line {
			it := &items[i]
			cross := it.defCross
			if cross.IsAuto() {
				switch {
				case it.stretch && single && !crossSize.IsAuto():
					cross = clamp(max(crossSize-it.marginCross(), 0), it.minCross, it.maxCross)
				case it.n.style.AspectRatio > 0:
					ar := Points(it.n.style.AspectRatio)
					if ax.row {
						cross = it.target / ar
					} else {
						cross = it.target * ar
					}
				default:
					sz := ax.measure(t, it.n, it.target, Auto, availMain, sub(availCross, it.marginCross()))
					_, cross = ax.pick(sz.Width, sz.Height)
				}
			}
			it.cross = cross
			lineCross[l] = max(lineCross[l], cross+it.marginCross())
		}
	}
	if single && !crossSize.IsAuto() {
		lineCross[0] = crossSize
	}

	var contentMain, contentCross Points
	for l, line := range lines {
		var sum Points
		for k, i := range line {
			if k > 0 {
				sum += gapMain
			}
			sum += items[i].target + items[i].marginMain()
		}
		contentMain = max(contentMain, sum)
		if l > 0 {
			contentCross += gapCross
		}
		contentCross += lineCross[l]
	}
	outMain := ownMain
	if outMain.IsAuto() {
		outMain = contentMain
	}
	outCross := crossSize
	if outCross.IsAuto() {
		outCross = contentCross
	}
	insMain, insCross := ax.pick(in.horizontal(), in.vertical())
	size := ax.size(outMain+insMain, outCross+insCross)
	if !place {
		return size
	}

	// Placement.
	extent := outMain
	if scroll {
		extent = max(contentMain, outMain)
		n.scroll.content = ax.size(extent+insMain, outCross+insCross)
	}
	startMain, startCross := ax.pick(in.Left, in.Top)
	free := outCross - contentCross
	crossPos, crossSpacing := startCross, gapCross
	switch s.AlignContent {
	case AlignCenter:
		crossPos += free / 2
	case AlignEnd:
		crossPos += free
	case AlignSpaceBetween:
		if len(lines) > 1 && free > 0 {
			crossSpacing += free / Points(len(lines)-1)
		}
	case AlignSpaceAround:
		if free > 0 {
			crossSpacing += free / Points(len(lines))
			crossPos += free / Points(len(lines)) / 2
		}
	case AlignStretch:
		if !single && free > 0 {
			for l := range lineCross {
				lineCross[l] += free / Points(len(lines))
			}
		}
	}
	order := make([]int, len(lines))
	for l := range order {
		order[l] = l
	}
	if s.Wrap == WrapReverse {
		for a, b := 0, len(order)-1; a < b; a, b = a+1, b-1 {
			order[a], order[b] = order[b], order[a]
		}
	}
	for _, l := range order {
		line := lines[l]
		var sum Points
		for k, i := range line {
			if k > 0 {
				sum += gapMain
			}
			sum += items[i].target + items[i].marginMain()
		}
		freeMain := max(extent-sum, 0)
		pos, spacing := Points(0), gapMain
		count := Points(len(line))
		switch s.JustifyContent {
		case JustifyCenter:
			pos = freeMain / 2
		case JustifyEnd:
			pos = freeMain
		case JustifySpaceBetween:
			if len(line) > 1 {
				spacing += freeMain / (count - 1)
			}
		case JustifySpaceAround:
			spacing += freeMain / count
			pos = freeMain / count / 2
		case JustifySpaceEvenly:
			spacing += freeMain / (count + 1)
			pos = freeMain / (count + 1)
		}
		lc := lineCross[l]
		for _, i := range line {
			it := &items[i]
			cross := it.cross
			if it.stretch {
				cross = clamp(max(lc-it.marginCross(), 0), it.minCross, it.maxCross)
			}
			mainAt := pos + it.mainStart
			if dir.reverse() {
				mainAt = extent - mainAt - it.target
			}
			var crossAt Points
			switch it.align {
			case AlignCenter:
				crossAt = (lc - cross - it.marginCross()) / 2
			case AlignEnd:
				crossAt = lc - cross - it.marginCross()
			}
			crossAt += crossPos + it.crossStart
			origin := ax.point(startMain+mainAt, crossAt)
			t.setFrame(it.n, origin, ax.size(it.target, cross), innerW, innerH)
			pos += it.target + it.marginMain() + spacing
		}
		crossPos += lc + crossSpacing
	}

	// Absolutely positioned children, in the padding box.
	bw := s.BorderWidth
	padW, padH := sub(w, 2*bw), sub(h, 2*bw)
	for _, c := range abs {
		cs := c.style
		l, r := cs.Left.resolve(padW), cs.Right.resolve(padW)
		tp, b := cs.Top.resolve(padH), cs.Bottom.resolve(padH)
		m := cs.Margin
		cw, ch := cs.Width.resolve(padW), cs.Height.resolve(padH)
		if cw.IsAuto() && !l.IsAuto() && !r.IsAuto() {
			cw = max(padW-l-r-m.horizontal(), 0)
		}
		if ch.IsAuto() && !tp.IsAuto() && !b.IsAuto() {
			ch = max(padH-tp-b-m.vertical(), 0)
		}
		sz := t.measure(c, cw, ch, padW, padH)
		x := in.Left + m.Left
		switch {
		case !l.IsAuto():
			x = bw + l + m.Left
		case !r.IsAuto():
			x = bw + padW - r - m.Right - sz.Width
		}
		y := in.Top + m.Top
		switch {
		case !tp.IsAuto():
			y = bw + tp + m.Top
		case !b.IsAuto():
			y = bw + padH - b - m.Bottom - sz.Height
		}
		t.setFrame(c, Point{x, y}, sz, Auto, Auto)
	}
	return size
}

// setFrame sets c's frame, offsetting a relative node by its insets, and
// places its content when its size changed or it changed inside.
func (t *Tree) setFrame(c *node, origin Point, size Size, innerW, innerH Points) {
	if c.style.Position != Absolute {
		cs := c.style
		if l := cs.Left.resolve(innerW); !l.IsAuto() {
			origin.X += l
		} else if r := cs.Right.resolve(innerW); !r.IsAuto() {
			origin.X -= r
		}
		if tp := cs.Top.resolve(innerH); !tp.IsAuto() {
			origin.Y += tp
		} else if b := cs.Bottom.resolve(innerH); !b.IsAuto() {
			origin.Y -= b
		}
	}
	resized := c.frame.Size != size
	c.frame = Bounds{origin, size}
	if resized || c.layoutDirty {
		t.place(c, size)
	}
}

func (t *Tree) newItem(c *node, ax axes, alignItems Align, innerW, innerH, mainSize, crossSize, availMain, availCross Points) flexItem {
	cs := c.style
	it := flexItem{n: c, grow: cs.FlexGrow, shrink: cs.FlexShrink}
	m := cs.Margin
	if ax.row {
		it.mainStart, it.mainEnd, it.crossStart, it.crossEnd = m.Left, m.Right, m.Top, m.Bottom
		it.minMain, it.maxMain, it.minCross, it.maxCross = c.limits[0], c.limits[1], c.limits[2], c.limits[3]
	} else {
		it.mainStart, it.mainEnd, it.crossStart, it.crossEnd = m.Top, m.Bottom, m.Left, m.Right
		it.minMain, it.maxMain, it.minCross, it.maxCross = c.limits[2], c.limits[3], c.limits[0], c.limits[1]
	}
	it.defMain, it.defCross = ax.pick(cs.Width.resolve(innerW), cs.Height.resolve(innerH))
	if !it.defCross.IsAuto() {
		it.defCross = clamp(it.defCross, it.minCross, it.maxCross)
	}
	it.align = cs.AlignSelf
	if it.align == 0 {
		it.align = alignItems
	}
	if it.align == 0 {
		it.align = AlignStretch
	}
	it.stretch = it.align == AlignStretch && it.defCross.IsAuto()
	basis := cs.FlexBasis.resolve(mainSize)
	if basis.IsAuto() {
		basis = it.defMain
	}
	if basis.IsAuto() {
		cross := it.defCross
		if cross.IsAuto() && it.stretch && !crossSize.IsAuto() {
			cross = clamp(max(crossSize-it.marginCross(), 0), it.minCross, it.maxCross)
		}
		sz := ax.measure(t, c, Auto, cross, sub(availMain, it.marginMain()), sub(availCross, it.marginCross()))
		basis, _ = ax.pick(sz.Width, sz.Height)
	}
	it.basis = basis
	it.hypo = clamp(basis, it.minMain, it.maxMain)
	return it
}

// resolveFlexible grows or shrinks a line's items to fill space, freezing
// those min and max sizes stop, as CSS does.
func resolveFlexible(items []flexItem, line []int, space Points) {
	var sumHypo Points
	for _, i := range line {
		sumHypo += items[i].hypo
	}
	growing := sumHypo < space
	for _, i := range line {
		it := &items[i]
		it.target, it.frozen = it.hypo, false
		factor := it.shrink
		if growing {
			factor = it.grow
		}
		if factor == 0 || (growing && it.basis > it.hypo) || (!growing && it.basis < it.hypo) {
			it.frozen = true
		}
	}
	for range len(line) + 1 {
		remaining := space
		var sumGrow, sumShrink float32
		unfrozen := 0
		for _, i := range line {
			it := &items[i]
			if it.frozen {
				remaining -= it.target
				continue
			}
			remaining -= it.basis
			sumGrow += it.grow
			sumShrink += it.shrink * float32(it.basis)
			unfrozen++
		}
		if unfrozen == 0 {
			return
		}
		if growing && sumGrow < 1 {
			remaining *= Points(sumGrow)
		}
		var total Points
		for _, i := range line {
			it := &items[i]
			if it.frozen {
				continue
			}
			target := it.basis
			switch {
			case growing && sumGrow > 0:
				target += remaining * Points(it.grow/sumGrow)
			case !growing && sumShrink > 0:
				target += remaining * Points(it.shrink*float32(it.basis)/sumShrink)
			}
			clamped := clamp(max(target, 0), it.minMain, it.maxMain)
			it.violation = clamped - target
			total += it.violation
			it.target = clamped
		}
		for _, i := range line {
			it := &items[i]
			if !it.frozen && (total == 0 || (total > 0) == (it.violation > 0) && it.violation != 0) {
				it.frozen = true
			}
		}
		if total == 0 {
			return
		}
	}
}
