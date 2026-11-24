//go:build js && wasm

package web

import (
	goimage "image"
	"math"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// canvasPixels returns the pixels v's <canvas> holds.
func canvasPixels(v *view) *goimage.RGBA {
	w, h := v.inner.Get("width").Int(), v.inner.Get("height").Int()
	img := goimage.NewRGBA(goimage.Rect(0, 0, w, h))
	data := v.inner.Call("getContext", "2d").Call("getImageData", 0, 0, w, h).Get("data")
	js.CopyBytesToGo(img.Pix, js.Global().Get("Uint8Array").New(data.Get("buffer")))
	return img
}

// TestPartialCanvasRedrawsMatchFullOnes draws the split samples whole,
// moves their bound value and plays the drawing again clipped to what
// that changed, and compares that with drawing them whole at the new
// value; a pixel the change does not reach keeps what it held. Chrome's
// software rasterizer anti-aliases a path cut by a clip a few levels
// apart from the whole path, so the pixels may differ by that much.
func TestPartialCanvasRedrawsMatchFullOnes(t *testing.T) {
	for _, smp := range canvastest.Split(nil) {
		t.Run(smp.Name, func(t *testing.T) {
			kept := func(v *view) bool {
				c := canvasPixels(v).RGBAAt(0, 0)
				return c.R == 0xff && c.G == 0 && c.B == 0xff
			}
			mark := func(v *view) {
				ctx := v.inner.Call("getContext", "2d")
				ctx.Call("save")
				ctx.Call("setTransform", 1, 0, 0, 1, 0, 0)
				ctx.Set("fillStyle", "#ff00ff")
				ctx.Call("fillRect", 0, 0, 1, 1)
				ctx.Call("restore")
			}
			draw := func(value float64, change bool) *goimage.RGBA {
				s, _ := canvasZoomSurface(t)
				s.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: value}})
				v := mountCanvas(s, 20, 21, 1, driver.Points(smp.Width), driver.Points(smp.Height))
				s.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCanvas, ID: 21, Drawing: smp.Drawing}}})
				s.changedCanvases()
				if !change {
					return canvasPixels(v)
				}
				// A pixel no change reaches, which a whole play would
				// clear.
				mark(v)
				s.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 60}})
				s.changedCanvases()
				img := canvasPixels(v)
				if !kept(v) {
					t.Errorf("the change drew the whole canvas: the pixel it does not reach is %v", img.RGBAAt(0, 0))
				}
				img.SetRGBA(0, 0, canvasPixels(v).RGBAAt(1, 0))
				return img
			}
			want := draw(60, false)
			want.SetRGBA(0, 0, want.RGBAAt(1, 0))
			moved := draw(120, true)
			if want.Rect != moved.Rect {
				t.Fatalf("the canvases are %v and %v", want.Rect, moved.Rect)
			}
			if n, at := differ(want, moved, 8); n > 0 {
				t.Errorf("a partial redraw differs from a full one in %d pixels, the first at %v", n, at)
			}
		})
	}
}

// differ counts the pixels of a and b with a channel more than by apart,
// and returns the first.
func differ(a, b *goimage.RGBA, by int) (int, goimage.Point) {
	n, first := 0, goimage.Point{}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			c, d := a.RGBAAt(x, y), b.RGBAAt(x, y)
			for _, p := range [][2]uint8{{c.R, d.R}, {c.G, d.G}, {c.B, d.B}, {c.A, d.A}} {
				if int(p[0]) > int(p[1])+by || int(p[1]) > int(p[0])+by {
					if n == 0 {
						first = goimage.Pt(x, y)
					}
					n++
					break
				}
			}
		}
	}
	return n, first
}

type damageAnimNode uint32

func (n damageAnimNode) AnimatedNodeID() uint32 { return uint32(n) }

// TestPartialCanvasRedrawsStartFromDefaultState checks that a partial replay
// starts with the 2D context's defaults (black fill/stroke, line width 1, empty
// current path) rather than inheriting the state or clip path left by the
// previous play.
func TestPartialCanvasRedrawsStartFromDefaultState(t *testing.T) {
	d := driver.NewDrawing(nil)
	// Stroke before any BeginPath: would draw the clip rectangle if paintChange leaked it.
	d.Stroke()
	// Rely on default black fill and default stroke/line width over the damaged area before changing them.
	d.FillRect(10, 10, 140, 35)
	d.StrokeRect(20, 50, 120, 35)
	d.SetLineWidth(6).SetStrokeColor(driver.RGB(0x00ff00)).SetFillColor(driver.RGB(0xff0000)).SetAlpha(0.5)
	d.BeginPath().Arc(40, 50, 10, 0, 2*math.Pi, false).Bind(driver.ArgX, damageAnimNode(1))
	d.Fill()
	d.Stroke()

	draw := func(value float64, change bool) *goimage.RGBA {
		s, _ := canvasZoomSurface(t)
		s.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: value}})
		v := mountCanvas(s, 20, 21, 1, 160, 100)
		s.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCanvas, ID: 21, Drawing: d}}})
		s.changedCanvases()
		if !change {
			return canvasPixels(v)
		}
		ctx := v.inner.Call("getContext", "2d")
		ctx.Call("save")
		ctx.Call("setTransform", 1, 0, 0, 1, 0, 0)
		ctx.Set("fillStyle", "#ff00ff")
		ctx.Call("fillRect", 0, 0, 1, 1)
		ctx.Call("restore")
		s.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 60}})
		s.changedCanvases()
		img := canvasPixels(v)
		if c := img.RGBAAt(0, 0); c.R != 0xff || c.G != 0 || c.B != 0xff {
			t.Errorf("the change drew the whole canvas: pixel (0,0) = %v", c)
		}
		img.SetRGBA(0, 0, img.RGBAAt(1, 0))
		return img
	}
	want := draw(60, false)
	want.SetRGBA(0, 0, want.RGBAAt(1, 0))
	moved := draw(120, true)
	if want.Rect != moved.Rect {
		t.Fatalf("the canvases are %v and %v", want.Rect, moved.Rect)
	}
	if n, at := differ(want, moved, 8); n > 0 {
		t.Errorf("a partial redraw differs from a full one in %d pixels, the first at %v: want=%v got=%v", n, at, want.RGBAAt(at.X, at.Y), moved.RGBAAt(at.X, at.Y))
	}
}
