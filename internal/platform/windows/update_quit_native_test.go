//go:build windows && !nsgo_steam

package windows

import (
	"context"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func bindTestUpdateQuit(t *testing.T, root uintptr) *updateQuit {
	t.Helper()
	var gate, previous *updateQuit
	onMain(t, root, func(w *window) {
		gate = newUpdateQuit(w)
		previous = updateQuitOwner.Swap(gate)
	})
	t.Cleanup(func() {
		gate.close()
		updateQuitOwner.CompareAndSwap(gate, previous)
	})
	return gate
}

func askTestUpdateQuit() <-chan uintptr {
	result := make(chan uintptr, 1)
	go func() {
		allowed, _, _ := syscall.SyscallN(canShutdown)
		result <- allowed
	}()
	return result
}

func awaitTestUpdateQuit(t *testing.T, result <-chan uintptr) uintptr {
	t.Helper()
	select {
	case allowed := <-result:
		return allowed
	case <-time.After(5 * time.Second):
		t.Fatal("WinSparkle did not receive the quit decision")
		return 0
	}
}

func awaitTestUpdateAnswer(t *testing.T, answers <-chan func(bool)) func(bool) {
	t.Helper()
	select {
	case answer := <-answers:
		return answer
	case <-time.After(5 * time.Second):
		t.Fatal("the updater did not ask the app's quit handler")
		return func(bool) {}
	}
}

func TestWinSparkleQuitWaitsForApproval(t *testing.T) {
	var requests, cleaned atomic.Int32
	answers := make(chan func(bool), 2)
	root, done := runQuitTestApp(t, "nsgo-update-quit", core.AppRoot(func(cx *core.Scope) {
		cx.OnQuitRequest(func(q *core.QuitRequest) {
			if requests.Add(1) == 1 {
				q.Cancel()
			} else {
				answers <- q.Defer()
			}
		})
		cx.OnQuit(func(context.Context) { cleaned.Add(1) })
	}))
	gate := bindTestUpdateQuit(t, root)
	if got := awaitTestUpdateQuit(t, askTestUpdateQuit()); got != 0 {
		t.Fatalf("canceled app allowed installation: %d", got)
	}
	deferred := askTestUpdateQuit()
	answer := awaitTestUpdateAnswer(t, answers)
	select {
	case got := <-deferred:
		t.Fatalf("pending decision returned %d", got)
	case <-time.After(50 * time.Millisecond):
	}
	var oldID uint32
	onMain(t, root, func(*window) { oldID = gate.id })
	answer(false)
	if got := awaitTestUpdateQuit(t, deferred); got != 0 {
		t.Fatalf("deferred cancellation allowed installation: %d", got)
	}
	approved := askTestUpdateQuit()
	answer = awaitTestUpdateAnswer(t, answers)
	onMain(t, root, func(w *window) {
		if !answerUpdateQuit(w, &driver.MountItem{Count: oldID, On: true}) {
			t.Error("a stale updater answer became an ordinary quit")
		}
	})
	select {
	case got := <-approved:
		t.Fatalf("stale answer approved the next request: %d", got)
	case <-time.After(50 * time.Millisecond):
	}
	answer(true)
	if got := awaitTestUpdateQuit(t, approved); got != 1 || cleaned.Load() != 0 {
		t.Fatalf("approval=%d, early cleanups=%d", got, cleaned.Load())
	}
	// If installer launch failed after approval, a later installation
	// attempt must ask again rather than reuse a stale decision.
	retry := askTestUpdateQuit()
	awaitTestUpdateAnswer(t, answers)(false)
	if got := awaitTestUpdateQuit(t, retry); got != 0 {
		t.Fatalf("later canceled attempt reused an old approval: %d", got)
	}
	retry = askTestUpdateQuit()
	awaitTestUpdateAnswer(t, answers)(true)
	if got := awaitTestUpdateQuit(t, retry); got != 1 {
		t.Fatalf("later approved attempt returned %d", got)
	}
	// WinSparkle has now launched its installer and asks for the approved
	// shutdown. It must not run another cancelable quit request.
	syscall.SyscallN(shutdownRequest)
	syscall.SyscallN(shutdownRequest)
	waitQuitTestApp(t, done)
	if requests.Load() != 5 || cleaned.Load() != 1 {
		t.Fatalf("requests=%d, cleanups=%d", requests.Load(), cleaned.Load())
	}
}

func TestWinSparkleQuitCleanupReleasesTheCallback(t *testing.T) {
	answers := make(chan func(bool), 1)
	root, done := runQuitTestApp(t, "nsgo-update-cleanup", core.AppRoot(func(cx *core.Scope) {
		cx.OnQuitRequest(func(q *core.QuitRequest) { answers <- q.Defer() })
	}))
	bindTestUpdateQuit(t, root)
	result := askTestUpdateQuit()
	answer := awaitTestUpdateAnswer(t, answers)
	onMain(t, root, func(*window) { stopWinSparkle() })
	if got := awaitTestUpdateQuit(t, result); got != 0 {
		t.Fatalf("closed updater allowed installation: %d", got)
	}
	answer(true)
	// A delayed response after cleanup must also be ignored.
	select {
	case <-done:
		t.Fatal("a closed updater's delayed answer quit the app")
	case <-time.After(100 * time.Millisecond):
	}
}
