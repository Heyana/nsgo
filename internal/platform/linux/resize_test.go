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

// TestWindowDestroyDuringAllocationWaitsForGTK applies an opened window's WindowDestroy from the batches its resize drains inside GTK's size_allocate: the window outlives the allocation, which still holds its surface, and goes once the main loop runs again.
func TestWindowDestroyDuringAllocationWaitsForGTK(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OpenWindow(core.WindowOptions{Title: "nsgo-destroyed", Width: 160, Height: 120}, resizeTestApp)
	})
	w := startTestWindow(t, app, Options{Title: "nsgo-app", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	var opened *window
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		w.onLoop(t, func() {
			windowHandles.Range(func(_, v any) bool {
				if o := v.(*window); !o.primary && !o.embedded && o.win != 0 && o.kind == driver.SurfaceWindow && o.view(o.root) != nil {
					opened = o
				}
				return opened == nil
			})
		})
		if opened != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the window did not open")
		}
	}
	w.onLoop(t, func() {
		defer lengthenResizeWait()()
		opened.batches.Push(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemWindow, Window: &driver.WindowCommand{Action: driver.WindowDestroy}}}})
		gtkWidgetAllocate(opened.surface, 220, 150, -1, 0)
		if !opened.destroyPending {
			t.Fatal("the allocation did not apply the queued WindowDestroy")
		}
		if opened.win == 0 || opened.surface == 0 {
			t.Error("WindowDestroy released the surface GTK was allocating")
		}
	})
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var gone bool
		w.onLoop(t, func() {
			_, registered := windowHandles.Load(opened.handle)
			gone = opened.win == 0 && opened.surface == 0 && !registered
		})
		if gone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the window was not destroyed after the allocation returned")
		}
	}
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

// TestQueueAllocateDefersGTKDuringSurfaceResize verifies that queueAllocate
// marks the Go view tree and queues a draw without calling
// gtk_widget_queue_allocate during allocateSurface's resize drain, while
// forwarding normally to GTK outside it.
func TestQueueAllocateDefersGTKDuringSurfaceResize(t *testing.T) {
	oldAlloc, oldDraw := gtkWidgetQueueAllocate, gtkWidgetQueueDraw
	defer func() {
		gtkWidgetQueueAllocate = oldAlloc
		gtkWidgetQueueDraw = oldDraw
	}()
	var allocCalls, drawCalls []uintptr
	gtkWidgetQueueAllocate = func(widget uintptr) { allocCalls = append(allocCalls, widget) }
	gtkWidgetQueueDraw = func(widget uintptr) { drawCalls = append(drawCalls, widget) }

	w := &window{surface: 0x100}
	root := &view{win: w, widget: 0x101}
	mid := &view{win: w, widget: 0x102, parent: root}
	leaf := &view{win: w, widget: 0x103, parent: mid}
	setWidget(w.surface, widgetRef{w: w})
	setWidget(root.widget, widgetRef{w: w, v: root})
	setWidget(mid.widget, widgetRef{w: w, v: mid})
	setWidget(leaf.widget, widgetRef{w: w, v: leaf})
	defer func() {
		forgetWidget(w.surface)
		forgetWidget(root.widget)
		forgetWidget(mid.widget)
		forgetWidget(leaf.widget)
	}()

	w.deferAllocate = 1
	w.queueAllocate(mid.widget)
	w.queueAllocate(w.surface)
	if len(allocCalls) != 0 {
		t.Fatalf("gtk_widget_queue_allocate called %d times during resize drain, want 0", len(allocCalls))
	}
	if len(drawCalls) != 2 || drawCalls[0] != mid.widget || drawCalls[1] != w.surface {
		t.Fatalf("gtk_widget_queue_draw calls = %v, want [%x %x]", drawCalls, mid.widget, w.surface)
	}
	if !mid.allocNeeded || mid.childAllocNeeded {
		t.Errorf("mid flags = (alloc=%v, child=%v), want (true, false)", mid.allocNeeded, mid.childAllocNeeded)
	}
	if root.allocNeeded || !root.childAllocNeeded {
		t.Errorf("root flags = (alloc=%v, child=%v), want (false, true)", root.allocNeeded, root.childAllocNeeded)
	}
	if leaf.allocNeeded || leaf.childAllocNeeded {
		t.Errorf("leaf flags = (alloc=%v, child=%v), want (false, false)", leaf.allocNeeded, leaf.childAllocNeeded)
	}

	w.deferAllocate = 0
	w.queueAllocate(mid.widget)
	if len(allocCalls) != 1 || allocCalls[0] != mid.widget {
		t.Fatalf("gtk_widget_queue_allocate calls outside resize drain = %v, want [%x]", allocCalls, mid.widget)
	}
}

// TestSurfaceAllocationPlacesNestedChildrenWithoutRequeueing verifies that
// resizing a surface synchronously allocates both resized row children and
// descendants inside an unchanged-size container within the same size_allocate
// pass, without calling gtk_widget_queue_allocate back into GTK.
func TestSurfaceAllocationPlacesNestedChildrenWithoutRequeueing(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const (
		red, blue, green, yellow = 0xff0000, 0x0000ff, 0x00ff00, 0xffff00
		left, strip              = 160, 24
	)
	app := func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: []core.Node{
			// Fixed-size container whose own allocation never changes on resize;
			// its child moves with the window width.
			core.View{TestID: "fixed", Style: core.Style{Width: driver.Pt(left), Height: driver.Pt(120), Background: driver.RGB(red)}, Children: []core.Node{
				core.View{TestID: "nested", StyleFunc: func() core.Style {
					size := cx.Env().Window()
					x := min(max(size.Width-220, 0), left-20)
					return abs(x, 20, 20, 20, core.RGB(yellow))
				}},
			}},
			core.View{TestID: "fill", Style: core.Style{FlexGrow: 1, Background: driver.RGB(blue)}},
			core.View{TestID: "strip", StyleFunc: func() core.Style {
				_ = cx.Env().Window()
				return core.Style{Width: driver.Pt(strip), Background: driver.RGB(green)}
			}},
		}}
	}
	w := startTestWindow(t, app, Options{Width: 300, Height: 160})
	waitPixels(t, w, "the initial nested resize layout", func(p pixels) bool {
		return isColor(p.at(p.width-12, p.height/2), green) && isColor(p.at(90, 30), yellow)
	})
	viewByTestID := func(id string) *view {
		for _, v := range w.views {
			if v != nil && v.props.TestID == id {
				return v
			}
		}
		return nil
	}
	w.onLoop(t, func() {
		defer lengthenResizeWait()()
		origQueueAlloc := gtkWidgetQueueAllocate
		var queued int
		gtkWidgetQueueAllocate = func(widget uintptr) {
			queued++
			origQueueAlloc(widget)
		}
		defer func() { gtkWidgetQueueAllocate = origQueueAlloc }()

		for _, size := range []driver.Size{{Width: 360, Height: 180}, {Width: 240, Height: 150}, {Width: 320, Height: 170}} {
			queued = 0
			gtkWidgetAllocate(w.surface, int32(size.Width), int32(size.Height), -1, 0)
			if queued != 0 {
				t.Errorf("size %v: gtk_widget_queue_allocate called %d times during size_allocate, want 0", size, queued)
			}
			if w.deferAllocate != 0 || w.allocating != 0 {
				t.Errorf("size %v: lingering allocation state deferAllocate=%d allocating=%d", size, w.deferAllocate, w.allocating)
			}
			for _, v := range w.views {
				if v != nil && (v.allocNeeded || v.childAllocNeeded) {
					t.Errorf("size %v: view %d (%q) left dirty (alloc=%v, child=%v)", size, v.id, v.props.TestID, v.allocNeeded, v.childAllocNeeded)
				}
			}

			stripView := viewByTestID("strip")
			nestedView := viewByTestID("nested")
			if stripView == nil || nestedView == nil {
				t.Fatalf("missing test views: strip=%v nested=%v", stripView, nestedView)
			}
			var sb, nb grapheneRect
			if gtkWidgetComputeBounds(stripView.widget, w.surface, &sb) == 0 {
				t.Fatalf("size %v: could not compute native bounds for strip", size)
			}
			wantStripX := float32(size.Width - strip)
			if sb.x != wantStripX || sb.w != strip || sb.h != float32(size.Height) {
				t.Errorf("size %v: strip native bounds = (%v,%v %vx%v), want (%v,0 %vx%v)", size, sb.x, sb.y, sb.w, sb.h, wantStripX, strip, size.Height)
			}
			if gtkWidgetComputeBounds(nestedView.widget, w.surface, &nb) == 0 {
				t.Fatalf("size %v: could not compute native bounds for nested", size)
			}
			wantNestedX := float32(min(max(size.Width-220, 0), left-20))
			if nb.x != wantNestedX || nb.y != 20 || nb.w != 20 || nb.h != 20 {
				t.Errorf("size %v: nested native bounds = (%v,%v %vx%v), want (%v,20 20x20)", size, nb.x, nb.y, nb.w, nb.h, wantNestedX)
			}

			width, height := int32(size.Width), int32(size.Height)
			surface := cairoImageSurfaceCreate(0, width, height)
			cr := cairoCreate(surface)
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
			if !isColor(p.at(p.width-12, p.height/2), green) || !isColor(p.at(p.width-strip-10, p.height/2), blue) {
				t.Errorf("size %v: snapshot before allocation returned did not place the right-edge green strip", size)
			}
			if !isColor(p.at(int(wantNestedX)+10, 30), yellow) {
				t.Errorf("size %v: snapshot before allocation returned did not place the nested child at x=%v", size, wantNestedX)
			}
			cairoDestroy(cr)
			cairoSurfaceDestroy(surface)
		}
	})
}
