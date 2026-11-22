package shaderpixels_test

import (
	"image"
	"image/color"
	"image/draw"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

type colors struct{ left, right color.RGBA }

// TestShaderPixelFrames reads the composited output of image0 as one 960×540
// buffer is rewritten. Individual versions wait for a capture before the next
// update. The same shader then runs for 120 display callbacks without waiting
// for captures.
func TestShaderPixelFrames(t *testing.T) {
	target := nsgotest.Target()
	if target == "" {
		t.Skip("checks the native shader renderer")
	}
	program, err := core.CompileShader(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos * image0Size() / rectSize())
}
`)
	if err != nil {
		t.Fatal(err)
	}
	red := color.RGBA{R: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	palette := []colors{{red, green}, {green, blue}, {blue, red}, {green, red}, {red, blue}, {blue, green}}
	rect := image.Rect(0, 0, 960, 540)
	frames := make([]*image.RGBA, len(palette))
	for i, c := range palette {
		p := image.NewRGBA(rect)
		draw.Draw(p, image.Rect(0, 0, 480, 540), image.NewUniform(c.left), image.Point{}, draw.Src)
		draw.Draw(p, image.Rect(480, 0, 960, 540), image.NewUniform(c.right), image.Point{}, draw.Src)
		frames[i] = p
	}
	var submitted atomic.Uint64
	requests := make(chan func(int), 1)
	const viewWidth, viewHeight, probeCenter = 240, 160, 16
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		pixels := image.NewRGBA(rect)
		copy(pixels.Pix, frames[0].Pix)
		version := core.NewSignal(cx, uint64(0))
		requests <- func(count int) {
			cx.Post(func() {
				var next func(time.Time)
				next = func(time.Time) {
					n := version.Get() + 1
					copy(pixels.Pix, frames[n%uint64(len(frames))].Pix)
					version.Set(n)
					submitted.Store(n)
					count--
					if count > 0 {
						cx.RequestAnimationFrame(next)
					}
				}
				cx.RequestAnimationFrame(next)
			})
		}
		children := []core.Node{core.Shader{
			Program: program, TestID: "texture", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			ImagesFunc: func() []core.ImageSource {
				return []core.ImageSource{{Pixels: pixels, Version: version.Get()}}
			},
		}}
		if target == "macos" {
			// The AppKit fallback omits both this filter and Metal layers.
			children = append(children, core.View{Style: core.Style{
				Position: core.Absolute, Left: core.Pt(8), Top: core.Pt(8),
				Width: core.Pt(16), Height: core.Pt(16),
				Background: core.RGB(0xff0000), Filters: []core.Filter{core.Invert(1)},
			}})
		}
		return core.View{TestID: "surface", Style: core.Style{
			Width: core.Pct(100), Height: core.Pct(100), Background: core.RGB(0xff00ff),
		}, Children: children}
	}, nsgotest.Size(viewWidth, viewHeight))
	request := <-requests
	bounds := app.Find("id:texture").Bounds()
	if bounds.Size.Width <= 0 || bounds.Size.Height <= 0 {
		t.Fatalf("shader's frame is empty: %v, root %v", bounds, app.Find("id:surface").Bounds())
	}
	capture := func() colors {
		img := app.Capture()
		b := img.Bounds()
		if target == "macos" {
			r, g, b, _ := img.At(b.Min.X+b.Dx()*probeCenter/viewWidth, b.Min.Y+b.Dy()*probeCenter/viewHeight).RGBA()
			if r>>8 >= 200 && g>>8 < 80 && b>>8 < 80 {
				t.Skip("AppKit's software capture leaves the compositor's inverted marker red and omits Metal layers")
			}
			if r>>8 > 160 || g>>8 < 200 || b>>8 < 200 {
				t.Fatalf("the capture's compositor marker is %d,%d,%d, want cyan", r>>8, g>>8, b>>8)
			}
		}
		y := b.Min.Y + b.Dy()/2
		at := func(x int) color.RGBA {
			r, g, b, a := img.At(x, y).RGBA()
			// Some native captures report display-space primaries through
			// UIKit. Require one dominant channel and keep other colors
			// distinguishable from the test's six expected pairs.
			r, g, b, a = r>>8, g>>8, b>>8, a>>8
			if a >= 230 {
				switch {
				case r >= 200 && r >= max(g, b)+80:
					return red
				case g >= 200 && g >= max(r, b)+80:
					return green
				case b >= 200 && b >= max(r, g)+80:
					return blue
				}
			}
			return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
		}
		return colors{at(b.Min.X + b.Dx()/4), at(b.Min.X + 3*b.Dx()/4)}
	}
	waitForVersion := func(n uint64, allowed func(colors) bool) {
		t.Helper()
		want := palette[n%uint64(len(palette))]
		timeout := 4 * time.Second
		if n == 0 {
			// The first version also waits for the shader's GL context and surface, which the Android emulator's software renderer can take seconds to create.
			timeout = 20 * time.Second
		}
		deadline := time.Now().Add(timeout)
		for {
			got := capture()
			if got == want {
				return
			}
			if allowed != nil && !allowed(got) {
				t.Fatalf("invalid pixels while waiting for version %d: %v", n, got)
			}
			if time.Now().After(deadline) {
				app.Screenshot("shader-pixels")
				t.Fatalf("version %d drew %v, want %v", n, got, want)
			}
			time.Sleep(12 * time.Millisecond)
		}
	}

	waitForVersion(0, nil)
	for n := uint64(1); n < uint64(len(palette)); n++ {
		request(1)
		waitForVersion(n, func(got colors) bool { return got == palette[n-1] })
	}

	const stream = 120
	end := submitted.Load() + stream
	request(stream)
	deadline := time.Now().Add(15 * time.Second)
	for submitted.Load() < end {
		if got := capture(); !slices.Contains(palette, got) {
			t.Fatalf("invalid pixels during the stream: %v", got)
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d updates ran", submitted.Load()-(end-stream), stream)
		}
	}
	waitForVersion(end, func(got colors) bool { return slices.Contains(palette, got) })
}
