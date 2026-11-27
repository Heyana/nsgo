//go:build darwin

package apple

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasLayerDrawsWhatAFullReplayDoes plays the split samples whole
// and over a layer of their static part, and compares the pixels.
func TestCanvasLayerDrawsWhatAFullReplayDoes(t *testing.T) {
	for _, s := range canvastest.Split(TextEngine{}) {
		t.Run(s.Name, func(t *testing.T) {
			d := s.Drawing
			split := driver.SplitCanvas(d)
			if !split.Static(d) {
				t.Fatalf("split %+v keeps nothing", split)
			}
			w, h := float64(s.Width), float64(s.Height)
			draw := func(over bool) *image.RGBA {
				ctx, img := bitmap(s.Width, s.Height)
				defer Release(ctx)
				DrawTopDown(ctx, h, true, func(baseUp bool) {
					host := CanvasHost{Ratio: canvastest.Scale, BaseUp: baseUp}
					if !over {
						Play(ctx, d, d.Ops, host)
						return
					}
					l := NewCanvasLayer(ctx, d, d.Ops, split, w, h, canvastest.Scale, host)
					if l == nil {
						t.Fatal("no layer")
					}
					defer l.Release()
					l.Draw(ctx, w, h)
					PlayOver(ctx, d, d.Ops, split, host)
				})
				return img
			}
			whole, over := draw(false), draw(true)
			if dir := os.Getenv("NSGO_CANVAS_PNG"); dir != "" {
				for name, img := range map[string]*image.RGBA{"whole": whole, "over": over} {
					if f, err := os.Create(dir + "/apple-split-" + s.Name + "-" + name + ".png"); err == nil {
						png.Encode(f, img)
						f.Close()
					}
				}
			}
			if n, at := canvastest.Differ(whole, over); n > 0 {
				t.Errorf("%d pixels differ from a full replay, the first at %v", n, at)
			}
		})
	}
}

// TestStretchCanvasUpscalesAHalfResolution draws canvastest.Resolution
// into a content layer at half canvastest.Scale, as a canvas view at a
// ResolutionScale of 0.5 does, and checks its backing store, its filter
// and the stretched pixels. renderInContext: stretches with the context's
// interpolation rather than the layer's filter, which the window server
// takes, so the context's follows the filter.
func TestStretchCanvasUpscalesAHalfResolution(t *testing.T) {
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	var width, height func(ctx uintptr) uintptr
	purego.RegisterLibFunc(&width, cg, "CGBitmapContextGetWidth")
	purego.RegisterLibFunc(&height, cg, "CGBitmapContextGetHeight")
	for _, sampling := range []driver.ImageSampling{driver.ImageSamplingSmooth, driver.ImageSamplingNearest} {
		t.Run(fmt.Sprint(sampling), func(t *testing.T) {
			const side = canvastest.ResolutionSize
			d := canvastest.Resolution(TextEngine{}, sampling)
			scale := canvastest.Scale * d.ResolutionScale()
			ctx, img := bitmap(side, side)
			defer Release(ctx)
			if layerDrawer == 0 {
				play(t, "resolution", driver.NewDrawing(TextEngine{}), 1, 1, false)
			}
			layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
			defer layer.Send(selRelease)
			layer.Send(sel("setBounds:"), Rect(0, 0, side, side))
			layer.Send(sel("setContentsScale:"), scale)
			StretchCanvas(layer, d)
			layer.Send(sel("setDelegate:"), layerDrawer)
			var pw, ph uintptr
			layerDraw = func(lctx uintptr) {
				pw, ph = width(lctx), height(lctx)
				DrawTopDown(lctx, side, !MsgBool(layer, sel("contentsAreFlipped")), func(baseUp bool) {
					Play(lctx, d, d.Ops, CanvasHost{Ratio: scale, BaseUp: baseUp})
				})
			}
			layer.Send(sel("display"))
			filter, want := GoString(layer.Send(sel("magnificationFilter"))), "linear"
			const none, high = 1, 3
			quality := int32(high)
			if sampling == driver.ImageSamplingNearest {
				want, quality = "nearest", none
			}
			if filter != want {
				t.Errorf("magnification filter %q, want %q", filter, want)
			}
			cgContextSetInterpolationQuality(ctx, quality)
			layer.Send(sel("renderInContext:"), ctx)
			if pw != canvastest.ResolutionPixels || ph != canvastest.ResolutionPixels {
				t.Errorf("backing store is %d × %d pixels, want %d a side", pw, ph, canvastest.ResolutionPixels)
			}
			for _, f := range canvastest.CheckResolution(img, image.Point{}, sampling) {
				t.Error(f)
			}
		})
	}
}
