package layout

import "sort"

// The grid track sizing algorithm
// (https://www.w3.org/TR/css-grid-1/#layout-algorithm).

// gridTrackEstimate estimates a track's size from its sizing function or
// its size so far, for sizing items in the other axis.
type gridTrackEstimate func(t *gridTrack, basis Opt) Opt

// estimateFromMaxSizingFunction estimates a track by its fixed max track
// sizing function.
func estimateFromMaxSizingFunction(t *gridTrack, basis Opt) Opt { return t.max.definiteValue(basis) }

// estimateFromBaseSize estimates a track by its base size.
func estimateFromBaseSize(t *gridTrack, _ Opt) Opt { return Some(t.baseSize) }

// gridItemBatcher iterates over items sorted by span, in batches of equal
// span, all items crossing a flexible track forming the last batch.
type gridItemBatcher struct {
	axis          AbstractAxis
	indexOffset   int
	currentSpan   uint16
	currentIsFlex bool
}

func (b *gridItemBatcher) next(items []*gridItem) ([]*gridItem, bool, bool) {
	if b.currentIsFlex || b.indexOffset >= len(items) {
		return nil, false, false
	}
	item := items[b.indexOffset]
	b.currentSpan = item.span(b.axis)
	b.currentIsFlex = item.crossesFlexibleTrack(b.axis)

	next := len(items)
	if !b.currentIsFlex {
		for i, it := range items {
			if it.crossesFlexibleTrack(b.axis) || it.span(b.axis) > b.currentSpan {
				next = i
				break
			}
		}
	}
	batch := items[b.indexOffset:next]
	b.indexOffset = next
	return batch, b.currentIsFlex, true
}

// gridIntrinsicSizer computes items' outer contributions on an axis.
type gridIntrinsicSizer struct {
	otherAxisTracks []gridTrack
	estimate        gridTrackEstimate
	axis            AbstractAxis
	innerNodeSize   Size[Opt]
}

func (s *gridIntrinsicSizer) gridAreaSize(item *gridItem, axisTracks []gridTrack) Size[Opt] {
	return item.gridAreaSizeCached(s.axis, axisTracks, s.otherAxisTracks, s.innerNodeSize, s.estimate)
}

func (s *gridIntrinsicSizer) minContentContribution(item *gridItem, axisTracks []gridTrack) float32 {
	area := s.gridAreaSize(item, axisTracks)
	available := area.withAbstract(s.axis, None)
	margins := item.marginsAxisSumsWithBaselineShims(available.Width)
	return item.minContentContributionCached(s.axis, area, available) + margins.getAbstract(s.axis)
}

func (s *gridIntrinsicSizer) maxContentContribution(item *gridItem, axisTracks []gridTrack) float32 {
	area := s.gridAreaSize(item, axisTracks)
	available := area.withAbstract(s.axis, None)
	margins := item.marginsAxisSumsWithBaselineShims(available.Width)
	return item.maxContentContributionCached(s.axis, area, available) + margins.getAbstract(s.axis)
}

func (s *gridIntrinsicSizer) minimumContribution(item *gridItem, axisTracks []gridTrack) float32 {
	area := s.gridAreaSize(item, axisTracks)
	available := area.withAbstract(s.axis, None)
	margins := item.marginsAxisSumsWithBaselineShims(available.Width)
	return item.minimumContributionCached(s.axis, axisTracks, area, s.innerNodeSize) + margins.getAbstract(s.axis)
}

// gridAlignmentGutterAdjustment estimates the space content alignment adds
// to each inner gutter.
func gridAlignmentGutterAdjustment(alignment AlignContent, axisInnerNodeSize Opt, estimate gridTrackEstimate,
	tracks []gridTrack) float32 {
	if len(tracks) <= 1 {
		return 0
	}

	// Items never cross the outermost gutters, so start and end can be
	// treated alike. Safety does not change gutter weights; overflow
	// fallback is handled when offsets are computed.
	var outerWeight, innerWeight int
	switch alignment.Keyword {
	case ContentStart, ContentFlexStart, ContentEnd, ContentFlexEnd, ContentCenter:
		outerWeight, innerWeight = 1, 0
	case ContentSpaceBetween:
		outerWeight, innerWeight = 0, 1
	case ContentSpaceAround:
		outerWeight, innerWeight = 1, 2
	case ContentSpaceEvenly:
		outerWeight, innerWeight = 1, 1
	default:
		// Normal is resolved to stretch by the grid container.
		outerWeight, innerWeight = 0, 0
	}
	if innerWeight == 0 || !axisInnerNodeSize.Valid {
		return 0
	}

	freeSpace := float32(0)
	sum := float32(0)
	ok := true
	for i := range tracks {
		e := estimate(&tracks[i], axisInnerNodeSize)
		if !e.Valid {
			ok = false
			break
		}
		sum += e.Value
	}
	if ok {
		freeSpace = fmax(0, axisInnerNodeSize.Value-sum)
	}
	weightedTrackCount := ((len(tracks)-3)/2)*innerWeight + 2*outerWeight
	return (freeSpace / float32(weightedTrackCount)) * float32(innerWeight)
}

// resolveItemTrackIndexes converts items' placements to track vector
// indexes.
func resolveItemTrackIndexes(items []gridItem, columnCounts, rowCounts trackCounts) {
	for i := range items {
		it := &items[i]
		it.columnIndexes = Line[uint16]{
			uint16(it.column.Start.trackVecIndex(columnCounts)), uint16(it.column.End.trackVecIndex(columnCounts)),
		}
		it.rowIndexes = Line[uint16]{
			uint16(it.row.Start.trackVecIndex(rowCounts)), uint16(it.row.End.trackVecIndex(rowCounts)),
		}
	}
}

// determineIfItemCrossesFlexibleOrIntrinsicTracks records, per item and
// axis, whether it spans flexible or intrinsic tracks.
func determineIfItemCrossesFlexibleOrIntrinsicTracks(items []gridItem, columns, rows []gridTrack) {
	for i := range items {
		it := &items[i]
		it.crossesFlexibleColumn, it.crossesIntrinsicColumn = false, false
		for _, t := range it.spannedTracks(Inline, columns) {
			it.crossesFlexibleColumn = it.crossesFlexibleColumn || t.isFlexible()
			it.crossesIntrinsicColumn = it.crossesIntrinsicColumn || t.hasIntrinsicSizingFunction()
		}
		it.crossesFlexibleRow, it.crossesIntrinsicRow = false, false
		for _, t := range it.spannedTracks(Block, rows) {
			it.crossesFlexibleRow = it.crossesFlexibleRow || t.isFlexible()
			it.crossesIntrinsicRow = it.crossesIntrinsicRow || t.hasIntrinsicSizingFunction()
		}
	}
}

// trackSizingAlgorithm sizes the tracks of one axis.
func trackSizingAlgorithm(axis AbstractAxis, axisMinSize, axisMaxSize Opt, axisAlignment,
	otherAxisAlignment AlignContent, availableGridSpace Size[AvailableSpace], innerNodeSize Size[Opt],
	axisTracks, otherAxisTracks []gridTrack, items []gridItem, estimate gridTrackEstimate,
	hasBaselineAlignedItem bool) {
	// 11.4 Initialise Track sizes: each track's base size and growth limit.
	percentageBasis := innerNodeSize.getAbstract(axis).OrOpt(axisMinSize)
	initializeTrackSizes(axisTracks, percentageBasis)

	// Resolve fit-content() limits against the container's inner size (they
	// are fixed for the rest of this pass).
	axisInnerNodeSize := innerNodeSize.getAbstract(axis)
	for i := range axisTracks {
		axisTracks[i].resolveFitContentLimit(axisInnerNodeSize)
	}

	// 11.5.1 Shim item baselines.
	if hasBaselineAlignedItem {
		resolveItemBaselines(axis, items, innerNodeSize)
	}

	// If every track has a fixed min track sizing function and its base size
	// equals its growth limit, the track sizes are final. Tracks with an
	// intrinsic min track sizing function can still grow beyond a fixed
	// growth limit (as minmax(auto, 0px) does), so they cannot be skipped.
	allFinal := true
	for i := range axisTracks {
		t := &axisTracks[i]
		if !(t.baseSize == t.growthLimit && t.min.definiteValue(percentageBasis).Valid) {
			allFinal = false
			break
		}
	}
	if allFinal {
		return
	}

	// Pre-computations for 11.5 Resolve Intrinsic Track Sizes.

	// The space content alignment adds to each spanned gutter when estimating
	// an item's size on the other axis.
	adjustment := gridAlignmentGutterAdjustment(otherAxisAlignment, innerNodeSize.getAbstract(axis.other()),
		estimate, otherAxisTracks)
	if len(otherAxisTracks) > 3 {
		for i := 2; i < len(otherAxisTracks); i += 2 {
			otherAxisTracks[i].contentAlignmentAdjustment = adjustment
		}
	}

	// 11.5 Resolve Intrinsic Track Sizes.
	resolveIntrinsicTrackSizes(axis, axisTracks, otherAxisTracks, items, availableGridSpace.getAbstract(axis),
		innerNodeSize, estimate)

	// 11.6. Maximise Tracks: distribute free space (if any) to tracks with
	// finite growth limits, up to their limits.
	maximiseTracks(axisTracks, availableGridSpace.getAbstract(axis))

	// The last two expansion steps (Expand Flexible Tracks and Stretch auto
	// Tracks) only expand into space the grid container's size creates (from
	// its preferred size, or from its parent through stretch alignment), not
	// into any available space. So definite available space counts as
	// max-content when the inner size is indefinite. A min-content constraint
	// on the block axis counts as max-content too, as a grid container's
	// min-content block size is its max-content block size.
	var expansionSpace AvailableSpace
	switch {
	case innerNodeSize.getAbstract(axis).Valid:
		expansionSpace = Definite(innerNodeSize.getAbstract(axis).Value)
	case availableGridSpace.getAbstract(axis).isMinContent() && axis == Inline:
		expansionSpace = MinContentSpace
	default:
		expansionSpace = MaxContentSpace
	}

	// 11.7. Expand Flexible Tracks: size flexible tracks using the largest fr
	// that does not exceed the available space.
	expandFlexibleTracks(axis, axisTracks, otherAxisTracks, items, axisMinSize, axisMaxSize, expansionSpace,
		innerNodeSize, estimate)

	// 11.8. Stretch auto Tracks: divide remaining positive, definite free
	// space equally among tracks with an auto max track sizing function.
	if axisAlignment == contentOf(ContentStretch) {
		stretchAutoTracks(axisTracks, axisMinSize, expansionSpace)
	}
}

func flushPlannedBaseSizeIncreases(tracks []gridTrack) {
	for i := range tracks {
		tracks[i].baseSize += tracks[i].baseSizePlannedIncrease
		tracks[i].baseSizePlannedIncrease = 0
	}
}

func flushPlannedGrowthLimitIncreases(tracks []gridTrack, setInfinitelyGrowable bool) {
	for i := range tracks {
		t := &tracks[i]
		if t.growthLimitPlannedIncrease > 0 {
			if t.growthLimit == inf32 {
				t.growthLimit = t.baseSize + t.growthLimitPlannedIncrease
			} else {
				t.growthLimit = t.growthLimit + t.growthLimitPlannedIncrease
			}
			t.infinitelyGrowable = setInfinitelyGrowable
		} else {
			t.infinitelyGrowable = false
		}
		t.growthLimitPlannedIncrease = 0
	}
}

func initializeTrackSizes(tracks []gridTrack, axisInnerNodeSize Opt) {
	for i := range tracks {
		t := &tracks[i]
		// A fixed min track sizing function gives the initial base size; an
		// intrinsic one gives 0. (Indefinite lengths cannot occur: they are
		// treated as auto.)
		t.baseSize = t.min.definiteValue(axisInnerNodeSize).Or(0)
		// A fixed max track sizing function gives the initial growth limit;
		// an intrinsic or flexible one gives infinity.
		t.growthLimit = t.max.definiteValue(axisInnerNodeSize).Or(inf32)
		// In all cases, the growth limit is at least the base size.
		if t.growthLimit < t.baseSize {
			t.growthLimit = t.baseSize
		}
	}
}

// resolveItemBaselines computes the baseline shims of baseline-aligned
// items, row by row.
func resolveItemBaselines(axis AbstractAxis, items []gridItem, innerNodeSize Size[Opt]) {
	// Sort the items by their start in the other axis (row) so that items in
	// the same row can be visited together.
	other := axis.other()
	sorted := make([]*gridItem, len(items))
	for i := range items {
		sorted[i] = &items[i]
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].placement(other).Start < sorted[j].placement(other).Start })

	for len(sorted) > 0 {
		currentRow := sorted[0].placement(other).Start
		n := len(sorted)
		for i, it := range sorted {
			if it.placement(other).Start != currentRow {
				n = i
				break
			}
		}
		rowItems := sorted[:n]
		sorted = sorted[n:]

		// With one or no baseline-aligned item in the row, baseline alignment
		// does nothing.
		count := 0
		for _, it := range rowItems {
			if it.participatesInBaselineAlignment() {
				count++
			}
		}
		if count <= 1 {
			continue
		}

		// Compute the baselines of the row's baseline-aligned items.
		for _, it := range rowItems {
			if !it.participatesInBaselineAlignment() {
				continue
			}
			out := performChildLayout(it.node, noneSize, innerNodeSize, minContentSpaces, inherentSize, Line[bool]{})
			height := out.size.Height
			// A scroll container's baseline comes from its content as if
			// scrolled to the start, clamped to its border box
			// (https://github.com/w3c/csswg-drafts/issues/7660).
			baseline := out.baselines.first.Or(height)
			if it.overflow.Y.isScrollContainer() {
				baseline = fmax(fmin(baseline, height), 0)
			}
			it.baseline = Some(baseline + it.margin.Top.resolveOrZero(innerNodeSize.Width))
		}

		// The row's max baseline.
		var rowMax float32
		first := true
		for _, it := range rowItems {
			if it.participatesInBaselineAlignment() {
				b := it.baseline.Or(0)
				if first || b > rowMax {
					rowMax = b
				}
				first = false
			}
		}

		// The baseline shim of each baseline-aligned item.
		for _, it := range rowItems {
			if it.participatesInBaselineAlignment() {
				it.baselineShim = rowMax - it.baseline.Or(0)
				it.hasKnownDimensionsCache = false
			}
		}
	}
}

type intrinsicContributionType uint8

const (
	contributionMinimum intrinsicContributionType = iota
	contributionMaximum
)

// resolveIntrinsicTrackSizes is 11.5 Resolve Intrinsic Track Sizes.
func resolveIntrinsicTrackSizes(axis AbstractAxis, axisTracks, otherAxisTracks []gridTrack, items []gridItem,
	axisAvailableGridSpace AvailableSpace, innerNodeSize Size[Opt], estimate gridTrackEstimate) {
	// Step 1. Shim baseline-aligned items so that their intrinsic size
	// contributions reflect their baseline alignment: done already, see
	// resolveItemBaselines.

	// Step 2.

	// The items are visited in ascending order of the number of tracks they
	// span. Items crossing a flexible track are all handled in one batch
	// (whatever their span), so they only move to the end of the list; and
	// the order within a batch does not change the result, so the others are
	// sorted only if any of them spans several tracks.
	sorted := make([]*gridItem, len(items))
	front, back := 0, len(items)
	needsSort := false
	for i := range items {
		it := &items[i]
		if it.crossesFlexibleTrack(axis) {
			back--
			sorted[back] = it
		} else {
			needsSort = needsSort || it.span(axis) > 1
			sorted[front] = it
			front++
		}
	}
	if needsSort {
		nonFlex := sorted[:front]
		sort.SliceStable(nonFlex, func(i, j int) bool { return nonFlex[i].span(axis) < nonFlex[j].span(axis) })
	}

	axisInnerNodeSize := innerNodeSize.getAbstract(axis)
	sizer := gridIntrinsicSizer{otherAxisTracks: otherAxisTracks, axis: axis, innerNodeSize: innerNodeSize, estimate: estimate}

	// Steps 2, 3 and 4.
	// 2 & 3. Visit the items that don't cross a flexible track, in ascending
	// order of span. Step 2, the span-1 case, has an optimised implementation.
	// 4. Then repeat the previous step for all the items that span a flexible
	// track, together rather than grouped by span.
	//
	// Note: for items with a min size of auto (the initial value), the
	// minimum contribution is usually the min-content contribution, but can
	// differ (see §6.6 Automatic Minimum Size of Grid Items). Also, minimum
	// contribution <= min-content contribution <= max-content contribution.
	batcher := gridItemBatcher{axis: axis, currentSpan: 1}
	for {
		batch, isFlex, ok := batcher.next(sorted)
		if !ok {
			break
		}
		// 2. Size tracks to fit non-spanning items: for each track with an
		// intrinsic, non-flexible track sizing function, consider its items
		// with a span of 1.
		if !isFlex && ozSpan(batch[0].placement(axis)) == 1 {
			resolveSpanOneItems(&sizer, batch, axis, axisTracks, axisAvailableGridSpace, axisInnerNodeSize)
			continue
		}

		// 1. For intrinsic minimums: first increase the base size of tracks
		// with an intrinsic min track sizing function...
		for _, item := range batch {
			if !item.crossesIntrinsicTrack(axis) {
				continue
			}
			// ...by distributing extra space as needed to accommodate these
			// items' minimum contributions.
			//
			// QUIRK: the spec says to use the items' limited min-content
			// contributions in place of their minimum contributions when the
			// grid container is sized under a min- or max-content constraint.
			// But browsers only do so for items that are not scroll containers
			// (overflow: hidden counts), whose automatic minimum size (zero)
			// takes precedence over their min-content contributions.
			var space float32
			if !axisAvailableGridSpace.isDefinite() && !item.overflowOn(axis).isScrollContainer() {
				minimum := sizer.minimumContribution(item, axisTracks)
				minContent := sizer.minContentContribution(item, axisTracks)
				limit := item.spannedTrackLimit(axis, axisTracks, axisInnerNodeSize)
				limitedMinContent := fmax(fminOpt(minContent, limit), minimum)

				// For items crossing flexible tracks, browsers give the content
				// contribution to the flexible tracks only in proportion to the
				// crossed flex factor sum, clamped at one (CSS Grid 2, 12.5: "if
				// the sum is less than one, distribute that proportion of
				// space"). The proportion applies to the space left after the
				// spanned inflexible tracks, and the minimum contribution is a
				// floor: a definite size still spreads over 0fr tracks in full.
				// This lets a 0fr track holding a min-height: 0 item collapse to
				// zero, as the 0fr to 1fr collapse animation pattern relies on.
				if isFlex {
					spanned := item.spannedTracks(axis, axisTracks)
					inflexible := inflexibleTrackSizes(spanned)
					scale := fmin(crossedFlexFactorSum(spanned), 1)
					excess := fmax(limitedMinContent-inflexible, 0)
					space = fmax(minimum, inflexible+float32(excess*scale))
				} else {
					space = limitedMinContent
				}
			} else {
				space = sizer.minimumContribution(item, axisTracks)
			}
			tracks := item.spannedTracks(axis, axisTracks)
			if space > 0 {
				hasIntrinsicMin := func(t *gridTrack) bool { return !t.min.definiteValue(axisInnerNodeSize).Valid }
				if item.overflowOn(axis).isScrollContainer() {
					distributeItemSpaceToBaseSize(isFlex, space, tracks, hasIntrinsicMin,
						(*gridTrack).fitContentLimitedGrowthLimit, contributionMinimum)
				} else {
					distributeItemSpaceToBaseSize(isFlex, space, tracks, hasIntrinsicMin, trackGrowthLimit,
						contributionMinimum)
				}
			}
		}
		flushPlannedBaseSizeIncreases(axisTracks)

		// 2. For content-based minimums: next increase the base size of tracks
		// with a min track sizing function of min-content or max-content by
		// distributing extra space as needed for these items' min-content
		// contributions.
		hasMinOrMaxContentMin := func(t *gridTrack) bool { return t.min.isMinOrMaxContent() }
		for _, item := range batch {
			if !item.spansTrackMatching(axis, axisTracks, hasMinOrMaxContentMin) {
				continue
			}
			space := sizer.minContentContribution(item, axisTracks)
			tracks := item.spannedTracks(axis, axisTracks)
			if space > 0 {
				if item.overflowOn(axis).isScrollContainer() {
					distributeItemSpaceToBaseSize(isFlex, space, tracks, hasMinOrMaxContentMin,
						(*gridTrack).fitContentLimitedGrowthLimit, contributionMinimum)
				} else {
					distributeItemSpaceToBaseSize(isFlex, space, tracks, hasMinOrMaxContentMin, trackGrowthLimit,
						contributionMinimum)
				}
			}
		}
		flushPlannedBaseSizeIncreases(axisTracks)

		// 3. For max-content minimums: if the grid container is sized under a
		// max-content constraint, continue to increase the base size of tracks
		// with a min track sizing function of auto or max-content by
		// distributing extra space as needed for these items' limited
		// max-content contributions.
		if axisAvailableGridSpace.isMaxContent() {
			hasAutoMin := func(t *gridTrack) bool { return t.min.IsAuto() && !t.max.isMinContent() }
			hasMaxContentMin := func(t *gridTrack) bool { return t.min.isMaxContent() }
			for _, item := range batch {
				if !item.spansTrackMatching(axis, axisTracks, func(t *gridTrack) bool {
					return hasAutoMin(t) || hasMaxContentMin(t)
				}) {
					continue
				}
				maxContent := sizer.maxContentContribution(item, axisTracks)
				limit := item.spannedTrackLimit(axis, axisTracks, axisInnerNodeSize)
				space := fminOpt(maxContent, limit)

				// As for intrinsic minimums above: scale the space beyond the
				// spanned inflexible tracks by the crossed flex factor sum,
				// clamped at one. Anchoring at the inflexible track sizes rather
				// than the current base sizes keeps this pass from compounding
				// with the scaling already applied to the min-content
				// contribution.
				if isFlex {
					spanned := item.spannedTracks(axis, axisTracks)
					inflexible := inflexibleTrackSizes(spanned)
					scale := fmin(crossedFlexFactorSum(spanned), 1)
					space = inflexible + float32(fmax(space-inflexible, 0)*scale)
				}
				tracks := item.spannedTracks(axis, axisTracks)
				if space > 0 {
					// If any spanned track has a max-content min track sizing
					// function, distribute space only to those; otherwise to
					// those with an auto one.
					//
					// This prioritisation of max-content over auto is not in the
					// spec (which suggests distributing evenly between them), but
					// Chrome and Firefox both do it, and it has a certain logic.
					// https://www.w3.org/TR/css-grid-1/#track-size-max-content-min
					anyMaxContent := false
					for i := range tracks {
						if hasMaxContentMin(&tracks[i]) {
							anyMaxContent = true
							break
						}
					}
					if anyMaxContent {
						distributeItemSpaceToBaseSize(isFlex, space, tracks, hasMaxContentMin,
							func(*gridTrack) float32 { return inf32 }, contributionMaximum)
					} else {
						distributeItemSpaceToBaseSize(isFlex, space, tracks, hasAutoMin,
							(*gridTrack).fitContentLimitedGrowthLimit, contributionMaximum)
					}
				}
			}
			flushPlannedBaseSizeIncreases(axisTracks)
		}

		// In all cases, continue to increase the base size of tracks with a
		// max-content min track sizing function by distributing extra space as
		// needed for these items' max-content contributions.
		hasMaxContentMin := func(t *gridTrack) bool { return t.min.isMaxContent() }
		for _, item := range batch {
			if !item.spansTrackMatching(axis, axisTracks, hasMaxContentMin) {
				continue
			}
			space := sizer.maxContentContribution(item, axisTracks)
			tracks := item.spannedTracks(axis, axisTracks)
			if space > 0 {
				distributeItemSpaceToBaseSize(isFlex, space, tracks, hasMaxContentMin, trackGrowthLimit,
					contributionMaximum)
			}
		}
		flushPlannedBaseSizeIncreases(axisTracks)

		// 4. If a track's growth limit is now less than its base size,
		// increase its growth limit to match its base size.
		for i := range axisTracks {
			if axisTracks[i].growthLimit < axisTracks[i].baseSize {
				axisTracks[i].growthLimit = axisTracks[i].baseSize
			}
		}

		// A flexible track has a flexible max track sizing function, which
		// cannot also be intrinsic, so these steps do not apply.
		if isFlex {
			continue
		}

		// 5. For intrinsic maximums: next increase the growth limit of tracks
		// with an intrinsic max track sizing function by distributing extra
		// space as needed for these items' min-content contributions.
		hasIntrinsicMax := func(t *gridTrack) bool { return !t.max.hasDefiniteValue(axisInnerNodeSize) }
		for _, item := range batch {
			if !item.spansTrackMatching(axis, axisTracks, hasIntrinsicMax) {
				continue
			}
			space := sizer.minContentContribution(item, axisTracks)
			tracks := item.spannedTracks(axis, axisTracks)
			if space > 0 {
				distributeItemSpaceToGrowthLimit(space, tracks, hasIntrinsicMax)
			}
		}
		// Mark tracks whose growth limit changed from infinite to finite in
		// this step as infinitely growable for the next step.
		flushPlannedGrowthLimitIncreases(axisTracks, true)

		// 6. For max-content maximums: lastly continue to increase the growth
		// limit of tracks with a max-content max track sizing function by
		// distributing extra space as needed for these items' max-content
		// contributions, limiting the growth of fit-content() tracks by their
		// argument.
		hasMaxContentMax := func(t *gridTrack) bool {
			return t.max.isMaxContentAlike() || (t.max.usesPercentage() && !axisInnerNodeSize.Valid)
		}
		for _, item := range batch {
			if !item.spansTrackMatching(axis, axisTracks, hasMaxContentMax) {
				continue
			}
			space := sizer.maxContentContribution(item, axisTracks)
			tracks := item.spannedTracks(axis, axisTracks)
			if space > 0 {
				distributeItemSpaceToGrowthLimit(space, tracks, hasMaxContentMax)
			}
		}
		flushPlannedGrowthLimitIncreases(axisTracks, false)
	}

	// Step 5. A track whose growth limit is still infinite (having no items,
	// or being flexible) gets its base size as growth limit. This keeps
	// Maximise Tracks from affecting flexible tracks.
	for i := range axisTracks {
		if axisTracks[i].growthLimit == inf32 {
			axisTracks[i].growthLimit = axisTracks[i].baseSize
		}
	}
}

func trackGrowthLimit(t *gridTrack) float32 { return t.growthLimit }

func inflexibleTrackSizes(tracks []gridTrack) float32 {
	var sum float32
	for i := range tracks {
		if !tracks[i].isFlexible() {
			sum += tracks[i].baseSize
		}
	}
	return sum
}

func crossedFlexFactorSum(tracks []gridTrack) float32 {
	var sum float32
	for i := range tracks {
		if tracks[i].isFlexible() {
			sum += tracks[i].flexFactor()
		}
	}
	return sum
}

// resolveSpanOneItems is step 2 for items spanning one track.
func resolveSpanOneItems(sizer *gridIntrinsicSizer, batch []*gridItem, axis AbstractAxis, axisTracks []gridTrack,
	axisAvailableGridSpace AvailableSpace, axisInnerNodeSize Opt) {
	for _, item := range batch {
		trackIndex := int(item.placementIndexes(axis).Start) + 1
		track := &axisTracks[trackIndex]

		// Base sizes.
		var newBaseSize float32
		switch {
		case track.min.isMinContent():
			newBaseSize = fmax(track.baseSize, sizer.minContentContribution(item, axisTracks))
		case track.min.isMaxContent():
			newBaseSize = fmax(track.baseSize, sizer.maxContentContribution(item, axisTracks))
		case track.min.behavesAsAuto(axisInnerNodeSize):
			// A percentage track with an indefinite, unresolved container size
			// is treated as auto.
			var space float32
			// QUIRK: see the intrinsic minimums step: browsers use limited
			// min-content contributions under a min- or max-content constraint
			// only for items that are not scroll containers.
			if !axisAvailableGridSpace.isDefinite() && !item.overflowOn(axis).isScrollContainer() {
				minimum := sizer.minimumContribution(item, axisTracks)
				minContent := sizer.minContentContribution(item, axisTracks)
				limit := track.max.definiteLimit(axisInnerNodeSize)
				space = fmax(fminOpt(minContent, limit), minimum)
			} else {
				space = sizer.minimumContribution(item, axisTracks)
			}
			newBaseSize = fmax(track.baseSize, space)
		default:
			// Not an intrinsic track sizing function.
			newBaseSize = track.baseSize
		}
		track.baseSize = newBaseSize

		// Growth limits.
		//
		// Contributions call into the child, so they are computed only when
		// used. The min-content contribution is computed for every item in an
		// intrinsic track (even where only the max-content contribution sets
		// the growth limit), as the check for re-running track sizing compares
		// each such item's min-content contribution with the one cached here.
		maxFn := track.max
		fitContentLimit := track.fitContentLimit
		var minContent Opt
		if track.hasIntrinsicSizingFunction() {
			minContent = Some(sizer.minContentContribution(item, axisTracks))
		}
		var planned Opt
		switch {
		case maxFn.isFitContent():
			// The growth limit is at least the fit-content limited max-content
			// contribution...
			maxContent := fmin(sizer.maxContentContribution(item, axisTracks), fitContentLimit)
			// ...and, if the item is not a scroll container, its min-content
			// contribution.
			if minContent.Valid && !item.overflowOn(axis).isScrollContainer() {
				planned = Some(fmax(minContent.Value, maxContent))
			} else {
				planned = Some(maxContent)
			}
		case maxFn.isMaxContentAlike() || (maxFn.usesPercentage() && !axisInnerNodeSize.Valid):
			// A percentage track with an indefinite, unresolved container size
			// is treated as auto (as Chrome does).
			planned = Some(sizer.maxContentContribution(item, axisTracks))
		case maxFn.isIntrinsic():
			planned = minContent
		}
		if planned.Valid {
			track.growthLimitPlannedIncrease = fmax(track.growthLimitPlannedIncrease, planned.Value)
		}
	}

	for i := range axisTracks {
		t := &axisTracks[i]
		if t.growthLimitPlannedIncrease > 0 {
			if t.growthLimit == inf32 {
				t.growthLimit = t.growthLimitPlannedIncrease
			} else {
				t.growthLimit = fmax(t.growthLimit, t.growthLimitPlannedIncrease)
			}
		}
		t.infinitelyGrowable = false
		t.growthLimitPlannedIncrease = 0
		if t.growthLimit < t.baseSize {
			t.growthLimit = t.baseSize
		}
	}
}

func distributeItemSpaceToBaseSize(isFlex bool, space float32, tracks []gridTrack, affected func(*gridTrack) bool,
	limit func(*gridTrack) float32, contribution intrinsicContributionType) {
	if !isFlex {
		distributeItemSpaceToBaseSizeInner(space, tracks, affected, oneProportion, limit, contribution)
		return
	}
	filter := func(t *gridTrack) bool { return t.isFlexible() && affected(t) }

	// If the flex factor sum of the affected tracks is positive, distribute
	// space in proportion to their flex factors; otherwise equally.
	//
	// The spec says to sum over all the flexible tracks the item spans, and
	// to blend proportional and equal distribution when the sum is below one.
	// But Chrome sums over the affected tracks only and uses pure flex factor
	// ratios whenever that sum is positive, so this does the same.
	var flexSum float32
	for i := range tracks {
		if filter(&tracks[i]) {
			flexSum += tracks[i].flexFactor()
		}
	}
	if flexSum > 0 {
		distributeItemSpaceToBaseSizeInner(space, tracks, filter, (*gridTrack).flexFactor, limit, contribution)
	} else {
		distributeItemSpaceToBaseSizeInner(space, tracks, filter, oneProportion, limit, contribution)
	}
}

func oneProportion(*gridTrack) float32 { return 1 }

func trackBaseSize(t *gridTrack) float32 { return t.baseSize }

func distributeItemSpaceToBaseSizeInner(space float32, tracks []gridTrack, affected func(*gridTrack) bool,
	proportion func(*gridTrack) float32, limit func(*gridTrack) float32, contribution intrinsicContributionType) {
	// Skip the distribution with no space, or no affected tracks.
	if space == 0 {
		return
	}
	anyAffected := false
	for i := range tracks {
		if affected(&tracks[i]) {
			anyAffected = true
			break
		}
	}
	if !anyAffected {
		return
	}

	// 1. Find the space to distribute.
	var trackSizes float32
	for i := range tracks {
		trackSizes += tracks[i].baseSize
	}
	extraSpace := fmax(0, space-trackSizes)

	// 2. Distribute space up to limits. The loop ends when the space runs out
	// (falls below the threshold) or the growable tracks do.
	const threshold = 0.000001
	extraSpace = distributeSpaceUpToLimits(extraSpace, tracks, affected, proportion, trackBaseSize, limit)

	// 3. Distribute the remaining space beyond limits (if any).
	if extraSpace > threshold {
		// When accommodating minimum or min-content contributions: to affected
		// tracks that also have an intrinsic max track sizing function. When
		// accommodating max-content contributions: to affected tracks that
		// also have a max-content max track sizing function.
		filter := func(t *gridTrack) bool { return t.max.isIntrinsic() }
		if contribution == contributionMaximum {
			filter = func(t *gridTrack) bool { return t.max.isMaxOrFitContent() }
		}

		// If there are no such tracks, use all affected tracks.
		count := 0
		for i := range tracks {
			if affected(&tracks[i]) && filter(&tracks[i]) {
				count++
			}
		}
		if count == 0 {
			filter = func(*gridTrack) bool { return true }
		}

		// Beyond limits, growth limits are ignored, but a fit-content()
		// argument still caps growth.
		distributeSpaceUpToLimits(extraSpace, tracks, func(t *gridTrack) bool { return affected(t) && filter(t) },
			proportion, trackBaseSize, func(t *gridTrack) float32 { return t.fitContentLimit })
	}

	// 4. For each affected track, a larger item-incurred increase than its
	// planned increase becomes its planned increase.
	for i := range tracks {
		t := &tracks[i]
		if t.itemIncurredIncrease > t.baseSizePlannedIncrease {
			t.baseSizePlannedIncrease = t.itemIncurredIncrease
		}
		// Reset the item-incurred increase for the next distribution.
		t.itemIncurredIncrease = 0
	}
}

func growthLimitOrBaseSize(t *gridTrack) float32 {
	if t.growthLimit == inf32 {
		return t.baseSize
	}
	return t.growthLimit
}

func distributeItemSpaceToGrowthLimit(space float32, tracks []gridTrack, affected func(*gridTrack) bool) {
	// Skip the distribution with no space, or no affected tracks.
	if space == 0 {
		return
	}
	anyAffected := false
	for i := range tracks {
		if affected(&tracks[i]) {
			anyAffected = true
			break
		}
	}
	if !anyAffected {
		return
	}

	// 1. Find the space to distribute.
	var trackSizes float32
	for i := range tracks {
		trackSizes += growthLimitOrBaseSize(&tracks[i])
	}
	extraSpace := fmax(0, space-trackSizes)

	// 2. Distribute space up to limits. For growth limits the limit is the
	// growth limit itself unless the track is infinitely growable (or its
	// growth limit still infinite), in which case it is its fit-content()
	// argument, or infinity without one. As the affected size of a track that
	// is not growable is its growth limit, such tracks get no space here.
	fitContentLimit := func(t *gridTrack) float32 { return t.fitContentLimit }
	extraSpace = distributeSpaceUpToLimits(extraSpace, tracks, func(t *gridTrack) bool {
		return affected(t) && (t.infinitelyGrowable || t.growthLimit == inf32)
	}, oneProportion, growthLimitOrBaseSize, fitContentLimit)

	// 3. Distribute space beyond limits: if space remains after all tracks
	// are frozen, unfreeze and keep distributing to the item-incurred
	// increase of all affected tracks.
	if extraSpace > 0 {
		distributeSpaceUpToLimits(extraSpace, tracks, affected, oneProportion, growthLimitOrBaseSize, fitContentLimit)
	}

	// 4. For each affected track, a larger item-incurred increase than its
	// planned increase becomes its planned increase.
	for i := range tracks {
		t := &tracks[i]
		if t.itemIncurredIncrease > t.growthLimitPlannedIncrease {
			t.growthLimitPlannedIncrease = t.itemIncurredIncrease
		}
		t.itemIncurredIncrease = 0
	}
}

// maximiseTracks is 11.6 Maximise Tracks.
func maximiseTracks(tracks []gridTrack, available AvailableSpace) {
	var used float32
	for i := range tracks {
		used += tracks[i].baseSize
	}
	free := available.computeFreeSpace(used)
	switch {
	case free == inf32:
		for i := range tracks {
			tracks[i].baseSize = tracks[i].growthLimit
		}
	case free > 0:
		distributeSpaceUpToLimits(free, tracks, func(*gridTrack) bool { return true }, oneProportion, trackBaseSize,
			(*gridTrack).fitContentLimitedGrowthLimit)
		for i := range tracks {
			tracks[i].baseSize += tracks[i].itemIncurredIncrease
			tracks[i].itemIncurredIncrease = 0
		}
	}
}

// expandFlexibleTracks is 11.7 Expand Flexible Tracks.
func expandFlexibleTracks(axis AbstractAxis, axisTracks, otherAxisTracks []gridTrack, items []gridItem,
	axisMinSize, axisMaxSize Opt, expansionSpace AvailableSpace, innerNodeSize Size[Opt],
	estimate gridTrackEstimate) {
	sizer := gridIntrinsicSizer{otherAxisTracks: otherAxisTracks, axis: axis, innerNodeSize: innerNodeSize, estimate: estimate}

	// First, find the grid's used flex fraction.
	var flexFraction float32
	switch expansionSpace.Kind {
	case SpaceDefinite:
		// If the free space is zero, the used flex fraction is zero.
		// Otherwise, if the free space is definite, it is the size of an fr
		// using all the grid tracks and a space to fill of the available grid
		// space.
		var used float32
		for i := range axisTracks {
			used += axisTracks[i].baseSize
		}
		if free := expansionSpace.Value - used; !(free <= 0) {
			flexFraction = findSizeOfFr(axisTracks, expansionSpace.Value)
		}
	case SpaceMinContent:
		// Under a min-content constraint the used flex fraction is zero.
	case SpaceMaxContent:
		// Otherwise, if the free space is indefinite, the used flex fraction
		// is the maximum of:
		//   - for each flexible track, its base size divided by its flex
		//     factor if that is more than one, its base size otherwise;
		var fromTracks float32
		seen := false
		for i := range axisTracks {
			t := &axisTracks[i]
			if !t.max.isFr() {
				continue
			}
			v := t.baseSize
			if f := t.flexFactor(); f > 1 {
				v = t.baseSize / f
			}
			if !seen {
				fromTracks, seen = v, true
			} else {
				fromTracks = max32Total(fromTracks, v)
			}
		}
		//   - for each grid item crossing a flexible track, the size of an fr
		//     using the tracks it crosses and a space to fill of its
		//     max-content contribution.
		var fromItems float32
		seen = false
		for i := range items {
			item := &items[i]
			if !item.crossesFlexibleTrack(axis) {
				continue
			}
			maxContent := sizer.maxContentContribution(item, axisTracks)
			v := findSizeOfFr(item.spannedTracks(axis, axisTracks), maxContent)
			if !seen {
				fromItems, seen = v, true
			} else {
				fromItems = max32Total(fromItems, v)
			}
		}
		flexFraction = fmax(fromTracks, fromItems)

		// If this flex fraction would make the grid smaller than the
		// container's min size (or larger than its max size), redo this step
		// with a definite free space and an available grid space of the inner
		// size at the min (max) size. The min size takes precedence.
		var hypothetical float32
		for i := range axisTracks {
			t := &axisTracks[i]
			if t.max.isFr() {
				hypothetical += fmax(t.baseSize, float32(t.max.Value*flexFraction))
			} else {
				hypothetical += t.baseSize
			}
		}
		minSize := axisMinSize.Or(0)
		maxSize := axisMaxSize.Or(inf32)
		if hypothetical < minSize {
			flexFraction = findSizeOfFr(axisTracks, minSize)
		} else if hypothetical > maxSize {
			flexFraction = findSizeOfFr(axisTracks, maxSize)
		}
	}

	// A flexible track whose flex factor times the flex fraction exceeds its
	// base size gets that product as its base size.
	for i := range axisTracks {
		t := &axisTracks[i]
		if t.max.isFr() {
			t.baseSize = fmax(t.baseSize, float32(t.max.Value*flexFraction))
		}
	}
}

// max32Total is the larger of a and b as Rust's f32::total_cmp orders
// values (NaN being the largest), for Taffy's max_by(total_cmp).
func max32Total(a, b float32) float32 {
	if b > a || b != b {
		return b
	}
	return a
}

// findSizeOfFr finds the size of an fr
// (https://www.w3.org/TR/css-grid-1/#algo-find-fr-size).
func findSizeOfFr(tracks []gridTrack, spaceToFill float32) float32 {
	// With no space to fill there is nothing to do (and the loop below would
	// not end).
	if spaceToFill == 0 {
		return 0
	}

	// If the hypothetical fr size times a flexible track's flex factor is
	// less than its base size, the algorithm restarts treating such tracks
	// as inflexible. It runs in a loop, starting with an infinite
	// hypothetical fr size so that this cannot happen on the first pass.
	hypothetical := inf32
	var previous float32
	// Every restart treats at least one more track as inflexible, so a valid
	// size is found within len(tracks)+1 iterations; non-finite inputs break
	// that (comparisons with NaN are false), so the count is bounded.
	for range len(tracks) + 1 {
		// The leftover space is the space to fill minus the base sizes of the
		// non-flexible tracks; the flex factor sum, at least 1, that of the
		// flexible ones.
		var used, flexSum float32
		for i := range tracks {
			t := &tracks[i]
			if t.max.isFr() && float32(t.max.Value*hypothetical) >= t.baseSize {
				flexSum += t.max.Value
			} else {
				used += t.baseSize
			}
		}
		leftover := spaceToFill - used
		flexFactor := fmax(flexSum, 1)

		// The hypothetical fr size is the leftover space divided by the flex
		// factor sum.
		previous = hypothetical
		hypothetical = leftover / flexFactor

		// If the hypothetical fr size times a flexible track's flex factor is
		// less than its base size, restart treating such tracks as
		// inflexible.
		valid := true
		for i := range tracks {
			t := &tracks[i]
			if !t.max.isFr() {
				continue
			}
			f := t.max.Value
			if !(float32(f*hypothetical) >= t.baseSize || float32(f*previous) < t.baseSize) {
				valid = false
				break
			}
		}
		if valid {
			break
		}
	}
	return hypothetical
}

// stretchAutoTracks is 11.8 Stretch auto Tracks.
func stretchAutoTracks(tracks []gridTrack, axisMinSize Opt, expansionSpace AvailableSpace) {
	numAuto := 0
	for i := range tracks {
		if tracks[i].max.IsAuto() {
			numAuto++
		}
	}
	if numAuto == 0 {
		return
	}
	var used float32
	for i := range tracks {
		used += tracks[i].baseSize
	}
	// With indefinite free space but a definite min size, the min size gives
	// the free space for this step.
	var free float32
	if expansionSpace.isDefinite() {
		free = expansionSpace.computeFreeSpace(used)
	} else if axisMinSize.Valid {
		free = axisMinSize.Value - used
	}
	if free > 0 {
		per := free / float32(numAuto)
		for i := range tracks {
			if tracks[i].max.IsAuto() {
				tracks[i].baseSize += per
			}
		}
	}
}

// distributeSpaceUpToLimits distributes space to the affected tracks in
// proportion, up to their limits, as item-incurred increases. It returns
// the space left.
func distributeSpaceUpToLimits(space float32, tracks []gridTrack, affected func(*gridTrack) bool,
	proportion func(*gridTrack) float32, property func(*gridTrack) float32, limit func(*gridTrack) float32) float32 {
	const threshold = 0.01

	// Each iteration freezes at least one track at its limit, so the loop
	// ends within len(tracks)+1 iterations; non-finite values can keep space
	// from being distributed, so the count is bounded.
	for range len(tracks) + 1 {
		if space <= threshold {
			break
		}
		var proportionSum float32
		for i := range tracks {
			t := &tracks[i]
			if property(t)+t.itemIncurredIncrease < limit(t) && affected(t) {
				proportionSum += proportion(t)
			}
		}
		if proportionSum == 0 {
			break
		}

		// The item-incurred increase for this iteration.
		var minIncreaseLimit float32
		first := true
		for i := range tracks {
			t := &tracks[i]
			if property(t)+t.itemIncurredIncrease < limit(t) && affected(t) {
				v := (limit(t) - property(t) - t.itemIncurredIncrease) / proportion(t)
				if first || min32Total(v, minIncreaseLimit) {
					minIncreaseLimit = v
				}
				first = false
			}
		}
		increasePerProportion := fmin(minIncreaseLimit, space/proportionSum)

		for i := range tracks {
			t := &tracks[i]
			if !affected(t) {
				continue
			}
			increase := float32(increasePerProportion * proportion(t))
			if increase > 0 && property(t)+t.itemIncurredIncrease+increase <= limit(t)+threshold {
				t.itemIncurredIncrease += increase
				space -= increase
			}
		}
	}
	return space
}

// min32Total reports whether v is less than current as Rust's
// f32::total_cmp orders values, for Taffy's min_by(total_cmp).
func min32Total(v, current float32) bool {
	if v != v {
		return false
	}
	if current != current {
		return true
	}
	return v < current
}
