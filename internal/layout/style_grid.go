package layout

// TrackSizing is a grid track's sizing function, minmax(Min, Max). A single
// size such as 1fr or 40px is written as both: Track(Fr(1)).
type TrackSizing struct {
	// Min is a fixed length or percentage, auto, min-content or max-content.
	Min Length
	// Max is any of Min's values, fr or fit-content(<length-percentage>).
	Max Length
}

// Track returns the track sizing function for a single CSS track size, as
// CSS parses one: the minimum of 1fr or fit-content(…) is auto.
func Track(size Length) TrackSizing {
	min := size
	if size.isFr() || size.isFitContent() {
		min = Auto
	}
	return TrackSizing{Min: min, Max: size}
}

// MinMax returns minmax(min, max).
func MinMax(min, max Length) TrackSizing { return TrackSizing{Min: min, Max: max} }

func (t TrackSizing) hasFixedComponent() bool {
	return t.Min.isLengthOrPercentage() || t.Max.isLengthOrPercentage()
}

// GridTemplateComponent is one entry of grid-template-rows or
// grid-template-columns: a single track, or a repeat() when Repeat is set.
type GridTemplateComponent struct {
	Track  TrackSizing
	Repeat *Repetition
}

func (c GridTemplateComponent) isAutoRepetition() bool {
	return c.Repeat != nil && c.Repeat.Count.isAuto()
}

// Repetition is repeat(Count, Tracks…).
type Repetition struct {
	Count  RepetitionCount
	Tracks []TrackSizing
	// LineNames names the repetition's lines: group i names the line before
	// track i, and the last group the line after the last track.
	LineNames [][]string
}

// trackCount is the number of tracks in one repetition, saturating at the
// u16 maximum as Taffy's does.
func (r *Repetition) trackCount() uint16 {
	if len(r.Tracks) > 0xFFFF {
		return 0xFFFF
	}
	return uint16(len(r.Tracks))
}

// RepetitionCount is a positive repeat() count, AutoFill or AutoFit.
type RepetitionCount int32

const (
	AutoFill RepetitionCount = -1
	AutoFit  RepetitionCount = -2
)

func (c RepetitionCount) isAuto() bool { return c == AutoFill || c == AutoFit }

// GridAutoFlow is the CSS grid-auto-flow property.
type GridAutoFlow uint8

const (
	FlowRow GridAutoFlow = iota
	FlowColumn
	FlowRowDense
	FlowColumnDense
)

func (f GridAutoFlow) isDense() bool { return f == FlowRowDense || f == FlowColumnDense }

// primaryAxis is the axis along which auto-placed items are placed first.
func (f GridAutoFlow) primaryAxis() AbsoluteAxis {
	if f == FlowRow || f == FlowRowDense {
		return Horizontal
	}
	return Vertical
}

// GridTemplateAreas is the CSS grid-template-areas property. The zero value
// is none.
type GridTemplateAreas struct {
	Areas []GridTemplateArea
	// Rows and Columns are the number of rows and columns the areas span.
	Rows, Columns uint16
}

// GridTemplateArea is a named area, from its start lines to its end lines
// (1-based).
type GridTemplateArea struct {
	Name                   string
	RowStart, RowEnd       uint16
	ColumnStart, ColumnEnd uint16
}

// GridPlacement is the start or end of an item's grid-row or grid-column.
// The zero value is auto.
type GridPlacement struct {
	Kind PlacementKind
	// Line is the line of a PlacementLine: 1-based, negative counting from
	// the end of the explicit grid. For a named line it is the occurrence of
	// the name, negative counting from the end; 0 is a bare name, which
	// first matches a named area's edge and otherwise means 1.
	Line int16
	// Span is the number of tracks or named lines of a PlacementSpan.
	Span uint16
	// Name is the line name of a named line or span, empty otherwise.
	Name string
}

// PlacementKind is the kind of a GridPlacement.
type PlacementKind uint8

const (
	PlacementAuto PlacementKind = iota
	PlacementLine
	PlacementSpan
)

// GridLine returns a placement at a numbered line.
func GridLine(line int16) GridPlacement { return GridPlacement{Kind: PlacementLine, Line: line} }

// GridSpan returns a placement spanning tracks.
func GridSpan(span uint16) GridPlacement { return GridPlacement{Kind: PlacementSpan, Span: span} }

// maxGridTracks is the most tracks a grid may have in either axis, beyond
// which placements are clamped (https://www.w3.org/TR/css-grid-1/#overlarge-grids).
const maxGridTracks = 10000

const (
	minOZLine = -maxGridTracks
	maxOZLine = maxGridTracks
)
