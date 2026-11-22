//go:build windows

package windows

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This owned Win32 frame is a stand-in for the unresolved/suspended UWP case, not an actual packaged application. The real window property store must keep its app ID without attributing it to the frame host.
func TestFrontmostFramePropertyDoesNotBecomeTheHost(t *testing.T) {
	defer apartmentThread()()
	instance, _, _ := getModuleHandleW.Call(0)
	name := wide("ApplicationFrameWindow")
	class := wndClassEx{WndProc: defWindowProcW.Addr(), Instance: instance, ClassName: name}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, err := registerClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		t.Fatalf("register the owned frame class: %v", err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), instance)
	hwnd, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(wide("NSGo 1393 Metadata Fixture"))), wsOverlappedWindow, 40, 40, 240, 160, 0, 0, instance, 0)
	if hwnd == 0 {
		t.Fatalf("create the owned frame: %v", err)
	}
	defer destroyWindow.Call(hwnd)
	var store *comObject
	hr, _, _ := shGetPropertyStoreForWindow.Call(hwnd, uintptr(unsafe.Pointer(&iidPropertyStore)), uintptr(unsafe.Pointer(&store)))
	if failed(hr) || store == nil {
		t.Fatalf("the owned frame has no native property store: %#x", uint32(hr))
	}
	defer store.release()
	defer func() {
		var empty propVariant
		store.call(propertyStoreSetValue, uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&empty)))
	}()
	last := driver.FrontmostAppInfo{PID: os.Getpid(), ID: "stale host", Name: "stale host"}
	for _, id := range []string{"dev.nsgo.frontmost1393.First", "dev.nsgo.frontmost1393.Second"} {
		words := syscall.StringToUTF16(id)
		value := propVariant{vt: 31, value: unsafe.Pointer(&words[0])}
		if hr := store.call(propertyStoreSetValue, uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&value))); failed(hr) {
			t.Fatalf("set native app ID: %#x", uint32(hr))
		}
		if got := windowApplicationID(hwnd); got != id {
			t.Fatalf("native frame ID=%q, want %q", got, id)
		}
		got := readWindowFrontmost(hwnd, last)
		if got.ID != id || got.PID != 0 || strings.Contains(strings.ToLower(got.Name), "applicationframehost") {
			t.Fatalf("unresolved frame attributed to its host: %+v", got)
		}
		last = got
	}
}

func TestFrontmostUnpackagedProcessKeepsItsPath(t *testing.T) {
	pid := uint32(os.Getpid())
	if id := processApplicationID(pid); id != "" {
		t.Skip("the native test binary itself is packaged")
	}
	path := processPath(pid)
	if path == "" {
		t.Fatal("cannot read the owned process path")
	}
	own, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(path, own) {
		t.Fatalf("owned process path=%q, want %q", path, own)
	}
	if id := processApplicationID(0); id != "" {
		t.Fatalf("PID 0 had an application identity: %q", id)
	}
}

// The interactive session must show the owned player and allow it to become foreground. This asserts actual Win32/Media Foundation state, including maximized -> borderless fullscreen with IsZoomed still set.
func TestFrontmostTracksOwnedFullscreenPlayer(t *testing.T) {
	if os.Getenv("NSGO_FRONTMOST_NATIVE") != "1" {
		t.Skip("set NSGO_FRONTMOST_NATIVE=1 in an owned interactive Windows desktop")
	}
	testFrontmostOwnedPlayer(t, true)
}

func TestFrontmostReadsOwnedFullscreenPlayer(t *testing.T) {
	if os.Getenv("NSGO_FRONTMOST_NATIVE_WINDOW_ONLY") != "1" {
		t.Skip("set NSGO_FRONTMOST_NATIVE_WINDOW_ONLY=1 for owned window-state acceptance")
	}
	testFrontmostOwnedPlayer(t, false)
}

// Window-state acceptance can run without foreground access. The separate tracked test additionally requires actual OS foreground and public signals.
func testFrontmostOwnedPlayer(t *testing.T, tracked bool) {
	t.Helper()
	source := testVideo(t)
	const title = "NSGo 1393 Frontmost Player"
	var played atomic.Bool
	failures := make(chan error, 1)
	var mu sync.Mutex
	var latest core.FrontmostAppInfo
	hwnd := startAppWith(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { app := cx.FrontmostApp(); mu.Lock(); latest = app; mu.Unlock() })
		return core.Video{Source: source, Muted: true, Repeat: true, Controls: true, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			OnProgress: func(p, _ time.Duration) {
				if p > 0 {
					played.Store(true)
				}
			},
			OnError: func(err error) {
				select {
				case failures <- err:
				default:
				}
			},
		}
	}, Options{ID: "dev.nsgo.frontmost1393", Title: title, Width: 480, Height: 300})
	onMain(t, hwnd, func(*window) { showWindow.Call(hwnd, swShow); setForegroundWindow.Call(hwnd) })
	wait := func(what string, condition func() bool) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			select {
			case err := <-failures:
				t.Fatalf("owned player: %v", err)
			default:
			}
			if condition() {
				return
			}
		}
		front, _, _ := getForegroundWindow.Call()
		visible, _, _ := isWindowVisible.Call(hwnd)
		mu.Lock()
		last := latest
		mu.Unlock()
		var native driver.FrontmostAppInfo
		onMain(t, hwnd, func(*window) { native = readWindowFrontmost(hwnd, driver.FrontmostAppInfo{}) })
		t.Fatalf("the owned player did not reach %s; hwnd=%#x visible=%v foreground=%#x signal=%+v native=%+v", what, hwnd, visible != 0, front, last, native)
	}
	seen := func(full bool) bool {
		if !tracked {
			var app driver.FrontmostAppInfo
			onMain(t, hwnd, func(*window) { app = readWindowFrontmost(hwnd, driver.FrontmostAppInfo{}) })
			return app.PID == os.Getpid() && app.ID != "" && app.Name != "" && app.FullScreen == full
		}
		mu.Lock()
		defer mu.Unlock()
		return latest.PID == os.Getpid() && latest.ID != "" && latest.Name != "" && latest.FullScreen == full
	}
	wait("visible foreground", func() bool {
		front, _, _ := getForegroundWindow.Call()
		visible, _, _ := isWindowVisible.Call(hwnd)
		return visible != 0 && (!tracked || front == hwnd) && seen(false)
	})
	wait("real Media Foundation playback", played.Load)
	onMain(t, hwnd, func(w *window) { w.toggleMaximize() })
	wait("maximized without fullscreen", func() bool { zoomed, _, _ := isZoomed.Call(hwnd); return zoomed != 0 && seen(false) })
	onMain(t, hwnd, func(w *window) { w.toggleFullScreen() })
	wait("fullscreen from maximized", func() bool { return seen(true) })
	onMain(t, hwnd, func(w *window) { w.toggleFullScreen() })
	wait("restored maximized", func() bool { zoomed, _, _ := isZoomed.Call(hwnd); return zoomed != 0 && seen(false) })
	onMain(t, hwnd, func(w *window) { w.toggleMaximize() })
	wait("restored ordinary window", func() bool { zoomed, _, _ := isZoomed.Call(hwnd); return zoomed == 0 && seen(false) })
	t.Logf("actual identity, playing video, maximized/fullscreen/restore transitions passed; public foreground tracking required=%v", tracked)
}

// A launcher may opt in after creating one fresh packaged application and retaining its PID for cleanup. The test never launches or closes other apps.
func TestFrontmostReadsOwnedPackagedApplication(t *testing.T) {
	pidText := os.Getenv("NSGO_FRONTMOST_PACKAGED_PID")
	if pidText == "" {
		t.Skip("needs a freshly owned packaged app and NSGO_FRONTMOST_PACKAGED_PID")
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		t.Fatal("invalid owned packaged PID")
	}
	id := os.Getenv("NSGO_FRONTMOST_PACKAGED_ID")
	if id == "" {
		t.Fatal("the launcher must supply the expected AUMID")
	}
	defer apartmentThread()()
	if got := processApplicationID(uint32(pid)); got != id {
		t.Fatalf("actual process AUMID=%q, want %q", got, id)
	}
	var found driver.FrontmostAppInfo
	var handle uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		visible, _, _ := isWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		app := readWindowFrontmost(hwnd, driver.FrontmostAppInfo{})
		if app.PID == pid && app.ID == id {
			found, handle = app, hwnd
			return 0
		}
		return 1
	})
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		user32.NewProc("EnumWindows").Call(callback, 0)
		if handle != 0 {
			break
		}
	}
	if handle == 0 || found.Name == "" || strings.Contains(strings.ToLower(found.Name), "applicationframehost") {
		t.Fatalf("owned packaged application's actual visible frame unresolved: %+v", found)
	}
	front, _ := readFrontmost(driver.FrontmostAppInfo{})
	t.Logf("owned packaged window class=%q owner PID=%d app=%+v foreground matches=%v", windowClassName(handle), windowProcessID(handle), found, front == found)
}
