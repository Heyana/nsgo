package linux

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	fields := bytes.Fields(buf[:n])
	if len(fields) < 2 {
		return 0
	}
	id, _ := strconv.ParseUint(string(fields[1]), 10, 64)
	return id
}

// testGTKHost mirrors testWindowCalls + window.run + loopCalls in native_test.go:
// one locked OS thread serves sequential test windows, each running its own
// serveLoop over callQueue and a wakeup signal.
type testGTKHost struct {
	calls chan func()
	queue callQueue
	wake  chan struct{}
	gid   uint64
}

func startTestGTKHost() *testGTKHost {
	h := &testGTKHost{
		calls: make(chan func()),
		wake:  make(chan struct{}, 1),
	}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		h.gid = currentGoroutineID()
		close(ready)
		serveLoop(func() bool {
			fn, ok := <-h.calls
			if !ok {
				return false
			}
			fn()
			return true
		})
	}()
	<-ready
	return h
}

func (h *testGTKHost) post(fn func()) {
	h.queue.push(fn)
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

type fakeGTKWindow struct {
	host      *testGTKHost
	closing   atomic.Bool
	cleanedUp atomic.Bool
}

func (h *testGTKHost) startWindow(t *testing.T) *fakeGTKWindow {
	t.Helper()
	w := &fakeGTKWindow{host: h}
	ready := make(chan struct{})
	done := make(chan struct{})
	open := func() {
		defer close(done)
		defer func() { w.cleanedUp.Store(true) }()
		close(ready)
		serveLoop(func() bool {
			h.queue.drain()
			if w.closing.Load() {
				return false
			}
			<-h.wake
			h.queue.drain()
			return !w.closing.Load()
		})
	}
	select {
	case h.calls <- open:
	case <-time.After(5 * time.Second):
		t.Fatal("the previous GTK window did not stop")
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the window did not start")
	}
	t.Cleanup(func() {
		if !w.closing.Load() {
			w.onLoop(t, func() { w.close() })
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
		if !w.cleanedUp.Load() {
			t.Error("the window's cleanup defer did not run")
		}
	})
	return w
}

func (w *fakeGTKWindow) close() {
	w.closing.Store(true)
	select {
	case w.host.wake <- struct{}{}:
	default:
	}
}

func (w *fakeGTKWindow) onLoop(t *testing.T, fn func()) {
	t.Helper()
	if !runOnThread(w.host.post, 5*time.Second, "panic on the GTK thread", fn) {
		t.Fatal("the GTK thread did not run the step")
	}
}

var (
	sharedGTKHostOnce sync.Once
	sharedGTKHost     *testGTKHost
)

func getSharedGTKHost() *testGTKHost {
	sharedGTKHostOnce.Do(func() {
		sharedGTKHost = startTestGTKHost()
	})
	return sharedGTKHost
}

func TestOnLoopExitPropagation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"normal", "fatal", "failnow", "skip", "panic", "goexit", "fatal-then-panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable,
				"-test.run=^TestOnLoop(Exit|ExitAgain|Survivor)Child$", "-test.v", "-test.timeout=5s")
			cmd.Env = append(os.Environ(), "NSGO_LINUX_LOOP_EXIT_CASE="+mode)
			output, err := cmd.CombinedOutput()
			text := string(output)
			switch mode {
			case "normal":
				if err != nil || !strings.Contains(text, "PASS: TestOnLoopExitChild") || !strings.Contains(text, "ON_LOOP_NORMAL_DONE") {
					t.Fatalf("normal return did not pass: %v\n%s", err, text)
				}
			case "skip":
				if err != nil || !strings.Contains(text, "SKIP: TestOnLoopExitChild") {
					t.Fatalf("skip was not reported: %v\n%s", err, text)
				}
			default:
				if err == nil || !strings.Contains(text, "gtk-thread "+mode) {
					t.Fatalf("failure was not reported: %v\n%s", err, text)
				}
			}
			if strings.Contains(text, "ON_LOOP_CONTINUED") || strings.Contains(text, "test timed out") || ctx.Err() != nil {
				t.Fatalf("the test continued after an exit or hung: %v\n%s", err, text)
			}
			if mode != "panic" && mode != "goexit" && mode != "fatal-then-panic" && !strings.Contains(text, "PASS: TestOnLoopSurvivorChild") {
				t.Fatalf("later test did not run on the shared GTK thread: %v\n%s", err, text)
			}
			if (mode == "panic" || mode == "fatal-then-panic") && !strings.Contains(text, "exitOnLoopChild.func1") {
				t.Fatalf("panic lost the originating GTK-thread stack:\n%s", text)
			}
			// A panic on the GTK thread after an earlier exit still reaches its own test: the earlier exit left serveLoop serving from a deferred call.
			if mode == "fatal-then-panic" && (!strings.Contains(text, "--- FAIL: TestOnLoopExitChild") || !strings.Contains(text, "gtk-thread fatal-then-panic again")) {
				t.Fatalf("the panic after an exit was not reported:\n%s", text)
			}
		})
	}
}

func TestOnLoopExitChild(t *testing.T)      { exitOnLoopChild(t, false) }
func TestOnLoopExitAgainChild(t *testing.T) { exitOnLoopChild(t, true) }

func exitOnLoopChild(t *testing.T, again bool) {
	mode := os.Getenv("NSGO_LINUX_LOOP_EXIT_CASE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	h := getSharedGTKHost()
	w := h.startWindow(t)
	var ranOn uint64
	w.onLoop(t, func() {
		ranOn = currentGoroutineID()
		switch mode {
		case "normal":
			return
		case "fatal-then-panic":
			if again {
				panic("gtk-thread fatal-then-panic again")
			}
			t.Fatal("gtk-thread fatal-then-panic")
		case "fatal":
			t.Fatal("gtk-thread fatal")
		case "failnow":
			t.Log("gtk-thread failnow")
			t.FailNow()
		case "skip":
			t.Skip("gtk-thread skip")
		case "panic":
			panic("gtk-thread panic")
		case "goexit":
			t.Log("gtk-thread goexit")
			runtime.Goexit()
		}
	})
	if mode == "normal" {
		if ranOn != h.gid {
			t.Fatalf("closure ran on goroutine %d, want %d", ranOn, h.gid)
		}
		t.Log("ON_LOOP_NORMAL_DONE")
		return
	}
	t.Error("ON_LOOP_CONTINUED")
}

func TestOnLoopSurvivorChild(t *testing.T) {
	if os.Getenv("NSGO_LINUX_LOOP_EXIT_CASE") == "" {
		t.Skip("subprocess helper")
	}
	h := getSharedGTKHost()
	w := h.startWindow(t)
	var ranOn uint64
	w.onLoop(t, func() {
		ranOn = currentGoroutineID()
	})
	if ranOn != h.gid {
		t.Fatalf("later window ran on goroutine %d, want shared GTK goroutine %d", ranOn, h.gid)
	}
}

func TestOnLoopCallQueuePreservesRemainingCallsOnExit(t *testing.T) {
	h := startTestGTKHost()
	defer close(h.calls)
	w := h.startWindow(t)

	// Hold the loop in a barrier call so two calls accumulate in the same batch.
	entered := make(chan struct{})
	release := make(chan struct{})
	h.post(func() {
		close(entered)
		<-release
	})
	<-entered

	exitedCaller := make(chan bool, 1)
	go func() {
		continued := false
		defer func() { exitedCaller <- !continued }()
		w.onLoop(t, func() {
			runtime.Goexit()
		})
		continued = true
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		h.queue.mu.Lock()
		n := len(h.queue.fns)
		h.queue.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exiting call was not queued")
		}
		time.Sleep(time.Millisecond)
	}

	survivorDone := make(chan uint64, 1)
	h.post(func() {
		survivorDone <- currentGoroutineID()
	})
	close(release)

	select {
	case ok := <-exitedCaller:
		if !ok {
			t.Fatal("caller continued after runtime.Goexit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller hung after runtime.Goexit")
	}

	select {
	case gid := <-survivorDone:
		if gid != h.gid {
			t.Fatalf("survivor call ran on goroutine %d, want %d", gid, h.gid)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sibling call in the same batch was dropped after Goexit")
	}

	var followUp uint64
	w.onLoop(t, func() {
		followUp = currentGoroutineID()
	})
	if followUp != h.gid {
		t.Fatalf("subsequent call ran on goroutine %d, want %d", followUp, h.gid)
	}
}

func TestOnLoopPropagatesPanicAndKeepsLoopAlive(t *testing.T) {
	h := startTestGTKHost()
	defer close(h.calls)
	w := h.startWindow(t)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		w.onLoop(t, func() {
			panic("boom")
		})
	}()
	if recovered != "boom" {
		t.Fatalf("recovered %v, want %q", recovered, "boom")
	}

	var gid uint64
	w.onLoop(t, func() {
		gid = currentGoroutineID()
	})
	if gid != h.gid {
		t.Fatalf("loop goroutine changed: got %d, want %d", gid, h.gid)
	}
}

func TestOnLoopTimeout(t *testing.T) {
	ok := runOnThread(func(func()) {}, 30*time.Millisecond, "panic on the GTK thread", func() {})
	if ok {
		t.Fatal("runOnThread reported completion when the posted closure never ran")
	}
}
