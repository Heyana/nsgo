//go:build windows

package windows

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasSamples plays the shared samples into a Direct2D bitmap at
// canvastest.Scale and reads its pixels back. NSGO_CANVAS_PNG names a
// directory to write them to.
func TestCanvasSamples(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	logo := canvastest.Logo("../../..")
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	pic, err := decode(context.Background(), logo)
	if err != nil {
		t.Fatal(err)
	}
	src := driver.ImageSource{Data: logo}
	w.canvasImages = map[string]*imageSlot{src.Key(): {src: src, pic: pic}}
	for _, s := range append(canvastest.Samples(w, logo), canvastest.Sampling(w), canvastest.AtlasSampling(w)) {
		t.Run(s.Name, func(t *testing.T) {
			img := drawImage(t, g, s.Width, s.Height, func(p *painter) {
				p.canvas(&view{drawing: s.Drawing}, float32(s.Width), float32(s.Height))
			}, w)
			if dir := os.Getenv("NSGO_CANVAS_PNG"); dir != "" {
				if f, err := os.Create(dir + "/windows-" + s.Name + ".png"); err == nil {
					png.Encode(f, img)
					f.Close()
				}
			}
			for _, f := range s.Check(img) {
				t.Error(f)
			}
		})
	}
}

// TestCanvasResolution plays a canvas at half the screen's density into a
// bitmap with half the pixels a side, stretched to the canvas smoothly and
// to the nearest pixel.
func TestCanvasResolution(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	for _, sampling := range []driver.ImageSampling{driver.ImageSamplingSmooth, driver.ImageSamplingNearest} {
		v := &view{drawing: canvastest.Resolution(w, sampling)}
		img := drawImage(t, g, canvastest.ResolutionSize, canvastest.ResolutionSize, func(p *painter) {
			p.canvas(v, canvastest.ResolutionSize, canvastest.ResolutionSize)
		}, w)
		if c := v.canvasCache; c.low == nil || c.lowW != canvastest.ResolutionPixels || c.lowH != canvastest.ResolutionPixels {
			t.Errorf("sampling %v: backing %d × %d, want %d a side", sampling, c.lowW, c.lowH, canvastest.ResolutionPixels)
		}
		for _, f := range canvastest.CheckResolution(img, image.Point{}, sampling) {
			t.Errorf("sampling %v: %s", sampling, f)
		}
		v.canvasCache.release()
	}
}

// TestImageNearestSampling draws the same 320×180 frame with two
// sampling settings to verify that changing the view's look updates it.
func TestImageNearestSampling(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	const width, height = 320, 180
	w := &window{scale: canvastest.Scale}
	frame := canvastest.Checkerboard(width, height)
	v := &view{image: driver.ImageConfig{ResizeMode: driver.Stretch}}
	v.source.showPixels(driver.ImageSource{Pixels: frame})
	t.Cleanup(func() { v.source.reset(driver.ImageSource{}) })
	draw := func() *image.RGBA {
		return drawImage(t, g, width, height, func(p *painter) {
			p.drawImage(v, 0, rectF{right: width, bottom: height})
		}, w)
	}
	if _, differs := canvastest.SamplingDifference(draw(), image.Point{}, frame, canvastest.Scale, 0); !differs {
		t.Error("smooth draw matches the nearest-neighbor reference")
	}
	v.image.Sampling = driver.ImageSamplingNearest
	img := draw()
	if p, differs := canvastest.SamplingDifference(img, image.Point{}, frame, canvastest.Scale, 0); differs {
		t.Errorf("nearest Image differs at %v: %v", p, img.At(p.X, p.Y))
	}
}

func TestCanvasMesh(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	draw := func(d *driver.Drawing) *image.RGBA {
		return drawImage(t, g, 96, 96, func(p *painter) {
			p.canvas(&view{drawing: d}, 96, 96)
		}, w)
	}
	vert := func(x, y, u, v driver.Points, tint driver.Color) driver.CanvasVertex {
		return driver.CanvasVertex{Position: driver.Point{X: x, Y: y}, TexCoord: driver.Point{X: u, Y: v}, Color: tint}
	}
	tex := image.NewRGBA(image.Rect(0, 0, 4, 4))
	tex.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
	tex.SetRGBA(2, 1, color.RGBA{G: 255, A: 255})
	tex.SetRGBA(1, 2, color.RGBA{B: 255, A: 255})
	tex.SetRGBA(2, 2, color.RGBA{R: 255, G: 255, A: 255})
	src := driver.ImageSource{Pixels: tex}
	a, b := vert(16, 16, 1, 1, 0), vert(80, 16, 3, 1, 0)
	c, e := vert(80, 80, 3, 3, 0), vert(16, 80, 1, 3, 0)
	ref := draw(driver.NewDrawing(nil).DrawImageFrom(src, 1, 1, 2, 2, 16, 16, 64, 64))
	got := draw(driver.NewDrawing(nil).DrawVertices(src, []driver.CanvasVertex{a, b, c, a, c, e}))
	if !bytes.Equal(ref.Pix, got.Pix) {
		t.Fatal("the textured mesh differs from the rectangular crop")
	}
	if c := got.RGBAAt(64, 64); c.R < 200 || c.G > 20 || c.B > 20 {
		t.Fatalf("the crop's first texel was not drawn: %v", c)
	}

	white := image.NewRGBA(image.Rect(0, 0, 1, 1))
	white.SetRGBA(0, 0, color.RGBA{R: 128, G: 192, B: 255, A: 255})
	tinted := draw(driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: white}, []driver.CanvasVertex{
		vert(0, 0, 0, 0, driver.RGB(0xff0000)),
		vert(72, 0, 1, 0, driver.RGB(0x00ff00)),
		vert(0, 72, 0, 1, driver.RGB(0x0000ff)),
	}))
	if c := tinted.RGBAAt(48, 48); c.R < 35 || c.R > 48 || c.G < 52 || c.G > 74 || c.B < 74 || c.B > 97 {
		t.Fatalf("the mesh's interpolated tint at its barycenter was %v", c)
	}

	// A triangle from an offscreen drawing reads the GPU bitmap back, upright.
	off := &driver.OffscreenCanvas{ID: 1, Version: 1, Size: driver.Size{Width: 40, Height: 20}, Scale: 2,
		Drawing: driver.NewDrawing(nil).
			SetFillColor(driver.RGB(0xff0000)).FillRect(0, 0, 40, 10).
			SetFillColor(driver.RGB(0x0000ff)).FillRect(0, 10, 40, 10)}
	sampled := draw(driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Offscreen: off}, []driver.CanvasVertex{
		vert(0, 0, 0, 0, 0), vert(80, 0, 40, 0, 0), vert(0, 80, 0, 20, 0),
	}))
	w.closeOffscreen()
	at := func(x, y int) color.RGBA { return sampled.RGBAAt(x*canvastest.Scale, y*canvastest.Scale) }
	if c := at(10, 10); c.R < 240 || c.G > 15 || c.B > 15 {
		t.Errorf("the offscreen mesh's top was %v, want red", c)
	}
	if c := at(10, 60); c.R > 15 || c.G > 15 || c.B < 240 {
		t.Errorf("the offscreen mesh's bottom was %v, want blue", c)
	}
}

// TestCanvasImageTint draws the shared tint sample from an encoded image, a pixel buffer and an offscreen image.
func TestCanvasImageTint(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	encoded, err := canvastest.TintSource()
	if err != nil {
		t.Fatal(err)
	}
	pic, err := decode(context.Background(), encoded.Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"encoded", "pixels", "offscreen"} {
		t.Run(name, func(t *testing.T) {
			w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
			defer w.closeCanvasImages()
			defer w.closeOffscreen()
			var src driver.ImageSource
			switch name {
			case "encoded":
				src = encoded
				w.canvasImages = map[string]*imageSlot{src.Key(): {src: src, pic: pic}}
			case "pixels":
				src = canvastest.TintPixelsSource()
			case "offscreen":
				src = canvastest.TintOffscreenSource(driver.NewDrawing(w), 1)
			}
			const width, height = canvastest.TintWidth, canvastest.TintHeight
			d := driver.NewDrawing(w)
			canvastest.DrawTintSample(d, src, width, height)
			img := drawImage(t, g, width, height, func(p *painter) {
				p.canvas(&view{drawing: d}, width, height)
			}, w)
			for _, f := range canvastest.CheckTintSample(img, driver.Point{}, canvastest.Scale) {
				t.Error(f)
			}
		})
	}
}

// drawImage draws with draw into a Direct2D bitmap width by height points
// at canvastest.Scale, cleared white, and reads its pixels back.
func drawImage(t *testing.T, g *gfx, width, height int, draw func(p *painter), w *window) *image.RGBA {
	t.Helper()
	pw, ph := uint32(width*canvastest.Scale), uint32(height*canvastest.Scale)
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * canvastest.Scale,
		dpiY: 96 * canvastest.Scale, options: d2dBitmapTarget}
	var target, readable *comObject
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
		t.Fatal("no target bitmap")
	}
	defer target.release()
	props.options = d2dBitmapCPURead | d2dBitmapCannotDraw
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
		t.Fatal("no readable bitmap")
	}
	defer readable.release()
	g.dc.call(dcSetTarget, target.ptr())
	g.dc.callF(rtSetDpi, 0b11, f32(96*canvastest.Scale), f32(96*canvastest.Scale))
	g.dc.call(rtBeginDraw)
	white := colorF{1, 1, 1, 1}
	g.dc.call(rtClear, uintptr(unsafe.Pointer(&white)))
	p := newPainter(w, g)
	p.setTransform(identity)
	draw(p)
	p.release()
	if hr := g.dc.call(rtEndDraw, 0, 0); failed(hr) {
		t.Fatal(hresultError("EndDraw", hr))
	}
	g.dc.call(dcSetTarget, 0)
	readable.call(bitmapCopyFromBitmap, 0, target.ptr(), 0)
	var mapped struct {
		pitch uint32
		bits  *byte
	}
	if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
		t.Fatal("cannot map the pixels")
	}
	img := image.NewRGBA(image.Rect(0, 0, int(pw), int(ph)))
	rows := unsafe.Slice(mapped.bits, int(mapped.pitch)*int(ph))
	for y := range int(ph) {
		for x := range int(pw) {
			b := rows[y*int(mapped.pitch)+x*4:]
			// Premultiplied BGRA over the white cleared first; a
			// cleared pixel reads white.
			img.Pix[y*img.Stride+x*4+0] = b[2] + 255 - b[3]
			img.Pix[y*img.Stride+x*4+1] = b[1] + 255 - b[3]
			img.Pix[y*img.Stride+x*4+2] = b[0] + 255 - b[3]
			img.Pix[y*img.Stride+x*4+3] = 255
		}
	}
	readable.call(bitmapUnmap)
	return img
}

// TestCanvasListsDrawWhatAFullPlayDoes draws the split samples from
// their static part's list and the rest's, and played whole.
func TestCanvasListsDrawWhatAFullPlayDoes(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	for _, s := range canvastest.Split(nil) {
		t.Run(s.Name, func(t *testing.T) {
			w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
			w.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 60}})
			v := &view{id: 1, drawing: s.Drawing}
			w.animator.BindCanvas(v.id, s.Drawing.Slots)
			whole := drawImage(t, g, s.Width, s.Height, func(p *painter) {
				p.playCanvas(v, w.animator.CanvasOps(v.id, v.drawing), float32(s.Width), float32(s.Height))
			}, w)
			lists := drawImage(t, g, s.Width, s.Height, func(p *painter) {
				p.canvas(v, float32(s.Width), float32(s.Height))
			}, w)
			if v.canvasCache.split.At == 0 || v.canvasCache.tail == nil {
				t.Fatalf("the drawing was not split: %+v", v.canvasCache.split)
			}
			v.canvasCache.release()
			if n, at := canvastest.Differ(whole, lists); n > 0 {
				t.Errorf("%d pixels differ from a full play, the first at %v", n, at)
			}
		})
	}
}

// TestStyleTransformAndScrollZoomAreSharp verifies in Direct2D pixels that a 1-point black line in a canvas (both vector command-list and offscreen mixing paths) draws sharply at 3x scale under a style transform and under a zooming scroll view, with no grey edge blur.
func TestStyleTransformAndScrollZoomAreSharp(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	checkSharpLine := func(t *testing.T, img *image.RGBA, zoom int) {
		t.Helper()
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
		want := zoom * canvastest.Scale
		if black != want || gray != 0 {
			t.Errorf("zoom %dx: line has %d black and %d gray pixels, want %d black and 0 gray", zoom, black, gray, want)
		}
	}
	makeLine := func(mix bool) *driver.Drawing {
		d := driver.NewDrawing(nil)
		if mix {
			d.Clear(0, 0, 100, 100)
		}
		d.SetFillColor(driver.RGB(0xffffff)).FillRect(0, 0, 100, 100)
		d.SetFillColor(driver.RGB(0x000000)).FillRect(20, 0, 1, 100)
		return d
	}
	for _, mix := range []bool{false, true} {
		name := "vector"
		if mix {
			name = "offscreen"
		}
		t.Run("style-transform-"+name, func(t *testing.T) {
			w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
			canvas := &view{id: 2, kind: driver.KindCanvas, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}},
				style: driver.ViewStyle{Opacity: 1}, drawing: makeLine(mix)}
			holder := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}},
				style:    driver.ViewStyle{Opacity: 1, Origin: driver.Point{X: -50, Y: -50}, Transform: driver.Transform{Scale: 3}},
				children: []*view{canvas}}
			canvas.parent = holder
			defer canvas.canvasCache.release()
			img := drawImage(t, g, 300, 300, func(p *painter) {
				p.draw(holder, identity, 0, 0)
			}, w)
			checkSharpLine(t, img, 3)
		})
		t.Run("scroll-zoom-"+name, func(t *testing.T) {
			w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
			canvas := &view{id: 2, kind: driver.KindCanvas, frame: driver.Bounds{Size: driver.Size{Width: 100, Height: 100}},
				style: driver.ViewStyle{Opacity: 1}, drawing: makeLine(mix)}
			scroll := &view{id: 1, kind: driver.KindScroll, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 300}},
				size:   driver.Size{Width: 100, Height: 100},
				scroll: driver.ScrollConfig{MinZoomScale: 0.5, MaxZoomScale: 4, ZoomScale: 3},
				zoom:   3, style: driver.ViewStyle{Opacity: 1}, children: []*view{canvas}}
			canvas.parent = scroll
			defer canvas.canvasCache.release()
			img := drawImage(t, g, 300, 300, func(p *painter) {
				p.draw(scroll, identity, 0, 0)
			}, w)
			checkSharpLine(t, img, 3)
		})
	}
}

// TestCanvasCompositeInClip verifies that non-SourceOver compositing inside an
// active Clip preserves earlier pixels inside and outside the clip, both in an
// OffscreenCanvas (at 1x and 2x) and in a direct offscreen-backed Canvas, and
// includes prior SourceOver draws made inside the same clip layer (#2391).
func TestCanvasCompositeInClip(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	const panel = 160
	record := func(priorInClip bool, op driver.CompositeOp) *driver.Drawing {
		d := driver.NewDrawing(nil)
		d.SetFillColor(driver.RGB(0x2e7d32)).FillRect(0, 0, panel, panel)
		d.Save()
		d.BeginPath().Rect(20, 20, panel-40, panel-40)
		d.Clip()
		if priorInClip {
			d.SetFillColor(driver.RGB(0x102080)).FillRect(20, 20, 40, 40)
		}
		d.SetComposite(op)
		d.SetFillColor(driver.RGBA(0xff4040, 0.6))
		d.BeginPath().MoveTo(panel/2+50, panel/2).Arc(panel/2, panel/2, 50, 0, 2*math.Pi, false).ClosePath().Fill()
		d.Restore()
		return d
	}
	checkLighter := func(t *testing.T, img *image.RGBA, scale int) {
		t.Helper()
		at := func(x, y int) color.RGBA { return img.RGBAAt(x*scale, y*scale) }
		// Outside the clip: green background stays intact (0x2e, 0x7d, 0x32).
		if c := at(10, 10); c.R > 60 || c.G < 110 || c.G > 140 || c.B > 65 {
			t.Errorf("outside clip = %v, want green background", c)
		}
		// Inside the clip, outside the disc: green background is preserved, not erased.
		if c := at(25, 130); c.R > 60 || c.G < 110 || c.G > 140 || c.B > 65 {
			t.Errorf("inside clip outside disc = %v, want preserved green background", c)
		}
		// Inside the clip and inside the disc: green (46,125,50) + 0.6*red (153,38,38) ≈ (199,163,88).
		// Reject both transparent (255,255,255) and unblended red over white (255,140,140).
		if c := at(80, 80); c.R < 170 || c.R > 230 || c.G < 140 || c.G > 190 || c.B < 65 || c.B > 115 {
			t.Errorf("inside clip inside disc = %v, want additive blend of green and red (~{199 163 88 255})", c)
		}
	}
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run("offscreen-lighter", func(t *testing.T) {
			w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
			defer w.closeOffscreen()
			off := &driver.OffscreenCanvas{
				ID:      1,
				Version: 1,
				Size:    driver.Size{Width: panel, Height: panel},
				Scale:   scale,
				Drawing: record(false, driver.CompositeLighter),
			}
			d := driver.NewDrawing(nil).DrawImage(driver.ImageSource{Offscreen: off}, 0, 0)
			img := drawImage(t, g, panel, panel, func(p *painter) {
				p.canvas(&view{drawing: d}, panel, panel)
			}, w)
			checkLighter(t, img, canvastest.Scale)
		})
	}
	t.Run("direct-canvas-lighter-after-clipped-draw", func(t *testing.T) {
		w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
		d := record(true, driver.CompositeLighter)
		img := drawImage(t, g, panel, panel, func(p *painter) {
			p.canvas(&view{drawing: d}, panel, panel)
		}, w)
		checkLighter(t, img, canvastest.Scale)
		// The prior SourceOver blue rect drawn inside the clip before Lighter must survive too.
		if c := img.RGBAAt(30*canvastest.Scale, 30*canvastest.Scale); c.R > 35 || c.G > 50 || c.B < 110 {
			t.Errorf("prior SourceOver draw inside clip = %v, want blue rect", c)
		}
	})
	t.Run("offscreen-copy-respects-clip", func(t *testing.T) {
		w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
		defer w.closeOffscreen()
		off := &driver.OffscreenCanvas{
			ID:      2,
			Version: 1,
			Size:    driver.Size{Width: panel, Height: panel},
			Scale:   1,
			Drawing: record(false, driver.CompositeCopy),
		}
		d := driver.NewDrawing(nil).DrawImage(driver.ImageSource{Offscreen: off}, 0, 0)
		img := drawImage(t, g, panel, panel, func(p *painter) {
			p.canvas(&view{drawing: d}, panel, panel)
		}, w)
		// Outside clip stays green; inside clip outside disc is cleared to transparent (white over drawImage's white backdrop).
		if c := img.RGBAAt(10*canvastest.Scale, 10*canvastest.Scale); c.R > 60 || c.G < 110 || c.B > 65 {
			t.Errorf("Copy outside clip = %v, want green", c)
		}
		if c := img.RGBAAt(25*canvastest.Scale, 130*canvastest.Scale); c.R < 240 || c.G < 240 || c.B < 240 {
			t.Errorf("Copy inside clip outside disc = %v, want cleared to white backdrop", c)
		}
	})
}
