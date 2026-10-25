// Command base shows what apps build on beneath the ui package: core's
// elements and services and base's unstyled components, a page each, in a
// standard window. Every look is the app's own, set in theme.go.
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
	group string
	about string
	page  func(cx *core.Scope, t *theme) core.Node
}

// The section list's groups, in its order.
const (
	groupComponents = "Components"
	groupElements   = "Elements"
	groupMedia      = "Media"
	groupSystem     = "System"
)

// sections are set in init because the App Menu section lists them.
var sections []section

func init() {
	sections = []section{
		{"Button", groupComponents, "base.Button is a pressable view with the button role; its look is the app's.", buttonPage},
		{"Popups", groupComponents, "Tooltips, popovers, menus, context menus, dialogs, sheets and the platform's alerts.", popupsPage},
		{"Navigation", groupComponents, "base.Nav's stack of routes, with the system header on iOS and Android.", navigationPage},
		{"Safe Area", groupComponents, "base.SafeAreaView keeps its content clear of the system's UI.", safeAreaPage},
		{"Split", groupComponents, "base.Split resizes two panes with a divider.", splitPage},

		{"Text", groupElements, "Fonts, weights, decoration, alignment, line limits, links and selection.", textPage},
		{"Input", groupElements, "The platform's field: keyboard types, editing props, the caret, pasting and a keyboard accessory.", inputPage},
		{"Layout", groupElements, "Flexbox, wrapping, positioning, grid and block layout, intrinsic sizes, stacking and direction.", layoutPage},
		{"Styles", groupElements, "Backgrounds, corners, borders, shadows, gradients, filters, transforms, keyframes and state styles.", stylesPage},
		{"Images", groupElements, "Embedded, remote, animated and vector images, placeholders and resize modes.", imagesPage},
		{"Canvas", groupElements, "2D drawing recorded on the engine and played by the platform.", canvasPage},
		{"Shader", groupElements, "Fragment shaders in Go's syntax, run on the GPU with uniforms the platform animates.", shaderPage},
		{"Lists", groupElements, "Virtualized lists: sticky sections, columns, a chat, a list in the page, and a keyed list you edit.", listsPage},
		{"Scrolling", groupElements, "Paging, snapping, insets, pull to refresh, the scroll events and zooming.", scrollingPage},

		{"Web View", groupMedia, "The platform's web view: inline HTML, a site, history and messages both ways.", webPage},
		{"Video", groupMedia, "The platform's video player: playback the app drives, resize modes and the platform's controls.", videoPage},
		{"Native View", groupMedia, "The app's own platform views inside the tree, with props, events and commands.", nativePage},
		{"Animation", groupMedia, "Transitions, animated values and layout animations run by the platform.", animationPage},
		{"Gestures", groupMedia, "Pointer events, pans, pinches and rotations, sharing touches with scrolling.", gesturesPage},

		{"Window", groupSystem, "The desktop window's title bar and commands, and dragging it from a view.", windowPage},
		{"App Menu", groupSystem, "The application menu: roles, shortcuts and checkable items.", menuPage},
		{"Environment", groupSystem, "What the platform reports, and the system services.", environmentPage},
		{"Accessibility", groupSystem, "What screen readers read: hints, values, actions, groups, hidden elements, live regions and focus.", accessibilityPage},
		{"Storage", groupSystem, "Key-value storage kept on the device, and the app's directories.", storagePage},
		{"File Drop", groupSystem, "Files dragged in from other apps, on desktops and the web.", filesPage},
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

// groups returns the sections by group, in the list's order.
func groups() [][]section {
	var out [][]section
	for _, s := range sections {
		if n := len(out); n > 0 && out[n-1][0].group == s.group {
			out[n-1] = append(out[n-1], s)
		} else {
			out = append(out, []section{s})
		}
	}
	return out
}

// wideWidth is the narrowest window that shows the section list beside
// the page.
const wideWidth core.Points = 720

// listWidth is the width of the wide layout's section list.
const listWidth core.Points = 200

// gallery is the state the layouts share, so switching between them (a
// window resized) keeps what is open.
type gallery struct {
	// section is the selected section; on phones, the one last opened.
	section *core.Signal[string]
	// nav is the phone layout's stack: "" is the index, a name a section.
	nav *base.Nav[string]
	// descriptions shows each section's description line.
	descriptions *core.Signal[bool]
	// chosen is the label of the last application menu item chosen.
	chosen *core.Signal[string]
	// wide is whether the layout built last is the list's.
	wide bool
}

// galleryKey gives sections the shell's state.
var galleryKey = core.NewKey[*gallery]("gallery", nil)

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

// Root builds the gallery again when the window's width calls for the
// other layout or the appearance changes, keeping its state in a gallery.
// NSGO_GALLERY_OPEN names a section to open at start, for screenshots.
func Root(cx *core.Scope) core.Node {
	start := sections[0].name
	if s, _ := find(os.Getenv("NSGO_GALLERY_OPEN")); s.name != start {
		start = s.name
	}
	g := &gallery{
		section:      core.NewSignal(cx, start),
		nav:          base.NewNav(cx, ""),
		descriptions: core.NewSignal(cx, true),
		chosen:       core.NewSignal(cx, "nothing yet"),
	}
	if start != sections[0].name {
		g.nav.SetRoutes("", start)
	}
	galleryKey.Provide(cx, g)
	env := cx.Env()
	wide := core.NewMemo(cx, func() bool { return env.Window().Width >= wideWidth })

	cx.Effect(func() { cx.SetApplicationMenu(g.menu(cx)) })
	cx.Effect(func() { cx.SetTitle("NSGo Base — " + g.section.Get()) })
	// Pages step from a field too, as a browser's tabs do.
	always := core.KeyOptions{When: core.KeyAlways}
	cx.OnKeyWith("ctrl+tab", always, func() { g.step(1) })
	cx.OnKeyWith("ctrl+shift+tab", always, func() { g.step(-1) })

	return core.Dynamic(func(cx *core.Scope) core.Node {
		isWide, t := wide.Get(), newTheme(env.Dark())
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
	view := []core.MenuItem{
		{Label: "Show Descriptions", Checkable: true, Checked: g.descriptions.Get(),
			OnSelect: choose("Show Descriptions", func() { g.descriptions.Update(func(on bool) bool { return !on }) })},
		{Separator: true},
	}
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
	)
	return []core.MenuItem{
		{Label: "Base", Items: []core.MenuItem{
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
			{Role: core.MenuClose},
		}},
	}
}

// desktop is the wide layout: the section list beside the selected
// section, under the system's title bar.
func (g *gallery) desktop(cx *core.Scope, t *theme) core.Node {
	g.wide = true
	// Coming from the phone layout, show the section it had open.
	if routes := core.Untracked(cx, g.nav.Routes); len(routes) > 1 {
		g.section.Set(routes[len(routes)-1])
	}
	env := cx.Env()
	content := &core.Ref{}
	cx.Effect(func() {
		g.section.Get()
		content.ScrollTo(core.Point{}, false)
	})
	var list []core.Node
	for _, group := range groups() {
		var items []core.Node
		for _, s := range group {
			items = append(items, core.View{OnPress: func(core.PressEvent) { g.open(s.name) }, Role: core.RoleButton,
				Label: s.name, TestID: "nav-" + s.name, Style: t.navItem(),
				Selected: func() bool { return g.section.Get() == s.name },
				Children: []core.Node{core.Text{Content: s.name, MaxLines: 1}}})
		}
		list = append(list, core.View{TestID: "nav-group-" + group[0].group, Style: core.Style{Gap: 2}, Children: append([]core.Node{
			core.Text{Content: group[0].group, Role: core.RoleHeader, Style: t.caption().Merge(core.Style{
				FontWeight: core.FontWeightSemibold, Padding: core.Edges{Top: 8, Right: 10, Bottom: 4, Left: 10}})},
		}, items...)})
	}
	return core.View{
		// A hidden title bar leaves the window controls over the top edge,
		// so the layout keeps clear of the area they report.
		StyleFunc: func() core.Style {
			return core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0),
				Padding: core.Edges{Top: env.TitleBar().Top}, Background: t.background, Color: t.ink}
		},
		Children: []core.Node{
			// A scroll view shrinks by default, and Merge cannot set
			// FlexShrink back to 0, so the minimum keeps the list's width.
			core.Scroll{TestID: "nav-sections", Style: core.Style{Width: core.Pt(listWidth), MinWidth: core.Pt(listWidth), Background: t.sunken,
				Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: t.line}},
				Children: []core.Node{core.View{Style: core.Style{Padding: core.All(8), Gap: 8}, Children: list}}},
			core.Scroll{Ref: content, TestID: "content", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
				Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
					s, _ := find(g.section.Get())
					return core.Func(func(cx *core.Scope) core.Node {
						return core.View{Style: core.Style{Padding: core.All(24), Gap: 16}, Children: []core.Node{
							core.Text{Content: s.name, Role: core.RoleHeader, TestID: "title",
								Style: core.Style{FontSize: 22, FontWeight: core.FontWeightSemibold}},
							g.sectionBody(cx, t, s),
						}}
					})
				})}},
		}}
}

// sectionBody is a section's demos under its description.
func (g *gallery) sectionBody(cx *core.Scope, t *theme, s section) core.Node {
	about := core.Show{When: g.descriptions.Get, Then: func(*core.Scope) core.Node {
		return core.Text{Content: s.about, TestID: "about", Style: core.Style{Color: t.muted, FontSize: 15}}
	}}
	return core.View{Style: core.Style{Gap: 16}, TestID: "section-" + s.name,
		Children: []core.Node{about, s.page(cx, t)}}
}

// phone is the narrow layout: a stack of the index and the section opened
// over it. iOS and Android draw the system header; elsewhere the screens
// draw their own.
func (g *gallery) phone(cx *core.Scope, t *theme) core.Node {
	// Coming from the wide layout, keep its section open.
	if g.wide {
		g.nav.SetRoutes("", g.section.GetUntracked())
	}
	g.wide = false
	headerStyle := core.Style{Background: t.sunken, Color: t.ink, TintColor: t.accent}
	return g.nav.Stack(core.Style{FlexGrow: 1, Background: t.background, Color: t.ink},
		func(cx *core.Scope, name string, _ func() int) core.Screen {
			if name == "" {
				return core.Screen{Title: "Base", TestID: "index", HeaderStyle: headerStyle,
					Header:   g.phoneHeader(t, "Base", nil),
					Children: []core.Node{g.index(t)}}
			}
			s, _ := find(name)
			return core.Screen{Title: s.name, TestID: "screen-" + s.name, HeaderStyle: headerStyle,
				Header: g.phoneHeader(t, s.name, func() { g.nav.Pop(1) }),
				Children: []core.Node{core.Scroll{Style: core.Style{FlexGrow: 1, Background: t.background}, Children: []core.Node{
					base.SafeAreaView{Edges: base.Bottom, Style: core.Style{Padding: core.All(16)},
						Children: []core.Node{g.sectionBody(cx, t, s)}},
				}}}}
		})
}

// phoneHeader is a phone screen's bar: a back button and the title.
func (g *gallery) phoneHeader(t *theme, title string, back func()) core.Node {
	var children []core.Node
	if back != nil {
		children = append(children, base.Button{OnPress: func(core.PressEvent) { back() }, Label: "Back", Style: core.Style{Padding: core.Symmetric(6, 8)},
			Children: []core.Node{core.Text{Content: "‹ Back", Style: core.Style{Color: t.accent}}}})
	}
	children = append(children, core.Text{Content: title, Role: core.RoleHeader, MaxLines: 1,
		Style: core.Style{FontSize: 18, FontWeight: core.FontWeightSemibold, FlexGrow: 1, FlexShrink: 1}})
	return core.View{Style: core.Style{Background: t.sunken, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}},
		Children: []core.Node{base.SafeAreaView{Edges: base.Top | base.Left | base.Right, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
				Height: core.Pt(52), Padding: core.Symmetric(0, 8)}, Children: children}}}}}
}

// index is the phone layout's list of sections by group.
func (g *gallery) index(t *theme) core.Node {
	var items []core.Node
	for _, group := range groups() {
		items = append(items, core.Text{Content: group[0].group, Role: core.RoleHeader,
			Style: t.caption().Merge(core.Style{Padding: core.Edges{Top: 18, Right: 16, Bottom: 6, Left: 16}, FontWeight: core.FontWeightSemibold})})
		for _, s := range group {
			items = append(items, core.View{OnPress: func(core.PressEvent) { g.open(s.name) }, Role: core.RoleButton, Label: s.name, TestID: "nav-" + s.name,
				Style: core.Style{Gap: 2, Padding: core.Symmetric(10, 16), Background: t.surface,
					Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}, Active: &core.Style{Background: t.hover}},
				Children: []core.Node{
					core.Text{Content: s.name, Style: core.Style{FontSize: 15, FontWeight: core.FontWeightMedium}},
					core.Text{Content: s.about, MaxLines: 1, Style: t.caption()},
				}})
		}
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1, Background: t.background},
		Children: []core.Node{base.SafeAreaView{Edges: base.Bottom, Children: items}}}
}

func main() {
	// The Go fonts' small capitals, shipped in resources/fonts.
	opts := []nsgo.Option{nsgo.Fonts("fonts/Go-Smallcaps.ttf", "fonts/Go-Smallcaps-Italic.ttf")}
	for name, factory := range components {
		opts = append(opts, nsgo.Component(name, factory))
	}
	nsgo.Run(Root, opts...)
}
