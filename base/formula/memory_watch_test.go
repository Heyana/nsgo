//go:build ios && nsgowatch

package formula

import (
	"encoding/json"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestWatchFormulaMemory is an opt-in runtime probe, not a device memory budget.
// Run this test alone in a fresh process: earlier formula tests load the default font.
func TestWatchFormulaMemory(t *testing.T) {
	if os.Getenv("NSGO_FORMULA_MEMORY") != "1" {
		t.Skip("set NSGO_FORMULA_MEMORY=1 for the watch runtime memory probe")
	}
	var show *core.Signal[bool]
	var renderer *Renderer
	// These are the three formulas in the gallery's Markdown document.
	const markdown = "A list stays smooth at $60$ frames a second, $16.7\\,\\text{ms}$ each.\n\n$$\nt_{\\text{frame}} = \\tfrac{1000}{60}\\,\\text{ms}\n$$"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if cx.Env().Idiom() != core.IdiomWatch {
			t.Fatal("the probe requires a real watchOS host")
		}
		show = core.NewSignal(cx, false)
		return core.Scroll{Style: core.Style{Background: core.RGB(0x000000), Color: core.RGB(0xffffff)}, Children: []core.Node{
			core.Show{When: show.Get, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Padding: core.All(6)}, Children: []core.Node{textview.TextView{Markdown: markdown, RenderMath: renderer.RenderMath}}}
			}, Else: func(cx *core.Scope) core.Node { return core.Text{Content: "Formula memory probe"} }},
		}}
	})
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(lib)
	var available func() uint64
	purego.RegisterLibFunc(&available, lib, "os_proc_available_memory")
	sample := func(stage string) {
		t.Helper()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		footprint, ok := apple.ProcessMemory()
		if !ok {
			t.Fatal("proc_pid_rusage failed")
		}
		data, err := json.Marshal(struct {
			Stage         string `json:"stage"`
			PID           int    `json:"pid"`
			FontBytes     int    `json:"font_bytes"`
			HeapAlloc     uint64 `json:"heap_alloc"`
			HeapInuse     uint64 `json:"heap_inuse"`
			HeapSys       uint64 `json:"heap_sys"`
			GoSys         uint64 `json:"go_sys"`
			TotalAlloc    uint64 `json:"total_alloc"`
			PhysFootprint uint64 `json:"phys_footprint"`
			Available     uint64 `json:"available_before_limit"`
		}{stage, os.Getpid(), len(newCMMath), m.HeapAlloc, m.HeapInuse, m.HeapSys, m.Sys, m.TotalAlloc, footprint, available()})
		if err != nil {
			t.Fatal(err)
		}
		t.Log(string(data))
	}
	sample("host")
	var fontPages byte
	for i := 0; i < len(newCMMath); i += 4096 {
		fontPages ^= newCMMath[i]
	}
	t.Logf("embedded font page witness: %d", fontPages)
	sample("embedded_font_touched")
	// Sample transient loading memory too: retained bytes after GC omit parsing's peak.
	var peakHeap, peakFootprint atomic.Uint64
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			peakHeap.Store(max(peakHeap.Load(), stats.HeapAlloc))
			if footprint, ok := apple.ProcessMemory(); ok {
				peakFootprint.Store(max(peakFootprint.Load(), footprint))
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	renderer, err = NewRenderer(Fonts{Math: newCMMath})
	close(stop)
	<-stopped
	t.Logf(`{"stage":"font_parse_peak","sample_interval_ms":2,"heap_alloc":%d,"phys_footprint":%d}`, peakHeap.Load(), peakFootprint.Load())
	if err != nil {
		t.Fatal(err)
	}
	sample("parsed_font")
	app.Post(func() { show.Set(true) })
	app.Settle()
	waitForFont(app, renderer, "label:60", "label:16.7 ms", "label:t sub frame equals 1000 over 60 ms")
	for _, sel := range []string{"label:60", "label:16.7 ms", "label:t sub frame equals 1000 over 60 ms"} {
		if app.Find(sel).Kind != nsgotest.KindCanvas {
			t.Fatalf("%s was not typeset", sel)
		}
	}
	// Let the host draw the mounted paths before sampling and capturing it.
	app.Advance(100 * time.Millisecond)
	app.Screenshot("watch-gallery-formulas")
	sample("gallery_formulas")
	runtime.KeepAlive(renderer)
}
