//go:build linux && !android

package linux

import (
	"image"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// rasterImage returns v's raster's pixels.
func rasterImage(r *canvasRaster) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.pw, r.ph))
	data, stride := cairoImageSurfaceGetData(r.surface), int(cairoImageSurfaceGetStride(r.surface))
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*r.ph)
	for y := range r.ph {
		for x := range r.pw {
			p := rows[y*stride+x*4:]
			img.Pix[y*img.Stride+x*4] = p[2]
			img.Pix[y*img.Stride+x*4+1] = p[1]
			img.Pix[y*img.Stride+x*4+2] = p[0]
			img.Pix[y*img.Stride+x*4+3] = p[3]
		}
	}
	return img
}

// TestPartialRastersMatchFullOnes draws the split samples whole, moves
// their bound value and draws what it damaged over the kept static
// part, and compares that with drawing them whole at the new value.
func TestPartialRastersMatchFullOnes(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	for _, s := range canvastest.Split(nil) {
		t.Run(s.Name, func(t *testing.T) {
			const scale = canvastest.Scale
			pw, ph := s.Width*scale, s.Height*scale
			draw := func(value float64, then func(w *window, v *view)) *image.RGBA {
				w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: scale}}
				w.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: value}})
				v := &view{id: 1}
				w.views = []*view{nil, v}
				w.setCanvas(v, s.Drawing)
				if v.canvas.split.At == 0 {
					t.Fatal("no static part is kept")
				}
				w.changedCanvases()
				w.rasterize(v, pw, ph, scale)
				v.canvas.whole = false
				if then != nil {
					then(w, v)
					w.rasterize(v, pw, ph, scale)
				}
				defer v.canvas.raster.release()
				if v.canvas.raster.static == 0 {
					t.Fatal("the static part's pixels were not kept")
				}
				return rasterImage(v.canvas.raster)
			}
			want := draw(60, nil)
			moved := draw(120, func(w *window, v *view) {
				w.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 60}})
				w.changedCanvases()
			})
			if n, at := canvastest.Differ(want, moved); n > 0 {
				t.Errorf("a partial redraw differs from a full one in %d pixels, the first at %v", n, at)
			}
			// All of it drawn over the static part.
			over := draw(60, func(w *window, v *view) {
				v.canvas.area = driver.Bounds{Size: driver.Size{Width: driver.Points(s.Width), Height: driver.Points(s.Height)}}
			})
			if n, at := canvastest.Differ(want, over); n > 0 {
				t.Errorf("drawing over the static part differs in %d pixels, the first at %v", n, at)
			}
		})
	}
}

// TestCanvasesCairoCannotHoldDrawNothing lays a canvas out wider than a
// Cairo surface can be, and shows a raster whose textures were never
// made, as one whose surface failed: neither panics, and neither draws.
func TestCanvasesCairoCannotHoldDrawNothing(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	s := canvastest.Split(nil)[0]
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	v := &view{id: 1}
	w.views = []*view{nil, v}
	w.setCanvas(v, s.Drawing)
	w.rasterize(v, maxRaster+1, 10, 1)
	if v.canvas.raster != nil {
		t.Fatalf("a %d-pixel-wide canvas has a raster", maxRaster+1)
	}
	v.canvas.whole = false
	v.canvas.raster = &canvasRaster{pw: 300, ph: 10, scale: 1}
	w.snapshotCanvas(v, 0, 300, 10)
}

// TestRasterCanvasSharpUnderStyleTransformAndScrollZoom verifies in Cairo pixels that a 1-point black line in a canvas rasterizes sharply at 3x scale under both a style transform and a zooming scroll view, with no grey blur.
func TestRasterCanvasSharpUnderStyleTransformAndScrollZoom(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	d := driver.NewDrawing(nil)
	d.SetFillColor(driver.RGB(0xffffff)).FillRect(0, 0, 100, 100)
	d.SetFillColor(driver.RGB(0x000000)).FillRect(20, 0, 1, 100)

	check := func(t *testing.T, parent *view, wantZoom int) {
		t.Helper()
		w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: canvastest.Scale}}
		canvas := &view{id: 2, kind: driver.KindCanvas, parent: parent, style: driver.ViewStyle{Opacity: 1}}
		w.views = []*view{nil, parent, canvas}
		w.setCanvas(canvas, d)
		scale := w.canvasRasterScale(canvas, 100, 100)
		wantScale := float64(wantZoom * canvastest.Scale)
		if scale != wantScale {
			t.Fatalf("canvasRasterScale = %v, want %v", scale, wantScale)
		}
		w.rasterize(canvas, int(100*scale), int(100*scale), scale)
		defer canvas.canvas.raster.release()
		img := rasterImage(canvas.canvas.raster)
		b := img.Bounds()
		y := b.Min.Y + b.Dy()/2
		black, gray := 0, 0
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			switch {
			case r < 0x2000:
				black++
			case r < 0xe000:
				gray++
			}
		}
		want := wantZoom * canvastest.Scale
		if black != want || gray != 0 {
			t.Errorf("zoom %dx: line has %d black and %d gray pixels, want %d black and 0 gray", wantZoom, black, gray, want)
		}
	}

	t.Run("style-transform", func(t *testing.T) {
		holder := &view{id: 1, kind: driver.KindView, style: driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Scale: 3}}}
		check(t, holder, 3)
	})
	t.Run("scroll-zoom", func(t *testing.T) {
		scroll := &view{id: 1, kind: driver.KindScroll, scroll: driver.ScrollConfig{MinZoomScale: 0.5, MaxZoomScale: 4, ZoomScale: 3},
			zoom: 3, style: driver.ViewStyle{Opacity: 1}}
		check(t, scroll, 3)
	})
}
