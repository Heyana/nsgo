//go:build linux && !android

package linux

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// xTranslateCoordinates is Xlib's XTranslateCoordinates, which reads where the X server put a window, bound once; nil where libX11 does not load.
var xTranslateCoordinates = sync.OnceValue(func() func(display, src, dst uintptr, x, y int32, dx, dy *int32, child *uintptr) int32 {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	var f func(display, src, dst uintptr, x, y int32, dx, dy *int32, child *uintptr) int32
	purego.RegisterLibFunc(&f, lib, "XTranslateCoordinates")
	return f
})

// waitPanel polls ok for up to five seconds, failing the test with what it waited for.
func waitPanel(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// panelSnapshotPixels renders the complete GtkWindow snapshot, including the root surface background and GTK's native render nodes, which the software paint helper most tests use bypasses.
func panelSnapshotPixels(t *testing.T, w *window) pixels {
	t.Helper()
	var out pixels
	w.onLoop(t, func() {
		width, height := gtkWidgetGetWidth(w.win), gtkWidgetGetHeight(w.win)
		if width <= 0 || height <= 0 {
			return
		}
		paintable := gtkWidgetPaintableNew(w.win)
		if paintable == 0 {
			return
		}
		defer gObjectUnref(paintable)
		snapshot := gtkSnapshotNew()
		gdkPaintableSnapshot(paintable, snapshot, float64(width), float64(height))
		node := gtkSnapshotFreeToNode(snapshot)
		if node == 0 {
			return
		}
		defer gskRenderNodeUnref(node)
		surface := cairoImageSurfaceCreate(0, width, height)
		cr := cairoCreate(surface)
		gskRenderNodeDraw(node, cr)
		cairoSurfaceFlush(surface)
		stride := int(cairoImageSurfaceGetStride(surface))
		data := (*[1 << 30]byte)(cPointer(cairoImageSurfaceGetData(surface)))[: stride*int(height) : stride*int(height)]
		out = pixels{data: append([]byte(nil), data...), stride: stride, width: int(width), height: int(height)}
		cairoDestroy(cr)
		cairoSurfaceDestroy(surface)
	})
	return out
}

// TestTransparentFittedPanelKeepsItsAnchor shows a transparent fitted panel beside an anchor on X11 and checks its rounded card's pixels and that it stays beside the anchor, where driver.PlaceWindow puts it, as its content grows and shrinks.
func TestTransparentFittedPanelKeepsItsAnchor(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	type control struct {
		panel  *core.Window
		height *core.Signal[driver.Points]
		scope  *core.Scope
	}
	ready := make(chan control, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		h := core.NewSignal(cx, driver.Points(80))
		p := cx.OpenPanel(core.PanelOptions{Width: 280, Height: 240, FitContent: true, Transparent: true}, func(*core.Scope) core.Node {
			return core.View{StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(180), Height: core.Pt(h.Get()), Background: core.RGB(0x2255aa), CornerRadius: 12}
			}}
		})
		ready <- control{p, h, cx}
	})
	host := startTestWindow(t, app, Options{Title: "nsgo-fitted-panel-host", Width: 100, Height: 80, Hidden: true})
	defer host.onLoop(t, host.close)
	ctl := <-ready
	// Asking for the panel's X window would realize it at its first size, before the fitted size goes ahead of its show: tell X11 from the display instead.
	var x11 bool
	host.onLoop(t, func() { x11 = drawsOnX11() })
	if !x11 || xTranslateCoordinates() == nil {
		t.Skip("placing a panel beside its anchor needs X11")
	}
	var p *window
	waitPanel(t, "the panel", func() bool {
		host.onLoop(t, func() {
			windowHandles.Range(func(_, v any) bool {
				w := v.(*window)
				// Only this test's panel: an earlier test's may not have gone yet.
				if w.opener == host && w.kind == driver.SurfacePanel && !w.closing {
					p = w
				}
				return p == nil
			})
		})
		return p != nil && p.win != 0
	})
	anchor := driver.Bounds{Origin: driver.Point{X: 300, Y: 180}, Size: driver.Size{Width: 40, Height: 24}}
	// The core's windows belong to the engine's goroutine: show the panel there, not from the test's.
	shown := make(chan error, 1)
	ctl.scope.Post(func() { shown <- ctl.panel.ShowAt(anchor, core.PlaceRight) })
	if err := <-shown; err != nil {
		t.Fatal(err)
	}
	waitPanel(t, "the panel to map", func() bool {
		var mapped bool
		host.onLoop(t, func() { mapped = gtkWidgetGetMapped(p.win) != 0 })
		return mapped
	})
	var composited bool
	host.onLoop(t, func() { composited = p.environment().WindowTransparent })
	var initial pixels
	waitPanel(t, "the transparent card's pixels at its fitted size", func() bool {
		initial = panelSnapshotPixels(t, p)
		return initial.width == 180 && initial.height == 80
	})
	if q := initial.at(initial.width/2, initial.height/2); q[3] != 255 {
		t.Fatalf("the card center is not opaque: alpha=%d", q[3])
	}
	if composited {
		clear, antialias := 0, 0
		for y := range initial.height {
			for x := range initial.width {
				switch a := initial.at(x, y)[3]; {
				case a == 0:
					clear++
				case a < 255:
					antialias++
				}
			}
		}
		if clear == 0 || antialias == 0 {
			t.Fatalf("the transparent rounded panel has no clear or antialiased pixels: clear=%d antialias=%d", clear, antialias)
		}
	}
	var display, xid, root uintptr
	host.onLoop(t, func() {
		display, xid, _ = p.x11Window()
		root = x11LevelRoot(display)
		if p.environment().WindowTransparent != displayComposites() {
			t.Errorf("transparent=%v compositor=%v", p.environment().WindowTransparent, displayComposites())
		}
	})
	// placed reports where the panel is, in device pixels on the root window, and where driver.PlaceWindow puts a panel of size beside the anchor.
	placed := func(size driver.Size) (got, want driver.Point, ok bool) {
		host.onLoop(t, func() {
			displays := p.environment().Displays
			if len(displays) == 0 {
				return
			}
			area := displays[0].VisibleFrame
			center := driver.Point{X: anchor.Origin.X + anchor.Size.Width/2, Y: anchor.Origin.Y + anchor.Size.Height/2}
			for _, d := range displays {
				if d.Frame.Contains(center) {
					area = d.VisibleFrame
					break
				}
			}
			want, _ = driver.PlaceWindow(anchor, size, area, driver.WindowPlaceRight)
			scale := float64(max(gtkWidgetGetScaleFactor(p.win), 1))
			want = driver.Point{X: driver.Points(math.Round(float64(want.X) * scale)), Y: driver.Points(math.Round(float64(want.Y) * scale))}
			var x, y int32
			var child uintptr
			xTranslateCoordinates()(display, xid, root, 0, 0, &x, &y, &child)
			got, ok = driver.Point{X: driver.Points(x), Y: driver.Points(y)}, true
		})
		return got, want, ok
	}
	// The window manager may move the panel a turn after it resizes, so wait for the place rather than reading it once.
	assertPlaced := func(size driver.Size, label string) {
		t.Helper()
		var got, want driver.Point
		var ok bool
		for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if got, want, ok = placed(size); !ok {
				t.Fatal("the native test has no display")
			}
			if got == want || time.Now().After(end) {
				break
			}
		}
		if got != want {
			t.Errorf("%s panel origin = %v, want %v", label, got, want)
		}
	}
	surfaceSize := func(width, height int32) func() bool {
		return func() bool {
			var w, h int32
			host.onLoop(t, func() { w, h = gtkWidgetGetWidth(p.surface), gtkWidgetGetHeight(p.surface) })
			return w == width && h == height
		}
	}
	assertPlaced(driver.Size{Width: 180, Height: 80}, "initial")
	ctl.scope.Post(func() { ctl.height.Set(180) })
	waitPanel(t, "the panel to grow", surfaceSize(180, 180))
	assertPlaced(driver.Size{Width: 180, Height: 180}, "grown")
	ctl.scope.Post(func() { ctl.height.Set(40) })
	waitPanel(t, "the panel to shrink", surfaceSize(180, 40))
	assertPlaced(driver.Size{Width: 180, Height: 40}, "shrunk")
}
