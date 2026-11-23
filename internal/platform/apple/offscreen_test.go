//go:build darwin

package apple

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

func init() { EnableCanvas() }

func TestOffscreenImageRenderCacheInvalidateRelease(t *testing.T) {
	const id uint64 = 987654321
	defer ReleaseOffscreen(id)

	red := driver.NewDrawing(nil).
		SetFillColor(driver.RGB(0xff0000)).FillRect(0, 0, 20, 20).
		SetFillColor(driver.RGB(0x0000ff)).FillRect(20, 0, 20, 20)
	off := &driver.OffscreenCanvas{
		ID:      id,
		Version: 1,
		Size:    driver.Size{Width: 40, Height: 20},
		Scale:   2,
		Drawing: red,
	}
	src := driver.ImageSource{Offscreen: off, Size: off.Size, Version: off.Version}

	// Render a host drawing that draws the offscreen image at natural size.
	before := offscreenRenders.Load()
	host1 := driver.NewDrawing(nil).DrawImage(src, 0, 0)
	img1 := play(t, "offscreen-v1", host1, 40, 20, true)
	if got := offscreenRenders.Load() - before; got != 1 {
		t.Fatalf("first draw offscreen renders = %d, want 1", got)
	}

	// Verify pixels: left half red, right half blue.
	pxLeft := img1.RGBAAt(10*canvastest.Scale, 10*canvastest.Scale)
	pxRight := img1.RGBAAt(30*canvastest.Scale, 10*canvastest.Scale)
	if pxLeft != (color.RGBA{R: 0xff, G: 0, B: 0, A: 0xff}) {
		t.Fatalf("left pixel = %v, want red", pxLeft)
	}
	if pxRight != (color.RGBA{R: 0, G: 0, B: 0xff, A: 0xff}) {
		t.Fatalf("right pixel = %v, want blue", pxRight)
	}

	// Drawing the same (ID, Version) across 5 subsequent frames (including
	// DrawImageFrom cropping the right half) must hit the cache with zero
	// additional offscreen renders.
	for range 4 {
		play(t, "offscreen-v1-cached", host1, 40, 20, true)
	}
	crop := driver.NewDrawing(nil).DrawImageFrom(src, 20, 0, 20, 20, 0, 0, 40, 20)
	imgCrop := play(t, "offscreen-v1-crop", crop, 40, 20, true)
	if got := offscreenRenders.Load() - before; got != 1 {
		t.Fatalf("cached frames triggered re-render: renders = %d, want 1", got)
	}
	pxCrop := imgCrop.RGBAAt(10*canvastest.Scale, 10*canvastest.Scale)
	if pxCrop != (color.RGBA{R: 0, G: 0, B: 0xff, A: 0xff}) {
		t.Fatalf("cropped right-half pixel = %v, want blue", pxCrop)
	}

	// Bumping Version (Invalidate) triggers one re-render with the new content.
	green := driver.NewDrawing(nil).SetFillColor(driver.RGB(0x00ff00)).FillRect(0, 0, 40, 20)
	off2 := &driver.OffscreenCanvas{
		ID:      id,
		Version: 2,
		Size:    driver.Size{Width: 40, Height: 20},
		Scale:   2,
		Drawing: green,
	}
	src2 := driver.ImageSource{Offscreen: off2, Size: off2.Size, Version: off2.Version}
	img2 := play(t, "offscreen-v2", driver.NewDrawing(nil).DrawImage(src2, 0, 0), 40, 20, true)
	if got := offscreenRenders.Load() - before; got != 2 {
		t.Fatalf("after version bump renders = %d, want 2", got)
	}
	pxGreen := img2.RGBAAt(10*canvastest.Scale, 10*canvastest.Scale)
	if pxGreen != (color.RGBA{R: 0, G: 0xff, B: 0, A: 0xff}) {
		t.Fatalf("v2 pixel = %v, want green", pxGreen)
	}

	// Release drops the cache entry cleanly.
	ReleaseOffscreen(id)
	offscreenMu.Lock()
	_, stillCached := offscreenCache[id]
	offscreenMu.Unlock()
	if stillCached {
		t.Fatal("ReleaseOffscreen did not remove cache entry")
	}
}

func readCGImageRGBA(img uintptr) *image.RGBA {
	w, h := int(cgImageGetWidth(img)), int(cgImageGetHeight(img))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	const premultipliedLast = 1
	ctx := cgOffscreenBitmapCreate(uintptr(unsafe.Pointer(&out.Pix[0])), uintptr(w), uintptr(h), 8, uintptr(out.Stride), srgb, premultipliedLast)
	if ctx == 0 {
		return out
	}
	cgContextDrawImage(ctx, Rect(0, 0, float64(w), float64(h)), img)
	Release(ctx)
	runtime.KeepAlive(out)
	return out
}

func TestRasterizeDrawing(t *testing.T) {
	if got := RasterizeDrawing(nil, 24, 24, 2); got != 0 {
		Release(got)
		t.Fatal("nil drawing produced a CGImage")
	}
	if got := RasterizeDrawing(&driver.Drawing{}, 24, 24, 2); got != 0 {
		Release(got)
		t.Fatal("empty drawing produced a CGImage")
	}
	svg, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="0" y="0" width="24" height="6" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	d := &driver.Drawing{}
	svg.Draw(d, driver.Size{Width: 24, Height: 24}, driver.RGB(0xffffff))

	if got := RasterizeDrawing(d, 0, 24, 2); got != 0 {
		Release(got)
		t.Fatal("zero width produced a CGImage")
	}
	if got := RasterizeDrawing(d, math.NaN(), 24, 2); got != 0 {
		Release(got)
		t.Fatal("NaN width produced a CGImage")
	}

	for _, scale := range []float64{1, 2, 3} {
		cg := RasterizeDrawing(d, 24, 24, scale)
		if cg == 0 {
			t.Fatalf("scale %v: returned 0", scale)
		}
		want := int(24 * scale)
		if w, h := int(cgImageGetWidth(cg)), int(cgImageGetHeight(cg)); w != want || h != want {
			Release(cg)
			t.Fatalf("scale %v: CGImage size = %dx%d, want %dx%d", scale, w, h, want, want)
		}
		rgba := readCGImageRGBA(cg)
		Release(cg)
		// Top band must be opaque white and bottom area transparent, verifying
		// top-down orientation and transparent template background.
		top := rgba.RGBAAt(int(12*scale), int(2*scale))
		if top != (color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}) {
			t.Fatalf("scale %v: top pixel = %v, want opaque white", scale, top)
		}
		bottom := rgba.RGBAAt(int(12*scale), int(18*scale))
		if bottom.A != 0 {
			t.Fatalf("scale %v: bottom pixel = %v, want transparent", scale, bottom)
		}
	}

	// Non-positive scale falls back to 1x rather than failing.
	if cg := RasterizeDrawing(d, 24, 24, 0); cg == 0 {
		t.Fatal("zero scale returned 0, want 1x fallback")
	} else {
		if w, h := int(cgImageGetWidth(cg)), int(cgImageGetHeight(cg)); w != 24 || h != 24 {
			Release(cg)
			t.Fatalf("zero scale fallback size = %dx%d, want 24x24", w, h)
		}
		Release(cg)
	}
}
