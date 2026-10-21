package core

// Style is a node's layout, look and text. Every field's zero value means
// unset, so styles compose with Merge and text fields inherit from the
// parent as in CSS. A style with nothing set draws nothing: NSGo has no
// look of its own.
type Style struct {
	// Layout, with React Native's defaults: column flexbox, no shrinking,
	// children stretched on the cross axis.
	Display        Display
	Position       Position
	Direction      FlexDirection
	Wrap           FlexWrap
	JustifyContent Justify
	AlignItems     Align
	AlignSelf      Align
	AlignContent   Align
	FlexGrow       float32
	FlexShrink     float32
	FlexBasis      Length

	Width, Height       Length
	MinWidth, MinHeight Length
	MaxWidth, MaxHeight Length
	// AspectRatio is width divided by height; 0 is unset.
	AspectRatio float32

	// Top, Right, Bottom and Left place an absolutely positioned node in
	// its parent, or offset a relative one.
	Top, Right, Bottom, Left Length

	Margin  Edges
	Padding Edges
	// Gap separates children; RowGap and ColumnGap override it per axis.
	Gap, RowGap, ColumnGap Points

	// Look.
	Background   Color
	CornerRadius Points
	BorderWidth  Points
	BorderColor  Color
	Opacity      Opacity
	// Overflow Hidden clips children to the node's bounds and corners.
	Overflow     Overflow
	ShadowColor  Color
	ShadowOffset Point
	ShadowRadius Points

	// Text, inherited by descendants that set none.
	Color          Color
	FontFamily     string
	FontSize       Points
	FontWeight     FontWeight
	FontStyle      FontStyle
	LineHeight     Points
	LetterSpacing  Points
	TextAlign      TextAlign
	TextDecoration TextDecoration

	// State styles merge over the rest while the node is pressed, hovered,
	// focused (text fields) or disabled, in that order of precedence from
	// lowest to highest.
	Hover, Focus, Active, Disabled *Style
}

// Display says whether a node takes part in layout.
type Display uint8

const (
	DisplayFlex Display = iota + 1
	// DisplayNone hides the node and takes it out of layout.
	DisplayNone
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
// align-content).
type Align uint8

const (
	AlignStretch Align = iota + 1
	AlignStart
	AlignCenter
	AlignEnd
	AlignSpaceBetween // align-content only
	AlignSpaceAround  // align-content only
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
)

// Overflow says whether a node clips its children.
type Overflow uint8

const (
	Visible Overflow = iota + 1
	Hidden
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
	set(&s.Top, o.Top)
	set(&s.Right, o.Right)
	set(&s.Bottom, o.Bottom)
	set(&s.Left, o.Left)
	set(&s.Margin, o.Margin)
	set(&s.Padding, o.Padding)
	set(&s.Gap, o.Gap)
	set(&s.RowGap, o.RowGap)
	set(&s.ColumnGap, o.ColumnGap)
	set(&s.Background, o.Background)
	set(&s.CornerRadius, o.CornerRadius)
	set(&s.BorderWidth, o.BorderWidth)
	set(&s.BorderColor, o.BorderColor)
	set(&s.Opacity, o.Opacity)
	set(&s.Overflow, o.Overflow)
	set(&s.ShadowColor, o.ShadowColor)
	set(&s.ShadowOffset, o.ShadowOffset)
	set(&s.ShadowRadius, o.ShadowRadius)
	s.mergeText(o)
	set(&s.Hover, o.Hover)
	set(&s.Focus, o.Focus)
	set(&s.Active, o.Active)
	set(&s.Disabled, o.Disabled)
	return s
}

// Interaction states a node's style depends on.
const (
	statePressed uint8 = 1 << iota
	stateHovered
	stateFocused
)

// withStates returns s with the state styles of the states merged in.
func (s Style) withStates(states uint8, disabled bool) Style {
	out := s
	if s.Hover != nil && states&stateHovered != 0 {
		out = out.Merge(*s.Hover)
	}
	if s.Focus != nil && states&stateFocused != 0 {
		out = out.Merge(*s.Focus)
	}
	if s.Active != nil && states&statePressed != 0 {
		out = out.Merge(*s.Active)
	}
	if s.Disabled != nil && disabled {
		out = out.Merge(*s.Disabled)
	}
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
		TextAlign: s.TextAlign, TextDecoration: s.TextDecoration,
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
	return ViewStyle{
		Background:   s.Background,
		CornerRadius: s.CornerRadius,
		BorderWidth:  s.BorderWidth,
		BorderColor:  s.BorderColor,
		Opacity:      s.Opacity.Value(),
		Clip:         s.Overflow == Hidden,
		ShadowColor:  s.ShadowColor,
		ShadowOffset: s.ShadowOffset,
		ShadowRadius: s.ShadowRadius,
	}
}
