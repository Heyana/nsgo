package core

// FlexDirection is the main axis of a container, as in CSS.
type FlexDirection uint8

const (
	// Column lays children out top to bottom, the default as in React Native.
	Column FlexDirection = iota
	// Row lays children out left to right.
	Row
)

// Align places children on the cross axis (align-items, align-self).
type Align uint8

const (
	// AlignAuto takes the parent's AlignItems; on AlignItems it means stretch.
	AlignAuto Align = iota
	AlignStretch
	AlignStart
	AlignCenter
	AlignEnd
)

// Justify places children on the main axis (justify-content).
type Justify uint8

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
)

// FontWeight is a CSS font weight from 100 to 900; 0 means normal.
type FontWeight uint16

const (
	FontWeightNormal FontWeight = 400
	FontWeightBold   FontWeight = 700
)

// Style is a node's layout and look. The zero value has no look of its
// own: NSGo draws nothing the app does not ask for.
type Style struct {
	Width, Height  Points // 0 or Auto: sized by the layout
	Padding        Edges
	Gap            Points
	Direction      FlexDirection
	JustifyContent Justify
	AlignItems     Align
	AlignSelf      Align
	FlexGrow       float32

	Background   Color
	CornerRadius Points

	// Text styles apply to Text and Input and inherit from no one.
	Color      Color
	FontSize   Points // 0 means 17
	FontWeight FontWeight
}

// DefaultFontSize is the font size of text that sets none.
const DefaultFontSize Points = 17

func (s *Style) normalize() {
	if s.Width == 0 {
		s.Width = Auto
	}
	if s.Height == 0 {
		s.Height = Auto
	}
}

// Font is what a text engine needs to shape text.
type Font struct {
	Size   Points
	Weight FontWeight
}

func (s Style) font() Font {
	f := Font{Size: s.FontSize, Weight: s.FontWeight}
	if f.Size == 0 {
		f.Size = DefaultFontSize
	}
	if f.Weight == 0 {
		f.Weight = FontWeightNormal
	}
	return f
}
