package harmonyos

import (
	"fmt"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHarmonyOSCanvasImageCacheBoundsLRUPinsAndClamping(t *testing.T) {
	var released []string
	var c canvasImageCache[string]
	c.onEvict = func(pm string) { released = append(released, pm) }

	// Byte budget allows 2 x 100B unkept PixelMaps.
	c.setLimit(200)
	c.addPending("a")
	c.complete("a", "pm-a", 100)
	c.endPass()
	c.addPending("b")
	c.complete("b", "pm-b", 100)
	c.endPass()

	// Touch "a" so "b" is LRU, then decode "c".
	if got, ok := c.get("a"); !ok || got != "pm-a" {
		t.Fatalf("get(a) = %q, %v", got, ok)
	}
	c.endPass()
	c.addPending("c")
	c.complete("c", "pm-c", 100)
	c.endPass()

	if !slices.Equal(released, []string{"pm-b"}) {
		t.Fatalf("released PixelMaps = %v, want [pm-b]", released)
	}

	// KeepImage pin protects "a" across negative->0 clamping.
	c.keep("a")
	c.setLimit(-100)
	if c.effectiveLimit() != 0 {
		t.Fatalf("negative limit = %d, want 0", c.effectiveLimit())
	}
	if !slices.Equal(released, []string{"pm-b", "pm-c"}) {
		t.Fatalf("released after zero limit = %v, want [pm-b pm-c]", released)
	}
	if !c.has("a") {
		t.Fatal("kept image was evicted at zero limit")
	}

	// Failed entries are evictable without calling PixelMap release.
	c.setLimit(200)
	c.addPending("bad")
	c.fail("bad")
	c.endPass()
	c.setLimit(0)
	if c.has("bad") {
		t.Fatal("failed entry was not evicted at zero limit")
	}
	if len(released) != 2 {
		t.Fatalf("failed entry triggered PixelMap release: %v", released)
	}

	// Releasing pin trims "a" immediately.
	c.release("a")
	if c.len() != 0 || !slices.Equal(released, []string{"pm-b", "pm-c", "pm-a"}) {
		t.Fatalf("after release: len=%d released=%v", c.len(), released)
	}

	// Count ceiling (64) holds under default large byte budget.
	c.setLimit(driver.DefaultImageCacheLimit)
	for i := range maxCanvasImages + 20 {
		k := fmt.Sprintf("pm-%d", i)
		c.addPending(k)
		c.complete(k, k, 16)
		c.endPass()
		if c.len() > maxCanvasImages {
			t.Fatalf("cache len = %d, want <= %d", c.len(), maxCanvasImages)
		}
	}
}
