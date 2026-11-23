package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

func TestClickTrayByIndexAndTitle(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var primary int
	var selected int
	app := StartApp(t, func(cx *core.Scope) {
		desktop.NewTray(cx, desktop.TrayOptions{Title: "status", OnClick: func() { primary++ }})
		desktop.NewTray(cx, desktop.TrayOptions{Title: "tools", Menu: func() []core.MenuItem {
			return []core.MenuItem{{Label: "Settings", OnSelect: func() { selected++ }}}
		}})
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "main"} })
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	app.ClickTrayIndex(0, core.MouseButtonLeft)
	if primary != 1 {
		t.Fatalf("primary clicks = %d, want 1", primary)
	}

	want := core.Bounds{Origin: core.Point{X: 10, Y: 20}, Size: core.Size{Width: 30, Height: 22}}
	app.ClickTrayTitle("tools", core.MouseButtonRight, want)
	app.ChoosePopup("Settings")
	if selected != 1 {
		t.Fatalf("menu selections = %d, want 1", selected)
	}
}

func TestClickTrayReportsUnavailableBoundsByDefault(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var available bool
	app := StartApp(t, func(cx *core.Scope) {
		var tray *desktop.Tray
		tray = desktop.NewTray(cx, desktop.TrayOptions{Title: "status", OnClick: func() {
			_, available = tray.ClickBounds()
		}})
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "main"} })
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.ClickTray("title:status", core.MouseButtonLeft)
	if available {
		t.Fatal("headless tray click unexpectedly reported bounds")
	}
}
