package ui

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestSheetNamesItsDialog(t *testing.T) {
	app, _ := start(t, phoneEnv, func(*core.Scope) core.Node {
		return Sheet{Open: func() bool { return true }, Label: "Filters", TestID: "filters",
			Children: []core.Node{core.Text{Content: "Filter choices"}}}
	})
	panel := app.Find("id:filters")
	if panel.Props.Role != core.RoleDialog || app.Find("role:dialog Filters").ID != panel.ID {
		t.Fatalf("sheet role = %v, label = %q", panel.Props.Role, panel.Props.Label)
	}
}

func TestSheetLabelFuncKeepsItsOpenContent(t *testing.T) {
	var label *core.Signal[string]
	app, _ := start(t, phoneEnv, func(cx *core.Scope) core.Node {
		label = core.NewSignal(cx, "Filters")
		return Sheet{Open: func() bool { return true }, LabelFunc: label.Get, TestID: "filters",
			Children: []core.Node{Input{Label: "Search", TestID: "query"}}}
	})
	panel := app.Find("role:dialog Filters")
	app.Type("query", "draft")
	field := app.Find("query")
	focused := field.Focused
	app.Post(func() { label.Set("Search filters") })
	app.Settle()
	if updated := app.Find("role:dialog Search filters"); updated.ID != panel.ID || updated.ID != app.Find("filters").ID {
		t.Fatal("changing the accessible name replaced the sheet")
	}
	if updated := app.Find("query"); updated.ID != field.ID || updated.Text != "draft" || updated.Focused != focused {
		t.Fatal("changing the accessible name reset its input")
	}
}

func TestSheetCloseOnSwipe(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
	}{{"default", false}, {"enabled", true}} {
		t.Run(test.name, func(t *testing.T) {
			var open *core.Signal[bool]
			closed := 0
			app, _ := start(t, phoneEnv, func(cx *core.Scope) core.Node {
				open = core.NewSignal(cx, true)
				return Sheet{Open: open.Get, CloseOnSwipe: test.enabled, TestID: "filters",
					Style:    core.Style{Height: core.Pt(300)},
					OnClose:  func() { closed++; open.Set(false) },
					Children: []core.Node{core.Text{Content: "Filter choices", TestID: "choices"}}}
			})
			app.Advance(300 * time.Millisecond)
			app.Drag("filters", core.Point{Y: 120})
			app.Advance(250 * time.Millisecond)
			if test.enabled {
				if closed != 1 || app.Exists("choices") {
					t.Fatalf("swipe closed %d times, content visible = %v", closed, app.Exists("choices"))
				}
			} else if closed != 0 || !app.Exists("choices") {
				t.Fatalf("default sheet closed %d times, content visible = %v", closed, app.Exists("choices"))
			}
		})
	}
}

func TestSheetStyleOverridesSurface(t *testing.T) {
	app, _ := start(t, phoneEnv, func(*core.Scope) core.Node {
		return Sheet{Open: func() bool { return true },
			Style:    core.Style{Background: core.RGB(0x123456), CornerRadii: core.Corners{TopLeft: 28, TopRight: 28}, Padding: core.All(24)},
			Children: []core.Node{core.Text{Content: "Sheet body", TestID: "body"}}}
	})
	surface := app.Find("id:body").Parent
	if surface.Style.Background != core.RGB(0x123456) {
		t.Fatalf("sheet background = %x, want 123456", surface.Style.Background)
	}
	if surface.Style.Corners != (core.Corners{TopLeft: 28, TopRight: 28}) {
		t.Fatalf("sheet corners = %+v", surface.Style.Corners)
	}
	if body := app.Find("id:body"); body.Frame.Origin.X != 24 || body.Frame.Origin.Y != 41 {
		t.Fatalf("sheet padding was not applied: surface=%+v body=%+v", surface.Frame, body.Frame)
	}
}

// TestSheetCornerRadiusRoundsEveryCorner checks that a CornerRadius alone replaces the default rounded top rather than being outranked by it.
func TestSheetCornerRadiusRoundsEveryCorner(t *testing.T) {
	app, _ := start(t, phoneEnv, func(*core.Scope) core.Node {
		return Sheet{Open: func() bool { return true }, Style: core.Style{CornerRadius: 24},
			Children: []core.Node{core.Text{Content: "Sheet body", TestID: "body"}}}
	})
	surface := app.Find("id:body").Parent
	if surface.Style.CornerRadius != 24 || surface.Style.Corners != (core.Corners{}) {
		t.Fatalf("sheet corners = radius %v, corners %+v; want 24 on every corner", surface.Style.CornerRadius, surface.Style.Corners)
	}
}

// TestSheetStyleKeepsGrabberAndSafeArea checks that a style without padding keeps the grabber and the bottom safe-area inset.
func TestSheetStyleKeepsGrabberAndSafeArea(t *testing.T) {
	e := phoneEnv
	e.SafeArea = core.Edges{Top: 47, Bottom: 34}
	app, _ := start(t, e, func(*core.Scope) core.Node {
		return Sheet{Open: func() bool { return true }, Style: core.Style{Background: core.RGB(0x123456)},
			Children: []core.Node{core.Text{Content: "Sheet body", TestID: "body"}}}
	})
	body := app.Find("id:body")
	surface := body.Parent
	if grabber := surface.Children[0]; grabber.Frame.Size.Width != 36 || grabber.Frame.Size.Height != 5 {
		t.Fatalf("grabber = %+v, want 36x5", grabber.Frame)
	}
	if body.Frame.Origin.X != 16 || body.Frame.Origin.Y != 25 {
		t.Fatalf("body = %+v, want the default padding", body.Frame)
	}
	if below := surface.Frame.Size.Height - body.Frame.Origin.Y - body.Frame.Size.Height; below != 34+16 {
		t.Fatalf("space below the body = %v, want the safe area 34 + 16", below)
	}
}

// TestSheetStyleFuncFollowsAppearanceAndSignals checks that StyleFunc restyles the open sheet in place when the appearance or a signal it reads changes.
func TestSheetStyleFuncFollowsAppearanceAndSignals(t *testing.T) {
	for _, e := range []core.Environment{phoneEnv, func() core.Environment { d := phoneEnv; d.Dark = true; return d }()} {
		var radius *core.Signal[core.Points]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			radius = core.NewSignal(cx, core.Points(12))
			colors := ColorsOf(cx)
			return Sheet{Open: func() bool { return true }, StyleFunc: func() core.Style {
				return core.Style{Background: colors.Accent(), CornerRadii: core.Corners{TopLeft: radius.Get(), TopRight: radius.Get()}}
			}, Children: []core.Node{core.Text{Content: "Sheet body", TestID: "body"}}}
		})
		check := func(dark bool, r core.Points) {
			t.Helper()
			palette := core.DefaultTheme().Light
			if dark {
				palette = core.DefaultTheme().Dark
			}
			s := app.Find("id:body").Parent.Style
			if s.Background != palette.Accent {
				t.Fatalf("dark=%v: sheet background = %v, want %v", dark, s.Background, palette.Accent)
			}
			if s.Corners != (core.Corners{TopLeft: r, TopRight: r}) {
				t.Fatalf("dark=%v: sheet corners = %+v, want top %v", dark, s.Corners, r)
			}
		}
		check(e.Dark, 12)
		id := app.Find("id:body").Parent.ID
		e.Dark = !e.Dark
		app.SetEnvironment(e)
		check(e.Dark, 12)
		app.Post(func() { radius.Set(30) })
		app.Settle()
		check(e.Dark, 30)
		if app.Find("id:body").Parent.ID != id {
			t.Fatal("restyling rebuilt the sheet's surface")
		}
	}
}
