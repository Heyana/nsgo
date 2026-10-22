package layout

import (
	"sort"
	"strconv"
	"strings"
)

// CSS grid layout: a partial implementation of CSS Grid Level 1
// (https://www.w3.org/TR/css-grid-1), Taffy's compute_grid_layout.

// computeGridLayout lays out a grid container.
func computeGridLayout(n Node, in layoutInput) layoutOutput {
	known := in.knownDimensions
	parentSize := in.parentSize
	style := n.Style()

	// Container constants.
	direction := style.Direction
	contain := style.Contain
	claims := style.isContainingBlock()

	// 1. Compute the available grid space
	// (https://www.w3.org/TR/css-grid-1/#available-grid-space).
	aspectRatio := style.AspectRatio
	padding := rectResolveOrZero(style.padding(), parentSize.Width)
	border := rectResolveOrZero(style.border(), parentSize.Width)
	paddingBorder := rectAdd(padding, border)
	paddingBorderSize := rectSumAxes(paddingBorder)
	var boxSizingAdjustment Size[float32]
	if style.BoxSizing == ContentBox {
		boxSizingAdjustment = paddingBorderSize
	}

	minSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MinSize, parentSize), aspectRatio), boxSizingAdjustment)
	maxSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MaxSize, parentSize), aspectRatio), boxSizingAdjustment)
	var preferredSize Size[Opt]
	if in.sizingMode == inherentSize {
		preferredSize = optsAddF(optsAspectRatio(sizeMaybeResolve(style.Size, parentSize), aspectRatio),
			boxSizingAdjustment)
	}

	// Scrollbar gutters are reserved on the axis opposite the scrolling one.
	gutter := style.scrollbarGutter()
	isScrollContainer := style.Overflow.X.isScrollContainer() || style.Overflow.Y.isScrollContainer()
	contentBoxInset := paddingBorder
	contentBoxInset.Bottom += gutter.Y
	if direction.isRTL() {
		contentBoxInset.Left += gutter.X
	} else {
		contentBoxInset.Right += gutter.X
	}
	insetSum := rectSumAxes(contentBoxInset)

	// Normal behaves as stretch for grid containers.
	alignContent := style.gridAlignContent(Block)
	justifyContent := style.gridAlignContent(Inline)
	alignItems := style.alignItems()
	justifyItems := style.justifyItems()

	resolvedStyleSize := optsOr(known, preferredSize)
	constrained := spacesMaxF(spacesClampOpt(spacesMaybeSet(in.availableSpace, resolvedStyleSize), minSize, maxSize),
		paddingBorderSize)
	availableGridSpace := Size[AvailableSpace]{constrained.Width.subF(insetSum.Width), constrained.Height.subF(insetSum.Height)}

	outerNodeSize := optsMaxF(optsClamp(resolvedStyleSize, minSize, maxSize), paddingBorderSize)

	// Track sizing works on the content box, so the (border box) min and max
	// sizes are converted to content box sizes for it.
	innerMinSize := optsSubF(minSize, insetSum)
	innerMaxSize := optsSubF(maxSize, insetSum)
	innerNodeSize := optsSubF(outerNodeSize, insetSum)

	// Short-circuit when only the container's size is requested and its
	// size is fully determined.
	if in.runMode == computeSize {
		if optsBoth(outerNodeSize) {
			return outputFromOuterSize(optsUnwrapOr(outerNodeSize, Size[float32]{}))
		}
		// Likewise when only the width is requested and it is known.
		if in.axis == requestHorizontal && outerNodeSize.Width.Valid {
			return outputFromOuterSize(Size[float32]{outerNodeSize.Width.Value, 0})
		}
	}

	// 2. Resolve the explicit grid.

	// Like the inner node size, except that when that is indefinite a min
	// or max size style is used in its place.
	autoFitContainerSize := optsSubF(optsMaxF(optsClamp(optsOr(optsOr(outerNodeSize, maxSize), minSize), minSize,
		maxSize), paddingBorderSize), insetSum)

	// With a definite size or max size on the axis, the number of
	// repetitions is the largest that does not overflow the content box.
	// Otherwise, with a definite min size, it is the smallest that fulfils
	// the minimum. Otherwise the track list repeats once.
	strategy := func(o Opt) autoRepeatStrategy {
		if o.Valid {
			return maxRepetitionsThatDoNotOverflow
		}
		return minRepetitionsThatDoOverflow
	}
	strategySize := optsOr(outerNodeSize, maxSize)

	// The number of rows and columns of the explicit grid template (explicit
	// tracks from grid areas are counted below).
	colAutoRepetitions, templateColCount := explicitGridSizeInAxis(style, autoFitContainerSize.Width,
		strategy(strategySize.Width), Horizontal)
	rowAutoRepetitions, templateRowCount := explicitGridSizeInAxis(style, autoFitContainerSize.Height,
		strategy(strategySize.Height), Vertical)

	names := newNamedLineResolver(style, colAutoRepetitions, rowAutoRepetitions)

	// Clamp the explicit grid to the track limit on each axis
	// (https://www.w3.org/TR/css-grid-1/#overlarge-grids).
	explicitColCount := min(max(templateColCount, names.areaColumnCount), maxGridTracks)
	explicitRowCount := min(max(templateRowCount, names.areaRowCount), maxGridTracks)
	names.explicitColumnCount = explicitColCount
	names.explicitRowCount = explicitRowCount

	// The names of each line of the explicit grid.
	columnLineNames := names.detailedLineNames(Horizontal)
	rowLineNames := names.detailedLineNames(Vertical)

	// 3. Create the grid items, in document order, resolving the named lines
	// of their grid-row and grid-column. Absolutely positioned children do
	// not take part in placement and create no implicit tracks, so they are
	// left out here.
	childCount := n.ChildCount()
	items := make([]gridItem, 0, childCount)
	placements := make([]gridItemPlacement, 0, childCount)
	for i := range childCount {
		child := n.Child(i)
		cs := child.Style()
		if cs.Display == DisplayNone || cs.Position.isOutOfFlow() {
			continue
		}
		placements = append(placements, gridItemPlacement{
			horizontal: nnLineIntoOriginZero(names.resolveColumnNames(cs.GridColumn), explicitColCount),
			vertical:   nnLineIntoOriginZero(names.resolveRowNames(cs.GridRow), explicitRowCount),
		})
		items = append(items, newGridItem(child, cs, alignItems, justifyItems, uint16(i)))
	}

	// 4. Implicit grid: estimate the track counts, so that placement rarely
	// has to grow the occupancy matrix.
	estColCounts, estRowCounts := gridSizeEstimate(explicitColCount, explicitRowCount, placements)

	// 5. Grid item placement: give each item a definite area.
	matrix := newCellOccupancyMatrix(estColCounts, estRowCounts)
	placeGridItems(matrix, items, placements, style.GridAutoFlow)

	// Placement can add tracks.
	finalColCounts := matrix.trackCounts(Horizontal)
	finalRowCounts := matrix.trackCounts(Vertical)

	// 5. Initialise the tracks (explicit and implicit) and gutters, resolving
	// their sizing functions.
	columns := initializeGridTracks(nil, finalColCounts, style, Horizontal, colAutoRepetitions, matrix.columnIsOccupied)
	rows := initializeGridTracks(nil, finalRowCounts, style, Vertical, rowAutoRepetitions, matrix.rowIsOccupied)

	// 6. Track sizing.

	// Convert placements to track vector indexes, now that the final number
	// of negative implicit tracks is known.
	resolveItemTrackIndexes(items, finalColCounts, finalRowCounts)
	// Record whether each item crosses flexible or intrinsic tracks.
	determineIfItemCrossesFlexibleOrIntrinsicTracks(items, columns, rows)

	hasBaselineAlignedItem := false
	for i := range items {
		if items[i].participatesInBaselineAlignment() {
			hasBaselineAlignedItem = true
			break
		}
	}

	// Size the columns.
	trackSizingAlgorithm(Inline, innerMinSize.Width, innerMaxSize.Width, justifyContent, alignContent,
		availableGridSpace, innerNodeSize, columns, rows, items, estimateFromMaxSizingFunction, hasBaselineAlignedItem)
	initialColumnSum := sumBaseSizes(columns)
	innerNodeSize.Width = innerNodeSize.Width.OrOpt(Some(initialColumnSum))

	// If only the width is requested, the rows need not be sized (which saves
	// measuring every item's height), as column sizing does not re-run when
	// only computing the container's size. (A known width was handled above.)
	if in.runMode == computeSize && in.axis == requestHorizontal {
		width := fmax(fclamp(initialColumnSum+rectHSum(contentBoxInset), minSize.Width, maxSize.Width),
			paddingBorderSize.Width)
		return outputFromOuterSize(Size[float32]{width, 0})
	}

	for i := range items {
		items[i].hasGridAreaSizeCache = false
	}

	// Size the rows. Baseline alignment on the vertical axis is not
	// supported yet.
	trackSizingAlgorithm(Block, innerMinSize.Height, innerMaxSize.Height, alignContent, justifyContent,
		availableGridSpace, innerNodeSize, rows, columns, items, estimateFromBaseSize, false)
	initialRowSum := sumBaseSizes(rows)

	// 6. Compute the container size.
	borderBox := Size[float32]{
		Width: fmax(fclamp(resolvedStyleSize.Width.Or(initialColumnSum+rectHSum(contentBoxInset)), minSize.Width,
			maxSize.Width), paddingBorderSize.Width),
		Height: fmax(fclamp(resolvedStyleSize.Height.Or(initialRowSum+rectVSum(contentBoxInset)), minSize.Height,
			maxSize.Height), paddingBorderSize.Height),
	}
	contentBox := Size[float32]{
		fmax(0, borderBox.Width-rectHSum(contentBoxInset)),
		fmax(0, borderBox.Height-rectVSum(contentBoxInset)),
	}

	// If only the container's size is requested.
	if in.runMode == computeSize {
		return outputFromOuterSize(borderBox)
	}

	// The container's size is now known, so percentages resolve against its
	// content box when track sizing re-runs (a no-op if its size was known
	// upfront).
	innerNodeSize = sizeSome(contentBox)

	// Column sizing re-runs (once) if the container's width was indefinite
	// and some column has a percentage sizing function, or if the min-content
	// contribution width of an item crossing an intrinsic track changed.
	// TODO (Taffy): re-run only for the tracks that need it.
	hasPercentageColumn := anyTrackUsesPercentage(columns)
	hasPercentageRow := anyTrackUsesPercentage(rows)
	rerunColumnSizing := !outerNodeSize.Width.Valid && hasPercentageColumn
	intrinsicColumnContributionChanged := false

	if !rerunColumnSizing {
		// Every item is visited, as this updates each item's caches.
		for i := range items {
			it := &items[i]
			if !it.crossesIntrinsicColumn {
				continue
			}
			area := it.gridAreaSize(Inline, columns, rows, innerNodeSize, estimateFromBaseSize)
			available := area.withAbstract(Inline, None)
			contribution := it.minContentContribution(Inline, area, available)
			changed := !it.minContentContributionCache.Width.Valid ||
				it.minContentContributionCache.Width.Value != contribution
			it.gridAreaSizeCache, it.hasGridAreaSizeCache = area, true
			it.minContentContributionCache.Width = Some(contribution)
			it.maxContentContributionCache.Width = None
			it.minimumContributionCache.Width = None
			intrinsicColumnContributionChanged = intrinsicColumnContributionChanged || changed
		}
		rerunColumnSizing = intrinsicColumnContributionChanged
	} else {
		// Clear the intrinsic width caches.
		for i := range items {
			it := &items[i]
			it.hasGridAreaSizeCache = false
			it.minContentContributionCache.Width = None
			it.maxContentContributionCache.Width = None
			it.minimumContributionCache.Width = None
		}
	}

	// Row sizing re-runs (once) if the container's height was indefinite and
	// some row has a percentage sizing function, or if columns were re-sized
	// and the min-content contribution height of an item crossing an
	// intrinsic track changed.
	rerunRowSizing := !outerNodeSize.Height.Valid && hasPercentageRow
	intrinsicRowContributionChanged := false

	if rerunRowSizing {
		// Clear the intrinsic height caches.
		for i := range items {
			it := &items[i]
			it.hasGridAreaSizeCache = false
			it.minContentContributionCache.Height = None
			it.maxContentContributionCache.Height = None
			it.minimumContributionCache.Height = None
		}
	}

	if rerunColumnSizing {
		trackSizingAlgorithm(Inline, innerMinSize.Width, innerMaxSize.Width, justifyContent, alignContent,
			availableGridSpace, innerNodeSize, columns, rows, items, estimateFromBaseSize, hasBaselineAlignedItem)

		// The column widths may have changed, so the items' intrinsic height
		// contributions need recomputing (unless row sizing re-runs anyway,
		// whose caches are already cleared).
		if !rerunRowSizing {
			for i := range items {
				it := &items[i]
				if !it.crossesIntrinsicRow {
					continue
				}
				area := it.gridAreaSize(Block, rows, columns, innerNodeSize, estimateFromBaseSize)
				available := area.withAbstract(Block, None)
				contribution := it.minContentContribution(Block, area, available)
				changed := !it.minContentContributionCache.Height.Valid ||
					it.minContentContributionCache.Height.Value != contribution
				it.gridAreaSizeCache, it.hasGridAreaSizeCache = area, true
				it.minContentContributionCache.Height = Some(contribution)
				it.maxContentContributionCache.Height = None
				it.minimumContributionCache.Height = None
				intrinsicRowContributionChanged = intrinsicRowContributionChanged || changed
			}
			rerunRowSizing = intrinsicRowContributionChanged
		}
	}

	if rerunRowSizing {
		trackSizingAlgorithm(Block, innerMinSize.Height, innerMaxSize.Height, alignContent, justifyContent,
			availableGridSpace, innerNodeSize, rows, columns, items, estimateFromBaseSize, false)
	}

	if (intrinsicColumnContributionChanged && !hasPercentageColumn) ||
		(intrinsicRowContributionChanged && !hasPercentageRow) {
		finalColumnSum := sumBaseSizes(columns)
		finalRowSum := sumBaseSizes(rows)
		if intrinsicColumnContributionChanged && !hasPercentageColumn {
			borderBox.Width = fmax(fclamp(resolvedStyleSize.Width.Or(finalColumnSum+rectHSum(contentBoxInset)),
				minSize.Width, maxSize.Width), paddingBorderSize.Width)
			contentBox.Width = fmax(0, borderBox.Width-rectHSum(contentBoxInset))
		}
		if intrinsicRowContributionChanged && !hasPercentageRow {
			borderBox.Height = fmax(fclamp(resolvedStyleSize.Height.Or(finalRowSum+rectVSum(contentBoxInset)),
				minSize.Height, maxSize.Height), paddingBorderSize.Height)
			contentBox.Height = fmax(0, borderBox.Height-rectVSum(contentBoxInset))
		}
	}

	// 8. Track alignment.

	// Align the columns.
	inlineSizeWithoutScrollbar := fmax(borderBox.Width-paddingBorderSize.Width, 0)
	inlineGutterForAlignment := fmin(gutter.X, inlineSizeWithoutScrollbar)
	columnPadding := Line[float32]{padding.Left, padding.Right + inlineGutterForAlignment}
	if direction.isRTL() {
		columnPadding = Line[float32]{padding.Left + inlineGutterForAlignment, padding.Right}
	}
	gridAlignTracks(contentBox.Width, columnPadding, Line[float32]{border.Left, border.Right}, columns, justifyContent,
		direction.isRTL())
	// Align the rows.
	gridAlignTracks(contentBox.Height, Line[float32]{padding.Top, padding.Bottom},
		Line[float32]{border.Top, border.Bottom}, rows, alignContent, false)

	// 9. Size, align and position the grid items.
	var itemOverflow Rect[float32]

	// Out-of-flow candidates in document order: direct out-of-flow children
	// interleaved with candidates bubbled from in-flow children's subtrees,
	// laid out by the out-of-flow pass after this algorithm.
	var candidates []oofCandidate

	for i := range items {
		it := &items[i]
		// Tracks are in logical order. In RTL physical offsets are assigned
		// right to left, so an item's physical left edge comes from its
		// logical end line and its right edge from its start line.
		area := Rect[float32]{
			Top:    rows[it.rowIndexes.Start+1].offset,
			Bottom: rows[it.rowIndexes.End].offset,
			Left:   columns[it.columnIndexes.Start+1].offset,
			Right:  columns[it.columnIndexes.End].offset,
		}
		if direction.isRTL() {
			area.Left = columns[it.columnIndexes.End-1].offset
			area.Right = columns[it.columnIndexes.Start].offset
		}
		contribution, y, height := gridAlignAndPositionItem(it.node, uint32(i), area,
			Size[float32]{area.Right - area.Left, area.Bottom - area.Top}, alignItems, justifyItems, it.baselineShim,
			direction, borderBox.Width, border, isScrollContainer, &it.oofCandidates)
		it.yPosition = y
		it.height = height
		itemOverflow = rectUnion(itemOverflow, contribution)
	}

	// Position hidden and out-of-flow children, merging in the candidates
	// bubbled from in-flow children (items are in child order).
	order := uint32(len(items))
	next := 0
	for index := range childCount {
		if next < len(items) && int(items[next].sourceOrder) == index {
			candidates = append(candidates, items[next].oofCandidates...)
			items[next].oofCandidates = nil
			next++
			continue
		}

		child := n.Child(index)
		cs := child.Style()

		// Hidden children.
		if cs.Display == DisplayNone {
			child.LayoutState().Unrounded = Layout{Order: order}
			performChildLayout(child, noneSize, noneSize, maxContentSpaces, inherentSize, Line[bool]{})
			order++
			continue
		}

		// Out-of-flow children become candidates for the out-of-flow pass,
		// which lays out those this grid contains after this algorithm
		// (resolving their grid areas from the detailed grid info stored
		// below).
		if cs.Position.isOutOfFlow() {
			position := cs.Position
			itemDirection := cs.Direction
			justifySelf := cs.justifySelfOr(justifyItems).resolveSelfRelative(itemDirection, direction, true)
			alignSelf := cs.alignSelfOr(alignItems).resolveSelfRelative(itemDirection, direction, false)

			// The static-position rectangle: the grid area of the placement
			// properties when this grid is the child's containing block, and
			// otherwise the grid's content box, as if the child were the sole
			// item in an area matching the grid's content edges (placement
			// does not apply when the grid is not the containing block).
			// https://www.w3.org/TR/css-grid-1/#static-position
			var area Rect[float32]
			if claims.forPosition(position) {
				area = gridStaticPositionArea(cs, names, finalRowCounts, finalColCounts, rows, columns, direction,
					border, gutter, borderBox)
			} else {
				area = Rect[float32]{
					Left:   border.Left + padding.Left,
					Right:  borderBox.Width - border.Right - padding.Right - gutter.X,
					Top:    border.Top + padding.Top,
					Bottom: borderBox.Height - border.Bottom - padding.Bottom - gutter.Y,
				}
				if direction.isRTL() {
					area.Left = border.Left + padding.Left + gutter.X
					area.Right = borderBox.Width - border.Right - padding.Right
				}
			}

			candidates = append(candidates, oofCandidate{
				node:     child,
				order:    order,
				position: position,
				staticPosition: Point[axisStaticPosition]{
					X: staticPositionFromAlignment(justifySelf, Line[float32]{area.Left, area.Right}, direction.isRTL()),
					Y: staticPositionFromAlignment(alignSelf, Line[float32]{area.Top, area.Bottom}, false),
				},
			})
			order++
		}
	}

	areaInset := border
	if direction.isRTL() {
		areaInset.Left += gutter.X
	} else {
		areaInset.Right += gutter.X
	}
	areaInset.Bottom += gutter.Y
	oofArea := oofPositioningArea{
		size:   sizeSub(borderBox, rectSumAxes(areaInset)),
		offset: Point[float32]{areaInset.Left, areaInset.Top},
	}

	// Store the detailed grid info before the out-of-flow pass, which uses it
	// to resolve the grid areas of the out-of-flow boxes this grid contains.
	columnLineNames.resolver = names.columnLines
	rowLineNames.resolver = names.rowLines
	st := n.LayoutState()
	info := st.grid
	if info == nil {
		info = &gridInfo{}
	}
	info.rows.set(finalRowCounts, rows, rowLineNames)
	info.columns.set(finalColCounts, columns, columnLineNames)
	info.areas = names.areas
	st.grid = info

	// A scroll container's own end padding is part of its scrollable
	// overflow; other boxes don't extend their overflow by their padding.
	overflow := itemOverflow
	if isScrollContainer {
		if direction.isRTL() {
			overflow.Right += padding.Left
		} else {
			overflow.Right += padding.Right
		}
		overflow.Bottom += padding.Bottom
	}

	// Without in-flow items, there is no baseline.
	if len(items) == 0 {
		out := outputFromSizes(borderBox, overflow, baselines{})
		out.oofCandidates = candidates
		out.oofArea, out.hasOofArea = oofArea, true
		return out
	}

	// The grid container's first baseline. Layout containment suppresses it.
	var baseline Opt
	if !contain.suppressesBaseline() {
		// The first row containing items.
		firstRow := items[0].rowIndexes.Start
		for i := range items {
			firstRow = min(firstRow, items[i].rowIndexes.Start)
		}
		// An item of that row participating in baseline alignment, if any
		// (items with an auto block-axis margin do not:
		// https://www.w3.org/TR/css-align-3/#baseline-align-self), else its
		// first item.
		var first, chosen *gridItem
		for i := range items {
			it := &items[i]
			if it.rowIndexes.Start != firstRow {
				continue
			}
			if first == nil {
				first = it
			}
			if it.participatesInBaselineAlignment() {
				chosen = it
				break
			}
		}
		if chosen == nil {
			chosen = first
		}
		baseline = Some(chosen.yPosition + chosen.baseline.Or(chosen.height))
	}

	out := outputFromSizes(borderBox, overflow, baselines{first: baseline})
	out.oofCandidates = candidates
	out.oofArea, out.hasOofArea = oofArea, true
	return out
}

func sumBaseSizes(tracks []gridTrack) float32 {
	var sum float32
	for i := range tracks {
		sum += tracks[i].baseSize
	}
	return sum
}

func anyTrackUsesPercentage(tracks []gridTrack) bool {
	for i := range tracks {
		if tracks[i].usesPercentage() {
			return true
		}
	}
	return false
}

// gridStaticPositionArea is the static-position rectangle of an
// out-of-flow child this grid contains: the grid area its placement
// properties name.
func gridStaticPositionArea(cs *Style, names *namedLineResolver, rowCounts, colCounts trackCounts, rows,
	columns []gridTrack, direction Direction, border Rect[float32], gutter Point[float32],
	borderBox Size[float32]) Rect[float32] {
	// Convert the placements into indexes into the track vectors, absent for
	// auto and unresolvable spans.
	trackIndexes := func(l Line[optOZLine], counts trackCounts) (start, end int, okStart, okEnd bool) {
		if l.Start.valid {
			start, okStart = l.Start.line.tryTrackVecIndex(counts)
		}
		if l.End.valid {
			end, okEnd = l.End.line.tryTrackVecIndex(counts)
		}
		return
	}
	colStart, colEnd, okColStart, okColEnd := trackIndexes(ozResolveAbsolutelyPositioned(
		nnLineIntoOriginZero(names.resolveColumnNames(cs.GridColumn), colCounts.explicit)), colCounts)
	rowStart, rowEnd, okRowStart, okRowEnd := trackIndexes(ozResolveAbsolutelyPositioned(
		nnLineIntoOriginZero(names.resolveRowNames(cs.GridRow), rowCounts.explicit)), rowCounts)

	lineAsStartEdge := func(tracks []gridTrack, i int) float32 {
		if i+1 < len(tracks) {
			return tracks[i+1].offset
		}
		return tracks[i].offset
	}
	lineAsEndEdge := func(tracks []gridTrack, i int) float32 {
		if i == 0 {
			if len(tracks) > 1 {
				return tracks[1].offset
			}
			return tracks[0].offset
		}
		return tracks[i].offset
	}
	// In RTL tracks stay in logical order but physical offsets are assigned
	// right to left: a line used as an inline-start edge resolves to the
	// physical right edge of the track after it (its gutter's offset), and a
	// line used as an inline-end edge to the physical left edge (offset) of
	// the track before it.
	rtlLineAsStartEdge := func(tracks []gridTrack, i int) float32 {
		switch {
		case len(tracks) > i+1:
			// The gutter's offset is the physical right edge of the following
			// track.
			return tracks[i].offset
		case i == 0:
			return tracks[0].offset
		}
		// No track follows the line: resolve to the line itself, the physical
		// left edge of the track before it (the trailing gutter gets its
		// offset before any alignment offset, so it cannot be used).
		return tracks[i-1].offset
	}
	rtlLineAsEndEdge := func(tracks []gridTrack, i int) float32 {
		if i == 0 {
			return tracks[0].offset
		}
		return tracks[i-1].offset
	}

	// In RTL the physical left edge comes from the logical end line and the
	// right edge from the start line.
	var area Rect[float32]
	if direction.isRTL() {
		area.Left = border.Left + gutter.X
		if okColEnd {
			area.Left = rtlLineAsEndEdge(columns, colEnd)
		}
		area.Right = borderBox.Width - border.Right
		if okColStart {
			area.Right = rtlLineAsStartEdge(columns, colStart)
		}
	} else {
		area.Left = border.Left
		if okColStart {
			area.Left = lineAsStartEdge(columns, colStart)
		}
		area.Right = borderBox.Width - border.Right - gutter.X
		if okColEnd {
			area.Right = lineAsEndEdge(columns, colEnd)
		}
	}
	area.Top = border.Top
	if okRowStart {
		area.Top = lineAsStartEdge(rows, rowStart)
	}
	area.Bottom = borderBox.Height - border.Bottom - gutter.Y
	if okRowEnd {
		area.Bottom = lineAsEndEdge(rows, rowEnd)
	}
	return area
}

// gridInfo is a grid container's detailed layout information (Taffy's
// DetailedGridInfo): its tracks' positions and line names, used to position
// the out-of-flow boxes it contains against grid areas.
type gridInfo struct {
	rows, columns gridTracksInfo
	// areas resolves the grid area edges of out-of-flow children.
	areas gridAreasMap
}

// gridTracksInfo is the detailed information of one axis's tracks.
type gridTracksInfo struct {
	negativeImplicit, explicit, positiveImplicit uint16
	// positions are the start and end of each track relative to the
	// container's border box, after borders, padding, gaps, content
	// alignment and collapsed tracks.
	positions []Line[float32]
	// collapsed are the sorted, disjoint index ranges (into positions) of
	// collapsed auto-fit tracks, whose adjacent gutters overlap when
	// resolving grid area edges.
	collapsed [][2]int
	// emptyAxisLine is the position of the axis's single line when it has no
	// tracks (an empty grid still has one line per axis, placed by content
	// alignment).
	emptyAxisLine Opt
	// lineNames are the names of each explicit line.
	lineNames gridLineNames
}

// set records the tracks of an axis, reusing the info's slices.
func (t *gridTracksInfo) set(counts trackCounts, tracks []gridTrack, lineNames gridLineNames) {
	t.negativeImplicit, t.explicit, t.positiveImplicit = counts.negativeImplicit, counts.explicit,
		counts.positiveImplicit
	t.positions = t.positions[:0]
	t.collapsed = t.collapsed[:0]
	for i := range tracks {
		track := &tracks[i]
		if track.isGutter {
			continue
		}
		if track.isCollapsed {
			index := len(t.positions)
			if n := len(t.collapsed); n > 0 && t.collapsed[n-1][1] == index {
				t.collapsed[n-1][1]++
			} else {
				t.collapsed = append(t.collapsed, [2]int{index, index + 1})
			}
		}
		t.positions = append(t.positions, Line[float32]{track.offset, track.offset + track.baseSize})
	}
	// An axis without tracks is a single gutter, whose offset is where
	// content alignment put the axis's single line.
	t.emptyAxisLine = None
	if len(t.positions) == 0 && len(tracks) > 0 {
		t.emptyAxisLine = Some(tracks[0].offset)
	}
	t.lineNames = lineNames
}

func (t *gridTracksInfo) counts() trackCounts {
	return trackCounts{t.negativeImplicit, t.explicit, t.positiveImplicit}
}

// resolveAbsoluteGridAxis resolves an out-of-flow child's placement on one
// axis to physical start and end coordinates.
func (t *gridTracksInfo) resolveAbsoluteGridAxis(placement Line[GridPlacement], areas gridAreasMap,
	axis gridAreaAxis, paddingStart, paddingEnd float32, reversed bool) Line[float32] {
	counts := t.counts()
	minLine := ozLine(-int16(counts.negativeImplicit))
	maxLine := ozLine(int16(counts.explicit + counts.positiveImplicit))
	// Resolve the placement to a pair of (sorted) lines first, and only then
	// treat lines outside the grid as auto. A placement whose lines end up
	// swapped (grid-column: 3 / 1, or a missing named line resolving past the
	// end of the explicit grid) is normalized before the out-of-grid check,
	// so foo / 3 on a 2-column grid resolves to the area between line 3 and
	// the end padding edge.
	lines := ozResolveAbsolutelyPositioned(nnLineIntoOriginZero(
		t.lineNames.resolveLineNames(placement, areas, axis, t.explicit), t.explicit))
	index := func(l optOZLine) (int, bool) {
		if !l.valid || l.line < minLine || l.line > maxLine {
			return 0, false
		}
		i, ok := l.line.tryTrackVecIndex(counts)
		return i / 2, ok
	}

	startPosition := paddingStart
	if reversed {
		startPosition = paddingEnd
	}
	if line, ok := index(lines.Start); ok {
		switch {
		case line < len(t.positions):
			startPosition = t.absoluteTrackEdge(line, true, reversed)
		case len(t.positions) > 0:
			last := t.positions[len(t.positions)-1]
			startPosition = last.End
			if reversed {
				startPosition = last.Start
			}
		case t.emptyAxisLine.Valid:
			startPosition = t.emptyAxisLine.Value
		}
	}
	endPosition := paddingEnd
	if reversed {
		endPosition = paddingStart
	}
	if line, ok := index(lines.End); ok {
		switch {
		case line >= 1 && line-1 < len(t.positions):
			endPosition = t.absoluteTrackEdge(line-1, false, reversed)
		case len(t.positions) > 0:
			first := t.positions[0]
			endPosition = first.Start
			if reversed {
				endPosition = first.End
			}
		case t.emptyAxisLine.Valid:
			endPosition = t.emptyAxisLine.Value
		}
	}

	// An area whose lines all fall within a collapsed run is zero-sized and
	// anchored at its logical start edge (as in Chrome; Gecko anchors it
	// before the shared gutter instead).
	if reversed {
		return Line[float32]{fmin(startPosition, endPosition), startPosition}
	}
	return Line[float32]{startPosition, fmax(startPosition, endPosition)}
}

// absoluteTrackEdge resolves a track edge, skipping gutters that overlap
// across a collapsed run of tracks.
func (t *gridTracksInfo) absoluteTrackEdge(index int, isStart, reversed bool) float32 {
	track := t.positions[index]
	r := sort.Search(len(t.collapsed), func(i int) bool { return t.collapsed[i][1] > index })
	if r < len(t.collapsed) && t.collapsed[r][0] <= index && index < t.collapsed[r][1] {
		adjacent := t.collapsed[r][1]
		ok := true
		if !isStart {
			adjacent = t.collapsed[r][0] - 1
			ok = t.collapsed[r][0] >= 1
		}
		if ok && adjacent < len(t.positions) {
			track = t.positions[adjacent]
		}
	}
	// Collapsed gutters overlap: a start edge follows the shared gutter, and
	// an end edge precedes it.
	if isStart != reversed {
		return track.Start
	}
	return track.End
}

// namesForLine returns the names of a line of the whole grid (implicit
// tracks included), line i bounding the start of track i.
func (t *gridTracksInfo) namesForLine(i int) []string {
	stored := i - int(t.negativeImplicit)
	if stored < 0 {
		return nil
	}
	return t.lineNames.line(stored)
}

// trackList is the resolved value of grid-template-rows or
// grid-template-columns (https://www.w3.org/TR/css-grid-1/#resolved-track-list),
// with sizes divided by zoom.
func (t *gridTracksInfo) trackList(zoom float32) string {
	if len(t.positions) == 0 {
		return "none"
	}
	var b strings.Builder
	writeNames := func(names []string) {
		b.WriteByte('[')
		b.WriteString(strings.Join(names, " "))
		b.WriteByte(']')
	}
	needsSpace := false
	for i, p := range t.positions {
		if names := t.namesForLine(i); len(names) > 0 {
			if needsSpace {
				b.WriteByte(' ')
			}
			writeNames(names)
			needsSpace = true
		}
		if needsSpace {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.FormatFloat(float64((p.End-p.Start)/zoom), 'f', -1, 32))
		b.WriteString("px")
		needsSpace = true
	}
	if names := t.namesForLine(len(t.positions)); len(names) > 0 {
		b.WriteByte(' ')
		writeNames(names)
	}
	return b.String()
}

// resolveAbsoluteGridArea resolves the physical grid area of an absolutely
// positioned box from its placement. The padding box and the result are
// relative to the grid container's border box.
func (g *gridInfo) resolveAbsoluteGridArea(row, column Line[GridPlacement], direction Direction,
	area Rect[float32]) Rect[float32] {
	cols := g.columns.resolveAbsoluteGridAxis(column, g.areas, areaColumn, area.Left, area.Right, direction.isRTL())
	rows := g.rows.resolveAbsoluteGridAxis(row, g.areas, areaRow, area.Top, area.Bottom, false)
	return Rect[float32]{Left: cols.Start, Right: cols.End, Top: rows.Start, Bottom: rows.End}
}

// resolvedTrackList is the resolved value of grid-template-rows (Vertical)
// or grid-template-columns (Horizontal), as getComputedStyle reports it.
func (g *gridInfo) resolvedTrackList(axis AbsoluteAxis, scale float32) string {
	if axis == Horizontal {
		return g.columns.trackList(scale)
	}
	return g.rows.trackList(scale)
}
