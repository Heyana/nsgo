//go:build linux && !android

package linux

import (
	"image"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload/imageloadtest"
)

func TestCanvasImageCacheBoundAndReload(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(1000)
	load := func(src driver.ImageSource) *imageSlot {
		t.Helper()
		s := w.canvasImage(src, &view{deleted: true})
		for deadline := time.Now().Add(5 * time.Second); s.load != nil; {
			w.runTasks()
			if time.Now().After(deadline) {
				t.Fatal("image load timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if s.pic == nil || s.surfaceOf(0) == 0 {
			t.Fatal("no decoded image surface")
		}
		return s
	}
	first := load(sources[0])
	old := load(sources[1])
	for i := 2; i < len(sources); i++ {
		load(sources[i])
		if i%32 == 0 {
			w.canvasImage(sources[0], &view{deleted: true})
		}
		if len(w.canvasImages) > maxCanvasImages {
			t.Fatalf("%d cached images", len(w.canvasImages))
		}
	}
	if w.canvasImages[sources[0].Key()] != first {
		t.Fatal("recently drawn image evicted")
	}
	if old.pic != nil || old.surface != 0 || old.load != nil {
		t.Fatal("evicted resources retained")
	}
	reloaded := load(sources[1])
	if reloaded == old {
		t.Fatal("evicted image slot reused")
	}
	d := (&driver.Drawing{}).DrawImageIn(sources[1], 0, 0, 2, 2)
	surface := cairoImageSurfaceCreate(0, 2, 2)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	w.playCanvas(&view{drawing: d}, cr, rect{w: 2, h: 2})
	cairoDestroy(cr)
	cairoSurfaceFlush(surface)
	pix := unsafe.Slice((*byte)(cPointer(cairoImageSurfaceGetData(surface))), 4)
	if pix[0] != 0 || pix[1] != 0 || pix[2] != 255 || pix[3] != 255 {
		t.Fatalf("reloaded pixel BGRA %v", pix)
	}
	pixels := w.canvasImage(driver.ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2))}, &view{deleted: true})
	if pixels.pic == nil {
		t.Fatal("pixel frame did not draw")
	}
	w.closeCanvasImages()
	if pixels.pic != nil || len(w.canvasFrames) != 0 || w.canvasFramePixels != 0 {
		t.Fatal("closed cache retained pixel frames")
	}
	if reloaded.pic != nil || reloaded.surface != 0 || len(w.canvasImages) != 0 || w.canvasImage(sources[1], &view{}) != nil {
		t.Fatal("closed cache retained resources")
	}
}

// Loads under way are never evicted, even by later frames: the cache exceeds its bound only by them, and shrinks back once they have loaded. A result queued when the window closes revives nothing.
func TestCanvasImageCacheKeepsLoadsUnderWayAndDropsQueuedResults(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(2 * maxCanvasImages)
	slots := map[*imageSlot]bool{}
	for _, src := range sources {
		w.endCanvasPass()
		slots[w.canvasImage(src, &view{deleted: true})] = true
	}
	if len(w.canvasImages) != len(sources) {
		t.Fatalf("%d of %d loads under way kept", len(w.canvasImages), len(sources))
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		w.runTasks()
		loaded := 0
		for s := range slots {
			if s.load == nil && s.pic != nil {
				loaded++
			}
		}
		if loaded == len(slots) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d loaded", loaded, len(slots))
		}
	}
	w.endCanvasPass()
	later := canvastest.CacheSources(1)[0]
	w.canvasImage(later, &view{deleted: true})
	if len(w.canvasImages) != maxCanvasImages || len(w.canvasWaiting) > maxCanvasImages {
		t.Fatalf("%d cached once the loads under way loaded", len(w.canvasImages))
	}
	// The paint's end is posted at once; the load's result follows it.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		w.mu.Lock()
		n := len(w.tasks)
		w.mu.Unlock()
		if n > 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("load did not post a result")
		}
	}
	queued := w.canvasImages[later.Key()]
	w.closeCanvasImages()
	w.runTasks()
	if len(w.canvasImages) != 0 || len(w.canvasWaiting) != 0 || queued.pic != nil {
		t.Fatal("closed cache revived")
	}
}

// Canvases drawn in frames of their own whose images together exceed the bound and load more slowly than a frame all get every image: a canvas's frame must not cancel another's loads, which would draw again and cancel the first's, forever.
func TestCanvasImageCacheCanvasesDrawnApartLoadSlowImages(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	var sources []driver.ImageSource
	for _, uri := range imageloadtest.Slow(t, 88, 150*time.Millisecond, canvastest.RedPNG()) {
		sources = append(sources, driver.ImageSource{URI: uri})
	}
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	defer w.closeCanvasImages()
	canvases := []struct {
		v       *view
		sources []driver.ImageSource
	}{{&view{}, sources[:40]}, {&view{}, sources[40:80]}, {&view{}, sources[80:]}}
	draws := 0
	draw := func(i int) {
		c := canvases[i]
		c.v.canvas.whole = false
		draws++
		for _, src := range c.sources {
			w.canvasImage(src, c.v)
		}
		// The frame ends before the next canvas draws in a frame of its own.
		w.runTasks()
	}
	for i := range canvases {
		draw(i)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		w.runTasks()
		for i, c := range canvases {
			if c.v.canvas.whole {
				draw(i)
			}
		}
		loaded := 0
		for _, src := range sources {
			if s := w.canvasImages[src.Key()]; s != nil && s.pic != nil {
				loaded++
			}
		}
		if loaded == len(sources) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d loaded after %d draws, %d cached", loaded, len(sources), draws, len(w.canvasImages))
		}
	}
	if draws > 3+len(sources) {
		t.Fatalf("%d draws for %d images", draws, len(sources))
	}
}

// A frame drawing more sources than the bound keeps them all: evicting one would load it again at every frame, forever.
func TestCanvasImageCacheKeepsAWideFrame(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(2 * maxCanvasImages)
	slots := map[*imageSlot]bool{}
	for _, src := range sources {
		slots[w.canvasImage(src, &view{deleted: true})] = true
	}
	if len(w.canvasImages) != len(sources) {
		t.Fatalf("a frame of %d sources kept %d", len(sources), len(w.canvasImages))
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		w.runTasks()
		loaded := 0
		for s := range slots {
			if s.load == nil && s.pic != nil {
				loaded++
			}
		}
		if loaded == len(slots) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d loaded", loaded, len(slots))
		}
	}
	for _, src := range sources {
		if s := w.canvasImage(src, &view{deleted: true}); !slots[s] || s.pic == nil {
			t.Fatal("the next frame loaded a wide frame's image anew")
		}
	}
	w.runTasks()
	for _, src := range canvastest.CacheSources(maxCanvasImages) {
		w.canvasImage(src, &view{deleted: true})
		w.endCanvasPass()
	}
	if len(w.canvasImages) > maxCanvasImages || len(w.canvasWaiting) > maxCanvasImages {
		t.Fatalf("%d cached after later frames", len(w.canvasImages))
	}
	for s := range slots {
		if s.pic != nil || s.surface != 0 {
			t.Fatal("a wide frame's evicted image kept its resources")
		}
	}
}

func TestCanvasImageCacheKeepPinsOutOfBudget(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	defer w.closeCanvasImages()
	load := func(src driver.ImageSource) *imageSlot {
		t.Helper()
		s := w.canvasImage(src, &view{deleted: true})
		for deadline := time.Now().Add(5 * time.Second); s.load != nil; {
			w.runTasks()
			if time.Now().After(deadline) {
				t.Fatal("image load timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if s.pic == nil {
			t.Fatal("no decoded image")
		}
		w.endCanvasPass()
		return s
	}

	sources := canvastest.CacheSources(200)
	atlas := sources[0]
	w.keepCanvasImage(atlas.Key())
	w.keepCanvasImage(atlas.Key())
	atlasSlot := load(atlas)

	for i := 1; i <= 100; i++ {
		load(sources[i])
		if unkept := w.unkeptCanvasImages(); unkept > maxCanvasImages {
			t.Fatalf("%d unkept images, want <= %d", unkept, maxCanvasImages)
		}
	}
	if w.canvasImages[atlas.Key()] != atlasSlot || atlasSlot.pic == nil {
		t.Fatal("kept atlas was evicted")
	}
	w.releaseCanvasImage(atlas.Key())
	load(sources[101])
	if w.canvasImages[atlas.Key()] != atlasSlot || atlasSlot.pic == nil {
		t.Fatal("atlas evicted while second pin remained")
	}
	w.releaseCanvasImage(atlas.Key())
	w.releaseCanvasImage(atlas.Key())
	if w.canvasImages[atlas.Key()] != nil || atlasSlot.pic != nil {
		t.Fatal("released old atlas was not evicted and freed")
	}
	if len(w.canvasImages) != maxCanvasImages {
		t.Fatalf("%d cached after release, want %d", len(w.canvasImages), maxCanvasImages)
	}

	keptMany := sources[102 : 102+maxCanvasImages+10]
	keptSlots := make([]*imageSlot, len(keptMany))
	for i, src := range keptMany {
		w.keepCanvasImage(src.Key())
		keptSlots[i] = load(src)
	}
	for i := 1; i <= maxCanvasImages+5; i++ {
		load(sources[i])
		if unkept := w.unkeptCanvasImages(); unkept > maxCanvasImages {
			t.Fatalf("%d unkept images with %d kept, want <= %d", unkept, len(keptMany), maxCanvasImages)
		}
	}
	for i, src := range keptMany {
		if w.canvasImages[src.Key()] != keptSlots[i] || keptSlots[i].pic == nil {
			t.Fatalf("kept source %d was evicted", i)
		}
	}
	for _, src := range keptMany {
		w.releaseCanvasImage(src.Key())
	}
	if len(w.canvasImages) != maxCanvasImages {
		t.Fatalf("%d cached after releasing %d kept images, want %d", len(w.canvasImages), len(keptMany), maxCanvasImages)
	}
}

// Byte pressure releases the decoded native resource even below the source-count bound.
func TestCanvasImageCacheByteBudgetAndReconfiguration(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(5)
	limit := 2 * int64(len(sources[len(sources)-1].Data))
	w.setCanvasImageCacheLimit(limit)
	load := func(src driver.ImageSource) *imageSlot {
		t.Helper()
		s := w.canvasImage(src, &view{deleted: true})
		for deadline := time.Now().Add(5 * time.Second); s.load != nil; {
			w.runTasks()
			if time.Now().After(deadline) {
				t.Fatal("image load timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if s.pic == nil || s.surfaceOf(0) == 0 {
			t.Fatal("no decoded native image")
		}
		w.endCanvasPass()
		return s
	}
	freed := func(slot *imageSlot) bool { return slot.pic == nil && slot.surface == 0 }
	w.keepCanvasImage(sources[0].Key())
	pinned := load(sources[0])
	old := load(sources[1])
	for _, src := range sources[2:] {
		load(src)
		_, bytes := w.unkeptCanvasStats()
		if bytes > limit {
			t.Fatalf("%d unkept bytes above %d", bytes, limit)
		}
	}
	if w.canvasImages[sources[1].Key()] != nil || !freed(old) {
		t.Fatal("byte pressure retained the old native resource")
	}
	if w.canvasImages[sources[0].Key()] != pinned || freed(pinned) {
		t.Fatal("kept image was evicted")
	}
	w.setCanvasImageCacheLimit(0)
	if len(w.canvasImages) != 1 {
		t.Fatalf("zero limit retained %d images", len(w.canvasImages))
	}
	w.releaseCanvasImage(sources[0].Key())
	if len(w.canvasImages) != 0 || !freed(pinned) {
		t.Fatal("last pin release retained the native resource")
	}
}
