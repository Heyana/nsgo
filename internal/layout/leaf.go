package layout

// computeLeafLayout sizes a node without children around its measured
// content: Taffy's compute_leaf_layout.
func computeLeafLayout(in layoutInput, style *Style, n Node) layoutOutput {
	known, parentSize := in.knownDimensions, in.parentSize

	// Both horizontal and vertical percentage padding and borders resolve
	// against the containing block's inline size (width), as CSS specifies.
	padding := rectResolveOrZero(style.padding(), parentSize.Width)
	border := rectResolveOrZero(style.border(), parentSize.Width)
	paddingBorder := rectAdd(padding, border)
	pbSum := rectSumAxes(paddingBorder)
	var boxSizingAdjustment Size[float32]
	if style.BoxSizing == ContentBox {
		boxSizingAdjustment = pbSum
	}

	// Resolve the node's preferred, min and max sizes against the available
	// space. In content-size mode the node's size styles are ignored.
	var nodeSize, nodeMinSize, nodeMaxSize Size[Opt]
	var aspectRatio Opt
	if in.sizingMode == contentSize {
		nodeSize = known
	} else {
		aspectRatio = style.AspectRatio
		styleSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.Size, parentSize), aspectRatio), boxSizingAdjustment)
		nodeMinSize = optsAddF(optsAspectRatio(sizeMaybeResolve(style.MinSize, parentSize), aspectRatio), boxSizingAdjustment)
		nodeMaxSize = optsAddF(sizeMaybeResolve(style.MaxSize, parentSize), boxSizingAdjustment)
		nodeSize = optsOr(known, styleSize)
	}

	// Scrollbar gutters are reserved on the axis opposite the scrolling one.
	gutter := style.scrollbarGutter()
	contentBoxInset := paddingBorder
	contentBoxInset.Right += gutter.X
	contentBoxInset.Bottom += gutter.Y

	preventsCollapseThrough := !style.isBlock() ||
		style.Overflow.X.isScrollContainer() ||
		style.Overflow.Y.isScrollContainer() ||
		style.Position.isOutOfFlow() ||
		style.Contain.establishesIndependentFormattingContext() ||
		padding.Top > 0 || padding.Bottom > 0 || border.Top > 0 || border.Bottom > 0 ||
		(nodeSize.Height.Valid && nodeSize.Height.Value > 0) ||
		(nodeMinSize.Height.Valid && nodeMinSize.Height.Value > 0)

	// Return early if both width and height are known.
	if in.runMode == computeSize && preventsCollapseThrough && optsBoth(nodeSize) {
		size := sizeMaxOpt(sizeClamp(optsUnwrapOr(nodeSize, Size[float32]{}), nodeMinSize, nodeMaxSize), sizeSome(pbSum))
		return layoutOutput{size: size}
	}

	// Compute the available space.
	available := Size[AvailableSpace]{
		Width: leafAvailable(in.availableSpace.Width, known.Width, nodeSize.Width, nodeMinSize.Width,
			nodeMaxSize.Width, rectHSum(contentBoxInset)),
		Height: leafAvailable(in.availableSpace.Height, known.Height, nodeSize.Height, nodeMinSize.Height,
			nodeMaxSize.Height, rectVSum(contentBoxInset)),
	}

	// Measure the node.
	measureKnown := known
	if in.runMode == performLayout {
		measureKnown = noneSize
	}
	measured := n.Measure(measureKnown, available)
	clamped := sizeClamp(optsUnwrapOr(optsOr(known, nodeSize), sizeAdd(measured, rectSumAxes(contentBoxInset))),
		nodeMinSize, nodeMaxSize)
	size := Size[float32]{
		Width:  clamped.Width,
		Height: fmax(clamped.Height, aspectHeight(clamped.Width, aspectRatio)),
	}
	size = sizeMaxOpt(size, sizeSome(pbSum))

	// A scroll container's own end padding is part of its scrollable
	// overflow; other boxes don't extend their overflow by their padding.
	isScrollContainer := style.Overflow.X.isScrollContainer() || style.Overflow.Y.isScrollContainer()
	startPadding, endPadding := padding.Left, padding.Right
	if style.Direction.isRTL() {
		startPadding, endPadding = padding.Right, padding.Left
	}
	overflow := Rect[float32]{
		Right:  startPadding + measured.Width,
		Bottom: padding.Top + measured.Height,
	}
	if isScrollContainer {
		overflow.Right += endPadding
		overflow.Bottom += padding.Bottom
	}

	return layoutOutput{
		size:                      size,
		scrollableOverflow:        overflow,
		marginsCanCollapseThrough: !preventsCollapseThrough && size.Height == 0 && measured.Height == 0,
	}
}

// leafAvailable is the space a leaf's content may use on one axis.
func leafAvailable(available AvailableSpace, known, nodeSize, min, max Opt, inset float32) AvailableSpace {
	if known.Valid {
		available = Definite(known.Value)
	}
	available = available.maybeSet(known).maybeSet(nodeSize)
	if available.isDefinite() {
		available.Value = fclamp(available.Value, min, max) - inset
	}
	return available
}

// aspectHeight is the height an aspect ratio gives a width, or 0 without a
// ratio.
func aspectHeight(width float32, ratio Opt) float32 {
	if ratio.Valid {
		return width / ratio.Value
	}
	return 0
}
