package core

// Style is a node's layout, look and text. Every field's zero value means
// unset, so styles compose with Merge and text fields inherit from the
// parent as in CSS. A style with nothing set draws nothing: NSGo has no
// look of its own.
type Style struct {
	// Layout, with React Native's defaults: a column flexbox, items that
	// do not shrink and stretch on the cross axis, border-box sizes, no
	// automatic minimum sizes. Grid and block layout are CSS's.
	Display        Display
	Position       Position
	Direction      FlexDirection
	Wrap           FlexWrap
	JustifyContent Justify
	AlignItems     Align
	AlignSelf      Align
	AlignContent   Align
	// JustifyItems and JustifySelf align grid items (and block children)
	// along the row.
	JustifyItems Align
	JustifySelf  Align
	FlexGrow     float32
	FlexShrink   float32
	FlexBasis    Length

	Width, Height       Length
	MinWidth, MinHeight Length
	MaxWidth, MaxHeight Length
	// AspectRatio is width divided by height; 0 is unset.
	AspectRatio float32
	// BoxSizing says whether sizes include padding and borders, which they
	// do unless it is ContentBox.
	BoxSizing BoxSizing

	// Top, Right, Bottom and Left place an absolutely positioned node in
	// its parent, or offset a relative one. Right to left, left and right
	// swap, as in React Native.
	Top, Right, Bottom, Left Length

	Margin Edges
	// AutoMargin makes the margins on those sides auto: they take the free
	// space, centering a node between two of them.
	AutoMargin Sides
	Padding    Edges
	// Gap separates children; RowGap and ColumnGap override it per axis.
	Gap, RowGap, ColumnGap Points

	// GridTemplateColumns and GridTemplateRows are a grid's tracks in CSS
	// syntax, such as "repeat(3, 1fr)" or "[full-start] 100px minmax(0, 1fr)";
	// GridAutoColumns and GridAutoRows size the tracks it adds.
	GridTemplateColumns, GridTemplateRows GridTracks
	GridAutoColumns, GridAutoRows         GridTracks
	GridAutoFlow                          GridFlow
	// GridRow and GridColumn place the node in its parent grid.
	GridRow, GridColumn GridLines

	// Look.
	Background   Color
	CornerRadius Points
	// BorderWidth is the width of the border on every side. Borders, when
	// set, gives each side's width to layout instead.
	BorderWidth Points
	Borders     Edges
	BorderColor Color
	// BorderColors, when set, colors each side; a side left 0 takes
	// BorderColor.
	BorderColors EdgeColors
	Opacity      Opacity
	// Overflow Hidden clips children to the node's bounds and corners, and
	// keeps their overflow out of an enclosing scroll view's content.
	// OverflowX and OverflowY override it per axis.
	Overflow             Overflow
	OverflowX, OverflowY Overflow
	ShadowColor          Color
	ShadowOffset         Point
	ShadowRadius         Points
	// TintColor recolors an image's opaque pixels.
	TintColor Color
	// VerticalAlign places an attachment in its text's line, set on the
	// span or on the attachment's view.
	VerticalAlign VerticalAlign
	Transform     Transform
	// Transition animates changes of the look on the platform.
	Transition Transition
	// Animated binds properties to animated values the platform moves
	// every frame, over the ones above.
	Animated *AnimatedStyle

	// Text, inherited by descendants that set none.
	Color Color
	// FontFamily names a font family; empty is the system font, and
	// FontMonospace the system's fixed-pitch font.
	FontFamily     string
	FontSize       Points
	FontWeight     FontWeight
	FontStyle      FontStyle
	LineHeight     Points
	LetterSpacing  Points
	TextAlign      TextAlign
	TextDecoration TextDecoration
	// SelectionColor is the highlight of selected text; 0 is the
	// platform's.
	SelectionColor Color

	// State styles merge over the rest while the node is in the state, in
	// NSH's order of precedence, lowest first: checked, indeterminate and
	// selected, which the app sets, then hovered, focused (text fields) and
	// pressed, which the platform reports, then disabled over all.
	Hover, Focus, Active, Selected, Checked, Indeterminate, Disabled *Style
}

// Display says whether a node takes part in layout.
type Display uint8

const (
	DisplayFlex Display = iota + 1
	// DisplayNone hides the node and takes it out of layout.
	DisplayNone
	// DisplayGrid lays children out in a CSS grid.
	DisplayGrid
	// DisplayBlock stacks children as CSS block boxes, collapsing their
	// vertical margins.
	DisplayBlock
)

// BoxSizing is the CSS box-sizing property.
type BoxSizing uint8

const (
	BorderBox BoxSizing = iota + 1
	ContentBox
)

// Sides is a set of sides of a box.
type Sides uint8

const (
	SideTop Sides = 1 << iota
	SideRight
	SideBottom
	SideLeft
	// SidesHorizontal and SidesVertical are left and right, top and bottom.
	SidesHorizontal = SideLeft | SideRight
	SidesVertical   = SideTop | SideBottom
	SidesAll        = SidesHorizontal | SidesVertical
)

// Position says how a node is placed in its parent.
type Position uint8

const (
	// Relative nodes flow in their parent; Top and Left offset them.
	Relative Position = iota + 1
	// Absolute nodes leave the flow and are placed by Top, Right, Bottom
	// and Left in the parent's padding box.
	Absolute
)

// FlexDirection is the main axis of a container, as in CSS.
type FlexDirection uint8

const (
	// Column lays children out top to bottom, the default as in React Native.
	Column FlexDirection = iota + 1
	Row
	ColumnReverse
	RowReverse
)

func (d FlexDirection) row() bool     { return d == Row || d == RowReverse }
func (d FlexDirection) reverse() bool { return d == ColumnReverse || d == RowReverse }

// FlexWrap says whether children wrap onto more lines.
type FlexWrap uint8

const (
	NoWrap FlexWrap = iota + 1
	Wrap
	WrapReverse
)

// Align places children on the cross axis (align-items, align-self,
// align-content) and grid items along the row (justify-items,
// justify-self). Start and end are React Native's flex-start and flex-end.
type Align uint8

const (
	AlignStretch Align = iota + 1
	AlignStart
	AlignCenter
	AlignEnd
	AlignSpaceBetween // align-content only
	AlignSpaceAround  // align-content only
	AlignBaseline     // items only
	AlignSpaceEvenly  // align-content only
)

// Justify places children on the main axis (justify-content).
type Justify uint8

const (
	JustifyStart Justify = iota + 1
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
	JustifySpaceAround
	JustifySpaceEvenly
	// JustifyStretch stretches a grid's auto tracks across the row, which
	// grids do when it is unset.
	JustifyStretch
)

// Overflow says whether a node clips its children.
type Overflow uint8

const (
	Visible Overflow = iota + 1
	Hidden
	// OverflowScroll makes the node a scroll container in layout, as
	// CSS's overflow: scroll, and clips it; only Scroll views scroll.
	OverflowScroll
)

// Opacity is a node's opacity from 0 to 1. Its zero value is unset,
// which is opaque; make one with Alpha.
type Opacity struct {
	value float32
	set   bool
}

// Alpha returns an opacity from 0 (invisible) to 1 (opaque).
func Alpha(a float32) Opacity { return Opacity{min(max(a, 0), 1), true} }

// Value returns the opacity, 1 when unset.
func (o Opacity) Value() float32 {
	if !o.set {
		return 1
	}
	return o.value
}

// FontWeight is a CSS font weight from 100 to 900; 0 is unset.
type FontWeight uint16

const (
	FontWeightLight    FontWeight = 300
	FontWeightNormal   FontWeight = 400
	FontWeightMedium   FontWeight = 500
	FontWeightSemibold FontWeight = 600
	FontWeightBold     FontWeight = 700
)

// FontStyle is upright or italic.
type FontStyle uint8

const (
	FontStyleNormal FontStyle = iota + 1
	FontStyleItalic
)

// TextAlign aligns lines of text.
type TextAlign uint8

const (
	TextAlignStart TextAlign = iota + 1
	TextAlignCenter
	TextAlignEnd
)

// VerticalAlign is where an attachment sits in its line, as CSS's
// vertical-align places a replaced inline box, against the font of the
// text around it. Its line grows to hold it wherever it reaches past that
// font.
type VerticalAlign uint8

const (
	// VerticalAlignBaseline puts its bottom on the baseline; it is the
	// default.
	VerticalAlignBaseline VerticalAlign = iota + 1
	// VerticalAlignTextTop puts its top at the font's ascent.
	VerticalAlignTextTop
	// VerticalAlignTextBottom puts its bottom at the font's descent.
	VerticalAlignTextBottom
	// VerticalAlignMiddle puts its middle half the font's x height above
	// the baseline.
	VerticalAlignMiddle
)

// TextDecoration draws lines on text; the values combine.
type TextDecoration uint8

const (
	TextDecorationNone        TextDecoration = 1 << iota
	TextDecorationUnderline                  // underline
	TextDecorationLineThrough                // strikethrough
)

// DefaultFontSize is the font size of text no ancestor sizes.
const DefaultFontSize Points = 17

// Merge returns s with every field o sets replacing s's.
func (s Style) Merge(o Style) Style {
	set(&s.Display, o.Display)
	set(&s.Position, o.Position)
	set(&s.Direction, o.Direction)
	set(&s.Wrap, o.Wrap)
	set(&s.JustifyContent, o.JustifyContent)
	set(&s.AlignItems, o.AlignItems)
	set(&s.AlignSelf, o.AlignSelf)
	set(&s.AlignContent, o.AlignContent)
	set(&s.JustifyItems, o.JustifyItems)
	set(&s.JustifySelf, o.JustifySelf)
	set(&s.FlexGrow, o.FlexGrow)
	set(&s.FlexShrink, o.FlexShrink)
	set(&s.FlexBasis, o.FlexBasis)
	set(&s.Width, o.Width)
	set(&s.Height, o.Height)
	set(&s.MinWidth, o.MinWidth)
	set(&s.MinHeight, o.MinHeight)
	set(&s.MaxWidth, o.MaxWidth)
	set(&s.MaxHeight, o.MaxHeight)
	set(&s.AspectRatio, o.AspectRatio)
	set(&s.BoxSizing, o.BoxSizing)
	set(&s.Top, o.Top)
	set(&s.Right, o.Right)
	set(&s.Bottom, o.Bottom)
	set(&s.Left, o.Left)
	set(&s.Margin, o.Margin)
	set(&s.AutoMargin, o.AutoMargin)
	set(&s.Padding, o.Padding)
	set(&s.Gap, o.Gap)
	set(&s.RowGap, o.RowGap)
	set(&s.ColumnGap, o.ColumnGap)
	set(&s.GridTemplateColumns, o.GridTemplateColumns)
	set(&s.GridTemplateRows, o.GridTemplateRows)
	set(&s.GridAutoColumns, o.GridAutoColumns)
	set(&s.GridAutoRows, o.GridAutoRows)
	set(&s.GridAutoFlow, o.GridAutoFlow)
	set(&s.GridRow, o.GridRow)
	set(&s.GridColumn, o.GridColumn)
	set(&s.Background, o.Background)
	set(&s.CornerRadius, o.CornerRadius)
	set(&s.BorderWidth, o.BorderWidth)
	set(&s.Borders, o.Borders)
	set(&s.BorderColor, o.BorderColor)
	set(&s.BorderColors, o.BorderColors)
	set(&s.Opacity, o.Opacity)
	set(&s.Overflow, o.Overflow)
	set(&s.OverflowX, o.OverflowX)
	set(&s.OverflowY, o.OverflowY)
	set(&s.ShadowColor, o.ShadowColor)
	set(&s.ShadowOffset, o.ShadowOffset)
	set(&s.ShadowRadius, o.ShadowRadius)
	set(&s.TintColor, o.TintColor)
	set(&s.VerticalAlign, o.VerticalAlign)
	set(&s.Transform, o.Transform)
	set(&s.Transition, o.Transition)
	set(&s.Animated, o.Animated)
	s.mergeText(o)
	set(&s.Hover, o.Hover)
	set(&s.Focus, o.Focus)
	set(&s.Active, o.Active)
	set(&s.Selected, o.Selected)
	set(&s.Checked, o.Checked)
	set(&s.Indeterminate, o.Indeterminate)
	set(&s.Disabled, o.Disabled)
	return s
}

// Interaction states a node's style depends on.
const (
	statePressed uint8 = 1 << iota
	stateHovered
	stateFocused
)

// withStates returns s with the styles of the states n is in merged in.
func (s Style) withStates(states uint8, p ViewProps) Style {
	out := s
	for _, st := range [...]struct {
		on    bool
		style *Style
	}{
		{p.Checked, s.Checked},
		{p.Indeterminate, s.Indeterminate},
		{p.Selected, s.Selected},
		{states&stateHovered != 0, s.Hover},
		{states&stateFocused != 0, s.Focus},
		{states&statePressed != 0, s.Active},
		{p.Disabled, s.Disabled},
	} {
		if st.on && st.style != nil {
			out = out.Merge(*st.style)
		}
	}
	out.Hover, out.Focus, out.Active = s.Hover, s.Focus, s.Active
	out.Selected, out.Checked, out.Indeterminate, out.Disabled = s.Selected, s.Checked, s.Indeterminate, s.Disabled
	return out
}

func (s *Style) mergeText(o Style) {
	set(&s.Color, o.Color)
	set(&s.FontFamily, o.FontFamily)
	set(&s.FontSize, o.FontSize)
	set(&s.FontWeight, o.FontWeight)
	set(&s.FontStyle, o.FontStyle)
	set(&s.LineHeight, o.LineHeight)
	set(&s.LetterSpacing, o.LetterSpacing)
	set(&s.TextAlign, o.TextAlign)
	set(&s.TextDecoration, o.TextDecoration)
	set(&s.SelectionColor, o.SelectionColor)
}

func set[T comparable](dst *T, v T) {
	var zero T
	if v != zero {
		*dst = v
	}
}

// inherit returns s's text fields, falling back to parent's.
func (s Style) inherit(parent Style) Style {
	t := parent.textOnly()
	t.mergeText(s)
	return t
}

func (s Style) textOnly() Style {
	return Style{
		Color: s.Color, FontFamily: s.FontFamily, FontSize: s.FontSize, FontWeight: s.FontWeight,
		FontStyle: s.FontStyle, LineHeight: s.LineHeight, LetterSpacing: s.LetterSpacing,
		TextAlign: s.TextAlign, TextDecoration: s.TextDecoration, SelectionColor: s.SelectionColor,
	}
}

// textStyle resolves inherited text fields into what a text engine needs.
func (s Style) textStyle() TextStyle {
	t := TextStyle{
		Font: Font{
			Family: s.FontFamily,
			Size:   s.FontSize,
			Weight: s.FontWeight,
			Italic: s.FontStyle == FontStyleItalic,
		},
		Color:         s.Color,
		LineHeight:    s.LineHeight,
		LetterSpacing: s.LetterSpacing,
		Align:         s.TextAlign,
		Decoration:    s.TextDecoration &^ TextDecorationNone,
	}
	if t.Font.Size == 0 {
		t.Font.Size = DefaultFontSize
	}
	if t.Font.Weight == 0 {
		t.Font.Weight = FontWeightNormal
	}
	if t.Color == 0 {
		t.Color = RGB(0x000000)
	}
	if t.Align == 0 {
		t.Align = TextAlignStart
	}
	return t
}

// viewStyle is what platforms draw for any view.
func (s Style) viewStyle() ViewStyle {
	v := ViewStyle{
		Background:   s.Background,
		CornerRadius: s.CornerRadius,
		BorderWidth:  s.BorderWidth,
		BorderColor:  s.BorderColor,
		Opacity:      s.Opacity.Value(),
		Clip:         s.clips(),
		ShadowColor:  s.ShadowColor,
		ShadowOffset: s.ShadowOffset,
		ShadowRadius: s.ShadowRadius,
		TintColor:    s.TintColor,
		Transform:    s.Transform,
		Transition:   s.Transition,
	}
	if s.Borders != (Edges{}) || s.BorderColors != (EdgeColors{}) {
		w, c := s.borders(), s.BorderColors.or(s.BorderColor)
		if w == All(w.Top) && c == (EdgeColors{c.Top, c.Top, c.Top, c.Top}) {
			v.BorderWidth, v.BorderColor = w.Top, c.Top
		} else {
			v.BorderWidth, v.BorderColor, v.Borders, v.BorderColors = 0, 0, w, c
		}
	}
	return v
}

// EdgeColors is a color for each side of a box.
type EdgeColors struct{ Top, Right, Bottom, Left Color }

// AllColors returns edge colors of the same color on every side.
func AllColors(c Color) EdgeColors { return EdgeColors{c, c, c, c} }

// or returns e with its unset sides c.
func (e EdgeColors) or(c Color) EdgeColors {
	for _, side := range []*Color{&e.Top, &e.Right, &e.Bottom, &e.Left} {
		if *side == 0 {
			*side = c
		}
	}
	return e
}

// overflow returns the overflow on each axis.
func (s *Style) overflow() (x, y Overflow) {
	x, y = s.Overflow, s.Overflow
	if s.OverflowX != 0 {
		x = s.OverflowX
	}
	if s.OverflowY != 0 {
		y = s.OverflowY
	}
	return x, y
}

// clips reports whether the node clips its children when drawn.
func (s *Style) clips() bool {
	x, y := s.overflow()
	return x == Hidden || x == OverflowScroll || y == Hidden || y == OverflowScroll
}
