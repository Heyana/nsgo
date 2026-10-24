//go:build darwin

package apple

import (
	"container/list"
	"sync"
)

// refCache is a bounded LRU of CoreFoundation objects. Each entry keeps
// one reference; get and put hand the caller another to release. Both
// retain under the cache's lock, so an eviction cannot free an entry
// between the lookup and the caller's reference.
type refCache[K comparable] struct {
	mu      sync.Mutex
	entries map[K]*list.Element
	order   *list.List // most recently used first
	limit   int
}

type refEntry[K comparable] struct {
	key   K
	value uintptr
}

func newRefCache[K comparable](limit int) *refCache[K] {
	return &refCache[K]{
		entries: make(map[K]*list.Element),
		order:   list.New(),
		limit:   limit,
	}
}

// get returns a retained reference for k, or 0 when the cache has none.
// The caller releases it.
func (c *refCache[K]) get(k K) uintptr {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok {
		return 0
	}
	c.order.MoveToFront(e)
	return cfRetain(e.Value.(refEntry[K]).value)
}

// put stores v for k and returns a reference for the caller. It takes
// ownership of v; the caller releases the returned reference. When another
// goroutine stored k first, put keeps that object and releases v.
func (c *refCache[K]) put(k K, v uintptr) uintptr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[k]; ok {
		c.order.MoveToFront(e)
		kept := cfRetain(e.Value.(refEntry[K]).value)
		Release(v)
		return kept
	}
	if c.order.Len() >= c.limit {
		oldest := c.order.Back()
		e := oldest.Value.(refEntry[K])
		c.order.Remove(oldest)
		delete(c.entries, e.key)
		Release(e.value)
	}
	c.entries[k] = c.order.PushFront(refEntry[K]{key: k, value: v})
	return cfRetain(v)
}

// len is the number of entries.
func (c *refCache[K]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// clear releases every entry.
func (c *refCache[K]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for e := c.order.Front(); e != nil; e = e.Next() {
		Release(e.Value.(refEntry[K]).value)
	}
	c.entries = make(map[K]*list.Element)
	c.order.Init()
}
