package miniprogram

import (
	"fmt"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniProgramCanvasImageCacheBoundsLRUPinsAndClamping(t *testing.T) {
	var evicted []string
	var c canvasImageCache[string]
	c.onEvict = func(v string) { evicted = append(evicted, v) }

	// Byte budget holds 2 x 100B unkept entries.
	c.setLimit(200)
	c.addPending("a", "mp-a", 100)
	c.complete("a", 100)
	c.endPass()
	c.addPending("b", "mp-b", 100)
	c.complete("b", 100)
	c.endPass()

	// Touch "a" so "b" is LRU, then add "c".
	if got, ok := c.get("a"); !ok || got != "mp-a" {
		t.Fatalf("get(a) = %q, %v", got, ok)
	}
	c.endPass()
	c.addPending("c", "mp-c", 100)
	c.complete("c", 100)
	c.endPass()

	if !slices.Equal(evicted, []string{"mp-b"}) {
		t.Fatalf("evicted = %v, want [mp-b]", evicted)
	}

	// KeepImage pin protects "a" across negative->0 clamping.
	c.keep("a")
	c.setLimit(-10)
	if c.effectiveLimit() != 0 {
		t.Fatalf("negative limit = %d, want 0", c.effectiveLimit())
	}
	if !slices.Equal(evicted, []string{"mp-b", "mp-c"}) {
		t.Fatalf("evicted after zero limit = %v, want [mp-b mp-c]", evicted)
	}
	if _, ok := c.peek("a"); !ok {
		t.Fatal("kept image was evicted at zero limit")
	}

	// Release pin trims "a" immediately.
	c.release("a")
	if c.len() != 0 || !slices.Equal(evicted, []string{"mp-b", "mp-c", "mp-a"}) {
		t.Fatalf("after release: len=%d evicted=%v", c.len(), evicted)
	}

	// Count ceiling (64) holds under default large byte budget.
	c.setLimit(driver.DefaultImageCacheLimit)
	for i := range maxCanvasImages + 20 {
		k := fmt.Sprintf("img-%d", i)
		c.addPending(k, k, 16)
		c.complete(k, 16)
		c.endPass()
		if c.len() > maxCanvasImages {
			t.Fatalf("cache len = %d, want <= %d", c.len(), maxCanvasImages)
		}
	}
}

func TestMounterRoutesImageCacheItemsToService(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{
		Items: []driver.MountItem{
			{Kind: driver.ItemKeepImage, Value: "asset:/atlas.png"},
			{Kind: driver.ItemImageCacheLimit, Bytes: 4096},
			{Kind: driver.ItemReleaseImage, Value: "asset:/atlas.png"},
		},
	})
	want := []driver.ItemKind{driver.ItemKeepImage, driver.ItemImageCacheLimit, driver.ItemReleaseImage}
	if !slices.Equal(h.services, want) {
		t.Fatalf("service kinds = %v, want %v", h.services, want)
	}
}

func TestMiniProgramCanvasImageCacheCountsMeshTexturePixels(t *testing.T) {
	var evicted []string
	var c canvasImageCache[string]
	c.onEvict = func(v string) { evicted = append(evicted, v) }
	c.setLimit(300)
	c.addPending("a", "mp-a", 100)
	c.complete("a", 100)
	c.endPass()
	c.addPending("b", "mp-b", 100)
	c.complete("b", 100)

	// A mesh reads b's pixels in the pass that draws it: b stays, and the older a goes to make room.
	c.grow("b", 150)
	if !slices.Equal(evicted, []string{"mp-a"}) {
		t.Fatalf("evicted = %v, want [mp-a]", evicted)
	}
	if _, ok := c.peek("b"); !ok {
		t.Fatal("the image a mesh is drawing was evicted")
	}
	c.grow("missing", 1000)
	c.grow("b", -5)
	if _, bytes := c.unkeptStats(); bytes != 250 {
		t.Fatalf("bytes = %d, want 250", bytes)
	}
}
