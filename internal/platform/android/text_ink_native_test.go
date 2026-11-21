//go:build android && nsgotest

package android

import (
	"bytes"
	"image/color"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestTextInkDrawsPastItsBoxOnDevice renders texts with decoration lines
// and shadows extending past their layout boxes and checks screenshot
// pixels on Android.
func TestTextInkDrawsPastItsBoxOnDevice(t *testing.T) {
	window, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Background: core.RGB(0xffffff)},
			Children: []core.Node{
				core.Text{
					Content: "MMMM",
					Style: core.Style{
						Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(20),
						Width: core.Pt(100), Height: core.Pt(20),
						FontSize: 16, Color: core.RGB(0x000000),
						TextDecoration:          core.TextDecorationUnderline,
						TextDecorationColor:     core.RGB(0xff0000),
						TextDecorationThickness: core.Pt(4),
						TextUnderlineOffset:     core.Pt(16),
					},
				},
				core.Text{
					Content: "MMMM",
					Style: core.Style{
						Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(80),
						Width: core.Pt(100), Height: core.Pt(20),
						FontSize: 16, Color: core.RGB(0x000000),
						TextShadowColor:  core.RGB(0x00aa00),
						TextShadowOffset: core.Point{X: 0, Y: 18},
						TextShadowRadius: 2,
					},
				},
				core.View{
					Style: core.Style{
						Position: core.Absolute, Left: core.Pt(160), Top: core.Pt(20),
						Width: core.Pt(100), Height: core.Pt(20),
						Overflow: core.Hidden,
					},
					Children: []core.Node{
						core.Text{
							Content: "MMMM",
							Style: core.Style{
								Width: core.Pt(100), Height: core.Pt(20),
								FontSize: 16, Color: core.RGB(0x000000),
								TextDecoration:          core.TextDecorationUnderline,
								TextDecorationColor:     core.RGB(0xff0000),
								TextDecorationThickness: core.Pt(4),
								TextUnderlineOffset:     core.Pt(16),
								TextShadowColor:         core.RGB(0x00aa00),
								TextShadowOffset:        core.Point{X: 0, Y: 18},
								TextShadowRadius:        2,
							},
						},
					},
				},
			},
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := window.(*testSurface)
	s.Sync()
	ratio := float64(math.Float32frombits(s.s.ratio.Load()))
	px := func(p float64) int { return int(math.Round(p * ratio)) }

	isRed := func(c color.RGBA) bool { return c.R > 0xb0 && c.G < 0x60 && c.B < 0x60 }
	isGreen := func(c color.RGBA) bool { return c.G > 0x70 && c.R < 0x70 && c.B < 0x70 }

	deadline := time.Now().Add(4 * time.Second)
	for {
		data, err := s.Screenshot()
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		has := func(x0, y0, x1, y1 int, match func(color.RGBA) bool) bool {
			for y := max(y0, b.Min.Y); y < min(y1, b.Max.Y); y++ {
				for x := max(x0, b.Min.X); x < min(x1, b.Max.X); x++ {
					c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
					if match(c) {
						return true
					}
				}
			}
			return false
		}
		red := has(px(20), px(42), px(120), px(65), isRed)
		green := has(px(20), px(102), px(120), px(135), isGreen)
		clipped := has(px(160), px(42), px(260), px(65), func(c color.RGBA) bool { return isRed(c) || isGreen(c) })
		if red && green && !clipped {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("text ink overflow check failed: red=%v green=%v escapedClip=%v", red, green, clipped)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
