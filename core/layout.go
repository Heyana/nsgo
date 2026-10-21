package core

// A small flexbox with React Native's defaults: columns, stretched
// children, no shrinking and no wrapping. It sets every node's frame.

type axis struct{ row bool }

func (a axis) main(s Size) Points {
	if a.row {
		return s.Width
	}
	return s.Height
}

func (a axis) cross(s Size) Points {
	if a.row {
		return s.Height
	}
	return s.Width
}

func (a axis) size(main, cross Points) Size {
	if a.row {
		return Size{main, cross}
	}
	return Size{cross, main}
}

func (a axis) padding(e Edges) (mainStart, mainSum, crossStart, crossSum Points) {
	if a.row {
		return e.Left, e.Left + e.Right, e.Top, e.Top + e.Bottom
	}
	return e.Top, e.Top + e.Bottom, e.Left, e.Left + e.Right
}

func orAuto(forced, styled Points) Points {
	if !forced.IsAuto() {
		return forced
	}
	return styled
}

// layout sizes n and lays out its subtree. availWidth bounds text that
// wraps; forced sizes come from the parent (stretch, grow) and win over
// the style.
func (t *Tree) layout(n *Node, availWidth, forcedW, forcedH Points) Size {
	s := n.style
	w, h := orAuto(forcedW, s.Width), orAuto(forcedH, s.Height)
	padH, padV := s.Padding.Left+s.Padding.Right, s.Padding.Top+s.Padding.Bottom

	switch n.tag {
	case TagText:
		if w.IsAuto() || h.IsAuto() {
			maxW := availWidth
			if !w.IsAuto() {
				maxW = w
			}
			if !maxW.IsAuto() {
				maxW -= padH
			}
			m := t.measure(n.text, s.font(), maxW)
			if w.IsAuto() {
				w = m.Width + padH
			}
			if h.IsAuto() {
				h = m.Height + padV
			}
		}
	case TagInput:
		if h.IsAuto() {
			h = t.measure("Ag", s.font(), Auto).Height + padV
		}
		if w.IsAuto() {
			w = availWidth
			if w.IsAuto() {
				w = 160
			}
		}
	default:
		w, h = t.layoutChildren(n, availWidth, w, h)
	}
	n.frame.Size = Size{w, h}
	return n.frame.Size
}

func (t *Tree) layoutChildren(n *Node, availWidth, w, h Points) (Points, Points) {
	s := n.style
	ax := axis{row: s.Direction == Row}
	mainStart, padMain, crossStart, padCross := ax.padding(s.Padding)

	innerMain, innerCross := ax.main(Size{w, h}), ax.cross(Size{w, h})
	if !innerMain.IsAuto() {
		innerMain -= padMain
	}
	if !innerCross.IsAuto() {
		innerCross -= padCross
	}
	childAvail := availWidth
	if !w.IsAuto() {
		childAvail = w
	}
	if !childAvail.IsAuto() {
		childAvail -= s.Padding.Left + s.Padding.Right
	}

	count := len(n.children)
	sizes := make([]Size, count)
	forcedMain := make([]Points, count)
	stretch := make([]bool, count)
	var sumMain, maxCross Points
	var grow float32
	for i, c := range n.children {
		forcedMain[i] = Auto
		align := c.style.AlignSelf
		if align == AlignAuto {
			align = s.AlignItems
		}
		stretch[i] = (align == AlignAuto || align == AlignStretch) && ax.cross(c.style.sizeStyle()).IsAuto()
		sizes[i] = t.layoutAxis(c, ax, childAvail, Auto, t.crossForce(stretch[i], innerCross))
		sumMain += ax.main(sizes[i])
		grow += c.style.FlexGrow
	}
	if count > 1 {
		sumMain += s.Gap * Points(count-1)
	}

	if !innerMain.IsAuto() && grow > 0 && sumMain < innerMain {
		free := innerMain - sumMain
		for i, c := range n.children {
			if c.style.FlexGrow > 0 {
				forcedMain[i] = ax.main(sizes[i]) + free*Points(c.style.FlexGrow/grow)
				sizes[i] = t.layoutAxis(c, ax, childAvail, forcedMain[i], t.crossForce(stretch[i], innerCross))
			}
		}
		sumMain = innerMain
	}

	for _, sz := range sizes {
		maxCross = max(maxCross, ax.cross(sz))
	}
	if innerCross.IsAuto() {
		innerCross = maxCross
		for i, c := range n.children {
			if stretch[i] && ax.cross(sizes[i]) != innerCross {
				sizes[i] = t.layoutAxis(c, ax, childAvail, forcedMain[i], innerCross)
			}
		}
	}
	if innerMain.IsAuto() {
		innerMain = sumMain
	}

	offset, spacing := mainStart, s.Gap
	free := max(innerMain-sumMain, 0)
	switch s.JustifyContent {
	case JustifyCenter:
		offset += free / 2
	case JustifyEnd:
		offset += free
	case JustifySpaceBetween:
		if count > 1 {
			spacing += free / Points(count-1)
		}
	}
	for i, c := range n.children {
		align := c.style.AlignSelf
		if align == AlignAuto {
			align = s.AlignItems
		}
		crossOffset := crossStart
		switch align {
		case AlignCenter:
			crossOffset += (innerCross - ax.cross(sizes[i])) / 2
		case AlignEnd:
			crossOffset += innerCross - ax.cross(sizes[i])
		}
		if ax.row {
			c.frame.Origin = Point{offset, crossOffset}
		} else {
			c.frame.Origin = Point{crossOffset, offset}
		}
		offset += ax.main(sizes[i]) + spacing
	}

	size := ax.size(innerMain+padMain, innerCross+padCross)
	return size.Width, size.Height
}

func (t *Tree) crossForce(stretch bool, innerCross Points) Points {
	if stretch {
		return innerCross
	}
	return Auto
}

func (t *Tree) layoutAxis(c *Node, ax axis, avail, main, cross Points) Size {
	f := ax.size(main, cross)
	return t.layout(c, avail, f.Width, f.Height)
}

func (s Style) sizeStyle() Size { return Size{s.Width, s.Height} }
