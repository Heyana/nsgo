package capture_test

import (
	"image"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A text or canvas needs a clip around its content layer, including text ink
// outside its layout box, while the styled layer retains its outer decoration.
func TestAppleContentClipKeepsOuterDecoration(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator", "tvos_simulator":
	default:
		t.Skip("checks Apple content layers")
	}
	type sample struct {
		kind, corners  string
		visible, plain bool
	}
	program, err := core.CompileWGSL("fn shade(uv: vec2f) -> vec4f { return vec4f(1.0, 1.0, 0.0, 1.0); }")
	if err != nil {
		t.Fatal(err)
	}
	var selected *core.Signal[sample]
	var ratio float32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		selected = core.NewSignal(cx, sample{kind: "text", corners: "round", plain: true})
		var children []core.Node
		for _, kind := range []string{"view", "canvas", "text", "shader"} {
			style := func() core.Style {
				current := selected.Get()
				style := core.Style{Position: core.Absolute, Left: core.Pt(80), Top: core.Pt(80),
					Width: core.Pt(120), Height: core.Pt(80), Overflow: core.Hidden,
					Background: core.RGB(0x0000ff), CornerRadius: 24,
					Shadows:      []core.BoxShadow{{Spread: 12, Color: core.RGB(0xff0000)}},
					OutlineWidth: 4, OutlineOffset: 2, OutlineColor: core.RGB(0x00ff00)}
				if current.kind != kind {
					style.Display = core.DisplayNone
				}
				if current.plain {
					style.Shadows = nil
					style.OutlineWidth = 0
				}
				if current.visible {
					style.Overflow = core.Visible
				}
				switch current.corners {
				case "shaped":
					style.CornerRadii = core.Corners{TopLeft: 24, TopRight: 12, BottomRight: 32, BottomLeft: 8}
				case "continuous":
					style.BorderCurve = core.BorderCurveContinuous
				}
				if kind == "text" {
					style.Color, style.FontSize = core.RGB(0xffff00), 40
					style.WhiteSpace = core.WhiteSpaceNoWrap
					style.TextShadowColor = core.RGB(0xffff00)
					style.TextShadowOffset = core.Point{Y: 100}
				}
				return style
			}
			switch kind {
			case "shader":
				children = append(children, core.Shader{TestID: "clip-shader", StyleFunc: style, Program: program})
			case "text":
				children = append(children, core.Text{TestID: "clip-text", StyleFunc: style, Content: "MMMM"})
			case "canvas":
				children = append(children, core.Canvas{TestID: "clip-canvas", StyleFunc: style, Draw: func(d *core.Drawing, _ core.Size) {
					d.SetFillColor(core.RGB(0xffff00)).FillRect(-40, -40, 200, 160)
				}})
			default:
				children = append(children, core.View{TestID: "clip-view", StyleFunc: style, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(180), Height: core.Pt(140), Background: core.RGB(0xffff00)}},
				}})
			}
		}
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, Children: children}
	}, nsgotest.Size(300, 260))
	var capture image.Image
	var origin core.Point
	at := func(x, y core.Points) (uint32, uint32, uint32) {
		px := capture.Bounds().Min.X + int(float32(origin.X+x)*ratio)
		py := capture.Bounds().Min.Y + int(float32(origin.Y+y)*ratio)
		r, g, b, _ := capture.At(px, py).RGBA()
		return r >> 8, g >> 8, b >> 8
	}
	near := func(got, want uint32) bool { return got+3 >= want && got <= want+3 }
	for _, kind := range []string{"view", "canvas", "text", "shader"} {
		for _, corners := range []string{"round", "shaped", "continuous"} {
			t.Run(kind+"/"+corners, func(t *testing.T) {
				app.Post(func() { selected.Set(sample{kind: kind, corners: corners}) })
				app.Settle()
				origin = app.Find("id:clip-" + kind).Bounds().Origin
				capture = app.Capture()
				if kind == "shader" {
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						r, g, b := at(60, 40)
						if r > 200 && g > 200 && b < 40 {
							break
						}
						time.Sleep(20 * time.Millisecond)
						capture = app.Capture()
					}
				}
				check := func(label string, x, y core.Points, want uint32) {
					t.Helper()
					r, g, b := at(x, y)
					if !near(r, want>>16) || !near(g, want>>8&255) || !near(b, want&255) {
						t.Errorf("%s at (%g,%g): #%02x%02x%02x, want #%06x", label, x, y, r, g, b, want)
					}
				}
				check("outer shadow", -9, 40, 0xff0000)
				check("outline", -4, 40, 0x00ff00)
				check("clipped overflow", 129, 40, 0xff0000)
				check("past shadow", 137, 40, 0xffffff)
				check("rounded content", 1, 1, 0xff0000)
				if kind != "text" {
					check("content", 60, 40, 0xffff00)
					if corners == "shaped" {
						check("smaller corner", 116, 5, 0xffff00)
					}
				}
			})
		}
	}
	// The content layer's frame includes the text's ink outside its box. A
	// holder must neither crop that frame permanently nor leave it unclipped.
	for _, visible := range []bool{true, false, true} {
		app.Post(func() { selected.Set(sample{kind: "text", corners: "shaped", visible: visible}) })
		app.Settle()
		origin = app.Find("id:clip-text").Bounds().Origin
		capture = app.Capture()
		ink := false
		for y := core.Points(100); y < 150 && !ink; y++ {
			for x := core.Points(0); x < 120; x++ {
				r, g, b := at(x, y)
				if r > 200 && g > 200 && b < 40 {
					ink = true
					break
				}
			}
		}
		if ink != visible {
			t.Errorf("text ink below its box: visible=%v, found=%v", visible, ink)
		}
	}
}
