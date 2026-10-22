package ui

import "github.com/oneo-me/nsgo/core"

// Sidebar is a column of SidebarSections for an app's navigation. It
// paints no background, so the surface it sits on shows; put it in a
// core.Scroll when it can outgrow its room. The app owns the selection.
type Sidebar struct {
	Style    core.Style
	Children []core.Node
	Label    string
	TestID   string
}

// Render implements core.Node.
func (s Sidebar) Render(*core.Scope) core.Node {
	return core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 12, Left: 12}, Gap: 12}.Merge(s.Style),
		Children: s.Children, Label: s.Label, TestID: s.TestID}
}

// SidebarSection is a muted heading over its items. Actions, such as an
// add button, end the heading's row; IconButtons fit it.
type SidebarSection struct {
	Title    string
	Actions  []core.Node
	Style    core.Style
	Children []core.Node
	TestID   string
}

// Render implements core.Node.
func (s SidebarSection) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	heading := core.Text{Content: s.Title, Role: core.RoleHeader, MaxLines: 1, StyleFunc: func() core.Style {
		t := theme()
		return core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.Symmetric(4, 8),
			FontSize: t.small(), FontWeight: core.FontWeightSemibold, Color: t.MutedForeground}
	}}
	var head core.Node = heading
	if len(s.Actions) > 0 {
		head = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Height: core.Pt(28)},
			Children: append([]core.Node{heading}, s.Actions...)}
	}
	var children []core.Node
	if s.Title != "" {
		children = append(children, head)
	}
	return core.View{Style: core.Style{Gap: 2}.Merge(s.Style), TestID: s.TestID,
		Children: append(children, s.Children...)}
}

// SidebarItem is a navigation button: an icon in the primary color, then
// its label, then Trailing, such as a count. Selected gives it a neutral
// face; hovering does not change it.
type SidebarItem struct {
	Label    string
	Icon     *core.SVG
	Selected func() bool
	Disabled func() bool
	OnPress  func()
	Trailing []core.Node
	Style    core.Style
	TestID   string
}

// Render implements core.Node.
func (i SidebarItem) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	var children []core.Node
	if i.Icon != nil {
		children = append(children, core.Icon{SVG: i.Icon, StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(18), Height: core.Pt(18), FlexShrink: 0, Color: theme().Primary}
		}})
	}
	children = append(children, core.Text{Content: i.Label, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1}})
	selected := orFalse(i.Selected)
	return core.View{
		OnPress: i.OnPress, Disabled: i.Disabled, Selected: selected,
		Role: core.RoleButton, Label: i.Label, TestID: i.TestID,
		StyleFunc: look(cx, i.Style, Tokens.sidebarItem),
		Children:  append(children, i.Trailing...),
	}
}

func (t Tokens) sidebarItem() core.Style {
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(7, 8),
		CornerRadius: t.Radius, Color: t.Foreground, FontSize: t.FontSize,
		Active:   &core.Style{Background: t.Tint(0.06)},
		Selected: &core.Style{Background: t.Tint(0.1)},
		Disabled: disabled}
}
