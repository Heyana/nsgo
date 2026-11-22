package press_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPressAtFindsZoomedScrollContent(t *testing.T) {
	switch nsgotest.Target() {
	case "", "macos", "ios", "ios_simulator":
	default:
		t.Skip("checks native zoom on Apple and the headless model")
	}
	for _, c := range []struct {
		name       string
		horizontal bool
		scale      float32
		offset, at core.Point
	}{
		{"vertical doubled", false, 2, core.Point{Y: 320}, core.Point{X: 60, Y: 40}},
		{"vertical halved", false, 0.5, core.Point{Y: 40}, core.Point{X: 30, Y: 60}},
		{"horizontal doubled", true, 2, core.Point{X: 480}, core.Point{X: 60, Y: 40}},
		{"horizontal halved", true, 0.5, core.Point{X: 60}, core.Point{X: 90, Y: 20}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var scroll core.Ref
			pressed := -1
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				var bands []core.Node
				for index := range 3 {
					bands = append(bands, core.View{TestID: []string{"first", "middle", "last"}[index], Style: core.Style{Width: core.Pt(120), Height: core.Pt(80), FlexShrink: 0, Background: core.RGB([]uint32{0xffff00, 0x00ffff, 0xff00ff}[index])}, OnPress: func(core.PressEvent) { pressed = index }})
				}
				direction := core.Column
				if c.horizontal {
					direction = core.Row
				}
				return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{core.Scroll{TestID: "zoom", Ref: &scroll, Horizontal: c.horizontal, MinZoomScale: 0.5, MaxZoomScale: 2, Style: core.Style{Position: core.Absolute, Left: core.Pt(40), Top: core.Pt(40), Width: core.Pt(120), Height: core.Pt(80), Direction: direction}, Children: bands}}}
			}, nsgotest.Size(300, 260))
			app.Post(func() { scroll.ZoomTo(c.scale, nil, false); scroll.ScrollTo(c.offset, false) })
			app.Settle()
			view := app.Find("id:zoom")
			if math.Abs(float64(view.Offset.X-c.offset.X)) > 1 || math.Abs(float64(view.Offset.Y-c.offset.Y)) > 1 {
				t.Errorf("zoomed offset: %v, want %v", view.Offset, c.offset)
			}
			app.PressAt("id:zoom", c.at)
			if pressed != 2 {
				t.Errorf("point press reached band%d, want last band", pressed)
			}
			pressed = -1
			app.Press("id:last")
			if pressed != 2 {
				t.Errorf("revealed press reached band%d, want last band", pressed)
			}
		})
	}
}
