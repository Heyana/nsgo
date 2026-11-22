package windows

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// serveLoop calls step repeatedly on the current goroutine until it returns
// false. If a task running inside step calls runtime.Goexit (such as
// testing.T.Fatal, FailNow, or SkipNow), the deferred restart keeps serving on
// the same goroutine and locked OS thread rather than letting the window's
// thread exit.
func serveLoop(step func() bool) {
	finished := false
	defer func() {
		if value := recover(); value != nil {
			panic(value)
		}
		if !finished {
			// Goexit still runs defers. Keep serving on this same goroutine and locked OS thread: replacing it would lose the window's thread.
			serveLoop(step)
		}
	}()
	for step() {
	}
	finished = true
}

// drainTasks runs the tasks currently queued in *tasks in order. If a task
// exits early (for example via runtime.Goexit), the remaining tasks in the
// batch are placed back ahead of any newly posted tasks and wake is called so
// the loop resumes them on its next turn.
func drainTasks(mu *sync.Mutex, tasks *[]func(), wake func()) {
	mu.Lock()
	fns := *tasks
	*tasks = nil
	mu.Unlock()
	next := 0
	defer func() {
		if next >= len(fns) {
			return
		}
		mu.Lock()
		needWake := len(*tasks) == 0
		*tasks = append(fns[next:], *tasks...)
		mu.Unlock()
		if needWake && wake != nil {
			wake()
		}
	}()
	for next < len(fns) {
		fn := fns[next]
		next++
		fn()
	}
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
		// Fatal/FailNow/SkipNow exited the main call; finish the test on its own goroutine too, before it can execute another statement.
		runtime.Goexit()
	}
	return true
}
