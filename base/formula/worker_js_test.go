//go:build js && wasm && !nsgoharmony && !nsgominiprogram

package formula

import (
	"context"
	"math"
	"reflect"
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/caelyreth/untie"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestWebFontLoadsInWorkerWithoutBlockingFrames verifies that loading New
// Computer Modern Math in a browser runs in the dedicated Web Worker so
// requestAnimationFrame continues ticking throughout font initialization.
func TestWebFontLoadsInWorkerWithoutBlockingFrames(t *testing.T) {
	if nsgotest.Target() != "web" {
		t.Skip("measures browser requestAnimationFrame responsiveness")
	}

	g := js.Global()
	perf := g.Get("performance")
	now := func() float64 { return perf.Call("now").Float() }

	var mu sync.Mutex
	var frames []float64
	stop := false
	var tick js.Func
	tick = js.FuncOf(func(_ js.Value, args []js.Value) any {
		mu.Lock()
		stopped := stop
		if !stopped {
			ts := now()
			if len(args) > 0 && args[0].Type() == js.TypeNumber {
				ts = args[0].Float()
			}
			frames = append(frames, ts)
		}
		mu.Unlock()
		if !stopped {
			g.Call("requestAnimationFrame", tick)
		}
		return nil
	})
	g.Call("requestAnimationFrame", tick)
	defer func() {
		mu.Lock()
		stop = true
		mu.Unlock()
		tick.Release()
	}()

	// Wait for a few animation frames before starting the font load so the
	// browser's frame clock is warmed up and ticking.
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(frames)
		mu.Unlock()
		if n >= 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	r := newRenderer(Fonts{Math: newCMMath})
	var loadStart, loadEnd float64
	origEngine := r.engine
	r.engine = func() (mathEngine, error) {
		mu.Lock()
		if loadStart == 0 {
			loadStart = now()
		}
		mu.Unlock()
		eng, err := origEngine()
		mu.Lock()
		if loadEnd == 0 {
			loadEnd = now()
		}
		mu.Unlock()
		return eng, err
	}

	const src = `\int_0^\infty \frac{x^2}{\sqrt{1+x}}\,dx`
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FontSize: 24, Padding: core.All(16)}, Children: []core.Node{
			View{TestID: "integral", Source: src, Display: true, Renderer: r},
		}}
	}, nsgotest.Size(480, 240))

	waitForFont(app, r, "id:integral")

	// Let two more frames arrive after completion to bound any trailing gap.
	time.Sleep(50 * time.Millisecond)

	eng, err := r.engine()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := eng.(*workerEngine); !ok {
		t.Fatalf("renderer engine is %T, want *workerEngine so font parsing stays off the page thread", eng)
	}

	mu.Lock()
	snap := append([]float64(nil), frames...)
	start, end := loadStart, loadEnd
	mu.Unlock()

	loadDuration := end - start
	var during int
	var maxGap float64
	for i := 0; i < len(snap); i++ {
		if snap[i] > start && snap[i] < end {
			during++
		}
		if i > 0 && snap[i] >= start && snap[i-1] <= end {
			if gap := snap[i] - snap[i-1]; gap > maxGap {
				maxGap = gap
			}
		}
	}
	t.Logf("web formula font load: duration=%.1fms frames_during_load=%d max_overlapping_frame_gap=%.1fms total_frames=%d",
		loadDuration, during, maxGap, len(snap))

	if loadDuration > 40 && during == 0 {
		t.Errorf("0 requestAnimationFrame callbacks ran during %.1fms font load (max overlapping gap %.1fms): main thread was blocked", loadDuration, maxGap)
	}
	// On a blocked main thread, the overlapping frame gap exceeds the full font
	// initialization duration (233-250ms in Chrome 157); with the worker, frames
	// keep ticking well below the load duration.
	if loadDuration > 80 && maxGap >= loadDuration {
		t.Errorf("max overlapping frame gap %.1fms >= font load duration %.1fms: main thread stalled during load", maxGap, loadDuration)
	}

	v := app.Find("id:integral")
	if v.Frame.Size.Width <= 24 || v.Frame.Size.Height <= 24 || len(v.Ops) == 0 {
		t.Fatalf("formula did not render after worker load: size=%+v ops=%d", v.Frame.Size, len(v.Ops))
	}
}

// TestWebWorkerMatchesLocalLayout verifies that layouts, hits, carets, and
// system-text rewrites produced by the Web Worker match in-process Untie.
func TestWebWorkerMatchesLocalLayout(t *testing.T) {
	if nsgotest.Target() != "web" {
		t.Skip("requires browser Web Worker")
	}
	workerEng, err := loadMathEngine(Fonts{Math: newCMMath})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := workerEng.(*workerEngine); !ok {
		t.Fatalf("loadMathEngine returned %T, want *workerEngine", workerEng)
	}
	localEng, err := loadLocalEngine(Fonts{Math: newCMMath})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		src      string
		dialect  untie.Dialect
		mode     untie.Mode
		maxWidth float32
	}{
		{src: `\int_0^\infty \frac{x^2}{\sqrt{1+x}}\,dx`, dialect: untie.TeX, mode: untie.Display},
		{src: wrappingFormula, dialect: untie.TeX, mode: untie.Display, maxWidth: 600},
		{src: `x+\text{中文}+y`, dialect: untie.TeX, mode: untie.Inline},
		{src: `sum_(i=1)^n x_i^2`, dialect: untie.Typst, mode: untie.Display},
	} {
		expr := untie.Parse(tc.src, tc.dialect, tc.mode, untie.ParseOptions{})
		style := untie.Style{Size: referenceSize, MaxWidth: tc.maxWidth, Align: untie.AlignCenter}
		opts := untie.LayoutOptions{Recovery: untie.RecoveryVisible}
		got, err := workerEng.LayoutContextWithOptions(context.Background(), expr, style, opts)
		if err != nil {
			t.Fatalf("%s: worker layout failed: %v", tc.src, err)
		}
		want, err := localEng.LayoutContextWithOptions(context.Background(), expr, style, opts)
		if err != nil {
			t.Fatalf("%s: local layout failed: %v", tc.src, err)
		}
		if math.Abs(float64(got.Bounds().Width-want.Bounds().Width)) > 0.001 ||
			math.Abs(float64(got.Bounds().Height-want.Bounds().Height)) > 0.001 {
			t.Errorf("%s: worker bounds %+v != local %+v", tc.src, got.Bounds(), want.Bounds())
		}
		if !reflect.DeepEqual(got.Items, want.Items) || !reflect.DeepEqual(got.Paths, want.Paths) {
			t.Errorf("%s: worker items/paths differ from local", tc.src)
		}
		if !reflect.DeepEqual(got.Hits, want.Hits) || !reflect.DeepEqual(got.CaretStops, want.CaretStops) {
			t.Errorf("%s: worker hits/carets differ from local", tc.src)
		}
		if !reflect.DeepEqual(got.FontUsage, want.FontUsage) {
			t.Errorf("%s: worker font usage %+v != local %+v", tc.src, got.FontUsage, want.FontUsage)
		}
	}
}
