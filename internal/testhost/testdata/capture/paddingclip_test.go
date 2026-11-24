package capture_test

import (
	"image"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestClipStaysInsideTheBorder checks that Overflow: Hidden clips children to the padding box, as CSS does: a red child that covers the whole view shows inside the border and under none of it, so the half-transparent green border shows over the view's blue background, the padding box's corners included, each the outer radius less the widths of the sides it joins.
func TestClipStaysInsideTheBorder(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator", "watchos_simulator", "windows", "linux", "android", "web":
	default:
		t.Skip("checks a platform's drawing")
	}
	var ratio float32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.View{TestID: "clipped", Style: core.Style{Position: core.Absolute, Left: core.Pt(10), Top: core.Pt(10),
				Width: core.Pt(120), Height: core.Pt(80), Overflow: core.Hidden, CornerRadius: 24, Background: core.RGB(0x0000ff),
				Borders: core.Edges{Top: 4, Right: 8, Bottom: 12, Left: 16}, BorderColor: core.RGBA(0x00ff00, 0.5)},
				Children: []core.Node{core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(-40), Top: core.Pt(-40),
					Width: core.Pt(200), Height: core.Pt(160), Background: core.RGB(0xff0000)}}}},
		}}
	})
	frame := app.Find("id:clipped").Bounds()
	img := app.Capture()
	at := func(img image.Image, x, y core.Points) uint32 {
		px := img.Bounds().Min.X + int(float32(frame.Origin.X+x)*ratio)
		py := img.Bounds().Min.Y + int(float32(frame.Origin.Y+y)*ratio)
		r, g, b, _ := img.At(px, py).RGBA()
		return r>>8<<16 | g>>8<<8 | b>>8
	}
	// The border's half green blends with the blue under it as the platform composites, in its own color space.
	border := func(c uint32) bool { return c>>16 < 0x40 && c>>8&0xff > 0x60 && c&0xff > 0x60 }
	exactly := func(want uint32) func(uint32) bool {
		return func(c uint32) bool {
			for shift := 0; shift < 24; shift += 8 {
				if a, b := int(c>>shift&0xff), int(want>>shift&0xff); a-b > 8 || b-a > 8 {
					return false
				}
			}
			return true
		}
	}
	for _, c := range []struct {
		label string
		x, y  core.Points
		want  func(uint32) bool
	}{
		{"left border", 8, 40, border},
		{"top border", 60, 2, border},
		{"right border", 116, 40, border},
		{"bottom border", 60, 74, border},
		{"inside", 60, 40, exactly(0xff0000)},
		// Inside the padding box's square corner but outside its rounded one, whose radii are 8 across and 20 down.
		{"padding box's corner", 18, 6, border},
		{"outside", 130, 40, exactly(0xffffff)},
	} {
		if got := at(img, c.x, c.y); !c.want(got) {
			t.Errorf("%s at (%g,%g): #%06x", c.label, c.x, c.y, got)
		}
	}
}
