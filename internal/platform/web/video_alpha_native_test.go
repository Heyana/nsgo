//go:build js && wasm && nsgotest

package web

import (
	"bytes"
	"image/color"
	"image/png"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	alphaclips "github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestVideoShowsWhatIsBehindItsTransparentPixels plays VP9 with alpha in WebM over a red view in the real browser and checks a composited screenshot: the red shows through around the blue center square.
func TestVideoShowsWhatIsBehindItsTransparentPixels(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("needs a browser: run with nsgo test -target web")
	}
	if doc.Call("createElement", "video").Call("canPlayType", `video/webm; codecs="vp9"`).String() == "" {
		t.Skip("browser does not play VP9 WebM")
	}
	raw := js.Global().Get("Uint8Array").New(len(alphaclips.VP9Alpha))
	js.CopyBytesToJS(raw, alphaclips.VP9Alpha)
	blob := js.Global().Get("Blob").New([]any{raw}, map[string]any{"type": "video/webm"})
	url := js.Global().Get("URL").Call("createObjectURL", blob).String()
	defer js.Global().Get("URL").Call("revokeObjectURL", url)

	for _, controls := range []bool{false, true} {
		name := "plain"
		if controls {
			name = "controls"
		}
		t.Run(name, func(t *testing.T) {
			loaded := make(chan struct{}, 1)
			failed := make(chan error, 1)
			const viewSize = 120
			native, err := testhost.Current().Open(func(*core.Scope) core.Node {
				return core.View{
					Style: core.Style{Width: core.Pt(viewSize), Height: core.Pt(viewSize), Background: core.RGB(0xff0000)},
					Children: []core.Node{
						core.Video{
							Source:   url,
							Muted:    true,
							Repeat:   true,
							Controls: controls,
							Style:    core.Style{Width: core.Pt(viewSize), Height: core.Pt(viewSize)},
							OnLoad: func(core.VideoInfo) {
								select {
								case loaded <- struct{}{}:
								default:
								}
							},
							OnError: func(err error) {
								select {
								case failed <- err:
								default:
								}
							},
						},
					},
				}
			}, testhost.Options{Size: core.Size{Width: 200, Height: 200}})
			if err != nil {
				t.Fatal(err)
			}
			host := native.(*testSurface)
			defer host.s.engine.Close()
			host.Sync()

			select {
			case <-loaded:
			case err := <-failed:
				t.Fatal(err)
			case <-time.After(10 * time.Second):
				t.Fatal("VP9 alpha video did not load")
			}

			red := func(c color.RGBA) bool { return c.R > 200 && c.G < 60 && c.B < 60 }
			blue := func(c color.RGBA) bool { return c.B > 200 && c.R < 60 && c.G < 60 }
			secondY := 110.0
			if controls {
				// Browser controls sit along the bottom; sample the top corners.
				secondY = 10.0
			}
			var c1, c2, mid color.RGBA
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
				shot, err := host.Screenshot()
				if err == nil {
					if img, err := png.Decode(bytes.NewReader(shot)); err == nil {
						b := img.Bounds()
						sx, sy := float64(b.Dx())/200.0, float64(b.Dy())/200.0
						at := func(x, y float64) color.RGBA {
							return color.RGBAModel.Convert(img.At(b.Min.X+int(x*sx), b.Min.Y+int(y*sy))).(color.RGBA)
						}
						c1, c2, mid = at(10, 10), at(110, secondY), at(60, 60)
						if red(c1) && red(c2) && blue(mid) {
							return
						}
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("want red around a blue square, got corners %v, %v and center %v (RGBA)", c1, c2, mid)
				}
			}
		})
	}
}
