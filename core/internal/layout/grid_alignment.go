package layout

// Aligning grid tracks and positioning grid items.

// gridAlignTracks sets the offsets of the tracks of one axis, aligning them
// in the container (justify-content or align-content). Tracks are stored in
// logical order; on a reversed (RTL) axis, offsets are assigned right to
// left.
func gridAlignTracks(contentBoxSize float32, padding, border Line[float32], tracks []gridTrack,
	style AlignContent, axisIsReversed bool) {
	var used float32
	for i := range tracks {
		used += tracks[i].baseSize
	}
	freeSpace := contentBoxSize - used
	origin := padding.Start + border.Start

	// Count the non-collapsed tracks (gutters aside).
	numTracks := 0
	for i := 1; i < len(tracks); i += 2 {
		if !tracks[i].isCollapsed {
			numTracks++
		}
	}

	// Grid layout treats gaps as full tracks rather than applying them at
	// alignment, so the gap is zero here; grid layout is never flex-reversed.
	const gap = 0
	alignment := applyAlignmentFallback(freeSpace, numTracks, style)
	if axisIsReversed {
		alignment = alignment.reversed()
	}

	// With every track collapsed, no track gets the alignment offset below,
	// but the grid's lines should still be aligned within the container
	// (at the inline-start edge in RTL, say), so the origin gets it instead.
	var emptyGridOffset float32
	if numTracks == 0 {
		emptyGridOffset = computeAlignmentOffset(freeSpace, numTracks, gap, alignment, false, true)
	}

	totalOffset := origin + emptyGridOffset
	seenNonCollapsedTrack := false
	position := func(i int, t *gridTrack) {
		// Odd tracks are gutters, but with zero-based indexes they have even
		// indexes.
		isGutter := i%2 == 0
		isNonCollapsedTrack := !isGutter && !t.isCollapsed

		// Alignment offsets apply only to non-collapsed tracks.
		isFirst := isNonCollapsedTrack && !seenNonCollapsedTrack
		var offset float32
		if isNonCollapsedTrack {
			offset = computeAlignmentOffset(freeSpace, numTracks, gap, alignment, false, isFirst)
		}

		t.offset = totalOffset + offset
		totalOffset = totalOffset + offset + t.baseSize
		if isNonCollapsedTrack {
			seenNonCollapsedTrack = true
		}
	}
	if axisIsReversed {
		for i := range tracks {
			position(i, &tracks[len(tracks)-1-i])
		}
	} else {
		for i := range tracks {
			position(i, &tracks[i])
		}
	}
}

// gridAlignAndPositionItem sizes, lays out and positions a grid item in its
// grid area. It returns the item's scrollable overflow contribution, its y
// position and its height.
func gridAlignAndPositionItem(node Node, order uint32, gridArea Rect[float32], containingBlockSize Size[float32],
	containerAlignItems Align, containerJustifyItems Align, baselineShim float32, direction Direction,
	containerBorderBoxWidth float32, containerBorder Rect[float32], containerIsScrollContainer bool,
	bubbled *[]oofCandidate) (Rect[float32], float32, float32) {
	style := node.Style()
	overflow := style.Overflow
	contain := style.Contain
	scrollbarWidth := style.ScrollbarWidth
	aspectRatio := style.AspectRatio
	isReplaced := style.isReplaced()
	// Fall back to the container's justify-items and align-items, then
	// resolve self-start and self-end against the item's own direction. The
	// horizontal axis is the inline axis (Taffy supports only horizontal-tb);
	// on the block axis they resolve to plain start and end.
	itemDirection := style.Direction
	justifySelf := style.justifySelfOr(containerJustifyItems).resolveSelfRelative(itemDirection, direction, true)
	alignSelf := style.alignSelfOr(containerAlignItems).resolveSelfRelative(itemDirection, direction, false)

	position := style.Position
	insetH := Line[Opt]{
		style.Inset.Left.resolveToOption(containingBlockSize.Width),
		style.Inset.Right.resolveToOption(containingBlockSize.Width),
	}
	insetV := Line[Opt]{
		style.Inset.Top.resolveToOption(containingBlockSize.Height),
		style.Inset.Bottom.resolveToOption(containingBlockSize.Height),
	}
	padding := rectResolveOrZero(style.padding(), Some(containingBlockSize.Width))
	border := rectResolveOrZero(style.border(), Some(containingBlockSize.Width))
	paddingBorderSize := rectSumAxes(rectAdd(padding, border))
	var boxSizingAdjustment Size[float32]
	if style.BoxSizing == ContentBox {
		boxSizingAdjustment = paddingBorderSize
	}

	cbOpts := sizeSome(containingBlockSize)
	sizeStyle := style.Size
	styleSize := optsAddF(optsAspectRatio(sizeMaybeResolve(sizeStyle, cbOpts), aspectRatio), boxSizingAdjustment)
	minSize := optsAspectRatio(optsMaxF(optsOr(optsAddF(sizeMaybeResolve(style.MinSize, cbOpts), boxSizingAdjustment),
		sizeSome(paddingBorderSize)), paddingBorderSize), aspectRatio)
	maxSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MaxSize, cbOpts), aspectRatio), boxSizingAdjustment)

	// Resolve normal alignment (the default when alignment is set on neither
	// the parent nor the node). With a preferred aspect ratio but no width or
	// height, the width is stretched and the height follows from the ratio.
	// Replaced elements are not stretched: they are sized as for start.
	// https://www.w3.org/TR/css-grid-1/#grid-item-sizing
	alignH := justifySelf
	if justifySelf.Keyword == AlignNormal {
		if styleSize.Width.Valid || sizeStyle.Width.isSizingKeyword() || isReplaced {
			alignH = alignOf(AlignStart)
		} else {
			alignH = alignOf(AlignStretch)
		}
	}
	alignV := alignSelf
	if alignSelf.Keyword == AlignNormal {
		if styleSize.Height.Valid || sizeStyle.Height.isSizingKeyword() || aspectRatio.Valid || isReplaced {
			alignV = alignOf(AlignStart)
		} else {
			alignV = alignOf(AlignStretch)
		}
	}

	// Both horizontal and vertical margins resolve against the width of the
	// grid area, as CSS specifies.
	sm := style.margin()
	margin := Rect[Opt]{
		Left:   sm.Left.resolveToOption(containingBlockSize.Width),
		Right:  sm.Right.resolveToOption(containingBlockSize.Width),
		Top:    sm.Top.resolveToOption(containingBlockSize.Width),
		Bottom: sm.Bottom.resolveToOption(containingBlockSize.Width),
	}

	areaMinusMargins := Size[float32]{
		Width:  fmax(fsubOpt(fsubOpt(gridArea.Right-gridArea.Left, margin.Left), margin.Right), 0),
		Height: fmax(fsubOpt(fsubOpt(gridArea.Bottom-gridArea.Top, margin.Top), margin.Bottom)-baselineShim, 0),
	}

	// A size that is a sizing keyword either resolves to an exact size or
	// is resolved by measuring the item under the keyword's constraint.
	var kw, kh sizingKeywordResolution
	var okW, okH bool
	if !styleSize.Width.Valid {
		kw, okW = resolveSizingKeyword(sizeStyle.Width, Some(areaMinusMargins.Width), Some(containingBlockSize.Width))
	}
	if !styleSize.Height.Valid {
		kh, okH = resolveSizingKeyword(sizeStyle.Height, Some(areaMinusMargins.Height),
			Some(containingBlockSize.Height))
	}

	// If both axes need measuring, measure them in one call.
	var keywordMeasured Size[Opt]
	if okW && okH && kw.measure && kh.measure && !position.isOutOfFlow() {
		keywordMeasured = sizeSome(measureChildSizeBoth(node, noneSize, cbOpts,
			Size[AvailableSpace]{kw.available, kh.available}, inherentSize, Line[bool]{}))
	}

	width := styleSize.Width
	if !width.Valid {
		switch {
		case position.isOutOfFlow() && insetH.Start.Valid && insetH.End.Valid:
			// An absolutely positioned item with both left and right set and no
			// width gets its width from them.
			width = Some(fmax(areaMinusMargins.Width-insetH.Start.Value-insetH.End.Value, 0))
		case okW && !kw.measure:
			width = Some(kw.exact)
		case okW:
			if keywordMeasured.Width.Valid {
				width = keywordMeasured.Width
			} else {
				width = Some(measureChildSize(node, noneSize, cbOpts,
					Size[AvailableSpace]{kw.available, Definite(areaMinusMargins.Height)}, inherentSize, Horizontal,
					Line[bool]{}))
			}
		case margin.Left.Valid && margin.Right.Valid && alignH == alignOf(AlignStretch) && !position.isOutOfFlow():
			// Stretch alignment of an in-flow item without auto margins on
			// this axis.
			width = Some(areaMinusMargins.Width)
		}
	}

	// Reapply the aspect ratio after stretching.
	s := optsAspectRatio(Size[Opt]{width, styleSize.Height}, aspectRatio)

	if !s.Height.Valid {
		switch {
		case position.isOutOfFlow() && insetV.Start.Valid && insetV.End.Valid:
			s.Height = Some(fmax(areaMinusMargins.Height-insetV.Start.Value-insetV.End.Value, 0))
		case okH && !kh.measure:
			s.Height = Some(kh.exact)
		case okH:
			if keywordMeasured.Height.Valid {
				s.Height = keywordMeasured.Height
			} else {
				availWidth := Definite(areaMinusMargins.Width)
				if s.Width.Valid {
					availWidth = Definite(s.Width.Value)
				}
				s.Height = Some(measureChildSize(node, Size[Opt]{Width: s.Width}, cbOpts,
					Size[AvailableSpace]{availWidth, kh.available}, inherentSize, Vertical, Line[bool]{}))
			}
		case margin.Top.Valid && margin.Bottom.Valid && alignV == alignOf(AlignStretch) && !position.isOutOfFlow():
			s.Height = Some(areaMinusMargins.Height)
		}
	}
	// Reapply the aspect ratio after stretching.
	s = optsAspectRatio(s, aspectRatio)

	// Clamp by the min and max sizes.
	s = optsClamp(s, minSize, maxSize)

	// Lay the node out.
	size := s
	if position.isOutOfFlow() && (!s.Width.Valid || !s.Height.Valid) {
		size = sizeSome(measureChildSizeBoth(node, s, cbOpts, spacesDefinite(areaMinusMargins), inherentSize,
			Line[bool]{}))
	}

	out := performChildLayout(node, size, cbOpts, spacesDefinite(areaMinusMargins), inherentSize, Line[bool]{})

	// The final size.
	final := sizeClamp(optsUnwrapOr(size, out.size), minSize, maxSize)

	x, xMargin := gridAlignItemWithinArea(Line[float32]{gridArea.Left, gridArea.Right}, alignH, final.Width,
		position, insetH, Line[Opt]{margin.Left, margin.Right}, 0, direction)
	y, yMargin := gridAlignItemWithinArea(Line[float32]{gridArea.Top, gridArea.Bottom}, alignV, final.Height,
		position, insetV, Line[Opt]{margin.Top, margin.Bottom}, baselineShim, LTR)

	var scrollbar Size[float32]
	if overflow.Y == OverflowScroll {
		scrollbar.Width = scrollbarWidth
	}
	if overflow.X == OverflowScroll {
		scrollbar.Height = scrollbarWidth
	}

	// Bubble the out-of-flow candidates of the item's subtree, relative to
	// the container.
	if len(out.oofCandidates) > 0 {
		translateCandidates(out.oofCandidates, Point[float32]{x, y})
		*bubbled = append(*bubbled, out.oofCandidates...)
	}

	node.LayoutState().Unrounded = Layout{
		Order:              order,
		Location:           Point[float32]{x, y},
		Size:               final,
		ScrollableOverflow: out.scrollableOverflow,
		ScrollbarSize:      scrollbar,
		Padding:            padding,
		Border:             border,
		Margin:             Rect[float32]{Left: xMargin.Start, Right: xMargin.End, Top: yMargin.Start, Bottom: yMargin.End},
	}

	// Contributions to the container's scrollable overflow are measured from
	// its padding box origin (mirrored in RTL), as the scrollable overflow
	// region is.
	location := Point[float32]{x - containerBorder.Left, y - containerBorder.Top}
	if direction.isRTL() {
		location.X = containerBorderBoxWidth - (x + final.Width) - containerBorder.Right
	}
	contribution := scrollableOverflowContribution(location, final, out.scrollableOverflow, overflow, contain,
		containerIsScrollContainer)
	return contribution, y, final.Height
}

// gridAlignItemWithinArea positions an item of a resolved size within its
// grid area on one axis, returning its start and resolved margins.
func gridAlignItemWithinArea(area Line[float32], alignment Align, size float32, position Position,
	inset, margin Line[Opt], baselineShim float32, direction Direction) (float32, Line[float32]) {
	// The grid area's size on the axis.
	nonAutoMargin := Line[float32]{margin.Start.Or(0) + baselineShim, margin.End.Or(0)}
	areaSize := fmax(area.End-area.Start, 0)
	freeSpace := fmax(areaSize-size-lineSum(nonAutoMargin), 0)

	// Auto margins expand to fill the free space.
	autoMargins := 0
	if !margin.Start.Valid {
		autoMargins++
	}
	if !margin.End.Valid {
		autoMargins++
	}
	var autoMarginSize float32
	if autoMargins > 0 {
		autoMarginSize = freeSpace / float32(autoMargins)
	}
	resolved := Line[float32]{margin.Start.Or(autoMarginSize) + baselineShim, margin.End.Or(autoMarginSize)}

	overflows := size+lineSum(nonAutoMargin) > areaSize
	keyword := resolveSelfAlignmentSafety(alignment, overflows)

	// The offset on the axis.
	var alignmentOffset float32
	switch keyword {
	case AlignEnd, AlignFlexEnd:
		if direction.isRTL() {
			alignmentOffset = resolved.Start
		} else {
			alignmentOffset = areaSize - size - resolved.End
		}
	case AlignCenter:
		alignmentOffset = (areaSize - size + resolved.Start - resolved.End) / 2
	default:
		// Start, flex-start, stretch and (for now, unsupported) baseline.
		// Normal is resolved, and self-start and self-end are resolved to
		// start and end, by gridAlignAndPositionItem.
		if direction.isRTL() {
			alignmentOffset = areaSize - size - resolved.End
		} else {
			alignmentOffset = resolved.Start
		}
	}

	offset := alignmentOffset
	if position.isOutOfFlow() {
		switch {
		case inset.Start.Valid && inset.End.Valid:
			if direction.isRTL() {
				offset = areaSize - inset.End.Value - size - nonAutoMargin.End
			} else {
				offset = inset.Start.Value + nonAutoMargin.Start
			}
		case inset.Start.Valid:
			offset = inset.Start.Value + nonAutoMargin.Start
		case inset.End.Valid:
			offset = areaSize - inset.End.Value - size - nonAutoMargin.End
		}
	}

	start := area.Start + offset
	if position == PositionRelative {
		negEnd := Opt{-inset.End.Value, inset.End.Valid}
		var relative Opt
		if direction.isRTL() {
			relative = negEnd.OrOpt(inset.Start)
		} else {
			relative = inset.Start.OrOpt(negEnd)
		}
		start += relative.Or(0)
	}
	return start, resolved
}
