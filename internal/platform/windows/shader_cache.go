package windows

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// maxShaderTextures bounds the unkept textures a shader view keeps in GPU memory, matching the canvas image cache's 64-source ceiling.
const maxShaderTextures = 64

// shaderTextureCache is a count- and byte-bounded LRU cache for a shader view's Direct3D textures. Kept (pinned) entries and textures used in the draw pass under way stay out of eviction; evicted entries invoke release so their COM resources are freed promptly.
type shaderTextureCache[T any] struct {
	entries  map[string]*shaderTextureEntry[T]
	use      uint64
	pass     uint64
	limit    int64
	limitSet bool
	release  func(T)
}

type shaderTextureEntry[T any] struct {
	value T
	bytes int64
	used  uint64
}

func (c *shaderTextureCache[T]) effectiveLimit() int64 {
	if !c.limitSet {
		return driver.DefaultImageCacheLimit
	}
	return c.limit
}

func (c *shaderTextureCache[T]) get(key string) (T, bool) {
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

func (c *shaderTextureCache[T]) delete(key string) {
	if e, ok := c.entries[key]; ok {
		if c.release != nil {
			c.release(e.value)
		}
		delete(c.entries, key)
	}
}

func (c *shaderTextureCache[T]) put(key string, value T, bytes int64, isKept func(string) bool) {
	if bytes < 0 {
		bytes = 0
	}
	if c.entries == nil {
		c.entries = map[string]*shaderTextureEntry[T]{}
	}
	c.use++
	if old, ok := c.entries[key]; ok {
		if c.release != nil {
			c.release(old.value)
		}
		delete(c.entries, key)
	}
	c.entries[key] = &shaderTextureEntry[T]{value: value, bytes: bytes, used: c.use}
	c.evict(isKept)
}

// endPass ends the shader render pass so textures bound for that draw call may be evicted on subsequent draws or immediately when the byte budget is zero.
func (c *shaderTextureCache[T]) endPass(isKept func(string) bool) {
	c.pass = c.use
	if c.effectiveLimit() == 0 {
		c.evict(isKept)
	}
}

// setLimit updates the byte budget, clamping negative values to zero, and immediately evicts excess unkept textures.
func (c *shaderTextureCache[T]) setLimit(maxBytes int64, isKept func(string) bool) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	c.limit = maxBytes
	c.limitSet = true
	c.pass = c.use
	c.evict(isKept)
}

// trim evicts excess unkept textures, such as after a KeepImage pin releases.
func (c *shaderTextureCache[T]) trim(isKept func(string) bool) {
	c.pass = c.use
	c.evict(isKept)
}

func (c *shaderTextureCache[T]) clear() {
	for key, e := range c.entries {
		if c.release != nil {
			c.release(e.value)
		}
		delete(c.entries, key)
	}
}

func (c *shaderTextureCache[T]) len() int { return len(c.entries) }

func (c *shaderTextureCache[T]) totalBytes() int64 {
	var total int64
	for _, e := range c.entries {
		total += e.bytes
	}
	return total
}

func (c *shaderTextureCache[T]) unkeptStats(isKept func(string) bool) (count int, bytes int64) {
	for k, e := range c.entries {
		if isKept == nil || !isKept(k) {
			count++
			bytes += e.bytes
		}
	}
	return count, bytes
}

func (c *shaderTextureCache[T]) evict(isKept func(string) bool) {
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
