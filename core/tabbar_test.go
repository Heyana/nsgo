package core_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestTabBarTakesTheAccent(t *testing.T) {
	theme := core.DefaultTheme()
	theme.Light.Accent = core.RGB(0xbd481b)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(theme)
		return core.TabBar{Items: []core.TabBarItem{{Title: "Home", Icon: "house"}}, Style: core.Style{Height: core.Pt(49)}, TestID: "bar"}
	}, nsgotest.Environment(driver.Environment{OS: driver.OSIOS}))
	if got := app.Find("id:bar").NativeProps["tint"]; got != float64(core.RGB(0xbd481b)) {
		t.Fatalf("tint = %v, want the theme's accent", got)
	}
}

func TestTabBarSendsCustomPaletteRoles(t *testing.T) {
	fallback := core.DefaultTheme()
	fallback.Light.Accent, fallback.Light.Surface = core.RGB(0xbd481b), core.RGB(0x102030)
	legacy := fallback.Clone()
	legacy.Light.SetColor("accentContainer", core.RGB(0x708090))
	custom := legacy.Clone()
	custom.Light.SetColor("tabBarBackground", core.RGB(0x182430))
	custom.Light.SetColor("tabBarIdle", core.RGB(0x405060))
	custom.Light.SetColor("tabBarSelected", core.RGB(0xabcdef))
	custom.Light.SetColor("tabBarIndicator", core.RGB(0x305080))
	selectedOnly := core.DefaultTheme()
	selectedOnly.Light.SetColor("tabBarSelected", core.RGB(0x503090))
	var setTheme func(*core.Theme)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(fallback)
		setTheme = cx.SetTheme
		return core.TabBar{Items: []core.TabBarItem{{Title: "Home"}}, Style: core.Style{Height: core.Pt(80)}, TestID: "bar"}
	}, nsgotest.Environment(driver.Environment{OS: driver.OSAndroid}))
	for _, step := range []struct {
		name                                           string
		theme                                          *core.Theme
		background, inactive, activeContent, indicator core.Color
	}{
		{"shared roles", fallback, fallback.Light.Surface, fallback.Light.TextMuted, fallback.Light.Accent, fallback.Light.Accent.Faded(0.12)},
		{"accent container", legacy, legacy.Light.Surface, legacy.Light.TextMuted, legacy.Light.Accent, core.RGB(0x708090)},
		{"tab bar roles", custom, core.RGB(0x182430), core.RGB(0x405060), core.RGB(0xabcdef), core.RGB(0x305080)},
		{"selected role only", selectedOnly, 0, 0, core.RGB(0x503090), 0},
		{"default theme restored", core.DefaultTheme(), 0, 0, 0, 0},
	} {
		app.Post(func() { setTheme(step.theme) })
		app.Settle()
		props := app.Find("id:bar").NativeProps
		for name, want := range map[string]core.Color{
			"background": step.background, "inactive": step.inactive,
			"activeContent": step.activeContent, "indicator": step.indicator,
		} {
			if got := props[name]; got != float64(want) {
				t.Errorf("%s: %s = %v, want %v", step.name, name, got, want)
			}
		}
	}
}

// A web build that runs as iOS or Android has no system tab bar to show.
func TestTabBarShowsNothingSimulated(t *testing.T) {
	for _, os := range []core.OS{core.OSIOS, core.OSAndroid} {
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return core.TabBar{Items: []core.TabBarItem{{Title: "Home", Icon: "house"}}, TestID: "bar"}
		}, nsgotest.Environment(driver.Environment{OS: os, Idiom: driver.IdiomPhone, Simulated: true}))
		if v := app.FindMounted("id:bar"); v.Component != "" {
			t.Fatalf("%v: mounted the component %q", os, v.Component)
		}
	}
}

func TestTabBarSVGIsMobileAndReactive(t *testing.T) {
	for _, os := range []core.OS{core.OSAndroid, core.OSIOS} {
		t.Run(string(os), func(t *testing.T) {
			var set func(*core.SVG)
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				icon := core.NewSignal(cx, lucide.House())
				set = icon.Set
				return core.TabBar{TestID: "bar", Style: core.Style{Height: core.Pt(80)}, ItemsFunc: func() []core.TabBarItem {
					return []core.TabBarItem{{Title: "Home", Icon: "house", SVG: icon.Get()}}
				}}
			}, nsgotest.Environment(driver.Environment{OS: os}))
			item := func() map[string]any {
				return app.Find("id:bar").NativeProps["items"].([]any)[0].(map[string]any)
			}
			first, has := item()["drawing"].(*driver.Drawing)
			if !has || item()["icon"] != "house" {
				t.Fatalf("%s tab props = %+v", os, item())
			}
			if len(first.Ops) == 0 {
				t.Fatal("SVG produced no drawing")
			}
			app.Post(func() { set(lucide.Search()) })
			app.Settle()
			if item()["drawing"] == first {
				t.Fatal("the SVG did not update")
			}
			app.Post(func() { set(nil) })
			app.Settle()
			if _, ok := item()["drawing"]; ok {
				t.Fatal("a removed SVG left a drawing behind")
			}
		})
	}
}

func TestTabBarEdgeAndAutomatic(t *testing.T) {
	if got := core.TabBarBottom.String(); got != "bottom" {
		t.Errorf("TabBarBottom.String() = %q", got)
	}
	if got := core.TabBarLeading.String(); got != "leading" {
		t.Errorf("TabBarLeading.String() = %q", got)
	}
	if got := core.TabBarTrailing.String(); got != "trailing" {
		t.Errorf("TabBarTrailing.String() = %q", got)
	}
	if got := core.TabBarAutomatic.String(); got != "automatic" {
		t.Errorf("TabBarAutomatic.String() = %q", got)
	}
	if got := core.TabBarAutomatic.Resolve(nil); got != core.TabBarBottom {
		t.Errorf("TabBarAutomatic.Resolve(nil) = %v, want bottom", got)
	}

	var setEdge func(core.TabBarEdge)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		edge := core.NewSignal(cx, core.TabBarBottom)
		setEdge = edge.Set
		return core.TabBar{
			Items:    []core.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
			EdgeFunc: edge.Get,
			Style:    core.Style{Width: core.Pt(72), Height: core.Pt(49)},
			TestID:   "bar",
		}
	}, nsgotest.Environment(driver.Environment{OS: driver.OSIOS}))

	propEdge := func() any { return app.Find("id:bar").NativeProps["edge"] }
	if got := propEdge(); got != "bottom" {
		t.Fatalf("default edge = %v, want bottom", got)
	}
	app.Post(func() { setEdge(core.TabBarLeading) })
	app.Settle()
	if got := propEdge(); got != "leading" {
		t.Fatalf("leading edge = %v, want leading", got)
	}
	app.Post(func() { setEdge(core.TabBarTrailing) })
	app.Settle()
	if got := propEdge(); got != "trailing" {
		t.Fatalf("trailing edge = %v, want trailing", got)
	}
	app.Post(func() { setEdge(core.TabBarAutomatic) })
	app.Settle()
	if got := propEdge(); got != "bottom" {
		t.Fatalf("automatic with unspecified env edge = %v, want bottom", got)
	}
	app.SetVerticalBarEdge(core.VerticalBarTrailing)
	if got := propEdge(); got != "trailing" {
		t.Fatalf("automatic with trailing env edge = %v, want trailing", got)
	}
	app.SetVerticalBarEdge(core.VerticalBarLeading)
	if got := propEdge(); got != "leading" {
		t.Fatalf("automatic with leading env edge = %v, want leading", got)
	}
	app.SetVerticalBarEdge(core.VerticalBarUnspecified)
	if got := propEdge(); got != "bottom" {
		t.Fatalf("automatic restored to unspecified = %v, want bottom", got)
	}
}

func TestTabBarUpdatesBadgesWithoutChangingItems(t *testing.T) {
	var change func(core.TabBadge)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		badge := core.NewSignal(cx, core.TabBadge{Text: "99+"})
		change = badge.Set
		return core.TabBar{TestID: "bar", ItemsFunc: func() []core.TabBarItem {
			return []core.TabBarItem{{Title: "Inbox", Icon: "envelope", Badge: badge.Get()}, {Title: "Library", Icon: "books.vertical"}}
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}))
	bar := app.FindMounted("id:bar")
	items := bar.NativeProps["items"].([]any)
	for _, want := range []core.TabBadge{{Text: "99+"}, {Dot: true}, {}} {
		app.Post(func() { change(want) })
		app.Settle()
		shown := app.FindMounted("id:bar")
		if got := shown.TabBadge(0); got != want || shown.TabBadge(1) != (core.TabBadge{}) {
			t.Fatalf("badges = %v, %v, want %v, empty", got, shown.TabBadge(1), want)
		}
		if next := shown.NativeProps["items"].([]any); shown.ID != bar.ID || &items[0] != &next[0] {
			t.Fatal("updating the badge replaced the bar or its tab items")
		}
	}
}
