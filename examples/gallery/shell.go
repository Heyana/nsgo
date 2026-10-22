package main

import (
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// The desktop shell follows Onevra's: a sidebar of grouped sections with
// a search field and a toolbar of settings and appearance, and a header
// over the content naming the section. The system's title bar is hidden,
// so the sidebar's and the header's top rows are the window's drag handle,
// clear of its controls.

// headerHeight is the height of the rows that stand in for the title bar.
const headerHeight core.Points = 48

// sidebarWidth is the sidebar's width while it shows.
const sidebarWidth core.Points = 248

// desktop is the wide layout: the sidebar beside the header and the
// selected section.
func (g *gallery) desktop(cx *core.Scope) core.Node {
	g.wide = true
	// Coming from the phone layout, show the section it had open.
	if routes := core.Untracked(cx, g.nav.Routes); len(routes) > 1 {
		g.section.Set(routes[len(routes)-1])
	}
	theme := ui.UseTheme(cx)
	env := cx.Env()

	// width follows the sidebar's toggle, animated where motion is welcome.
	width := core.NewSignal(cx, core.Points(0))
	if core.Untracked(cx, g.sidebar.Get) {
		width.Set(sidebarWidth)
	}
	var stop func()
	cx.Effect(func() {
		to := core.Points(0)
		if g.sidebar.Get() {
			to = sidebarWidth
		}
		from := width.GetUntracked()
		if stop != nil {
			stop()
		}
		if from == to || core.Untracked(cx, env.ReduceMotion) {
			width.Set(to)
			return
		}
		stop = cx.Animate(220*time.Millisecond, core.EaseInOut, func(p float64) {
			width.Set(from + (to-from)*core.Points(p))
		})
	})

	content := &core.Ref{}
	viewport := core.NewSignal(cx, core.Points(0))
	cx.Effect(func() {
		g.section.Get()
		content.ScrollTo(core.Point{}, false)
	})
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Direction: core.Row}, Children: []core.Node{
		core.Show{When: func() bool { return width.Get() > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: "sidebar",
				StyleFunc: func() core.Style {
					t := theme()
					return core.Style{Width: core.Pt(width.Get()), FlexShrink: 0, Overflow: core.Hidden,
						Background: sidebarColor(t), Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: t.Border}}
				},
				// The sidebar's content keeps its width while the column opens.
				Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(sidebarWidth), FlexGrow: 1}, Children: []core.Node{
					g.sidebarHeader(cx),
					g.searchField(cx),
					g.navigation(),
					g.toolbar(),
				}}}}
		}},
		// Base pages sit on their own black and white.
		core.View{TestID: "page", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: func() core.Style {
			if s, _ := find(g.section.Get()); s.kind == kindBase {
				return core.Style{Background: monoTheme(g.dark()).background}
			}
			return core.Style{}
		}, Children: []core.Node{
			g.header(cx),
			core.Scroll{Ref: content, TestID: "content", Style: core.Style{FlexGrow: 1, FlexShrink: 1},
				OnLayout: func(b core.Bounds) { viewport.Set(b.Size.Height) },
				Children: []core.Node{g.page(viewport.Get)}},
		}},
	}}
}

// sidebarColor is the sidebar's surface: the background faintly tinted
// with the text, as Onevra's navigation is.
func sidebarColor(t ui.Tokens) core.Color { return ui.Mix(t.Background, t.Foreground, 0.035) }

// titleRow is a row standing in for the title bar: as tall as the window
// controls' area and a drag handle. pad gives its padding from the area
// the controls keep, so it can clear them.
func titleRow(cx *core.Scope, testID string, pad func(controls core.Edges) core.Edges, children ...core.Node) core.Node {
	env := cx.Env()
	return core.View{WindowDrag: core.WindowDragOn, TestID: testID, Children: children,
		StyleFunc: func() core.Style {
			tb := env.TitleBar()
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexShrink: 0,
				Height: core.Pt(max(headerHeight, tb.Top)), Padding: pad(tb)}
		}}
}

// undraggable keeps a control in a title row pressable.
func undraggable(n core.Node) core.Node {
	return core.View{WindowDrag: core.WindowDragOff, Children: []core.Node{n}}
}

func (g *gallery) sidebarHeader(cx *core.Scope) core.Node {
	return titleRow(cx, "sidebar-header", func(tb core.Edges) core.Edges { return core.Edges{Left: max(tb.Left, 4) + 8, Right: 8} },
		undraggable(ui.IconButton{Icon: lucide.PanelLeft(), Label: "Hide sidebar", OnPress: g.toggleSidebar, TestID: "hide-sidebar"}))
}

// searchField is the sidebar's search trigger: it looks like a field and
// opens the command palette.
func (g *gallery) searchField(cx *core.Scope) core.Node {
	theme := ui.UseTheme(cx)
	return core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 10, Left: 12}}, Children: []core.Node{
		base.Button{OnPress: g.openPalette, Label: "Search", TestID: "search",
			StyleFunc: func() core.Style {
				t := theme()
				face := func(tint, border float64) core.Style {
					return core.Style{Background: ui.Fade(ui.Mix(t.Background, t.Foreground, tint), 0.6), BorderColor: ui.Fade(t.Foreground, border)}
				}
				rest, hover, press := face(0.08, 0.18), face(0.12, 0.3), face(0.18, 0.4)
				return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Height: core.Pt(34),
					Padding: core.Symmetric(0, 10), CornerRadius: t.Radius, BorderWidth: 1,
					Background: rest.Background, BorderColor: rest.BorderColor, Color: t.MutedForeground,
					Hover: &hover, Active: &press}
			},
			Children: []core.Node{
				core.Icon{SVG: lucide.Search(), Style: core.Style{Width: core.Pt(16), Height: core.Pt(16)}},
				core.Text{Content: "Search…", MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
				core.Text{Content: shortcut(cx, "K"), Style: core.Style{FontSize: 12}},
			}},
	}}
}

// shortcut is how the platform writes mod+key.
func shortcut(cx *core.Scope, key string) string {
	if m := modName(cx); m != "⌘" {
		return m + "+" + key
	}
	return "⌘" + key
}

// navigation is the sidebar's list: Welcome, then tabs choosing Base or
// UI over that category's sections by group.
func (g *gallery) navigation() core.Node {
	item := func(s section) core.Node {
		return ui.SidebarItem{Label: s.name, Icon: s.icon(), TestID: "nav-" + s.name,
			Selected: func() bool { return g.section.Get() == s.name }, OnPress: func() { g.open(s.name) }}
	}
	tab := func(k kind) core.Node {
		return ui.Tab{Label: k.label(), TestID: "tab-" + string(k), Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter},
			Selected: func() bool { return g.tab.Get() == k }, OnPress: func() { g.tab.Set(k) }}
	}
	list := func(k kind) core.Node {
		var sections []core.Node
		for _, group := range groups(k) {
			var items []core.Node
			for _, s := range group {
				items = append(items, item(s))
			}
			sections = append(sections, ui.SidebarSection{Title: group[0].group, Children: items})
		}
		return ui.Sidebar{Label: k.label(), Children: sections}
	}
	welcome, _ := find("Welcome")
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 10, Left: 12}, Gap: 10}, Children: []core.Node{
			item(welcome),
			ui.Tabs{Label: "Category", Style: core.Style{AlignSelf: core.AlignStretch},
				Children: []core.Node{tab(kindBase), tab(kindUI)}},
		}},
		core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node { return list(g.tab.Get()) }),
		}},
	}}
}

// toolbar is the sidebar's bottom row: settings, then the appearance.
func (g *gallery) toolbar() core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.All(8), FlexShrink: 0},
		Children: []core.Node{
			ui.IconButton{Icon: lucide.Settings(), Label: "Settings", OnPress: g.openSettings, TestID: "open-settings"},
			core.View{Style: core.Style{FlexGrow: 1}},
			g.appearanceMenu(core.PlaceTop),
		}}
}

// appearanceMenu is an icon button whose menu chooses the appearance.
func (g *gallery) appearanceMenu(place core.Placement) core.Node {
	var items []core.Node
	for _, a := range appearanceOptions() {
		items = append(items, ui.MenuItem{Label: a.Label, Checked: func() bool { return g.appearance.Get() == a.Value },
			OnSelect: func() { g.appearance.Set(a.Value) }})
	}
	return ui.IconButton{Label: "Appearance", TestID: "appearance", Menu: items, Placement: place, Align: core.AlignEnd,
		IconFunc: func() *core.SVG {
			switch g.appearance.Get() {
			case "light":
				return lucide.Sun()
			case "dark":
				return lucide.Moon()
			}
			return lucide.SunMoon()
		}}
}

// header names the section over the content: its group, then its name,
// with the sidebar's toggle while the sidebar is hidden.
func (g *gallery) header(cx *core.Scope) core.Node {
	theme := ui.UseTheme(cx)
	// The title lines up with the content beside the sidebar, and follows
	// the sidebar's toggle past the window controls without it.
	pad := func(tb core.Edges) core.Edges {
		if g.sidebar.Get() {
			return core.Edges{Left: 32, Right: tb.Right + 16}
		}
		return core.Edges{Left: max(tb.Left, 4) + 8, Right: tb.Right + 16}
	}
	return titleRow(cx, "header", pad,
		core.Show{When: func() bool { return !g.sidebar.Get() }, Then: func(*core.Scope) core.Node {
			return undraggable(ui.IconButton{Icon: lucide.PanelLeft(), Label: "Show sidebar", OnPress: g.toggleSidebar, TestID: "show-sidebar"})
		}},
		core.Text{MaxLines: 1,
			ContentFunc: func() string {
				if s, _ := find(g.section.Get()); s.kind != kindNone {
					return s.kind.label() + " / " + s.group + " /"
				}
				return ""
			},
			StyleFunc: func() core.Style {
				return core.Style{Color: theme().MutedForeground, FontWeight: core.FontWeightMedium}
			}},
		core.Text{ContentFunc: g.section.Get, Role: core.RoleHeader, TestID: "title", MaxLines: 1,
			Style: core.Style{FontWeight: core.FontWeightSemibold, FlexShrink: 1}},
	)
}

// page shows the selected section in the gallery's theme for the
// appearance, again when either changes. viewport is the content's height,
// which Welcome fills.
func (g *gallery) page(viewport func() core.Points) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		s, _ := find(g.section.Get())
		t := newTheme(g.dark(), s.kind)
		return core.Func(func(cx *core.Scope) core.Node {
			if s.name == "Welcome" {
				return core.View{StyleFunc: func() core.Style { return core.Style{MinHeight: core.Pt(viewport())} },
					Children: []core.Node{s.page(cx, t)}}
			}
			return core.View{Style: core.Style{Padding: core.Edges{Top: 8, Right: 32, Bottom: 40, Left: 32}, MaxWidth: core.Pt(880)},
				Children: []core.Node{g.sectionBody(cx, t, s)}}
		})
	})
}

// sectionBody is a section's demos under its description.
func (g *gallery) sectionBody(cx *core.Scope, t *theme, s section) core.Node {
	about := core.Show{When: func() bool { return g.descriptions.Get() && s.about != "" }, Then: func(*core.Scope) core.Node {
		return core.Text{Content: s.about, TestID: "about", Style: core.Style{Color: t.muted, FontSize: 15}}
	}}
	return core.View{Style: core.Style{Gap: 16}, TestID: "section-" + s.name,
		Children: []core.Node{about, s.page(cx, t)}}
}

// phone is the narrow layout: a stack of the index and the section opened
// over it.
func (g *gallery) phone(cx *core.Scope) core.Node {
	// Coming from the wide layout, keep its section open.
	if g.wide {
		g.nav.SetRoutes("", g.section.GetUntracked())
	}
	g.wide = false
	theme := ui.UseTheme(cx)
	return g.nav.Stack(core.Style{FlexGrow: 1},
		func(cx *core.Scope, name string, _ func() int) core.Screen {
			if name == "" {
				return core.Screen{Title: "Gallery", TestID: "index",
					Header: g.phoneHeader(cx, "Gallery", nil,
						ui.IconButton{Icon: lucide.Search(), Label: "Search", OnPress: g.openPalette, NoTooltip: true},
						ui.IconButton{Icon: lucide.Settings(), Label: "Settings", OnPress: g.openSettings, NoTooltip: true}),
					Children: []core.Node{g.index(cx)}}
			}
			s, _ := find(name)
			body := core.Dynamic(func(cx *core.Scope) core.Node {
				t := newTheme(g.dark(), s.kind)
				return core.Func(func(cx *core.Scope) core.Node {
					if s.name == "Welcome" {
						return s.page(cx, t)
					}
					return base.SafeAreaView{Edges: base.Bottom, Style: core.Style{Padding: core.All(16)},
						Children: []core.Node{g.sectionBody(cx, t, s)}}
				})
			})
			return core.Screen{Title: s.name, Header: g.phoneHeader(cx, s.name, func() { g.nav.Pop(1) }), TestID: "screen-" + s.name,
				Children: []core.Node{core.Scroll{Style: core.Style{FlexGrow: 1}, StyleFunc: func() core.Style {
					if s.kind == kindBase {
						return core.Style{Background: monoTheme(g.dark()).background}
					}
					return core.Style{Background: theme().Background}
				}, Children: []core.Node{body}}}}
		})
}

// phoneHeader is a phone screen's bar: a back button, the title and
// actions.
func (g *gallery) phoneHeader(cx *core.Scope, title string, back func(), actions ...core.Node) core.Node {
	theme := ui.UseTheme(cx)
	var children []core.Node
	if back != nil {
		children = append(children, ui.IconButton{Icon: lucide.ChevronLeft(), Label: "Back", OnPress: back, NoTooltip: true, Size: 20})
	}
	children = append(children, core.Text{Content: title, Role: core.RoleHeader, MaxLines: 1,
		Style: core.Style{FontSize: 18, FontWeight: core.FontWeightSemibold, FlexGrow: 1, FlexShrink: 1}})
	return core.View{
		StyleFunc: func() core.Style {
			t := theme()
			return core.Style{Background: sidebarColor(t), Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.Border}}
		},
		Children: []core.Node{base.SafeAreaView{Edges: base.Top | base.Left | base.Right, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
				Height: core.Pt(52), Padding: core.Symmetric(0, 8)}, Children: append(children, actions...)}}}}}
}

// index is the phone layout's list of sections by group.
func (g *gallery) index(cx *core.Scope) core.Node {
	theme := ui.UseTheme(cx)
	var items []core.Node
	var all [][]section
	for _, k := range []kind{kindNone, kindBase, kindUI} {
		all = append(all, groups(k)...)
	}
	for _, group := range all {
		heading := group[0].group
		if k := group[0].kind; k != kindNone {
			heading = k.label() + " · " + heading
		}
		items = append(items, core.Text{Content: heading, Role: core.RoleHeader, StyleFunc: func() core.Style {
			t := theme()
			return core.Style{Padding: core.Edges{Top: 18, Right: 16, Bottom: 6, Left: 16}, FontSize: 13,
				FontWeight: core.FontWeightSemibold, Color: t.MutedForeground}
		}})
		for _, s := range group {
			items = append(items, core.View{OnPress: func() { g.open(s.name) }, Role: core.RoleButton, Label: s.name, TestID: "nav-" + s.name,
				StyleFunc: func() core.Style {
					t := theme()
					return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 16),
						Background: t.Surface, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.Border},
						Active: &core.Style{Background: t.Accent}}
				},
				Children: []core.Node{
					core.Icon{SVG: s.icon(), StyleFunc: func() core.Style {
						return core.Style{Width: core.Pt(20), Height: core.Pt(20), FlexShrink: 0, Color: theme().Primary}
					}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
						core.Text{Content: s.name, Style: core.Style{FontSize: 15, FontWeight: core.FontWeightMedium}},
						core.Show{When: func() bool { return s.about != "" }, Then: func(*core.Scope) core.Node {
							return core.Text{Content: s.about, MaxLines: 1, StyleFunc: func() core.Style {
								return core.Style{Color: theme().MutedForeground, FontSize: 13}
							}}
						}},
					}},
					core.Icon{SVG: lucide.ChevronRight(), StyleFunc: func() core.Style {
						return core.Style{Width: core.Pt(16), Height: core.Pt(16), Color: theme().MutedForeground}
					}},
				}})
		}
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1}, StyleFunc: func() core.Style { return core.Style{Background: theme().Background} },
		Children: []core.Node{base.SafeAreaView{Edges: base.Bottom, Children: items}}}
}
