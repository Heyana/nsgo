package layout

// Style is a node's layout style, Taffy's Style. Every field's zero value is
// unset, which means the property's initial value: CSS's where Taffy follows
// CSS, Taffy's otherwise (display flex, border-box sizing). NSGo's core sets
// React Native's defaults itself when it maps its own styles onto this one.
type Style struct {
	// Display picks the layout algorithm for the node's children. Zero is flex.
	Display Display
	// ItemIsTable makes an absolutely positioned or block item size as a
	// table: it is never stretched narrower than its content.
	ItemIsTable bool
	// ItemIsReplaced marks a replaced element such as an image.
	ItemIsReplaced bool
	// ItemIsCompressibleReplaced marks a replaced element whose min-content
	// contribution may be compressed, such as a text field.
	ItemIsCompressibleReplaced bool
	// BoxSizing says whether sizes include padding and border. Zero is
	// border-box.
	BoxSizing BoxSizing
	// Direction is the inline direction. Zero is left to right.
	Direction Direction

	// Overflow is the overflow on each axis. Zero is visible.
	Overflow Point[Overflow]
	// ScrollbarWidth is the space reserved for a scrollbar on an axis whose
	// overflow is scroll.
	ScrollbarWidth float32
	// Contain is the CSS contain property. Zero is none.
	Contain Contain

	// Position says how the node is positioned. Zero is static.
	Position Position
	// Inset is top, right, bottom and left. Zero is auto.
	Inset Rect[Length]

	// Size, MinSize and MaxSize are the preferred, minimum and maximum
	// sizes. Zero is auto (none for MaxSize).
	Size, MinSize, MaxSize Size[Length]
	// AspectRatio is width divided by height, when present.
	AspectRatio Opt

	// Margin, Padding and Border are the box's edges. Zero is 0px. Margins
	// may be Auto.
	Margin, Padding, Border Rect[Length]

	// AlignItems and JustifyItems are the default self-alignment of
	// children. Zero is normal.
	AlignItems, JustifyItems Align
	// AlignSelf and JustifySelf override the parent's AlignItems and
	// JustifyItems. Zero is auto: use the parent's.
	AlignSelf, JustifySelf Align
	// AlignContent and JustifyContent distribute lines, tracks or items.
	// Zero is normal.
	AlignContent, JustifyContent AlignContent
	// Gap is the column gap (Width) and the row gap (Height). Zero is 0px.
	Gap Size[Length]

	// TextAlign is the legacy -webkit-left/right/center alignment of block
	// children. Zero is auto.
	TextAlign TextAlign

	// FlexDirection is the main axis of a flex container. Zero is row.
	FlexDirection FlexDirection
	// FlexWrap says whether flex items wrap. Zero is nowrap.
	FlexWrap FlexWrap
	// FlexBasis is the flex item's initial main size. Zero is auto.
	FlexBasis Length
	// FlexGrow is the flex grow factor. Zero is 0.
	FlexGrow float32
	// FlexShrink is the flex shrink factor. Absent is CSS's initial 1.
	FlexShrink Opt

	// GridTemplateRows and GridTemplateColumns are the explicit grid's
	// tracks.
	GridTemplateRows, GridTemplateColumns []GridTemplateComponent
	// GridTemplateRowNames and GridTemplateColumnNames name the explicit
	// grid's lines: group i names line i, repetitions counting as one track.
	GridTemplateRowNames, GridTemplateColumnNames [][]string
	// GridTemplateAreas names areas of the explicit grid.
	GridTemplateAreas GridTemplateAreas
	// GridAutoRows and GridAutoColumns size implicit tracks. Empty is auto.
	GridAutoRows, GridAutoColumns []TrackSizing
	// GridAutoFlow places auto-placed grid items. Zero is row.
	GridAutoFlow GridAutoFlow
	// GridRow and GridColumn place the node in its parent grid. Zero is auto.
	GridRow, GridColumn Line[GridPlacement]
}

// Display is the CSS display property.
type Display uint8

const (
	DisplayFlex Display = iota
	DisplayBlock
	DisplayFlowRoot
	DisplayGrid
	// DisplayNone hides the node and its subtree: they get zero layouts.
	DisplayNone
)

// BoxSizing is the CSS box-sizing property.
type BoxSizing uint8

const (
	BorderBox BoxSizing = iota
	ContentBox
)

// Direction is the CSS direction property.
type Direction uint8

const (
	LTR Direction = iota
	RTL
)

func (d Direction) isRTL() bool { return d == RTL }

// Overflow is the CSS overflow property on one axis.
type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowClip
	OverflowHidden
	OverflowScroll
)

func (o Overflow) isScrollContainer() bool { return o == OverflowHidden || o == OverflowScroll }

// automaticMinSize is Overflow::maybe_into_automatic_min_size: a scroll
// container's automatic minimum size is 0.
func (o Overflow) automaticMinSize() Opt {
	if o.isScrollContainer() {
		return Some(0)
	}
	return None
}

// Contain is the CSS contain property, a set of flags.
type Contain uint8

const (
	ContainLayout Contain = 1 << iota
	ContainPaint
	// ContainContent is layout and paint containment.
	ContainContent = ContainLayout | ContainPaint
)

func (c Contain) establishesIndependentFormattingContext() bool {
	return c&(ContainLayout|ContainPaint) != 0
}

func (c Contain) suppressesBaseline() bool { return c&ContainLayout != 0 }

func (c Contain) containsScrollableOverflow() bool { return c&(ContainLayout|ContainPaint) != 0 }

// Position is the CSS position property.
type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

func (p Position) isOutOfFlow() bool { return p == PositionAbsolute || p == PositionFixed }

func (p Position) isPositioned() bool { return p != PositionStatic }

// containingBlockClaims says which out-of-flow boxes a node is the
// containing block for.
type containingBlockClaims struct {
	absolute, fixed bool
}

var claimAll = containingBlockClaims{true, true}

func (c containingBlockClaims) forPosition(p Position) bool {
	switch p {
	case PositionAbsolute:
		return c.absolute
	case PositionFixed:
		return c.fixed
	}
	return false
}

// TextAlign is the legacy text-align values that align block children.
type TextAlign uint8

const (
	TextAlignAuto TextAlign = iota
	TextAlignLegacyLeft
	TextAlignLegacyRight
	TextAlignLegacyCenter
)

// FlexDirection is the CSS flex-direction property.
type FlexDirection uint8

const (
	Row FlexDirection = iota
	Column
	RowReverse
	ColumnReverse
)

func (d FlexDirection) isRow() bool     { return d == Row || d == RowReverse }
func (d FlexDirection) isColumn() bool  { return d == Column || d == ColumnReverse }
func (d FlexDirection) isReverse() bool { return d == RowReverse || d == ColumnReverse }

func (d FlexDirection) mainAxis() AbsoluteAxis {
	if d.isRow() {
		return Horizontal
	}
	return Vertical
}

func (d FlexDirection) crossAxis() AbsoluteAxis {
	if d.isRow() {
		return Vertical
	}
	return Horizontal
}

// FlexWrap is the CSS flex-wrap property.
type FlexWrap uint8

const (
	NoWrap FlexWrap = iota
	Wrap
	WrapReverse
)

func (w FlexWrap) isMultiLine() bool { return w != NoWrap }
func (w FlexWrap) isReverse() bool   { return w == WrapReverse }

// AlignmentSafety is the safe or unsafe overflow-position modifier of an
// alignment.
type AlignmentSafety uint8

const (
	// SafetyDefault is neither safe nor unsafe: the alignment's default
	// overflow behavior.
	SafetyDefault AlignmentSafety = iota
	Unsafe
	Safe
)

// Align is a value of align-items, justify-items, align-self or
// justify-self.
type Align struct {
	Keyword AlignKeyword
	Safety  AlignmentSafety
}

// AlignKeyword is the keyword of an Align.
type AlignKeyword uint8

const (
	// AlignAuto is unset: normal for align-items and justify-items, the
	// parent's items alignment for align-self and justify-self.
	AlignAuto AlignKeyword = iota
	AlignNormal
	AlignStart
	AlignEnd
	AlignFlexStart
	AlignFlexEnd
	AlignSelfStart
	AlignSelfEnd
	AlignCenter
	AlignBaseline
	AlignStretch
)

func alignOf(k AlignKeyword) Align { return Align{Keyword: k} }

func (a Align) isSafe() bool { return a.Safety == Safe }

func (a Align) isStretchOrNormal() bool {
	return a.Keyword == AlignStretch || a.Keyword == AlignNormal
}

// resolveSelfRelative turns self-start and self-end into start or end: an
// item whose direction differs from its container's flips them on the
// inline axis.
func (a Align) resolveSelfRelative(item, container Direction, axisIsInline bool) Align {
	flip := axisIsInline && item != container
	switch a.Keyword {
	case AlignSelfStart:
		if flip {
			a.Keyword = AlignEnd
		} else {
			a.Keyword = AlignStart
		}
	case AlignSelfEnd:
		if flip {
			a.Keyword = AlignStart
		} else {
			a.Keyword = AlignEnd
		}
	}
	return a
}

// AlignContent is a value of align-content or justify-content.
type AlignContent struct {
	Keyword ContentKeyword
	Safety  AlignmentSafety
}

// ContentKeyword is the keyword of an AlignContent.
type ContentKeyword uint8

const (
	ContentNormal ContentKeyword = iota
	ContentStart
	ContentEnd
	ContentFlexStart
	ContentFlexEnd
	ContentCenter
	ContentStretch
	ContentSpaceBetween
	ContentSpaceEvenly
	ContentSpaceAround
)

func contentOf(k ContentKeyword) AlignContent { return AlignContent{Keyword: k} }

func (a AlignContent) isSafe() bool { return a.Safety == Safe }

// reversed swaps start and end keywords, as for a reversed axis.
func (k ContentKeyword) reversed() ContentKeyword {
	switch k {
	case ContentStart:
		return ContentEnd
	case ContentEnd:
		return ContentStart
	case ContentFlexStart:
		return ContentFlexEnd
	case ContentFlexEnd:
		return ContentFlexStart
	case ContentStretch:
		return ContentEnd
	}
	return k
}

// The style accessors below are Taffy's style traits. Layout reads margins,
// padding, borders, gaps, flex-shrink and self-alignment through them so that
// unset values become their initial values; other fields are read directly.

func (s *Style) margin() Rect[Length] { return zeroUnset(s.Margin) }

func (s *Style) padding() Rect[Length] { return zeroUnset(s.Padding) }

func (s *Style) border() Rect[Length] { return zeroUnset(s.Border) }

func (s *Style) gap() Size[Length] {
	g := s.Gap
	if g.Width.Unit == UnitUnset {
		g.Width = Px(0)
	}
	if g.Height.Unit == UnitUnset {
		g.Height = Px(0)
	}
	return g
}

func zeroUnset(r Rect[Length]) Rect[Length] {
	if r.Left.Unit == UnitUnset {
		r.Left = Px(0)
	}
	if r.Right.Unit == UnitUnset {
		r.Right = Px(0)
	}
	if r.Top.Unit == UnitUnset {
		r.Top = Px(0)
	}
	if r.Bottom.Unit == UnitUnset {
		r.Bottom = Px(0)
	}
	return r
}

func (s *Style) flexShrink() float32 { return s.FlexShrink.Or(1) }

func (s *Style) alignItems() Align { return itemsAlign(s.AlignItems) }

func (s *Style) justifyItems() Align { return itemsAlign(s.JustifyItems) }

func itemsAlign(a Align) Align {
	if a.Keyword == AlignAuto {
		a.Keyword = AlignNormal
	}
	return a
}

// alignSelf and justifySelf return the self-alignment and whether it is set
// (Taffy's Option<AlignSelf>, None being auto).
func (s *Style) alignSelf() (Align, bool) { return s.AlignSelf, s.AlignSelf.Keyword != AlignAuto }

func (s *Style) justifySelf() (Align, bool) { return s.JustifySelf, s.JustifySelf.Keyword != AlignAuto }

// alignSelfOr returns the align-self, or def when it is auto.
func (s *Style) alignSelfOr(def Align) Align {
	if a, ok := s.alignSelf(); ok {
		return a
	}
	return def
}

func (s *Style) justifySelfOr(def Align) Align {
	if a, ok := s.justifySelf(); ok {
		return a
	}
	return def
}

func (s *Style) isBlock() bool { return s.Display == DisplayBlock }

func (s *Style) isReplaced() bool { return s.ItemIsReplaced }

func (s *Style) isCompressibleReplaced() bool {
	return s.ItemIsReplaced || s.ItemIsCompressibleReplaced
}

// isContainingBlock says which out-of-flow descendants the node is the
// containing block for: a positioned node contains absolute boxes, and fixed
// boxes go to the root.
func (s *Style) isContainingBlock() containingBlockClaims {
	return containingBlockClaims{absolute: s.Position.isPositioned()}
}

// scrollbarGutter is the space reserved for scrollbars: on the x axis for a
// vertical scrollbar (overflow-y scroll), on the y axis for a horizontal one.
func (s *Style) scrollbarGutter() Point[float32] {
	var p Point[float32]
	if s.Overflow.Y == OverflowScroll {
		p.X = s.ScrollbarWidth
	}
	if s.Overflow.X == OverflowScroll {
		p.Y = s.ScrollbarWidth
	}
	return p
}

// gridAlignContent returns justify-content for the inline axis and
// align-content for the block axis, normal behaving as stretch.
func (s *Style) gridAlignContent(axis AbstractAxis) AlignContent {
	a := s.AlignContent
	if axis == Inline {
		a = s.JustifyContent
	}
	if a.Keyword == ContentNormal {
		return contentOf(ContentStretch)
	}
	return a
}

func (s *Style) gridTemplateTracks(axis AbsoluteAxis) []GridTemplateComponent {
	if axis == Horizontal {
		return s.GridTemplateColumns
	}
	return s.GridTemplateRows
}

func (s *Style) gridTemplateLineNames(axis AbsoluteAxis) [][]string {
	if axis == Horizontal {
		return s.GridTemplateColumnNames
	}
	return s.GridTemplateRowNames
}

func (s *Style) gridAutoTracks(axis AbsoluteAxis) []TrackSizing {
	if axis == Horizontal {
		return s.GridAutoColumns
	}
	return s.GridAutoRows
}

func (s *Style) gridPlacement(axis AbsoluteAxis) Line[GridPlacement] {
	if axis == Horizontal {
		return s.GridColumn
	}
	return s.GridRow
}
