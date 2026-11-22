package capture_test

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestImageSampling checks the Canvas and Image presentation paths, for encoded, pixel-buffer and offscreen sources, with original black and white texels as the nearest-neighbor reference.
func TestImageSampling(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios_simulator", "android", "linux", "web":
	default:
		t.Skip("checks the composited capture on macOS, iOS simulator, Android, Linux and web")
	}
	src := canvastest.Checkerboard(16, 16)
	frame := canvastest.Checkerboard(320, 180)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatal(err)
	}
	// Android draws no offscreen images yet.
	offscreen := nsgotest.Target() != "android"
	var ratio float32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		pt := func(px float32) core.Length { return core.Pt(core.Points(px / ratio)) }
		// The offscreen forms sit beside the canvas so the whole stays within a small display's height: AppKit lets a window run past the screen's bottom, and the capture shows nothing there.
		top := []core.Node{
			core.Canvas{TestID: "canvas", Style: core.Style{Width: pt(256), Height: pt(256)}, Draw: func(d *core.Drawing, size core.Size) {
				canvastest.DrawSampling(d, core.ImageSource{Data: encoded.Bytes()}, float32(size.Width)/2)
			}},
		}
		if offscreen {
			// One backing pixel per texel, filled rather than scaled, so the offscreen surface itself is exact.
			checker := cx.NewOffscreenImage(core.Size{Width: 16, Height: 16}, 1, func(d *core.Drawing) {
				d.SetFillColor(core.RGB(0xffffff)).FillRect(0, 0, 16, 16)
				d.SetFillColor(core.RGB(0x000000))
				for y := range 16 {
					for x := range 16 {
						if (x+y)%2 == 0 {
							d.FillRect(float32(x), float32(y), 1, 1)
						}
					}
				}
			})
			top = append(top,
				core.Canvas{TestID: "offscreen-canvas", Style: core.Style{Width: pt(128), Height: pt(128)}, Draw: func(d *core.Drawing, size core.Size) {
					d.SetImageSmoothing(false).DrawImageIn(checker.Source(), 0, 0, float32(size.Width), float32(size.Height))
				}},
				core.Image{TestID: "offscreen-image", Style: core.Style{Width: pt(128), Height: pt(128)},
					SourceFunc: checker.Source, ResizeMode: core.Stretch, Sampling: core.ImageSamplingNearest},
			)
		}
		children := []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart}, Children: top},
			core.Image{TestID: "image", Style: core.Style{Width: pt(640), Height: pt(360)},
				Source: core.ImageSource{Pixels: frame}, ResizeMode: core.Stretch, Sampling: core.ImageSamplingNearest},
		}
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(core.Points(16 / ratio)), Background: core.RGB(0xff0000)}, Children: children}
	}, nsgotest.Size(700, 680))
	origin := func(id string) image.Point {
		b := app.Find("id:" + id).Bounds()
		return image.Pt(int(math.Round(float64(float32(b.Origin.X)*ratio))), int(math.Round(float64(float32(b.Origin.Y)*ratio))))
	}
	canvas, picture := origin("canvas"), origin("image")
	var offscreenOrigins []image.Point
	if offscreen {
		offscreenOrigins = []image.Point{origin("offscreen-canvas"), origin("offscreen-image")}
	}
	var img image.Image
	// The PNG canvas image loads off-thread. Wait until both views have
	// drawn at least one black texel before comparing the whole capture.
	deadline := time.Now().Add(5 * time.Second)
	for {
		img = app.Capture()
		origins := []image.Point{canvas.Add(image.Pt(129, 1)), picture.Add(image.Pt(1, 1))}
		for _, o := range offscreenOrigins {
			origins = append(origins, o.Add(image.Pt(1, 1)))
		}
		ready := true
		for _, p := range origins {
			r, g, b, a := img.At(p.X, p.Y).RGBA()
			ready = ready && r == 0 && g == 0 && b == 0 && a == 65535
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			for _, p := range origins {
				t.Errorf("at %v: %v", p, img.At(p.X, p.Y))
			}
			t.Fatalf("images not drawn in %v", img.Bounds())
		}
		time.Sleep(20 * time.Millisecond)
		app.Settle()
	}
	var tolerance uint8
	if nsgotest.Target() == "macos" {
		// WindowServer's capture converts the display's profile to sRGB.
		// The offscreen Core Graphics sample checks every texel exactly.
		tolerance = 1
	}
	for _, y := range []int{0, 128} {
		if first, differs := canvastest.SamplingDifference(img, canvas.Add(image.Pt(128, y)), src, 8, tolerance); differs {
			t.Errorf("nearest canvas draw at y=%d differs at capture pixel %v: %v", y, first, img.At(first.X, first.Y))
		}
		blended := false
		for x := 4; x < 12; x++ {
			r, _, _, _ := img.At(canvas.X+x, canvas.Y+y+4).RGBA()
			blended = blended || (r > 0 && r < 65535)
		}
		if !blended {
			t.Errorf("smooth canvas draw at y=%d has no blended texel edge", y)
		}
	}
	if first, differs := canvastest.SamplingDifference(img, picture, frame, 2, tolerance); differs {
		t.Errorf("nearest Image differs at capture pixel %v: %v", first, img.At(first.X, first.Y))
	}
	for i, what := range []string{"canvas", "Image"} {
		if !offscreen {
			break
		}
		if first, differs := canvastest.SamplingDifference(img, offscreenOrigins[i], src, 8, tolerance); differs {
			t.Errorf("nearest offscreen %s differs at capture pixel %v: %v", what, first, img.At(first.X, first.Y))
		}
	}
}
