package linux

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// callQueue holds closures posted to the GTK thread outside GLib callbacks so
// a test exit (runtime.Goexit) unwinds only Go frames and serveLoop can keep
// serving on the same locked OS thread.
type callQueue struct {
	mu  sync.Mutex
	fns []func()
}

func (q *callQueue) push(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

func (q *callQueue) drain() {
	q.mu.Lock()
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	next := 0
	defer func() {
		if next >= len(fns) {
			return
		}
		// A closure exited before the rest of this batch ran; keep the rest ahead of newer calls.
		q.mu.Lock()
		q.fns = append(fns[next:], q.fns...)
		q.mu.Unlock()
	}()
	for next < len(fns) {
		fn := fns[next]
		next++
		fn()
	}
}

// serveLoop calls step repeatedly on the current goroutine until it returns
// false. If a closure running inside step calls runtime.Goexit (such as
// testing.T.Fatal, FailNow, or SkipNow), the deferred restart keeps serving on
// the same goroutine and locked OS thread rather than letting GTK's thread exit.
func serveLoop(step func() bool) {
	finished := false
	defer func() {
		if value := recover(); value != nil {
			panic(value)
		}
		if !finished {
			// Goexit still runs defers. Keep serving on this same goroutine and locked OS thread: replacing it would lose GTK's thread.
			serveLoop(step)
		}
	}()
	for step() {
	}
	finished = true
}

// runOnThread posts fn via post, waits up to timeout for it to finish, and
// propagates panics and runtime.Goexit (Fatal/FailNow/SkipNow) back to the
// caller's goroutine. It returns false only when timeout expires before fn
// finishes.
func runOnThread(post func(func()), timeout time.Duration, panicHeader string, fn func()) bool {
	done := make(chan struct{})
	returned := false
	var failure any
	var stack []byte
	post(func() {
		defer func() {
			failure = recover()
			if failure != nil {
				stack = debug.Stack()
			}
			close(done)
		}()
		fn()
		returned = true
	})
	select {
	case <-done:
	case <-time.After(timeout):
		return false
	}
	if failure != nil {
		fmt.Fprintf(os.Stderr, "%s:\n%s", panicHeader, stack)
		panic(failure)
	}
	if !returned {
		// Fatal/FailNow/SkipNow exited the loop call; finish the test on its own goroutine too, before it can execute another statement.
		runtime.Goexit()
	}
	return true
}
