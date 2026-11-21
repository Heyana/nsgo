//go:build darwin

package apple

import (
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

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
