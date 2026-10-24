package layout

// Grid coordinates.
//
// Taffy uses two coordinate systems to refer to grid lines (the gaps between
// rows and columns):
//
//   - CSS grid line coordinates, as in grid-row and grid-column: 0 is not a
//     valid index, the line at the start edge of the explicit grid is 1 and
//     the line at its end edge is -1.
//   - Origin-zero coordinates (ozLine): the line at the start edge of the
//     explicit grid is 0, and lines count up (towards the end) and down
//     (towards the start) from there.
//
// And two coordinate systems to refer to tracks, both covering the whole
// implicit grid:
//
//   - Cell occupancy matrix indexes, which index tracks only, from 0 at the
//     start of the implicit grid.
//   - Track vector indexes, which index lines and tracks: even indexes are
//     lines (gutters), odd ones tracks.

// ozLine is a grid line in origin-zero coordinates.
type ozLine int16

// ozLineClamped converts line to an ozLine, clamping it into the limited
// grid.
func ozLineClamped(line int32) ozLine {
	return ozLine(min(max(line, minOZLine), maxOZLine))
}

// cssLineToOZ converts a CSS grid line (not 0) to origin-zero coordinates.
func cssLineToOZ(line int16, explicitTrackCount uint16) ozLine {
	explicitLineCount := int32(explicitTrackCount) + 1
	switch {
	case line > 0:
		return ozLineClamped(int32(line) - 1)
	case line < 0:
		return ozLineClamped(int32(line) + explicitLineCount)
	}
	panic("grid line of zero is invalid")
}

// cssLine converts l to CSS grid line coordinates.
func (l ozLine) cssLine(explicitTrackCount uint16) int16 {
	explicitLineCount := explicitTrackCount + 1
	if l >= 0 {
		return int16(l) + 1
	}
	return int16(l) - int16(explicitLineCount)
}

// trackVecIndex converts l to an index into a track vector.
func (l ozLine) trackVecIndex(counts trackCounts) int {
	i, ok := l.tryTrackVecIndex(counts)
	if !ok {
		if l > 0 {
			panic("origin-zero grid line cannot be more than the number of positive grid lines")
		}
		panic("origin-zero grid line cannot be less than the number of negative grid lines")
	}
	return i
}

func (l ozLine) tryTrackVecIndex(counts trackCounts) (int, bool) {
	if int(l) < -int(counts.negativeImplicit) {
		return 0, false
	}
	if int(l) > int(counts.explicit)+int(counts.positiveImplicit) {
		return 0, false
	}
	return 2 * (int(l) + int(counts.negativeImplicit)), true
}

// impliedNegativeImplicitTracks is the number of tracks before the explicit
// grid needed to reach l.
func (l ozLine) impliedNegativeImplicitTracks() uint16 {
	if l < 0 {
		return uint16(-int32(l))
	}
	return 0
}

// impliedPositiveImplicitTracks is the number of tracks after the explicit
// grid needed to reach l.
func (l ozLine) impliedPositiveImplicitTracks(explicitTrackCount uint16) uint16 {
	if int32(l) > int32(explicitTrackCount) {
		return uint16(int32(l) - int32(explicitTrackCount))
	}
	return 0
}

// ozSpan is the number of tracks between a line range's start and end.
func ozSpan(l Line[ozLine]) uint16 {
	return uint16(max(int32(l.End)-int32(l.Start), 0))
}

// trackCounts is the number of tracks in the explicit grid and on either
// side of it.
type trackCounts struct {
	negativeImplicit, explicit, positiveImplicit uint16
}

func (c trackCounts) len() int {
	return int(c.negativeImplicit) + int(c.explicit) + int(c.positiveImplicit)
}

func (c trackCounts) implicitStartLine() ozLine { return ozLine(-int16(c.negativeImplicit)) }

func (c trackCounts) implicitEndLine() ozLine { return ozLine(int16(c.explicit + c.positiveImplicit)) }

// ozLineToNextTrack converts a line to the cell occupancy index of the
// track that follows it.
func (c trackCounts) ozLineToNextTrack(l ozLine) int16 { return int16(l) + int16(c.negativeImplicit) }

// ozLineRangeToTrackRange converts a line range to an exclusive range of
// cell occupancy indexes.
func (c trackCounts) ozLineRangeToTrackRange(l Line[ozLine]) (start, end int16) {
	return c.ozLineToNextTrack(l.Start), c.ozLineToNextTrack(l.End)
}

// trackToPrevOZLine converts a cell occupancy index to the line before it.
func (c trackCounts) trackToPrevOZLine(index uint16) ozLine {
	return ozLine(int16(index) - int16(c.negativeImplicit))
}

// nnPlacement is a grid placement whose named lines are resolved, in CSS
// grid line coordinates (Taffy's NonNamedGridPlacement).
type nnPlacement struct {
	kind PlacementKind
	line int16
	span uint16
}

// ozPlacement is a grid placement in origin-zero coordinates.
type ozPlacement struct {
	kind PlacementKind
	line ozLine
	span uint16
}

// intoOriginZero converts p: spans are clamped between 1 (a zero span is
// invalid and treated as 1) and the track limit
// (https://www.w3.org/TR/css-grid-1/#overlarge-grids), and line 0 is invalid
// and treated as auto.
func (p nnPlacement) intoOriginZero(explicitTrackCount uint16) ozPlacement {
	switch p.kind {
	case PlacementSpan:
		return ozPlacement{kind: PlacementSpan, span: min(max(p.span, 1), maxGridTracks)}
	case PlacementLine:
		if p.line == 0 {
			return ozPlacement{}
		}
		return ozPlacement{kind: PlacementLine, line: cssLineToOZ(p.line, explicitTrackCount)}
	}
	return ozPlacement{}
}

func nnLineIntoOriginZero(l Line[nnPlacement], explicitTrackCount uint16) Line[ozPlacement] {
	return Line[ozPlacement]{l.Start.intoOriginZero(explicitTrackCount), l.End.intoOriginZero(explicitTrackCount)}
}

// ozIsDefinite reports whether a placement has a line on either end.
func ozIsDefinite(l Line[ozPlacement]) bool {
	return l.Start.kind == PlacementLine || l.End.kind == PlacementLine
}

// ozIndefiniteSpan is the span of an indefinite placement.
func ozIndefiniteSpan(l Line[ozPlacement]) uint16 {
	s, e := l.Start, l.End
	switch {
	case s.kind == PlacementLine && e.kind == PlacementLine:
		panic("indefiniteSpan should only be called on indefinite grid tracks")
	case s.kind == PlacementLine && e.kind == PlacementSpan:
		return e.span
	case s.kind == PlacementSpan:
		return s.span
	case e.kind == PlacementSpan:
		return e.span
	}
	return 1
}

// ozResolveDefiniteGridLines resolves a definite placement to lines.
func ozResolveDefiniteGridLines(l Line[ozPlacement]) Line[ozLine] {
	s, e := l.Start, l.End
	switch {
	case s.kind == PlacementLine && e.kind == PlacementLine:
		if s.line == e.line {
			return Line[ozLine]{s.line, s.line + 1}
		}
		return Line[ozLine]{min(s.line, e.line), max(s.line, e.line)}
	case s.kind == PlacementLine && e.kind == PlacementSpan:
		return Line[ozLine]{s.line, s.line + ozLine(e.span)}
	case s.kind == PlacementLine:
		return Line[ozLine]{s.line, s.line + 1}
	case s.kind == PlacementSpan && e.kind == PlacementLine:
		return Line[ozLine]{e.line - ozLine(s.span), e.line}
	case e.kind == PlacementLine:
		return Line[ozLine]{e.line - 1, e.line}
	}
	panic("resolveDefiniteGridLines should only be called on definite grid tracks")
}

// optOZLine is an optional ozLine.
type optOZLine struct {
	line  ozLine
	valid bool
}

// ozResolveAbsolutelyPositioned resolves the lines of an absolutely
// positioned item's placement, absent where auto.
func ozResolveAbsolutelyPositioned(l Line[ozPlacement]) Line[optOZLine] {
	s, e := l.Start, l.End
	some := func(v ozLine) optOZLine { return optOZLine{v, true} }
	switch {
	case s.kind == PlacementLine && e.kind == PlacementLine:
		if s.line == e.line {
			return Line[optOZLine]{some(s.line), some(s.line + 1)}
		}
		return Line[optOZLine]{some(min(s.line, e.line)), some(max(s.line, e.line))}
	case s.kind == PlacementLine && e.kind == PlacementSpan:
		return Line[optOZLine]{some(s.line), some(s.line + ozLine(e.span))}
	case s.kind == PlacementLine:
		return Line[optOZLine]{Start: some(s.line)}
	case s.kind == PlacementSpan && e.kind == PlacementLine:
		return Line[optOZLine]{some(e.line - ozLine(s.span)), some(e.line)}
	case e.kind == PlacementLine && s.kind == PlacementAuto:
		return Line[optOZLine]{End: some(e.line)}
	}
	return Line[optOZLine]{}
}

// gridTrack is a track or gutter of a grid axis during layout.
type gridTrack struct {
	isGutter    bool
	isCollapsed bool
	// min and max are the track's sizing functions.
	min, max Length

	// offset is the track's position from the container's border box start.
	offset float32
	// baseSize and growthLimit are the track sizing algorithm's values.
	baseSize, growthLimit float32
	// contentAlignmentAdjustment is extra space a gutter gets from content
	// alignment (space-between and the like), estimated while sizing items
	// in the other axis.
	contentAlignmentAdjustment float32

	itemIncurredIncrease       float32
	baseSizePlannedIncrease    float32
	growthLimitPlannedIncrease float32
	infinitelyGrowable         bool

	// fitContentLimit is the resolved fit-content() argument, or infinity.
	fitContentLimit float32
}

func newGridTrack(min, max Length) gridTrack {
	return gridTrack{min: min, max: max, fitContentLimit: inf32}
}

func newGridGutter(size Length) gridTrack {
	return gridTrack{isGutter: true, min: size, max: size, fitContentLimit: inf32}
}

// collapse makes the track a collapsed (zero-sized) auto-fit track.
func (t *gridTrack) collapse() {
	t.isCollapsed = true
	t.min = Px(0)
	t.max = Px(0)
	t.fitContentLimit = inf32
}

func (t *gridTrack) isFlexible() bool { return t.max.isFr() }

func (t *gridTrack) usesPercentage() bool { return t.min.usesPercentage() || t.max.usesPercentage() }

func (t *gridTrack) hasIntrinsicSizingFunction() bool {
	return t.min.isIntrinsic() || t.max.isIntrinsic()
}

func (t *gridTrack) resolveFitContentLimit(axisInnerNodeSize Opt) {
	t.fitContentLimit = t.max.fitContentLimit(axisInnerNodeSize).Or(inf32)
}

func (t *gridTrack) fitContentLimitedGrowthLimit() float32 {
	return fmin(t.growthLimit, t.fitContentLimit)
}

func (t *gridTrack) flexFactor() float32 {
	if t.max.isFr() {
		return t.max.Value
	}
	return 0
}

// cellState says how a grid cell is occupied.
type cellState uint8

const (
	cellUnoccupied cellState = iota
	cellDefinitelyPlaced
	cellAutoPlaced
)

// occupiedInterval is a run of cells in one track, in origin-zero
// coordinates of the other axis.
type occupiedInterval struct {
	start, end int16
	state      cellState
}

func (i occupiedInterval) overlaps(start, end int16) bool { return i.start < end && i.end > start }

// trackIntervals are the occupied cells of one track, as sorted, disjoint
// intervals.
type trackIntervals struct {
	intervals []occupiedInterval
	// cursor is where auto-placement continues in this track, when set.
	cursor    ozLine
	hasCursor bool
}

func (t *trackIntervals) isEmpty() bool { return len(t.intervals) == 0 }

func (t *trackIntervals) stateAt(coordinate int16) cellState {
	for _, iv := range t.intervals {
		if iv.start <= coordinate && coordinate < iv.end {
			return iv.state
		}
	}
	return cellUnoccupied
}

// paint marks the cells start..end as state.
func (t *trackIntervals) paint(start, end int16, state cellState) {
	if start >= end {
		return
	}

	// Fast path: the painted range lies after all existing intervals (the
	// common case, as placement mostly advances forwards).
	if len(t.intervals) == 0 {
		t.intervals = append(t.intervals, occupiedInterval{start, end, state})
		return
	}
	if last := &t.intervals[len(t.intervals)-1]; last.end <= start {
		if last.state == state && last.end == start {
			last.end = end
		} else {
			t.intervals = append(t.intervals, occupiedInterval{start, end, state})
		}
		return
	}

	result := make([]occupiedInterval, 0, len(t.intervals)+2)

	// Intervals (or parts of them) entirely before the painted range.
	for _, iv := range t.intervals {
		if iv.end <= start {
			result = append(result, iv)
		} else if iv.start < start {
			result = append(result, occupiedInterval{iv.start, start, iv.state})
		}
	}

	// The painted range, merged with the preceding interval if it touches it
	// and has the same state.
	if n := len(result); n > 0 && result[n-1].state == state && result[n-1].end == start {
		result[n-1].end = end
	} else {
		result = append(result, occupiedInterval{start, end, state})
	}

	// Intervals (or parts of them) entirely after the painted range.
	for _, iv := range t.intervals {
		var trimmed occupiedInterval
		switch {
		case iv.start >= end:
			trimmed = iv
		case iv.end > end:
			trimmed = occupiedInterval{end, iv.end, iv.state}
		default:
			continue
		}
		if n := len(result); n > 0 && result[n-1].state == trimmed.state && result[n-1].end == trimmed.start {
			result[n-1].end = trimmed.end
		} else {
			result = append(result, trimmed)
		}
	}
	t.intervals = result
}

// collisionExtent returns the last occupied cell of the last interval
// overlapping start..end.
func (t *trackIntervals) collisionExtent(start, end int16) (int16, bool) {
	for i := len(t.intervals) - 1; i >= 0; i-- {
		iv := t.intervals[i]
		if iv.end <= start {
			break
		}
		if iv.overlaps(start, end) {
			return iv.end - 1, true
		}
	}
	return 0, false
}

// cellOccupancyMatrix records which grid cells are occupied during item
// placement, per track of each axis.
type cellOccupancyMatrix struct {
	columns, rows                 trackCounts
	rowIntervals, columnIntervals []trackIntervals
}

func newCellOccupancyMatrix(columns, rows trackCounts) *cellOccupancyMatrix {
	return &cellOccupancyMatrix{
		columns:         columns,
		rows:            rows,
		rowIntervals:    make([]trackIntervals, rows.len()),
		columnIntervals: make([]trackIntervals, columns.len()),
	}
}

func (m *cellOccupancyMatrix) trackLists(axis AbsoluteAxis) []trackIntervals {
	if axis == Horizontal {
		return m.columnIntervals
	}
	return m.rowIntervals
}

func (m *cellOccupancyMatrix) trackCounts(axis AbsoluteAxis) trackCounts {
	if axis == Horizontal {
		return m.columns
	}
	return m.rows
}

// expandToFitRange adds tracks so that the matrix covers the given spans.
func (m *cellOccupancyMatrix) expandToFitRange(rowSpan, colSpan Line[ozLine]) {
	reqNegativeRows := max(-int16(m.rows.negativeImplicit)-int16(rowSpan.Start), 0)
	reqPositiveRows := max(int16(rowSpan.End)-int16(m.rows.implicitEndLine()), 0)
	reqNegativeCols := max(-int16(m.columns.negativeImplicit)-int16(colSpan.Start), 0)
	reqPositiveCols := max(int16(colSpan.End)-int16(m.columns.implicitEndLine()), 0)

	// Add empty tracks at the front or back of the per-track interval lists.
	// Intervals are stored in origin-zero coordinates, so they do not shift.
	if reqNegativeRows > 0 {
		m.rowIntervals = append(make([]trackIntervals, reqNegativeRows), m.rowIntervals...)
	}
	if reqPositiveRows > 0 {
		m.rowIntervals = append(m.rowIntervals, make([]trackIntervals, reqPositiveRows)...)
	}
	if reqNegativeCols > 0 {
		m.columnIntervals = append(make([]trackIntervals, reqNegativeCols), m.columnIntervals...)
	}
	if reqPositiveCols > 0 {
		m.columnIntervals = append(m.columnIntervals, make([]trackIntervals, reqPositiveCols)...)
	}

	m.rows.negativeImplicit += uint16(reqNegativeRows)
	m.rows.positiveImplicit += uint16(reqPositiveRows)
	m.columns.negativeImplicit += uint16(reqNegativeCols)
	m.columns.positiveImplicit += uint16(reqPositiveCols)
}

// markAreaAs marks an area of cells, growing the grid to fit it.
func (m *cellOccupancyMatrix) markAreaAs(primaryAxis AbsoluteAxis, primarySpan, secondarySpan Line[ozLine],
	value cellState) {
	rowSpan, columnSpan := primarySpan, secondarySpan
	if primaryAxis == Horizontal {
		rowSpan, columnSpan = secondarySpan, primarySpan
	}

	m.expandToFitRange(rowSpan, columnSpan)

	if value == cellAutoPlaced && primarySpan.Start < primarySpan.End && secondarySpan.Start < secondarySpan.End {
		m.advanceAutoPlacementCursor(primaryAxis, secondarySpan.Start, primarySpan.End)
	}

	rowStart, rowEnd := m.rows.ozLineRangeToTrackRange(rowSpan)
	colStart, colEnd := m.columns.ozLineRangeToTrackRange(columnSpan)
	for i := rowStart; i < rowEnd; i++ {
		m.rowIntervals[i].paint(int16(columnSpan.Start), int16(columnSpan.End), value)
	}
	for i := colStart; i < colEnd; i++ {
		m.columnIntervals[i].paint(int16(rowSpan.Start), int16(rowSpan.End), value)
	}
}

// lineAreaCollisionJump returns, if the area collides with occupied cells,
// the primary axis line just past the furthest collision.
func (m *cellOccupancyMatrix) lineAreaCollisionJump(primaryAxis AbsoluteAxis, primarySpan,
	secondarySpan Line[ozLine]) (ozLine, bool) {
	lists := m.trackLists(primaryAxis.other())
	secondaryCounts := m.trackCounts(primaryAxis.other())
	secondaryStart, secondaryEnd := secondaryCounts.ozLineRangeToTrackRange(secondarySpan)

	// Out of bounds cells are unoccupied, so clamp the secondary range to the
	// tracks that exist.
	secondaryStart = max(secondaryStart, 0)
	secondaryEnd = min(secondaryEnd, int16(len(lists)))

	var extent int16
	found := false
	for i := secondaryStart; i < secondaryEnd; i++ {
		cell, ok := lists[i].collisionExtent(int16(primarySpan.Start), int16(primarySpan.End))
		if !ok {
			continue
		}
		if !found || cell > extent {
			extent = cell
		}
		found = true
	}
	if !found {
		return 0, false
	}
	return ozLine(extent + 1), true
}

// occupiedTrackJump returns the line after the last non-empty track the
// span covers, if any.
func (m *cellOccupancyMatrix) occupiedTrackJump(axis AbsoluteAxis, span Line[ozLine]) (ozLine, bool) {
	counts := m.trackCounts(axis)
	lists := m.trackLists(axis)
	start, end := counts.ozLineRangeToTrackRange(span)
	start = max(start, 0)
	end = min(end, int16(len(lists)))
	for i := end - 1; i >= start; i-- {
		if !lists[i].isEmpty() {
			return counts.trackToPrevOZLine(uint16(i)) + 1, true
		}
	}
	return 0, false
}

func (m *cellOccupancyMatrix) rowIsOccupied(index int) bool {
	return index < len(m.rowIntervals) && !m.rowIntervals[index].isEmpty()
}

func (m *cellOccupancyMatrix) columnIsOccupied(index int) bool {
	return index < len(m.columnIntervals) && !m.columnIntervals[index].isEmpty()
}

// autoPlacementCursor returns where auto-placement continues in the track
// of the other axis starting at startAt.
func (m *cellOccupancyMatrix) autoPlacementCursor(axis AbsoluteAxis, startAt ozLine) (ozLine, bool) {
	counts := m.trackCounts(axis.other())
	index := counts.ozLineToNextTrack(startAt)
	lists := m.trackLists(axis.other())
	if index < 0 || int(index) >= len(lists) {
		return 0, false
	}
	t := &lists[index]
	return t.cursor, t.hasCursor
}

func (m *cellOccupancyMatrix) advanceAutoPlacementCursor(primaryAxis AbsoluteAxis, secondaryStart,
	primaryEnd ozLine) {
	index := m.trackCounts(primaryAxis.other()).ozLineToNextTrack(secondaryStart)
	tracks := m.columnIntervals
	if primaryAxis == Horizontal {
		tracks = m.rowIntervals
	}
	t := &tracks[index]
	if !t.hasCursor || primaryEnd > t.cursor {
		t.cursor = primaryEnd
	}
	t.hasCursor = true
}
