package capture_test

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestAppleScrollClipKeepsOuterDecoration(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator":
	default:
		t.Skip("checks Apple's moving scroll viewport")
	}
	type sample struct {
		kind, corners string
		plain         bool
	}
	var selected *core.Signal[sample]
	var ratio float32
	refs := map[string]*core.Ref{"vertical": new(core.Ref), "horizontal": new(core.Ref)}
	lastPress := -1
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		selected = core.NewSignal(cx, sample{kind: "vertical", corners: "round", plain: true})
		var children []core.Node
		for _, kind := range []string{"vertical", "horizontal"} {
			var bands []core.Node
			for index, c := range []uint32{0xffff00, 0x00ffff, 0xff00ff} {
				bands = append(bands, core.View{TestID: fmt.Sprintf("%s-%d", kind, index), Style: core.Style{Width: core.Pt(120), Height: core.Pt(80), FlexShrink: 0, Background: core.RGB(c)}, OnPress: func(core.PressEvent) { lastPress = index }})
			}
			direction := core.Column
			if kind == "horizontal" {
				direction = core.Row
			}
			children = append(children, core.Scroll{TestID: kind, Ref: refs[kind], Horizontal: kind == "horizontal", HideIndicator: true, MaxZoomScale: 2, StyleFunc: func() core.Style {
				current := selected.Get()
				style := core.Style{Position: core.Absolute, Left: core.Pt(80), Top: core.Pt(80), Width: core.Pt(120), Height: core.Pt(80), Overflow: core.Hidden, Direction: direction, CornerRadius: 24, Background: core.RGB(0x0000ff), Shadows: []core.BoxShadow{{Spread: 12, Color: core.RGB(0xff0000)}}, OutlineWidth: 4, OutlineOffset: 2, OutlineColor: core.RGB(0x00ff00)}
				if current.kind != kind {
					style.Display = core.DisplayNone
				}
				if current.plain {
					style.Shadows = nil
					style.OutlineWidth = 0
				}
				if current.corners == "shaped" || current.corners == "shaped-continuous" {
					style.CornerRadii = core.Corners{TopLeft: 24, TopRight: 12, BottomRight: 32, BottomLeft: 8}
				}
				if current.corners == "continuous" || current.corners == "shaped-continuous" {
					style.BorderCurve = core.BorderCurveContinuous
				}
				return style
			}, Children: bands})
		}
		// A fall-through press must differ from every band's native handler.
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, OnPress: func(core.PressEvent) { lastPress = -2 }, Children: children}
	}, nsgotest.Size(300, 260))
	near := func(got, want uint32) bool { return got+3 >= want && got <= want+3 }
	for _, kind := range []string{"vertical", "horizontal"} {
		for _, corners := range []string{"round", "shaped", "continuous", "shaped-continuous", "zoom-round", "zoom-shaped"} {
			for _, band := range []int{0, 1, 2, 0, -1} {
				t.Run(fmt.Sprintf("%s/%s/%d", kind, corners, band), func(t *testing.T) {
					actual := strings.TrimPrefix(corners, "zoom-")
					zoom := core.Points(1)
					if actual != corners {
						zoom = 2
					}
					stride, center := core.Points(80), core.Points(40)
					if kind == "horizontal" {
						stride, center = 120, 60
					}
					app.Post(func() {
						selected.Set(sample{kind: kind, corners: actual})
						refs[kind].ZoomTo(float32(zoom), nil, false)
						distance := core.Points(max(band, 0)) * stride * zoom
						at := core.Point{Y: distance}
						if kind == "horizontal" {
							at = core.Point{X: distance}
						}
						refs[kind].ScrollTo(at, false)
					})
					app.Settle()
					shown := band
					if band < 0 {
						app.Scroll("id:"+kind, center)
						offset := app.Find("id:" + kind).Offset.Y
						if kind == "horizontal" {
							offset = app.Find("id:" + kind).Offset.X
						}
						if offset <= 0 {
							t.Errorf("native scrolling left the offset at %g", offset)
						}
						shown = min(2, max(0, int((offset+center)/(stride*zoom))))
					}
					origin := app.Find("id:" + kind).Bounds().Origin
					img := app.Capture()
					at := func(img image.Image, x, y core.Points) (uint32, uint32, uint32) {
						r, g, b, _ := img.At(img.Bounds().Min.X+int(float32(origin.X+x)*ratio), img.Bounds().Min.Y+int(float32(origin.Y+y)*ratio)).RGBA()
						return r >> 8, g >> 8, b >> 8
					}
					check := func(label string, x, y core.Points, want uint32) {
						t.Helper()
						r, g, b := at(img, x, y)
						if !near(r, want>>16) || !near(g, want>>8&255) || !near(b, want&255) {
							t.Errorf("%s/%s/%d %s: #%02x%02x%02x, want #%06x", kind, corners, band, label, r, g, b, want)
						}
					}
					check("shadow", -9, 40, 0xff0000)
					check("outline", -4, 40, 0x00ff00)
					check("corner", 1, 1, 0xff0000)
					check("overflow", 129, 40, 0xff0000)
					check("past shadow", 137, 40, 0xffffff)
					check("visible content", 60, 40, []uint32{0xffff00, 0x00ffff, 0xff00ff}[shown])
					if strings.HasPrefix(corners, "zoom-") {
						check("corner content at fixed radius", 10, 10, []uint32{0xffff00, 0x00ffff, 0xff00ff}[shown])
					}
					lastPress = -1
					app.PressAt("id:"+kind, core.Point{X: 60, Y: 40})
					if lastPress != shown {
						t.Errorf("press at scrolled content: band%d, want%d", lastPress, shown)
					}
				})
			}
		}
	}
}
