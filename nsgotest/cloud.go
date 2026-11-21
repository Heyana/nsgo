package nsgotest

import (
	"bytes"
	"slices"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// CloudAvailable starts a headless app with its per-app in-memory cloud store enabled or disabled, setting core.Env.CanUseCloudStorage accordingly from the first render.
func CloudAvailable(available bool) Option {
	return func(a *App) {
		v := available
		a.cloudInit = &v
	}
}

// SetCloudAvailable toggles the per-app in-memory cloud store's availability and updates core.Env.CanUseCloudStorage on every surface, then settles. When false, every package cloud operation returns cloud.ErrUnavailable and never silently drops writes.
func (a *App) SetCloudAvailable(available bool) {
	a.t.Helper()
	if a.owner != nil {
		a.owner.SetCloudAvailable(available)
		return
	}
	if a.cloud != nil {
		a.cloud.setAvailable(available)
	}
	var env driver.Environment
	if a.env != nil {
		env = *a.env
	}
	env.CloudStorage = available
	a.SetEnvironment(env)
}

// SimulateCloudChange applies external multi-device changes to the app's in-memory cloud store, delivers a ChangeEvent to all active cloud.WatchChanges subscribers on the engine goroutine, and settles. A nil slice in kv or files deletes that key or file; a non-nil slice writes or replaces it.
func (a *App) SimulateCloudChange(kv map[string][]byte, files map[string][]byte, reason core.CloudChangeReason) {
	a.t.Helper()
	if a.owner != nil {
		a.owner.SimulateCloudChange(kv, files, reason)
		return
	}
	if a.cloud == nil {
		return
	}
	a.cloud.simulateChange(kv, files, reason)
	a.Settle()
}

// SimulateCloudConflict injects a conflicting remote version for the file named name in the app's in-memory cloud store, delivers a Conflict (with a working Resolve callback) to all active cloud.OnConflict subscribers on the engine goroutine, and settles. Until resolved (or overwritten/deleted), cloud.ReadFile on name returns a *cloud.ConflictError unwrapping to cloud.ErrConflict.
func (a *App) SimulateCloudConflict(name string, remote []byte) {
	a.t.Helper()
	if a.owner != nil {
		a.owner.SimulateCloudConflict(name, remote)
		return
	}
	if a.cloud == nil {
		return
	}
	a.cloud.simulateConflict(name, remote)
	a.Settle()
}

type cloudEntry struct {
	data    []byte
	modTime time.Time
}

type cloudPendingConflict struct {
	local      []byte
	remote     []byte
	localTime  time.Time
	remoteTime time.Time
}

// memoryCloudStore is an isolated per-app in-memory implementation of core.CloudStore for headless nsgotest runs.
type memoryCloudStore struct {
	mu        sync.Mutex
	available bool
	now       func() time.Time
	kv        map[string]cloudEntry
	files     map[string]cloudEntry
	conflicts map[string]*cloudPendingConflict
	nextID    uint64
	watchers  map[uint64]func(core.CloudChangeEvent)
	handlers  map[uint64]func(core.CloudConflict)
}

func newMemoryCloudStore(now func() time.Time) *memoryCloudStore {
	return &memoryCloudStore{
		now:       now,
		kv:        map[string]cloudEntry{},
		files:     map[string]cloudEntry{},
		conflicts: map[string]*cloudPendingConflict{},
		watchers:  map[uint64]func(core.CloudChangeEvent){},
		handlers:  map[uint64]func(core.CloudConflict){},
	}
}

func (s *memoryCloudStore) setAvailable(avail bool) {
	s.mu.Lock()
	s.available = avail
	s.mu.Unlock()
}

func (s *memoryCloudStore) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.available
}

func (s *memoryCloudStore) Get(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, core.ErrCloudUnavailable
	}
	e, ok := s.kv[key]
	if !ok {
		return nil, core.ErrCloudNotFound
	}
	return bytes.Clone(e.data), nil
}

func (s *memoryCloudStore) Set(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return core.ErrCloudUnavailable
	}
	s.kv[key] = cloudEntry{data: bytes.Clone(value), modTime: s.now()}
	return nil
}

func (s *memoryCloudStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return core.ErrCloudUnavailable
	}
	delete(s.kv, key)
	return nil
}

func (s *memoryCloudStore) Keys() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, core.ErrCloudUnavailable
	}
	out := make([]string, 0, len(s.kv))
	for k := range s.kv {
		out = append(out, k)
	}
	slices.Sort(out)
	return out, nil
}

func (s *memoryCloudStore) ReadFile(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, core.ErrCloudUnavailable
	}
	if c, ok := s.conflicts[name]; ok {
		return nil, &core.CloudConflictError{
			Key:        name,
			Local:      bytes.Clone(c.local),
			Remote:     bytes.Clone(c.remote),
			LocalTime:  c.localTime,
			RemoteTime: c.remoteTime,
		}
	}
	e, ok := s.files[name]
	if !ok {
		return nil, core.ErrCloudNotFound
	}
	return bytes.Clone(e.data), nil
}

func (s *memoryCloudStore) WriteFile(name string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return core.ErrCloudUnavailable
	}
	s.files[name] = cloudEntry{data: bytes.Clone(data), modTime: s.now()}
	delete(s.conflicts, name)
	return nil
}

func (s *memoryCloudStore) DeleteFile(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return core.ErrCloudUnavailable
	}
	delete(s.files, name)
	delete(s.conflicts, name)
	return nil
}

func (s *memoryCloudStore) ListFiles() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, core.ErrCloudUnavailable
	}
	out := make([]string, 0, len(s.files))
	for k := range s.files {
		out = append(out, k)
	}
	slices.Sort(out)
	return out, nil
}

func (s *memoryCloudStore) WatchChanges(onChange func(core.CloudChangeEvent)) func() {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.watchers[id] = onChange
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.watchers, id)
		s.mu.Unlock()
	}
}

func (s *memoryCloudStore) OnConflict(resolve func(core.CloudConflict)) func() {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.handlers[id] = resolve
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.handlers, id)
		s.mu.Unlock()
	}
}

func (s *memoryCloudStore) simulateChange(kv map[string][]byte, files map[string][]byte, reason core.CloudChangeReason) {
	s.mu.Lock()
	now := s.now()
	var keys, changedFiles []string
	for k, v := range kv {
		if v == nil {
			delete(s.kv, k)
		} else {
			s.kv[k] = cloudEntry{data: bytes.Clone(v), modTime: now}
		}
		keys = append(keys, k)
	}
	for name, v := range files {
		if v == nil {
			delete(s.files, name)
		} else {
			s.files[name] = cloudEntry{data: bytes.Clone(v), modTime: now}
		}
		delete(s.conflicts, name)
		changedFiles = append(changedFiles, name)
	}
	slices.Sort(keys)
	slices.Sort(changedFiles)
	watchers := make([]func(core.CloudChangeEvent), 0, len(s.watchers))
	for _, fn := range s.watchers {
		watchers = append(watchers, fn)
	}
	s.mu.Unlock()

	ev := core.CloudChangeEvent{
		Keys:   keys,
		Files:  changedFiles,
		Reason: reason,
	}
	for _, fn := range watchers {
		fn(ev)
	}
}

func (s *memoryCloudStore) simulateConflict(name string, remote []byte) {
	s.mu.Lock()
	now := s.now()
	localEntry := s.files[name]
	c := &cloudPendingConflict{
		local:      bytes.Clone(localEntry.data),
		remote:     bytes.Clone(remote),
		localTime:  localEntry.modTime,
		remoteTime: now,
	}
	s.conflicts[name] = c
	handlers := make([]func(core.CloudConflict), 0, len(s.handlers))
	for _, fn := range s.handlers {
		handlers = append(handlers, fn)
	}
	s.mu.Unlock()

	conflict := core.CloudConflict{
		Name:       name,
		Local:      bytes.Clone(c.local),
		Remote:     bytes.Clone(c.remote),
		LocalTime:  c.localTime,
		RemoteTime: c.remoteTime,
		Resolve: func(chosen []byte) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.available {
				return core.ErrCloudUnavailable
			}
			s.files[name] = cloudEntry{data: bytes.Clone(chosen), modTime: s.now()}
			delete(s.conflicts, name)
			return nil
		},
	}
	for _, fn := range handlers {
		fn(conflict)
	}
}
