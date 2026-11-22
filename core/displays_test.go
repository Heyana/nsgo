package core_test

import (
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestDisplaysFollowEnvironment(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		e := cx.Env()
		return core.Text{TestID: "count", ContentFunc: func() string { return strconv.Itoa(len(e.Displays())) }}
	}, nsgotest.Environment(core.Environment{Displays: []core.DisplayInfo{{ID: "main", Main: true}}}))
	if app.Find("id:count").Text != "1" {
		t.Fatalf("initial displays: %q", app.Find("id:count").Text)
	}
	app.SetEnvironment(core.Environment{Displays: []core.DisplayInfo{{ID: "main", Main: true}, {ID: "second"}}})
	if app.Find("id:count").Text != "2" {
		t.Fatalf("updated displays: %q", app.Find("id:count").Text)
	}
}

// TestDisplaysKeepReadersWhenUnchanged runs a reader of Displays again only when the displays change, not with every other change of the environment.
func TestDisplaysKeepReadersWhenUnchanged(t *testing.T) {
	runs := 0
	displays := []core.DisplayInfo{{ID: "main", Main: true}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		e := cx.Env()
		return core.Text{TestID: "count", ContentFunc: func() string { runs++; return strconv.Itoa(len(e.Displays())) }}
	}, nsgotest.Environment(core.Environment{Displays: displays}))
	before := runs
	app.SetEnvironment(core.Environment{Displays: displays, Locale: "fr-FR"})
	if runs != before {
		t.Fatalf("the reader ran %d more times for unchanged displays", runs-before)
	}
}

func TestDisplayTracksTheWindowsDisplay(t *testing.T) {
	displays := []core.DisplayInfo{{ID: "desk", RefreshRate: 60}, {ID: "fast", RefreshRate: 120, MaxRefreshRate: 144}}
	env := core.Environment{Displays: displays, DisplayID: "desk"}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Text{TestID: "display", ContentFunc: func() string {
			d := cx.Env().Display()
			return d.ID + ":" + strconv.FormatFloat(d.RefreshRate, 'f', -1, 64)
		}}
	}, nsgotest.Environment(env))
	check := func(want string) {
		t.Helper()
		if got := app.Find("id:display").Text; got != want {
			t.Fatalf("current display = %q, want %q", got, want)
		}
	}
	check("desk:60")
	env.DisplayID = "fast"
	app.SetEnvironment(env)
	check("fast:120")
	env.Displays = []core.DisplayInfo{displays[0], {ID: "fast", RefreshRate: 144, MaxRefreshRate: 144}}
	app.SetEnvironment(env)
	check("fast:144")
	env.Displays = env.Displays[:1]
	app.SetEnvironment(env)
	check(":0")
}
