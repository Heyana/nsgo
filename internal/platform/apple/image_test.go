//go:build darwin

package apple

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/oneo-me/nsgo/internal/driver"
)

func TestImageSizeReadsTheHeader(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 7, 3)))
	if got, err := imageSize(b.Bytes()); err != nil || got != (driver.Size{Width: 7, Height: 3}) {
		t.Fatalf("PNG size = %v, %v", got, err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"/>`)
	if got, err := imageSize(svg); err != nil || got != (driver.Size{Width: 20, Height: 10}) {
		t.Fatalf("SVG size = %v, %v", got, err)
	}
	if _, err := imageSize([]byte("not an image")); err == nil {
		t.Fatal("no error for bytes that are no image")
	}
}

func TestDecodePointsPerPixel(t *testing.T) {
	view := CGSize{Width: 100, Height: 100}
	for _, c := range []struct {
		mode driver.ResizeMode
		w, h float64
		want float64
	}{
		{driver.Center, 400, 200, 0.5}, // a 2x file at its natural size
		{driver.ScaleDown, 400, 200, 0.25},
		{driver.ScaleDown, 100, 50, 0.5},
		{driver.Contain, 400, 200, 0.25},
		{driver.Cover, 400, 200, 0.5},
	} {
		r := request{mode: c.mode, view: view, scale: 2}
		if got := r.pointsPerPixel(c.w, c.h); got != c.want {
			t.Errorf("%v %gx%g: %g points per pixel, want %g", c.mode, c.w, c.h, got, c.want)
		}
	}
}

// A closed image view must let the bitmap go: Core Animation retains what a
// layer's contents points at, so dropping only the Go side leaves the
// picture alive for as long as the layer is cached. Scrolling a grid of
// 202 pictures kept all of them that way (112.4 MB of CG raster data).
func TestCloseDropsThePictureLayersContents(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	if layer == 0 {
		t.Skip("no CALayer without an app")
	}
	defer layer.Send(selRelease)

	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	d, err := decode(b.Bytes(), request{scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	if d.image == 0 {
		t.Fatal("decode gave no image")
	}

	iv := &ImageView{picture: layer, shown: d, image: d.image}
	layer.Send(selSetContents, d.image)
	if got := uintptr(layer.Send(sel("contents"))); got != d.image {
		t.Fatalf("contents = %v before Close, want %v", got, d.image)
	}

	iv.Close()

	if got := uintptr(layer.Send(sel("contents"))); got != 0 {
		t.Fatalf("contents = %v after Close, want nil", got)
	}
}

// Every image view that comes into view starts a decode of its own. Without
// a bound, a fast scroll on a 1200-cell grid ran 25 at once, and that is
// where the process's peak came from (1.0 GB measured).
func TestDecodeSlotsBoundHowManyDecodeAtOnce(t *testing.T) {
	const workers = 40
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		now  int
		peak int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, ok := acquireDecodeSlot(context.Background())
			if !ok {
				return
			}
			defer release()
			mu.Lock()
			now++
			if now > peak {
				peak = now
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			now--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if limit := cap(decodeSlots); peak > limit {
		t.Fatalf("peak concurrency = %d, want at most %d", peak, limit)
	}
	if peak < 2 {
		t.Fatalf("peak concurrency = %d, want the slots used at once", peak)
	}
}

// A cancelled context takes no slot: a scroll past hundreds of pictures
// cancels most of their loads, and a cancelled one must not queue behind
// the bound.
func TestCancelledDecodeTakesNoSlot(t *testing.T) {
	for i := 0; i < cap(decodeSlots); i++ {
		decodeSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(decodeSlots); i++ {
			<-decodeSlots
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := acquireDecodeSlot(ctx); ok {
		t.Fatal("a cancelled decode took a slot")
	}
}

// The allocator keeps freed pages until it is asked, so closed pictures are
// counted and given back once enough have gone: 115.6 MB of empty malloc
// pool stayed resident without this.
func TestNoteClosedCountsUntilItGivesBack(t *testing.T) {
	giveBackMu.Lock()
	was := giveBackPending
	giveBackPending = 0
	giveBackMu.Unlock()
	t.Cleanup(func() {
		giveBackMu.Lock()
		giveBackPending = was
		giveBackMu.Unlock()
	})

	one := decoded{width: 1000, height: 1000} // 4,000,000 bytes
	noteClosed(one)
	giveBackMu.Lock()
	got := giveBackPending
	giveBackMu.Unlock()
	if got != 4_000_000 {
		t.Fatalf("pending after one = %d, want %d", got, int64(4_000_000))
	}

	for i := 0; i < int(giveBackAfter/(4_000_000))+1; i++ {
		noteClosed(one)
	}
	giveBackMu.Lock()
	got = giveBackPending
	giveBackMu.Unlock()
	if got >= int64(giveBackAfter) {
		t.Fatalf("pending = %d, want it reset after giving back", got)
	}

	noteClosed(decoded{})
	giveBackMu.Lock()
	after := giveBackPending
	giveBackMu.Unlock()
	if after != got {
		t.Fatalf("an empty picture changed pending: %d -> %d", got, after)
	}
}
