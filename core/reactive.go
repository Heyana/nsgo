package core

// The reactive graph: signals hold values, effects read them and run again
// when they change, as Solid's do. A Scope owns one graph; the engine runs
// it on its own goroutine, so nothing here locks.

// Scope is an app's reactive context, passed to components as cx.
type Scope struct {
	observer *effect
	pending  []*effect
	flushing bool
}

// NewScope returns an empty reactive context.
func NewScope() *Scope { return &Scope{} }

type source interface {
	unsubscribe(e *effect)
}

type effect struct {
	cx       *Scope
	fn       func()
	sources  []source
	queued   bool
	disposed bool
}

// Effect runs fn now and again whenever a signal it read changes.
// The returned function stops it.
func (cx *Scope) Effect(fn func()) (dispose func()) {
	e := &effect{cx: cx, fn: fn}
	e.run()
	return e.dispose
}

// Untrack runs fn without subscribing the current effect to what it reads.
func (cx *Scope) Untrack(fn func()) {
	prev := cx.observer
	cx.observer = nil
	defer func() { cx.observer = prev }()
	fn()
}

// Batch runs fn and then the effects its writes scheduled, once each.
func (cx *Scope) Batch(fn func()) {
	if cx.flushing {
		fn()
		return
	}
	cx.flushing = true
	fn()
	cx.flushing = false
	cx.flush()
}

func (e *effect) run() {
	if e.disposed {
		return
	}
	e.clear()
	prev := e.cx.observer
	e.cx.observer = e
	defer func() { e.cx.observer = prev }()
	e.fn()
}

func (e *effect) clear() {
	for _, s := range e.sources {
		s.unsubscribe(e)
	}
	e.sources = e.sources[:0]
}

func (e *effect) dispose() {
	e.disposed = true
	e.clear()
}

func (cx *Scope) schedule(e *effect) {
	if e.queued || e.disposed {
		return
	}
	e.queued = true
	cx.pending = append(cx.pending, e)
	if !cx.flushing {
		cx.flush()
	}
}

func (cx *Scope) flush() {
	cx.flushing = true
	for len(cx.pending) > 0 {
		e := cx.pending[0]
		cx.pending = cx.pending[1:]
		e.queued = false
		e.run()
	}
	cx.pending = nil
	cx.flushing = false
}

// Signal is a value effects subscribe to by reading it.
type Signal[T comparable] struct {
	cx    *Scope
	value T
	subs  []*effect
}

// NewSignal returns a signal of cx holding value.
func NewSignal[T comparable](cx *Scope, value T) *Signal[T] {
	return &Signal[T]{cx: cx, value: value}
}

// Get returns the value and subscribes the running effect to it.
func (s *Signal[T]) Get() T {
	if e := s.cx.observer; e != nil {
		for _, sub := range s.subs {
			if sub == e {
				return s.value
			}
		}
		s.subs = append(s.subs, e)
		e.sources = append(e.sources, s)
	}
	return s.value
}

// Set stores value and runs the effects that read it, unless it is unchanged.
func (s *Signal[T]) Set(value T) {
	if s.value == value {
		return
	}
	s.value = value
	subs := append([]*effect(nil), s.subs...)
	for _, e := range subs {
		s.cx.schedule(e)
	}
}

// Update sets the value fn returns for the current one.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }

func (s *Signal[T]) unsubscribe(e *effect) {
	for i, sub := range s.subs {
		if sub == e {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			return
		}
	}
}
