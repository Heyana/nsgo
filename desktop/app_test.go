package desktop_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestLaunchedAtLoginChoosesInitialView(t *testing.T) {
	for _, login := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "login"}[login], func(t *testing.T) {
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				// Read once when the app first runs, as a tray app decides whether to open its welcome window.
				if desktop.LaunchedAtLogin(cx) {
					return core.Text{Content: "Background"}
				}
				return core.Text{Content: "Welcome"}
			}, nsgotest.Environment(core.Environment{LaunchedAtLogin: login}))
			if login {
				app.Find("text:Background")
			} else {
				app.Find("text:Welcome")
			}
		})
	}
}

func TestWindowLevelAndPlacementFollowTheEnvironment(t *testing.T) {
	var cx *core.Scope
	app := nsgotest.Start(t, func(c *core.Scope) core.Node {
		cx = c
		text := func(s string) func(*core.Scope) core.Node {
			return func(*core.Scope) core.Node { return core.Text{Content: s} }
		}
		return core.View{Children: []core.Node{
			core.Show{When: func() bool { return desktop.CanSetWindowLevel(c) }, Then: text("Floats"), Else: text("Stacked")},
			core.Show{When: func() bool { return desktop.CanPlaceWindows(c) }, Then: text("Placed"), Else: text("Unplaced")},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop}))
	// As on Wayland: the app tells, and the level still goes to the platform, which ignores it.
	app.Find("text:Stacked")
	app.Find("text:Unplaced")
	cx.Post(func() { desktop.SetWindowLevel(cx, desktop.WindowLevelFloating) })
	app.Settle()
	if n := len(app.Window); n == 0 || app.Window[n-1] != (nsgotest.WindowRequest{Action: nsgotest.WindowSetLevel, Level: desktop.WindowLevelFloating}) {
		t.Fatalf("window commands = %+v", app.Window)
	}
	// As on X11, then macOS and Windows.
	app.SetEnvironment(core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop, CanSetWindowLevel: true})
	app.Find("text:Floats")
	app.Find("text:Unplaced")
	app.SetEnvironment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop, CanSetWindowLevel: true, CanPlaceWindows: true})
	app.Find("text:Floats")
	app.Find("text:Placed")
}

// TestWindowWorkspacesFollowTheEnvironment checks that an app can tell whether SetWindowWorkspaces takes effect, as on Windows and Wayland, where it does not and the request still goes to the platform, and on macOS and X11, where it does.
func TestWindowWorkspacesFollowTheEnvironment(t *testing.T) {
	var cx *core.Scope
	app := nsgotest.Start(t, func(c *core.Scope) core.Node {
		cx = c
		text := func(s string) func(*core.Scope) core.Node {
			return func(*core.Scope) core.Node { return core.Text{Content: s} }
		}
		return core.Show{When: func() bool { return desktop.CanSetWindowWorkspaces(c) }, Then: text("Everywhere"), Else: text("Here")}
	}, nsgotest.Environment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop}))
	app.Find("text:Here")
	cx.Post(func() { desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll) })
	app.Settle()
	if n := len(app.Window); n == 0 || app.Window[n-1] != (nsgotest.WindowRequest{Action: nsgotest.WindowSetWorkspaces, Workspaces: desktop.WindowWorkspacesAll}) {
		t.Fatalf("window commands = %+v", app.Window)
	}
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, CanSetWindowWorkspaces: true})
	app.Find("text:Everywhere")
}

func TestFrontmostAppFollowsTheAppInFront(t *testing.T) {
	editor := desktop.FrontmostAppInfo{ID: "com.example.editor", Name: "Editor", PID: 41}
	player := desktop.FrontmostAppInfo{ID: "com.example.player", Name: "Player", PID: 42, FullScreen: true}
	var seen []desktop.FrontmostAppInfo
	var can []bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { seen = append(seen, desktop.FrontmostApp(cx)) })
		cx.Effect(func() { can = append(can, desktop.CanReadFrontmostApp(cx)) })
		return core.View{}
	}, nsgotest.Environment(core.Environment{FrontmostApp: editor, CanReadFrontmostApp: true}))
	app.SetEnvironment(core.Environment{FrontmostApp: player, CanReadFrontmostApp: true})
	// Another change to the environment leaves the app in front alone.
	app.SetEnvironment(core.Environment{FrontmostApp: player, CanReadFrontmostApp: true, Dark: true})
	// As on Wayland: a platform that does not follow the app in front reports none, whatever it holds.
	app.SetEnvironment(core.Environment{FrontmostApp: player})
	if want := []desktop.FrontmostAppInfo{editor, player, {}}; !slices.Equal(seen, want) {
		t.Fatalf("frontmost apps %v, want %v", seen, want)
	}
	if want := []bool{true, false}; !slices.Equal(can, want) {
		t.Fatalf("CanReadFrontmostApp %v, want %v", can, want)
	}
}

// refuseAll is the Access of code that may reach nothing.
type refuseAll struct{}

func (refuseAll) Allows(core.AccessRequest) bool { return false }
func (refuseAll) WebHosts() []string             { return nil }

// onlyFrontmost allows the frontmost app alone.
type onlyFrontmost struct{}

func (onlyFrontmost) Allows(req core.AccessRequest) bool {
	return req.Permission == core.AccessFrontmostApp
}
func (onlyFrontmost) WebHosts() []string { return nil }

func TestFrontmostAppIsHeldToAccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access core.Access
		want   string
	}{
		{"refused", refuseAll{}, "text:none false"},
		{"granted", onlyFrontmost{}, "text:Editor true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				cx.ProvideAccess(tc.access)
				name := desktop.FrontmostApp(cx).Name
				if name == "" {
					name = "none"
				}
				return core.Text{Content: fmt.Sprintf("%s %v", name, desktop.CanReadFrontmostApp(cx))}
			}, nsgotest.Environment(core.Environment{FrontmostApp: desktop.FrontmostAppInfo{ID: "com.example.editor", Name: "Editor"}, CanReadFrontmostApp: true}))
			app.Find(tc.want)
		})
	}
}

// A web build that runs as a desktop OS has none of its window functions.
func TestIsAvailableSimulated(t *testing.T) {
	for _, simulated := range []bool{false, true} {
		var available bool
		nsgotest.Start(t, func(cx *core.Scope) core.Node {
			available = desktop.IsAvailable(cx)
			return core.View{}
		}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Simulated: simulated}))
		if available == simulated {
			t.Errorf("simulated %v: IsAvailable = %v", simulated, available)
		}
	}
}

func TestMenuIconsRecordedInMenus(t *testing.T) {
	svg := core.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="currentColor"/></svg>`))
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	var cx *core.Scope
	app := nsgotest.Start(t, func(c *core.Scope) core.Node {
		cx = c
		desktop.SetApplicationMenu(c, []desktop.MenuItem{{
			Label: "File",
			Items: []desktop.MenuItem{
				{Label: "New", Icon: desktop.MenuIcon{Data: pngBytes, Template: true}},
				{Label: "Open", Icon: desktop.MenuIcon{SVG: svg}},
			},
		}})
		desktop.SetDockMenu(c, []desktop.MenuItem{
			{Label: "Recent", Icon: desktop.MenuIcon{Source: core.ImageSource{URI: "asset:/recent.png"}}},
		})
		return core.View{}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	if len(app.Menu) != 1 || len(app.Menu[0].Items) != 2 {
		t.Fatalf("app.Menu = %+v", app.Menu)
	}
	if !app.Menu[0].Items[0].Icon.Equal(desktop.MenuIcon{Data: pngBytes, Template: true}) {
		t.Errorf("New icon = %+v", app.Menu[0].Items[0].Icon)
	}
	if app.Menu[0].Items[1].Icon.SVG != svg {
		t.Errorf("Open icon = %+v", app.Menu[0].Items[1].Icon)
	}
	if len(app.DockMenu) != 1 || app.DockMenu[0].Icon.Source.URI != "asset:/recent.png" {
		t.Errorf("DockMenu = %+v", app.DockMenu)
	}

	cx.Post(func() {
		desktop.PopupMenu(cx, []desktop.MenuItem{
			{Label: "Copy", Icon: desktop.MenuIcon{SVG: svg}},
		}, core.Point{X: 10, Y: 20}, nil)
	})
	app.Settle()
	if len(app.PopupMenu) != 1 || app.PopupMenu[0].Icon.SVG != svg {
		t.Errorf("PopupMenu = %+v", app.PopupMenu)
	}
}

// desktop.OpenWindow and desktop.OpenPanel each mount DevTools' overlay when devtools.MountOverlay is installed, and unregister cleanly on Close.
func TestDesktopOpenWindowAndPanelMountDevToolsOverlay(t *testing.T) {
	prev := devtools.MountOverlay
	var mounted, cleaned []int
	devtools.MountOverlay = func(s devtools.Surface) (any, func(), func()) {
		return core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
			func() { mounted = append(mounted, s.ID()) },
			func() { cleaned = append(cleaned, s.ID()) }
	}
	t.Cleanup(func() { devtools.MountOverlay = prev })

	root := core.NewRoot(core.RootOptions{Post: func(fn func()) { fn() }, Windows: &core.WindowOptions{Width: 400, Height: 300}})
	app := core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()

	var mainWin, secWin, panel *desktop.Window
	app.Mount(core.AppRoot(func(cx *core.Scope) {
		mainWin = desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Main"}, func(cx *core.Scope) core.Node {
			return core.View{TestID: "main"}
		})
		desktop.SetMainWindow(cx, mainWin)
		secWin = desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Settings"}, func(cx *core.Scope) core.Node {
			return core.View{TestID: "settings"}
		})
		panel = desktop.OpenPanel(cx, desktop.PanelOptions{Width: 280, Height: 180}, func(cx *core.Scope) core.Node {
			return core.View{TestID: "panel"}
		})
	}))
	app.Commit()

	if len(mounted) != 3 {
		t.Fatalf("mounted overlays on surfaces %v, want 3 (main window, secondary window, panel — excluding hidden app tree 0)", mounted)
	}
	if slices.Contains(mounted, 0) {
		t.Fatalf("mounted overlay on hidden app coordinator tree 0: %v", mounted)
	}
	for _, s := range app.Surfaces() {
		b := s.Commit()
		foundOverlay := false
		if b != nil {
			for _, it := range b.Items {
				if it.Kind == driver.ItemViewProps && it.ViewProps != nil && it.ViewProps.TestID == devtools.OverlayTestID {
					foundOverlay = true
				}
			}
		}
		if !foundOverlay {
			t.Errorf("surface %d (%v) did not mount DevTools' overlay", s.ID(), s.Window())
		}
	}

	secWin.Close()
	panel.Close()
	if len(cleaned) != 2 {
		t.Fatalf("cleaned up %v after closing secondary window and panel, want 2", cleaned)
	}
}
