package linux

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// maxShaderTextures bounds the unkept GL textures a shader view keeps in GPU
// memory, matching the canvas image cache's 64-source ceiling.
const maxShaderTextures = 64

// glTextureCache is a count- and byte-bounded LRU cache for a shader view's
// OpenGL textures. Kept (pinned) entries and textures used in the draw pass
// under way stay out of eviction; evicted entries invoke release so
// glDeleteTextures frees them while the area's GL context is current.
type glTextureCache[T any] struct {
	entries  map[string]*glTextureEntry[T]
	use      uint64
	pass     uint64
	limit    int64
	limitSet bool
	release  func(T)
}

type glTextureEntry[T any] struct {
	value T
	bytes int64
	used  uint64
}

func (c *glTextureCache[T]) effectiveLimit() int64 {
	if !c.limitSet {
		return driver.DefaultImageCacheLimit
	}
	return c.limit
}

func (c *glTextureCache[T]) get(key string) (T, bool) {
	var zero T
	if c.entries == nil {
		return zero, false
	}
	e, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	c.use++
	e.used = c.use
	return e.value, true
}

func (c *glTextureCache[T]) put(key string, value T, bytes int64, isKept func(string) bool) {
	if bytes < 0 {
		bytes = 0
	}
	if c.entries == nil {
		c.entries = map[string]*glTextureEntry[T]{}
	}
	c.use++
	if old, ok := c.entries[key]; ok {
		if c.release != nil {
			c.release(old.value)
		}
		delete(c.entries, key)
	}
	c.entries[key] = &glTextureEntry[T]{value: value, bytes: bytes, used: c.use}
	c.evict(isKept)
}

func (c *glTextureCache[T]) endPass(isKept func(string) bool) {
	c.pass = c.use
	if c.effectiveLimit() == 0 {
		c.evict(isKept)
	}
}

func (c *glTextureCache[T]) setLimit(maxBytes int64, isKept func(string) bool) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	c.limit = maxBytes
	c.limitSet = true
	c.pass = c.use
	c.evict(isKept)
}

func (c *glTextureCache[T]) trim(isKept func(string) bool) {
	c.pass = c.use
	c.evict(isKept)
}

func (c *glTextureCache[T]) clear() {
	for key, e := range c.entries {
		if c.release != nil {
			c.release(e.value)
		}
		delete(c.entries, key)
	}
}

func (c *glTextureCache[T]) len() int { return len(c.entries) }

func (c *glTextureCache[T]) unkeptStats(isKept func(string) bool) (count int, bytes int64) {
	for k, e := range c.entries {
		if isKept == nil || !isKept(k) {
			count++
			bytes += e.bytes
		}
	}
	return count, bytes
}

func (c *glTextureCache[T]) evict(isKept func(string) bool) {
	limit := c.effectiveLimit()
	unkept, unkeptBytes := c.unkeptStats(isKept)
	for unkept > maxShaderTextures || unkeptBytes > limit || (limit == 0 && unkept > 0) {
		var oldest string
		var at uint64 = math.MaxUint64
		for k, e := range c.entries {
			if (isKept == nil || !isKept(k)) && e.used < at {
				oldest, at = k, e.used
			}
		}
		if at > c.pass {
			break
		}
		old := c.entries[oldest]
		if c.release != nil {
			c.release(old.value)
		}
		unkeptBytes -= old.bytes
		delete(c.entries, oldest)
		unkept--
	}
}
