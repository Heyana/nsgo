package nsgotest

import (
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

func TestClosedSurfaceStepNamesItsWindow(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the diagnostic in the headless model")
	}
	message := fails(t, func(tb testing.TB) {
		app := StartApp(tb, func(cx *core.Scope) {
			cx.SetMainView(func(*core.Scope) core.Node { return core.View{} })
			desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 180, Height: 120},
				func(*core.Scope) core.Node { return core.Text{Content: "Property"} })
		}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

		window := app.Surface("title:Inspector")
		if !window.Close() {
			t.Fatal("Inspector did not close")
		}
		window.Find("text:Property")
	})
	if !strings.Contains(message, `surface "Inspector" is closed`) {
		t.Fatalf("a step on a closed surface reported %q", message)
	}
}

func TestSurfaceConfigRecordsOpeningOptions(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	limits := desktop.WindowConstraints{MinWidth: 100, MinHeight: 80}
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.View{} })
		desktop.OpenWindow(cx, desktop.WindowOptions{
			Title: "Inspector", Width: 180, Height: 120, Workspaces: desktop.WindowWorkspacesAll,
			Constraints: limits, NoShadow: true,
		}, func(*core.Scope) core.Node { return core.Text{Content: "Property"} })
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	config := app.Surface("title:Inspector").Config()
	if config.ID == 0 || config.Kind != SurfaceWindow || config.Main || config.Title != "Inspector" ||
		config.Width != 180 || config.Height != 120 || config.Constraints != limits ||
		!config.NoShadow || config.Workspaces != desktop.WindowWorkspacesAll {
		t.Fatalf("Inspector opened as %+v", config)
	}
	if !slices.Contains(app.Opened, config) {
		t.Fatalf("opened surfaces = %+v, want Inspector %+v", app.Opened, config)
	}
}

func TestSurfaceConfigRecordsDockedPanel(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.View{} })
		desktop.OpenPanel(cx, desktop.PanelOptions{Width: 220, Height: 80, NoActivate: true,
			Dock: desktop.PanelDock{Edge: core.PlaceBottom, Display: "screen-2", Margin: 12}},
			func(*core.Scope) core.Node { return core.View{} })
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	config := app.Surface("index:1").Config()
	want := desktop.PanelDock{Edge: core.PlaceBottom, Display: "screen-2", Margin: 12}
	if config.Dock != want || !config.NoActivate || config.Width != 220 || config.Height != 80 {
		t.Fatalf("docked panel opened as %+v", config)
	}
}

func TestStatusBarRecordsResolvedSettings(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarLight, Animated: core.ToggleOn})
		return core.View{TestID: "hide", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {
			cx.SetStatusBar(core.StatusBarSetting{Hidden: core.ToggleOn})
		}}
	}, Environment(core.Environment{OS: core.OSIOS}))
	if want := (StatusBarConfig{Style: core.StatusBarLight, Animated: true}); app.StatusBar != want {
		t.Fatalf("status bar = %+v, want %+v", app.StatusBar, want)
	}
	app.Press("id:hide")
	if want := (StatusBarConfig{Style: core.StatusBarLight, Hidden: true, Animated: true}); app.StatusBar != want {
		t.Fatalf("status bar = %+v, want %+v", app.StatusBar, want)
	}
}
