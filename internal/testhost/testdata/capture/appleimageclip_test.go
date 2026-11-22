package capture_test

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestAppleImageClipKeepsOuterDecoration(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator":
	default:
		t.Skip("checks Apple's image-layer clipping")
	}
	pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{255, 255, 0, 255}), image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	type sample struct {
		kind, corners string
		narrow        bool
	}
	var selected *core.Signal[sample]
	var ratio float32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		selected = core.NewSignal(cx, sample{kind: "pixels", corners: "round"})
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.Image{TestID: "image", SourceFunc: func() core.ImageSource {
				if selected.Get().kind == "encoded" {
					return core.ImageSource{Data: encoded.Bytes()}
				}
				return core.ImageSource{Pixels: pixels}
			}, ResizeMode: core.Stretch, StyleFunc: func() core.Style {
				current := selected.Get()
				style := core.Style{Position: core.Absolute, Left: core.Pt(80), Top: core.Pt(80), Width: core.Pt(120), Height: core.Pt(80), Overflow: core.Hidden, Background: core.RGB(0x0000ff), CornerRadius: 24,
					Shadows: []core.BoxShadow{{Spread: 12, Color: core.RGB(0xff0000)}}, OutlineWidth: 4, OutlineOffset: 2, OutlineColor: core.RGB(0x00ff00)}
				if current.corners == "shaped" || current.corners == "shaped-continuous" {
					style.CornerRadii = core.Corners{TopLeft: 24, TopRight: 12, BottomRight: 32, BottomLeft: 8}
				}
				if current.corners == "continuous" || current.corners == "shaped-continuous" {
					style.BorderCurve = core.BorderCurveContinuous
				}
				if current.narrow {
					style.Width, style.Height = core.Pt(70), core.Pt(50)
				}
				if current.kind == "tinted" {
					style.TintColor = core.RGB(0xffff00)
				}
				return style
			}},
		}}
	}, nsgotest.Size(300, 260))
	near := func(got, want uint32) bool { return got+3 >= want && got <= want+3 }
	for _, kind := range []string{"pixels", "encoded", "tinted"} {
		for _, corners := range []string{"round", "shaped", "continuous", "shaped-continuous", "resized"} {
			t.Run(kind+"/"+corners, func(t *testing.T) {
				current := sample{kind: kind, corners: corners}
				if corners == "resized" {
					current.corners = "shaped"
					current.narrow = true
				}
				app.Post(func() { selected.Set(current) })
				app.Settle()
				origin := app.Find("id:image").Bounds().Origin
				at := func(img image.Image, x, y core.Points) (uint32, uint32, uint32) {
					r, g, b, _ := img.At(img.Bounds().Min.X+int(float32(origin.X+x)*ratio), img.Bounds().Min.Y+int(float32(origin.Y+y)*ratio)).RGBA()
					return r >> 8, g >> 8, b >> 8
				}
				img := app.Capture()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					r, g, b := at(img, 35, 25)
					if r > 200 && g > 200 && b < 40 {
						break
					}
					time.Sleep(20 * time.Millisecond)
					img = app.Capture()
				}
				check := func(label string, x, y core.Points, want uint32) {
					t.Helper()
					r, g, b := at(img, x, y)
					if !near(r, want>>16) || !near(g, want>>8&255) || !near(b, want&255) {
						t.Errorf("%s/%s %s: #%02x%02x%02x, want #%06x", kind, corners, label, r, g, b, want)
					}
				}
				check("shadow", -9, 25, 0xff0000)
				check("outline", -4, 25, 0x00ff00)
				check("clipped corner", 1, 1, 0xff0000)
				check("image", 35, 25, 0xffff00)
				width := core.Points(120)
				if current.narrow {
					width = 70
				}
				check("overflow", width+9, 25, 0xff0000)
				check("past shadow", width+17, 25, 0xffffff)
				if current.corners == "shaped" {
					height := core.Points(80)
					if current.narrow {
						height = 50
					}
					check("smaller top-right corner", width-8, 3, 0xffff00)
					check("larger bottom-right corner outline", width-7, height-7, 0x00ff00)
					check("smaller bottom-left corner", 4, height-4, 0xffff00)
				}
			})
		}
	}
}
