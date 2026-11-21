package core

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// QuitTimeout is how long the engine waits for the work of the OnQuit
// handlers as the app quits, at most, as GPUI's shutdown timeout. The
// platform's main thread waits for the engine meanwhile, so it stays
// short.
const QuitTimeout = 200 * time.Millisecond

type quitHandler struct {
	owner *Scope
	fn    func(ctx context.Context)
}

// QuitRequest is the core's part of desktop.QuitRequest, which apps use for user and system requests to quit.
type QuitRequest struct {
	forced   bool
	canceled bool
	deferred bool
	answered atomic.Bool
	answer   func(bool)
}

// Forced reports whether the system is ending the session, as at logout, restart or shutdown.
func (q *QuitRequest) Forced() bool { return q != nil && q.forced }

// Cancel keeps the app running. It has an effect in a user quit's request handler.
func (q *QuitRequest) Cancel() {
	if q != nil && !q.forced && !q.deferred {
		q.canceled = true
	}
}

// Canceled reports whether a handler canceled the quit: what a test of a handler reads.
func (q *QuitRequest) Canceled() bool { return q != nil && q.canceled }

// Defer keeps a user quit pending and returns a one-shot answer. quit true proceeds with quitting; false cancels it. The returned function is safe to call from another goroutine. Call it in every case: until it is called, a later user quit request waits for this one's answer. A handler that panics after Defer, without having answered, cancels the request, and the function it got then does nothing. For a forced request, Defer returns a function that does nothing.
func (q *QuitRequest) Defer() func(quit bool) {
	if q == nil || q.forced || q.canceled || q.deferred || q.answer == nil {
		return func(bool) {}
	}
	q.deferred = true
	return func(quit bool) {
		if q.answered.Swap(true) {
			return
		}
		q.answer(quit)
	}
}

type quitRequestHandler struct {
	owner *Scope
	fn    func(*QuitRequest)
}

// OnQuitRequest is the core's part of desktop.OnQuitRequest. On desktop it runs fn while the current owner lives, when the user asks the app to quit or the system ends the session. Handlers run newest first. A user request goes on to the next handler until one cancels or defers it, or all accept it. A forced request cannot be canceled or deferred. Scope.Quit and Restart remain immediate.
func (cx *Scope) OnQuitRequest(fn func(*QuitRequest)) {
	rt := cx.rt
	h := &quitRequestHandler{owner: cx.current(), fn: fn}
	rt.quitRequests = append(rt.quitRequests, h)
	h.owner.addCleanup(func() {
		rt.quitRequests = slices.DeleteFunc(rt.quitRequests, func(o *quitRequestHandler) bool { return o == h })
	})
}

// RequestQuit is the core's part of desktop.RequestQuit.
func (cx *Scope) RequestQuit() {
	rt := cx.rt
	rt.requestQuit(false, func(approved bool) {
		if approved {
			rt.quit(false)
		}
	})
}

func (rt *runtime) quitRequest(id uint32, forced bool) {
	rt.requestQuit(forced, func(quit bool) {
		rt.app(driver.MountItem{Kind: driver.ItemQuitResponse, Count: id, On: quit})
	})
}

func (rt *runtime) requestQuit(forced bool, reply func(bool)) {
	if rt.tree == nil {
		return
	}
	if !forced {
		if rt.quitPending {
			reply(false)
			return
		}
		rt.quitPending = true
	}
	answer := func(quit bool) {
		if !forced {
			rt.quitPending = false
		}
		reply(quit)
	}
	// A handler may dispose the owners of others, as closing a window does, so they run from a copy.
	hs := slices.Clone(rt.quitRequests)
	for i := len(hs) - 1; i >= 0; i-- {
		h := hs[i]
		if h.owner.disposed {
			continue
		}
		q := &QuitRequest{forced: forced, answer: func(quit bool) {
			if rt.post != nil {
				rt.post(func() { answer(quit) })
			} else {
				answer(quit)
			}
		}}
		returned := false
		rt.tree.within(h.owner, func() {
			h.fn(q)
			returned = true
		})
		switch {
		case q.canceled:
			answer(false)
			return
		case q.deferred:
			// A handler that panics after Defer loses the function that would answer, and the platform would wait for it forever, so the request is canceled for it; the lost function then does nothing.
			if !returned && !q.answered.Swap(true) {
				answer(false)
			}
			return
		}
	}
	answer(true)
}

// OnQuit runs fn, while the current owner lives, as the app quits,
// however it stops: its own Quit, the last window closing, the Dock's or
// the system's quit, the runtime ending on Android, a host removing its
// view. A page hiding on the web and a watchOS app going to the
// background run it too, as the system may end either unheard, and keep
// it for the next time in case the app shows again. fn cleans up, such
// as saving a draft or closing a connection; it cannot keep the app
// running. The handlers run on the engine goroutine, oldest first, with
// one context that ends QuitTimeout after the first starts: a handler
// may wait on it for its own goroutines' work, and the engine ends once
// they return or the time is up.
func (cx *Scope) OnQuit(fn func(ctx context.Context)) {
	rt := cx.rt
	h := &quitHandler{owner: cx.current(), fn: fn}
	rt.quits = append(rt.quits, h)
	h.owner.addCleanup(func() { rt.quits = slices.DeleteFunc(rt.quits, func(o *quitHandler) bool { return o == h }) })
}

// Quitting runs the OnQuit handlers of the graph whose root cx is, for
// the engine as it stops, and reports whether there were any. Each runs
// once: a second call runs none.
func (cx *Scope) Quitting() bool {
	return cx.quitting(false)
}

// Hiding runs the OnQuit handlers as Quitting does but keeps them, for an
// app the system hides and may end without a word: a page the browser
// keeps in its back-forward cache, or a watchOS app in the background.
// It may show the app again, which runs them again at its next hide or
// quit.
func (cx *Scope) Hiding() bool {
	return cx.quitting(true)
}

func (cx *Scope) quitting(keep bool) bool {
	rt := cx.rt
	hs := slices.Clone(rt.quits)
	if len(hs) == 0 {
		return false
	}
	if !keep {
		rt.quits = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), QuitTimeout)
	defer cancel()
	for _, h := range hs {
		if h.owner.disposed || ctx.Err() != nil {
			continue
		}
		rt.guard(h.owner, func() { h.fn(ctx) })
	}
	return true
}
