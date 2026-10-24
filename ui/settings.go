package ui

import (
	"strings"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
)

// SettingsCategory is a page of settings, listed in the settings' sidebar
// under its Section. IDs must be unique and stay the same across languages.
type SettingsCategory struct {
	ID, Title, Section string
	// Description shows above the page's groups.
	Description string
	Icon        *core.SVG
	Groups      []SettingsGroup
}

// SettingsGroup groups related items under an optional Title. Actions,
// such as a reset button, end the title's row; search does not read them.
type SettingsGroup struct {
	ID, Title string
	Actions   []core.Node
	Items     []SettingsItem
}

// SettingsItem is a row of settings: its title and description, then its
// Control. The app owns the values the control shows and changes. IDs
// must be unique within their group.
type SettingsItem struct {
	ID, Title, Description string
	// Keywords are words search finds the item by without showing them.
	Keywords []string
	// Leading comes before the text, such as an icon.
	Leading core.Node
	// Control follows the text, or sits below it when Stacked or when the
	// settings are narrow: Stacked suits wide controls such as fields.
	Control core.Node
	Stacked bool
	// Content replaces the row with the app's own, spanning its width,
	// such as a profile card; Title, Description and Keywords are still
	// what search finds.
	Content core.Node
}

// SettingsResult is an item of a search: indexes into the categories, a
// category's groups and a group's items.
type SettingsResult struct{ Category, Group, Item int }

// SearchSettings returns the items every whitespace-separated term of
// query occurs in, ignoring case, in their order: in the item's title,
// description or keywords, or its group's or category's title. An empty
// query returns every item.
func SearchSettings(categories []SettingsCategory, query string) []SettingsResult {
	terms := strings.Fields(strings.ToLower(query))
	var out []SettingsResult
	for ci, c := range categories {
		for gi, g := range c.Groups {
			for ii, it := range g.Items {
				hay := strings.ToLower(strings.Join([]string{c.Title, c.Section, c.Description, g.Title,
					it.Title, it.Description, strings.Join(it.Keywords, " ")}, " "))
				if matchAll(hay, terms) {
					out = append(out, SettingsResult{ci, gi, ii})
				}
			}
		}
	}
	return out
}

func matchAll(hay string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// Settings is a settings surface: a sidebar of categories with a search
// field beside the selected category's page, or every category's items
// matching the search. It fills its parent, which gives it a height and
// its surface, such as a dialog or a window. Below NarrowWidth a selector
// replaces the sidebar and controls sit below their text.
type Settings struct {
	Categories []SettingsCategory
	// Category is the selected category's ID and OnCategoryChange receives
	// the one chosen. Without Category the settings keep their own,
	// starting at DefaultCategory or else the first.
	Category         func() string
	DefaultCategory  string
	OnCategoryChange func(id string)

	// The texts default to English.
	Title, SearchPlaceholder, ResultsTitle, EmptyText, CloseLabel string
	// HeaderActions end the page's heading, before the close button.
	HeaderActions []core.Node
	// OnClose, when set, shows a close button that calls it, for settings
	// shown in a dialog.
	OnClose func()
	// HideSearch leaves out the search field; HideSidebar the category
	// navigation, showing every category on one page.
	HideSearch, HideSidebar bool
	// NarrowWidth is 560 when 0.
	NarrowWidth core.Points
	Style       core.Style
	TestID      string
}

// Render implements core.Node.
func (s Settings) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	title := orString(s.Title, "Settings")
	results := orString(s.ResultsTitle, "Search results")
	narrowWidth := s.NarrowWidth
	if narrowWidth == 0 {
		narrowWidth = 560
	}

	own := core.NewSignal(cx, s.DefaultCategory)
	category := s.Category
	if category == nil {
		category = own.Get
	}
	selected := func() int {
		id := category()
		for i, c := range s.Categories {
			if c.ID == id {
				return i
			}
		}
		if len(s.Categories) == 0 {
			return -1
		}
		return 0
	}
	query := core.NewSignal(cx, "")
	choose := func(id string) {
		query.Set("")
		if s.Category == nil {
			own.Set(id)
		}
		if s.OnCategoryChange != nil {
			s.OnCategoryChange(id)
		}
	}
	width := core.NewSignal(cx, core.Points(0))
	narrow := core.NewMemo(cx, func() bool { w := width.Get(); return w > 0 && w < narrowWidth })
	searching := func() bool { return !s.HideSearch && strings.TrimSpace(query.Get()) != "" }

	search := func() core.Node {
		return Input{Value: query.Get, OnChange: query.Set, Icon: lucide.Search(),
			Placeholder: orString(s.SearchPlaceholder, "Search settings"), Label: "Search settings", TestID: "settings-search"}
	}
	heading := func(text func() string, actions bool) core.Node {
		children := []core.Node{core.Text{ContentFunc: text, Role: core.RoleHeader, MaxLines: 1,
			Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontWeight: core.FontWeightSemibold}}}
		if actions {
			children = append(children, s.HeaderActions...)
			if s.OnClose != nil {
				children = append(children, IconButton{Icon: lucide.X(), Label: orString(s.CloseLabel, "Close settings"),
					OnPress: func(core.PressEvent) { s.OnClose() }, NoTooltip: true, TestID: "settings-close"})
			}
		}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexShrink: 0,
			MinHeight: core.Pt(56), Padding: core.Symmetric(10, 16)}, Children: children}
	}

	nav := func() core.Node {
		var sections []core.Node
		for start := 0; start < len(s.Categories); {
			section := s.Categories[start].Section
			end := start + 1
			for end < len(s.Categories) && s.Categories[end].Section == section {
				end++
			}
			var items []core.Node
			for i := start; i < end; i++ {
				c := s.Categories[i]
				items = append(items, SidebarItem{Label: c.Title, Icon: c.Icon, TestID: "settings-category-" + c.ID,
					Selected: func() bool { return selected() == i }, OnPress: func(core.PressEvent) { choose(c.ID) }})
			}
			sections = append(sections, SidebarSection{Title: section, Children: items})
			start = end
		}
		children := []core.Node{heading(func() string { return title }, false)}
		if !s.HideSearch {
			children = append(children, core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 16, Left: 12}},
				Children: []core.Node{search()}})
		}
		children = append(children, core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{Sidebar{Children: sections}}})
		return core.View{
			StyleFunc: func() core.Style {
				t := theme()
				return core.Style{Width: core.Pt(216), FlexShrink: 0, Background: t.Tint(0.035),
					Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: t.Border}}
			},
			Children: children}
	}

	// bar is the narrow layout's category selector and search field.
	bar := func() core.Node {
		var children []core.Node
		if !s.HideSidebar && len(s.Categories) > 0 {
			options := make([]SelectOption, len(s.Categories))
			for i, c := range s.Categories {
				options[i] = SelectOption{Value: c.ID, Label: c.Title, Icon: c.Icon}
			}
			children = append(children, Select{Options: options, OnChange: choose, Label: "Category", TestID: "settings-category",
				Value: func() string { return s.Categories[max(selected(), 0)].ID }, Style: core.Style{AlignSelf: core.AlignStretch}})
		}
		if !s.HideSearch {
			children = append(children, search())
		}
		return core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 12, Left: 12}, Gap: 8}, Children: children}
	}

	page := core.Dynamic(func(cx *core.Scope) core.Node {
		q, compact, sel := "", narrow.Get(), selected()
		if !s.HideSearch {
			q = query.Get()
		}
		find := strings.TrimSpace(q) != ""
		all := find || s.HideSidebar
		muted := func() core.Style {
			t := theme()
			return core.Style{Color: t.MutedForeground, FontSize: core.Points(float32(t.FontSize)*0.85 + 0.5)}
		}
		var rows []core.Node
		lastCategory, lastGroup := -1, -1
		for _, r := range SearchSettings(s.Categories, q) {
			if !all && r.Category != sel {
				continue
			}
			c := s.Categories[r.Category]
			g := c.Groups[r.Group]
			if lastCategory != r.Category {
				if find || (all && len(s.Categories) > 1) {
					rows = append(rows, core.Text{Content: c.Title, Role: core.RoleHeader,
						Style: core.Style{FontWeight: core.FontWeightSemibold}})
				}
				if c.Description != "" {
					rows = append(rows, core.Text{Content: c.Description, StyleFunc: muted})
				}
			}
			if (lastCategory != r.Category || lastGroup != r.Group) && (g.Title != "" || len(g.Actions) > 0) {
				head := append([]core.Node{core.Text{Content: g.Title, Style: core.Style{FlexGrow: 1, FlexShrink: 1}}}, g.Actions...)
				rows = append(rows, core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: head},
					core.View{StyleFunc: func() core.Style { return core.Style{Height: core.Pt(1), Background: theme().Border} }},
				}})
			}
			lastCategory, lastGroup = r.Category, r.Group
			rows = append(rows, settingsRow(g.Items[r.Item], compact, muted))
		}
		if len(rows) == 0 {
			rows = append(rows, core.Text{Content: orString(s.EmptyText, "No matching settings"), StyleFunc: muted})
		}
		return core.View{Style: core.Style{Padding: core.Edges{Top: 4, Right: 20, Bottom: 24, Left: 20}, Gap: 20}, Children: rows}
	})

	main := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
		heading(func() string {
			switch i := selected(); {
			case searching():
				return results
			case narrow.Get() || s.HideSidebar || i < 0:
				return title
			default:
				return s.Categories[i].Title
			}
		}, true),
		core.Show{When: func() bool { return narrow.Get() && !(s.HideSidebar && s.HideSearch) }, Then: func(*core.Scope) core.Node { return bar() }},
		core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{page}},
	}}
	return core.View{
		Style:    core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}.Merge(s.Style),
		OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
		TestID:   s.TestID,
		Children: []core.Node{
			core.Show{When: func() bool { return !narrow.Get() && !s.HideSidebar }, Then: func(*core.Scope) core.Node { return nav() }},
			main,
		},
	}
}

// settingsRow is an item's row: its text, then its control beside it or
// below it.
func settingsRow(it SettingsItem, compact bool, muted func() core.Style) core.Node {
	if it.Content != nil {
		return core.View{TestID: "setting-" + it.ID, Children: []core.Node{it.Content}}
	}
	var lines []core.Node
	if it.Title != "" {
		lines = append(lines, core.Text{Content: it.Title})
	}
	if it.Description != "" {
		lines = append(lines, core.Text{Content: it.Description, StyleFunc: muted})
	}
	var text core.Node = core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 4}, Children: lines}
	if it.Leading != nil {
		text = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, FlexGrow: 1, FlexShrink: 1,
			MinWidth: core.Pt(0)}, Children: []core.Node{it.Leading, text}}
	}
	style := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16}
	if compact || it.Stacked {
		style = core.Style{Gap: 10}
	}
	children := []core.Node{text}
	if it.Control != nil {
		// The wrapper centers a control that aligns itself to the start.
		children = append(children, core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{it.Control}})
	}
	return core.View{Style: style, TestID: "setting-" + it.ID, Children: children}
}
