package press_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPressReportsFocus(t *testing.T) {
	inner := new(core.Ref)
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20), Gap: 8}, Children: []core.Node{
			core.Input{TestID: "direct", Style: core.Style{Height: core.Pt(32)}},
			core.View{TestID: "search", OnPress: func(core.PressEvent) { inner.Focus() }, Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
				core.Input{TestID: "inner", Ref: inner, Style: core.Style{Height: core.Pt(32)}},
			}},
		}}
	}, nsgotest.Size(320, 240))
	check := func(focused, blurred string) {
		t.Helper()
		if !app.Find(focused).Focused || app.Find(blurred).Focused {
			t.Fatalf("focus after press: %s=%v, %s=%v", focused, app.Find(focused).Focused, blurred, app.Find(blurred).Focused)
		}
	}
	// The search bar's padding takes the press, and its handler focuses its field.
	app.PressAt("id:search", core.Point{X: 10, Y: 10})
	check("id:inner", "id:direct")
	app.Press("id:direct")
	check("id:direct", "id:inner")
	if target := nsgotest.Target(); target != "watchos" && target != "watchos_simulator" {
		app.PressAtWith("id:search", core.Point{X: 10, Y: 10}, core.Modifiers{Shift: true})
		check("id:inner", "id:direct")
		app.PressWith("id:direct", core.Modifiers{Shift: true})
		check("id:direct", "id:inner")
	}
	app.PressAt("id:inner", core.Point{X: 50, Y: 16})
	check("id:inner", "id:direct")
}
