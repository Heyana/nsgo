package web

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// maxCanvasImages bounds the unkept encoded sources a surface's canvas and shader caches retain, matching the native backends' 64-source ceiling.
const maxCanvasImages = 64

// boundedImageCache is a count- and byte-bounded LRU cache for web canvas images and WebGL shader textures. Kept (pinned) entries, loads under way, and entries drawn in the current pass stay out of eviction until they finish or release.
type boundedImageCache[T any] struct {
	entries      map[string]*imageCacheEntry[T]
	kept         map[string]int
	externalKept func(string) bool
	use          uint64
	pass         uint64
	limit        int64
	limitSet     bool
	onEvict      func(T)
}

type imageCacheEntry[T any] struct {
	value   T
	bytes   int64
	used    uint64
	pending bool
}

func (c *boundedImageCache[T]) effectiveLimit() int64 {
	if !c.limitSet {
		return driver.DefaultImageCacheLimit
	}
	return c.limit
}

func (c *boundedImageCache[T]) isKept(key string) bool {
	if c.kept[key] > 0 {
		return true
	}
	return c.externalKept != nil && c.externalKept(key)
}

func (c *boundedImageCache[T]) keep(key string) {
	if key == "" {
		return
	}
	if c.kept == nil {
		c.kept = map[string]int{}
	}
	c.kept[key]++
}

func (c *boundedImageCache[T]) release(key string) {
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

func (c *boundedImageCache[T]) setLimit(maxBytes int64) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	c.limit = maxBytes
	c.limitSet = true
	c.pass = c.use
	c.evict(0)
}

func (c *boundedImageCache[T]) trim() {
	c.pass = c.use
	c.evict(0)
}

func (c *boundedImageCache[T]) endPass() {
	c.pass = c.use
	if c.effectiveLimit() == 0 {
		c.evict(0)
	}
}

func (c *boundedImageCache[T]) get(key string) (T, bool) {
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

func (c *boundedImageCache[T]) peek(key string) (T, bool) {
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

func (c *boundedImageCache[T]) addPending(key string, value T, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	if c.entries == nil {
		c.entries = map[string]*imageCacheEntry[T]{}
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
	c.entries[key] = &imageCacheEntry[T]{value: value, bytes: bytes, used: c.use, pending: true}
}

func (c *boundedImageCache[T]) put(key string, value T, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	if c.entries == nil {
		c.entries = map[string]*imageCacheEntry[T]{}
	}
	c.use++
	if old, ok := c.entries[key]; ok {
		if c.onEvict != nil {
			c.onEvict(old.value)
		}
		delete(c.entries, key)
	}
	c.entries[key] = &imageCacheEntry[T]{value: value, bytes: bytes, used: c.use}
	c.evict(0)
}

func (c *boundedImageCache[T]) complete(key string, bytes int64) {
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

func (c *boundedImageCache[T]) clear() {
	for key, e := range c.entries {
		if c.onEvict != nil {
			c.onEvict(e.value)
		}
		delete(c.entries, key)
	}
	clear(c.kept)
}

func (c *boundedImageCache[T]) len() int { return len(c.entries) }

func (c *boundedImageCache[T]) unkeptStats() (count int, bytes int64) {
	for k, e := range c.entries {
		if !c.isKept(k) {
			count++
			bytes += e.bytes
		}
	}
	return count, bytes
}

func (c *boundedImageCache[T]) evict(reserve int) {
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
