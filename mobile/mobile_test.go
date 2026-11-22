package mobile_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A web build that runs as iOS or Android has none of their components.
func TestIsAvailableSimulated(t *testing.T) {
	for _, os := range []core.OS{core.OSIOS, core.OSAndroid} {
		for _, simulated := range []bool{false, true} {
			var available bool
			nsgotest.Start(t, func(cx *core.Scope) core.Node {
				available = mobile.IsAvailable(cx)
				return core.View{}
			}, nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomPhone, Simulated: simulated}))
			if available == simulated {
				t.Errorf("%v, simulated %v: IsAvailable = %v", os, simulated, available)
			}
		}
	}
}

func TestIsUnavailableOnWatch(t *testing.T) {
	var available bool
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		available = mobile.IsAvailable(cx)
		return core.View{}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomWatch}))
	if available {
		t.Fatal("watchOS reported mobile system components")
	}
}

func TestTabBarEdgePlacementAndSystemChoice(t *testing.T) {
	var chosen int
	var reported core.VerticalBarEdge
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		selected := core.NewSignal(cx, 0)
		cx.Effect(func() { reported = cx.Env().VerticalBarEdge() })
		return mobile.TabBar{
			Items:    []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
			Selected: selected.Get,
			OnChange: func(i int) { chosen = i; selected.Set(i) },
			Edge:     mobile.TabBarAutomatic,
			Style:    core.Style{Width: core.Pt(72), Height: core.Pt(49)},
			TestID:   "tabs",
		}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.VerticalBarEdge(core.VerticalBarTrailing))

	if reported != core.VerticalBarTrailing {
		t.Fatalf("Env.VerticalBarEdge() = %v, want trailing", reported)
	}
	if got := app.Find("id:tabs").NativeProps["edge"]; got != "trailing" {
		t.Fatalf("edge prop = %v, want trailing", got)
	}
	app.SelectTab("id:tabs", 1)
	if chosen != 1 {
		t.Fatalf("SelectTab chose %d, want 1", chosen)
	}
	app.SetVerticalBarEdge(core.VerticalBarLeading)
	if reported != core.VerticalBarLeading {
		t.Fatalf("Env.VerticalBarEdge() after update = %v, want leading", reported)
	}
	if got := app.Find("id:tabs").NativeProps["edge"]; got != "leading" {
		t.Fatalf("edge prop after update = %v, want leading", got)
	}
}
