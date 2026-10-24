package layout

// gridItem is an in-flow child of a grid container during layout.
type gridItem struct {
	node Node
	// sourceOrder is the child's index in its parent.
	sourceOrder uint16

	// row and column are the item's placement.
	row, column Line[ozLine]

	isCompressibleReplaced bool
	isReplaced             bool
	overflow               Point[Overflow]
	boxSizing              BoxSizing
	size, minSize, maxSize Size[Length]
	aspectRatio            Opt
	padding, border        Rect[Length]
	margin                 Rect[Length]
	alignSelf, justifySelf Align
	// baseline is the item's first baseline, once computed, and
	// baselineShim the space added before it to align it with its row.
	baseline     Opt
	baselineShim float32

	// rowIndexes and columnIndexes are the item's placement as track vector
	// indexes.
	rowIndexes, columnIndexes Line[uint16]

	crossesFlexibleRow, crossesFlexibleColumn   bool
	crossesIntrinsicRow, crossesIntrinsicColumn bool

	// Caches for intrinsic size computation, valid for one run of the track
	// sizing algorithm.
	gridAreaSizeCache           Size[Opt]
	hasGridAreaSizeCache        bool
	knownDimensionsCacheArea    Size[Opt]
	knownDimensionsCache        Size[Opt]
	hasKnownDimensionsCache     bool
	minContentContributionCache Size[Opt]
	minimumContributionCache    Size[Opt]
	maxContentContributionCache Size[Opt]

	// yPosition and height are the item's final position and height.
	yPosition, height float32

	oofCandidates []oofCandidate
}

func newGridItem(node Node, style *Style, parentAlignItems, parentJustifyItems Align, sourceOrder uint16) gridItem {
	return gridItem{
		node:                   node,
		sourceOrder:            sourceOrder,
		isCompressibleReplaced: style.isCompressibleReplaced(),
		isReplaced:             style.isReplaced(),
		overflow:               style.Overflow,
		boxSizing:              style.BoxSizing,
		size:                   style.Size,
		minSize:                style.MinSize,
		maxSize:                style.MaxSize,
		aspectRatio:            style.AspectRatio,
		padding:                style.padding(),
		border:                 style.border(),
		margin:                 style.margin(),
		alignSelf:              style.alignSelfOr(parentAlignItems),
		justifySelf:            style.justifySelfOr(parentJustifyItems),
	}
}

func (it *gridItem) hasAutoBlockMargin() bool {
	return it.margin.Top.IsAuto() || it.margin.Bottom.IsAuto()
}

func (it *gridItem) hasCyclicBlockSizeDependency() bool {
	return it.size.Height.usesPercentage() && (it.crossesIntrinsicRow || it.crossesFlexibleRow)
}

// participatesInBaselineAlignment reports whether the item is baseline
// aligned.
func (it *gridItem) participatesInBaselineAlignment() bool {
	return it.alignSelf.Keyword == AlignBaseline && !it.hasAutoBlockMargin() && !it.hasCyclicBlockSizeDependency()
}

func (it *gridItem) placement(axis AbstractAxis) Line[ozLine] {
	if axis == Block {
		return it.row
	}
	return it.column
}

func (it *gridItem) placementIndexes(axis AbstractAxis) Line[uint16] {
	if axis == Block {
		return it.rowIndexes
	}
	return it.columnIndexes
}

// trackRange is the range of track vector indexes the item spans, without
// the lines on either side.
func (it *gridItem) trackRange(axis AbstractAxis) (start, end int) {
	ix := it.placementIndexes(axis)
	return int(ix.Start) + 1, int(ix.End)
}

func (it *gridItem) spannedTracks(axis AbstractAxis, tracks []gridTrack) []gridTrack {
	s, e := it.trackRange(axis)
	return tracks[s:e]
}

func (it *gridItem) spansTrackMatching(axis AbstractAxis, tracks []gridTrack, pred func(*gridTrack) bool) bool {
	spanned := it.spannedTracks(axis, tracks)
	for i := range spanned {
		if pred(&spanned[i]) {
			return true
		}
	}
	return false
}

func (it *gridItem) span(axis AbstractAxis) uint16 {
	if axis == Block {
		return ozSpan(it.row)
	}
	return ozSpan(it.column)
}

func (it *gridItem) crossesFlexibleTrack(axis AbstractAxis) bool {
	if axis == Inline {
		return it.crossesFlexibleColumn
	}
	return it.crossesFlexibleRow
}

func (it *gridItem) crossesIntrinsicTrack(axis AbstractAxis) bool {
	if axis == Inline {
		return it.crossesIntrinsicColumn
	}
	return it.crossesIntrinsicRow
}

// spannedTrackLimit is the sum of the definite limits (fit-content
// arguments or fixed sizes) of the spanned tracks, if they all have one.
func (it *gridItem) spannedTrackLimit(axis AbstractAxis, tracks []gridTrack, axisParentSize Opt) Opt {
	spanned := it.spannedTracks(axis, tracks)
	for i := range spanned {
		if !spanned[i].max.definiteLimit(axisParentSize).Valid {
			return None
		}
	}
	var limit float32
	for i := range spanned {
		limit += spanned[i].max.definiteLimit(axisParentSize).Value
	}
	return Some(limit)
}

// spannedFixedTrackLimit is the sum of the fixed max track sizing functions
// of the spanned tracks, if they all have one.
func (it *gridItem) spannedFixedTrackLimit(axis AbstractAxis, tracks []gridTrack, axisParentSize Opt) Opt {
	spanned := it.spannedTracks(axis, tracks)
	for i := range spanned {
		if !spanned[i].max.definiteValue(axisParentSize).Valid {
			return None
		}
	}
	var limit float32
	for i := range spanned {
		limit += spanned[i].max.definiteValue(axisParentSize).Value
	}
	return Some(limit)
}

func (it *gridItem) knownDimensionsCached(gridAreaSize Size[Opt]) Size[Opt] {
	if it.hasKnownDimensionsCache && it.knownDimensionsCacheArea == gridAreaSize {
		return it.knownDimensionsCache
	}
	known := it.knownDimensions(gridAreaSize)
	it.knownDimensionsCacheArea = gridAreaSize
	it.knownDimensionsCache = known
	it.hasKnownDimensionsCache = true
	return known
}

func (it *gridItem) isStretchAligned(alignment Align) bool {
	switch alignment.Keyword {
	case AlignStretch:
		return true
	case AlignNormal:
		return !it.isReplaced
	}
	return false
}

// knownDimensions is the size the item would have in a grid area, where it
// does not depend on its content.
func (it *gridItem) knownDimensions(gridAreaSize Size[Opt]) Size[Opt] {
	margins := it.marginsAxisSumsWithBaselineShims(gridAreaSize.Width)

	aspectRatio := it.aspectRatio
	// CSS resolves percentage padding and border against the inline size of
	// the containing block. For a grid item under intrinsic measurement, that
	// is the grid area's width when it is definite.
	// https://www.w3.org/TR/css-grid-1/#item-margins
	// https://www.w3.org/TR/CSS22/box.html#padding-properties
	padding := rectResolveOrZero(it.padding, gridAreaSize.Width)
	border := rectResolveOrZero(it.border, gridAreaSize.Width)
	paddingBorderSize := rectSumAxes(rectAdd(padding, border))
	var boxSizingAdjustment Size[float32]
	if it.boxSizing == ContentBox {
		boxSizingAdjustment = paddingBorderSize
	}
	inherentSize := optsAddF(optsAspectRatio(sizeMaybeResolve(it.size, gridAreaSize), aspectRatio),
		boxSizingAdjustment)
	// The size of the item is floored by its padding and border.
	minSize := optsMaxF(optsOr(optsAddF(optsAspectRatio(sizeMaybeResolve(it.minSize, gridAreaSize), aspectRatio),
		boxSizingAdjustment), sizeSome(paddingBorderSize)), paddingBorderSize)
	maxSize := optsAddF(optsAspectRatio(sizeMaybeResolve(it.maxSize, gridAreaSize), aspectRatio),
		boxSizingAdjustment)

	areaMinusMargins := optsMaxF(optsSubF(gridAreaSize, margins), Size[float32]{})

	width := inherentSize.Width
	if !width.Valid {
		switch {
		case it.size.Width.isSizingKeyword():
			// A width that is a sizing keyword is not auto, so it does not
			// stretch. Stretch resolves to an exact width; the others resolve
			// during content measurement.
			if r, ok := resolveSizingKeyword(it.size.Width, areaMinusMargins.Width, gridAreaSize.Width); ok &&
				!r.measure {
				width = Some(r.exact)
			}
		case !it.margin.Left.IsAuto() && !it.margin.Right.IsAuto() && it.isStretchAligned(it.justifySelf):
			// Stretch the width if the alignment is stretch or normal (normal
			// is treated as stretch here regardless of a preferred size or
			// aspect ratio, unlike when the item is finally aligned in
			// gridAlignAndPositionItem; replaced elements are not stretched by
			// normal) and the item has no auto margins on this axis.
			width = areaMinusMargins.Width
		}
	}
	// Reapply the aspect ratio after stretching.
	s := optsAspectRatio(Size[Opt]{width, inherentSize.Height}, aspectRatio)

	if !s.Height.Valid {
		switch {
		case it.size.Height.isSizingKeyword():
			if r, ok := resolveSizingKeyword(it.size.Height, areaMinusMargins.Height, gridAreaSize.Height); ok &&
				!r.measure {
				s.Height = Some(r.exact)
			}
		case !it.margin.Top.IsAuto() && !it.margin.Bottom.IsAuto() && it.isStretchAligned(it.alignSelf):
			s.Height = areaMinusMargins.Height
		}
	}
	s = optsAspectRatio(s, aspectRatio)

	// Clamp by the min and max sizes.
	return optsClamp(s, minSize, maxSize)
}

// gridAreaSize is the size of the item's grid area as far as it is known:
// on axis, the sum of its fixed tracks; on the other axis, the estimated
// track sizes.
func (it *gridItem) gridAreaSize(axis AbstractAxis, axisTracks, otherAxisTracks []gridTrack, availableSpace Size[Opt],
	estimate func(*gridTrack, Opt) Opt) Size[Opt] {
	var size Size[Opt]

	sum := Some(0)
	for _, t := range it.spannedTracks(axis, axisTracks) {
		basis := availableSpace.getAbstract(axis)
		minSize := t.min.definiteValue(basis)
		maxSize := t.max.definiteValue(basis)
		if !minSize.Valid || !maxSize.Valid || minSize.Value != maxSize.Value {
			sum = None
			break
		}
		sum.Value += t.baseSize
	}
	size.setAbstract(axis, sum)

	other := axis.other()
	sum = Some(0)
	spanned := it.spannedTracks(other, otherAxisTracks)
	for i := range spanned {
		e := estimate(&spanned[i], availableSpace.getAbstract(other))
		if !e.Valid {
			sum = None
			break
		}
		sum.Value += e.Value + spanned[i].contentAlignmentAdjustment
	}
	size.setAbstract(other, sum)
	return size
}

func (it *gridItem) gridAreaSizeCached(axis AbstractAxis, axisTracks, otherAxisTracks []gridTrack,
	availableSpace Size[Opt], estimate func(*gridTrack, Opt) Opt) Size[Opt] {
	if it.hasGridAreaSizeCache {
		return it.gridAreaSizeCache
	}
	size := it.gridAreaSize(axis, axisTracks, otherAxisTracks, availableSpace, estimate)
	it.gridAreaSizeCache = size
	it.hasGridAreaSizeCache = true
	return size
}

// marginsAxisSumsWithBaselineShims sums the item's margins per axis, the
// top one including the baseline shim.
func (it *gridItem) marginsAxisSumsWithBaselineShims(innerNodeWidth Opt) Size[float32] {
	return Size[float32]{
		Width:  it.margin.Left.resolveOrZero(Some(0)) + it.margin.Right.resolveOrZero(Some(0)),
		Height: it.margin.Top.resolveOrZero(innerNodeWidth) + it.baselineShim + it.margin.Bottom.resolveOrZero(innerNodeWidth),
	}
}

func (it *gridItem) minContentContribution(axis AbstractAxis, gridAreaSize, availableSpace Size[Opt]) float32 {
	return it.contentContribution(axis, gridAreaSize, availableSpace, MinContentSpace)
}

func (it *gridItem) maxContentContribution(axis AbstractAxis, gridAreaSize, availableSpace Size[Opt]) float32 {
	return it.contentContribution(axis, gridAreaSize, availableSpace, MaxContentSpace)
}

// contentContribution measures the item under a min- or max-content
// constraint where the available space is indefinite.
func (it *gridItem) contentContribution(axis AbstractAxis, gridAreaSize, availableSpace Size[Opt],
	constraint AvailableSpace) float32 {
	known := it.knownDimensionsCached(gridAreaSize)
	// If the item's size on the axis is already known then that size is its
	// contribution, and the child need not be measured.
	if k := known.getAbstract(axis); k.Valid {
		return k.Value
	}
	// The child sees the grid area as its containing block during intrinsic
	// measurement, so percentage box properties resolve against the grid
	// area when that size is definite.
	// https://www.w3.org/TR/css-grid-1/#grid-item-sizing
	// https://www.w3.org/TR/css-grid-1/#algo-overview
	minus := it.availableSpaceMinusMargins(gridAreaSize, availableSpace)
	space := Size[AvailableSpace]{constraint, constraint}
	if minus.Width.Valid {
		space.Width = Definite(minus.Width.Value)
	}
	if minus.Height.Valid {
		space.Height = Definite(minus.Height.Value)
	}
	return measureChildSize(it.node, known, gridAreaSize, it.keywordAdjustedAvailableSpace(gridAreaSize, space),
		inherentSize, axis.asAbs(), Line[bool]{})
}

func (it *gridItem) minContentContributionCached(axis AbstractAxis, gridAreaSize, availableSpace Size[Opt]) float32 {
	if c := it.minContentContributionCache.getAbstract(axis); c.Valid {
		return c.Value
	}
	size := it.minContentContribution(axis, gridAreaSize, availableSpace)
	it.minContentContributionCache.setAbstract(axis, Some(size))
	return size
}

func (it *gridItem) maxContentContributionCached(axis AbstractAxis, gridAreaSize, availableSpace Size[Opt]) float32 {
	if c := it.maxContentContributionCache.getAbstract(axis); c.Valid {
		return c.Value
	}
	size := it.maxContentContribution(axis, gridAreaSize, availableSpace)
	it.maxContentContributionCache.setAbstract(axis, Some(size))
	return size
}

func (it *gridItem) availableSpaceMinusMargins(gridAreaSize, availableSpace Size[Opt]) Size[Opt] {
	if !availableSpace.Width.Valid && !availableSpace.Height.Valid {
		return availableSpace
	}
	margin := rectResolveOrZero(it.margin, gridAreaSize.Width)
	out := availableSpace
	if out.Width.Valid {
		out.Width.Value = fmax(out.Width.Value-rectHSum(margin), 0)
	}
	if out.Height.Valid {
		out.Height.Value = fmax(out.Height.Value-rectVSum(margin)-it.baselineShim, 0)
	}
	return out
}

// keywordAdjustedAvailableSpace measures an axis whose size is a sizing
// keyword in the space the keyword asks for.
func (it *gridItem) keywordAdjustedAvailableSpace(gridAreaSize Size[Opt],
	available Size[AvailableSpace]) Size[AvailableSpace] {
	if !it.size.Width.isSizingKeyword() && !it.size.Height.isSizingKeyword() {
		return available
	}
	margins := it.marginsAxisSumsWithBaselineShims(gridAreaSize.Width)
	for _, axis := range [...]AbstractAxis{Inline, Block} {
		sizeStyle := it.size.getAbstract(axis)
		if !sizeStyle.isSizingKeyword() {
			continue
		}
		stretch := gridAreaSize.getAbstract(axis).subF(margins.getAbstract(axis)).maxF(0)
		if r, ok := resolveSizingKeyword(sizeStyle, stretch, gridAreaSize.getAbstract(axis)); ok && r.measure {
			available.setAbstract(axis, r.available)
		}
	}
	return available
}

// minimumContribution is the item's minimum contribution: its outer size
// when its min size is auto and it would get its automatic minimum size.
func (it *gridItem) minimumContribution(axis AbstractAxis, axisTracks []gridTrack, gridAreaSize,
	innerNodeSize Size[Opt]) float32 {
	padding := rectResolveOrZero(it.padding, gridAreaSize.Width)
	border := rectResolveOrZero(it.border, gridAreaSize.Width)
	paddingBorderSize := rectSumAxes(rectAdd(padding, border))
	var boxSizingAdjustment Size[float32]
	if it.boxSizing == ContentBox {
		boxSizingAdjustment = paddingBorderSize
	}

	preferred := optsAddF(optsAspectRatio(sizeMaybeResolve(it.size, gridAreaSize), it.aspectRatio),
		boxSizingAdjustment).getAbstract(axis)
	if preferred.Valid {
		// A definite preferred size is clamped by the min and max sizes. For a
		// compressible replaced element, an indefinite percentage max size
		// resolves against zero.
		minSize := it.minSize.getAbstract(axis)
		maxSize := it.maxSize.getAbstract(axis)
		if minSize.IsAuto() && maxSize.IsAuto() {
			return preferred.Value
		}
		basis := gridAreaSize.getAbstract(axis)
		maxBasis := basis
		if it.isCompressibleReplaced {
			maxBasis = basis.OrOpt(Some(0))
		}
		adjustment := boxSizingAdjustment.getAbstract(axis)
		return fclamp(preferred.Value, minSize.maybeResolve(basis).addF(adjustment),
			maxSize.maybeResolve(maxBasis).addF(adjustment))
	}

	// A preferred size that is a sizing keyword other than stretch does not
	// behave as auto, so the minimum contribution is the min-content
	// contribution.
	if size := it.size.getAbstract(axis); size.isSizingKeyword() && !size.isStretch() {
		return it.minContentContributionCached(axis, gridAreaSize, gridAreaSize)
	}

	zeroBasis := Size[Opt]{Some(gridAreaSize.Width.Or(0)), Some(gridAreaSize.Height.Or(0))}
	if min := optsAddF(optsAspectRatio(sizeMaybeResolve(it.minSize, zeroBasis), it.aspectRatio),
		boxSizingAdjustment).getAbstract(axis); min.Valid {
		return min.Value
	}
	if overflow := it.overflowOn(axis).automaticMinSize(); overflow.Valid {
		return overflow.Value
	}

	// Automatic minimum size (https://www.w3.org/TR/css-grid-1/#min-size-auto).
	//
	// To provide a more reasonable default minimum size for grid items, the
	// used value of its automatic minimum size in a given axis is the
	// content-based minimum size if all of the following are true:
	itemTracks := it.spannedTracks(axis, axisTracks)

	// - it is not a scroll container (handled above)
	// - it spans at least one track in that axis whose min track sizing
	//   function is auto
	spansAutoMinTrack := false
	for i := range itemTracks {
		if itemTracks[i].min.behavesAsAuto(innerNodeSize.getAbstract(axis)) {
			spansAutoMinTrack = true
			break
		}
	}
	// - if it spans more than one track in that axis, none of those tracks
	//   are flexible
	onlySpanOneTrack := len(itemTracks) == 1
	spansFr := false
	for i := range itemTracks {
		if itemTracks[i].max.isFr() {
			spansFr = true
			break
		}
	}
	useContentBasedMinimum := spansAutoMinTrack && (onlySpanOneTrack || !spansFr)

	// Otherwise, the automatic minimum size is zero, as usual.
	if !useContentBasedMinimum {
		return 0
	}
	minimum := it.minContentContributionCached(axis, gridAreaSize, gridAreaSize)

	// If the item is a compressible replaced element and has a definite
	// preferred or max size on the axis, the size suggestion is capped by
	// them; any indefinite percentages in them resolve against zero.
	if it.isCompressibleReplaced {
		size := it.size.getAbstract(axis).maybeResolve(Some(0))
		maxSize := it.maxSize.getAbstract(axis).maybeResolve(Some(0))
		minimum = fminOpt(fminOpt(minimum, size), maxSize)
	}

	// The content-based minimum size is also clamped by the sum of any fixed
	// max track sizing functions of the spanned tracks. The clamp does not
	// apply to explicit preferred or min sizes, and fit-content() arguments do
	// not clamp it as fixed max track sizing functions do. It is a stretch fit
	// into the limit, so it does not shrink the item below its padding and
	// border.
	limit := it.spannedFixedTrackLimit(axis, axisTracks, innerNodeSize.getAbstract(axis))
	if limit.Valid {
		limit.Value = fmax(limit.Value, paddingBorderSize.getAbstract(axis))
	}
	return fminOpt(minimum, limit)
}

func (it *gridItem) overflowOn(axis AbstractAxis) Overflow {
	if axis == Inline {
		return it.overflow.X
	}
	return it.overflow.Y
}

func (it *gridItem) minimumContributionCached(axis AbstractAxis, axisTracks []gridTrack, gridAreaSize,
	innerNodeSize Size[Opt]) float32 {
	if c := it.minimumContributionCache.getAbstract(axis); c.Valid {
		return c.Value
	}
	size := it.minimumContribution(axis, axisTracks, gridAreaSize, innerNodeSize)
	it.minimumContributionCache.setAbstract(axis, Some(size))
	return size
}
