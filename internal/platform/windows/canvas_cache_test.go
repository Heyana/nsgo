//go:build windows

package windows

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

func TestCanvasImageCacheBoundAndReload(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(1000)
	load := func(src driver.ImageSource) *imageSlot {
		t.Helper()
		s := w.canvasImage(src)
		for deadline := time.Now().Add(5 * time.Second); s.load != nil; {
			w.runTasks()
			if time.Now().After(deadline) {
				t.Fatal("image load timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if s.pic == nil || s.bitmapOf(g.dc, 0) == nil {
			t.Fatal("no decoded bitmap")
		}
		return s
	}
	first := load(sources[0])
	old := load(sources[1])
	for i := 2; i < len(sources); i++ {
		load(sources[i])
		if i%32 == 0 {
			w.canvasImage(sources[0])
		}
		if len(w.canvasImages) > maxCanvasImages {
			t.Fatalf("%d cached images", len(w.canvasImages))
		}
	}
	if w.canvasImages[sources[0].Key()] != first {
		t.Fatal("recently drawn image evicted")
	}
	if old.pic != nil || old.bitmap != nil || old.load != nil {
		t.Fatal("evicted resources retained")
	}
	reloaded := load(sources[1])
	if reloaded == old {
		t.Fatal("evicted image slot reused")
	}
	d := (&driver.Drawing{}).DrawImageIn(sources[1], 0, 0, 2, 2)
	img := drawImage(t, g, 2, 2, func(p *painter) { p.canvas(&view{drawing: d}, 2, 2) }, w)
	if got := img.RGBAAt(1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("reloaded pixel %v", got)
	}
	pixels := w.canvasImage(driver.ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2))})
	if pixels.pic == nil {
		t.Fatal("pixel frame did not draw")
	}
	w.closeCanvasImages()
	if pixels.pic != nil || len(w.canvasFrames) != 0 || w.canvasFramePixels != 0 {
		t.Fatal("closed cache retained pixel frames")
	}
	if reloaded.pic != nil || reloaded.bitmap != nil || len(w.canvasImages) != 0 || w.canvasImage(sources[1]) != nil {
		t.Fatal("closed cache retained resources")
	}
}

// Loads under way are never evicted, even by later paints: the cache exceeds its bound only by them, and shrinks back once they have loaded. A result queued when the window closes revives nothing.
func TestCanvasImageCacheKeepsLoadsUnderWayAndDropsQueuedResults(t *testing.T) {
	if _, err := devices(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(2 * maxCanvasImages)
	slots := map[*imageSlot]bool{}
	for _, src := range sources {
		w.endCanvasPass()
		slots[w.canvasImage(src)] = true
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
	queued := w.canvasImage(canvastest.CacheSources(1)[0])
	if len(w.canvasImages) != maxCanvasImages {
		t.Fatalf("%d cached once the loads under way loaded", len(w.canvasImages))
	}
	for s := range slots {
		if w.canvasImages[s.src.Key()] != s && (s.pic != nil || s.bitmap != nil) {
			t.Fatal("an evicted image kept its resources")
		}
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
	w.closeCanvasImages()
	w.runTasks()
	if len(w.canvasImages) != 0 || queued.pic != nil {
		t.Fatal("closed cache revived")
	}
}

// A paint drawing more sources than the bound keeps them all: evicting one would load it again at every paint, forever.
func TestCanvasImageCacheKeepsAWidePaint(t *testing.T) {
	if _, err := devices(); err != nil {
		t.Skip(err)
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(2 * maxCanvasImages)
	slots := map[*imageSlot]bool{}
	for _, src := range sources {
		slots[w.canvasImage(src)] = true
	}
	if len(w.canvasImages) != len(sources) {
		t.Fatalf("a paint of %d sources kept %d", len(sources), len(w.canvasImages))
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
		if s := w.canvasImage(src); !slots[s] || s.pic == nil {
			t.Fatal("the next paint loaded a wide paint's image anew")
		}
	}
	w.runTasks()
	for _, src := range canvastest.CacheSources(maxCanvasImages) {
		w.canvasImage(src)
		w.endCanvasPass()
	}
	if len(w.canvasImages) > maxCanvasImages {
		t.Fatalf("%d cached after later paints", len(w.canvasImages))
	}
	for s := range slots {
		if s.pic != nil || s.bitmap != nil {
			t.Fatal("a wide paint's evicted image kept its resources")
		}
	}
}

func TestCanvasImageCacheKeepPinsOutOfBudget(t *testing.T) {
	if _, err := devices(); err != nil {
		t.Skip(err)
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	defer w.closeCanvasImages()
	load := func(src driver.ImageSource) *imageSlot {
		t.Helper()
		s := w.canvasImage(src)
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
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	defer w.closeCanvasImages()
	sources := canvastest.CacheSources(5)
	limit := 2 * int64(len(sources[len(sources)-1].Data))
	w.setCanvasImageCacheLimit(limit)
	load := func(src driver.ImageSource) *imageSlot {
		t.Helper()
		s := w.canvasImage(src)
		for deadline := time.Now().Add(5 * time.Second); s.load != nil; {
			w.runTasks()
			if time.Now().After(deadline) {
				t.Fatal("image load timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if s.pic == nil || s.bitmapOf(g.dc, 0) == nil {
			t.Fatal("no decoded native image")
		}
		w.endCanvasPass()
		return s
	}
	freed := func(slot *imageSlot) bool { return slot.pic == nil && slot.bitmap == nil }
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
