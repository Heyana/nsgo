package layout

// This file holds what Taffy's layout algorithms share: content alignment,
// scrollable overflow and sizing keywords.

// resolveSelfAlignmentSafety turns a safe alignment that overflows into
// start.
func resolveSelfAlignmentSafety(alignment Align, overflows bool) AlignKeyword {
	if alignment.Safety == Safe && overflows {
		return AlignStart
	}
	return alignment.Keyword
}

// applyAlignmentFallback resolves the keyword content alignment uses, given
// the free space and the number of items aligned. Normal must already be
// resolved.
func applyAlignmentFallback(freeSpace float32, numItems int, mode AlignContent) ContentKeyword {
	keyword := mode.Keyword
	if keyword == ContentNormal {
		keyword = ContentStretch
	}
	isSafe := mode.Safety == Safe

	// 1. With a single item, or items overflowing the container, the
	//    distributed alignments (stretch, space-*) fall back to a positional
	//    keyword and gain implicit safe semantics, so that step 2 can flip
	//    them to start on overflow.
	//    https://www.w3.org/TR/css-align-3/#distribution-values
	if numItems <= 1 || freeSpace <= 0 {
		switch keyword {
		case ContentStretch, ContentSpaceBetween:
			keyword, isSafe = ContentFlexStart, true
		case ContentSpaceAround, ContentSpaceEvenly:
			keyword, isSafe = ContentCenter, true
		}
	}

	// 2. Safe alignment falls back to start whenever the alignment subject
	//    would overflow the alignment container.
	if freeSpace <= 0 && isSafe {
		keyword = ContentStart
	}
	return keyword
}

// computeAlignmentOffset is the offset of an item or line from its
// predecessor (or from the start, for the first) under content alignment.
func computeAlignmentOffset(freeSpace float32, numItems int, gap float32, mode ContentKeyword,
	layoutIsFlexReversed, isFirst bool) float32 {
	if isFirst {
		switch mode {
		case ContentStart:
			return 0
		case ContentFlexStart:
			if layoutIsFlexReversed {
				return freeSpace
			}
			return 0
		case ContentEnd:
			return freeSpace
		case ContentFlexEnd:
			if layoutIsFlexReversed {
				return 0
			}
			return freeSpace
		case ContentCenter:
			return freeSpace / 2
		case ContentSpaceAround:
			if freeSpace >= 0 {
				return (freeSpace / float32(numItems)) / 2
			}
			return freeSpace / 2
		case ContentSpaceEvenly:
			if freeSpace >= 0 {
				return freeSpace / float32(numItems+1)
			}
			return freeSpace / 2
		}
		return 0
	}
	freeSpace = fmax(freeSpace, 0)
	switch mode {
	case ContentSpaceBetween:
		return gap + freeSpace/float32(numItems-1)
	case ContentSpaceAround:
		return gap + freeSpace/float32(numItems)
	case ContentSpaceEvenly:
		return gap + freeSpace/float32(numItems+1)
	}
	return gap
}

// scrollableOverflowContribution is the part of a child's box and overflow
// that its parent can scroll to.
func scrollableOverflowContribution(location Point[float32], size Size[float32], overflowRect Rect[float32],
	overflow Point[Overflow], contain Contain, parentIsScrollContainer bool) Rect[float32] {
	isScrollContainer := overflow.X.isScrollContainer() || overflow.Y.isScrollContainer()
	contained := contain.containsScrollableOverflow()
	propagatesX := !isScrollContainer && !contained && overflow.X == OverflowVisible
	propagatesY := !isScrollContainer && !contained && overflow.Y == OverflowVisible
	endWidth, endHeight := size.Width, size.Height
	if propagatesX {
		endWidth = fmax(size.Width, overflowRect.Right)
	}
	if propagatesY {
		endHeight = fmax(size.Height, overflowRect.Bottom)
	}
	if endWidth <= 0 || endHeight <= 0 {
		return Rect[float32]{}
	}
	var startX, startY float32
	if propagatesX {
		startX = fmin(0, overflowRect.Left)
	}
	if propagatesY {
		startY = fmin(0, overflowRect.Top)
	}
	contribution := Rect[float32]{
		Left:   location.X + startX,
		Right:  location.X + endWidth,
		Top:    location.Y + startY,
		Bottom: location.Y + endHeight,
	}
	if parentIsScrollContainer && (contribution.Right <= 0 || contribution.Bottom <= 0) {
		return Rect[float32]{}
	}
	return contribution
}

// sizingKeywordResolution is how a sizing keyword resolves on one axis:
// exactly to a size, or by measuring the node in some available space.
type sizingKeywordResolution struct {
	measure   bool
	available AvailableSpace // when measure
	exact     float32        // otherwise
}

// resolveSizingKeyword resolves a size that is a sizing keyword, given the
// stretch-fit size and the percentage basis. It reports false for sizes that
// are not keywords, or that cannot resolve.
func resolveSizingKeyword(style Length, stretchSize, percentBasis Opt) (sizingKeywordResolution, bool) {
	measure := func(a AvailableSpace) (sizingKeywordResolution, bool) {
		return sizingKeywordResolution{measure: true, available: a}, true
	}
	switch style.Unit {
	case UnitMinContent:
		return measure(MinContentSpace)
	case UnitMaxContent:
		return measure(MaxContentSpace)
	case UnitFitContentPx:
		return measure(Definite(style.Value))
	case UnitFitContentPercent:
		if percentBasis.Valid {
			return measure(Definite(percentBasis.Value * style.Value))
		}
	case UnitFitContent:
		if stretchSize.Valid {
			return measure(Definite(stretchSize.Value))
		}
	case UnitStretch:
		if stretchSize.Valid {
			return sizingKeywordResolution{exact: stretchSize.Value}, true
		}
	}
	return sizingKeywordResolution{}, false
}

// resolveAbsoluteSizingKeywords resolves the sizing keywords of an
// absolutely positioned box's size into known dimensions.
func resolveAbsoluteSizingKeywords(n Node, known *Size[Opt], sizeStyle Size[Length], area Size[float32],
	inset, margin Rect[Opt], mode sizingMode) {
	stretch := Size[float32]{
		Width: fmax(area.Width-inset.Left.Or(0)-inset.Right.Or(0)-margin.Left.Or(0)-margin.Right.Or(0), 0),
		Height: fmax(area.Height-inset.Top.Or(0)-inset.Bottom.Or(0)-margin.Top.Or(0)-margin.Bottom.Or(0),
			0),
	}

	var kw, kh sizingKeywordResolution
	var okW, okH bool
	if !known.Width.Valid {
		kw, okW = resolveSizingKeyword(sizeStyle.Width, Some(stretch.Width), Some(area.Width))
	}
	if !known.Height.Valid {
		kh, okH = resolveSizingKeyword(sizeStyle.Height, Some(stretch.Height), Some(area.Height))
	}

	// If both axes need measuring, measure them in one call.
	if okW && okH && kw.measure && kh.measure {
		measured := measureChildSizeBoth(n, noneSize, sizeSome(area),
			Size[AvailableSpace]{kw.available, kh.available}, mode, Line[bool]{})
		*known = sizeSome(measured)
		return
	}
	if okW {
		if kw.measure {
			known.Width = Some(measureChildSize(n, *known, sizeSome(area),
				Size[AvailableSpace]{kw.available, Definite(stretch.Height)}, mode, Horizontal, Line[bool]{}))
		} else {
			known.Width = Some(kw.exact)
		}
	}
	if okH {
		if kh.measure {
			width := Definite(stretch.Width)
			if known.Width.Valid {
				width = Definite(known.Width.Value)
			}
			known.Height = Some(measureChildSize(n, *known, sizeSome(area),
				Size[AvailableSpace]{width, kh.available}, mode, Vertical, Line[bool]{}))
		} else {
			known.Height = Some(kh.exact)
		}
	}
}
