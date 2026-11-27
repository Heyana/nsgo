//go:build windows

package windows

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// TestActivateStartupFixture is opt-in: only the repository's isolated startup
// package may be activated. That fixture enables, queries, disables and queries
// its own StartupTask through the public desktop API, then writes its proof and
// exits. Its unsigned test package requires no change to certificate trust.
func TestActivateStartupFixture(t *testing.T) {
	aumid := os.Getenv("NSGO_STARTUP_FIXTURE_AUMID")
	if aumid == "" {
		t.Skip("needs the installed MSIX startup fixture")
	}
	if !strings.HasPrefix(aumid, "NSGo.Startup1389.Test_") || !strings.HasSuffix(aumid, "!App") {
		t.Fatal("the startup test may only activate its own fixture")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	comApartment()
	class := parseGUID("45ba127d-10a8-46ea-8ab7-56ea9078943c")
	iid := parseGUID("2e941141-7f97-4756-ba1d-9decde894a3d")
	var manager *comObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&class)), 0, clsctxInprocServer|4, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&manager)))
	if failed(hr) || manager == nil {
		t.Fatal(hresultError("ApplicationActivationManager", hr))
	}
	defer manager.release()
	var pid uint32
	if hr := manager.call(3, uintptr(unsafe.Pointer(wide(aumid))), uintptr(unsafe.Pointer(wide(""))), 2, uintptr(unsafe.Pointer(&pid))); failed(hr) {
		t.Fatal(hresultError("ActivateApplication", hr))
	}
	if pid == 0 {
		t.Fatal("activation returned no process")
	}
	t.Logf("activated own startup fixture, PID %d", pid)
}

type startupTestPlatform struct{ core.ApproxTextEngine }

func (startupTestPlatform) Mount(*driver.MountBatch)  {}
func (startupTestPlatform) RequestFrame()             {}
func (startupTestPlatform) SubmitGPU(driver.GPUFrame) {}

// TestStartupTaskRejectsUnpackaged runs the real WinRT load/error path without
// registering any login item. It verifies that native GetAsync failures finish
// queued reads and allow the next request to run.
func TestStartupTaskRejectsUnpackaged(t *testing.T) {
	if packaged() {
		t.Skip("needs an unpackaged test process")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := oleInitialize.Call(0)
	if failed(hr) && uint32(hr) != 0x80010106 {
		t.Fatal(hresultError("OleInitialize", hr))
	}
	if !failed(hr) {
		defer ole32.NewProc("OleUninitialize").Call()
	}
	factory := activationFactory(startupTaskClass, &iidStartupStatics)
	if factory == nil {
		t.Fatal("native StartupTask factory unavailable")
	}
	factory.release()
	var events []driver.Event
	e := engine.Start(startupTestPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, ev driver.Event) {
		if ev.Kind == driver.EventLaunchAtLogin {
			events = append(events, ev)
		}
	}})
	defer e.Close()
	w := &window{engine: e}
	c := &packagedStartup{root: w, requests: []startupRequest{{owner: w, id: 1}, {owner: w, id: 2}}}
	defer c.close()
	c.next()
	peek := user32.NewProc("PeekMessageW")
	for deadline := time.Now().Add(5 * time.Second); len(events) < 2; time.Sleep(time.Millisecond) {
		var m msg
		for {
			found, _, _ := peek.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
			if found == 0 {
				break
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&m)))
			dispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		w.runTasks()
		if time.Now().After(deadline) {
			t.Fatal("native StartupTask load stranded a read")
		}
	}
	if events[0].Count != 1 || events[1].Count != 2 || events[0].Finished || events[1].Finished || c.busy || len(c.requests) != 0 {
		t.Fatalf("unpackaged startup replies %v, busy %v", events, c.busy)
	}
}

func TestStartupIsEnabled(t *testing.T) {
	for _, tc := range []struct {
		state int32
		want  bool
	}{
		{0, false}, // Disabled
		{1, false}, // DisabledByUser
		{2, true},  // Enabled
		{3, false}, // DisabledByPolicy
		{4, true},  // EnabledByPolicy
		{-1, false},
		{5, false},
	} {
		if got := startupIsEnabled(tc.state); got != tc.want {
			t.Errorf("startupIsEnabled(%d) = %v, want %v", tc.state, got, tc.want)
		}
	}
}

func TestPackagedStartupFinishAndClose(t *testing.T) {
	var events []driver.Event
	e := engine.Start(startupTestPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, ev driver.Event) {
		if ev.Kind == driver.EventLaunchAtLogin {
			events = append(events, ev)
		}
	}})
	defer e.Close()
	live := &window{engine: e}
	gone := &window{engine: e, destroying: true}
	c := &packagedStartup{
		root: live,
		busy: true,
		requests: []startupRequest{
			{owner: live, id: 10},
			{owner: live, id: 11, set: true, on: true},
			{owner: gone, id: 12},
		},
	}
	// Finishing a single read request dispatches EventLaunchAtLogin; keep remaining
	// requests detached so next() does not invoke WinRT here.
	rest := c.requests[1:]
	c.requests = c.requests[:1]
	c.finish(true, nil)
	if len(events) != 1 || events[0].Count != 10 || !events[0].Finished {
		t.Fatalf("read event = %+v", events)
	}
	// A write request dispatches no read event.
	c.busy = true
	c.requests = rest[:1]
	c.finish(true, nil)
	if len(events) != 1 {
		t.Fatalf("write dispatched unexpected event: %+v", events)
	}
	// A destroying owner receives no read event.
	c.busy = true
	c.requests = rest[1:]
	c.finish(true, nil)
	if len(events) != 1 {
		t.Fatalf("destroying owner received event: %+v", events)
	}
	// Closing clears queued work and prevents finish/next from running later.
	c.requests = []startupRequest{{owner: live, id: 13}}
	c.close()
	c.finish(true, nil)
	c.next()
	if len(events) != 1 || c.busy || len(c.requests) != 0 {
		t.Fatalf("closed startup controller ran work: events=%+v busy=%v requests=%d", events, c.busy, len(c.requests))
	}
}
