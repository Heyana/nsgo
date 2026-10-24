package layout

// Placing items in the grid and resolving the implicit grid
// (https://www.w3.org/TR/css-grid-1/#placement).

// gridItemPlacement is an item's placement in both axes, in origin-zero
// coordinates.
type gridItemPlacement struct {
	horizontal, vertical Line[ozPlacement]
}

func (p gridItemPlacement) get(axis AbsoluteAxis) Line[ozPlacement] {
	if axis == Horizontal {
		return p.horizontal
	}
	return p.vertical
}

// gridSizeEstimate estimates the track counts of the implicit grid (the
// whole grid) from the explicit grid and the items' placements, so that the
// cell occupancy matrix rarely has to grow. It is a performance
// optimisation, not needed for correctness.
func gridSizeEstimate(explicitColCount, explicitRowCount uint16, placements []gridItemPlacement) (cols,
	rows trackCounts) {
	// Estimate the min and max lines (in origin-zero coordinates) and the
	// span of each item.
	var colMin, colMax, rowMin, rowMax ozLine
	var colMaxSpan, rowMaxSpan uint16
	for _, p := range placements {
		// Children reference the lines between and around tracks, not tracks
		// themselves, so 1 must be subtracted for a track count estimate.
		cmin, cmax, cspan := childMinLineMaxLineSpan(p.horizontal)
		rmin, rmax, rspan := childMinLineMaxLineSpan(p.vertical)
		colMin, colMax, colMaxSpan = min(colMin, cmin), max(colMax, cmax), max(colMaxSpan, cspan)
		rowMin, rowMax, rowMaxSpan = min(rowMin, rmin), max(rowMax, rmax), max(rowMaxSpan, rspan)
	}

	// Compute track count estimates per axis from the explicit track counts
	// and the min and max lines.
	cols = trackCounts{
		negativeImplicit: colMin.impliedNegativeImplicitTracks(),
		explicit:         explicitColCount,
		positiveImplicit: colMax.impliedPositiveImplicitTracks(explicitColCount),
	}
	rows = trackCounts{
		negativeImplicit: rowMin.impliedNegativeImplicitTracks(),
		explicit:         explicitRowCount,
		positiveImplicit: rowMax.impliedPositiveImplicitTracks(explicitRowCount),
	}

	// Grow the positive estimate on an axis if an item's span does not fit
	// in the estimated number of tracks.
	if cols.negativeImplicit+cols.explicit+cols.positiveImplicit < colMaxSpan {
		cols.positiveImplicit = colMaxSpan - cols.explicit - cols.negativeImplicit
	}
	if rows.negativeImplicit+rows.explicit+rows.positiveImplicit < rowMaxSpan {
		rows.positiveImplicit = rowMaxSpan - rows.explicit - rows.negativeImplicit
	}
	return cols, rows
}

// childMinLineMaxLineSpan returns the lowest and highest lines a placement
// reaches, and its span if it is indefinite.
//
// 8.3.1. Grid Placement Conflict Handling
// A. If the placement for a grid item contains two lines, and the start
// line is further end-ward than the end line, swap the two lines.
// B. If the start line is equal to the end line, remove the end line.
// C. If the placement contains two spans, remove the one contributed by the
// end grid-placement property.
// D. If the placement contains only a span for a named line, replace it with
// a span of 1.
func childMinLineMaxLineSpan(l Line[ozPlacement]) (ozLine, ozLine, uint16) {
	s, e := l.Start, l.End
	var lo, hi ozLine
	span := uint16(1)
	switch {
	case s.kind == PlacementLine && e.kind == PlacementLine:
		// See rules A and B above.
		if s.line == e.line {
			lo, hi = s.line, s.line+1
		} else {
			lo, hi = min(s.line, e.line), max(s.line, e.line)
		}
	case s.kind == PlacementLine && e.kind == PlacementAuto:
		lo, hi = s.line, s.line+1
	case s.kind == PlacementLine:
		lo, hi = s.line, s.line+ozLine(e.span)
	case s.kind == PlacementAuto && e.kind == PlacementLine:
		// An auto start with a definite end is a span of 1 ending at that line.
		lo, hi = e.line-1, e.line
	case e.kind == PlacementLine:
		lo, hi = e.line-ozLine(s.span), e.line
	default:
		// Only spans or autos: they are accounted for separately, by span.
		span = ozIndefiniteSpan(l)
	}

	// Clamp into the limited grid so that the estimated implicit track counts
	// stay within the track limit
	// (https://www.w3.org/TR/css-grid-1/#overlarge-grids), as placement
	// clamps the actual placements.
	return max(lo, minOZLine), min(hi, maxOZLine), span
}

func advancePosition(p ozLine) ozLine {
	if p == 32767 {
		return p
	}
	return p + 1
}

func resolveIndefiniteGridSpan(position ozLine, span uint16) Line[ozLine] {
	clamp := func(v int32) ozLine { return ozLine(min(max(v, -32768), 32767)) }
	return Line[ozLine]{clamp(int32(position)), clamp(int32(position) + int32(span))}
}

// placeGridItems places the items in the grid
// (https://www.w3.org/TR/css-grid-1/#auto-placement-algo).
func placeGridItems(m *cellOccupancyMatrix, items []gridItem, placements []gridItemPlacement, flow GridAutoFlow) {
	primaryAxis := flow.primaryAxis()
	secondaryAxis := primaryAxis.other()

	// 1. Place children with definite positions.
	for i := range items {
		p := placements[i]
		if !(ozIsDefinite(p.horizontal) && ozIsDefinite(p.vertical)) {
			continue
		}
		primarySpan := ozResolveDefiniteGridLines(p.get(primaryAxis))
		secondarySpan := ozResolveDefiniteGridLines(p.get(secondaryAxis))
		recordGridPlacement(m, &items[i], primaryAxis, primarySpan, secondarySpan, cellDefinitelyPlaced)
	}

	// 2. Place remaining children with definite secondary axis positions.
	for i := range items {
		p := placements[i]
		if !(ozIsDefinite(p.get(secondaryAxis)) && !ozIsDefinite(p.get(primaryAxis))) {
			continue
		}
		primarySpan, secondarySpan := placeDefiniteSecondaryAxisItem(m, p, flow)
		recordGridPlacement(m, &items[i], primaryAxis, primarySpan, secondarySpan, cellAutoPlaced)
	}

	// 3. Determine the number of columns in the implicit grid. This is
	// already accounted for: the grid size estimate pre-sizes the matrix with
	// the explicit columns (3.1) and the largest span of items without a
	// definite column (3.3), and marking areas grows it to fit items with a
	// definite column (3.2).

	// 4. Position the remaining grid items (which have a definite position
	// only in the secondary axis or indefinite positions in both axes).
	gridStart := [2]ozLine{
		m.trackCounts(primaryAxis).implicitStartLine(),
		m.trackCounts(secondaryAxis).implicitStartLine(),
	}
	position := gridStart
	for i := range items {
		p := placements[i]
		if ozIsDefinite(p.get(secondaryAxis)) {
			continue
		}
		primarySpan, secondarySpan := placeIndefinitelyPositionedItem(m, p, flow, position)
		recordGridPlacement(m, &items[i], primaryAxis, primarySpan, secondarySpan, cellAutoPlaced)

		// The dense algorithm restarts every search from the grid start;
		// otherwise the next item is placed after this one.
		if flow.isDense() {
			position = gridStart
		} else {
			position = [2]ozLine{primarySpan.End, secondarySpan.Start}
		}
	}
}

func placeDefiniteSecondaryAxisItem(m *cellOccupancyMatrix, p gridItemPlacement, flow GridAutoFlow) (Line[ozLine],
	Line[ozLine]) {
	primaryAxis := flow.primaryAxis()
	secondaryAxis := primaryAxis.other()
	primaryStart := m.trackCounts(primaryAxis).implicitStartLine()

	secondaryPlacement := ozResolveDefiniteGridLines(p.get(secondaryAxis))
	position := primaryStart
	if !flow.isDense() {
		if cursor, ok := m.autoPlacementCursor(primaryAxis, secondaryPlacement.Start); ok {
			position = cursor
		}
	}
	primarySpan := ozIndefiniteSpan(p.get(primaryAxis))

	for {
		primaryPlacement := resolveIndefiniteGridSpan(position, primarySpan)
		next, collides := m.lineAreaCollisionJump(primaryAxis, primaryPlacement, secondaryPlacement)
		if !collides {
			return primaryPlacement, secondaryPlacement
		}
		position = next
	}
}

func placeIndefinitelyPositionedItem(m *cellOccupancyMatrix, p gridItemPlacement, flow GridAutoFlow,
	position [2]ozLine) (Line[ozLine], Line[ozLine]) {
	primaryAxis := flow.primaryAxis()
	secondaryAxis := primaryAxis.other()

	primaryStyle := p.get(primaryAxis)
	secondaryStyle := p.get(secondaryAxis)

	secondarySpan := ozIndefiniteSpan(secondaryStyle)
	primaryStart := m.trackCounts(primaryAxis).implicitStartLine()
	primaryEnd := m.trackCounts(primaryAxis).implicitEndLine()
	secondaryStart := m.trackCounts(secondaryAxis).implicitStartLine()

	primaryIdx, secondaryIdx := position[0], position[1]

	if ozIsDefinite(primaryStyle) {
		primarySpan := ozResolveDefiniteGridLines(primaryStyle)

		// The search on the secondary axis starts at the first track with
		// dense auto-flow.
		if flow.isDense() {
			secondaryIdx = secondaryStart
		} else if primarySpan.Start < primaryIdx {
			secondaryIdx = advancePosition(secondaryIdx)
		}

		// The item has a fixed primary axis position, so increment the
		// secondary axis position until it fits.
		for {
			span := resolveIndefiniteGridSpan(secondaryIdx, secondarySpan)
			if next, collides := m.lineAreaCollisionJump(secondaryAxis, span, primarySpan); collides {
				secondaryIdx = next
				continue
			}
			return primarySpan, span
		}
	}

	primarySpanLen := ozIndefiniteSpan(primaryStyle)

	// Whether the item spans every track on the primary axis. Such an item
	// can only be placed at the primary axis start, in a stripe of entirely
	// unoccupied tracks.
	spansAllPrimaryTracks := int(primarySpanLen) >= m.trackCounts(primaryAxis).len()

	// The item has no fixed axis: search along the primary axis to the end of
	// the existing tracks, then go back to its start and increment the
	// secondary axis index, until the item fits.
	for {
		primarySpan := resolveIndefiniteGridSpan(primaryIdx, primarySpanLen)
		span := resolveIndefiniteGridSpan(secondaryIdx, secondarySpan)

		// Out of bounds on the primary axis: go to the next secondary track.
		if primarySpan.End > primaryEnd {
			// A span out of bounds even at the search start can never fit, as
			// searching only moves it further from the grid start. Bail out
			// and let recordGridPlacement clamp it into the limited grid.
			if primaryIdx == primaryStart {
				return primarySpan, span
			}
			secondaryIdx = advancePosition(secondaryIdx)
			primaryIdx = primaryStart
			continue
		}

		// An item spanning every primary axis track fits only if all the
		// secondary axis tracks it spans are empty: jump past any that are
		// not.
		if spansAllPrimaryTracks {
			if next, occupied := m.occupiedTrackJump(secondaryAxis, span); occupied {
				secondaryIdx = next
				primaryIdx = primaryStart
				continue
			}
			return primarySpan, span
		}

		// If the area is occupied, jump past the collision and try again.
		if next, collides := m.lineAreaCollisionJump(primaryAxis, primarySpan, span); collides {
			primaryIdx = next
			continue
		}
		return primarySpan, span
	}
}

func clampSpanToLimitedGrid(span Line[ozLine]) Line[ozLine] {
	start := min(max(span.Start, minOZLine), maxOZLine-1)
	end := min(max(span.End, start+1), maxOZLine)
	return Line[ozLine]{start, end}
}

func recordGridPlacement(m *cellOccupancyMatrix, item *gridItem, primaryAxis AbsoluteAxis, primarySpan,
	secondarySpan Line[ozLine], state cellState) {
	// Clamp placements into the limited grid to prevent arithmetic overflow
	// when growing the implicit grid
	// (https://www.w3.org/TR/css-grid-1/#overlarge-grids).
	primarySpan = clampSpanToLimitedGrid(primarySpan)
	secondarySpan = clampSpanToLimitedGrid(secondarySpan)

	m.markAreaAs(primaryAxis, primarySpan, secondarySpan, state)

	if primaryAxis == Horizontal {
		item.column, item.row = primarySpan, secondarySpan
	} else {
		item.column, item.row = secondarySpan, primarySpan
	}
}
