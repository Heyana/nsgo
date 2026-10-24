//go:build darwin

package apple

import "testing"

// The cache evicts the least recently used entry, not everything: a font
// or style the screen keeps asking for stays while a cold one goes.
func TestRefCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := newRefCache[int](2)
	defer c.clear()
	a, b, d := CFString("a"), CFString("b"), CFString("d")
	c.put(1, a)
	c.put(2, b)

	touch := c.get(1)
	if touch != a {
		t.Fatalf("get(1) returned %#x, want %#x", touch, a)
	}
	Release(touch)

	c.put(3, d)
	kept := c.get(1)
	if kept != a {
		t.Fatalf("the recently used entry was evicted: get(1) returned %#x, want %#x", kept, a)
	}
	Release(kept)
	if evicted := c.get(2); evicted != 0 {
		Release(evicted)
		t.Fatalf("the least recently used entry survived: get(2) returned %#x", evicted)
	}
	if n := c.len(); n != 2 {
		t.Fatalf("the cache holds %d entries, want 2", n)
	}
}
