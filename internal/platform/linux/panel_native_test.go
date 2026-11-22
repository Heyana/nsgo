//go:build linux && !android

package linux

import (
	"math"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"testing"
	"time"
)

func translate1394(d, x, r uintptr) (int32, int32) {
	var f func(uintptr, uintptr, uintptr, int32, int32, *int32, *int32, *uintptr) int32
	lib, _ := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	purego.RegisterLibFunc(&f, lib, "XTranslateCoordinates")
	var a, b int32
	f(d, x, r, 0, 0, &a, &b, new(uintptr))
	return a, b
}
func wait1394(t *testing.T, s string, p func() bool) {
	for d := time.Now().Add(5 * time.Second); time.Now().Before(d); time.Sleep(10 * time.Millisecond) {
		if p() {
			return
		}
	}
	t.Fatal("timed out waiting for ", s)
}

// nativePixels1394 renders the complete GtkWindow snapshot, including the
// root surface background and GTK's native render nodes. The software paint
// helper used by most tests deliberately bypasses that layer.
func nativePixels1394(t *testing.T, w *window) pixels {
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

func TestTransparentFittedPanelKeepsItsAnchor(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	ready := make(chan struct {
		p *core.Window
		h *core.Signal[driver.Points]
		s *core.Scope
	}, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		h := core.NewSignal(cx, driver.Points(80))
		p := cx.OpenPanel(core.PanelOptions{Width: 280, Height: 240, FitContent: true, Transparent: true}, func(*core.Scope) core.Node {
			return core.View{StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(180), Height: core.Pt(h.Get()), Background: core.RGB(0x2255aa), CornerRadius: 12}
			}}
		})
		ready <- struct {
			p *core.Window
			h *core.Signal[driver.Points]
			s *core.Scope
		}{p, h, cx}
	})
	host := startTestWindow(t, app, Options{Title: "nsgo-panel1394-host", Width: 100, Height: 80, Hidden: true})
	defer host.onLoop(t, host.close)
	ctl := <-ready
	var p *window
	wait1394(t, "panel", func() bool {
		host.onLoop(t, func() {
			windowHandles.Range(func(_, v any) bool {
				w := v.(*window)
				if !w.primary && w.kind == driver.SurfacePanel && !w.closing {
					p = w
				}
				return p == nil
			})
		})
		return p != nil && p.win != 0
	})
	if err := ctl.p.ShowAt(driver.Bounds{Origin: driver.Point{X: 300, Y: 180}, Size: driver.Size{Width: 40, Height: 24}}, core.PlaceRight); err != nil {
		t.Fatal(err)
	}
	wait1394(t, "map", func() bool {
		var m bool
		host.onLoop(t, func() {
			m = gtkWidgetGetMapped(p.win) != 0

		})
		return m
	})
	var composited bool
	host.onLoop(t, func() { composited = p.environment().WindowTransparent })
	var initial pixels
	wait1394(t, "the native transparent card pixels", func() bool {
		initial = nativePixels1394(t, p)
		return initial.width == 180 && initial.height == 80
	})
	if q := initial.at(initial.width/2, initial.height/2); q[3] != 255 {
		t.Fatalf("the card center is not opaque: alpha=%d", q[3])
	}
	if composited {
		clear, antialias := 0, 0
		for y := 0; y < initial.height; y++ {
			for x := 0; x < initial.width; x++ {
				a := initial.at(x, y)[3]
				switch {
				case a == 0:
					clear++
				case a < 255:
					antialias++
				}
			}
		}
		if clear == 0 || antialias == 0 {
			t.Fatalf("transparent rounded panel has no clear/antialiased pixels: clear=%d antialias=%d", clear, antialias)
		}
	}
	var d, x, r uintptr
	anchor := driver.Bounds{Origin: driver.Point{X: 300, Y: 180}, Size: driver.Size{Width: 40, Height: 24}}
	panelOrigin := func(size driver.Size) driver.Point {
		var want driver.Point
		host.onLoop(t, func() {
			displays := p.environment().Displays
			if len(displays) == 0 {
				t.Fatal("the native test has no display")
			}
			area := displays[0].VisibleFrame
			center := driver.Point{X: anchor.Origin.X + anchor.Size.Width/2, Y: anchor.Origin.Y + anchor.Size.Height/2}
			for _, display := range displays {
				if display.Frame.Contains(center) {
					area = display.VisibleFrame
					break
				}
			}
			want, _ = driver.PlaceWindow(anchor, size, area, driver.WindowPlaceRight)
		})
		return want
	}
	nativeOrigin := func() driver.Point {
		var got driver.Point
		host.onLoop(t, func() {
			a, b := translate1394(d, x, r)
			got = driver.Point{X: driver.Points(a), Y: driver.Points(b)}
		})
		return got
	}
	assertPlacement := func(size driver.Size, label string) {
		want := panelOrigin(size)
		got := nativeOrigin()
		var scale float64
		host.onLoop(t, func() { scale = float64(max(gtkWidgetGetScaleFactor(p.win), 1)) })
		want = driver.Point{X: driver.Points(math.Round(float64(want.X) * scale)), Y: driver.Points(math.Round(float64(want.Y) * scale))}
		if got != want {
			t.Errorf("%s panel origin = %v, want %v (scale %.1f)", label, got, want, scale)
		}
	}
	host.onLoop(t, func() {
		d, x, _ = p.x11Window()
		r = x11LevelRoot(d)
		if p.environment().WindowTransparent != displayComposites() {
			t.Errorf("transparent=%v compositor=%v", p.environment().WindowTransparent, displayComposites())
		}
	})
	assertPlacement(driver.Size{Width: 180, Height: 80}, "initial")
	ctl.s.Post(func() { ctl.h.Set(180) })
	wait1394(t, "grow", func() bool {
		var width, height int32
		host.onLoop(t, func() { width, height = gtkWidgetGetWidth(p.surface), gtkWidgetGetHeight(p.surface) })
		return width == 180 && height == 180
	})
	assertPlacement(driver.Size{Width: 180, Height: 180}, "grown")
	ctl.s.Post(func() { ctl.h.Set(40) })
	wait1394(t, "shrink", func() bool {
		var width, height int32
		host.onLoop(t, func() { width, height = gtkWidgetGetWidth(p.surface), gtkWidgetGetHeight(p.surface) })
		return width == 180 && height == 40
	})
	assertPlacement(driver.Size{Width: 180, Height: 40}, "shrunk")
}
