//go:build linux && !android

package linux

import (
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
			if m {
				gtkWindowSetResizable(p.win, 1)
			}
		})
		return m
	})
	var d, x, r uintptr
	var before driver.Point
	host.onLoop(t, func() {
		d, x, _ = p.x11Window()
		r = x11LevelRoot(d)
		a, b := translate1394(d, x, r)
		before = driver.Point{X: driver.Points(a), Y: driver.Points(b)}
		if p.environment().WindowTransparent != displayComposites() {
			t.Errorf("transparent=%v compositor=%v", p.environment().WindowTransparent, displayComposites())
		}
	})
	ctl.s.Post(func() { ctl.h.Set(180) })
	wait1394(t, "grow", func() bool {
		var z, h int32
		host.onLoop(t, func() { gtkWindowGetDefaultSize(p.win, &z, &h) })
		return h >= 180
	})
	var after driver.Point
	host.onLoop(t, func() { a, b := translate1394(d, x, r); after = driver.Point{X: driver.Points(a), Y: driver.Points(b)} })
	if after.X != before.X {
		t.Errorf("panel left its right-side anchor from %v to %v", before, after)
	}
	ctl.s.Post(func() { ctl.h.Set(40) })
	wait1394(t, "shrink", func() bool {
		var z, h int32
		host.onLoop(t, func() { gtkWindowGetDefaultSize(p.win, &z, &h) })
		return h <= 45
	})
}
