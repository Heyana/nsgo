package main

import (
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// The desktop shell follows Onevra's: a sidebar of grouped sections with
// a search field and a toolbar of settings and appearance, and a floating
// title row over them naming the section. The system's title bar is
// hidden, so the sidebar and the page paint the window's top edge behind
// the row, the row is the window's drag handle, and its controls clear
// the window's own.

// headerHeight is the height the gallery asks the title row to have; the
// platform's controls can keep more.
const headerHeight core.Points = 48

// sidebarWidth is the sidebar's width while it shows, and how narrow it
// can get; Onevra's gallery grows it up to 520.
const sidebarWidth core.Points = 220

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
	var stop func()
	cx.Effect(func() {
		to := core.Points(0)
		if g.sidebar.Get() {
			to = core.Untracked(cx, g.sidebarSize.Get)
		}
		from := g.sidebarPixels.GetUntracked()
		if stop != nil {
			stop()
		}
		if from == to || core.Untracked(cx, env.ReduceMotion) {
			g.sidebarPixels.Set(to)
			return
		}
		stop = cx.Animate(220*time.Millisecond, core.EaseInOut, func(p float64) {
			g.sidebarPixels.Set(from + (to-from)*core.Points(p))
		})
	})

	content := &core.Ref{}
	viewport := core.NewSignal(cx, core.Points(0))
	cx.Effect(func() {
		g.section.Get()
		content.ScrollTo(core.Point{}, false)
	})
	sidebar := core.Show{When: func() bool { return g.sidebarPixels.Get() > 0 }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "sidebar",
			StyleFunc: func() core.Style {
				t := theme()
				bg := sidebarColor(t)
				if g.material() {
					bg = ui.Fade(bg, 0.72)
				}
				return core.Style{Width: core.Pct(100), FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0),
					Overflow: core.Hidden, Background: bg}
			},
			// The sidebar reaches the top behind the title row, so its
			// content keeps clear of the row; it shrinks with its pane in
			// height, so its list scrolls. Its content keeps the chosen
			// width while the pane animates, so the page covers and
			// uncovers it instead of squeezing it, as Onevra's does.
			Children: []core.Node{core.View{TestID: "sidebar-content", StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(g.sidebarSize.Get()), FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0),
					Padding: core.Edges{Top: titleRowHeight(env)}}
			}, Children: []core.Node{
				g.searchField(cx),
				g.navigation(),
				g.toolbar(),
			}}}}
	}}
	split := ui.Split{
		Direction:   base.SplitHorizontal,
		TestID:      "sidebar-split",
		Label:       "Resize sidebar",
		Hint:        "Drag to resize the sidebar, or double click to restore its default width.",
		Style:       core.Style{FlexGrow: 1, FlexShrink: 1},
		Size:        func() core.Length { return core.Pt(g.sidebarPixels.Get()) },
		DefaultSize: core.Pt(sidebarWidth),
		Min:         core.Pt(sidebarWidth),
		Max:         core.Pt(520),
		Disabled:    func() bool { return !g.sidebar.Get() },
		LineStyleFunc: func() core.Style {
			if g.sidebarPixels.Get() <= 0 {
				return core.Style{Opacity: core.Alpha(0)}
			}
			return core.Style{}
		},
		OnResize: func(size core.Length) {
			width, ok := size.Points()
			if !ok {
				return
			}
			if stop != nil {
				stop()
			}
			g.sidebarPixels.Set(width)
			g.sidebarSize.Set(width)
		},
		First: sidebar,
		Second: core.View{TestID: "page",
			StyleFunc: func() core.Style {
				return core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Background: theme().Background}
			},
			Children: []core.Node{
				core.View{StyleFunc: func() core.Style {
					return core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0),
						Padding: core.Edges{Top: titleRowHeight(env)}}
				}, Children: []core.Node{
					core.Scroll{Ref: content, TestID: "content", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
						OnLayout: func(b core.Bounds) { viewport.Set(b.Size.Height) },
						Children: []core.Node{g.page(viewport.Get)}},
				}},
			}},
	}
	// The title row floats over both regions, which paint under it.
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{split, g.header(cx)}}
}

// sidebarColor is the sidebar's surface: the background faintly tinted
// with the text, as Onevra's navigation is.
func sidebarColor(t ui.Tokens) core.Color { return ui.Mix(t.Background, t.Foreground, 0.035) }

// titleRowHeight is how tall the title row is: Onevra's 48 points, or
// more where the platform's window controls keep a taller bar.
func titleRowHeight(env *core.Env) core.Points { return max(headerHeight, env.TitleBar().Top) }

// titleRow is the row standing in for the title bar: as tall as the
// window controls' area, a drag handle, and drawn over the window behind
// its contents. pad gives its padding from the area the controls keep, so
// it can clear them.
func titleRow(cx *core.Scope, testID string, pad func(controls core.Edges) core.Edges, children ...core.Node) core.Node {
	env := cx.Env()
	return core.View{WindowDrag: core.WindowDragOn, TestID: testID, Children: children,
		StyleFunc: func() core.Style {
			tb := env.TitleBar()
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexShrink: 0,
				Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
				Height: core.Pt(titleRowHeight(env)), Padding: pad(tb),
				// The row owns the pointer: the split's divider below it
				// keeps its resize cursor out of the title bar.
				Cursor: core.CursorDefault}
		}}
}

// undraggable keeps a control in a title row pressable.
func undraggable(n core.Node) core.Node {
	return core.View{WindowDrag: core.WindowDragOff, Children: []core.Node{n}}
}

// searchField is the sidebar's search trigger: it looks like a field and
// opens the command palette.
func (g *gallery) searchField(cx *core.Scope) core.Node {
	theme := ui.UseTheme(cx)
	return core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 10, Left: 12}}, Children: []core.Node{
		base.Button{OnPress: func(core.PressEvent) { g.openPalette() }, Label: "Search", TestID: "search",
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
				core.Text{Content: cx.KeyLabel("mod+k"), Style: core.Style{FontSize: 12}},
			}},
	}}
}

// navigation is the sidebar's list: every group of sections, scrolling
// together.
func (g *gallery) navigation() core.Node {
	item := func(s section) core.Node {
		return ui.SidebarItem{Label: s.name, Icon: s.icon(), TestID: "nav-" + s.name,
			Selected: func() bool { return g.section.Get() == s.name }, OnPress: func(core.PressEvent) { g.open(s.name) }}
	}
	var list []core.Node
	for _, group := range groups() {
		var items []core.Node
		for _, s := range group {
			items = append(items, item(s))
		}
		list = append(list, ui.SidebarSection{Title: group[0].group, TestID: "nav-group-" + group[0].group, Children: items})
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.Scroll{TestID: "nav-sections", Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			ui.Sidebar{Label: "Sections", Children: list},
		}},
	}}
}

// toolbar is the sidebar's bottom row: settings, then the appearance.
func (g *gallery) toolbar() core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.All(8), FlexShrink: 0},
		Children: []core.Node{
			ui.IconButton{Icon: lucide.Settings(), Label: "Settings", OnPress: func(core.PressEvent) { g.openSettings() }, TestID: "open-settings"},
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

// header is the floating title row: the sidebar's toggle, then the
// section's group and name, and on Linux the window's buttons. It drags
// the window, clear of the traffic lights and the caption buttons, and
// leaves the controls in it pressable.
func (g *gallery) header(cx *core.Scope) core.Node {
	theme, env := ui.UseTheme(cx), cx.Env()
	// The toggle clears the window controls, and macOS's traffic lights
	// keep 16 points after them; the platform's reported edge already
	// leaves 8. A drawn control keeps a gap before its buttons, as Onevra
	// leaves both.
	pad := func(tb core.Edges) core.Edges {
		left, right := max(tb.Left, 8), max(tb.Right, 8)
		if env.OS() == core.OSMacOS && tb.Left > 0 {
			left += 8
		}
		if tb.Right > 0 {
			right += 8
		}
		return core.Edges{Left: left, Right: right}
	}
	// The title sits 16 points into the page, as Onevra places it, so it
	// follows the sidebar as it animates; without the sidebar it follows
	// the toggle.
	spacer := core.View{StyleFunc: func() core.Style {
		left := pad(env.TitleBar()).Left
		return core.Style{Width: core.Pt(max(0, g.sidebarPixels.Get()-left-32)), FlexShrink: 0}
	}}
	windowButtons := core.Show{When: func() bool { return env.OS() == core.OSLinux },
		Then: func(*core.Scope) core.Node { return ui.WindowControls{TestID: "window-controls"} }}
	return titleRow(cx, "header", pad,
		core.Show{When: func() bool { return g.sidebar.Get() }, Then: func(*core.Scope) core.Node {
			return undraggable(ui.IconButton{Icon: lucide.PanelLeft(), Label: "Hide sidebar", OnPress: func(core.PressEvent) { g.toggleSidebar() }, TestID: "hide-sidebar"})
		}},
		core.Show{When: func() bool { return !g.sidebar.Get() }, Then: func(*core.Scope) core.Node {
			return undraggable(ui.IconButton{Icon: lucide.PanelLeft(), Label: "Show sidebar", OnPress: func(core.PressEvent) { g.toggleSidebar() }, TestID: "show-sidebar"})
		}},
		spacer,
		core.View{TestID: "header-title", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
			FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Overflow: core.Hidden}, Children: []core.Node{
			core.Text{MaxLines: 1,
				ContentFunc: func() string {
					s, _ := find(g.section.Get())
					if s.name == "Welcome" {
						return ""
					}
					return s.group + " /"
				},
				StyleFunc: func() core.Style {
					return core.Style{Color: theme().MutedForeground, FontWeight: core.FontWeightMedium}
				}},
			core.Text{ContentFunc: g.section.Get, Role: core.RoleHeader, TestID: "title", MaxLines: 1,
				Style: core.Style{FontWeight: core.FontWeightSemibold, FlexShrink: 1}},
		}},
		windowButtons,
	)
}

// page shows the selected section in the gallery's theme for the
// appearance, again when either changes. viewport is the content's height,
// which Welcome fills.
func (g *gallery) page(viewport func() core.Points) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		s, _ := find(g.section.Get())
		t := newTheme(g.dark())
		return core.Func(func(cx *core.Scope) core.Node {
			if s.name == "Welcome" {
				return core.View{StyleFunc: func() core.Style { return core.Style{MinHeight: core.Pt(viewport())} },
					Children: []core.Node{s.page(cx, t)}}
			}
			return core.View{Style: core.Style{Padding: core.All(24)},
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
	// iOS and Android draw the system header in the theme's colors;
	// elsewhere the screens draw their own.
	headerStyle := func() core.Style {
		t := theme()
		return core.Style{Background: sidebarColor(t), Color: t.Foreground, TintColor: t.Primary}
	}
	return g.nav.Stack(core.Style{FlexGrow: 1},
		func(cx *core.Scope, name string, _ func() int) core.Screen {
			if name == "" {
				actions := func() []core.Node {
					return []core.Node{
						ui.IconButton{Icon: lucide.Search(), Label: "Search", OnPress: func(core.PressEvent) { g.openPalette() }, NoTooltip: true},
						ui.IconButton{Icon: lucide.Settings(), Label: "Settings", OnPress: func(core.PressEvent) { g.openSettings() }, NoTooltip: true},
					}
				}
				return core.Screen{Title: "Gallery", TestID: "index", HeaderStyleFunc: headerStyle,
					Header:      g.phoneHeader(cx, "Gallery", nil, actions()...),
					HeaderRight: core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: actions()},
					Children:    []core.Node{g.index(cx)}}
			}
			s, _ := find(name)
			body := core.Dynamic(func(cx *core.Scope) core.Node {
				t := newTheme(g.dark())
				return core.Func(func(cx *core.Scope) core.Node {
					if s.name == "Welcome" {
						return s.page(cx, t)
					}
					return base.SafeAreaView{Edges: base.Bottom, Style: core.Style{Padding: core.All(16)},
						Children: []core.Node{g.sectionBody(cx, t, s)}}
				})
			})
			return core.Screen{Title: s.name, Header: g.phoneHeader(cx, s.name, func() { g.nav.Pop(1) }), TestID: "screen-" + s.name,
				HeaderStyleFunc: headerStyle,
				Children: []core.Node{core.Scroll{Style: core.Style{FlexGrow: 1}, StyleFunc: func() core.Style {
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
		children = append(children, ui.IconButton{Icon: lucide.ChevronLeft(), Label: "Back", OnPress: func(core.PressEvent) { back() }, NoTooltip: true, Size: 20})
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
	for _, group := range groups() {
		items = append(items, core.Text{Content: group[0].group, Role: core.RoleHeader, StyleFunc: func() core.Style {
			t := theme()
			return core.Style{Padding: core.Edges{Top: 18, Right: 16, Bottom: 6, Left: 16}, FontSize: 13,
				FontWeight: core.FontWeightSemibold, Color: t.MutedForeground}
		}})
		for _, s := range group {
			items = append(items, core.View{OnPress: func(core.PressEvent) { g.open(s.name) }, Role: core.RoleButton, Label: s.name, TestID: "nav-" + s.name,
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
