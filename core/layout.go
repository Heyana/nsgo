package core

import (
	"math"

	"github.com/oneo-me/nsgo/core/internal/layout"
)

// Layout runs NSGo's port of Taffy (core/internal/layout): CSS flexbox,
// grid and block layout. Each shadow node with a native view is a layout
// node whose children are its native children, regions flattened and
// overlays and modals left out; those are layout roots of their own, sized
// against their host. Styles map onto CSS with React Native's defaults, as
// NSH maps its styles onto Taffy's.

// ceil rounds text sizes up so that rendering never truncates a glyph.
func ceil(p Points) Points { return Points(math.Ceil(float64(p))) }

// borders returns the border widths layout uses.
func (s *Style) borders() Edges {
	if s.Borders != (Edges{}) {
		return s.Borders
	}
	return All(s.BorderWidth)
}

// The layout.Node methods.

func (n *node) ChildCount() int { return len(n.flatChildren()) }

func (n *node) Child(i int) layout.Node { return n.flatChildren()[i] }

func (n *node) LayoutState() *layout.State { return &n.lstate }

func (n *node) Style() *layout.Style {
	if !n.lstyleOK {
		n.lstyle = n.layoutStyle()
		n.lstyleOK = true
	}
	return &n.lstyle
}

// Measure measures a leaf's content: its text, a field's line, an image's
// declared size. Views and canvases have none.
func (n *node) Measure(known layout.Size[layout.Opt], available layout.Size[layout.AvailableSpace]) layout.Size[float32] {
	var m Size
	switch {
	case n.text != nil:
		// available is the content box: a known width is the border box,
		// so measuring at it would ignore the text's own padding.
		width := Auto
		if available.Width.Kind == layout.SpaceDefinite {
			width = Points(available.Width.Value)
		} else if known.Width.Valid {
			width = Points(known.Width.Value)
		}
		if !width.IsAuto() {
			width = max(width, 0)
		}
		if n.text.prepared != nil {
			m = n.text.prepared.Measure(width)
		}
		m = Size{ceil(m.Width), ceil(m.Height)}
	case n.input != nil:
		m = Size{160, ceil(n.input.lineH)}
	case n.image != nil:
		m = fitNatural(n.image.naturalSize(), known)
	case n.icon != nil:
		m = fitNatural(n.icon.naturalSize(), known)
	default:
		return layout.Size[float32]{}
	}
	return layout.Size[float32]{
		Width:  known.Width.Or(float32(m.Width)),
		Height: known.Height.Or(float32(m.Height)),
	}
}

// fitNatural fits a leaf's natural size to the sizes layout already knows.
func fitNatural(nat Size, known layout.Size[layout.Opt]) Size {
	w, h := known.Width, known.Height
	switch {
	case !w.Valid && !h.Valid:
		return nat
	case !w.Valid && nat.Height > 0:
		return Size{Width: Points(h.Value) * nat.Width / nat.Height}
	case !h.Valid && nat.Width > 0:
		return Size{Height: Points(w.Value) * nat.Height / nat.Width}
	}
	return Size{}
}

// markLayout marks n and its ancestors for layout, dropping their cached
// results. Regions are never laid out themselves, so they pass the mark
// on.
func (n *node) markLayout() {
	for p := n; p != nil; p = p.parent {
		p.layoutDirty = true
		p.lstate.Invalidate()
	}
}

// layoutStyle maps n's style onto a CSS style with React Native's
// defaults, as NSH does.
func (n *node) layoutStyle() layout.Style {
	s := &n.style
	var l layout.Style

	switch s.Display {
	case DisplayNone:
		l.Display = layout.DisplayNone
	case DisplayGrid:
		l.Display = layout.DisplayGrid
	case DisplayBlock:
		l.Display = layout.DisplayBlock
	}
	l.Position = layout.PositionRelative
	if s.Position == Absolute {
		l.Position = layout.PositionAbsolute
	}
	switch s.Direction {
	case Row:
		l.FlexDirection = layout.Row
	case RowReverse:
		l.FlexDirection = layout.RowReverse
	case ColumnReverse:
		l.FlexDirection = layout.ColumnReverse
	default:
		l.FlexDirection = layout.Column
	}
	switch s.Wrap {
	case Wrap:
		l.FlexWrap = layout.Wrap
	case WrapReverse:
		l.FlexWrap = layout.WrapReverse
	}
	l.FlexGrow = s.FlexGrow
	l.FlexShrink = layout.Some(s.FlexShrink)
	l.FlexBasis = s.FlexBasis.layout()

	l.Size = layout.Size[layout.Length]{Width: s.Width.layout(), Height: s.Height.layout()}
	// Yoga has no automatic minimum size: an unset minimum is 0.
	l.MinSize = layout.Size[layout.Length]{Width: s.MinWidth.minLayout(), Height: s.MinHeight.minLayout()}
	l.MaxSize = layout.Size[layout.Length]{Width: s.MaxWidth.layout(), Height: s.MaxHeight.layout()}
	if s.AspectRatio > 0 {
		l.AspectRatio = layout.Some(s.AspectRatio)
	}
	if s.BoxSizing == ContentBox {
		l.BoxSizing = layout.ContentBox
	}

	l.Inset = layout.Rect[layout.Length]{Left: s.Left.layout(), Right: s.Right.layout(),
		Top: s.Top.layout(), Bottom: s.Bottom.layout()}
	l.Margin = s.Margin.layout()
	for _, side := range [...]struct {
		bit Sides
		dst *layout.Length
	}{{SideTop, &l.Margin.Top}, {SideRight, &l.Margin.Right}, {SideBottom, &l.Margin.Bottom}, {SideLeft, &l.Margin.Left}} {
		if s.AutoMargin&side.bit != 0 {
			*side.dst = layout.Auto
		}
	}
	l.Padding = s.Padding.layout()
	l.Border = s.borders().layout()
	rowGap, colGap := s.Gap, s.Gap
	if s.RowGap != 0 {
		rowGap = s.RowGap
	}
	if s.ColumnGap != 0 {
		colGap = s.ColumnGap
	}
	l.Gap = layout.Size[layout.Length]{Width: layout.Px(float32(colGap)), Height: layout.Px(float32(rowGap))}

	l.AlignItems = s.AlignItems.items()
	l.AlignSelf = s.AlignSelf.items()
	l.JustifyItems = s.JustifyItems.items()
	l.JustifySelf = s.JustifySelf.items()
	l.AlignContent = s.AlignContent.content()
	l.JustifyContent = s.JustifyContent.layout()
	// An unset align-content is React Native's flex-start outside grids,
	// where CSS's normal would stretch wrapped lines; grids keep normal, so
	// their auto tracks stretch.
	if l.Display != layout.DisplayGrid && l.AlignContent.Keyword == layout.ContentNormal {
		l.AlignContent.Keyword = layout.ContentFlexStart
	}

	// Hidden clips in layout as well as in paint: what a box clips away
	// must not reach an enclosing scroll view's content. Clip, not Hidden,
	// which would also make the box a scroll container with no automatic
	// minimum size.
	x, y := s.overflow()
	l.Overflow = layout.Point[layout.Overflow]{X: x.layout(), Y: y.layout()}

	if t := s.GridTemplateColumns; t != "" {
		if g, err := parseTemplate(t); err == nil {
			l.GridTemplateColumns, l.GridTemplateColumnNames = g.tracks, g.names
		}
	}
	if t := s.GridTemplateRows; t != "" {
		if g, err := parseTemplate(t); err == nil {
			l.GridTemplateRows, l.GridTemplateRowNames = g.tracks, g.names
		}
	}
	if t := s.GridAutoColumns; t != "" {
		if g, err := parseTemplate(t); err == nil {
			l.GridAutoColumns = g.auto
		}
	}
	if t := s.GridAutoRows; t != "" {
		if g, err := parseTemplate(t); err == nil {
			l.GridAutoRows = g.auto
		}
	}
	switch s.GridAutoFlow {
	case FlowColumn:
		l.GridAutoFlow = layout.FlowColumn
	case FlowRowDense:
		l.GridAutoFlow = layout.FlowRowDense
	case FlowColumnDense:
		l.GridAutoFlow = layout.FlowColumnDense
	}
	l.GridRow = layout.Line[layout.GridPlacement]{Start: s.GridRow.Start.layout(), End: s.GridRow.End.layout()}
	l.GridColumn = layout.Line[layout.GridPlacement]{Start: s.GridColumn.Start.layout(), End: s.GridColumn.End.layout()}

	// A scroll view's content may overflow along its axis; a horizontal one
	// lays its content out in a row.
	horizontalScroll := n.scroll != nil && n.scroll.config.Horizontal
	if n.scroll != nil {
		if horizontalScroll {
			l.FlexDirection = layout.Row
			l.Overflow.X = layout.OverflowScroll
		} else {
			l.Overflow.Y = layout.OverflowScroll
		}
	}

	// Right to left, left and right swap, as in React Native. A horizontal
	// scroll view lays its content out left to right with its own sides as
	// written, and its children's frames are mirrored.
	if n.rtl {
		l.Margin.Left, l.Margin.Right = l.Margin.Right, l.Margin.Left
		l.Inset.Left, l.Inset.Right = l.Inset.Right, l.Inset.Left
		if !horizontalScroll {
			l.Direction = layout.RTL
			l.Padding.Left, l.Padding.Right = l.Padding.Right, l.Padding.Left
			l.Border.Left, l.Border.Right = l.Border.Right, l.Border.Left
		}
	}

	// Roots are placed by core: the root view and modals fill their host,
	// overlays size to their content.
	if n.isRoot {
		l.Position = layout.PositionRelative
		l.Inset = layout.Rect[layout.Length]{}
		if n.fill != nil {
			l.Size = layout.Size[layout.Length]{Width: layout.Px(float32(n.fill.Width)), Height: layout.Px(float32(n.fill.Height))}
			l.MinSize, l.MaxSize = layout.Size[layout.Length]{}, layout.Size[layout.Length]{}
		}
	}
	return l
}

func (l Length) layout() layout.Length {
	switch l.unit {
	case unitPoints:
		return layout.Px(l.value)
	case unitPercent:
		return layout.Percent(l.value / 100)
	}
	return layout.Auto
}

// minLayout maps a minimum size: auto is 0, as in Yoga.
func (l Length) minLayout() layout.Length {
	if l.IsAuto() {
		return layout.Px(0)
	}
	return l.layout()
}

func (e Edges) layout() layout.Rect[layout.Length] {
	return layout.Rect[layout.Length]{Left: layout.Px(float32(e.Left)), Right: layout.Px(float32(e.Right)),
		Top: layout.Px(float32(e.Top)), Bottom: layout.Px(float32(e.Bottom))}
}

func (o Overflow) layout() layout.Overflow {
	switch o {
	case Hidden:
		return layout.OverflowClip
	case OverflowScroll:
		return layout.OverflowScroll
	}
	return layout.OverflowVisible
}

// items maps an align-items, align-self, justify-items or justify-self
// value; start and end are flex-start and flex-end.
func (a Align) items() layout.Align {
	var k layout.AlignKeyword
	switch a {
	case AlignStretch:
		k = layout.AlignStretch
	case AlignStart:
		k = layout.AlignFlexStart
	case AlignCenter:
		k = layout.AlignCenter
	case AlignEnd:
		k = layout.AlignFlexEnd
	case AlignBaseline:
		k = layout.AlignBaseline
	}
	return layout.Align{Keyword: k}
}

// content maps an align-content value.
func (a Align) content() layout.AlignContent {
	k := layout.ContentNormal
	switch a {
	case AlignStretch:
		k = layout.ContentStretch
	case AlignStart:
		k = layout.ContentFlexStart
	case AlignCenter:
		k = layout.ContentCenter
	case AlignEnd:
		k = layout.ContentFlexEnd
	case AlignSpaceBetween:
		k = layout.ContentSpaceBetween
	case AlignSpaceAround:
		k = layout.ContentSpaceAround
	case AlignSpaceEvenly:
		k = layout.ContentSpaceEvenly
	}
	return layout.AlignContent{Keyword: k}
}

func (j Justify) layout() layout.AlignContent {
	k := layout.ContentNormal
	switch j {
	case JustifyStart:
		k = layout.ContentFlexStart
	case JustifyCenter:
		k = layout.ContentCenter
	case JustifyEnd:
		k = layout.ContentFlexEnd
	case JustifySpaceBetween:
		k = layout.ContentSpaceBetween
	case JustifySpaceAround:
		k = layout.ContentSpaceAround
	case JustifySpaceEvenly:
		k = layout.ContentSpaceEvenly
	case JustifyStretch:
		k = layout.ContentStretch
	}
	return layout.AlignContent{Keyword: k}
}

// layout lays the root view out at the surface size, then the overlays
// and modals it hosts.
func (t *Tree) layout() {
	if !t.view.layoutDirty {
		return
	}
	t.layoutRoot(t.view, t.size, true)
	t.view.frame = Bounds{Size: t.size}
	t.syncFrames(t.view)
	t.placeHoisted(t.view)
}

// layoutRoot lays root out in space, which it fills when fill is set.
func (t *Tree) layoutRoot(root *node, space Size, fill bool) {
	root.isRoot = true
	var want *Size
	if fill {
		want = &space
	}
	if (want == nil) != (root.fill == nil) || (want != nil && *want != *root.fill) {
		root.fill = want
		root.lstyleOK = false
		root.lstate.Invalidate()
	}
	layout.Compute(root, layout.Size[layout.AvailableSpace]{
		Width:  layout.Definite(float32(space.Width)),
		Height: layout.Definite(float32(space.Height)),
	})
	layout.Round(root)
}

// syncFrames sets the frames of n's children from their layouts, and
// what depends on them, through n's subtree. n's own frame is set.
func (t *Tree) syncFrames(n *node) {
	t.syncContent(n)
	n.layoutDirty = false
	mirror := n.rtl && n.scroll != nil && n.scroll.config.Horizontal
	for _, c := range n.flatChildren() {
		l := &c.lstate.Rounded
		f := Bounds{
			Origin: Point{Points(l.Location.X), Points(l.Location.Y)},
			Size:   Size{Points(l.Size.Width), Points(l.Size.Height)},
		}
		if mirror {
			f.Origin.X = n.scroll.content.Width - f.Origin.X - f.Size.Width
		}
		c.frame = f
		t.syncFrames(c)
	}
}

// syncContent sets what n's layout decides inside it: where its text goes
// and how far its content scrolls.
func (t *Tree) syncContent(n *node) {
	l := &n.lstate.Rounded
	switch {
	case n.text != nil:
		n.text.box = contentBox(l)
	case n.input != nil:
		n.input.config.Content = contentBox(l)
	case n.scroll != nil:
		n.scroll.content = Size{Points(l.Size.Width + l.ScrollWidth()), Points(l.Size.Height + l.ScrollHeight())}
	}
}

// contentBox is the box inside a layout's padding and border.
func contentBox(l *layout.Layout) Bounds {
	return Bounds{
		Origin: Point{Points(l.Border.Left + l.Padding.Left), Points(l.Border.Top + l.Padding.Top)},
		Size: Size{
			Width:  Points(max(l.Size.Width-l.Border.Left-l.Border.Right-l.Padding.Left-l.Padding.Right, 0)),
			Height: Points(max(l.Size.Height-l.Border.Top-l.Border.Bottom-l.Padding.Top-l.Padding.Bottom, 0)),
		},
	}
}

// setRTL lays every node out again in the new direction.
func (t *Tree) setRTL(rtl bool) {
	t.rtl = rtl
	for _, n := range t.nodes {
		if n != nil {
			n.rtl = rtl
			n.lstyleOK = false
			n.lstate.Invalidate()
		}
	}
	t.view.markLayout()
}
