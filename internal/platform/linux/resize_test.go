//go:build linux && !android

package linux

import (
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestSurfaceAllocationAppliesResizeBeforeReturning exercises GTK's actual size_allocate vfunc, reading both the native widget allocations and GSK's snapshot before the main loop can dispatch an idle callback.
func TestSurfaceAllocationAppliesResizeBeforeReturning(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	w := startTestWindow(t, resizeTestApp, Options{Width: 240, Height: 160})
	waitResizeApp(t, w)
	w.onLoop(t, func() {
		defer lengthenResizeWait()()
		for _, size := range []driver.Size{{Width: 320, Height: 200}, {Width: 180, Height: 140}, {Width: 280, Height: 180}} {
			gtkWidgetAllocate(w.surface, int32(size.Width), int32(size.Height), -1, 0)
			checkResizeAllocation(t, w, size)
		}
	})
}

// lengthenResizeWait gives a responsive engine time to commit on a loaded runner, so a test of the synchronous path does not race the production frame deadline. Call it on the main thread, which reads resizeWait, and call what it returns there to restore the deadline.
func lengthenResizeWait() (restore func()) {
	old := resizeWait
	resizeWait = 2 * time.Second
	return func() { resizeWait = old }
}

func resizeTestApp(cx *core.Scope) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xff0000)}, Children: []core.Node{
		core.View{TestID: "edge", StyleFunc: func() core.Style {
			size := cx.Env().Window()
			return abs(size.Width-20, size.Height-20, 20, 20, core.RGB(0x0000ff))
		}},
	}}
}

func waitResizeApp(t *testing.T, w *window) {
	t.Helper()
	waitPixels(t, w, "the app's initial layout", func(p pixels) bool {
		return isColor(p.at(p.width-10, p.height-10), 0x0000ff)
	})
}

func checkResizeAllocation(t *testing.T, w *window, size driver.Size) {
	t.Helper()
	root := w.view(w.root)
	if root == nil || root.frame.Size != size {
		t.Errorf("root layout does not match %v before allocation returned", size)
		return
	}
	if width, height := gtkWidgetGetWidth(root.widget), gtkWidgetGetHeight(root.widget); width != int32(size.Width) || height != int32(size.Height) {
		t.Errorf("native root allocation = %dx%d, want %v", width, height, size)
	}
	// GtkWidgetPaintable snapshots the last completed frame until GTK's paint phase. Invoke the surface's real snapshot callback here to test the next frame using the allocations we have just given GTK.
	width, height := int32(size.Width), int32(size.Height)
	surface := cairoImageSurfaceCreate(0, width, height)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	snap := gtkSnapshotNew()
	viewSnapshotCallback(w.surface, snap)
	if node := gtkSnapshotFreeToNode(snap); node != 0 {
		gskRenderNodeDraw(node, cr)
		gskRenderNodeUnref(node)
	}
	cairoSurfaceFlush(surface)
	stride := int(cairoImageSurfaceGetStride(surface))
	data := (*[1 << 30]byte)(cPointer(cairoImageSurfaceGetData(surface)))[:stride*int(height)]
	p := pixels{data: data, stride: stride, width: int(width), height: int(height)}
	if !isColor(p.at(p.width-10, p.height-10), 0x0000ff) || !isColor(p.at(p.width-30, p.height-10), 0xff0000) {
		t.Errorf("snapshot before allocation returned did not lay out the bottom right marker at %v", size)
	}
}

// TestSurfaceResizeTimeoutSkipsFurtherWaitsAndAppliesLateCommit holds the engine while GTK allocates several sizes: the first wait is bounded, later sizes are still reported without multiplying that wait, and the idle path eventually applies the latest size once the app becomes responsive again.
func TestSurfaceResizeTimeoutSkipsFurtherWaitsAndAppliesLateCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	ready := make(chan *core.Scope, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return resizeTestApp(cx)
	}, Options{Width: 240, Height: 160})
	waitResizeApp(t, w)
	cx := <-ready
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	cx.Post(func() { close(held); <-release })
	select {
	case <-held:
	case <-time.After(5 * time.Second):
		t.Fatal("the engine did not enter the held handler")
	}
	var timedOut <-chan struct{}
	w.onLoop(t, func() {
		// A native window command can allocate its surface inside a mount batch. Reporting those sizes must not wait on the busy engine.
		w.applying = true
		start := time.Now()
		for i := range 12 {
			w.resize(driver.Size{Width: driver.Points(250 + i), Height: 160})
		}
		w.applying = false
		if elapsed := time.Since(start); elapsed >= 6*resizeWait || w.resized != nil {
			t.Errorf("resize during a mount waited on the engine: %v, pending %v", elapsed, w.resized != nil)
		}
		start = time.Now()
		gtkWidgetAllocate(w.surface, 260, 170, -1, 0)
		elapsed := time.Since(start)
		if elapsed < resizeWait || elapsed > time.Second {
			t.Errorf("first allocation waited %v, want a bounded %v wait", elapsed, resizeWait)
		}
		timedOut = w.resized
		if timedOut == nil {
			t.Error("the allocation did not record the unfinished resize")
		}
		start = time.Now()
		for i := range 12 {
			gtkWidgetAllocate(w.surface, int32(280+i), 180, -1, 0)
		}
		if elapsed := time.Since(start); elapsed >= 6*resizeWait {
			t.Errorf("later allocations waited %v while the engine was still busy", elapsed)
		}
		if w.resized != timedOut {
			t.Error("later allocations replaced the unfinished resize wait")
		}
	})
	unblock()
	w.engine.Sync()
	waitPixels(t, w, "the last reported size after the engine resumes", func(p pixels) bool {
		return isColor(p.at(p.width-10, p.height-10), 0x0000ff)
	})
	w.onLoop(t, func() {
		// The window manager may have allocated the surface again; the idle commit must agree with its most recent allocation in either case.
		size := driver.Size{Width: w.lastWidth, Height: w.lastHeight}
		if root := w.view(w.root); root == nil || root.frame.Size != size {
			t.Errorf("late layout does not match the most recent allocation %v", size)
		}
		select {
		case <-timedOut:
		default:
			t.Error("the engine did not finish the timed-out resize")
		}
		defer lengthenResizeWait()()
		gtkWidgetAllocate(w.surface, 300, 190, -1, 0)
		if w.resized != nil {
			t.Error("responsive allocation did not resume waiting after the late commit")
		}
		checkResizeAllocation(t, w, driver.Size{Width: 300, Height: 190})
	})
}

// TestApplyBatchesDoesNotDrainRecursively keeps batches queued when GTK's native calls allocate another surface while a batch is being applied.
func TestApplyBatchesDoesNotDrainRecursively(t *testing.T) {
	w := &window{applying: true}
	batch := &driver.MountBatch{}
	w.batches.Push(batch)
	w.applyBatches()
	if batches := w.batches.Drain(); len(batches) != 1 || batches[0] != batch {
		t.Error("a recursive mount drain lost the pending batch")
	}
}
