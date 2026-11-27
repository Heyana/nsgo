//go:build darwin

package apple

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// makeTestSpriteSheet builds a 64x64 premultiplied RGBA sprite sheet with four
// distinct 16x16 tiles (including semi-transparent borders and bright neighbors
// to catch atlas bleeding or channel swaps).
func makeTestSpriteSheet() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			tileX, tileY := x/16, y/16
			lx, ly := x%16, y%16
			var c color.RGBA
			switch (tileX + tileY*2) % 4 {
			case 0: // red gradient with transparent corner
				if lx < 2 && ly < 2 {
					c = color.RGBA{}
				} else {
					a := uint8(160 + (lx+ly)*3)
					r := uint8(int(a) * (180 + lx*4) / 255)
					g := uint8(int(a) * (30 + ly*4) / 255)
					b := uint8(int(a) * 20 / 255)
					c = color.RGBA{R: r, G: g, B: b, A: a}
				}
			case 1: // opaque green-cyan checker
				if (lx/2+ly/2)%2 == 0 {
					c = color.RGBA{R: 20, G: 220, B: 140, A: 255}
				} else {
					c = color.RGBA{R: 10, G: 90, B: 200, A: 255}
				}
			case 2: // blue soft disc (premultiplied)
				dx, dy := float64(lx)-7.5, float64(ly)-7.5
				dist := math.Hypot(dx, dy)
				if dist < 7 {
					a := uint8(math.Round((1 - dist/7) * 255))
					c = color.RGBA{R: uint8(int(a) * 60 / 255), G: uint8(int(a) * 140 / 255), B: a, A: a}
				}
			case 3: // warm yellow-magenta stripes
				if ly%2 == 0 {
					c = color.RGBA{R: 240, G: 200, B: 40, A: 255}
				} else {
					c = color.RGBA{R: 200, G: 40, B: 220, A: 200}
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func playAtlasMode(t *testing.T, d *driver.Drawing, w, h int, flipped, fast bool) *image.RGBA {
	t.Helper()
	ctx, img := bitmap(w, h)
	defer Release(ctx)
	if layerDrawer == 0 {
		layerDrawer = objc.ID(RegisterClass("NsgoTestDrawer", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
			Cmd: sel("drawLayer:inContext:"),
			Fn:  func(_ objc.ID, _ objc.SEL, _ objc.ID, ctx uintptr) { layerDraw(ctx) },
		}})).Send(selNew)
	}
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	layer.Send(sel("setBounds:"), Rect(0, 0, float64(w), float64(h)))
	layer.Send(sel("setContentsScale:"), float64(canvastest.Scale))
	layer.Send(sel("setGeometryFlipped:"), flipped)
	layer.Send(sel("setDelegate:"), layerDrawer)
	layerDraw = func(lctx uintptr) {
		DrawTopDown(lctx, float64(h), !MsgBool(layer, sel("contentsAreFlipped")), func(baseUp bool) {
			if fast {
				Play(lctx, d.Ops, CanvasHost{Ratio: canvastest.Scale, BaseUp: baseUp})
				return
			}
			prev := disableAtlasFast
			disableAtlasFast = true
			Play(lctx, d.Ops, CanvasHost{Ratio: canvastest.Scale, BaseUp: baseUp})
			disableAtlasFast = prev
		})
	}
	layer.Send(sel("display"))
	if flipped {
		cgContextTranslateCTM(ctx, 0, float64(h))
		cgContextScaleCTM(ctx, 1, -1)
	}
	layer.Send(sel("renderInContext:"), ctx)
	return img
}

func compareImages(t *testing.T, label string, got, want *image.RGBA, maxChannelDiff int, maxDiffRatio float64) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatalf("%s: bounds mismatch %v vs %v", label, got.Bounds(), want.Bounds())
	}
	b := got.Bounds()
	total := b.Dx() * b.Dy()
	diffs := 0
	worst := 0
	var worstPt image.Point
	var worstGot, worstWant color.RGBA
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g, w := got.RGBAAt(x, y), want.RGBAAt(x, y)
			dr := absInt(int(g.R) - int(w.R))
			dg := absInt(int(g.G) - int(w.G))
			db := absInt(int(g.B) - int(w.B))
			da := absInt(int(g.A) - int(w.A))
			d := max(max(dr, dg), max(db, da))
			if d > maxChannelDiff {
				diffs++
				if d > worst {
					worst = d
					worstPt = image.Pt(x, y)
					worstGot, worstWant = g, w
				}
			}
		}
	}
	if float64(diffs)/float64(total) > maxDiffRatio {
		t.Errorf("%s: %d/%d pixels (%.2f%%) exceeded channel diff %d; worst diff=%d at %v got=%v want=%v",
			label, diffs, total, 100*float64(diffs)/float64(total), maxChannelDiff, worst, worstPt, worstGot, worstWant)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestAtlasFastMatchesCoreGraphics(t *testing.T) {
	sheet := makeTestSpriteSheet()
	srcPixels := driver.ImageSource{Pixels: sheet, Size: driver.Size{Width: 64, Height: 64}}

	// Encode a dedicated in-memory PNG to exercise the decoded CGImage -> meshPixels BGRA path
	// without sharing a cache key with TestCanvasSamples.
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, sheet); err != nil {
		t.Fatal(err)
	}
	srcPNG := driver.ImageSource{Data: pngBuf.Bytes()}
	loaded := make(chan bool, 1)
	if im := canvasImage(srcPNG, CanvasHost{Ratio: 2, Redraw: func() { loaded <- true }, Owner: t}); im == nil || im.image == 0 {
		deadline := time.Now().Add(5 * time.Second)
		for len(loaded) == 0 && time.Now().Before(deadline) {
			runMain()
			time.Sleep(time.Millisecond)
		}
		if len(loaded) == 0 {
			t.Fatal("test PNG did not load")
		}
	}

	rects := make([]driver.Bounds, 24)
	xformsAxis := make([]driver.RSXform, 24)
	xformsRot := make([]driver.RSXform, 24)
	alphas := make([]float32, 24)
	for i := range 24 {
		tx := float32((i % 2) * 16)
		ty := float32(((i / 2) % 2) * 16)
		rects[i] = driver.Bounds{
			Origin: driver.Point{X: driver.Points(tx), Y: driver.Points(ty)},
			Size:   driver.Size{Width: 16, Height: 16},
		}
		px := float32(8 + (i%6)*22)
		py := float32(8 + (i/6)*22)
		xformsAxis[i] = driver.RSXform{SCos: 1.5, TX: px, TY: py}
		ang := float64(i) * 0.25
		scale := 1.25
		xformsRot[i] = driver.RSXform{
			SCos: float32(math.Cos(ang) * scale),
			SSin: float32(math.Sin(ang) * scale),
			TX:   px + 12,
			TY:   py + 8,
		}
		alphas[i] = 0.25 + 0.75*float32(i%5)/4
	}

	cases := []struct {
		name     string
		w, h     int
		draw     func(d *driver.Drawing)
		maxDiff  int
		maxRatio float64
	}{
		{
			name: "axis-nearest-pixels",
			w:    160, h: 120,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x131628)).FillRect(0, 0, 160, 120)
				d.Save().SetImageSmoothing(false).DrawAtlas(srcPixels, rects, xformsAxis, alphas).Restore()
			},
			maxDiff:  1,
			maxRatio: 0.0,
		},
		{
			name: "axis-linear-pixels",
			w:    160, h: 120,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x131628)).FillRect(0, 0, 160, 120)
				d.DrawAtlas(srcPixels, rects, xformsAxis, alphas)
			},
			// Bilinear vs CoreGraphics default (bicubic) kernel differs modestly on 1px high-contrast stripes.
			maxDiff:  20,
			maxRatio: 0.0,
		},
		{
			name: "axis-nearest-png",
			w:    160, h: 120,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x203040)).FillRect(0, 0, 160, 120)
				d.Save().SetImageSmoothing(false).SetAlpha(0.75).DrawAtlas(srcPNG, rects, xformsAxis, alphas).Restore()
			},
			maxDiff:  2,
			maxRatio: 0.0,
		},
		{
			name: "rotated-nearest",
			w:    160, h: 120,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x181c2e)).FillRect(0, 0, 160, 120)
				d.Save().SetImageSmoothing(false).DrawAtlas(srcPixels, rects, xformsRot, alphas).Restore()
			},
			// CoreGraphics anti-aliases rotated quad perimeters (~3.8% border pixels); all interiors match exactly.
			maxDiff:  2,
			maxRatio: 0.05,
		},
		{
			name: "tier2-clipped-roundrect",
			w:    160, h: 120,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x131628)).FillRect(0, 0, 160, 120)
				d.Save().BeginPath().RoundRect(16, 16, 128, 88, 20).Clip()
				d.SetAlpha(0.8).SetImageSmoothing(false).DrawAtlas(srcPixels, rects, xformsAxis, alphas)
				d.Restore()
			},
			maxDiff:  2,
			maxRatio: 0.005,
		},
		{
			name: "shadow-and-multiply-fallback",
			w:    160, h: 120,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0xe0e4f0)).FillRect(0, 0, 160, 120)
				d.Save().SetShadow(driver.RGBA(0x000000, 0.5), 3, 3, 4).DrawAtlas(srcPixels, rects[:4], xformsAxis[:4], alphas[:4]).Restore()
				d.Save().SetComposite(driver.CompositeMultiply).DrawAtlas(srcPixels, rects[4:8], xformsAxis[4:8], alphas[4:8]).Restore()
			},
			maxDiff:  0,
			maxRatio: 0.0,
		},
		{
			name: "caverun-1000-nearest",
			w:    320, h: 180,
			draw: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x131628)).FillRect(0, 0, 320, 180)
				d.Save().SetImageSmoothing(false)
				r := make([]driver.Bounds, 1000)
				xf := make([]driver.RSXform, 1000)
				al := make([]float32, 1000)
				for i := range 1000 {
					r[i] = driver.Bounds{
						Origin: driver.Point{X: driver.Points((i % 4) * 16), Y: 0},
						Size:   driver.Size{Width: 16, Height: 16},
					}
					xf[i] = driver.RSXform{
						SCos: 0.5,
						TX:   float32((i*37)%304) + 4,
						TY:   float32((i*53)%164) + 4,
					}
					al[i] = 0.25 + 0.5*float32(i%7)/6
				}
				d.DrawAtlas(srcPixels, r, xf, al)
				d.Restore()
			},
			maxDiff:  2,
			maxRatio: 0.001,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := driver.NewDrawing(nil)
			tc.draw(d)
			for _, flipped := range []bool{false, true} {
				want := playAtlasMode(t, d, tc.w, tc.h, flipped, false)
				got := playAtlasMode(t, d, tc.w, tc.h, flipped, true)
				compareImages(t, tc.name+"/tier1", got, want, tc.maxDiff, tc.maxRatio)

				prev := forceAtlasScratch
				forceAtlasScratch = true
				gotScratch := playAtlasMode(t, d, tc.w, tc.h, flipped, true)
				forceAtlasScratch = prev
				compareImages(t, tc.name+"/tier2", gotScratch, want, tc.maxDiff, tc.maxRatio)
			}
		})
	}
}

func TestAtlasBitmapTargetDirectAndDamageClip(t *testing.T) {
	ctx, img := bitmap(64, 64)
	defer runtime.KeepAlive(img)
	defer Release(ctx)
	pix, w, h, stride, dstBGRA, ok := atlasBitmapTarget(ctx)
	if !ok {
		t.Fatal("atlasBitmapTarget(bitmap) returned ok=false")
	}
	if w != 64*canvastest.Scale || h != 64*canvastest.Scale || stride != img.Stride || dstBGRA || len(pix) != len(img.Pix) {
		t.Fatalf("unexpected bitmap target: w=%d h=%d stride=%d dstBGRA=%v len=%d", w, h, stride, dstBGRA, len(pix))
	}

	// Verify Tier 1 honors a pre-Play damage clip rectangle on an RGBA bitmap context.
	sheet := makeTestSpriteSheet()
	src := driver.ImageSource{Pixels: sheet, Size: driver.Size{Width: 64, Height: 64}}
	d := driver.NewDrawing(nil)
	d.SetImageSmoothing(false).DrawAtlas(src,
		[]driver.Bounds{{Origin: driver.Point{X: 16, Y: 0}, Size: driver.Size{Width: 16, Height: 16}}},
		[]driver.RSXform{{SCos: 4, TX: 0, TY: 0}},
		[]float32{1},
	)
	ctxRef, imgRef := bitmap(64, 64)
	defer runtime.KeepAlive(imgRef)
	defer Release(ctxRef)

	damage := Rect(16, 16, 32, 32)
	cgContextClipToRect(ctx, damage)
	cgContextClipToRect(ctxRef, damage)

	Play(ctx, d.Ops, CanvasHost{Ratio: canvastest.Scale})
	prev := disableAtlasFast
	disableAtlasFast = true
	Play(ctxRef, d.Ops, CanvasHost{Ratio: canvastest.Scale})
	disableAtlasFast = prev

	compareImages(t, "damage-clip", img, imgRef, 1, 0.0)
}

func TestRenderDrawingDrawAtlas(t *testing.T) {
	sheet := makeTestSpriteSheet()
	src := driver.ImageSource{Pixels: sheet, Size: driver.Size{Width: 64, Height: 64}}
	d := driver.NewDrawing(nil)
	d.SetFillColor(driver.RGB(0x101010)).FillRect(0, 0, 32, 32)
	d.SetImageSmoothing(false).DrawAtlas(src,
		[]driver.Bounds{{Origin: driver.Point{X: 16, Y: 0}, Size: driver.Size{Width: 16, Height: 16}}},
		[]driver.RSXform{{SCos: 1, TX: 8, TY: 8}},
		[]float32{1},
	)
	got, err := RenderDrawing(d, driver.Size{Width: 32, Height: 32}, 2)
	if err != nil {
		t.Fatal(err)
	}
	px := got.RGBAAt(16, 16)
	if px != (color.RGBA{R: 20, G: 220, B: 140, A: 255}) {
		t.Fatalf("center pixel = %v, want green tile texel", px)
	}
}

func caveRunAtlasDrawing(n int, nearest bool) *driver.Drawing {
	// 128x128 sprite sheet with 8x8 spark frames (semi-transparent disc), matching Cave Run's sprites.png.
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for frame := range 3 {
		ox := frame * 8
		for y := range 8 {
			for x := range 8 {
				dx, dy := float64(x)-3.5, float64(y)-3.5
				if math.Hypot(dx, dy) <= 3.2 {
					img.SetRGBA(ox+x, y, color.RGBA{R: 200, G: 180, B: 90, A: 200})
				}
			}
		}
	}
	src := driver.ImageSource{Pixels: img, Size: driver.Size{Width: 128, Height: 128}}
	rects := make([]driver.Bounds, n)
	xforms := make([]driver.RSXform, n)
	alphas := make([]float32, n)
	for i := range n {
		frame := i % 3
		rects[i] = driver.Bounds{
			Origin: driver.Point{X: driver.Points(frame * 8), Y: 0},
			Size:   driver.Size{Width: 8, Height: 8},
		}
		xforms[i] = driver.RSXform{
			SCos: 0.5,
			TX:   float32((i*37)%312) + 2,
			TY:   float32((i*53)%172) + 2,
		}
		alphas[i] = 0.2 + 0.55*float32(0.5+0.5*math.Sin(float64(i)*0.17))
	}
	d := driver.NewDrawing(nil)
	d.Save()
	if nearest {
		d.SetImageSmoothing(false)
	}
	d.Scale(4, 4) // 320x180 -> 1280x720
	d.DrawAtlas(src, rects, xforms, alphas)
	d.Restore()
	return d
}

func BenchmarkCaveRunAtlas1000_CALayer_Linear(b *testing.B) {
	benchmarkCaveRunCALayer(b, false, false)
}

func BenchmarkCaveRunAtlas1000_CALayer_Nearest(b *testing.B) {
	benchmarkCaveRunCALayer(b, true, false)
}

func BenchmarkCaveRunAtlas1000_Tier2Scratch_Linear(b *testing.B) {
	benchmarkCaveRunCALayer(b, false, true)
}

func BenchmarkCaveRunAtlas1000_Bitmap_Linear(b *testing.B) {
	d := caveRunAtlasDrawing(1000, false)
	ctx, img := bitmap(1280, 720)
	defer runtime.KeepAlive(img)
	defer Release(ctx)
	host := CanvasHost{Ratio: canvastest.Scale}
	Play(ctx, d.Ops, host)
	b.ResetTimer()
	for b.Loop() {
		Play(ctx, d.Ops, host)
	}
}

func benchmarkCaveRunCALayer(b *testing.B, nearest, forceScratch bool) {
	d := caveRunAtlasDrawing(1000, nearest)
	if layerDrawer == 0 {
		layerDrawer = objc.ID(RegisterClass("NsgoBenchDrawer", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
			Cmd: sel("drawLayer:inContext:"),
			Fn:  func(_ objc.ID, _ objc.SEL, _ objc.ID, ctx uintptr) { layerDraw(ctx) },
		}})).Send(selNew)
	}
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	layer.Send(sel("setBounds:"), Rect(0, 0, 1280, 720))
	layer.Send(sel("setContentsScale:"), float64(2))
	layer.Send(sel("setGeometryFlipped:"), true)
	layer.Send(sel("setDelegate:"), layerDrawer)
	layerDraw = func(lctx uintptr) {
		DrawTopDown(lctx, 720, !MsgBool(layer, sel("contentsAreFlipped")), func(baseUp bool) {
			Play(lctx, d.Ops, CanvasHost{Ratio: 2, BaseUp: baseUp})
		})
	}
	prev := forceAtlasScratch
	forceAtlasScratch = forceScratch
	defer func() { forceAtlasScratch = prev }()
	layer.Send(sel("setNeedsDisplay"))
	layer.Send(sel("display"))
	b.ResetTimer()
	for b.Loop() {
		layer.Send(sel("setNeedsDisplay"))
		layer.Send(sel("display"))
	}
}
