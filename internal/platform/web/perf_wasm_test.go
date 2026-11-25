//go:build js && wasm

package web

import (
	"fmt"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestPerfMonitorDisabledLeavesNoClockOrOverlay(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-perf-off")
	setStyle(mount, map[string]string{"width": "400px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	s := open(func(*core.Scope) core.Node {
		return core.Text{Content: "Plain"}
	}, Options{Mount: "#nsgo-perf-off"}, nil)
	defer s.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
	defer s.engine.Close()

	if s.perf != nil || s.hud != nil {
		t.Fatalf("perf unexpectedly installed without Perf option: perf=%v hud=%v", s.perf, s.hud)
	}
	if s.ticking {
		t.Fatal("animation frame clock running when Perf is disabled and app is idle")
	}
	if !mount.Call("querySelector", "[data-nsgo-perf]").IsNull() {
		t.Fatal("perf overlay rendered when Perf is disabled")
	}
}

func TestPerfPanelOverlayClockAndTeardown(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-perf-on")
	setStyle(mount, map[string]string{"width": "420px", "height": "300px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	scopes := make(chan *core.Scope, 1)
	var ticks *core.Signal[int]
	s := open(func(cx *core.Scope) core.Node {
		ticks = core.NewSignal(cx, 0)
		select {
		case scopes <- cx:
		default:
		}
		return core.Text{ContentFunc: func() string {
			return fmt.Sprintf("tick %d", ticks.Get())
		}}
	}, Options{Mount: "#nsgo-perf-on", Perf: true}, nil)
	defer s.engine.Close()

	var scope *core.Scope
	select {
	case scope = <-scopes:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for app scope to mount")
	}

	if s.perf == nil || s.hud == nil {
		t.Fatal("perf monitor and HUD were not installed")
	}
	if !s.ticking {
		t.Fatal("continuous requestAnimationFrame clock did not start with Perf")
	}

	panel := mount.Call("querySelector", `[data-nsgo-perf="panel"]`)
	if panel.IsNull() {
		t.Fatal("perf panel overlay not mounted in host element")
	}
	cs := js.Global().Call("getComputedStyle", panel)
	if cs.Get("pointerEvents").String() != "none" {
		t.Fatalf("panel pointer-events = %q, want none", cs.Get("pointerEvents").String())
	}
	if panel.Call("getAttribute", "aria-hidden").String() != "true" {
		t.Fatalf("panel aria-hidden = %q, want true", panel.Call("getAttribute", "aria-hidden").String())
	}

	// Drive enough commits and refreshes past the warmup count to observe live frame readings.
	deadline := time.Now().Add(5 * time.Second)
	for i := 1; i <= 10; i++ {
		scope.Post(func() { ticks.Set(i) })
		time.Sleep(35 * time.Millisecond)
	}
	for time.Now().Before(deadline) {
		snap := s.perf.Snapshot()
		if snap.RefreshRate != nil && len(snap.Frames) > 0 && snap.Commits > 0 && strings.Contains(s.hud.label.Get("textContent").String(), "Hz") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	snap := s.perf.Snapshot()
	if snap.RefreshRate == nil || *snap.RefreshRate <= 0 {
		t.Fatalf("expected measured refresh rate, got %v", snap.RefreshRate)
	}
	if len(snap.Frames) == 0 || snap.TickInterval.Mean <= 0 || snap.Commits <= 0 || snap.Items <= 0 {
		t.Fatalf("expected recorded frames, commits and tick intervals: frames=%d commits=%v items=%v tick=%+v",
			len(snap.Frames), snap.Commits, snap.Items, snap.TickInterval)
	}
	// Synchronous wasm Mount records hop latency when Sent precedes Mount.
	s.Mount(&driver.MountBatch{Sent: time.Now().Add(-3 * time.Millisecond)})
	if lat := s.perf.Snapshot().Latency; lat.Max < 2*time.Millisecond {
		t.Fatalf("expected Mount to record batch hop latency, got %+v", lat)
	}
	if text := s.hud.label.Get("textContent").String(); !strings.Contains(text, "FPS") || !strings.Contains(text, "Hz") {
		t.Fatalf("unexpected panel readings text:\n%s", text)
	}

	// Teardown stops the clock, removes the panel, and clears the monitor.
	s.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
	if s.perf != nil || s.hud != nil || s.ticking {
		t.Fatalf("teardown left perf state active: perf=%v hud=%v ticking=%v", s.perf, s.hud, s.ticking)
	}
	if !mount.Call("querySelector", `[data-nsgo-perf="panel"]`).IsNull() {
		t.Fatal("perf panel remained in DOM after teardown")
	}
}

func TestPerfPanelEmbeddedMountsAreScopedAndClamped(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	narrow := doc.Call("createElement", "div")
	narrow.Set("id", "nsgo-perf-narrow")
	setStyle(narrow, map[string]string{"width": "200px", "height": "220px"})
	doc.Get("body").Call("appendChild", narrow)
	defer narrow.Call("remove")

	wide := doc.Call("createElement", "div")
	wide.Set("id", "nsgo-perf-wide")
	setStyle(wide, map[string]string{"width": "400px", "height": "220px"})
	doc.Get("body").Call("appendChild", wide)
	defer wide.Call("remove")

	s1 := open(func(*core.Scope) core.Node { return core.Text{Content: "A"} }, Options{Mount: "#nsgo-perf-narrow", Perf: true}, nil)
	defer s1.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
	defer s1.engine.Close()

	s2 := open(func(*core.Scope) core.Node { return core.Text{Content: "B"} }, Options{Mount: "#nsgo-perf-wide", Perf: true}, nil)
	defer s2.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
	defer s2.engine.Close()

	p1 := narrow.Call("querySelector", `[data-nsgo-perf="panel"]`)
	p2 := wide.Call("querySelector", `[data-nsgo-perf="panel"]`)
	if p1.IsNull() || p2.IsNull() || p1.Equal(p2) {
		t.Fatal("each embedded mount must own its own perf panel")
	}
	if got := p1.Call("getBoundingClientRect").Get("width").Float(); got > 180.5 {
		t.Fatalf("narrow embedded panel width = %v, want <= 180", got)
	}
}

func TestPerfMonitorInTestModeRecordsFrames(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-perf-testmode")
	setStyle(mount, map[string]string{"width": "360px", "height": "240px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	test := &testOpen{options: testhost.Options{FPS: true}}
	s := open(func(*core.Scope) core.Node { return core.Text{Content: "Test"} }, Options{Mount: "#nsgo-perf-testmode"}, test)
	defer s.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
	defer s.engine.Close()

	if s.perf == nil || s.hud == nil || !s.ticking {
		t.Fatalf("expected test FPS mode to install monitor, panel and clock: perf=%v hud=%v ticking=%v", s.perf, s.hud, s.ticking)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap := s.perf.Snapshot(); snap.RefreshRate != nil && snap.TickInterval.Mean > 0 {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("timed out waiting for test-mode frame clock ticks")
}
