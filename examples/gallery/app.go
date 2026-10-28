// Command gallery shows what NSGo apps are made of: core's elements and
// services and base's unstyled components, a demo each, in NSH's gallery's
// design. A wide window shows a sidebar of the demos beside the selected
// one; a narrow one is a phone's index with demos pushed over it. Its
// look and components are its own (look.go, kit.go).
//
// It translates NSH's examples/gallery demo by demo, and departs from it
// where NSGo differs: icons are Lucide's (core.Icon) rather than glyphs
// drawn in canvases; NSH's Tailwind and standard-module demos have no NSGo counterpart, while
// Layout, Shader and Accessibility show what NSGo has beyond NSH's
// gallery.
package main

import (
	_ "embed"
	"os"
	"slices"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/desktop"
)

//go:embed logo.png
var logoPNG []byte

// demo is an entry of the index.
type demo struct {
	name   string
	detail string
	icon   iconName
	screen func(cx *core.Scope) core.Node
}

// group is demos by what they show.
type group struct {
	title string
	demos []demo
}

// groups are set in init because demos list them.
var groups []group

func init() {
	groups = []group{
		{"Apps", []demo{
			{"Stocks", "A live watchlist and a stock: candles, depth, the tape, a tick every 50 ms", iconStocks, stocksDemo},
			{"AI Chat", "Streaming replies with reasoning, tools and attachments", iconChat, chatDemo},
			{"Photos", "A long recycled list: images, pull to refresh, paging", iconPhotos, photosDemo},
		}},
		{"Layout and lists", []demo{
			{"Lists", "Columns, sticky sections, a chat, horizontal lists", iconLists, listsDemo},
			{"Scrolling", "Paging, snapping, zoom, scroll events, prepending, keyboard taps", iconScroll, scrollingDemo},
			{"Collapsing header", "A header that shrinks to a bar as the page scrolls, headings sticking below it", iconScroll, collapsingDemo},
			{"Layout", "Flexbox, wrapping, positioning, grid and block layout, direction", iconLayout, layoutDemo},
			{"Navigation", "Header options, modals, the back button, safe areas", iconNav, navigationDemo},
			{"Form", "Inputs, keyboard types, the keyboard over the content", iconForm, formDemo},
			{"Resizable", "Panels resized by dragging the lines between them, within their ranges", iconSidebar, resizableDemo},
			{"Dock", "A workspace of tabs dragged between groups and docks, split, closed, zoomed and resized", iconDock, dockDemo},
		}},
		{"Touch and motion", []demo{
			{"Gestures", "Pinch, rotate, press states, hit slop, pointer events, dropped files", iconGestures, gesturesDemo},
			{"Physics", "Drags and springs on the UI thread", iconPhysics, physicsDemo},
			{"Animations", "Easing, sequence / parallel / stagger / loop, springs, interpolation", iconAnimation, animationsDemo},
		}},
		{"Drawing and type", []demo{
			{"Canvas", "2D drawing: paths, gradients, text, images, native animation", iconCanvas, canvasDemo},
			{"Charts", "A line and area plot with hover, grid lines and motion, sparklines in a list", iconStocks, chartsDemo},
			{"Shader", "Fragment shaders in Go's syntax, uniforms the platform animates", iconShader, shaderDemo},
			{"Styles", "Corners, borders, shadows, clipping, filters, 3D transforms", iconStyles, stylesDemo},
			{"Text", "Fonts, spans, inline views, decorations, truncation, languages", iconText, textDemo},
			{"Text View", "Markdown and HTML documents, extensions, streaming replies", iconText, textViewDemo},
			{"Editor", "Code with highlighting, several carets, folding, find and replace, diagnostics, completion and definitions", iconText, editorDemo},
			{"DataTable", "A million stocks of sixty columns, their quotes ticking every 33 ms, fixed columns, sorting, selection, resized and moved columns", iconLists, dataTableDemo},
			{"Diff", "Two texts side by side or unified, changes navigated, reverted and kept, a patch", iconText, diffDemo},
			{"Images", "Resize modes, tint, GIF, placeholders, blur, fade", iconImage, imagesDemo},
		}},
		{"Platform", []demo{
			{"Components", "Press feedback, a sheet, layout animation, input accessory, dialog options", iconComponents, componentsDemo},
			{"Overlays", "Overlay, popovers, tooltips, sheets, toasts", iconOverlays, overlaysDemo},
			{"Menus", "A menu bar, submenus, checkable items, context menus on rows", iconMenu, menusDemo},
			{"Controls", "Switches in settings, a filter with select all, a plan picker, tabs, collapsible sections, progress bars, a table", iconControls, controlsDemo},
			{"Web and video", "Web views, video players and the app's own native views", iconMedia, mediaDemo},
			{"System", "Status bar, alerts, the window, environment, timers, the clipboard, fetch", iconSystem, systemDemo},
			{"Accessibility", "Labels, values, actions, groups, live regions and focus", iconAccess, accessibilityDemo},
			{"Storage", "A launch count, a note saved as you type, the stored keys", iconStorage, storageDemo},
		}},
	}
}

// allDemos is every demo, in the index's order.
func allDemos() []demo {
	var out []demo
	for _, g := range groups {
		out = append(out, g.demos...)
	}
	return out
}

// findDemo returns the demo named name.
func findDemo(name string) (demo, bool) {
	for _, d := range allDemos() {
		if d.name == name {
			return d, true
		}
	}
	return demo{}, false
}

// groupOf is the title of the group holding the demo named name.
func groupOf(name string) string {
	for _, g := range groups {
		for _, d := range g.demos {
			if d.name == name {
				return g.title
			}
		}
	}
	return "Gallery"
}

// route is a screen of the navigation stack: the index (no demo, no
// page), a demo, or a page a demo pushes.
type route struct {
	demo string
	page func(cx *core.Scope) base.NavPage
}

// gallery is the state the layouts share, so switching between them (a
// window resized, the appearance changed) keeps what is open.
type gallery struct {
	nav *base.Nav[route]
	// selected is the demo the desktop layout shows.
	selected *core.Signal[string]
	// sidebarOpen is whether the desktop layout shows its sidebar.
	sidebarOpen *core.Signal[bool]
	// chosen is the label of the last application menu item chosen.
	chosen *core.Signal[string]
	// wasDesktop is whether the desktop layout has been shown, for the
	// switch back.
	wasDesktop bool
	desktop    func() bool
	env        *core.Env
	toasts     *toaster
}

// shell is the running gallery's state, which demos reach for the layout
// they are in and to push pages. One process runs one gallery.
var shell *gallery

// paneWidth is the width of the desktop layout's content pane, right of
// the sidebar.
func (g *gallery) paneWidth() core.Points {
	w := g.env.Window().Width
	if g.sidebarOpen.Get() {
		w -= sidebarWidth
	}
	return w
}

// push shows a page over the demo.
func (g *gallery) push(page func(cx *core.Scope) base.NavPage) { g.nav.Push(route{page: page}) }

// selectDemo shows a demo in the desktop layout, closing what it had
// pushed.
func (g *gallery) selectDemo(name string) {
	if g.nav.Depth() > 0 {
		g.nav.PopToRoot()
	}
	g.selected.Set(name)
}

// open shows a demo in the layout shown: selected beside the sidebar, or
// pushed over the index.
func (g *gallery) open(name string) {
	if g.desktop() {
		g.selectDemo(name)
		return
	}
	g.nav.SetRoutes(route{}, route{demo: name})
}

// lastDemo is the storage key of the demo left open, shown again on the
// next launch.
const lastDemo = "lastDemo"

// Root builds the gallery again when the window's width calls for the
// other layout, keeping its state in shell; a change of theme or
// appearance restyles it in place.
// NSGO_GALLERY_OPEN names a demo to open at start, for screenshots.
func Root(cx *core.Scope) core.Node {
	cx.SetTheme(galleryTheme())
	env := cx.Env()
	first := groups[0].demos[0].name
	g := &gallery{
		nav:         base.NewNav(cx, route{}),
		selected:    core.NewSignal(cx, first),
		sidebarOpen: core.NewSignal(cx, true),
		chosen:      core.NewSignal(cx, "nothing yet"),
		env:         env,
		toasts:      newToaster(cx),
	}
	desktopLayout := core.NewMemo(cx, func() bool {
		limit := tabletWidth
		if env.Desktop() {
			limit = desktopWidth
		}
		return env.Window().Width >= limit
	})
	g.desktop = desktopLayout.Get
	shell = g

	hasWindow := desktop.IsAvailable(cx)
	if hasWindow {
		desktop.SetTitle(cx, "Gallery")
		desktop.SetTitleBar(cx, desktop.TitleBarHidden, 40)
	}
	rememberDemo(cx, g)
	cx.Effect(func() { desktop.SetApplicationMenu(cx, g.menu()) })

	return core.Dynamic(func(cx *core.Scope) core.Node {
		wide := desktopLayout.Get()
		// What the layout builds mounts untracked, outside this function.
		return core.Func(func(cx *core.Scope) core.Node {
			colors := colorsOf(cx)
			body := g.phone(cx)
			if wide {
				body = g.desktopLayout(cx)
			}
			var children []core.Node
			if hasWindow {
				children = append(children, g.titleBar(cx, wide))
			}
			children = append(children, body, g.toasts.view(wide))
			return core.View{StyleFunc: func() core.Style {
				return fillPage.Merge(core.Style{Background: chromeFill(cx), Color: colors.text()})
			}, Children: children}
		})
	})
}

// rememberDemo reopens the demo left open last time (selected on a
// desktop, pushed on a phone), then keeps the key current: the demo shown,
// or none when a phone is back on the index. A name no demo has any more
// is ignored.
func rememberDemo(cx *core.Scope, g *gallery) {
	store := cx.Storage()
	name, _ := store.GetItem(lastDemo)
	if open := os.Getenv("NSGO_GALLERY_OPEN"); open != "" {
		name = open
	}
	if d, ok := findDemo(name); ok {
		g.selected.Set(d.name)
		if !g.desktop() {
			g.nav.SetRoutes(route{}, route{demo: d.name})
		}
	}
	cx.Effect(func() {
		open := g.selected.Get()
		if !g.desktop() {
			open = ""
			for _, r := range g.nav.Routes() {
				if r.demo != "" {
					open = r.demo
				}
			}
		}
		core.Untracked(cx, func() bool {
			if open != "" {
				store.SetItem(lastDemo, open)
			} else {
				store.RemoveItem(lastDemo)
			}
			return true
		})
	})
}

// titleBar is window chrome the app draws; the platform keeps its own
// window controls. The sidebar's toggle follows the logo in the desktop
// layout.
func (g *gallery) titleBar(cx *core.Scope, wide bool) core.Node {
	colors := colorsOf(cx)
	children := []core.Node{logo(cx, 20)}
	if wide {
		children = append(children, g.sidebarToggle(cx))
	}
	children = append(children,
		core.Text{Content: "Gallery", MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, FontWeight: semibold}, StyleFunc: colored(colors.text)},
		text("NSGo", 11, regular, colors.muted),
	)
	return core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn}, TestID: "title-bar",
		StyleFunc: func() core.Style {
			tb := desktop.TitleBar(cx)
			return core.Style{Height: core.Pt(max(tb.Top, 40)), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
				Padding: core.Edges{Left: tb.Left + 12, Right: tb.Right + 12}, Background: chromeFill(cx),
				Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.line()}}
		},
		Children: children}
}

// chromeFill is the background of the window's chrome, the title bar
// and the sidebar: none while a window material shows through them.
func chromeFill(cx *core.Scope) core.Color {
	if desktop.ShowsMaterial(cx) {
		return 0
	}
	return colorsOf(cx).inset()
}

// toggleSidebar hides or shows the desktop layout's sidebar.
func (g *gallery) toggleSidebar() { g.sidebarOpen.Update(func(open bool) bool { return !open }) }

// sidebarToggle is the title bar's button that hides and shows the
// sidebar, where desktop apps put it: after the logo.
func (g *gallery) sidebarToggle(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	return core.View{OnPress: func(core.PressEvent) { g.toggleSidebar() }, Role: core.RoleButton, Label: "Toggle sidebar",
		Selected: g.sidebarOpen.Get, Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		Style: core.Style{Width: core.Pt(28), Height: core.Pt(28), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			CornerRadius: radiusControl, Cursor: core.CursorPointer},
		StyleFunc: func() core.Style {
			return core.Style{Hover: &core.Style{Background: colors.hover()}, Active: &core.Style{Background: colors.pressed()}}
		},
		Children: []core.Node{icon(iconSidebar, 18, colors.muted)}}
}

// phone is the narrow layout: the index, demos pushed over it.
func (g *gallery) phone(cx *core.Scope) core.Node {
	// Back from the desktop layout: the demo it showed stays open.
	if g.wasDesktop {
		g.nav.SetRoutes(route{}, route{demo: g.selected.GetUntracked()})
	}
	g.wasDesktop = false
	return g.nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		switch {
		case r.page != nil:
			return r.page(cx)
		case r.demo != "":
			d, _ := findDemo(r.demo)
			return base.NavPage{Title: d.name, TestID: "screen-" + d.name, HeaderStyleFunc: headerStyle(cx),
				Header:    pageHeader(cx, pageHeaderProps{title: d.name}),
				StyleFunc: pageStyle(cx),
				Children: []core.Node{core.View{Style: fillPage, StyleFunc: pageStyle(cx),
					Children: []core.Node{core.Func(func(cx *core.Scope) core.Node { return d.screen(cx) })}}}}
		}
		return base.NavPage{Title: "Gallery", TestID: "index", HeaderStyleFunc: headerStyle(cx), HeaderRight: barThemeToggle(cx),
			Header:   pageHeader(cx, pageHeaderProps{title: "Gallery", root: true, right: themeToggle(cx)}),
			Children: []core.Node{g.home(cx)}}
	})
}

// home is the phone layout's index of the demos by group.
func (g *gallery) home(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	var nodes []core.Node
	for i, grp := range groups {
		var rows []core.Node
		for j, d := range grp.demos {
			rows = append(rows, demoRow(cx, d, j == 0, func() { g.nav.Push(route{demo: d.name}) }))
		}
		nodes = append(nodes, appear(i+1, false, core.View{TestID: "group-" + grp.title, Style: core.Style{Gap: 8}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Padding: core.Symmetric(0, 4)}, Children: []core.Node{
				core.Text{Content: grp.title, Role: core.RoleHeader, Style: core.Style{FlexGrow: 1, FontSize: 13, FontWeight: semibold,
					LetterSpacing: 0.4, TextTransform: core.TextTransformUppercase}, StyleFunc: colored(colors.muted)},
				core.Text{Content: itoa(len(grp.demos)), Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(colors.faint)},
			}},
			core.View{Style: core.Style{CornerRadius: radiusCard, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1, Overflow: core.Hidden},
				StyleFunc: func() core.Style { return core.Style{Background: colors.surface(), BorderColor: colors.border()} }, Children: rows},
		}}))
	}
	return core.Scroll{TestID: "home", Style: core.Style{FlexGrow: 1}, StyleFunc: pageStyle(cx), Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 40, Left: 16}, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}, Gap: 20}, Children: nodes},
	}}
}

// demoRow is a demo in the phone layout's index.
func demoRow(cx *core.Scope, d demo, first bool, open func()) core.Node {
	colors := colorsOf(cx)
	s := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.All(12)}
	if !first {
		s.Borders = core.Edges{Top: 1}
	}
	return press(pressProps{onPress: open, scale: 0.98, label: d.name, testID: "nav-" + d.name},
		core.View{Style: s, StyleFunc: func() core.Style {
			if first {
				return core.Style{}
			}
			return core.Style{BorderColors: core.EdgeColors{Top: colors.line()}}
		}, Children: []core.Node{
			iconTile(cx, d.icon, 38),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				text(d.name, 16, medium, colors.text),
				core.Text{Content: d.detail, MaxLines: 2, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: colored(colors.muted)},
			}},
			icon(iconChevron, 16, colors.faint),
		}})
}

// desktopLayout is a sidebar of the demos and the selected one beside it,
// no index to go back to. What a demo pushes opens over it in the content
// pane, with a back button.
func (g *gallery) desktopLayout(cx *core.Scope) core.Node {
	// From the phone layout: show the demo it had open.
	routes := core.Untracked(cx, g.nav.Routes)
	for _, r := range routes {
		if r.demo != "" {
			g.selected.Set(r.demo)
		}
	}
	if len(routes) > 1 {
		g.nav.SetRoutes(routes[0])
	}
	g.wasDesktop = true
	// The application menu's View › Show Sidebar takes the key where
	// there is a menu bar.
	if !desktop.IsAvailable(cx) {
		cx.OnKey("mod+b", g.toggleSidebar)
	}
	return core.View{StyleFunc: func() core.Style {
		return core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Direction: core.Row, Background: chromeFill(cx)}
	}, Children: []core.Node{
		g.sidebar(cx),
		g.nav.Stack(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
			if r.page != nil {
				return r.page(cx)
			}
			return base.NavPage{HeaderHidden: true, Children: []core.Node{g.demoPane()}}
		}),
	}}
}

// sidebar is the desktop layout's list of the demos by group. Its place
// in the row changes at once (0 pt collapsed); the panel slides on the
// platform, over the content pane.
func (g *gallery) sidebar(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	start := 0.0
	if !g.sidebarOpen.GetUntracked() {
		start = float64(-sidebarWidth)
	}
	slide := core.NewAnimatedValue(cx, start)
	animated := &core.AnimatedStyle{TranslateX: slide}
	first := true
	cx.Effect(func() {
		open := g.sidebarOpen.Get()
		if first {
			first = false
			return
		}
		to := 0.0
		if !open {
			to = float64(-sidebarWidth)
		}
		timing(cx, slide, to, core.TimingConfig{Duration: 220e6, Easing: easeOutCubic})
	})
	env := cx.Env()
	var list []core.Node
	for _, grp := range groups {
		items := []core.Node{core.Text{Content: grp.title, Role: core.RoleHeader, Style: core.Style{FontSize: 11, FontWeight: semibold,
			LetterSpacing: 0.6, TextTransform: core.TextTransformUppercase, Padding: core.Edges{Top: 16, Right: 10, Bottom: 6, Left: 10}},
			StyleFunc: colored(colors.faint)}}
		for _, d := range grp.demos {
			items = append(items, g.sidebarRow(cx, d))
		}
		list = append(list, core.View{TestID: "nav-group-" + grp.title, Style: core.Style{Gap: 2}, Children: items})
	}
	return core.View{StyleFunc: func() core.Style {
		var w core.Points
		if g.sidebarOpen.Get() {
			w = sidebarWidth
		}
		return core.Style{Width: core.Pt(w), ZIndex: 1}
	}, Children: []core.Node{core.View{TestID: "sidebar", StyleFunc: func() core.Style {
		return core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
			Width: core.Pt(sidebarWidth), Background: chromeFill(cx), Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: colors.line()},
			Animated: animated}
	}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 20, Right: 20, Bottom: 12, Left: 20}}, Children: []core.Node{
			logo(cx, 32),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 1}, Children: []core.Node{
				text("Gallery", 16, semibold, colors.text),
				textf(func() string { return "NSGo · " + osName(env.OS()) }, 12, regular, colors.muted),
			}},
			themeToggle(cx),
		}},
		core.Scroll{TestID: "nav-sections", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.Edges{Right: 12, Bottom: 16, Left: 12}}, Children: list},
		}},
		core.View{Style: core.Style{Padding: core.Symmetric(12, 20), Borders: core.Edges{Top: 1}, Gap: 2},
			StyleFunc: func() core.Style { return core.Style{BorderColors: core.EdgeColors{Top: colors.line()}} }, Children: []core.Node{
				core.Text{ContentFunc: func() string {
					w := env.Window()
					return itoa(int(w.Width+0.5)) + " × " + itoa(int(w.Height+0.5)) + " pt"
				}, Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(colors.muted)},
				text("Narrower than 700 pt: the phone layout", 11, regular, colors.faint),
			}},
	}}}}
}

// sidebarRow is a demo in the sidebar: an accent bar and a raised fill
// when selected, a fill under the pointer otherwise.
func (g *gallery) sidebarRow(cx *core.Scope, d demo) core.Node {
	colors := colorsOf(cx)
	active := func() bool { return g.selected.Get() == d.name }
	return core.View{OnPress: func(core.PressEvent) { g.selectDemo(d.name) }, Role: core.RoleButton, Label: d.name, TestID: "nav-" + d.name,
		Selected: active,
		Style: core.Style{Height: core.Pt(36), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Symmetric(0, 10),
			CornerRadius: radiusControl, Cursor: core.CursorPointer},
		StyleFunc: func() core.Style {
			return core.Style{Hover: &core.Style{Background: colors.hover()}, Active: &core.Style{Background: colors.pressed()}, Selected: &core.Style{Background: colors.pressed()}}
		},
		Children: []core.Node{
			core.View{StyleFunc: func() core.Style {
				o := float32(0)
				if active() {
					o = 1
				}
				return core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(9), Width: core.Pt(3), Height: core.Pt(18), CornerRadius: 1.5,
					Background: colors.accent(), Opacity: core.Alpha(o)}
			}},
			icon(d.icon, 17, func() core.Color {
				if active() {
					return colors.accent()
				}
				return colors.muted()
			}),
			core.Text{Content: d.name, MaxLines: 1, StyleFunc: func() core.Style {
				if active() {
					return core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, FontWeight: semibold, Color: colors.text()}
				}
				return core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, FontWeight: regular, Color: colors.muted()}
			}},
		}}
}

// demoPane is the selected demo under a toolbar with its name and what it
// shows.
func (g *gallery) demoPane() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		d, _ := findDemo(g.selected.Get())
		return core.Func(func(cx *core.Scope) core.Node {
			colors := colorsOf(cx)
			return core.View{TestID: "demo-" + d.name, Style: fillPage, StyleFunc: pageStyle(cx), Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(64), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.Symmetric(0, 24),
					Borders: core.Edges{Bottom: 1}}, StyleFunc: func() core.Style { return core.Style{BorderColors: core.EdgeColors{Bottom: colors.line()}} },
					Children: []core.Node{
						iconTile(cx, d.icon, 36),
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
							core.Text{Content: d.name, MaxLines: 1, Role: core.RoleHeader, TestID: "title", Style: core.Style{FontSize: 18, FontWeight: semibold}, StyleFunc: colored(colors.text)},
							core.Text{Content: d.detail, MaxLines: 1, Style: core.Style{FontSize: 13}, StyleFunc: colored(colors.muted)},
						}},
						badge(cx, groupOf(d.name), toneNeutral),
					}},
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{d.screen(cx)}},
			}}
		})
	})
}

// menu is the application menu: the app menu and Edit of roles, View
// (the sidebar and a demo each, with shortcuts) and Window.
func (g *gallery) menu() []desktop.MenuItem {
	selected := g.selected.Get()
	choose := func(label string, fn func()) func() {
		return func() { g.chosen.Set(label); fn() }
	}
	view := []desktop.MenuItem{
		{Label: "Show Sidebar", Shortcut: "mod+b", Checkable: true, Checked: g.sidebarOpen.Get(), OnSelect: choose("Show Sidebar", g.toggleSidebar)},
		{Separator: true},
	}
	for i, d := range allDemos() {
		item := desktop.MenuItem{Label: d.name, Checkable: true, Checked: d.name == selected, OnSelect: choose(d.name, func() { g.open(d.name) })}
		if i < 9 {
			item.Shortcut = "mod+" + itoa(i+1)
		}
		view = append(view, item)
	}
	return []desktop.MenuItem{
		{Label: "Gallery", Items: []desktop.MenuItem{
			{Role: desktop.MenuAbout}, {Separator: true},
			{Role: desktop.MenuHide}, {Role: desktop.MenuHideOthers}, {Role: desktop.MenuShowAll}, {Separator: true},
			{Role: desktop.MenuQuit},
		}},
		{Label: "Edit", Items: []desktop.MenuItem{
			{Role: desktop.MenuUndo}, {Role: desktop.MenuRedo}, {Separator: true},
			{Role: desktop.MenuCut}, {Role: desktop.MenuCopy}, {Role: desktop.MenuPaste}, {Role: desktop.MenuSelectAll},
		}},
		{Label: "View", Items: view},
		{Label: "Window", Items: []desktop.MenuItem{
			{Role: desktop.MenuMinimize}, {Role: desktop.MenuZoom}, {Role: desktop.MenuToggleFullScreen}, {Separator: true},
			{Role: desktop.MenuClose},
		}},
	}
}

// osName is the platform's name, as the sidebar shows it.
func osName(os core.OS) string {
	names := map[core.OS]string{core.OSMacOS: "macOS", core.OSIOS: "iOS", core.OSAndroid: "Android", core.OSWindows: "Windows", core.OSLinux: "Linux", core.OSWeb: "web"}
	if n, ok := names[os]; ok {
		return n
	}
	return "unknown"
}

func main() {
	// The Go fonts' small capitals and Pacifico, shipped in
	// resources/fonts.
	opts := []nsgo.Option{nsgo.Fonts("fonts/Go-Smallcaps.ttf", "fonts/Go-Smallcaps-Italic.ttf", "fonts/Pacifico-Regular.ttf")}
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		opts = append(opts, nsgo.Component(name, components[name]))
	}
	nsgo.Run(Root, opts...)
}
