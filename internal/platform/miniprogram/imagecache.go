package miniprogram

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// maxCanvasImages bounds the unkept encoded sources a mini program's offscreen
// canvas image cache keeps, matching the native backends' 64-source ceiling.
const maxCanvasImages = 64

// canvasImageCache is a count- and byte-bounded LRU cache for mini-program
// canvas images. Pinned entries (KeepImage), loads under way, and images drawn
// in the current paint pass stay out of eviction until they finish or release.
type canvasImageCache[T any] struct {
	entries  map[string]*canvasImageEntry[T]
	kept     map[string]int
	use      uint64
	pass     uint64
	limit    int64
	limitSet bool
	onEvict  func(T)
}

type canvasImageEntry[T any] struct {
	value   T
	bytes   int64
	used    uint64
	pending bool
}

func (c *canvasImageCache[T]) effectiveLimit() int64 {
	if !c.limitSet {
		return driver.DefaultImageCacheLimit
	}
	return c.limit
}

func (c *canvasImageCache[T]) isKept(key string) bool { return c.kept[key] > 0 }

func (c *canvasImageCache[T]) keep(key string) {
	if key == "" {
		return
	}
	if c.kept == nil {
		c.kept = map[string]int{}
	}
	c.kept[key]++
}

func (c *canvasImageCache[T]) release(key string) {
	if key == "" {
		return
	}
	n := c.kept[key]
	if n <= 0 {
		return
	}
	if n == 1 {
		delete(c.kept, key)
	} else {
		c.kept[key] = n - 1
	}
	c.pass = c.use
	c.evict(0)
}

func (c *canvasImageCache[T]) setLimit(maxBytes int64) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	c.limit = maxBytes
	c.limitSet = true
	c.pass = c.use
	c.evict(0)
}

func (c *canvasImageCache[T]) endPass() {
	c.pass = c.use
	if c.effectiveLimit() == 0 {
		c.evict(0)
	}
}

func (c *canvasImageCache[T]) get(key string) (T, bool) {
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

func (c *canvasImageCache[T]) peek(key string) (T, bool) {
	var zero T
	if c.entries == nil {
		return zero, false
	}
	e, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	return e.value, true
}

func (c *canvasImageCache[T]) addPending(key string, value T, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	if c.entries == nil {
		c.entries = map[string]*canvasImageEntry[T]{}
	}
	c.use++
	reserve := 0
	if !c.isKept(key) {
		reserve = 1
	}
	c.evict(reserve)
	if old, ok := c.entries[key]; ok {
		if c.onEvict != nil {
			c.onEvict(old.value)
		}
		delete(c.entries, key)
	}
	c.entries[key] = &canvasImageEntry[T]{value: value, bytes: bytes, used: c.use, pending: true}
}

func (c *canvasImageCache[T]) complete(key string, bytes int64) {
	e, ok := c.entries[key]
	if !ok {
		return
	}
	if bytes < 0 {
		bytes = 0
	}
	e.pending = false
	if bytes > 0 {
		e.bytes = bytes
	}
	if _, unkeptBytes := c.unkeptStats(); unkeptBytes > c.effectiveLimit() || c.effectiveLimit() == 0 {
		c.evict(0)
	}
}

func (c *canvasImageCache[T]) len() int { return len(c.entries) }

func (c *canvasImageCache[T]) unkeptStats() (count int, bytes int64) {
	for k, e := range c.entries {
		if !c.isKept(k) {
			count++
			bytes += e.bytes
		}
	}
	return count, bytes
}

func (c *canvasImageCache[T]) evict(reserve int) {
	limit := c.effectiveLimit()
	unkept, unkeptBytes := c.unkeptStats()
	for unkept+reserve > maxCanvasImages || unkeptBytes > limit || (limit == 0 && unkept > 0) {
		var oldest string
		var at uint64 = math.MaxUint64
		for k, e := range c.entries {
			if !c.isKept(k) && !e.pending && e.used < at {
				oldest, at = k, e.used
			}
		}
		if at > c.pass {
			break
		}
		old := c.entries[oldest]
		if c.onEvict != nil {
			c.onEvict(old.value)
		}
		unkeptBytes -= old.bytes
		delete(c.entries, oldest)
		unkept--
	}
}
