package capture_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestCanvasImageTint draws the shared tint sample from an encoded image, a pixel buffer and an offscreen image.
func TestCanvasImageTint(t *testing.T) {
	if nsgotest.Target() == "" {
		t.Skip("run with nsgo test -target")
	}
	encoded, err := canvastest.TintSource()
	if err != nil {
		t.Fatal(err)
	}
	pixels := canvastest.TintPixelsSource()
	sources := []struct {
		name   string
		source func(cx *core.Scope) func() core.ImageSource
	}{
		{"encoded", func(*core.Scope) func() core.ImageSource { return func() core.ImageSource { return encoded } }},
		{"pixels", func(*core.Scope) func() core.ImageSource { return func() core.ImageSource { return pixels } }},
		{"offscreen", func(cx *core.Scope) func() core.ImageSource {
			off := cx.NewOffscreenImage(canvastest.TintSpriteSize, 1, canvastest.DrawTintSprite)
			return off.Source
		}},
	}
	for _, s := range sources {
		t.Run(s.name, func(t *testing.T) {
			if s.name == "offscreen" && nsgotest.Target() == "android" {
				t.Skip("Android's canvases draw no offscreen images yet")
			}
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				scope = cx
				src := s.source(cx)
				return core.Canvas{
					TestID: "tint",
					Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100)},
					Draw: func(d *core.Drawing, size core.Size) {
						canvastest.DrawTintSample(d, src(), float32(size.Width), float32(size.Height))
					},
				}
			}, nsgotest.Size(canvastest.TintWidth, canvastest.TintHeight))

			var scale float32
			app.Post(func() { scale = scope.Env().PixelRatio() })
			app.Settle()
			origin := app.Find("id:tint").Bounds().Origin
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
				failed := canvastest.CheckTintSample(app.Capture(), origin, scale)
				if len(failed) == 0 {
					return
				}
				if time.Now().After(deadline) {
					for _, f := range failed {
						t.Error(f)
					}
					return
				}
			}
		})
	}
}
