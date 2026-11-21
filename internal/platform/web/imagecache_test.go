package web

import (
	"fmt"
	"slices"
	"testing"
)

func TestWebCanvasImageCacheBoundsLRUPinsAndClamping(t *testing.T) {
	var evicted []string
	var c boundedImageCache[string]
	c.onEvict = func(v string) { evicted = append(evicted, v) }

	// Byte budget holds 2 x 100B unkept entries.
	c.setLimit(200)
	c.addPending("a", "img-a", 100)
	c.complete("a", 100)
	c.endPass()
	c.addPending("b", "img-b", 100)
	c.complete("b", 100)
	c.endPass()

	// Touch "a" so "b" is LRU, then add "c".
	if got, ok := c.get("a"); !ok || got != "img-a" {
		t.Fatalf("get(a) = %q, %v", got, ok)
	}
	c.endPass()
	c.addPending("c", "img-c", 100)
	c.complete("c", 100)
	c.endPass()

	if !slices.Equal(evicted, []string{"img-b"}) {
		t.Fatalf("evicted = %v, want [img-b]", evicted)
	}

	// KeepImage pin protects "a" across negative->0 clamping and multiple releases.
	c.keep("a")
	c.keep("a")
	c.setLimit(-99)
	if c.effectiveLimit() != 0 {
		t.Fatalf("negative limit = %d, want 0", c.effectiveLimit())
	}
	if !slices.Equal(evicted, []string{"img-b", "img-c"}) {
		t.Fatalf("evicted after zero limit = %v, want [img-b img-c]", evicted)
	}

	// Pending loads are never evicted mid-load even at limit 0.
	c.addPending("inflight", "img-inflight", 50)
	c.endPass()
	if _, ok := c.peek("inflight"); !ok {
		t.Fatal("pending load was evicted before completion")
	}
	c.complete("inflight", 50)
	c.endPass()
	if _, ok := c.peek("inflight"); ok {
		t.Fatal("completed unkept load stayed at zero limit after endPass")
	}

	// Releasing first pin keeps "a"; releasing second evicts "a".
	c.release("a")
	if _, ok := c.peek("a"); !ok {
		t.Fatal("a evicted while second pin remained")
	}
	c.release("a")
	if _, ok := c.peek("a"); ok || c.len() != 0 {
		t.Fatalf("released a not evicted: len=%d", c.len())
	}

	// Count ceiling (64) holds under default large byte budget.
	c.setLimit(32 << 20)
	for i := range maxCanvasImages + 20 {
		k := fmt.Sprintf("k-%d", i)
		c.put(k, k, 16)
		c.endPass()
		if c.len() > maxCanvasImages {
			t.Fatalf("cache len = %d, want <= %d", c.len(), maxCanvasImages)
		}
	}
}
