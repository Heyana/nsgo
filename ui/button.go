package ui

import (
	"cmp"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// ButtonVariant is a button's look, one of ui's.
type ButtonVariant uint8

const (
	// ButtonPrimary is filled with the accent, for the main action. It is
	// the default.
	ButtonPrimary ButtonVariant = iota
	// ButtonSecondary is a quiet fill of the text's color at a low opacity.
	ButtonSecondary
	// ButtonOutline is only a rim.
	ButtonOutline
	// ButtonGhost has no rim or fill at rest, only a fill under the pointer
	// and held down, as an IconButton's, such as a menu's trigger in a
	// toolbar.
	ButtonGhost
	// ButtonTinted uses accent text on a quiet accent fill.
	ButtonTinted
	// ButtonDanger uses danger text on a quiet danger fill.
	ButtonDanger
)

// Button is ui's button, as tall as its Size: Text with an optional
// leading Icon, or the Icon alone in a square. base.Button gives the
// behavior; the look is ui's.
type Button struct {
	Ref        *core.Ref
	LabelledBy []*core.Ref
	Text       string
	TextFunc   func() string
	// TextStyle overrides the typography of the button's label.
	TextStyle core.Style
	Variant   ButtonVariant
	// Color fills it with that color instead.
	Color func() core.Color
	Icon  func() *core.SVG
	// Label is what assistive technology reads, Text when empty: give it
	// to a button of an icon alone.
	Label     string
	LabelFunc func() string
	Disabled  func() bool
	// Busy says the button's action is still loading, for assistive technology too: while it returns true a spinner in the button's text color shows over its content, which keeps its room, and presses do nothing, so the action does not start twice.
	Busy func() bool
	// BusyLabel shows beside the spinner and names the busy button for
	// assistive technology. BusyLabelFunc supplies the text when set.
	BusyLabel     string
	BusyLabelFunc func() string
	// AutoFocus moves the keyboard focus here once it mounts, as base.Button's.
	AutoFocus bool
	// Flex grows it to share a row with its siblings.
	Flex float32
	Size Size
	// Style is merged onto the pressable, whose face fills its content box: use it for a full-row button (for example, Width: core.Pct(100)), an app-specific size or margins. The face keeps the variant's look, its corners following Style's CornerRadius.
	Style core.Style
	// Side is the square of a button of an icon alone, its height when 0,
	// to line it up with what is not a control's height. IconSize is its
	// glyph's, the Size's when 0, and Tint its color, the look's when nil.
	Side, IconSize core.Points
	Tint           func() core.Color
	TestID         string
	OnPress        func()
}

// ButtonHeight is a desktop button's height at SizeDefault.
const ButtonHeight core.Points = 28

// Render implements core.Node.
func (b Button) Render(cx *core.Scope) core.Node {
	style, focus, face := b.parts(cx)
	var press func(core.PressEvent)
	if b.OnPress != nil {
		press = func(core.PressEvent) {
			if !is(b.Busy) {
				b.OnPress()
			}
		}
	}
	label, labelFunc := b.labelBinding()
	if b.Busy != nil && (b.BusyLabel != "" || b.BusyLabelFunc != nil) {
		resting, restingFunc := label, labelFunc
		label = ""
		labelFunc = func() string {
			if b.Busy() {
				if busy := b.busyLabel(); busy != "" {
					return busy
				}
			}
			return textValue(resting, restingFunc)
		}
	}
	return base.Button{Ref: b.Ref, LabelledBy: b.LabelledBy, OnPress: press, Disabled: b.Disabled, Busy: b.Busy, AutoFocus: b.AutoFocus,
		Label: label, LabelFunc: labelFunc, TestID: b.TestID, Style: style, StyleFunc: focus, Children: []core.Node{face}}
}

// labelBinding is the button's accessible name while it rests.
func (b Button) labelBinding() (string, func() string) {
	return textBinding(b.Label, b.LabelFunc, b.Text, b.TextFunc)
}

// parts are the button's pressable style, its focus ring and its face,
// which follows the pressable's hover and press: what a popup's trigger
// takes from a Button.
func (b Button) parts(cx *core.Scope) (core.Style, func() core.Style, core.Node) {
	height := b.Size.heightFor(Touch(cx))
	colors := ColorsOf(cx)
	fg := colors.Text
	switch {
	case b.Color != nil:
		fg = Fixed(core.RGB(0xffffff))
	case b.Variant == ButtonPrimary:
		fg = colors.OnAccent
	case b.Variant == ButtonTinted:
		fg = colors.Accent
	case b.Variant == ButtonDanger:
		fg = colors.Danger
	}
	// fill is the face at rest, under the pointer and held down.
	fill := func() (rest, hover, pressed core.Color) {
		switch {
		case b.Color != nil:
			c := b.Color()
			return c, Mix(c, Black(1), 0.12), Mix(c, Black(1), 0.25)
		case b.Variant == ButtonPrimary:
			p := cx.Palette()
			return p.Accent, p.AccentHover, p.AccentPressed
		case b.Variant == ButtonSecondary:
			// The text's color at a low opacity, so it holds over any surface.
			text := colors.Text()
			return Alpha(text, 0.08), Alpha(text, 0.12), Alpha(text, 0.18)
		case b.Variant == ButtonTinted, b.Variant == ButtonDanger:
			tone := fg()
			return Alpha(tone, 0.12), Alpha(tone, 0.18), Alpha(tone, 0.25)
		}
		return 0, colors.Hover(), colors.Pressed()
	}
	var children []core.Node
	if b.Icon != nil {
		glyph, tint := b.IconSize, b.Tint
		if glyph == 0 {
			glyph = b.Size.glyph()
		}
		if tint == nil {
			tint = fg
		}
		children = append(children, Icon(b.Icon, glyph, tint))
	}
	textStyle := core.Style{FontSize: b.Size.text().FontSize, FontWeight: Regular}.Merge(b.TextStyle)
	labelStyle := func() core.Style { return core.Style{Color: cmp.Or(b.TextStyle.Color, fg())} }
	hasText := b.Text != "" || b.TextFunc != nil
	if hasText || b.Icon == nil {
		content := core.Text{Content: b.Text, ContentFunc: b.TextFunc, Style: textStyle, StyleFunc: labelStyle}
		if content.ContentFunc != nil {
			content.Content = ""
		}
		children = append(children, content)
	}
	withBusyLabel := b.Busy != nil && (b.BusyLabel != "" || b.BusyLabelFunc != nil)
	if b.Busy != nil {
		children = b.busyContent(children, textStyle, labelStyle, withBusyLabel)
	}
	shape := core.Style{Height: core.Pt(height), Padding: core.Symmetric(0, b.Size.inset())}
	var minWidth core.Length
	if !hasText && b.Icon != nil {
		side := cmp.Or(b.Side, height)
		shape = core.Style{Width: core.Pt(side), Height: core.Pt(side)}
		if withBusyLabel {
			minWidth, shape.Width = shape.Width, core.Length{}
		}
	}
	style := core.Style{Group: "button", FlexGrow: b.Flex, FlexShrink: b.Flex, CornerRadius: RadiusControl,
		Height: shape.Height, Width: shape.Width, MinWidth: minWidth, Disabled: &core.Style{Opacity: core.Alpha(0.4)}}.Merge(b.Style)
	// Only the pressable takes the app's layout style: repeating a percentage width, margin or padding on the face would size or space it twice. The face's corners follow the pressable's, so the focus ring and the fill agree.
	shape.Height, shape.Width = core.Length{}, core.Length{}
	shape.FlexGrow, shape.FlexShrink, shape.MinHeight = 1, 1, core.Pt(0)
	face := func() core.Style {
		rest, hover, pressed := fill()
		s := core.Style{CornerRadius: style.CornerRadius, BorderCurve: core.BorderCurveContinuous,
			Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6,
			Background: rest,
			GroupStyle: &core.GroupStyle{Hover: &core.Style{Background: hover}, Active: &core.Style{Background: pressed}, Expanded: &core.Style{Background: hover}}}
		// Only an outline has a rim.
		if b.Variant == ButtonOutline && b.Color == nil {
			s.BorderWidth, s.BorderColor = 1, colors.Border()
		}
		return shape.Merge(s)
	}
	return style, func() core.Style { return core.Style{FocusVisible: FocusRing(cx, 2)} }, core.View{StyleFunc: face, Children: children}
}

func (b Button) busyLabel() string {
	if b.BusyLabelFunc != nil {
		return b.BusyLabelFunc()
	}
	return b.BusyLabel
}

func (b Button) busyContent(children []core.Node, textStyle core.Style, labelStyle func() core.Style, withLabel bool) []core.Node {
	row := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6, FlexShrink: 1}
	content := core.View{Style: row, Children: children, AccessibilityElementsHidden: b.Busy,
		StyleFunc: func() core.Style {
			if b.Busy() {
				return core.Style{Opacity: core.Alpha(0)}
			}
			return core.Style{}
		}}
	spinner := Spinner{Size: b.Size.glyph(), Tint: func() core.Color { return labelStyle().Color }}
	if !withLabel {
		return []core.Node{content, core.Show{When: b.Busy, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{spinner}}
		}}}
	}
	// The two rows share a grid cell so the resting content still sets the
	// minimum width while the busy label can make the button wider.
	slot := core.GridLines{Start: core.GridLine(1)}
	row.GridColumn, row.GridRow = slot, slot
	content.Style = row
	busy := core.Show{When: b.Busy, Then: func(*core.Scope) core.Node {
		return core.View{Style: row, Children: []core.Node{spinner,
			core.Show{When: func() bool { return b.busyLabel() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: b.busyLabel, Style: textStyle, StyleFunc: labelStyle}
			}},
		}}
	}}
	return []core.Node{core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "auto", AlignItems: core.AlignCenter,
		JustifyItems: core.AlignCenter, FlexShrink: 1}, Children: []core.Node{content, busy}}}
}
