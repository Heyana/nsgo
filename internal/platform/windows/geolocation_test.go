//go:build windows

package windows

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// sensorTestWindow is a window whose engine records the sensor reports the backend dispatches, and a function that returns the one it got.
func sensorTestWindow(t *testing.T) (*window, func() driver.SensorReport) {
	t.Helper()
	var mu sync.Mutex
	var reports []driver.SensorReport
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) {
			if r, ok := ev.Data.(*driver.SensorReport); ok && ev.Kind == driver.EventSensor {
				mu.Lock()
				reports = append(reports, *r)
				mu.Unlock()
			}
		},
	})
	t.Cleanup(eng.Close)
	return &window{engine: eng}, func() driver.SensorReport {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(reports) != 1 {
			t.Fatalf("sensor reports = %+v, want one", reports)
		}
		return reports[0]
	}
}

func TestGeolocationStopDropsStaleGeneration(t *testing.T) {
	w := &window{}
	run := &windowsGeoRun{w: w, id: 9, gen: 4}
	windowsGeoRuns.Lock()
	windowsGeoRuns.m[run] = struct{}{}
	windowsGeoRuns.Unlock()
	cancelWindowsLocation(w, 9, 3)
	if !run.active() {
		t.Fatal("a stop for an older generation canceled the current request")
	}
	cancelWindowsLocation(w, 9, 4)
	if run.active() {
		t.Fatal("matching stop left the request active")
	}
}

// fakeGeolocator is a Geolocator whose GetGeopositionAsync hands out an operation that never completes by itself, and whose IAsyncInfo.Cancel calls the completion handler as cancelled on the calling thread, as WinRT may.
type fakeGeolocator struct {
	locatorTable, opTable, infoTable [128]uintptr
	locator, op, info                comObject
	completion                       *comHandler
	puts, cancels, closes            atomic.Int32
	opReleases, locatorReleases      atomic.Int32
}

func newFakeGeolocator() *fakeGeolocator {
	f := &fakeGeolocator{}
	f.locator.vtbl, f.op.vtbl, f.info.vtbl = &f.locatorTable, &f.opTable, &f.infoTable
	one := syscall.NewCallback(func(uintptr) uintptr { return 1 })
	f.infoTable[1] = one
	f.infoTable[2] = syscall.NewCallback(func(uintptr) uintptr { return 0 })
	f.infoTable[9] = syscall.NewCallback(func(uintptr) uintptr {
		f.cancels.Add(1)
		if f.completion != nil {
			syscall.SyscallN(f.completion.vtbl[3], f.completion.ptr(), 0, 2)
		}
		return 0
	})
	f.infoTable[10] = syscall.NewCallback(func(uintptr) uintptr { f.closes.Add(1); return 0 })
	f.opTable[0] = syscall.NewCallback(func(_ uintptr, _ *guid, out **comObject) uintptr { *out = &f.info; return 0 })
	f.opTable[1] = one
	f.opTable[2] = syscall.NewCallback(func(uintptr) uintptr { f.opReleases.Add(1); return 0 })
	f.opTable[geoAsyncPutCompleted] = syscall.NewCallback(func(_ uintptr, handler *comHandler) uintptr {
		f.completion = handler
		handlerAddRef(handler)
		f.puts.Add(1)
		return 0
	})
	f.locatorTable[2] = syscall.NewCallback(func(uintptr) uintptr { f.locatorReleases.Add(1); return 0 })
	f.locatorTable[geolocatorGetPosition] = syscall.NewCallback(func(_ uintptr, out **comObject) uintptr { *out = &f.op; return 0 })
	return f
}

// TestGeolocationStopCancelsPendingFix stops a pending one-shot fix: the stop cancels and closes the operation and releases it and the locator once, and the completion that the cancel raises on the stopping thread neither deadlocks nor releases them again.
func TestGeolocationStopCancelsPendingFix(t *testing.T) {
	f := newFakeGeolocator()
	old := activateGeolocator
	activateGeolocator = func() *comObject { return &f.locator }
	defer func() { activateGeolocator = old }()
	w := &window{}
	w.sensorLocation(21, driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorLocate, Generation: 8})
	if f.completion == nil || f.puts.Load() != 1 {
		t.Fatalf("completion handler set %d times", f.puts.Load())
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		cancelWindowsLocation(w, 21, 8)
	}()
	<-done
	if f.cancels.Load() != 1 || f.closes.Load() != 1 || f.opReleases.Load() != 1 || f.locatorReleases.Load() != 1 {
		t.Fatalf("cancels=%d closes=%d opReleases=%d locatorReleases=%d", f.cancels.Load(), f.closes.Load(), f.opReleases.Load(), f.locatorReleases.Load())
	}
	// A late completion after the stop finds the run gone.
	syscall.SyscallN(f.completion.vtbl[3], f.completion.ptr(), 0, asyncStatusCompleted)
	handlerRelease(f.completion)
	if f.opReleases.Load() != 1 || f.locatorReleases.Load() != 1 {
		t.Fatalf("late completion released again: operation=%d locator=%d", f.opReleases.Load(), f.locatorReleases.Load())
	}
}

// TestGeolocationRefusesBackground answers background location unavailable without activating a locator: a desktop app has no background state.
func TestGeolocationRefusesBackground(t *testing.T) {
	old := activateGeolocator
	activateGeolocator = func() *comObject { t.Fatal("activated a locator"); return nil }
	defer func() { activateGeolocator = old }()
	for _, req := range []driver.SensorRequest{
		{Kind: driver.SensorBackgroundLocation, Command: driver.SensorReadPermission},
		{Kind: driver.SensorBackgroundLocation, Command: driver.SensorRequestPermission},
		{Kind: driver.SensorLocation, Command: driver.SensorWatchLocation, Location: driver.LocationOptions{Background: true}},
		{Kind: driver.SensorMotion, Command: driver.SensorWatchMotion},
	} {
		w, got := sensorTestWindow(t)
		w.sensorLocation(3, req)
		r := got()
		if req.Command <= driver.SensorRequestPermission {
			if r.Grant == nil || r.Grant.Authorization != driver.SensorUnavailable {
				t.Errorf("%+v answered %+v", req, r)
			}
		} else if r.Error != "unavailable" || !r.End {
			t.Errorf("%+v answered %+v", req, r)
		}
	}
}

// TestWindowsGeolocatorAgainstTheSystem reads the permission, takes a fix and starts and stops a stream through the system's Geolocator, which an ABI mistake fails at once rather than through the operation's completion; it answers whatever this machine's location settings allow. It opts in with NSGO_WINDOWS_LOCATION_NATIVE=1, as a machine with location on reports where it is.
func TestWindowsGeolocatorAgainstTheSystem(t *testing.T) {
	if os.Getenv("NSGO_WINDOWS_LOCATION_NATIVE") != "1" {
		t.Skip("set NSGO_WINDOWS_LOCATION_NATIVE=1 to ask this machine's location service")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// comApartment initializes only the first thread that calls it, which another test may have been; this one stands for the window's thread.
	oleInitialize.Call(0)
	w, got := sensorTestWindow(t)
	w.sensorLocation(1, driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorReadPermission})
	t.Logf("permission: %+v", *got().Grant)
	if locator := activateGeolocator(); locator == nil {
		t.Log("Geolocator did not activate")
	} else {
		status, ok := windowsLocationStatus(locator)
		locator.release()
		t.Logf("LocationStatus: %d, %v", status, ok)
	}

	w, got = sensorTestWindow(t)
	w.sensorLocation(4, driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorRequestPermission})
	waitWindowsGeo(t, w, func() bool { return false })
	t.Logf("request: %+v", *got().Grant)

	w, got = sensorTestWindow(t)
	w.sensorLocation(2, driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorLocate, Generation: 1})
	waitWindowsGeo(t, w, func() bool { return windowsGeoRunCount() == 0 })
	r := got()
	switch {
	case r.Location != nil:
		t.Logf("fix: %+v", *r.Location)
	case r.Error == "not-allowed" || r.Error == "unavailable":
		t.Logf("fix failed through its completion: %s", r.Error)
	default:
		t.Fatalf("Locate answered %+v", r)
	}

	w, _ = sensorTestWindow(t)
	w.sensorLocation(3, driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorWatchLocation, Generation: 1})
	t.Logf("watching: %v", windowsGeoRunCount() == 1)
	cancelWindowsLocation(w, 3, 1)
	if n := windowsGeoRunCount(); n != 0 {
		t.Fatalf("%d runs left after the stop", n)
	}
}

func windowsGeoRunCount() int {
	windowsGeoRuns.Lock()
	defer windowsGeoRuns.Unlock()
	return len(windowsGeoRuns.m)
}

// waitWindowsGeo runs the first tasks a completion posts to w, or returns once answered says the request was answered at once.
func waitWindowsGeo(t *testing.T, w *window, answered func() bool) {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for {
		w.mu.Lock()
		pending := len(w.tasks)
		w.mu.Unlock()
		if pending > 0 {
			w.runTasks()
			return
		}
		if answered() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the operation did not complete")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
