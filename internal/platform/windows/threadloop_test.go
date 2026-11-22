package windows

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

type testWindowLoop struct {
	mu    sync.Mutex
	tasks []func()
	wake  chan struct{}
	quit  chan struct{}
	done  chan struct{}
	once  sync.Once
	gid   uint64
}

func startTestWindowLoop() *testWindowLoop {
	l := &testWindowLoop{
		wake: make(chan struct{}, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(l.done)
		l.gid = currentGoroutineID()
		close(ready)
		serveLoop(func() bool {
			select {
			case <-l.quit:
				return false
			case <-l.wake:
				drainTasks(&l.mu, &l.tasks, l.signalWake)
				return true
			}
		})
	}()
	<-ready
	return l
}

func (l *testWindowLoop) signalWake() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *testWindowLoop) post(fn func()) {
	l.mu.Lock()
	wake := len(l.tasks) == 0
	l.tasks = append(l.tasks, fn)
	l.mu.Unlock()
	if wake {
		l.signalWake()
	}
}

func (l *testWindowLoop) close() {
	l.once.Do(func() { close(l.quit) })
	<-l.done
}

var (
	sharedWindowLoopOnce sync.Once
	sharedWindowLoop     *testWindowLoop
)

func getSharedWindowLoop() *testWindowLoop {
	sharedWindowLoopOnce.Do(func() {
		sharedWindowLoop = startTestWindowLoop()
	})
	return sharedWindowLoop
}

func TestOnMainExitPropagation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"normal", "fatal", "failnow", "skip", "panic", "goexit", "fatal-then-panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable,
				"-test.run=^TestOnMain(Exit|ExitAgain|Survivor)Child$", "-test.v", "-test.timeout=5s")
			cmd.Env = append(os.Environ(), "NSGO_WIN_MAIN_EXIT_CASE="+mode)
			output, err := cmd.CombinedOutput()
			text := string(output)
			switch mode {
			case "normal":
				if err != nil || !strings.Contains(text, "PASS: TestOnMainExitChild") || !strings.Contains(text, "ON_MAIN_NORMAL_DONE") {
					t.Fatalf("normal return did not pass: %v\n%s", err, text)
				}
			case "skip":
				if err != nil || !strings.Contains(text, "SKIP: TestOnMainExitChild") {
					t.Fatalf("skip was not reported: %v\n%s", err, text)
				}
			default:
				if err == nil || !strings.Contains(text, "window-thread "+mode) {
					t.Fatalf("failure was not reported: %v\n%s", err, text)
				}
			}
			if strings.Contains(text, "ON_MAIN_CONTINUED") || strings.Contains(text, "test timed out") || ctx.Err() != nil {
				t.Fatalf("the test continued after an exit or hung: %v\n%s", err, text)
			}
			if mode != "panic" && mode != "goexit" && mode != "fatal-then-panic" && !strings.Contains(text, "PASS: TestOnMainSurvivorChild") {
				t.Fatalf("later test did not run on the window thread: %v\n%s", err, text)
			}
			if (mode == "panic" || mode == "fatal-then-panic") && !strings.Contains(text, "exitOnMainChild.func1") {
				t.Fatalf("panic lost the originating window-thread stack:\n%s", text)
			}
			// A panic on the window thread after an earlier exit still reaches its own test: the earlier exit left serveLoop serving from a deferred call.
			if mode == "fatal-then-panic" && (!strings.Contains(text, "--- FAIL: TestOnMainExitChild") || !strings.Contains(text, "window-thread fatal-then-panic again")) {
				t.Fatalf("the panic after an exit was not reported:\n%s", text)
			}
		})
	}
}

func TestOnMainExitChild(t *testing.T)      { exitOnMainChild(t, false) }
func TestOnMainExitAgainChild(t *testing.T) { exitOnMainChild(t, true) }

func exitOnMainChild(t *testing.T, again bool) {
	mode := os.Getenv("NSGO_WIN_MAIN_EXIT_CASE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	l := getSharedWindowLoop()
	var ranOn uint64
	if !runOnThread(l.post, 5*time.Second, "panic on the window thread", func() {
		ranOn = currentGoroutineID()
		switch mode {
		case "normal":
			return
		case "fatal-then-panic":
			if again {
				panic("window-thread fatal-then-panic again")
			}
			t.Fatal("window-thread fatal-then-panic")
		case "fatal":
			t.Fatal("window-thread fatal")
		case "failnow":
			t.Log("window-thread failnow")
			t.FailNow()
		case "skip":
			t.Skip("window-thread skip")
		case "panic":
			panic("window-thread panic")
		case "goexit":
			t.Log("window-thread goexit")
			runtime.Goexit()
		}
	}) {
		t.Fatal("the window's thread did not answer")
	}
	if mode == "normal" {
		if ranOn != l.gid {
			t.Fatalf("closure ran on goroutine %d, want %d", ranOn, l.gid)
		}
		t.Log("ON_MAIN_NORMAL_DONE")
		return
	}
	t.Error("ON_MAIN_CONTINUED")
}

func TestOnMainSurvivorChild(t *testing.T) {
	if os.Getenv("NSGO_WIN_MAIN_EXIT_CASE") == "" {
		t.Skip("subprocess helper")
	}
	l := getSharedWindowLoop()
	var ranOn uint64
	if !runOnThread(l.post, 5*time.Second, "panic on the window thread", func() {
		ranOn = currentGoroutineID()
	}) {
		t.Fatal("the window's thread did not answer")
	}
	if ranOn != l.gid {
		t.Fatalf("later work ran on goroutine %d, want original loop goroutine %d", ranOn, l.gid)
	}
}

func TestOnMainDrainPreservesRemainingTasksOnExit(t *testing.T) {
	l := startTestWindowLoop()
	defer l.close()

	// Hold the loop in a barrier task so two tasks accumulate in the same batch.
	entered := make(chan struct{})
	release := make(chan struct{})
	l.post(func() {
		close(entered)
		<-release
	})
	<-entered

	exitedCaller := make(chan bool, 1)
	go func() {
		continued := false
		defer func() { exitedCaller <- !continued }()
		if !runOnThread(l.post, 5*time.Second, "panic on the window thread", func() {
			runtime.Goexit()
		}) {
			t.Error("runOnThread timed out")
		}
		continued = true
	}()

	// Wait until the exiting task is queued before posting the sibling task in the same batch.
	deadline := time.Now().Add(2 * time.Second)
	for {
		l.mu.Lock()
		n := len(l.tasks)
		l.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exiting task was not queued")
		}
		time.Sleep(time.Millisecond)
	}

	survivorDone := make(chan uint64, 1)
	l.post(func() {
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
		if gid != l.gid {
			t.Fatalf("survivor task ran on goroutine %d, want %d", gid, l.gid)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sibling task in the same batch was dropped after Goexit")
	}

	var followUp uint64
	if !runOnThread(l.post, 2*time.Second, "panic on the window thread", func() {
		followUp = currentGoroutineID()
	}) {
		t.Fatal("subsequent runOnThread timed out")
	}
	if followUp != l.gid {
		t.Fatalf("subsequent call ran on goroutine %d, want %d", followUp, l.gid)
	}
}

func TestOnMainPropagatesPanicAndKeepsLoopAlive(t *testing.T) {
	l := startTestWindowLoop()
	defer l.close()

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		runOnThread(l.post, 2*time.Second, "panic on the window thread", func() {
			panic("boom")
		})
	}()
	if recovered != "boom" {
		t.Fatalf("recovered %v, want %q", recovered, "boom")
	}

	var gid uint64
	if !runOnThread(l.post, 2*time.Second, "panic on the window thread", func() {
		gid = currentGoroutineID()
	}) {
		t.Fatal("loop did not respond after a recovered panic")
	}
	if gid != l.gid {
		t.Fatalf("loop goroutine changed: got %d, want %d", gid, l.gid)
	}
}

func TestOnMainTimeout(t *testing.T) {
	ok := runOnThread(func(func()) {}, 30*time.Millisecond, "panic on the window thread", func() {})
	if ok {
		t.Fatal("runOnThread reported completion when the posted closure never ran")
	}
}
