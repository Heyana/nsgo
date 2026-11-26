//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestCanvasResolutionScale draws a canvas's content layer at its
// drawing's ResolutionScale times the window's density, stretched with
// its Upscale, and follows a drawing that changes them.
func TestCanvasResolutionScale(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{native: objc.ID(classView).Send(selNew), animator: driver.NewAnimator(nil)}
		defer s.native.Send(selRelease)
		s.env.PixelRatio = 2
		s.EnableCanvas()
		v := s.create(driver.KindCanvas, 1)
		defer v.native.Send(selRelease)
		defer delete(views, v.native)
		v.frame.Size = driver.Size{Width: 8, Height: 8}
		s.views, s.root = []*view{nil, v}, 1
		ops := canvases.Load()
		d := driver.NewDrawing(nil).FillRect(0, 0, 8, 8)
		d.Resolution, d.Upscale = 0.5, driver.ImageSamplingNearest
		ops.apply(s, v, d)
		layer := v.drawLayer()
		check := func(scale float64, filter string) {
			t.Helper()
			if got := objc.Send[float64](layer, sel("contentsScale")); got != scale {
				t.Errorf("contents scale %v, want %v", got, scale)
			}
			if got := apple.GoString(layer.Send(sel("magnificationFilter"))); got != filter {
				t.Errorf("magnification filter %q, want %q", got, filter)
			}
		}
		check(1, "nearest")
		again := *d
		again.Resolution, again.Upscale = 0.25, driver.ImageSamplingSmooth
		ops.apply(s, v, &again)
		check(0.5, "linear")
		again2 := again
		again2.Resolution = 0
		ops.apply(s, v, &again2)
		check(2, "linear")
	})
}
