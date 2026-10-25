package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func windowPage(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	return core.Show{When: func() bool { return env.Idiom() == core.IdiomDesktop },
		Else: func(*core.Scope) core.Node {
			return t.demo("Desktop only", "Window commands need a desktop window; phones, tablets and the web ignore them.",
				t.note("Run the gallery on macOS, Windows or Linux to try them."))
		},
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
				t.demo("Title bar", "SetTitleBar hides the system's title bar so the app draws its own; the window controls stay, in an area Env.TitleBar reports, which this layout keeps clear of.",
					flow(
						t.primary("Hide title bar", func() { cx.SetTitleBar(core.TitleBarHidden, 0) }),
						t.plain("Show title bar", func() { cx.SetTitleBar(core.TitleBarDefault, 0) }),
					),
					t.live(func() string {
						tb := env.TitleBar()
						if tb == (core.Edges{}) {
							return "The system draws the title bar."
						}
						return fmt.Sprintf("Hidden: %g pt tall, controls keep %g pt on the left and %g pt on the right.", tb.Top, tb.Left, tb.Right)
					}),
				),
				t.demo("Window commands", "What the window's own buttons do, asked for by the app.",
					flow(
						t.plain("Minimize", cx.MinimizeWindow),
						t.plain("Zoom", cx.ToggleMaximize),
						t.plain("Full screen", cx.ToggleFullScreen),
						t.destructive("Close", cx.CloseWindow),
					),
					t.note("Closing the window ends the gallery."),
				),
				t.demo("Frame and level", "SetWindowFrame moves and sizes the window from the main screen's top left; "+
					"a new height keeps the top edge. SetWindowLevel floats it above other windows or sinks it to the desktop.",
					flow(
						t.plain("640 × 480", func() { cx.SetWindowFrame(core.WindowFrame{Width: 640, Height: 480}) }),
						t.plain("Taller, animated", func() {
							cx.SetWindowFrame(core.WindowFrame{Height: env.Window().Height + 160, Animated: true})
						}),
						t.plain("Move to 80, 80", func() { cx.SetWindowFrame(core.WindowFrame{X: 80, Y: 80, Move: true}) }),
					),
					flow(
						t.plain("Normal level", func() { cx.SetWindowLevel(core.WindowLevelNormal) }),
						t.plain("Floating", func() { cx.SetWindowLevel(core.WindowLevelFloating) }),
						t.plain("Desktop", func() { cx.SetWindowLevel(core.WindowLevelDesktop) }),
					),
					flow(
						t.plain("Away for 2 s", func() {
							cx.MinimizeWindow()
							cx.AfterFunc(2*time.Second, cx.OrderFront)
						}),
					),
				),
				t.demo("Menu bar", "SetMenuBar hides the menu bar Windows and Linux draw until Alt is pressed alone; macOS's is the system's.",
					flow(
						t.plain("Always shown", func() { cx.SetMenuBar(core.MenuBarShown) }),
						t.plain("Auto-hide", func() { cx.SetMenuBar(core.MenuBarAutoHide) }),
					),
				),
				popupMenuDemo(cx, t),
				t.demo("Quit", "Quit ends the app; nsgo.Run then returns for the app's cleanup.",
					t.destructive("Quit", cx.Quit),
				),
				t.demo("Drag handle", "WindowDrag on a view moves the window from it; a double click zooms.",
					core.View{WindowDrag: core.WindowDragOn, Style: core.Style{Height: core.Pt(56), CornerRadius: radius, Background: t.sunken,
						Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, Padding: core.Symmetric(0, 16)},
						Children: []core.Node{
							t.note("Drag the window from here"),
							core.View{WindowDrag: core.WindowDragOff, Children: []core.Node{t.plain("Still clickable", func() {})}},
						}},
				),
			}}
		},
	}
}

// popupMenuDemo shows the platform's own context menu where the area is
// right-clicked or pressed.
func popupMenuDemo(cx *core.Scope, t *theme) core.Node {
	last := core.NewSignal(cx, "Nothing chosen yet.")
	items := func() []core.MenuItem {
		choose := func(s string) func() { return func() { last.Set("Chose " + s) } }
		return []core.MenuItem{
			{Label: "Cut", Shortcut: "mod+x", OnSelect: choose("Cut")},
			{Label: "Copy", Shortcut: "mod+c", OnSelect: choose("Copy")},
			{Label: "Paste", Disabled: true},
			{Separator: true},
			{Label: "Share", Items: []core.MenuItem{{Label: "Mail", OnSelect: choose("Mail")}, {Label: "Messages", OnSelect: choose("Messages")}}},
			{Label: "Pinned", Checked: true, OnSelect: choose("Pinned")},
		}
	}
	show := func(at core.Point) {
		cx.PopupMenu(items(), at, func() { last.Update(func(s string) string { return s + " (closed)" }) })
	}
	return t.demo("Native context menu", "PopupMenu shows the platform's own menu at a point, here where the area is right-clicked or pressed.",
		core.View{TestID: "popup-area", Style: core.Style{Height: core.Pt(72), CornerRadius: radius, Background: t.sunken,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			OnPress:       func(e core.PressEvent) { show(e.PagePosition) },
			OnContextMenu: func(e core.ContextMenuEvent) { show(e.PagePosition) },
			Children:      []core.Node{t.note("Right-click or press here")}},
		t.live(last.Get),
	)
}

func menuPage(cx *core.Scope, t *theme) core.Node {
	g := galleryKey.Use(cx)
	shortcut := func(i int) string { return cx.KeyLabel(fmt.Sprintf("mod+%d", i+1)) }
	var keys []core.Node
	for i, s := range sections[:9] {
		keys = append(keys, core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween,
			Padding: core.Symmetric(4, 0), Borders: core.Edges{Bottom: 1}, BorderColors: core.AllColors(t.line)}, Children: []core.Node{
			core.Text{Content: s.name},
			core.Text{Content: shortcut(i), Style: core.Style{FontFamily: "Menlo", FontSize: 13, Color: t.muted}},
		}})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("This gallery's menu", "SetApplicationMenu at the root: an app menu and an Edit menu of roles, a View menu of sections and a Window menu.",
			t.note("Roles (About, Hide, Quit, Undo … Select All, Minimize, Zoom) are carried out by the platform, with their usual labels and shortcuts."),
			t.live(func() string { return "Last chosen: " + g.chosen.Get() }),
		),
		t.demo("Shortcuts", "View menu items switch sections; ctrl+tab steps through them where there is no menu bar.",
			core.View{Children: keys},
		),
		t.demo("Checkable items", "View › Show Descriptions is checkable: it and this switch share one value.",
			t.toggle("Show descriptions", g.descriptions.Get, g.descriptions.Set),
		),
	}}
}

func environmentPage(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	keys := core.NewSignal(cx, 0)
	title := core.NewSignal(cx, "NSGo Base")
	cx.OnKey("mod+j", func() { keys.Update(func(n int) int { return n + 1 }) })

	edges := func(e core.Edges) string { return fmt.Sprintf("%g %g %g %g", e.Top, e.Right, e.Bottom, e.Left) }
	idioms := map[core.Idiom]string{core.IdiomPhone: "phone", core.IdiomTablet: "tablet", core.IdiomDesktop: "desktop",
		core.IdiomMac: "mac", core.IdiomTV: "tv"}
	values := []struct {
		name  string
		value func() string
	}{
		{"Window", func() string { w := env.Window(); return fmt.Sprintf("%g × %g pt", w.Width, w.Height) }},
		{"Safe area", func() string { return edges(env.SafeArea()) }},
		{"Title bar", func() string { return edges(env.TitleBar()) }},
		{"Dark", func() string { return fmt.Sprint(env.Dark()) }},
		{"Right to left", func() string { return fmt.Sprint(env.RTL()) }},
		{"Font scale", func() string { return fmt.Sprint(env.FontScale()) }},
		{"Pixel ratio", func() string { return fmt.Sprint(env.PixelRatio()) }},
		{"Locale", env.Locale},
		{"OS", func() string { return string(env.OS()) }},
		{"OS version", func() string { return env.OSVersion().String() }},
		{"Idiom", func() string { return idioms[env.Idiom()] }},
		{"Reduce motion", func() string { return fmt.Sprint(env.ReduceMotion()) }},
		{"Screen reader", func() string { return fmt.Sprint(env.ScreenReader()) }},
		{"Keyboard", func() string { return fmt.Sprintf("%g pt", env.KeyboardHeight()) }},
	}
	var table []core.Node
	for _, v := range values {
		table = append(table,
			core.Text{Content: v.name, Style: core.Style{Color: t.muted}},
			core.Text{ContentFunc: v.value, Style: core.Style{FontFamily: "Menlo", FontSize: 14}})
	}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Environment", "Env reports each part as a signal; this table updates as the window, appearance or settings change.",
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "max-content 1fr", ColumnGap: 24, RowGap: 6},
				Children: table},
		),
		t.demo("Keyboard shortcuts", "OnKey binds a key while its owner lives; this one only while this section shows.",
			t.live(func() string { return fmt.Sprintf("Press %s: pressed %d times", cx.KeyLabel("mod+j"), keys.Get()) }),
		),
		t.demo("Announcements", "Announce has the screen reader say something without moving focus.",
			t.plain("Announce", func() { cx.Announce("Hello from NSGo") }),
		),
		t.demo("Title", "SetTitle names the window on desktops and the page on the web.",
			core.Input{Value: title.Get, OnChange: title.Set, OnSubmit: cx.SetTitle, Style: t.field(), Label: "Title"},
			t.primary("Set title", func() { cx.SetTitle(strings.TrimSpace(title.GetUntracked())) }),
		),
		t.demo("Status bar", "SetStatusBar sets the app's default under the screens' and modals' own StatusBar.",
			flow(
				t.plain("Auto", func() { cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarAuto, Hidden: core.ToggleOff}) }),
				t.plain("Dark content", func() { cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarDark, Hidden: core.ToggleOff}) }),
				t.plain("Light content", func() {
					cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarLight, Hidden: core.ToggleOff})
				}),
				t.plain("Hidden", func() {
					cx.SetStatusBar(core.StatusBarSetting{Hidden: core.ToggleOn, Animated: core.ToggleOn})
				}),
			),
		),
	}}
}

// navigationPage shows base.Nav's stack of routes.
func navigationPage(cx *core.Scope, t *theme) core.Node {
	nav := base.NewNav(cx, 1)
	header := func(n int) core.Node {
		children := []core.Node{}
		if n > 1 {
			children = append(children, base.Button{OnPress: func(core.PressEvent) { nav.Pop(1) }, Label: "Back", Style: core.Style{Padding: core.Symmetric(4, 6)},
				Children: []core.Node{core.Text{Content: "‹ Back", Style: core.Style{Color: t.accent}}}})
		}
		children = append(children, core.Text{Content: fmt.Sprintf("Screen %d", n), Role: core.RoleHeader,
			Style: core.Style{FontWeight: core.FontWeightSemibold}})
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(10, 12),
			Background: t.surface, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}}, Children: children}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Stack", "base.Nav keeps routes and renders them as a core.Stack; back pops the innermost stack that can. "+
			"On iOS and Android each screen has the system header, with a back button and an action; "+
			"even screens hide the status bar while they show.",
			nav.Stack(core.Style{Height: core.Pt(240), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line, Overflow: core.Hidden,
				Background: t.background},
				func(cx *core.Scope, n int, depth func() int) core.Screen {
					var bar core.StatusBarSetting
					if n%2 == 0 {
						bar.Hidden = core.ToggleOn
					}
					return core.Screen{Title: fmt.Sprintf("Screen %d", n), Header: header(n), TestID: fmt.Sprintf("nav-screen-%d", n),
						StatusBar: bar, HeaderBackTitle: fmt.Sprintf("%d", n),
						HeaderStyle: core.Style{Background: t.surface, Color: t.ink, TintColor: t.accent},
						HeaderRight: base.Button{OnPress: func(core.PressEvent) { nav.Push(n + 1) }, Label: "Push", Style: core.Style{Padding: core.Symmetric(4, 6)},
							Children: []core.Node{core.Text{Content: "Push", Style: core.Style{Color: t.accent}}}},
						Children: []core.Node{core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: []core.Node{
							core.Text{ContentFunc: func() string { return fmt.Sprintf("Depth %d of a stack of routes.", depth()) }},
							flow(
								t.primary("Push", func() { nav.Push(n + 1) }),
								t.plain("Pop to root", nav.PopToRoot),
							),
						}}}}
				}),
		),
		headersDemo(cx, t),
		tabBarDemo(cx, t),
	}}
}

// headersDemo shows the system header's large title, which collapses as
// the list scrolls under it, and a transparent header the content shows
// through, on iOS and Android.
func headersDemo(cx *core.Scope, t *theme) core.Node {
	clear := core.NewSignal(cx, false)
	rows := func(n int) []core.Node {
		out := make([]core.Node, n)
		for i := range out {
			out[i] = core.View{Style: core.Style{Padding: core.Symmetric(12, 16), Borders: core.Edges{Bottom: 1},
				BorderColors: core.EdgeColors{Bottom: t.line}},
				Children: []core.Node{core.Text{Content: fmt.Sprintf("Message %d", i+1)}}}
		}
		return out
	}
	return t.demo("System header", "A large title collapses into the bar as the content scrolls under it; a transparent "+
		"header lays the content under it, and the scroll view at the top starts below it. Platforms measure each header.",
		flow(t.plain("Transparent header", func() { clear.Set(!clear.Get()) })),
		core.Stack{Style: core.Style{Height: core.Pt(300), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line,
			Overflow: core.Hidden, Background: t.background}, Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				transparent := clear.Get()
				return core.Screen{Title: "Inbox", HeaderLargeTitle: !transparent, HeaderTransparent: transparent, TestID: "large-title",
					Header: core.View{Style: core.Style{Padding: core.Symmetric(10, 12), Background: t.surface},
						Children: []core.Node{core.Text{Content: "Inbox", Role: core.RoleHeader, Style: core.Style{FontWeight: core.FontWeightSemibold}}}},
					Children: []core.Node{core.Scroll{Style: core.Style{FlexGrow: 1}, Children: rows(30)}}}
			}),
		}},
	)
}

// tabBarDemo shows the system's tab bar, which desktops and the web do
// not have.
func tabBarDemo(cx *core.Scope, t *theme) core.Node {
	tab := core.NewSignal(cx, 0)
	items := []core.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}, {Title: "Profile", Icon: "person"}}
	if cx.Env().OS() == core.OSAndroid {
		items = []core.TabBarItem{{Title: "Home", Icon: "android:ic_menu_today"}, {Title: "Search", Icon: "android:ic_menu_search"},
			{Title: "Profile", Icon: "android:ic_menu_myplaces"}}
	}
	return t.demo("System tab bar", "core.TabBar is the system's tab bar on iOS and Android; desktops and the web show nothing, "+
		"where base.Tabs is the app's own.",
		core.View{Style: core.Style{Height: core.Pt(220), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line,
			Overflow: core.Hidden, Background: t.background}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{core.Text{ContentFunc: func() string { return items[tab.Get()].Title + " tab" }}}},
			core.TabBar{Items: items, Selected: tab.Get, OnChange: tab.Set, TestID: "tab-bar"},
		}},
	)
}
