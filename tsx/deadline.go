package tsx

import (
	"time"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// A deadline owns a plugin call's async work, with or without a total time limit. It also holds the total limits of module loading and host events. Only the engine goroutine uses it; the watchdog posts to that goroutine.
type deadline struct {
	// at is zero when only the budget of each JS slice applies.
	at      time.Time
	expired bool
	// timers are the ids of the timers it set, which expire stops.
	timers map[int64]struct{}
	// done is the pending call's callback, nil once it settles or for work that is not a Plugin.Call.
	done func(any, error)
	// watch is the timer that expires it on time, set once work waits on it.
	watch *time.Timer
}

func newDeadline(d time.Duration) *deadline {
	dl := &deadline{}
	if d > 0 {
		dl.at = time.Now().Add(d)
	}
	return dl
}

// watch has dl expire on time from the engine's goroutine, once something waits on it.
func (r *runtime) watch(dl *deadline) {
	if dl.at.IsZero() || dl.watch != nil || dl.expired {
		return
	}
	if r.watches == nil {
		r.watches = make(map[*deadline]struct{})
	}
	r.watches[dl] = struct{}{}
	dl.watch = time.AfterFunc(time.Until(dl.at), func() {
		r.cx.Post(func() {
			// An extension can move the deadline before this post runs.
			if r.stoppedErr() == nil && dl.watch != nil {
				r.passed(dl)
			}
		})
	})
}

// stopWatches releases the watchdogs when the runtime stops or is disposed.
func (r *runtime) stopWatches() {
	for dl := range r.watches {
		dl.watch.Stop()
		dl.watch = nil
	}
	clear(r.watches)
}

// passed reports whether dl has run out, expiring it if it just did.
func (r *runtime) passed(dl *deadline) bool {
	if dl == nil {
		return false
	}
	if !dl.expired && !dl.at.IsZero() && !time.Now().Before(dl.at) {
		r.expire(dl)
	}
	return dl.expired
}

// expire drops what dl's work set going and fails its call, once.
func (r *runtime) expire(dl *deadline) {
	if dl.expired {
		return
	}
	dl.expired = true
	if dl.watch != nil {
		dl.watch.Stop()
		dl.watch = nil
		delete(r.watches, dl)
	}
	for id := range dl.timers {
		r.stopTimer(id)
	}
	dl.timers = nil
	r.finish(dl, nil, ErrBudget)
}

// under runs work with its owner unless it has already been dropped.
func (r *runtime) under(dl *deadline, fn func()) {
	if dl == nil {
		fn()
		return
	}
	if r.passed(dl) {
		return
	}
	prev := r.current
	r.current = dl
	defer func() { r.current = prev }()
	if r.depth > 0 {
		r.adopt(dl)
	}
	fn()
}

// adopt keeps the earliest total deadline for the JS slice, separately from the owner of the particular callback or promise reaction running.
func (r *runtime) adopt(dl *deadline) {
	if dl.expired || dl.at.IsZero() {
		return
	}
	if earlier := r.sliceDeadline; earlier != nil && !earlier.at.IsZero() && !dl.at.Before(earlier.at) {
		return
	}
	r.sliceDeadline = dl
	if r.slice != nil && dl.at.Before(r.sliceEnd) {
		r.sliceEnd = dl.at
		r.slice.Reset(time.Until(dl.at))
	}
}

// resumer returns a function that runs what the work running now set going, on the engine's goroutine, under the deadline it runs under, dropping it once that passed; the functions run as they are where no deadline holds.
func (r *runtime) resumer() func(fn func()) bool {
	dl := r.current
	if dl == nil {
		return func(fn func()) bool { fn(); return true }
	}
	r.watch(dl)
	return func(fn func()) bool {
		ran := false
		r.under(dl, func() { ran = true; fn() })
		return ran
	}
}

// newPromise is the VM's NewPromise, whose settling, as a host request answers, runs under the deadline of the work that made the promise, and not at all once it passed. Any uncatchable VM error (such as an interrupt from Limits.Call) is propagated to the enclosing slice instead of being returned and dropped.
func (r *runtime) newPromise() (*js.Promise, func(any) error, func(any) error) {
	p, resolve, reject := r.vm.NewPromise()
	resume := r.resumer()
	wrap := func(settle func(any) error) func(any) error {
		return func(v any) error {
			var err error
			if r.depth > 0 {
				var interruptedOwner *deadline
				resume(func() {
					if err = settle(v); err != nil {
						interruptedOwner = r.current
					}
				})
				if err != nil {
					// Restore the interrupted reaction's owner before panicking out of r.under, so r.within expires the right deadline.
					if interruptedOwner != nil {
						r.current = interruptedOwner
					}
					panic(err)
				}
				return nil
			}
			// Settled outside a call into JS, its jobs run in one of their own, under the deadline.
			resume(func() {
				r.inJS(func() {
					if err = settle(v); err != nil {
						panic(err)
					}
				})
			})
			return err
		}
	}
	return p, wrap(resolve), wrap(reject)
}

// CallBudget controls one waiting Plugin.Call's total deadline. It is safe
// to keep after a call settles.
type CallBudget struct {
	r  *runtime
	dl *deadline
}

// Extend adds time to a pending call's total budget before it expires. Call
// it on the engine goroutine between JavaScript entries, for example in a
// Scope.Post from a worker. It reports whether the deadline moved.
func (b *CallBudget) Extend(extra time.Duration) bool {
	if b == nil || b.dl == nil || extra <= 0 || b.r.depth != 0 || b.r.stoppedErr() != nil {
		return false
	}
	dl := b.dl
	if dl.expired || dl.done == nil || dl.watch == nil || !time.Now().Before(dl.at) {
		return false
	}
	dl.at = dl.at.Add(extra)
	dl.watch.Reset(time.Until(dl.at))
	return true
}
