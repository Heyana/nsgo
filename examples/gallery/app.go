// Command gallery shows NSGo's elements, components and services, a page
// each, in the design of Onevra's gallery: a sidebar of grouped pages
// beside the selected one on wide windows, a navigation stack of them on
// phones, and on both a command palette and a settings dialog. Compiled to
// the web, it is the official website, guide included.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

//go:embed logo.png
var logo []byte

// version is the gallery's, as nsgo.yaml gives it.
const version = "0.1.0"

// section is one page of the gallery.
type section struct {
	name  string
	group string
	icon  func() *core.SVG
	about string
	page  func(cx *core.Scope, t *theme) core.Node
}

// The sidebar's groups, in its order.
const (
	groupOverview   = "Overview"
	groupGuide      = "Guide"
	groupElements   = "Elements"
	groupComponents = "Components"
	groupMedia      = "Media"
	groupDesktop    = "Desktop & system"
	groupComposites = "Composites"
	groupTheming    = "Theming"
)

// sections are set in init because the App Menu section lists them.
var sections []section

func init() {
	sections = []section{
		{"Welcome", groupOverview, lucide.House, "", welcomePage},
		{"Base", groupOverview, lucide.Component, "The engine's unstyled parts: a pressable view, popups, menus, dialogs, sheets, navigation and safe areas, styled by the app.", basePage},
		{"Documentation", groupGuide, lucide.BookOpen, "Guides for building NSGo applications.", docsPage},

		{"Text", groupElements, lucide.Type, "Fonts, weights, decoration, alignment, line limits, links and selection.", textPage},
		{"Layout", groupElements, lucide.LayoutGrid, "Flexbox, wrapping, absolute positioning, grid and block layout.", layoutPage},
		{"Styles", groupElements, lucide.Paintbrush, "Backgrounds, borders, corners, shadows, transforms and state styles.", stylesPage},
		{"Images", groupElements, lucide.Image, "Embedded, remote, animated and vector images, placeholders and resize modes.", imagesPage},
		{"Icons", groupElements, lucide.Shapes, "The bundled Lucide pack, drawn in inherited text color at any size.", iconsPage},
		{"Canvas", groupElements, lucide.PenTool, "2D drawing recorded on the engine and played by the platform.", canvasPage},
		{"Lists", groupElements, lucide.List, "Virtualized lists: sticky sections, columns, a chat, a list in the page, and a keyed list you edit.", listsPage},
		{"Scrolling", groupElements, lucide.Scroll, "Paging, snapping, insets, pull to refresh, the scroll events and zooming.", scrollingPage},

		{"Buttons", groupComponents, lucide.MousePointerClick, "Buttons in each variant, checkboxes, switches, radios and tabs.", buttonsPage},
		{"Fields", groupComponents, lucide.TextCursorInput, "Inputs, selects, sliders, icon buttons and the platform field itself.", fieldsPage},
		{"Feedback", groupComponents, lucide.BellRing, "Badges, progress bars and toasts.", feedbackPage},
		{"Overlays", groupComponents, lucide.Layers, "Menus, popovers, tooltips, dialogs and sheets.", overlaysPage},
		{"Text View", groupComponents, lucide.FileText, "Markdown and HTML with code highlighting, tables, alerts, extensions and streaming.", textViewPage},

		{"Web View", groupMedia, lucide.Globe, "The platform's web view: inline HTML, a site, history and messages both ways.", webPage},
		{"Video", groupMedia, lucide.Clapperboard, "The platform's video player: playback the app drives, resize modes and the platform's controls.", videoPage},
		{"Native View", groupMedia, lucide.Puzzle, "The app's own platform views inside the tree, with props, events and commands.", nativePage},
		{"Animation", groupMedia, lucide.Sparkles, "Transitions, animated values and layout animations run by the platform.", animationPage},
		{"Gestures", groupMedia, lucide.Hand, "Pans, pinches and rotations, sharing touches with scrolling.", gesturesPage},

		{"Window", groupDesktop, lucide.AppWindow, "The gallery's own title bar and the desktop window's commands.", windowPage},
		{"App Menu", groupDesktop, lucide.Menu, "The application menu: roles, shortcuts and checkable items.", menuPage},
		{"Environment", groupDesktop, lucide.Monitor, "What the platform reports, and the system services.", environmentPage},
		{"Accessibility", groupDesktop, lucide.Accessibility, "What screen readers read: hints, values, actions, groups, hidden elements, live regions and focus.", accessibilityPage},
		{"Storage", groupDesktop, lucide.Database, "Key-value storage kept on the device, and the app's directories.", storagePage},
		{"File Drop", groupDesktop, lucide.FileUp, "Files dragged in from other apps, on desktops and the web.", filesPage},

		{"Sidebar", groupComposites, lucide.PanelLeft, "Sections of navigation items, with counts and heading actions.", sidebarPage},
		{"Settings", groupComposites, lucide.SlidersHorizontal, "A settings surface: categories, search, groups and rows.", settingsPage},
		{"Command Palette", groupComposites, lucide.Command, "Find and run commands from the keyboard.", commandPage},

		{"Theme", groupTheming, lucide.Palette, "One ThemeView restyles every component: NSGo's theme or one of the app's.", themePage},
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

// groups returns the sections by group, in the sidebar's order.
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

// wideWidth is the narrowest window that shows the sidebar.
const wideWidth core.Points = 720

// The gallery's preferences, kept in storage.
const (
	keySection      = "gallery.section"
	keyAppearance   = "gallery.appearance"
	keyDescriptions = "gallery.descriptions"
	keySidebar      = "gallery.sidebar"
	keyStart        = "gallery.start"
)

// gallery is the state the shell's layouts share, so switching between
// them (a window resized) keeps what is open.
type gallery struct {
	// section is the selected section; on phones, the one last opened.
	section *core.Signal[string]
	// nav is the phone layout's stack: "" is the index, a name a section.
	nav *base.Nav[string]
	// descriptions shows each section's description line.
	descriptions *core.Signal[bool]
	// appearance is "system", "light" or "dark"; dark is what it chooses.
	appearance *core.Signal[string]
	dark       func() bool
	// sidebar shows the wide layout's sidebar.
	sidebar *core.Signal[bool]
	// start is the page the gallery opens on: "welcome" or "last".
	start *core.Signal[string]
	// settings and palette open the settings dialog and the command
	// palette; category is the settings' page.
	settings, palette *core.Signal[bool]
	category          *core.Signal[string]
	// chosen is the label of the last application menu item chosen.
	chosen *core.Signal[string]
	// prefs are the settings dialog's template values, kept in memory.
	prefs *prefs
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

// Root builds the gallery again when the layout the window's width calls
// for changes, keeping its state in a gallery. NSGO_GALLERY_OPEN names a
// section to open at start, for screenshots.
func Root(cx *core.Scope) core.Node {
	store := cx.Storage()
	saved := func(key, def string) string {
		v, ok := def, false
		cx.Untrack(func() { v, ok = store.GetItem(key) })
		if !ok {
			return def
		}
		return v
	}
	opensOn := saved(keyStart, "welcome")
	start := sections[0].name
	if opensOn == "last" {
		start = saved(keySection, start)
	}
	if open := os.Getenv("NSGO_GALLERY_OPEN"); open != "" {
		start = open
	}
	if name := routeName(); name != "" {
		start = name
	}
	if s, _ := find(start); s.name != start {
		start = sections[0].name
	}
	g := &gallery{
		section:      core.NewSignal(cx, start),
		nav:          base.NewNav(cx, ""),
		descriptions: core.NewSignal(cx, saved(keyDescriptions, "true") == "true"),
		appearance:   core.NewSignal(cx, saved(keyAppearance, "system")),
		sidebar:      core.NewSignal(cx, saved(keySidebar, "true") == "true"),
		start:        core.NewSignal(cx, opensOn),
		settings:     core.NewSignal(cx, false),
		palette:      core.NewSignal(cx, false),
		category:     core.NewSignal(cx, "general"),
		chosen:       core.NewSignal(cx, "nothing yet"),
		prefs:        newPrefs(cx),
	}
	if start != sections[0].name {
		g.nav.SetRoutes("", start)
	}
	galleryKey.Provide(cx, g)
	listenRoute(cx, func(name string) {
		if g.section.GetUntracked() == name {
			return
		}
		if s, _ := find(name); s.name == name {
			g.open(name)
		}
	})
	addressShown := false
	cx.Effect(func() {
		setRoute(g.section.Get(), !addressShown)
		addressShown = true
	})
	env := cx.Env()
	g.dark = core.NewMemo(cx, func() bool {
		switch g.appearance.Get() {
		case "light":
			return false
		case "dark":
			return true
		}
		return env.Dark()
	}).Get
	wide := core.NewMemo(cx, func() bool { return env.Window().Width >= wideWidth })

	keep := func(key string, value func() string) {
		cx.Effect(func() { store.SetItem(key, value()) })
	}
	keep(keySection, g.section.Get)
	keep(keyAppearance, g.appearance.Get)
	keep(keyDescriptions, func() string { return strconv.FormatBool(g.descriptions.Get()) })
	keep(keySidebar, func() string { return strconv.FormatBool(g.sidebar.Get()) })
	keep(keyStart, g.start.Get)

	// The desktop layout draws the title bar's row itself where the window
	// keeps its controls without it; Linux's would lose them.
	cx.Effect(func() {
		switch env.OS() {
		case core.OSMacOS, core.OSWindows:
			cx.Untrack(func() { cx.SetTitleBar(core.TitleBarHidden, headerHeight) })
		}
	})
	cx.Effect(func() { cx.SetApplicationMenu(g.menu(cx)) })
	cx.Effect(func() { cx.SetTitle("NSGo Gallery — " + g.section.Get()) })
	cx.OnKey("ctrl+tab", func() { g.step(1) })
	cx.OnKey("ctrl+shift+tab", func() { g.step(-1) })
	cx.OnKey("mod+k", g.openPalette)
	cx.OnKey("mod+,", g.openSettings)
	cx.OnKey("mod+alt+s", g.toggleSidebar)

	return ui.ThemeView{Dark: g.dark, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		ui.ToastView{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				isWide := wide.Get()
				// What the layout builds mounts untracked, outside this function.
				return core.Func(func(cx *core.Scope) core.Node {
					if isWide {
						return g.desktop(cx)
					}
					return g.phone(cx)
				})
			}),
			g.settingsDialog(),
			g.commandPalette(),
		}},
	}}
}

func (g *gallery) openPalette() {
	g.settings.Set(false)
	g.palette.Set(true)
}

func (g *gallery) openSettings() {
	g.palette.Set(false)
	g.settings.Set(true)
}

func (g *gallery) toggleSidebar() { g.sidebar.Update(func(on bool) bool { return !on }) }

// menu is the application menu: the app menu, Edit, View (a section
// each, with shortcuts) and Window.
func (g *gallery) menu(cx *core.Scope) []core.MenuItem {
	selected, appearance := g.section.Get(), g.appearance.Get()
	choose := func(label string, fn func()) func() {
		return func() { g.chosen.Set(label); fn() }
	}
	view := []core.MenuItem{
		{Label: "Search…", Shortcut: "mod+k", OnSelect: choose("Search…", g.openPalette)},
		{Label: "Show Sidebar", Shortcut: "mod+alt+s", Checkable: true, Checked: g.sidebar.Get(),
			OnSelect: choose("Show Sidebar", g.toggleSidebar)},
		{Label: "Show Descriptions", Checkable: true, Checked: g.descriptions.Get(),
			OnSelect: choose("Show Descriptions", func() { g.descriptions.Update(func(on bool) bool { return !on }) })},
		{Separator: true},
	}
	var appearances []core.MenuItem
	for _, a := range appearanceOptions() {
		appearances = append(appearances, core.MenuItem{Label: a.Label, Checkable: true, Checked: appearance == a.Value,
			OnSelect: choose(a.Label, func() { g.appearance.Set(a.Value) })})
	}
	view = append(view, core.MenuItem{Label: "Appearance", Items: appearances}, core.MenuItem{Separator: true})
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
		{Label: "Gallery", Items: []core.MenuItem{
			{Role: core.MenuAbout}, {Separator: true},
			{Label: "Settings…", Shortcut: "mod+,", OnSelect: choose("Settings…", g.openSettings)}, {Separator: true},
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
			{Label: "Hide Title Bar", OnSelect: choose("Hide Title Bar", func() { cx.SetTitleBar(core.TitleBarHidden, headerHeight) })},
			{Label: "Show Title Bar", OnSelect: choose("Show Title Bar", func() { cx.SetTitleBar(core.TitleBarDefault, 0) })},
			{Separator: true}, {Role: core.MenuClose},
		}},
	}
}

// appearanceOptions are the choices of the gallery's appearance.
func appearanceOptions() []ui.SelectOption {
	return []ui.SelectOption{
		{Value: "system", Label: "System", Icon: lucide.SunMoon()},
		{Value: "light", Label: "Light", Icon: lucide.Sun()},
		{Value: "dark", Label: "Dark", Icon: lucide.Moon()},
	}
}

func main() {
	var opts []nsgo.Option
	for name, factory := range components {
		opts = append(opts, nsgo.Component(name, factory))
	}
	nsgo.Run(Root, opts...)
}
