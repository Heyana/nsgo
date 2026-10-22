package core

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"
)

// StorageQuota is the most an app may keep, in UTF-8 bytes of its keys and
// values.
const StorageQuota = 5 << 20

// StorageOp says what a StorageChange does.
type StorageOp uint8

const (
	// StorageSet sets Key to Value.
	StorageSet StorageOp = iota
	// StorageRemove removes Key.
	StorageRemove
	// StorageClear removes every entry.
	StorageClear
)

// StorageChange is a change to the entries, for the backend to keep.
type StorageChange struct {
	Op         StorageOp
	Key, Value string
}

// StorageBackend is where a platform keeps an app's entries: a file in
// the app's data directory, the browser's localStorage.
type StorageBackend interface {
	// Load returns the entries kept, read once when the app first uses
	// its storage. An error leaves the app with none, logged.
	Load() (map[string]string, error)
	// Write keeps change. It is called in order, on the goroutine of the
	// graph that made the change; a backend that blocks writes elsewhere.
	Write(change StorageChange)
}

// StorageFullError refuses a value that would grow the storage past
// StorageQuota.
type StorageFullError struct {
	// Bytes is what the storage would have held.
	Bytes int
}

func (e *StorageFullError) Error() string {
	return fmt.Sprintf("storage is full: %d bytes would exceed the quota of %d", e.Bytes, StorageQuota)
}

// Store holds an app's entries in memory, loaded from its backend on first
// use. Graphs given the same Store share its entries and quota, and each
// sees the others' changes through its signals. Hosts make one per app
// with NewStore and pass it in RootOptions.
type Store struct {
	mu       sync.Mutex
	backend  StorageBackend
	entries  map[string]string
	bytes    int
	watchers []*storageWatch
}

// NewStore returns a store kept by backend, or only in memory when backend
// is nil.
func NewStore(backend StorageBackend) *Store { return &Store{backend: backend} }

// load reads the backend's entries once. The caller holds mu.
func (s *Store) load() {
	if s.entries != nil {
		return
	}
	s.entries = map[string]string{}
	if s.backend == nil {
		return
	}
	entries, err := s.backend.Load()
	if err != nil {
		slog.Error("nsgo: the app's storage could not be read", "err", err)
		return
	}
	for k, v := range entries {
		s.entries[k] = v
		s.bytes += len(k) + len(v)
	}
}

func (s *Store) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	v, ok := s.entries[key]
	return v, ok
}

func (s *Store) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	keys := make([]string, 0, len(s.entries))
	for k := range s.entries {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// change applies c, hands it to the backend and tells the graphs watching,
// all under mu so every graph and the backend see changes in one order. It
// reports whether anything changed.
func (s *Store) change(c StorageChange, from *runtime) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	switch c.Op {
	case StorageSet:
		old, had := s.entries[c.Key]
		if had && old == c.Value {
			return false, nil
		}
		n := s.bytes + len(c.Key) + len(c.Value)
		if had {
			n -= len(c.Key) + len(old)
		}
		if n > StorageQuota {
			return false, &StorageFullError{Bytes: n}
		}
		s.entries[c.Key], s.bytes = c.Value, n
	case StorageRemove:
		old, had := s.entries[c.Key]
		if !had {
			return false, nil
		}
		delete(s.entries, c.Key)
		s.bytes -= len(c.Key) + len(old)
	case StorageClear:
		if len(s.entries) == 0 {
			return false, nil
		}
		clear(s.entries)
		s.bytes = 0
	}
	if s.backend != nil {
		s.backend.Write(c)
	}
	for _, w := range s.watchers {
		if w.rt != from {
			w.post(c)
		}
	}
	return true, nil
}

func (s *Store) watch(w *storageWatch) {
	s.mu.Lock()
	s.watchers = append(s.watchers, w)
	s.mu.Unlock()
}

func (s *Store) unwatch(w *storageWatch) {
	s.mu.Lock()
	s.watchers = slices.DeleteFunc(s.watchers, func(x *storageWatch) bool { return x == w })
	s.mu.Unlock()
}

// storageWatch is one graph's signals over a store: a version per key read
// and one for the set of keys.
type storageWatch struct {
	rt   *runtime
	post func(StorageChange)
	keys map[string]*Signal[uint64]
	all  *Signal[uint64]
}

func (w *storageWatch) key(k string) *Signal[uint64] {
	s := w.keys[k]
	if s == nil {
		s = &Signal[uint64]{rt: w.rt, equal: func(a, b uint64) bool { return a == b }}
		w.keys[k] = s
	}
	return s
}

// changed bumps the signals c affects; it runs on the watching graph.
func (w *storageWatch) changed(c StorageChange) {
	w.rt.root.Batch(func() {
		w.all.Update(func(n uint64) uint64 { return n + 1 })
		if c.Op == StorageClear {
			for _, s := range w.keys {
				s.Update(func(n uint64) uint64 { return n + 1 })
			}
			return
		}
		w.key(c.Key).Update(func(n uint64) uint64 { return n + 1 })
	})
}

// Storage is the app's key-value storage: strings by string key, kept on
// the device and shared by every graph of the app. Reads are synchronous,
// as the entries are in memory, and reading in a computation subscribes it
// to changes, the app's own and other graphs'. Use it where the app's code
// runs: components, handlers, effects and posted functions.
type Storage struct {
	store *Store
	watch *storageWatch
}

// Storage returns the app's storage: the host's Store, or one in memory
// for the graph's life when the host gave none.
func (cx *Scope) Storage() Storage {
	rt := cx.rt
	if rt.storage == nil {
		if rt.store == nil {
			rt.store = NewStore(nil)
		}
		w := &storageWatch{rt: rt, keys: map[string]*Signal[uint64]{}}
		w.all = &Signal[uint64]{rt: rt, equal: func(a, b uint64) bool { return a == b }}
		if rt.post != nil {
			w.post = func(c StorageChange) { rt.root.Post(func() { w.changed(c) }) }
		} else {
			w.post = func(StorageChange) {}
		}
		rt.store.watch(w)
		store := rt.store
		rt.root.addCleanup(func() { store.unwatch(w) })
		rt.storage = w
	}
	return Storage{rt.store, rt.storage}
}

// GetItem returns the value of key and whether it has one.
func (s Storage) GetItem(key string) (string, bool) {
	s.watch.key(key).Get()
	return s.store.get(key)
}

// SetItem sets key to value. It fails with a *StorageFullError when the
// storage would grow past StorageQuota, keeping what it held.
func (s Storage) SetItem(key, value string) error {
	return s.apply(StorageChange{Op: StorageSet, Key: key, Value: value})
}

// RemoveItem removes key.
func (s Storage) RemoveItem(key string) {
	s.apply(StorageChange{Op: StorageRemove, Key: key})
}

// Keys returns every key, sorted.
func (s Storage) Keys() []string {
	s.watch.all.Get()
	return s.store.keys()
}

// Clear removes every entry.
func (s Storage) Clear() {
	s.apply(StorageChange{Op: StorageClear})
}

func (s Storage) apply(c StorageChange) error {
	changed, err := s.store.change(c, s.watch.rt)
	if changed {
		s.watch.changed(c)
	}
	return err
}

// DataDir returns the directory the app keeps its files in: Application
// Support on Apple platforms, %APPDATA% on Windows, the files directory on
// Android, each with a directory of the app's own. It is "" on the web,
// which has none.
func (cx *Scope) DataDir() string { return cx.rt.dataDir }

// CacheDir returns the directory for files the app can make again, which
// the system may delete when space runs low; "" on the web.
func (cx *Scope) CacheDir() string { return cx.rt.cacheDir }
