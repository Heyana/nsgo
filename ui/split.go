package ui

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// Split is a base.Split whose divider is the theme's border color. It
// takes the same sizes, limits and reset; see base.Split for their units
// and behavior. Style merges over the split's root and LineStyle over the
// line the theme draws; LineStyleFunc merges after both, for a line whose
// look changes.
type Split struct {
	Direction base.SplitDirection
	First     core.Node
	Second    core.Node

	Size          func() core.Length
	DefaultSize   core.Length
	Min, Max      core.Length
	OnResize      func(size core.Length)
	Disabled      func() bool
	Thickness     core.Points
	Style         core.Style
	LineStyle     core.Style
	LineStyleFunc func() core.Style

	Label  string
	Hint   string
	TestID string
}

// Render implements core.Node.
func (s Split) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	return base.Split{
		Direction: s.Direction, First: s.First, Second: s.Second,
		Size: s.Size, DefaultSize: s.DefaultSize, Min: s.Min, Max: s.Max,
		OnResize: s.OnResize, Disabled: s.Disabled, Thickness: s.Thickness,
		Style: s.Style,
		LineStyleFunc: func() core.Style {
			style := core.Style{Background: theme().Border}.Merge(s.LineStyle)
			if s.LineStyleFunc != nil {
				style = style.Merge(s.LineStyleFunc())
			}
			return style
		},
		Label: s.Label, Hint: s.Hint, TestID: s.TestID,
	}
}
