// Command gallery shows NSGo's elements, base components and services, one
// section each: a sidebar beside the selected section on wide windows,
// a navigation stack of them on phones.
package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

//go:embed logo.png
var logo []byte

// section is one page of the gallery.
type section struct {
	name  string
	about string
	page  func(cx *core.Scope, t *theme) core.Node
}

// sections are set in init because the App Menu section lists them.
var sections []section

func init() {
	sections = []section{
		{"Text", "Fonts, weights, decoration, alignment, line limits, links and selection.", textPage},
		{"Text View", "Markdown and HTML with code highlighting, tables, alerts, extensions and streaming.", textViewPage},
		{"Layout", "Flexbox, wrapping, absolute positioning, grid and block layout.", layoutPage},
		{"Styles", "Backgrounds, borders, corners, shadows, transforms and state styles.", stylesPage},
		{"Controls", "Buttons, checkboxes, radios, switches, tabs and text fields.", controlsPage},
		{"Popups", "Tooltips, popovers, menus, dialogs, sheets and alerts.", popupsPage},
		{"Styled", "The ui package: base components with the default theme.", styledPage},
		{"Lists", "A virtualized list of many rows and a keyed list you edit.", listsPage},
		{"Images", "Embedded and remote images, placeholders and resize modes.", imagesPage},
		{"Canvas", "2D drawing recorded on the engine and played by the platform.", canvasPage},
		{"Web View", "The platform's web view: inline HTML, a site, history and messages both ways.", webPage},
		{"Animation", "Values stepped with Animate and transitions run by the platform.", animationPage},
		{"Gestures", "Pans, pinches and rotations, sharing touches with scrolling.", gesturesPage},
		{"Window", "A custom title bar and the desktop window's commands.", windowPage},
		{"App Menu", "The application menu: roles, shortcuts and checkable items.", menuPage},
		{"Environment", "What the platform reports, and the system services.", environmentPage},
		{"Storage", "Key-value storage kept on the device, and the app's directories.", storagePage},
		{"Navigation", "A navigation stack: push and pop screens.", navigationPage},
	}
}

func find(name string) (section, int) {
	for i, s := range sections {
		if s.name == name {
			return s, i
		}
	}
	return sections[0], 0
}

// wideWidth is the narrowest window that shows the sidebar.
const wideWidth core.Points = 720

// gallery is the state the shell's layouts share, so switching between
// them (a window resized, the appearance changed) keeps what is open.
type gallery struct {
	// section is the selected section; on phones, the one last opened.
	section *core.Signal[string]
	// nav is the phone layout's stack: "" is the index, a name a section.
	nav *base.Nav[string]
	// descriptions shows each section's description line; the View menu
	// toggles it.
	descriptions *core.Signal[bool]
	// chosen is the label of the last application menu item chosen.
	chosen *core.Signal[string]
	// wide is whether the layout built last is the sidebar's.
	wide bool
}

// open selects a section, pushing it over the index on phones.
func (g *gallery) open(name string) {
	g.section.Set(name)
	if !g.wide {
		g.nav.SetRoutes("", name)
	}
}

// step selects the section by steps after the selected one.
func (g *gallery) step(by int) {
	_, i := find(g.section.GetUntracked())
	g.open(sections[(i+by+len(sections))%len(sections)].name)
}

// Root builds the gallery again when the appearance or the layout the
// window's width calls for changes, keeping its state in a gallery.
// NSGO_GALLERY_OPEN names a section to open at start, for screenshots.
func Root(cx *core.Scope) core.Node {
	g := &gallery{
		section:      core.NewSignal(cx, sections[0].name),
		nav:          base.NewNav(cx, ""),
		descriptions: core.NewSignal(cx, true),
		chosen:       core.NewSignal(cx, "nothing yet"),
	}
	galleryKey.Provide(cx, g)
	if open := os.Getenv("NSGO_GALLERY_OPEN"); open != "" {
		g.section.Set(open)
		g.nav.SetRoutes("", open)
	}
	env := cx.Env()
	wide := core.NewMemo(cx, func() bool { return env.Window().Width >= wideWidth })
	dark := core.NewMemo(cx, env.Dark)

	cx.Effect(func() { cx.SetApplicationMenu(g.menu(cx)) })
	cx.Effect(func() { cx.SetTitle("NSGo Gallery — " + g.section.Get()) })
	cx.OnKey("ctrl+tab", func() { g.step(1) })
	cx.OnKey("ctrl+shift+tab", func() { g.step(-1) })

	return core.Dynamic(func(cx *core.Scope) core.Node {
		t, isWide := newTheme(dark.Get()), wide.Get()
		// What the layout builds mounts untracked, outside this function.
		return core.Func(func(cx *core.Scope) core.Node {
			if isWide {
				return g.desktop(cx, t)
			}
			return g.phone(cx, t)
		})
	})
}

// menu is the application menu: the app menu, Edit, View (a section
// each, with shortcuts) and Window.
func (g *gallery) menu(cx *core.Scope) []core.MenuItem {
	selected := g.section.Get()
	choose := func(label string, fn func()) func() {
		return func() { g.chosen.Set(label); fn() }
	}
	view := make([]core.MenuItem, 0, len(sections)+5)
	for i, s := range sections {
		item := core.MenuItem{Label: s.name, Checked: s.name == selected, OnSelect: choose(s.name, func() { g.open(s.name) })}
		if i < 9 {
			item.Shortcut = fmt.Sprintf("mod+%d", i+1)
		}
		view = append(view, item)
	}
	view = append(view,
		core.MenuItem{Separator: true},
		core.MenuItem{Label: "Previous Section", Shortcut: "mod+[", OnSelect: choose("Previous Section", func() { g.step(-1) })},
		core.MenuItem{Label: "Next Section", Shortcut: "mod+]", OnSelect: choose("Next Section", func() { g.step(1) })},
		core.MenuItem{Separator: true},
		core.MenuItem{Label: "Show Descriptions", Checkable: true, Checked: g.descriptions.Get(),
			OnSelect: choose("Show Descriptions", func() { g.descriptions.Update(func(on bool) bool { return !on }) })},
	)
	return []core.MenuItem{
		{Label: "Gallery", Items: []core.MenuItem{
			{Role: core.MenuAbout}, {Separator: true},
			{Role: core.MenuHide}, {Role: core.MenuHideOthers}, {Role: core.MenuShowAll}, {Separator: true},
			{Role: core.MenuQuit},
		}},
		{Label: "Edit", Items: []core.MenuItem{
			{Role: core.MenuUndo}, {Role: core.MenuRedo}, {Separator: true},
			{Role: core.MenuCut}, {Role: core.MenuCopy}, {Role: core.MenuPaste}, {Role: core.MenuSelectAll},
		}},
		{Label: "View", Items: view},
		{Label: "Window", Items: []core.MenuItem{
			{Role: core.MenuMinimize}, {Role: core.MenuZoom}, {Role: core.MenuToggleFullScreen}, {Separator: true},
			{Label: "Hide Title Bar", OnSelect: choose("Hide Title Bar", func() { cx.SetTitleBar(core.TitleBarHidden, titleBarHeight) })},
			{Label: "Show Title Bar", OnSelect: choose("Show Title Bar", func() { cx.SetTitleBar(core.TitleBarDefault, 0) })},
			{Separator: true}, {Role: core.MenuClose},
		}},
	}
}

// titleBarHeight is the height of the gallery's own title bar.
const titleBarHeight core.Points = 44

// desktop is the wide layout: the gallery's title bar while the system's
// is hidden, then the sidebar beside the selected section.
func (g *gallery) desktop(cx *core.Scope, t *theme) core.Node {
	g.wide = true
	// Coming from the phone layout, show the section it had open.
	if routes := core.Untracked(cx, g.nav.Routes); len(routes) > 1 {
		g.section.Set(routes[len(routes)-1])
	}
	content := &core.Ref{}
	cx.Effect(func() {
		g.section.Get()
		content.ScrollTo(core.Point{}, false)
	})
	return base.SafeAreaView{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Background: t.background, Color: t.ink}, Children: []core.Node{
		g.titleBar(cx, t),
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Direction: core.Row}, Children: []core.Node{
			g.sidebar(t),
			core.Scroll{Ref: content, TestID: "content", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.View{Style: core.Style{Padding: core.Edges{Top: 28, Right: 32, Bottom: 40, Left: 32}, Gap: 16,
					MaxWidth: core.Pt(880)}, Children: []core.Node{g.page(t)}},
			}},
		}},
	}}
}

// titleBar stands in for the system's title bar while it is hidden:
// a window drag handle as tall as Env.TitleBar's top, clear of the window
// controls on its left or right.
func (g *gallery) titleBar(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	return core.Show{When: func() bool { return env.TitleBar() != (core.Edges{}) }, Then: func(cx *core.Scope) core.Node {
		return core.View{WindowDrag: core.WindowDragOn, TestID: "titlebar",
			StyleFunc: func() core.Style {
				tb := env.TitleBar()
				return core.Style{Height: core.Pt(tb.Top), Padding: core.Edges{Left: tb.Left + 12, Right: tb.Right + 12}}
			},
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Background: t.sidebar, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}},
			Children: []core.Node{core.Text{ContentFunc: func() string { return "NSGo Gallery — " + g.section.Get() },
				Style: core.Style{FontSize: 13, FontWeight: core.FontWeightSemibold, Color: t.muted}}}}
	}}
}

// sidebar lists the sections, the selected one highlighted.
func (g *gallery) sidebar(t *theme) core.Node {
	items := []core.Node{core.View{Style: core.Style{Padding: core.Edges{Top: 8, Right: 10, Bottom: 12, Left: 10}, Direction: core.Row,
		AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
		core.Image{Source: core.ImageSource{Data: logo, Size: core.Size{Width: 28, Height: 28}}, Label: "NSGo"},
		core.Text{Content: "Gallery", Style: core.Style{FontSize: 17, FontWeight: core.FontWeightSemibold}},
	}}}
	for _, s := range sections {
		items = append(items, core.View{
			OnPress: func() { g.open(s.name) }, Selected: func() bool { return g.section.Get() == s.name },
			Role: core.RoleButton, Label: s.name, TestID: "nav-" + s.name,
			Style:    t.navItem(),
			Children: []core.Node{core.Text{Content: s.name}},
		})
	}
	return core.View{Style: core.Style{Width: core.Pt(220), Background: t.sidebar,
		Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: t.line}}, Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(12), Gap: 2}, Children: items},
		}},
	}}
}

// page shows the selected section under its title, again when the
// selection changes.
func (g *gallery) page(t *theme) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		s, _ := find(g.section.Get())
		return core.Func(func(cx *core.Scope) core.Node { return g.sectionBody(cx, t, s, true) })
	})
}

// sectionBody is a section's demos, under its title on wide windows.
func (g *gallery) sectionBody(cx *core.Scope, t *theme, s section, title bool) core.Node {
	var head []core.Node
	if title {
		head = append(head, core.Text{Content: s.name, Role: core.RoleHeader, TestID: "title",
			Style: core.Style{FontSize: 26, FontWeight: core.FontWeightSemibold}})
	}
	head = append(head, core.Show{When: g.descriptions.Get, Then: func(*core.Scope) core.Node {
		return core.Text{Content: s.about, TestID: "about", Style: core.Style{Color: t.muted, FontSize: 15}}
	}})
	return core.View{Style: core.Style{Gap: 16}, TestID: "section-" + s.name,
		Children: append(head, s.page(cx, t))}
}

// phone is the narrow layout: a stack of the index and the section opened
// over it.
func (g *gallery) phone(cx *core.Scope, t *theme) core.Node {
	// Coming from the wide layout, keep its section open.
	if g.wide {
		g.nav.SetRoutes("", g.section.GetUntracked())
	}
	g.wide = false
	return g.nav.Stack(core.Style{FlexGrow: 1, Background: t.background, Color: t.ink},
		func(cx *core.Scope, name string, _ func() int) core.Screen {
			if name == "" {
				return core.Screen{Title: "Gallery", Header: t.header("Gallery", nil), TestID: "index",
					Children: []core.Node{g.index(t)}}
			}
			s, _ := find(name)
			return core.Screen{Title: s.name, Header: t.header(s.name, func() { g.nav.Pop(1) }), TestID: "screen-" + s.name,
				Children: []core.Node{core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
					base.SafeAreaView{Edges: base.Bottom, Style: core.Style{Padding: core.All(16)},
						Children: []core.Node{g.sectionBody(cx, t, s, false)}},
				}}}}
		})
}

// header is a phone screen's bar: a back button and the title.
func (t *theme) header(title string, back func()) core.Node {
	children := []core.Node{}
	if back != nil {
		children = append(children, base.Button{OnPress: back, Label: "Back", Style: core.Style{Padding: core.Symmetric(4, 8),
			Active: &core.Style{Opacity: core.Alpha(0.6)}},
			Children: []core.Node{core.Text{Content: "‹ Back", Style: core.Style{Color: t.accent, FontSize: 15, FontWeight: core.FontWeightMedium}}}})
	}
	children = append(children, core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FontSize: 18, FontWeight: core.FontWeightSemibold}})
	return base.SafeAreaView{Edges: base.Top | base.Left | base.Right, Style: core.Style{Background: t.surface,
		Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
			Padding: core.Symmetric(12, 16)}, Children: children}}}
}

// index is the phone layout's list of sections.
func (g *gallery) index(t *theme) core.Node {
	items := []core.Node{}
	for _, s := range sections {
		items = append(items, core.View{OnPress: func() { g.open(s.name) }, Role: core.RoleButton, Label: s.name, TestID: "nav-" + s.name,
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 16),
				Background: t.surface, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line},
				Active: &core.Style{Background: t.hover}},
			Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
					core.Text{Content: s.name, Style: core.Style{FontSize: 15, FontWeight: core.FontWeightMedium}},
					core.Text{Content: s.about, MaxLines: 1, Style: t.caption()},
				}},
				core.Text{Content: "›", Style: core.Style{FontSize: 18, Color: t.muted}},
			}})
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		base.SafeAreaView{Edges: base.Bottom, Children: items},
	}}
}

func main() { nsgo.Run(Root) }
