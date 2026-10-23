package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// galleryKey gives sections the shell's state.
var galleryKey = core.NewKey[*gallery]("gallery", nil)

func windowPage(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	return core.Show{When: func() bool { return env.Idiom() == core.IdiomDesktop },
		Else: func(*core.Scope) core.Node {
			return t.demo("Desktop only", "Window commands need a desktop window; phones, tablets and the web ignore them.",
				t.note("Run the gallery on macOS or Windows to try them."))
		},
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
				t.demo("Title bar", "The gallery hides the system's title bar with SetTitleBar: its sidebar's and header's top rows are WindowDrag handles clear of the window controls.",
					flow(
						t.primary("Hide title bar", func() { cx.SetTitleBar(core.TitleBarHidden, headerHeight) }),
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

func menuPage(cx *core.Scope, t *theme) core.Node {
	g := galleryKey.Use(cx)
	shortcut := func(i int) string { return fmt.Sprintf("%s+%d", modName(cx), i+1) }
	var keys []core.Node
	for i, s := range sections[:9] {
		keys = append(keys, core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween,
			Padding: core.Symmetric(4, 0), Borders: core.Edges{Bottom: 1}, BorderColors: core.AllColors(t.line)}, Children: []core.Node{
			core.Text{Content: s.name},
			core.Text{Content: shortcut(i), Style: core.Style{FontFamily: "Menlo", FontSize: 13, Color: t.muted}},
		}})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("The gallery's menu", "SetApplicationMenu at the root: an app menu and an Edit menu of roles, a View menu of sections and a Window menu.",
			t.note("Roles (About, Hide, Quit, Undo … Select All, Minimize, Zoom) are carried out by the platform, with their usual labels and shortcuts."),
			t.live(func() string { return "Last chosen: " + g.chosen.Get() }),
		),
		t.demo("Shortcuts", "View menu items switch sections; ctrl+tab steps through them where there is no menu bar.",
			core.View{Children: keys},
		),
		t.demo("Checkable items", "View › Show Descriptions is checkable: it and this switch share one value.",
			ui.Switch{Checked: g.descriptions.Get, OnChange: g.descriptions.Set, Label: "Show descriptions"},
		),
	}}
}

// modName is what "mod" is on this platform.
func modName(cx *core.Scope) string {
	switch cx.Env().OS() {
	case core.OSMacOS, core.OSIOS:
		return "⌘"
	}
	return "Ctrl"
}

func environmentPage(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	clip := core.NewSignal(cx, "Hello from NSGo")
	pasted := core.NewSignal(cx, "")
	keys := core.NewSignal(cx, 0)
	title := core.NewSignal(cx, "NSGo Gallery")
	cx.OnKey("mod+j", func() { keys.Update(func(n int) int { return n + 1 }) })

	edges := func(e core.Edges) string { return fmt.Sprintf("%g %g %g %g", e.Top, e.Right, e.Bottom, e.Left) }
	idioms := map[core.Idiom]string{core.IdiomPhone: "phone", core.IdiomTablet: "tablet", core.IdiomDesktop: "desktop"}
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
		t.demo("Clipboard", "SetClipboard writes text; ReadClipboard answers with what it holds.",
			core.Input{Value: clip.Get, OnChange: clip.Set, Style: t.field(), Label: "Clipboard text"},
			flow(
				t.primary("Copy", func() { cx.SetClipboard(clip.GetUntracked()) }),
				t.plain("Paste", func() { cx.ReadClipboard(pasted.Set) }),
			),
			t.live(func() string {
				if pasted.Get() == "" {
					return "Nothing pasted yet."
				}
				return "Pasted: " + pasted.Get()
			}),
		),
		t.demo("Keyboard shortcuts", "OnKey binds a key while its owner lives; this one only while this section shows.",
			t.live(func() string { return fmt.Sprintf("Press %s+J: pressed %d times", modName(cx), keys.Get()) }),
		),
		t.demo("Announcements", "Announce has the screen reader say something without moving focus.",
			t.plain("Announce", func() { cx.Announce("Hello from the NSGo gallery") }),
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

// navigationDemo shows base.Nav's stack of routes.
func navigationDemo(cx *core.Scope, t *theme) core.Node {
	nav := base.NewNav(cx, 1)
	header := func(n int) core.Node {
		children := []core.Node{}
		if n > 1 {
			children = append(children, base.Button{OnPress: func() { nav.Pop(1) }, Label: "Back", Style: core.Style{Padding: core.Symmetric(4, 6)},
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
						HeaderRight: base.Button{OnPress: func() { nav.Push(n + 1) }, Label: "Push", Style: core.Style{Padding: core.Symmetric(4, 6)},
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
	}}
}
