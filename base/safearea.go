package base

import "github.com/oneo-me/nsgo/core"

// Side is a set of edges.
type Side uint8

const (
	Top Side = 1 << iota
	Right
	Bottom
	Left
	// AllSides is every edge, what a zero Side means.
	AllSides = Top | Right | Bottom | Left
)

// SafeAreaView keeps its content clear of system UI: it adds the insets
// of the nearest screen or modal, or the surface's safe area, to its
// padding on Edges (every edge when zero).
type SafeAreaView struct {
	Edges    Side
	Style    core.Style
	Children []core.Node
	TestID   string
}

// Render implements core.Node.
func (v SafeAreaView) Render(cx *core.Scope) core.Node {
	edges := v.Edges
	if edges == 0 {
		edges = AllSides
	}
	return core.View{
		Style: v.Style,
		StyleFunc: func() core.Style {
			in, p := core.Insets(cx), v.Style.Padding
			if edges&Top != 0 {
				p.Top += in.Top
			}
			if edges&Right != 0 {
				p.Right += in.Right
			}
			if edges&Bottom != 0 {
				p.Bottom += in.Bottom
			}
			if edges&Left != 0 {
				p.Left += in.Left
			}
			return core.Style{Padding: p}
		},
		Children: v.Children,
		TestID:   v.TestID,
	}
}

// AvoidBehavior is how a KeyboardAvoidingView makes room for the keyboard.
type AvoidBehavior uint8

const (
	// AvoidPadding adds the keyboard's height to the bottom padding.
	AvoidPadding AvoidBehavior = iota
	// AvoidHeight takes the keyboard's height off the view with a bottom
	// margin, so a growing view shrinks.
	AvoidHeight
)

// KeyboardAvoidingView keeps its content above the software keyboard.
type KeyboardAvoidingView struct {
	Behavior AvoidBehavior
	// Offset is how much of the keyboard's height is already covered by
	// what lies below the view, such as a tab bar.
	Offset   core.Points
	Disabled bool
	Style    core.Style
	Children []core.Node
	TestID   string
}

// Render implements core.Node.
func (v KeyboardAvoidingView) Render(cx *core.Scope) core.Node {
	return core.View{
		Style: v.Style,
		StyleFunc: func() core.Style {
			if v.Disabled {
				return core.Style{}
			}
			k := max(cx.Env().KeyboardHeight()-v.Offset, 0)
			if v.Behavior == AvoidHeight {
				m := v.Style.Margin
				m.Bottom += k
				return core.Style{Margin: m}
			}
			p := v.Style.Padding
			p.Bottom += k
			return core.Style{Padding: p}
		},
		Children: v.Children,
		TestID:   v.TestID,
	}
}

// SelectionArea makes the texts inside select together, as a web page's
// do: one selection runs from a place in one text to a place in another,
// across views. On desktops it is made by dragging, a double click (a
// word) or a triple click (a whole text), and extended by Shift-clicks;
// on touch screens by a long press. Cmd or Ctrl+A selects the area, Cmd
// or Ctrl+C copies and Escape clears. Texts with SelectableOff stay out,
// and an area inside another is part of the outer one. It has no look of
// its own: the highlight is each text's SelectionColor.
type SelectionArea struct {
	Style    core.Style
	Children []core.Node
	TestID   string
}

// Render implements core.Node.
func (a SelectionArea) Render(*core.Scope) core.Node {
	return core.View{SelectionArea: true, Style: a.Style, Children: a.Children, TestID: a.TestID}
}
