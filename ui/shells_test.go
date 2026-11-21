package ui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// windowEnv is a macOS window whose hidden title bar keeps 80 points for
// the traffic lights.
var windowEnv = func() core.Environment {
	e := desktopEnv
	e.TitleBar = core.Edges{Top: 28, Left: 80}
	return e
}()

// shell runs content under ui's theme alone, filling the
// window, as an app's root does, and returns the colors it shows.
func shell(t *testing.T, e core.Environment, content func(cx *core.Scope) core.Node) (*nsgotest.App, Palette) {
	t.Helper()
	var shown Palette
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(core.DefaultTheme())
		shown = core.Untracked(cx, ColorsOf(cx).Now)
		return core.Func(content)
	}, size(e), nsgotest.Environment(e))
	return app, shown
}

// within finds the view under v with testID, seen or not.
func within(v *nsgotest.View, testID string) *nsgotest.View {
	if v.Props.TestID == testID {
		return v
	}
	for _, c := range v.Children {
		if f := within(c, testID); f != nil {
			return f
		}
	}
	return nil
}

// x is v's distance from the start of its ancestor matching selector.
func x(app *nsgotest.App, selector string, v *nsgotest.View) core.Points {
	top := app.Find(selector)
	var out core.Points
	for ; v != nil && v != top; v = v.Parent {
		out += v.Frame.Origin.X
	}
	return out
}

// y is v's distance from the top of its ancestor matching selector.
func y(app *nsgotest.App, selector string, v *nsgotest.View) core.Points {
	top := app.Find(selector)
	var out core.Points
	for ; v != nil && v != top; v = v.Parent {
		out += v.Frame.Origin.Y
	}
	return out
}

// TestSidebarShell keeps the toggle after the window controls, slides
// the content over the sidebar, which keeps its width, and resizes the
// sidebar for the app.
func TestSidebarShell(t *testing.T) {
	open := true
	var width core.Points = 240
	app, p := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		o, w := core.NewSignal(cx, open), core.NewSignal(cx, width)
		return SidebarShell{TestID: "shell", ToggleLabel: "Toggle sidebar", ResizeLabel: "Resize sidebar",
			Open: o.Get, OnOpenChange: func(v bool) { open = v; o.Set(v) },
			Width: w.Get, OnWidthChange: func(v core.Points) { width = v; w.Set(v) },
			Sidebar:       []core.Node{Sidebar{TestID: "nav", Sections: []SidebarSection{{Items: []SidebarItem{{ID: "a", Text: "Inbox"}}}}}},
			SidebarFooter: []core.Node{Button{Text: "Settings"}},
			Header:        []core.Node{core.Text{Content: "Inbox", TestID: "title"}},
			Actions:       []core.Node{Button{Text: "Compose"}},
			Content:       []core.Node{core.Text{Content: "Body"}}}
	})
	toggleX := func() core.Points { return x(app, "shell", app.Find("role:button Toggle sidebar")) }
	if got := toggleX(); got != 80 {
		t.Fatalf("toggle at %v, want right after the window controls", got)
	}
	if got := app.Find("sidebar-header").Frame.Size.Height; got != HeaderHeight {
		t.Fatalf("header height = %v, want %v", got, HeaderHeight)
	}
	if got := app.Find("shell").Style.Background; got != p.Inset {
		t.Fatalf("chrome = %x, want the inset color without a material", got)
	}
	app.Find("sidebar-panel >> role:button Settings")
	if got := app.Find("sidebar-panel").Frame.Size.Width; got != 240 {
		t.Fatalf("sidebar width = %v, want 240", got)
	}
	if got := x(app, "shell", app.Find("title")); got != 240+12 {
		t.Fatalf("title at %v, want at the content's start", got)
	}
	app.AccessibilityAction("role:adjustable Resize sidebar", "increment")
	if width != 250 {
		t.Fatalf("width = %v after a step, want 250", width)
	}

	app.Press("role:button Toggle sidebar")
	if open {
		t.Fatal("the toggle left the sidebar open")
	}
	app.Advance(time.Second)
	if got := within(app.Find("shell"), "sidebar-slot").Frame.Size.Width; got != 0 {
		t.Fatalf("slot = %v, want closed", got)
	}
	if got := within(app.Find("shell"), "sidebar-panel").Frame.Size.Width; got != 250 {
		t.Fatalf("hiding sidebar = %v wide, want its width kept", got)
	}
	if got := toggleX(); got != 80 {
		t.Fatalf("toggle moved to %v", got)
	}
	if want := core.Points(80 + 32 + 8); x(app, "shell", app.Find("title")) != want {
		t.Fatalf("title at %v, want %v, after the toggle", x(app, "shell", app.Find("title")), want)
	}
	if app.Exists("sidebar-header") || app.Exists("role:adjustable Resize sidebar") {
		t.Fatal("the hidden sidebar is still seen")
	}
	app.Press("role:button Toggle sidebar")
	app.Advance(time.Second)
	if !open || within(app.Find("shell"), "sidebar-slot").Frame.Size.Width != 250 {
		t.Fatal("the sidebar came back without its width")
	}
}

// TestSidebarShellSafeArea keeps the shell's sidebar, footer, headers and content clear of the window's side safe area, open and closed, on both sides or one, while the panes run to the window's edges, and leaves none of it to their content: a NavHeader in the content pads by its own 12 points alone, and content padded by EnvSafeArea is not padded twice.
func TestSidebarShellSafeArea(t *testing.T) {
	for _, safe := range []core.Edges{{Left: 62, Right: 62}, {Right: 84}, {Left: 84}} {
		t.Run(fmt.Sprintf("left %v right %v", safe.Left, safe.Right), func(t *testing.T) {
			e := desktopEnv
			e.SafeArea = safe
			l, r := safe.Left, safe.Right
			var setOpen func(bool)
			app, _ := shell(t, e, func(cx *core.Scope) core.Node {
				o := core.NewSignal(cx, true)
				setOpen = o.Set
				return SidebarShell{TestID: "shell", ToggleLabel: "Toggle sidebar", Open: o.Get, OnOpenChange: o.Set, DefaultWidth: 240,
					Sidebar:       []core.Node{core.View{Style: core.Style{PaddingEnv: core.EnvAll(core.EnvSafeArea)}, Children: []core.Node{core.Text{Content: "Mail", TestID: "mail"}}}},
					SidebarFooter: []core.Node{Button{Text: "Settings"}},
					Header:        []core.Node{core.Text{Content: "Inbox", TestID: "title"}},
					Actions:       []core.Node{Button{Text: "Compose"}},
					Content: []core.Node{NavHeader{Title: "Page", OnBack: func() {}, BackLabel: "Back"},
						core.View{Style: core.Style{PaddingEnv: core.EnvAll(core.EnvSafeArea)}, Children: []core.Node{core.Text{Content: "Body", TestID: "body"}}}}}
			})
			at := func(name, what string, got, want core.Points) {
				t.Helper()
				if got != want {
					t.Errorf("%s: %s at %v, want %v", name, what, got, want)
				}
			}
			end := func(v *nsgotest.View) core.Points { return x(app, "shell", v) + v.Frame.Size.Width }
			check := func(name string, open bool) {
				t.Helper()
				at(name, "toggle", x(app, "shell", app.Find("role:button Toggle sidebar")), l+12)
				at(name, "content end", end(app.Find("content")), 1000)
				at(name, "trailing action end", end(app.Find("role:button Compose")), 1000-r-12)
				at(name, "content padded by EnvSafeArea, end", end(app.Find("body")), 1000-r)
				if open {
					at(name, "sidebar", x(app, "shell", app.Find("sidebar-panel")), 0)
					at(name, "sidebar content padded by EnvSafeArea", x(app, "shell", app.Find("mail")), l)
					at(name, "footer", x(app, "shell", app.Find("sidebar-footer")), l)
					at(name, "content", x(app, "shell", app.Find("content")), 240)
					at(name, "title", x(app, "shell", app.Find("title")), 240+12)
					at(name, "page back button", x(app, "shell", app.Find("role:button Back")), 240+12)
					at(name, "content padded by EnvSafeArea", x(app, "shell", app.Find("body")), 240)
					at(name, "resize handle", x(app, "shell", app.Find("sidebar-resize")), 240-2)
				} else {
					at(name, "content", x(app, "shell", app.Find("content")), 0)
					at(name, "title", x(app, "shell", app.Find("title")), l+12+32+8)
					at(name, "page back button", x(app, "shell", app.Find("role:button Back")), l+12)
					at(name, "content padded by EnvSafeArea", x(app, "shell", app.Find("body")), l)
				}
			}
			check("open", true)
			app.Post(func() { setOpen(false) })
			app.Advance(time.Second)
			check("closed", false)
		})
	}
}

// TestNavHeaderSafeArea adds the horizontal safe area to its usual padding.
func TestNavHeaderSafeArea(t *testing.T) {
	e := desktopEnv
	e.SafeArea = core.Edges{Left: 62, Right: 62}
	app, _ := shell(t, e, func(cx *core.Scope) core.Node {
		return NavHeader{TestID: "header", Title: "Inbox", OnBack: func() {}, BackLabel: "Back",
			Trailing: []core.Node{Button{Text: "More"}}}
	})
	header := app.Find("header")
	back := app.Find("role:button Back")
	more := app.Find("role:button More")
	if got := x(app, "header", back); got < 62+12 {
		t.Fatalf("back starts at %v, want at least %v", got, 62+12)
	}
	if got := x(app, "header", more) + more.Frame.Size.Width; got > header.Frame.Size.Width-62-12 {
		t.Fatalf("trailing button ends at %v, want at most %v", got, header.Frame.Size.Width-62-12)
	}
}

// TestSidebarShellFixed keeps a fixed sidebar at its width, its edge a
// line with no handle.
func TestSidebarShellFixed(t *testing.T) {
	app, p := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		return SidebarShell{TestID: "shell", ResizeLabel: "Resize sidebar", DefaultWidth: 280, Fixed: true,
			Sidebar: []core.Node{core.Text{Content: "Inbox"}}, Content: []core.Node{core.Text{Content: "Body"}}}
	})
	if got := app.Find("sidebar-panel").Frame.Size.Width; got != 280 {
		t.Fatalf("sidebar width = %v, want 280", got)
	}
	if app.Exists("role:adjustable Resize sidebar") {
		t.Fatal("a fixed sidebar offers a handle")
	}
	if got := within(app.Find("shell"), "sidebar-resize").Children[0].Style.Background; got != p.Border {
		t.Fatalf("edge = %x, want the line", got)
	}
}

// TestSidebarShellInset floats the content as a card with a margin and
// an edge that go as the sidebar hides, the window controls following it a frame at
// a time.
func TestSidebarShellInset(t *testing.T) {
	var o *core.Signal[bool]
	app, p := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		o = core.NewSignal(cx, true)
		return SidebarShell{TestID: "shell", Variant: SidebarShellInset, ToggleLabel: "Toggle sidebar",
			Open: o.Get, OnOpenChange: o.Set, Width: func() core.Points { return 240 },
			Header:  []core.Node{core.Text{Content: "Inbox", TestID: "title"}},
			Content: []core.Node{core.Text{Content: "Body"}}}
	})
	offset := func() core.Point {
		for i := len(app.Window) - 1; i >= 0; i-- {
			if app.Window[i].Action == nsgotest.WindowSetControls {
				return app.Window[i].Controls.Offset
			}
		}
		return core.Point{}
	}
	card := app.Find("content")
	if card.Style.Background != Alpha(p.Background, 0.9) || card.Style.CornerRadius != shellInsetRadius {
		t.Fatalf("content = %x with corners %v, want a card at 90%%", card.Style.Background, card.Style.CornerRadius)
	}
	if got := x(app, "shell", card); got != 5+240+5 {
		t.Fatalf("card at %v, want after the margin, the sidebar and a gap", got)
	}
	if got := y(app, "shell", card); got != 5 {
		t.Fatalf("card top = %v, want the margin", got)
	}
	if got := offset(); got != (core.Point{X: 5, Y: 5}) {
		t.Fatalf("window controls moved by %v, want the margin", got)
	}
	if got := app.Find("content-header").Style.Background; got != 0 {
		t.Fatalf("content header = %x, want the card's", got)
	}
	if card.Style.BorderWidth != 1 || card.Style.BorderColor != p.Line || card.Style.Effects == nil || len(card.Style.Effects.Shadows) == 0 {
		t.Fatalf("card edge = %v in %x, want an edge and a shadow", card.Style.BorderWidth, card.Style.BorderColor)
	}
	if got := within(app.Find("shell"), "sidebar-resize").Children[0].Style.Background; got != 0 {
		t.Fatalf("handle line = %x, want the card's edge alone", got)
	}

	app.Press("role:button Toggle sidebar")
	app.Advance(sidebarSlide / 2)
	if got := x(app, "shell", app.Find("content")); got <= 0 || got >= 250 {
		t.Fatalf("card at %v halfway, want it on its way", got)
	}
	if got := offset(); got.X <= 0 || got.X >= 5 {
		t.Fatalf("window controls jumped to %v, want them on their way", got)
	}
	app.Advance(time.Second)
	if got := x(app, "shell", app.Find("content")); got != 0 {
		t.Fatalf("card at %v once hidden, want the window's edge", got)
	}
	if got := offset(); got != (core.Point{}) {
		t.Fatalf("window controls moved by %v once hidden, want none", got)
	}
	if card := app.Find("content"); card.Style.BorderColor != Alpha(p.Line, 0) || card.Style.Effects != nil && len(card.Style.Effects.Shadows) > 0 {
		t.Fatalf("card edge = %x once hidden, want no edge or shadow", card.Style.BorderColor)
	}
}

// TestSidebarShellTitleSlides slides the content's title with the
// content, as the inset card's window controls and title bar follow it a
// frame at a time, until it meets the toggle, where it stays, both ways:
// the slide is laid out each frame, as the platform's layout animation
// would cut short, the title jumping to its place.
func TestSidebarShellTitleSlides(t *testing.T) {
	for _, c := range []struct {
		e core.Environment
		v SidebarShellVariant
	}{{windowEnv, SidebarShellPlain}, {desktopEnv, SidebarShellPlain}, {windowEnv, SidebarShellInset}, {desktopEnv, SidebarShellInset}} {
		e := c.e
		app, _ := shell(t, e, func(cx *core.Scope) core.Node {
			o := core.NewSignal(cx, true)
			return SidebarShell{TestID: "shell", Variant: c.v, ToggleLabel: "Toggle sidebar",
				Open: o.Get, OnOpenChange: o.Set, Width: func() core.Points { return 240 },
				Header:  []core.Node{core.Text{Content: "Inbox", TestID: "title"}},
				Content: []core.Node{core.Text{Content: "Body"}}}
		})
		offset := func() core.Point {
			for i := len(app.Window) - 1; i >= 0; i-- {
				if app.Window[i].Action == nsgotest.WindowSetControls {
					return app.Window[i].Controls.Offset
				}
			}
			return core.Point{}
		}
		// follow reports the title bar where the controls are, as macOS does.
		follow := func() {
			if e.TitleBar.Left > 0 {
				moved := e
				moved.TitleBar.Left += offset().X
				app.SetEnvironment(moved)
			}
		}
		title := func() core.Points { return x(app, "shell", app.Find("title")) }
		card := func() core.Points { return x(app, "shell", app.Find("content")) }
		follow()
		inCard := title() - card()
		app.Press("role:button Toggle sidebar")
		for range 20 {
			follow()
			app.Advance(sidebarSlide / 13)
		}
		follow()
		stop := title()
		if stop <= inCard {
			t.Fatalf("variant %d, title bar %v: title at %v once hidden, want it clear of the toggle", c.v, e.TitleBar, stop)
		}
		for _, open := range []bool{true, false} {
			app.Press("role:button Toggle sidebar")
			for i := range 20 {
				follow()
				// The toggle stopping it moves with the controls.
				want := max(card()+inCard, stop+offset().X)
				if got := title(); math.Abs(float64(got-want)) > 0.5 {
					t.Fatalf("variant %d, title bar %v, opening %v, frame %d: title at %v, want %v", c.v, e.TitleBar, open, i, got, want)
				}
				app.Advance(sidebarSlide / 13)
			}
		}
		if got := title(); got != stop {
			t.Fatalf("variant %d, title bar %v: title at %v once hidden again, want %v", c.v, e.TitleBar, got, stop)
		}
	}
}

// TestSidebarShellScrolls keeps a sidebar taller than the window within
// it, its items scrolling under the sidebar's header.
func TestSidebarShellScrolls(t *testing.T) {
	var items []SidebarItem
	for i := range 40 {
		items = append(items, SidebarItem{ID: fmt.Sprint(i), Text: fmt.Sprint("Item ", i), TestID: fmt.Sprint("item-", i)})
	}
	app, _ := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		return SidebarShell{TestID: "shell", Sidebar: []core.Node{Sidebar{TestID: "nav", Sections: []SidebarSection{{Items: items}}}}}
	})
	if got := app.Find("nav").Frame.Size.Height; got != 760-HeaderHeight {
		t.Fatalf("sidebar = %v tall, want the window's height under its header", got)
	}
	if got := y(app, "shell", app.Find("item-39")); got < 760 {
		t.Fatalf("the last item at %v without scrolling, want below the window", got)
	}
	app.Scroll("nav", 10000)
	scroll := app.Find("nav")
	if got, want := scroll.Offset.Y, scroll.Content.Height-scroll.Frame.Size.Height; got != want {
		t.Fatalf("scrolled to %v, want its end at %v", got, want)
	}
}

// TestShellMaterial leaves the chrome to a window material and keeps
// the content opaque.
func TestShellMaterial(t *testing.T) {
	e := windowEnv
	e.WindowMaterial = core.WindowMaterialSidebar
	app, p := shell(t, e, func(cx *core.Scope) core.Node {
		return SidebarShell{TestID: "shell", Content: []core.Node{core.Text{Content: "Body"}}}
	})
	if got := app.Find("shell").Style.Background; got != 0 {
		t.Fatalf("chrome = %x, want the material", got)
	}
	if got := app.Find("content-header").Style.Background; got != p.Background {
		t.Fatalf("content header = %x, want the background", got)
	}
}

// TestShellChromeStyle paints the chrome as the app asks.
func TestShellChromeStyle(t *testing.T) {
	paint := core.Style{Background: core.RGB(0x112233)}
	app, _ := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		return SidebarShell{TestID: "shell", ChromeStyle: func() core.Style { return paint }}
	})
	if got := app.Find("shell").Style.Background; got != paint.Background {
		t.Fatalf("chrome = %x, want the app's", got)
	}
}

// TestShellBackdrop fills the window behind the sidebar and the content.
func TestShellBackdrop(t *testing.T) {
	app, _ := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		return SidebarShell{TestID: "shell", Backdrop: []core.Node{core.View{TestID: "grain", Style: FillRest}},
			Content: []core.Node{core.Text{Content: "Body"}}}
	})
	backdrop := app.Find("backdrop")
	if backdrop.Parent != app.Find("shell") || backdrop.Parent.Children[0] != backdrop || backdrop.Frame.Size != app.Find("shell").Frame.Size {
		t.Fatal("the backdrop is not behind everything, filling the window")
	}
	app.Find("backdrop >> grain")
}

// TestPlainShell shows the title in the header, clear of the window
// controls at both ends, and the content below it or under it.
func TestPlainShell(t *testing.T) {
	for _, behind := range []bool{false, true} {
		e := windowEnv
		e.TitleBar.Right = 40
		app, _ := shell(t, e, func(cx *core.Scope) core.Node {
			return PlainShell{TestID: "shell", Title: "Notes", BehindHeader: behind,
				Actions: []core.Node{Button{Text: "Share"}},
				Content: []core.Node{core.View{TestID: "body", Style: core.Style{FlexGrow: 1}}}}
		})
		lead, share := app.Find("shell-header >> role:header Notes").Parent.Frame, app.Find("shell-header >> role:button Share").Frame
		if end := 1000 - share.Origin.X - share.Size.Width; lead.Origin.X != 80+12 || end != 40+12 {
			t.Fatalf("header starts at %v and ends %v before the edge, want clear of the window controls", lead.Origin.X, end)
		}
		top := app.Find("body").Parent.Frame.Origin.Y
		if !behind && top != HeaderHeight || behind && top != 0 {
			t.Fatalf("content top = %v with BehindHeader %v", top, behind)
		}
	}
}

// TestShellTitleBar hides the title bar for the header on its own, in
// every shell.
func TestShellTitleBar(t *testing.T) {
	for name, content := range map[string]core.Node{"sidebar": SidebarShell{}, "plain": PlainShell{}} {
		app, _ := shell(t, windowEnv, func(cx *core.Scope) core.Node { return content })
		var bar *nsgotest.WindowRequest
		for i := range app.Window {
			if app.Window[i].Action == nsgotest.WindowSetTitleBar {
				bar = &app.Window[i]
			}
		}
		if bar == nil || bar.TitleBar != core.TitleBarHidden || bar.Height != HeaderHeight {
			t.Fatalf("%s shell's title bar command = %+v, want hidden for %v", name, bar, HeaderHeight)
		}
	}
}

// TestShellPhone keeps the header clear of the status bar and leaves the
// window alone.
func TestShellPhone(t *testing.T) {
	e := phoneEnv
	e.SafeArea = core.Edges{Top: 47, Bottom: 34}
	app, _ := shell(t, e, func(cx *core.Scope) core.Node { return PlainShell{Title: "Notes"} })
	if len(app.Window) != 0 {
		t.Fatalf("window commands on a phone: %+v", app.Window)
	}
	if got := app.Find("shell-header >> role:header Notes").Parent.Frame.Origin.Y; got != 47 {
		t.Fatalf("header top = %v, want the safe area", got)
	}
}

// TestTabShellSystemBar takes the system's bar on a phone where every tab names its icon, and the drawn bar elsewhere, a web build that runs as iOS included.
func TestTabShellSystemBar(t *testing.T) {
	selected := 0
	tabs := func(cx *core.Scope) core.Node {
		sel := core.NewSignal(cx, 0)
		return TabShell{Label: "Sections", Selected: sel.Get, OnChange: func(i int) { selected = i; sel.Set(i) }, Tabs: []ShellTab{
			{Text: "Home", SystemIcon: "house", Content: func(cx *core.Scope) core.Node { return core.Text{Content: "Home page"} }},
			{Text: "Search", SystemIcon: "magnifyingglass", Content: func(cx *core.Scope) core.Node { return core.Text{Content: "Search page"} }},
		}}
	}
	phone := core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, Locale: "en-US"}
	app, _ := shell(t, phone, tabs)
	if app.Exists("role:tablist Sections") {
		t.Fatal("a phone draws the bar")
	}
	app.Emit("id:tab-bar", "change", float64(1))
	if selected != 1 || !app.Exists("Search page") {
		t.Fatal("the system's bar did not switch sections")
	}
	wide, _ := shell(t, desktopEnv, tabs)
	wide.Find("role:tablist Sections")
	phone.Simulated = true
	simulated, _ := shell(t, phone, tabs)
	simulated.Find("role:tablist Sections")
	if v := simulated.FindMounted("id:tab-bar"); v.Component != "" {
		t.Fatalf("simulated iOS mounted the component %q", v.Component)
	}
	selected = 0
	simulated.Press("role:tab Search")
	if selected != 1 || !simulated.Exists("Search page") {
		t.Fatal("the drawn bar did not switch sections")
	}
}

// TestTabShellSafeArea leaves no bottom safe area to the sections above a drawn bar, a nav page's included, following the safe area and giving it back while the keyboard hides the bar; under iOS's floating bar the sections keep it.
func TestTabShellRetitlesTheSelectedTabInPlace(t *testing.T) {
	headless(t)
	for _, tc := range []struct {
		name   string
		env    core.Environment
		native bool
	}{
		{"drawn", desktopEnv, false},
		{"iOS", core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}, true},
		{"Android", core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var scope *core.Scope
			builds := 0
			app, _ := shell(t, tc.env, func(cx *core.Scope) core.Node {
				scope = cx
				translated := func(english, french string) func() string {
					return func() string {
						if cx.Env().Locale() == "fr" {
							return french
						}
						return english
					}
				}
				return TabShell{LabelFunc: translated("Sections", "Rubriques"), Selected: func() int { return 1 }, Tabs: []ShellTab{
					{Text: "Home", SystemIcon: "house"},
					{TextFunc: translated("Search", "Rechercher"), SystemIcon: "magnifyingglass",
						Content: func(*core.Scope) core.Node {
							builds++
							return core.Text{TestID: "search-page", Content: "Search page"}
						}},
				}}
			})
			bar, page := app.FindMounted("id:tab-bar").ID, app.Find("id:search-page").ID
			if !tc.native {
				app.Focus("role:tab Search")
			}
			app.Post(func() { scope.SetLocale("fr") })
			app.Settle()
			updated := app.FindMounted("id:tab-bar")
			if updated.ID != bar || app.Find("id:search-page").ID != page || builds != 1 {
				t.Fatal("switching the locale replaced the bar or its selected page")
			}
			if tc.native {
				items := updated.NativeProps["items"].([]any)
				if updated.NativeProps["label"] != "Rubriques" || items[1].(map[string]any)["title"] != "Rechercher" || updated.NativeProps["selectedIndex"] != float64(1) {
					t.Fatalf("updated native bar = %+v", updated.NativeProps)
				}
			} else if tab := app.Find("role:tab Rechercher"); updated.Props.Label != "Rubriques" || !tab.Focused || !tab.Props.Selected || tab.Children[0].Text != "Rechercher" {
				t.Fatalf("updated drawn bar label = %q, tab = %+v (focused %t)", updated.Props.Label, tab.Props, tab.Focused)
			}
		})
	}
}

func TestTabShellSafeArea(t *testing.T) {
	var in core.Edges
	run := func(e core.Environment, icon string) *nsgotest.App {
		app, _ := shell(t, e, func(cx *core.Scope) core.Node {
			return TabShell{Label: "Sections", TestID: "shell", Tabs: []ShellTab{{Text: "Home", SystemIcon: icon, Content: func(cx *core.Scope) core.Node {
				nav := base.NewNav(cx, "home")
				return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, _ string, _ func() int) base.NavPage {
					return base.NavPage{Title: "Home", Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
						return core.View{TestID: "actions", Style: core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}},
							StyleFunc: func() core.Style { in = core.Insets(cx); return core.Style{} }}
					})}}
				})
			}}}}
		})
		return app
	}
	check := func(app *nsgotest.App, name string, bottom core.Points) {
		t.Helper()
		if in.Bottom != bottom {
			t.Fatalf("%s: a page's bottom inset = %v, want %v", name, in.Bottom, bottom)
		}
		if got := within(app.Find("shell"), "actions").Frame.Size.Height; got != bottom {
			t.Fatalf("%s: a bottom padding of the safe area = %v, want %v", name, got, bottom)
		}
	}
	e := desktopEnv
	e.SafeArea = core.Edges{Bottom: 34}
	app := run(e, "")
	check(app, "drawn bar", 0)
	if bar := app.Find("role:tablist Sections").Parent; bar.Frame.Size.Height <= 34 {
		t.Fatalf("drawn bar height = %v, want it to keep clear of the safe area", bar.Frame.Size.Height)
	}
	e.SafeArea.Left = 20
	app.SetEnvironment(e)
	if in.Left != 20 {
		t.Fatalf("a page's left inset = %v after the safe area changed, want 20", in.Left)
	}
	check(app, "drawn bar, safe area changed", 0)
	e.KeyboardHeight = 300
	app.SetEnvironment(e)
	check(app, "keyboard over the bar", 34)

	android := e
	android.OS, android.Idiom, android.KeyboardHeight, android.SafeArea = core.OSAndroid, core.IdiomPhone, 0, core.Edges{Top: 47, Bottom: 34}
	check(run(android, ""), "Android", 0)
	ios := android
	ios.OS = core.OSIOS
	check(run(ios, ""), "iOS drawn bar", 0)
	check(run(ios, "house"), "iOS system bar", 34)
}

// TestTabShellBarHidden restores the section's safe area and removes the drawn bar while a pushed page asks the shell to hide it.
func TestTabShellBarHidden(t *testing.T) {
	var hide func(bool)
	e := desktopEnv
	e.SafeArea = core.Edges{Bottom: 34}
	app, _ := shell(t, e, func(cx *core.Scope) core.Node {
		hidden := core.NewSignal(cx, false)
		hide = hidden.Set
		return TabShell{TestID: "shell", BarHidden: hidden.Get, Tabs: []ShellTab{{Text: "Home", Content: func(cx *core.Scope) core.Node {
			return core.View{TestID: "page", Style: FillRest, Children: []core.Node{
				core.View{TestID: "actions", Style: core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}},
			}}
		}}}}
	})
	check := func(name string, bar bool, bottom core.Points) {
		t.Helper()
		if app.Exists("role:tablist") != bar {
			t.Fatalf("%s: drawn bar shows = %v, want %v", name, !bar, bar)
		}
		if got := within(app.Find("shell"), "actions").Frame.Size.Height; got != bottom {
			t.Fatalf("%s: a bottom padding of the safe area = %v, want %v", name, got, bottom)
		}
	}
	check("shown", true, 0)
	app.Post(func() { hide(true) })
	app.Settle()
	check("hidden", false, 34)
	if got, want := app.Find("id:page").Frame.Size.Height, app.Find("id:shell").Frame.Size.Height; got != want {
		t.Fatalf("hidden bar left page height %v below shell %v", got, want)
	}
	app.Post(func() { hide(false) })
	app.Settle()
	check("shown again", true, 0)
}

// TestTabShellAndroidSVGBar puts SVG icons in Android's system bar, reactively and beside a SystemIcon, while iOS keeps to SF Symbols and desktops draw the bar.
func TestTabShellAndroidSVGBar(t *testing.T) {
	var set func(*core.SVG)
	built := 0
	tabs := func(cx *core.Scope) core.Node {
		selected, icon := core.NewSignal(cx, 0), core.NewSignal(cx, lucide.House())
		set = icon.Set
		return TabShell{Label: "Sections", Selected: selected.Get, OnChange: selected.Set, Tabs: []ShellTab{
			{Text: "Home", Icon: icon.Get, Content: func(*core.Scope) core.Node {
				built++
				return core.Text{Content: "Home page"}
			}},
			{Text: "Search", SystemIcon: "magnifyingglass", Icon: lucide.Search,
				Content: func(*core.Scope) core.Node { return core.Text{Content: "Search page"} }},
		}}
	}
	app, _ := shell(t, core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}, tabs)
	items := func() []any { return app.FindMounted("id:tab-bar").NativeProps["items"].([]any) }
	first := items()[0].(map[string]any)["drawing"].(*driver.Drawing)
	// An SF Symbol name shared with iOS names no Android drawable, so the tab carries its SVG for the bar to fall back on; a name that does name one takes precedence there.
	if shared := items()[1].(map[string]any); shared["icon"] != "magnifyingglass" || shared["drawing"] == nil {
		t.Fatalf("a SystemIcon tab lost its name or its SVG: %+v", shared)
	}
	if app.Exists("role:tablist Sections") || len(first.Ops) == 0 {
		t.Fatal("Android did not use native SVG icons")
	}
	app.Post(func() { set(lucide.Settings()) })
	app.Settle()
	if items()[0].(map[string]any)["drawing"] == first {
		t.Fatal("a reactive SVG did not update the native bar")
	}
	app.Emit("id:tab-bar", "change", float64(1))
	app.Find("Search page")
	app.Emit("id:tab-bar", "change", float64(0))
	app.Find("Home page")
	if built != 1 {
		t.Fatalf("Home built %d times", built)
	}
	// iOS still needs SF Symbols; desktops keep the drawn SVG bar at any width.
	for _, env := range []core.Environment{phoneEnv, desktopEnv} {
		other, _ := shell(t, env, tabs)
		other.Find("role:tablist Sections")
	}
	// Where every tab has an SF Symbol, iOS uses its bar without drawing the SVGs, which it cannot show.
	drawn := 0
	ios, _ := shell(t, phoneEnv, func(cx *core.Scope) core.Node {
		icon := func() *core.SVG { drawn++; return lucide.House() }
		return TabShell{Label: "Sections", Tabs: []ShellTab{{Text: "Home", SystemIcon: "house", Icon: icon}, {Text: "Search", SystemIcon: "magnifyingglass", Icon: icon}}}
	})
	for _, item := range ios.FindMounted("id:tab-bar").NativeProps["items"].([]any) {
		if tab := item.(map[string]any); tab["drawing"] != nil || tab["icon"] == nil {
			t.Fatalf("iOS tab = %+v, want its SF Symbol alone", tab)
		}
	}
	if drawn != 0 {
		t.Fatalf("iOS drew %d SVG icons for its system bar", drawn)
	}
}

// TestTabShell switches sections, keeping the first built while hidden,
// and hides the bar under the keyboard.
func TestTabShell(t *testing.T) {
	built, selected := 0, 0
	tabs := func(cx *core.Scope) core.Node {
		sel := core.NewSignal(cx, 0)
		return TabShell{Label: "Sections", Selected: sel.Get, OnChange: func(i int) { selected = i; sel.Set(i) }, Tabs: []ShellTab{
			{Text: "Home", Content: func(cx *core.Scope) core.Node {
				built++
				return core.Text{Content: "Home page"}
			}},
			{Text: "Search", Content: func(cx *core.Scope) core.Node { return core.Text{Content: "Search page"} }},
		}}
	}
	app, p := shell(t, desktopEnv, tabs)
	app.Find("Home page")
	if app.Exists("Search page") {
		t.Fatal("a section shows before it is selected")
	}
	app.Press("role:tablist Sections >> role:tab Search")
	if got := app.Find("role:tab Search >> Search").TextStyle.Color; got != p.Accent {
		t.Fatalf("selected tab = %x, want the accent", got)
	}
	if selected != 1 || !app.Exists("Search page") || app.Exists("Home page") {
		t.Fatal("the tab did not switch sections")
	}
	app.Press("role:tab Home")
	if app.Find("Home page"); built != 1 {
		t.Fatalf("the first section was built %d times, want once", built)
	}
	e := desktopEnv
	e.KeyboardHeight = 300
	app.SetEnvironment(e)
	if app.Exists("role:tablist Sections") {
		t.Fatal("the bar shows under the keyboard")
	}

	// A phone shows the drawn bar too, not the system's.
	selected = 0
	app, _ = shell(t, phoneEnv, tabs)
	app.Press("role:tablist Sections >> role:tab Search")
	if selected != 1 || !app.Exists("Search page") {
		t.Fatal("the phone's tab did not switch sections")
	}
}

// The content slides over the right panel, which stays against the window's
// edge, and draws the panel's handle on its own edge.
func TestSidebarShellRightPanel(t *testing.T) {
	var set func(bool)
	app, _ := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		o := core.NewSignal(cx, true)
		set = o.Set
		return SidebarShell{Content: []core.Node{core.View{Style: FillRest}}, Right: []core.Node{core.View{Style: FillRest}},
			RightOpen: o.Get, OnRightOpenChange: o.Set, RightResizeLabel: "Resize",
			RightWidth: func() core.Points { return 300 }, TestID: "shell"}
	})
	app.Settle()
	if got := x(app, "shell", app.Find("id:right-resize")) + 3; got != x(app, "shell", app.Find("id:right-slot")) {
		t.Fatalf("the handle ends at %v, not at the panel", got)
	}
	app.Post(func() { set(false) })
	app.Advance(sidebarSlide / 2)
	if got := within(app.Find("shell"), "right-slot").Frame.Size.Width; got != 0 {
		t.Fatalf("slot %v wide", got)
	}
	if got := within(app.Find("shell"), "right-panel").Frame.Size.Width; got != 300 {
		t.Fatalf("the panel narrowed to %v as the content slid over it", got)
	}
	app.Advance(sidebarSlide)
	if within(app.Find("shell"), "right-panel") != nil {
		t.Fatal("the panel stayed after the slide")
	}
}

// A panel's open state that follows more than itself slides the panel
// only when it changes, leaving the app's other changes unanimated.
func TestSidebarShellSlidesOnlyAsItOpens(t *testing.T) {
	var set func(int)
	app, _ := shell(t, windowEnv, func(cx *core.Scope) core.Node {
		rev := core.NewSignal(cx, 0)
		set = rev.Set
		open := func() bool { rev.Get(); return true }
		// The content changes with the state, as a tree's rows would.
		grows := core.View{StyleFunc: func() core.Style { return core.Style{Height: core.Pt(core.Points(20 + 20*rev.Get()))} }}
		return SidebarShell{Content: []core.Node{grows}, Right: []core.Node{core.View{Style: FillRest}},
			Open: open, RightOpen: open, RightWidth: func() core.Points { return 300 }, TestID: "shell"}
	})
	app.Settle()
	app.LayoutAnimation = nil
	app.Post(func() { set(1) })
	app.Settle()
	if app.LayoutAnimation != nil {
		t.Fatal("a change of the app's state slid the panels")
	}
}

// TestTabShellArrows moves between the tabs with the arrows, one Tab stop,
// where the system's tab bar is not available.
func TestTabShellArrows(t *testing.T) {
	var at *core.Signal[int]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		at = core.NewSignal(cx, 0)
		page := func(*core.Scope) core.Node { return core.View{} }
		return TabShell{Tabs: []ShellTab{{Text: "One", Content: page}, {Text: "Two", Content: page}}, Selected: at.Get, OnChange: at.Set}
	})
	if !app.Find("role:tab One").Props.Focusable || app.Find("role:tab Two").Props.Focusable {
		t.Fatal("the tab bar is not one stop of the focus")
	}
	app.Key("tab")
	app.Key("right")
	if at.Get() != 1 {
		t.Fatalf("Right moved to tab %d", at.Get())
	}
}
