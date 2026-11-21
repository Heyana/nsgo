package linux

import (
	"fmt"
	"slices"
	"testing"
)

func TestGLTextureCacheBoundsLRUPinsAndDelete(t *testing.T) {
	var deleted []uint32
	var c glTextureCache[uint32]
	c.release = func(id uint32) {
		deleted = append(deleted, id)
	}
	kept := map[string]bool{}
	isKept := func(k string) bool { return kept[k] }

	// Byte budget allows 2 x 100B unkept GL textures.
	c.setLimit(200, isKept)
	c.put("a", 1, 100, isKept)
	c.endPass(isKept)
	c.put("b", 2, 100, isKept)
	c.endPass(isKept)

	// Touch "a" so "b" (texture ID 2) is LRU.
	if got, ok := c.get("a"); !ok || got != 1 {
		t.Fatalf("get(a) = %d, %v", got, ok)
	}
	c.endPass(isKept)
	c.put("c", 3, 100, isKept)
	c.endPass(isKept)

	if !slices.Equal(deleted, []uint32{2}) {
		t.Fatalf("deleted GL textures = %v, want [2]", deleted)
	}

	// KeepImage pin protects "a" across negative->0 clamping.
	kept["a"] = true
	c.setLimit(-1, isKept)
	if c.effectiveLimit() != 0 {
		t.Fatalf("limit = %d, want 0", c.effectiveLimit())
	}
	if !slices.Equal(deleted, []uint32{2, 3}) {
		t.Fatalf("deleted after zero limit = %v, want [2 3]", deleted)
	}

	// Textures bound during one draw pass survive until endPass.
	c.put("d", 4, 64, isKept)
	c.put("e", 5, 64, isKept)
	if len(deleted) != 2 {
		t.Fatalf("evicted active pass textures: %v", deleted)
	}
	c.endPass(isKept)
	if len(deleted) != 4 {
		t.Fatalf("active pass textures not deleted after endPass: %v", deleted)
	}

	// Releasing pin deletes "a" immediately.
	delete(kept, "a")
	c.trim(isKept)
	if c.len() != 0 || !slices.Contains(deleted, 1) {
		t.Fatalf("after unpin: len=%d deleted=%v", c.len(), deleted)
	}

	// Count ceiling (64) holds under default budget.
	c.setLimit(32<<20, isKept)
	for i := range maxShaderTextures + 15 {
		c.put(fmt.Sprintf("k-%d", i), uint32(100+i), 16, isKept)
		c.endPass(isKept)
		if c.len() > maxShaderTextures {
			t.Fatalf("cache len = %d, want <= %d", c.len(), maxShaderTextures)
		}
	}
	before := len(deleted)
	c.clear()
	if c.len() != 0 || len(deleted)-before != maxShaderTextures {
		t.Fatalf("clear deleted %d textures, want %d", len(deleted)-before, maxShaderTextures)
	}
}
