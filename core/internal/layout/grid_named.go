package layout

import (
	"sort"
	"strings"
)

// Resolving named grid lines and areas.

// gridAreaAxis is the axis of a grid area edge.
type gridAreaAxis uint8

const (
	areaRow gridAreaAxis = iota
	areaColumn
)

// gridAreaEnd is which edge of a grid area.
type gridAreaEnd uint8

const (
	areaStart gridAreaEnd = iota
	areaEnd
)

// areaLines are the 1-based lines of a named area's edges; 0 is unset.
type areaLines struct {
	rowStart, rowEnd, columnStart, columnEnd uint32
}

func (a *areaLines) line(axis gridAreaAxis, end gridAreaEnd) *uint32 {
	switch {
	case axis == areaRow && end == areaStart:
		return &a.rowStart
	case axis == areaRow:
		return &a.rowEnd
	case end == areaStart:
		return &a.columnStart
	}
	return &a.columnEnd
}

// gridAreasMap maps area names to their lines.
type gridAreasMap map[string]*areaLines

// namedLinesMap maps line names to the sorted 1-based lines with that name.
type namedLinesMap map[string][]uint32

// splitAreaEdgeName splits foo-start or foo-end into foo and the edge.
func splitAreaEdgeName(name string) (string, gridAreaEnd, bool) {
	if area, ok := strings.CutSuffix(name, "-start"); ok {
		return area, areaStart, true
	}
	if area, ok := strings.CutSuffix(name, "-end"); ok {
		return area, areaEnd, true
	}
	return "", 0, false
}

func upsertLineName(lines namedLinesMap, areas gridAreasMap, axis gridAreaAxis, key string, value uint32) {
	// A line named foo-start or foo-end is also an edge of the (implicitly)
	// named area foo. Record it as such if it is the first such line. If foo
	// is also an area defined by grid-template-areas, then the line that area
	// generated also has this name.
	var areaLine uint32
	if area, end, ok := splitAreaEdgeName(key); ok {
		a, exists := areas[area]
		if !exists {
			a = &areaLines{}
			areas[area] = a
		}
		edge := a.line(axis, end)
		areaLine = *edge
		if *edge == 0 {
			*edge = value
		} else {
			*edge = min(*edge, value)
		}
	}

	if existing, ok := lines[key]; ok {
		lines[key] = append(existing, value)
		return
	}
	// Until this entry is created, the only line that can have been recorded
	// for this edge is the area's own.
	if areaLine == 0 {
		lines[key] = []uint32{value}
	} else {
		lines[key] = []uint32{areaLine, value}
	}
}

// namedLineResolverAxis resolves the named lines of one axis.
type namedLineResolverAxis struct {
	lines              namedLinesMap
	areas              gridAreasMap
	axis               gridAreaAxis
	explicitTrackCount uint16
}

func (r *namedLineResolverAxis) areaLine(name string, end gridAreaEnd) (uint32, bool) {
	a, ok := r.areas[name]
	if !ok {
		return 0, false
	}
	l := *a.line(r.axis, end)
	return l, l != 0
}

// resolveLineNames resolves the named lines and spans of a placement.
func (r *namedLineResolverAxis) resolveLineNames(line Line[GridPlacement]) Line[nnPlacement] {
	start, end := line.Start, line.End
	if start.Kind == PlacementLine && start.Name != "" {
		start = GridLine(r.findNamedLine(start.Name, start.Line, areaStart))
	}
	if end.Kind == PlacementLine && end.Name != "" {
		end = GridLine(r.findNamedLine(end.Name, end.Line, areaEnd))
	}

	// If both the *-start and *-end values of its grid-placement properties
	// specify a line, its grid span is implicit. If it has an explicit span
	// value, its grid span is explicit. Otherwise, its grid span is automatic:
	//   - if it is subgridded in that axis, its grid span is determined from
	//     its <line-name-list>;
	//   - otherwise its grid span is 1.
	//
	// https://drafts.csswg.org/css-grid-2/#grid-span
	isNamedSpan := func(p GridPlacement) bool { return p.Kind == PlacementSpan && p.Name != "" }
	switch {
	case start.Kind == PlacementLine && start.Line != 0 && isNamedSpan(end):
		afterStart := cssLineToOZ(start.Line, r.explicitTrackCount) + 1
		endLine := r.nthLineForwards(r.linesNamed(end.Name), afterStart, int32(max(end.Span, 1)))
		return Line[nnPlacement]{
			nnPlacement{kind: PlacementLine, line: start.Line},
			nnPlacement{kind: PlacementLine, line: endLine.cssLine(r.explicitTrackCount)},
		}
	case isNamedSpan(start) && end.Kind == PlacementLine && end.Line != 0:
		beforeEnd := cssLineToOZ(end.Line, r.explicitTrackCount) - 1
		startLine := r.nthLineBackwards(r.linesNamed(start.Name), beforeEnd, int32(max(start.Span, 1)))
		return Line[nnPlacement]{
			nnPlacement{kind: PlacementLine, line: startLine.cssLine(r.explicitTrackCount)},
			nnPlacement{kind: PlacementLine, line: end.Line},
		}
	}
	return Line[nnPlacement]{unnamedPlacement(start), unnamedPlacement(end)}
}

func unnamedPlacement(p GridPlacement) nnPlacement {
	switch p.Kind {
	case PlacementLine:
		return nnPlacement{kind: PlacementLine, line: p.Line}
	case PlacementSpan:
		if p.Name != "" {
			return nnPlacement{kind: PlacementSpan, span: 1}
		}
		return nnPlacement{kind: PlacementSpan, span: p.Span}
	}
	return nnPlacement{}
}

func (r *namedLineResolverAxis) findNamedLine(name string, idx int16, end gridAreaEnd) int16 {
	// A <custom-ident> on its own first attempts to match the edge of a named
	// grid area: the first line named <custom-ident>-start (or -end for an
	// end line). Otherwise it is treated as if the integer 1 had been
	// specified along with it.
	if idx == 0 {
		if first, ok := r.areaLine(name, end); ok {
			return ozLineClamped(lineToOZ(first)).cssLine(r.explicitTrackCount)
		}
	}

	lines := r.linesNamed(name)
	var line ozLine
	if idx >= 0 {
		line = r.nthLineForwards(lines, 0, max(int32(idx), 1))
	} else {
		line = r.nthLineBackwards(lines, r.lastExplicitLine(), -int32(idx))
	}
	return line.cssLine(r.explicitTrackCount)
}

func (r *namedLineResolverAxis) linesNamed(name string) []uint32 {
	if lines, ok := r.lines[name]; ok {
		return lines
	}
	if area, end, ok := splitAreaEdgeName(name); ok {
		if l, ok := r.areaLine(area, end); ok {
			return []uint32{l}
		}
	}
	return nil
}

func (r *namedLineResolverAxis) lastExplicitLine() ozLine { return ozLine(int16(r.explicitTrackCount)) }

// nthLineForwards finds the nth line in lines at or after firstLine,
// counting implicit lines after the explicit grid as named.
func (r *namedLineResolverAxis) nthLineForwards(lines []uint32, firstLine ozLine, n int32) ozLine {
	first := int32(max(int16(firstLine), 0))
	lines = lines[sort.Search(len(lines), func(i int) bool { return lineToOZ(lines[i]) >= first }):]
	var line int32
	if int(n-1) < len(lines) {
		line = lineToOZ(lines[n-1])
	} else {
		line = max(first, int32(r.lastExplicitLine())+1) + (n - int32(len(lines))) - 1
	}
	return ozLineClamped(line)
}

// nthLineBackwards finds the nth line in lines at or before lastLine,
// counting implicit lines before the explicit grid as named.
func (r *namedLineResolverAxis) nthLineBackwards(lines []uint32, lastLine ozLine, n int32) ozLine {
	last := int32(min(lastLine, r.lastExplicitLine()))
	lines = lines[:sort.Search(len(lines), func(i int) bool { return lineToOZ(lines[i]) > last })]
	var line int32
	if int(n) <= len(lines) {
		line = lineToOZ(lines[len(lines)-int(n)])
	} else {
		line = min(last, -1) - (n - int32(len(lines))) + 1
	}
	return ozLineClamped(line)
}

// lineToOZ converts a 1-based line to origin-zero coordinates.
func lineToOZ(line uint32) int32 { return int32(line) - 1 }

// namedLineResolver resolves the line names of a grid container's template
// and areas.
type namedLineResolver struct {
	rowLines, columnLines                 namedLinesMap
	areas                                 gridAreasMap
	areaColumnCount, areaRowCount         uint16
	explicitColumnCount, explicitRowCount uint16
	columnLineNamePairs, rowLineNamePairs []lineNamePair
}

type lineNamePair struct {
	line uint32
	name string
}

func newNamedLineResolver(style *Style, columnAutoRepetitions, rowAutoRepetitions uint16) *namedLineResolver {
	r := &namedLineResolver{
		rowLines:    namedLinesMap{},
		columnLines: namedLinesMap{},
		areas:       gridAreasMap{},
		// The size of the area template may be larger than the extents of
		// the named areas due to unnamed (.) cells, so it is taken from the
		// style rather than derived from the areas themselves.
		areaColumnCount: style.GridTemplateAreas.Columns,
		areaRowCount:    style.GridTemplateAreas.Rows,
	}
	for _, area := range style.GridTemplateAreas.Areas {
		r.areas[area.Name] = &areaLines{
			rowStart:    uint32(area.RowStart),
			rowEnd:      uint32(area.RowEnd),
			columnStart: uint32(area.ColumnStart),
			columnEnd:   uint32(area.ColumnEnd),
		}
	}

	r.collectLineNames(style.GridTemplateColumns, style.GridTemplateColumnNames, columnAutoRepetitions,
		areaColumn, r.columnLines, &r.columnLineNamePairs)
	r.collectLineNames(style.GridTemplateRows, style.GridTemplateRowNames, rowAutoRepetitions, areaRow,
		r.rowLines, &r.rowLineNamePairs)

	// Sort and dedup the lines of each name.
	for _, m := range []namedLinesMap{r.columnLines, r.rowLines} {
		for name, lines := range m {
			sort.Slice(lines, func(i, j int) bool { return lines[i] < lines[j] })
			out := lines[:0]
			for i, l := range lines {
				if i == 0 || l != lines[i-1] {
					out = append(out, l)
				}
			}
			m[name] = out
		}
	}
	return r
}

func (r *namedLineResolver) collectLineNames(tracks []GridTemplateComponent, names [][]string,
	autoRepetitions uint16, axis gridAreaAxis, lines namedLinesMap, pairs *[]lineNamePair) {
	var currentLine uint32
	trackIndex := 0
	for _, lineNames := range names {
		currentLine++
		for _, name := range lineNames {
			*pairs = append(*pairs, lineNamePair{currentLine, name})
			upsertLineName(lines, r.areas, axis, name, currentLine)
		}

		var repeat *Repetition
		if trackIndex < len(tracks) {
			repeat = tracks[trackIndex].Repeat
			trackIndex++
		}
		if repeat == nil {
			continue
		}
		var repeatCount uint16
		if repeat.Count.isAuto() {
			repeatCount = autoRepetitions
		} else {
			repeatCount = uint16(repeat.Count)
		}

		// Line name sets are positional: set i names the ith line of each
		// repetition, and the final line name set of each repetition
		// collapses with the first line name set of the following one. An
		// empty list means the repetition's lines are unnamed; any other
		// length must be exactly track count + 1 (one set per line, including
		// both edge lines).
		setCount := uint32(len(repeat.LineNames))
		linesPerRepetition := uint32(repeat.trackCount())
		if setCount != 0 && setCount != linesPerRepetition+1 {
			panic("grid template repetition must have no line name sets or exactly track count + 1 of them")
		}

		for range repeatCount {
			for i, set := range repeat.LineNames {
				line := currentLine + uint32(i)
				for _, name := range set {
					*pairs = append(*pairs, lineNamePair{line, name})
					upsertLineName(lines, r.areas, axis, name, line)
				}
			}
			currentLine += linesPerRepetition

			// Names for lines beyond the maximum track limit are never
			// resolvable: stop generating them (the explicit grid is clamped
			// to maxGridTracks).
			if currentLine > maxGridTracks {
				break
			}
		}
		// The last line name set collapses with the following one.
		if repeatCount > 0 && currentLine > 0 {
			currentLine--
		}
	}
}

// detailedLineNames builds the names of each explicit line of an axis.
func (r *namedLineResolver) detailedLineNames(axis AbsoluteAxis) gridLineNames {
	pairs, explicitTrackCount := r.rowLineNamePairs, r.explicitRowCount
	if axis == Horizontal {
		pairs, explicitTrackCount = r.columnLineNamePairs, r.explicitColumnCount
	}
	if len(pairs) == 0 {
		return gridLineNames{}
	}

	// A stable sort by line keeps the source order of each line's names.
	sorted := append([]lineNamePair(nil), pairs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].line < sorted[j].line })

	lineCount := int(explicitTrackCount) + 1
	names := gridLineNames{offsets: make([]uint32, 1, lineCount+2)}
	k := 0
	for line := uint32(1); line <= uint32(lineCount); line++ {
		names.startLine()
		for k < len(sorted) && sorted[k].line == line {
			if !names.currentLineContains(sorted[k].name) {
				names.pushName(sorted[k].name)
			}
			k++
		}
	}
	return names
}

func (r *namedLineResolver) resolveRowNames(line Line[GridPlacement]) Line[nnPlacement] {
	return r.resolveLineNames(line, areaRow)
}

func (r *namedLineResolver) resolveColumnNames(line Line[GridPlacement]) Line[nnPlacement] {
	return r.resolveLineNames(line, areaColumn)
}

func (r *namedLineResolver) resolveLineNames(line Line[GridPlacement], axis gridAreaAxis) Line[nnPlacement] {
	ax := namedLineResolverAxis{lines: r.rowLines, areas: r.areas, axis: axis, explicitTrackCount: r.explicitRowCount}
	if axis == areaColumn {
		ax.lines, ax.explicitTrackCount = r.columnLines, r.explicitColumnCount
	}
	return ax.resolveLineNames(line)
}

// gridLineNames are the names of each explicit grid line of an axis: line i
// is names[offsets[i]:offsets[i+1]].
type gridLineNames struct {
	names   []string
	offsets []uint32
	// resolver resolves named lines of out-of-flow children after layout.
	resolver namedLinesMap
}

func (n *gridLineNames) startLine() { n.offsets = append(n.offsets, uint32(len(n.names))) }

func (n *gridLineNames) pushName(name string) {
	n.names = append(n.names, name)
	n.offsets[len(n.offsets)-1] = uint32(len(n.names))
}

func (n *gridLineNames) currentLineContains(name string) bool {
	for _, have := range n.line(n.lineCount() - 1) {
		if have == name {
			return true
		}
	}
	return false
}

func (n *gridLineNames) isEmpty() bool { return len(n.names) == 0 }

func (n *gridLineNames) lineCount() int { return max(len(n.offsets)-1, 0) }

// line returns the names of line i, or nothing when out of range.
func (n *gridLineNames) line(i int) []string {
	if i < 0 || i+1 >= len(n.offsets) {
		return nil
	}
	return n.names[n.offsets[i]:n.offsets[i+1]]
}

func (n *gridLineNames) resolveLineNames(line Line[GridPlacement], areas gridAreasMap, axis gridAreaAxis,
	explicitTrackCount uint16) Line[nnPlacement] {
	ax := namedLineResolverAxis{lines: n.resolver, areas: areas, axis: axis, explicitTrackCount: explicitTrackCount}
	return ax.resolveLineNames(line)
}
