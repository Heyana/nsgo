package windows

import (
	"fmt"
	"slices"
	"testing"
)

func TestShaderTextureCacheBoundsLRUPinsAndRelease(t *testing.T) {
	type fakeCOM struct {
		name     string
		released bool
	}
	var freed []string
	var c shaderTextureCache[*fakeCOM]
	c.release = func(o *fakeCOM) {
		if o.released {
			t.Errorf("double COM release for %s", o.name)
		}
		o.released = true
		freed = append(freed, o.name)
	}
	kept := map[string]bool{}
	isKept := func(k string) bool { return kept[k] }

	// Byte budget allows 2 x 100B unkept textures.
	c.setLimit(200, isKept)
	a := &fakeCOM{name: "a"}
	b := &fakeCOM{name: "b"}
	d := &fakeCOM{name: "d"}
	c.put("a", a, 100, isKept)
	c.endPass(isKept)
	c.put("b", b, 100, isKept)
	c.endPass(isKept)

	// Touch "a" so "b" is LRU, then insert "d".
	if got, ok := c.get("a"); !ok || got != a {
		t.Fatalf("get(a) = %v, %v", got, ok)
	}
	c.endPass(isKept)
	c.put("d", d, 100, isKept)
	c.endPass(isKept)

	if !b.released || a.released || d.released {
		t.Fatalf("unexpected release state: a=%v b=%v d=%v", a.released, b.released, d.released)
	}
	if !slices.Equal(freed, []string{"b"}) {
		t.Fatalf("freed = %v, want [b]", freed)
	}

	// KeepImage pin protects "a" even when budget shrinks to 0 (clamped from negative).
	kept["a"] = true
	c.setLimit(-50, isKept)
	if c.effectiveLimit() != 0 {
		t.Fatalf("limit = %d, want 0", c.effectiveLimit())
	}
	if a.released || !d.released {
		t.Fatalf("after zero limit: a.released=%v d.released=%v", a.released, d.released)
	}

	// Current render pass keeps textures bound until endPass even at limit 0.
	e := &fakeCOM{name: "e"}
	f := &fakeCOM{name: "f"}
	c.put("e", e, 64, isKept)
	c.put("f", f, 64, isKept)
	if e.released || f.released {
		t.Fatal("textures used in active shader pass were evicted before draw finished")
	}
	c.endPass(isKept)
	if !e.released || !f.released {
		t.Fatal("unkept textures were not released after endPass at limit 0")
	}

	// Releasing pin trims "a" immediately.
	delete(kept, "a")
	c.trim(isKept)
	if !a.released || c.len() != 0 {
		t.Fatalf("after unpin: a.released=%v len=%d", a.released, c.len())
	}

	// Count ceiling (64) holds under default large byte budget.
	c.setLimit(32<<20, isKept)
	for i := range maxShaderTextures + 20 {
		key := fmt.Sprintf("tex-%d", i)
		c.put(key, &fakeCOM{name: key}, 16, isKept)
		c.endPass(isKept)
		if c.len() > maxShaderTextures {
			t.Fatalf("cache len = %d, want <= %d", c.len(), maxShaderTextures)
		}
	}
	beforeClear := len(freed)
	c.clear()
	if c.len() != 0 || len(freed)-beforeClear != maxShaderTextures {
		t.Fatalf("clear freed %d entries, want %d", len(freed)-beforeClear, maxShaderTextures)
	}
}
