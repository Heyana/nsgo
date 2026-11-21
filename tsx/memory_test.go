package tsx_test

import (
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
)

// memoryApp runs a sandboxed app whose press of "go" runs body, holding its built-ins to limit bytes, and returns it with what its boundary caught.
func memoryApp(t *testing.T, body string, limit int) (*nsgotest.App, *error) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("the page's engine keeps its own heap, which Limits.Memory does not hold")
	}
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, Text } from "nsgo";
const kept = [];
mount(() => <Text testID="go" onPress={() => { ` + body + ` }}>ok</Text>);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	limits := tsx.DefaultLimits
	limits.Call = 30 * time.Second
	limits.Memory = limit
	var failed error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Catch{Try: tsx.Sandboxed("App.tsx", code, tsx.Sandbox{Limits: &limits}), Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{Content: "cut"}
		}}
	})
	return app, &failed
}

// stoppedAtMemory fails t unless err is the memory quota's.
func stoppedAtMemory(t *testing.T, err error) {
	t.Helper()
	if core.ErrorCode(err) != core.CodeQuotaExceeded || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("ended with %v, want the memory quota", err)
	}
}

// TestSandboxMemoryStopsOneLargeAllocation stops an allocation past Limits.Memory before it is made, however large: one that takes more than the system has would end the host.
func TestSandboxMemoryStopsOneLargeAllocation(t *testing.T) {
	for _, body := range []string{`kept.push(new ArrayBuffer(512 * 1024 * 1024));`, `kept.push(new ArrayBuffer(2 ** 32));`} {
		app, failed := memoryApp(t, body, 1<<20)
		app.Press("id:go")
		stoppedAtMemory(t, *failed)
	}
}

// TestSandboxMemoryCountsWhatCallsKeep holds what a bundle keeps from one call to the next to Limits.Memory, rather than what each call adds.
func TestSandboxMemoryCountsWhatCallsKeep(t *testing.T) {
	app, failed := memoryApp(t, `kept.push(new Array(200_000).fill(1));`, 32<<20)
	for range 60 {
		app.Press("id:go")
		if *failed != nil {
			break
		}
	}
	stoppedAtMemory(t, *failed)
}

func TestSandboxMemoryCountsBlobCopies(t *testing.T) {
	app, failed := memoryApp(t, `const bytes = new TextEncoder().encode("界".repeat(4000)); kept.push(new Blob([bytes, bytes]));`, 48<<10)
	app.Press("id:go")
	stoppedAtMemory(t, *failed)
}

var hostSink [][]byte

// TestSandboxMemoryIgnoresTheHostsAllocations does not stop a bundle that allocates nothing while the host allocates past its Limits.Memory, here 256 MiB 50 ms into a call of a few hundred milliseconds.
func TestSandboxMemoryIgnoresTheHostsAllocations(t *testing.T) {
	app, failed := memoryApp(t, `let x = 0; for (let i = 0; i < 2_000_000; i++) x += i; kept.push(x);`, 64<<20)
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(50 * time.Millisecond)
		for range 256 {
			hostSink = append(hostSink, make([]byte, 1<<20))
		}
	}()
	app.Press("id:go")
	<-done
	hostSink = nil
	if *failed != nil {
		t.Fatalf("a bundle that allocates nothing ended with %v", *failed)
	}
}
